package probe

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

// HTTP performs one bounded policy-checked HTTP execution, including explicit redirects.
func HTTP(ctx context.Context, p monitor.Plan, c monitor.Check, s monitor.Slot, allowed []netip.Prefix) (monitor.Observation, error) {
	return httpWithRoots(ctx, p, c, s, allowed, nil)
}
func httpWithRoots(ctx context.Context, p monitor.Plan, c monitor.Check, s monitor.Slot, allowed []netip.Prefix, roots *x509.CertPool) (monitor.Observation, error) {
	if e := allowedHTTPCheck(c, allowed); e != nil {
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
	tr := &http.Transport{Proxy: nil, DialContext: policyHTTPDial(c, allowed), DisableKeepAlives: true, MaxResponseHeaderBytes: 16384, TLSHandshakeTimeout: c.Timeout}
	tr.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}
	defer tr.CloseIdleConnections()
	client := &http.Client{Transport: tr, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if !c.FollowRedirects {
			return http.ErrUseLastResponse
		}
		if len(via) >= 10 {
			return fmt.Errorf("HTTP redirect limit exceeded")
		}
		next := c
		next.URL = req.URL.String()
		origin, _ := url.Parse(c.URL)
		if !strings.EqualFold(req.URL.Hostname(), origin.Hostname()) {
			next.ConnectIP = ""
		}
		if req.URL.Scheme != origin.Scheme || !strings.EqualFold(req.URL.Host, origin.Host) {
			req.Header = make(http.Header)
		}
		if err := allowedHTTPCheck(next, allowed); err != nil {
			return err
		}
		return nil
	}}
	req, e := http.NewRequestWithContext(child, c.Method, u.String(), nil)
	if e != nil {
		return monitor.Observation{}, e
	}
	for key, value := range c.Headers {
		req.Header.Set(key, value)
	}
	response, e := client.Do(req)
	if e != nil {
		o.Reason = "target_request_failed"
		if errors.Is(e, errHTTPPolicy) {
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
			o.Reason = "target_policy_denied"
		}
		if ctx.Err() != nil {
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
			o.Reason = "cancelled"
		}
	} else {
		o.StatusCode = response.StatusCode
		body, readErr := io.ReadAll(io.LimitReader(response.Body, c.MaxBodyBytes+1))
		n := int64(len(body))
		if n > c.MaxBodyBytes {
			readErr = fmt.Errorf("body limit exceeded")
		}
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil {
			o.Execution = "execution_failed"
			o.Outcome = "unknown"
			o.Reason = "response_read_failed"
		} else {
			o.Values, o.ExtractionStatus = extract(body, c.Extract)
			o.Reason = "unexpected_status"
			for _, status := range c.ExpectedStatus {
				if status == response.StatusCode {
					o.Outcome = "success"
					o.Reason = "expected_status"
				}
			}
			if o.Outcome == "success" && c.BodyMatchesRegex != "" {
				pattern, err := regexp.Compile(c.BodyMatchesRegex)
				if err != nil {
					o.Execution = "execution_failed"
					o.Outcome = "unknown"
					o.Reason = "invalid_body_regex"
				} else if !pattern.Match(body) {
					o.Outcome = "failure"
					o.Reason = "http_body_mismatch"
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
	return allowedAddress(ip, allowed)
}

func allowedAddress(ip netip.Addr, allowed []netip.Prefix) error {
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
