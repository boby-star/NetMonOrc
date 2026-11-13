package webhook

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPSBearerAndNoRedirect(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer private-test-value" || r.Header.Get("X-NetMonOrc-Event-ID") != "same-event" {
			t.Error("missing authentication/id")
		}
		w.Header().Set("Location", "/forbidden-follow")
		w.WriteHeader(307)
	}))
	defer server.Close()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret"), []byte("private-test-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	ca := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
		t.Fatal(err)
	}
	e := Endpoint{URL: server.URL, Enabled: true, AuthType: "bearer", SecretRef: "secret", Timeout: time.Second}
	allow := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	// A hostile process proxy must not receive webhook credentials.
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:1")
	s, err := New(e, allow, dir, ca)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	status, err := s.Deliver(context.Background(), "same-event", []byte(`{}`))
	if err != nil || status != 307 || calls.Load() != 1 {
		t.Fatalf("redirect/proxy: %d %v %d", status, err, calls.Load())
	}
	untrusted, err := New(e, allow, dir, "")
	if err != nil {
		t.Fatal(err)
	}
	defer untrusted.Close()
	if _, err := untrusted.Deliver(context.Background(), "same-event", []byte(`{}`)); err == nil {
		t.Fatal("untrusted certificate accepted")
	}
	if err := os.Chmod(filepath.Join(dir, "secret"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := New(e, allow, dir, ca); err == nil {
		t.Fatal("public bearer file accepted")
	}
}

func TestDestinationAndBackoffBounds(t *testing.T) {
	allow := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0"), netip.MustParsePrefix("::/0")}
	for _, raw := range []string{"http://127.0.0.1", "https://user:pass@127.0.0.1", "https://169.254.169.254", "https://[::ffff:169.254.169.254]", "https://100.100.100.200", "https://[fd00:ec2::254]", "https://168.63.129.16", "https://example.com", "https://127.0.0.1:99999", "https://127.0.0.1/#fragment"} {
		if Allowed(raw, allow) == nil {
			t.Errorf("accepted %s", raw)
		}
	}
	if Allowed("https://192.0.2.1", []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}) == nil {
		t.Fatal("allowlist ignored")
	}
	e := Endpoint{InitialBackoff: time.Minute, MaxBackoff: time.Second}
	for n := 1; n < 20; n++ {
		if d := e.Backoff(n); d < time.Second/2 || d > time.Second {
			t.Fatalf("backoff out of bounds: %s", d)
		}
	}
}
