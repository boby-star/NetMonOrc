package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestReadOnlyContractAndFreshness(t *testing.T) {
	token := []byte(strings.Repeat("a", 43))
	now := time.Now().UTC()
	a := monitor.Assessment{Monitor: "service", State: "healthy", Revision: "r", At: now, Zones: []monitor.ZoneAssessment{{Zone: "office", State: "healthy", Coverage: "complete", Evidence: []string{"id"}}}}
	calls := 0
	h, err := New("service", token, func(context.Context, string) (monitor.Assessment, bool, error) { calls++; return a, true, nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, auth string
		code               int
	}{{"GET", "/api/v1/monitors/service/assessment", "", 401}, {"GET", "/api/v1/monitors/service/assessment", "wrong", 401}, {"GET", "/api/v1/monitors/other/assessment", string(token), 404}, {"POST", "/api/v1/monitors/service/assessment", string(token), 405}, {"GET", "/api/v1/monitors/service/assessment?secret=x", string(token), 400}, {"GET", "/api/v1/monitors/service/assessment", string(token), 200}} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.auth != "" {
			r.Header.Set("Authorization", "Bearer "+tc.auth)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body)
		}
		if w.Code != 200 {
			var body struct {
				Error struct {
					RequestID string `json:"requestId"`
				} `json:"error"`
			}
			if json.Unmarshal(w.Body.Bytes(), &body) != nil || body.Error.RequestID == "" {
				t.Fatal("missing error envelope")
			}
		} else {
			var body Assessment
			if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.MonitorID != "service" || body.CoverageState != "sufficient" || body.EvaluatedAt.IsZero() {
				t.Fatal("OpenAPI shape lost", body, err)
			}
		}
	}
	if calls != 1 {
		t.Fatal("unauthorized request accessed storage")
	}
	lower := httptest.NewRequest("GET", "/api/v1/monitors/service/assessment", nil)
	lower.Header.Set("Authorization", "bearer "+string(token))
	lowerResponse := httptest.NewRecorder()
	h.ServeHTTP(lowerResponse, lower)
	if lowerResponse.Code != 200 {
		t.Fatal("Bearer scheme must be case insensitive")
	}
	h.readSlots <- struct{}{}
	busyRequest := httptest.NewRequest("GET", "/api/v1/monitors/service/assessment", nil)
	busyRequest.Header.Set("Authorization", "Bearer "+string(token))
	busyResponse := httptest.NewRecorder()
	h.ServeHTTP(busyResponse, busyRequest)
	<-h.readSlots
	if busyResponse.Code != 429 || busyResponse.Header().Get("Retry-After") != "1" {
		t.Fatal("API reader admission bound ignored")
	}
	a.At = now.Add(-10 * time.Second)
	r := httptest.NewRequest("GET", "/api/v1/monitors/service/assessment", nil)
	r.Header.Set("Authorization", "Bearer "+string(token))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("stale assessment exposed as live")
	}
}
func TestPrivateTokenFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(file, []byte(strings.Repeat("a", 43)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(file); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(file); err == nil {
		t.Fatal("public token accepted")
	}
	link := file + "-link"
	if err := os.Symlink(file, link); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(link); err == nil {
		t.Fatal("symlink token accepted")
	}
}
