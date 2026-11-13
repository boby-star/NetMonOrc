// Package webhook performs bounded HTTPS delivery to an explicit IP allowlist.
package webhook

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type Endpoint struct {
	Name           string
	URL            string
	Enabled        bool
	AuthType       string
	SecretRef      string
	Timeout        time.Duration
	MaxAttempts    int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
}

func (e Endpoint) Hash() string {
	b, _ := json.Marshal(e)
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}
func (e Endpoint) Backoff(attempt int) time.Duration {
	d := min(e.InitialBackoff, e.MaxBackoff)
	for i := 1; i < attempt && d < e.MaxBackoff; i++ {
		d = min(d*2, e.MaxBackoff)
	}
	return time.Duration(float64(d) * (.5 + rand.Float64()*.5))
}
func Allowed(raw string, allowed []netip.Prefix) error {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return fmt.Errorf("webhook requires an HTTPS URL without credentials or fragment")
	}
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid webhook port")
		}
	}
	ip, e := netip.ParseAddr(u.Hostname())
	if e != nil || ip.Zone() != "" {
		return fmt.Errorf("research webhook requires an IP-literal URL")
	}
	ip = ip.Unmap()
	if ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.String() == "100.100.100.200" || ip.String() == "fd00:ec2::254" || ip.String() == "168.63.129.16" {
		return fmt.Errorf("forbidden webhook address")
	}
	for _, p := range allowed {
		if p.Contains(ip) {
			return nil
		}
	}
	return fmt.Errorf("webhook is outside explicit allowed CIDRs")
}

type Sender struct {
	endpoint  Endpoint
	client    *http.Client
	transport *http.Transport
	token     string
	allowed   []netip.Prefix
}

func New(e Endpoint, allowed []netip.Prefix, secretDir, caFile string) (*Sender, error) {
	if err := Allowed(e.URL, allowed); err != nil {
		return nil, err
	}
	var token string
	if e.AuthType == "bearer" {
		if secretDir == "" || e.SecretRef == "" || filepath.Base(e.SecretRef) != e.SecretRef {
			return nil, fmt.Errorf("explicit webhook secret directory and local reference required")
		}
		fd, err := unix.Open(filepath.Join(secretDir, e.SecretRef), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0)
		if err != nil {
			return nil, fmt.Errorf("webhook secret file unavailable")
		}
		f := os.NewFile(uintptr(fd), "webhook secret")
		info, err := f.Stat()
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
			f.Close()
			return nil, fmt.Errorf("webhook secret must be a private regular file")
		}
		b, err := io.ReadAll(io.LimitReader(f, 4097))
		closeErr := f.Close()
		if err != nil || closeErr != nil || len(b) > 4096 {
			return nil, fmt.Errorf("webhook secret unreadable or too large")
		}
		token = strings.TrimSpace(string(b))
		if token == "" {
			return nil, fmt.Errorf("empty webhook secret")
		}
		for _, c := range token {
			if c < 33 || c > 126 {
				return nil, fmt.Errorf("invalid webhook bearer secret")
			}
		}
	}
	roots, err := x509.SystemCertPool()
	if err != nil {
		roots = x509.NewCertPool()
	}
	if caFile != "" {
		b, err := os.ReadFile(caFile)
		if err != nil || !roots.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("invalid webhook CA bundle")
		}
	}
	tr := &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: e.Timeout}).DialContext, DisableKeepAlives: true, MaxResponseHeaderBytes: 16384, TLSHandshakeTimeout: e.Timeout, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: roots}}
	client := &http.Client{Transport: tr, Timeout: e.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Sender{e, client, tr, token, append([]netip.Prefix(nil), allowed...)}, nil
}
func (s *Sender) Close() { s.transport.CloseIdleConnections() }
func (s *Sender) Deliver(ctx context.Context, eventID string, payload []byte) (int, error) {
	if !s.endpoint.Enabled {
		return 0, fmt.Errorf("webhook disabled")
	}
	if err := Allowed(s.endpoint.URL, s.allowed); err != nil {
		return 0, err
	}
	if len(payload) > 1<<20 {
		return 0, fmt.Errorf("webhook payload exceeds bound")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.endpoint.URL, bytes.NewReader(payload))
	if err != nil {
		return 0, fmt.Errorf("invalid webhook request")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-NetMonOrc-Event-ID", eventID)
	if s.token != "" {
		req.Header.Set("Authorization", "Bearer "+s.token)
	}
	response, err := s.client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("webhook transport failed")
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	_ = response.Body.Close()
	return response.StatusCode, nil
}
