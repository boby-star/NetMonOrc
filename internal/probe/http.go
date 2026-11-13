package probe

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// HTTP performs exactly one bounded request. This first slice accepts literal
// IP targets only, so policy validation cannot race a second DNS resolution.
// No environment proxy, redirects, cookies or insecure TLS are permitted.
func HTTP(ctx context.Context, p monitor.Plan, c monitor.Check, s monitor.Slot, allowed []netip.Prefix) (monitor.Observation, error) {
	if e := allowedTarget(c.URL, allowed); e != nil {
		return monitor.Observation{}, e
	}
	u, e := url.Parse(c.URL)
	if e != nil {
		return monitor.Observation{}, e
	}
	var id [16]byte
	if _, e = rand.Read(id[:]); e != nil {
		return monitor.Observation{}, e
	}
	o := monitor.Observation{ID: hex.EncodeToString(id[:]), Monitor: p.Name, Revision: p.Revision, Check: c.ID, Agent: s.Agent, Zone: s.Zone, StartedAt: time.Now().UTC(), Execution: "completed", Outcome: "failure"}
	child, cancel := context.WithTimeout(ctx, c.Timeout)
	defer cancel()
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: c.Timeout}).DialContext, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384, TLSHandshakeTimeout: c.Timeout}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	req, e := http.NewRequestWithContext(child, c.Method, u.String(), nil)
	if e != nil {
		return monitor.Observation{}, e
	}
	response, e := client.Do(req)
	if e != nil {
		o.Reason = "target_request_failed"
		if ctx.Err() != nil {
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
			o.Reason = "cancelled"
		}
	} else {
		o.StatusCode = response.StatusCode
		n, readErr := io.Copy(io.Discard, io.LimitReader(response.Body, c.MaxBodyBytes+1))
		if n > c.MaxBodyBytes {
			readErr = fmt.Errorf("body limit exceeded")
		}
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
			o.Reason = "response_read_failed"
		} else {
			o.Reason = "unexpected_status"
			for _, status := range c.ExpectedStatus {
				if status == response.StatusCode {
					o.Outcome = "success"
					o.Reason = "expected_status"
				}
			}
		}
	}
	o.FinishedAt = time.Now().UTC()
	return o, nil
}

func allowedTarget(raw string, allowed []netip.Prefix) error {
	u, e := url.Parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("invalid HTTP target")
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if e != nil {
		return fmt.Errorf("first slice requires IP-literal probe URLs")
	}
	ip = ip.Unmap()
	permitted := false
	for _, n := range allowed {
		if n.Contains(ip) {
			permitted = true
		}
	}
	if !permitted || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return fmt.Errorf("probe target is outside explicit allowed CIDRs or is a forbidden address")
	}
	return nil
}
