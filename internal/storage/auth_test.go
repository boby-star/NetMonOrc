package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/httpapi"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestPostgresBrowserSessionsAndCSRF(t *testing.T) {
	path := os.Getenv("NETMONORC_TEST_DSN_FILE")
	if path == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	s, e := Open(ctx, strings.TrimSpace(string(b)))
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	username := "auth-" + newID()
	password := "test-password-unique-long"
	hash, e := auth.HashPassword(password)
	if e != nil {
		t.Fatal(e)
	}
	created, e := s.BootstrapUser(ctx, username, hash)
	if e != nil || !created {
		t.Fatal("isolated initial bootstrap", e)
	}
	if created, e = s.BootstrapUser(ctx, "replacement", hash); e != nil || created {
		t.Fatal("bootstrap overwrote identity", e)
	}
	service := auth.New(s)
	h, e := httpapi.NewWithSessions("service", nil, func(context.Context, string) (monitor.Assessment, bool, error) {
		return monitor.Assessment{Monitor: "service", At: time.Now().UTC(), State: "unknown"}, true, nil
	}, nil, service)
	if e != nil {
		t.Fatal(e)
	}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	client := server.Client()
	client.Jar, _ = cookiejar.New(nil)
	request := func(method, path string, body any, csrf, origin string) (int, []byte, []*http.Cookie) {
		t.Helper()
		var input io.Reader
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			input = bytes.NewReader(data)
		}
		r, e := http.NewRequest(method, server.URL+path, input)
		if e != nil {
			t.Fatal(e)
		}
		if body != nil {
			r.Header.Set("Content-Type", "application/json")
		}
		if csrf != "" {
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data, response.Cookies()
	}
	input := map[string]string{"username": username, "password": password}
	if code, _, _ := request("POST", "/api/v1/auth/login", input, "", "https://foreign.example"); code != 403 {
		t.Fatal("login CSRF accepted", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/login", map[string]string{"username": username, "password": "wrong"}, "", server.URL); code != 401 {
		t.Fatal("invalid password accepted", code)
	}
	code, data, cookies := request("POST", "/api/v1/auth/login", input, "", server.URL)
	if code != 200 || len(cookies) != 1 {
		t.Fatal("login failed", code)
	}
	cookie := cookies[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Path != "/" {
		t.Fatal("unsafe cookie")
	}
	var view auth.SessionView
	if e = json.Unmarshal(data, &view); e != nil || view.Role != "admin" || view.CSRFToken == "" {
		t.Fatal("invalid session view", e)
	}
	if strings.Contains(string(data), password) || strings.Contains(string(data), cookie.Value) {
		t.Fatal("credential disclosed")
	}
	if _, e = auth.New(s).Me(ctx, cookie.Value); e != nil {
		t.Fatal("session not durable across service restart", e)
	}
	if code, _, _ := request("GET", "/api/v1/monitors/service/assessment", nil, "", ""); code != 200 {
		t.Fatal("cookie cannot read assessment", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/logout", nil, "", server.URL); code != 403 {
		t.Fatal("logout without CSRF", code)
	}
	if code, _, _ := request("GET", "/api/v1/auth/me", nil, "", ""); code != 200 {
		t.Fatal("rejected CSRF revoked session", code)
	}
	if code, _, _ := request("POST", "/api/v1/auth/logout", nil, view.CSRFToken, server.URL); code != 204 {
		t.Fatal("logout failed", code)
	}
	if _, e = service.Me(ctx, cookie.Value); e != auth.ErrCredentials {
		t.Fatal("revoked cookie reusable", e)
	}
	// Failed-login state must survive a fresh Service instance.
	for i := 0; i < 5; i++ {
		_, _, e = service.Login(ctx, username, "wrong")
		if e != auth.ErrCredentials {
			t.Fatal("failure not recorded", e)
		}
	}
	if _, _, e = auth.New(s).Login(ctx, username, password); e != auth.ErrCredentials {
		t.Fatal("restart bypassed lockout", e)
	}
	if _, e = s.pool.Exec(ctx, "UPDATE nmo_users SET locked_until=NULL,failed_logins=0,role='viewer' WHERE username=$1", username); e != nil {
		t.Fatal(e)
	}
	v, raw, e := auth.New(s).Login(ctx, username, password)
	if e != nil || v.Role != "viewer" {
		t.Fatal("viewer login", e)
	}
	for _, scope := range v.Scopes {
		if scope == "resources:write" || scope == "system:admin" {
			t.Fatal("viewer escalated")
		}
	}
	if _, e = s.pool.Exec(ctx, "UPDATE nmo_browser_sessions SET expires_at=now()-interval '1 second' WHERE token_hash=$1", auth.HashToken(raw)); e != nil {
		t.Fatal(e)
	}
	if _, e = service.Me(ctx, raw); e != auth.ErrCredentials {
		t.Fatal("expired session accepted", e)
	}
}
