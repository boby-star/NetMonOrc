package probe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// Exporter owns a prepared external binary, a private config and a loopback
// listener. Manifest content cannot supply executable paths or shell arguments.
type Exporter struct {
	endpoint string
	stop     func()
}

func (e *Exporter) Close() { e.stop() }

func StartExporter(ctx context.Context, binary string, p monitor.Plan) (*Exporter, error) {
	modules := map[string]any{}
	for _, c := range p.Checks {
		if c.Runner == "blackbox" {
			module, err := blackboxModule(p, c)
			if err != nil {
				return nil, err
			}
			modules[c.ID] = module
		}
	}
	b, err := json.Marshal(map[string]any{"modules": modules})
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "netmonorc-blackbox-")
	if err != nil {
		return nil, err
	}
	clean := func() { _ = os.RemoveAll(dir) }
	path := dir + "/blackbox.json"
	if err = os.WriteFile(path, b, 0600); err != nil {
		clean()
		return nil, err
	}
	child, cancel := context.WithCancel(ctx)
	cmd := exec.CommandContext(child, binary, "--config.file="+path, "--web.listen-address=127.0.0.1:0", "--timeout-offset=0", "--log.level=debug", "--log.prober=debug")
	cmd.WaitDelay = time.Second
	logs := &listenLog{ready: make(chan string, 1)}
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err = cmd.Start(); err != nil {
		cancel()
		clean()
		return nil, fmt.Errorf("start exporter: %w", err)
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	var once sync.Once
	stop := func() { once.Do(func() { cancel(); <-done; clean() }) }
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case address := <-logs.ready:
		return &Exporter{endpoint: "http://" + address, stop: stop}, nil
	case <-done:
		stop()
		return nil, fmt.Errorf("exporter exited before readiness (check binary/config compatibility)")
	case <-timer.C:
		stop()
		return nil, fmt.Errorf("exporter startup timed out")
	case <-ctx.Done():
		stop()
		return nil, ctx.Err()
	}
}

// Capture only the listening address, never echo arbitrary subprocess logs.
type listenLog struct {
	mu    sync.Mutex
	text  string
	ready chan string
}

var listenAddress = regexp.MustCompile(`address="?(127\.0\.0\.1:[0-9]+)`)

func (l *listenLog) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.text += string(b)
	if len(l.text) > 65536 {
		l.text = l.text[len(l.text)-65536:]
	}
	if m := listenAddress.FindStringSubmatch(l.text); len(m) == 2 {
		select {
		case l.ready <- m[1]:
		default:
		}
	}
	return len(b), nil
}

func (e *Exporter) Run(ctx context.Context, p monitor.Plan, c monitor.Check, s monitor.Slot, allowed []netip.Prefix) (monitor.Observation, error) {
	target, err := validateTarget(p, c, allowed)
	if err != nil {
		return monitor.Observation{}, err
	}
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return monitor.Observation{}, err
	}
	o := monitor.Observation{ID: hex.EncodeToString(id[:]), Monitor: p.Name, Revision: p.Revision, Check: c.ID, Agent: s.Agent, Zone: s.Zone, StartedAt: time.Now().UTC(), Execution: "execution_failed", Outcome: "unknown", Reason: "exporter_request_failed"}
	child, cancel := context.WithTimeout(ctx, c.Timeout+time.Second)
	defer cancel()
	query := url.Values{"module": {c.ID}, "target": {target}}
	if c.Type == "icmp" {
		query.Set("debug", "true")
	}
	u := e.endpoint + "/probe?" + query.Encode()
	req, err := http.NewRequestWithContext(child, "GET", u, nil)
	if err != nil {
		return o, err
	}
	tr := &http.Transport{Proxy: nil, MaxResponseHeaderBytes: 16384}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err == nil {
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, (1<<20)+1))
		closeErr := resp.Body.Close()
		if resp.StatusCode == 200 && readErr == nil && closeErr == nil && len(b) <= 1<<20 {
			logs := ""
			metrics := string(b)
			var debugErr error
			if c.Type == "icmp" {
				logs, metrics, debugErr = icmpDebug(metrics)
			}
			success, status, parseErr := parseMetrics(metrics, c.Type)
			regexFailure, regexErr := booleanMetric(metrics, "probe_failed_due_to_regex")
			if regexErr != nil {
				parseErr = regexErr
			}
			if debugErr != nil {
				parseErr = debugErr
			}
			if parseErr == nil {
				o.Execution = "completed"
				o.StatusCode = status
				o.Outcome = "failure"
				o.Reason = "negative_probe"
				if success {
					o.Outcome = "success"
					o.Reason = "positive_probe"
				}
				// With this restricted module, a failed probe after an accepted
				// status can indicate a truncated/bounded response. Do not turn
				// incomplete response handling into a confirmed service failure.
				if !success && c.Type == "http" {
					for _, expected := range c.ExpectedStatus {
						if status == expected && !(regexFailure && c.BodyMatchesRegex != "") {
							o.Execution = "execution_failed"
							o.Outcome = "unknown"
							o.Reason = "incomplete_http_probe"
						}
					}
				}
				if c.Type == "icmp" && !success && !icmpNegative(logs) {
					o.Execution = "execution_failed"
					o.Outcome = "unknown"
					o.Reason = "icmp_execution_unavailable"
				}
			} else {
				o.Reason = "invalid_exporter_metrics"
			}
		}
	}
	o.FinishedAt = time.Now().UTC()
	return o, nil
}

func parseMetrics(body, kind string) (bool, int, error) {
	found := false
	success := false
	status := 0
	statusFound := false
	for _, line := range strings.Split(body, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || strings.HasPrefix(f[0], "#") {
			continue
		}
		if f[0] == "probe_success" {
			if found || len(f) != 2 || (f[1] != "0" && f[1] != "1") {
				return false, 0, fmt.Errorf("invalid probe_success")
			}
			found = true
			success = f[1] == "1"
		}
		if f[0] == "probe_http_status_code" {
			if statusFound || len(f) != 2 {
				return false, 0, fmt.Errorf("invalid status metric")
			}
			n, err := strconv.ParseFloat(f[1], 64)
			if err != nil || n < 0 || n > 599 || n != float64(int(n)) {
				return false, 0, fmt.Errorf("invalid status code")
			}
			status = int(n)
			statusFound = true
		}
	}
	if !found || (kind == "http" && !statusFound) {
		return false, 0, fmt.Errorf("missing probe_success")
	}
	return success, status, nil
}

func booleanMetric(body, name string) (bool, error) {
	found := false
	value := false
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != name {
			continue
		}
		if found || len(fields) != 2 || (fields[1] != "0" && fields[1] != "1") {
			return false, fmt.Errorf("invalid boolean metric")
		}
		found = true
		value = fields[1] == "1"
	}
	return value, nil
}
