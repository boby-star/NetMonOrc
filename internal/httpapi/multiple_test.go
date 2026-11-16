package httpapi

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func TestAssessmentRoutesKeepMonitorRevisionIsolation(t *testing.T) {
	token := []byte(strings.Repeat("m", 43))
	revisions := map[string]string{"first": "r1", "second": "r2"}
	stale := false
	h, err := New("first", token, func(_ context.Context, name string) (monitor.Assessment, bool, error) {
		state := "healthy"
		if name == "second" {
			state = "down"
		}
		revision := revisions[name]
		if stale {
			revision = "old"
		}
		return monitor.Assessment{Monitor: name, Revision: revision, State: state, At: time.Now().UTC()}, true, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.SetMonitorRevisions(revisions)
	for _, name := range []string{"second", "first", "missing"} {
		r := httptest.NewRequest("GET", "/api/v1/monitors/"+name+"/assessment", nil)
		r.Header.Set("Authorization", "Bearer "+string(token))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if name == "missing" {
			if w.Code != 404 {
				t.Fatal(w.Code)
			}
			continue
		}
		var a Assessment
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &a) != nil || a.MonitorID != name || a.Revision != revisions[name] {
			t.Fatal("routing mixed Monitor state", w.Code, w.Body.String())
		}
	}
	stale = true
	r := httptest.NewRequest("GET", "/api/v1/monitors/second/assessment", nil)
	r.Header.Set("Authorization", "Bearer "+string(token))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 503 {
		t.Fatal("old revision exposed as live", w.Code)
	}
	if h.monitor != "first" {
		t.Fatal("request changed shared routing state")
	}
}
