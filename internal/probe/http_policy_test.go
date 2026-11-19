package probe

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func policyCheck(raw string) monitor.Check {
	return monitor.Check{ID: "http", Type: "http", Runner: "native-http", URL: raw, Method: "GET", Timeout: time.Second, MaxBodyBytes: 16384, ExpectedStatus: []int{200}}
}
func TestConnectIPPreservesHostSNIAndCertificateVerification(t *testing.T) {
	var host, sni string
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { host = r.Host; w.WriteHeader(200) }))
	server.TLS = &tls.Config{GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) { sni = info.ServerName; return nil, nil }}
	server.StartTLS()
	defer server.Close()
	u, _ := url.Parse(server.URL)
	_, port, _ := net.SplitHostPort(u.Host)
	roots := x509.NewCertPool()
	roots.AddCert(server.Certificate())
	c := policyCheck("https://example.com:" + port + "/")
	c.ConnectIP = "127.0.0.1"
	allowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	o, err := httpWithRoots(context.Background(), monitor.Plan{Name: "tls", Revision: "r"}, c, monitor.Slot{Agent: "a", Zone: "z"}, allowed, roots)
	if err != nil || o.Outcome != "success" || sni != "example.com" || host != "example.com:"+port {
		t.Fatal("IP pin lost Host/SNI or TLS trust", o, host, sni, err)
	}
	c.URL = "https://wrong.invalid:" + port + "/"
	o, err = httpWithRoots(context.Background(), monitor.Plan{Name: "tls", Revision: "r"}, c, monitor.Slot{Agent: "a", Zone: "z"}, allowed, roots)
	if err != nil || o.Outcome != "failure" {
		t.Fatal("connectIP bypassed certificate name verification", o, err)
	}
}
func TestRedirectPolicyAndHeaderForwarding(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/end", http.StatusFound)
			return
		}
		if r.Header.Get("X-Probe") != "safe" {
			t.Error("configured same-origin header missing")
		}
		hits.Add(1)
		w.WriteHeader(200)
	}))
	defer server.Close()
	c := policyCheck(server.URL + "/start")
	c.Headers = map[string]string{"X-Probe": "safe"}
	allowed := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}
	p := monitor.Plan{Name: "redirect", Revision: "r"}
	slot := monitor.Slot{Agent: "a", Zone: "z"}
	o, err := HTTP(context.Background(), p, c, slot, allowed)
	if err != nil || o.StatusCode != 302 || hits.Load() != 0 {
		t.Fatal("redirect followed implicitly", o, err)
	}
	c.FollowRedirects = true
	o, err = HTTP(context.Background(), p, c, slot, allowed)
	if err != nil || o.Outcome != "success" || hits.Load() != 1 {
		t.Fatal("explicit redirect failed", o, err)
	}
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "http://127.0.0.2:1/", 302) }))
	defer denied.Close()
	c.URL = denied.URL
	o, err = HTTP(context.Background(), p, c, slot, allowed)
	if err != nil || o.Execution != "execution_failed" || o.Outcome != "unknown" || o.Reason != "target_policy_denied" {
		t.Fatal("denied redirect became a service failure or dial", o, err)
	}
}
