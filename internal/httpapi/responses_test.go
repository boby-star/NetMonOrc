package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/responseview"
)

func TestResponseHistoryAuthenticationAndReadOnly(t *testing.T) {
	token := strings.Repeat("r", 40)
	handler, err := New("web", []byte(token), func(context.Context, string) (monitor.Assessment, bool, error) {
		return monitor.Assessment{}, false, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	handler.SetResponses(func(_ context.Context, name string) ([]responseview.Event, error) {
		calls++
		if name != "web" {
			t.Fatal(name)
		}
		return []responseview.Event{{ID: "response", Status: "applied"}}, nil
	})
	for _, test := range []struct {
		token, query string
		status       int
	}{{"", "", 401}, {token, "", 200}, {token, "?limit=999", 400}} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/monitors/web/responses"+test.query, nil)
		if test.token != "" {
			request.Header.Set("Authorization", "Bearer "+test.token)
		}
		result := httptest.NewRecorder()
		handler.ServeHTTP(result, request)
		if result.Code != test.status {
			t.Fatalf("status=%d body=%s", result.Code, result.Body.String())
		}
	}
	if calls != 1 {
		t.Fatal("unauthorized/invalid request reached history", calls)
	}
}
