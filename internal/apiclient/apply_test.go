package apiclient

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/resources"
)

func TestApplyRecoversInterruptedCommittedResponse(t *testing.T) {
	var attempts atomic.Int32
	var committed atomic.Int32
	var saved atomic.Value
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		if err != nil {
			t.Error(err)
			return
		}
		var batch resources.Batch
		if r.URL.Path != "/api/v1/config/apply" || json.Unmarshal(b, &batch) != nil || batch.RequestID != "stable-recovery" {
			t.Error("invalid retry contract")
			w.WriteHeader(400)
			return
		}
		if attempts.Add(1) == 1 {
			committed.Add(1)
			saved.Store(string(b))
			conn, _, err := w.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			conn.Close()
			return
		}
		if saved.Load().(string) != string(b) {
			t.Error("retry changed admitted body")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"same-operation","state":"accepted"}`))
	}))
	defer server.Close()
	client := &Client{server: server.URL, http: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	data, err := client.Apply(ctx, resources.Batch{RequestID: "stable-recovery"})
	if err != nil || string(data) != `{"id":"same-operation","state":"accepted"}` || attempts.Load() != 2 || committed.Load() != 1 {
		t.Fatal("interrupted response recovery", err, attempts.Load())
	}
}
func TestApplyDoesNotRetryPreconditionOrTrustFailures(t *testing.T) {
	var attempts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts.Add(1)
		w.WriteHeader(412)
		_, _ = w.Write([]byte(`{"error":{"code":"PRECONDITION_FAILED"}}`))
	}))
	defer server.Close()
	client := &Client{server: server.URL, http: server.Client()}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := client.Apply(ctx, resources.Batch{RequestID: "stable-recovery"}); err == nil || attempts.Load() != 1 {
		t.Fatal("precondition retried", err)
	}
	untrusted := &Client{server: server.URL, http: &http.Client{Timeout: time.Second}}
	if _, err := untrusted.Apply(ctx, resources.Batch{RequestID: "stable-recovery"}); err == nil {
		t.Fatal("untrusted TLS accepted")
	}
	if attempts.Load() != 1 {
		t.Fatal("untrusted request reached receiver")
	}
	if _, err := client.Apply(ctx, resources.Batch{}); err == nil {
		t.Fatal("apply without idempotency allowed")
	}
}
