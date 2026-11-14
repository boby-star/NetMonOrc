package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/boby-star/NetMonOrc/internal/auth"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resources"
)

func testApplyUseCases(t *testing.T, ctx context.Context, s *Store, server *httptest.Server, client *http.Client, session auth.SessionView) {
	t.Helper()
	request := func(method, path string, body any) (int, []byte) {
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
		r.Header.Set("Origin", server.URL)
		r.Header.Set("X-CSRF-Token", session.CSRFToken)
		response, e := client.Do(r)
		if e != nil {
			t.Fatal(e)
		}
		defer response.Body.Close()
		data, e := io.ReadAll(response.Body)
		if e != nil {
			t.Fatal(e)
		}
		return response.StatusCode, data
	}
	current, _, e := s.Resource(ctx, "service")
	if e != nil {
		t.Fatal(e)
	}
	original := current.Manifest
	inactive := original
	inactive.Metadata.Name = "batch-new"
	stale := "stale-version"
	batch := resources.Batch{Manifests: []resources.Manifest{inactive, original}, ExpectedVersions: map[string]*string{"Monitor/batch-new": nil, "Monitor/service": &stale}, RequestID: "apply-atomic"}
	if code, _ := request("POST", "/api/v1/config/apply", batch); code != 412 {
		t.Fatal("conflicting batch accepted", code)
	}
	if _, found, e := s.Resource(ctx, "batch-new"); e != nil || found {
		t.Fatal("partial apply escaped rollback", e)
	}
	var count int
	if e = s.pool.QueryRow(ctx, `SELECT count(*) FROM nmo_operations WHERE request_key='apply-atomic'`).Scan(&count); e != nil || count != 0 {
		t.Fatal("failed batch created operation", e)
	}
	batch.ExpectedVersions["Monitor/service"] = &current.ResourceVersion
	code, data := request("POST", "/api/v1/config/apply", batch)
	var op resources.Operation
	if code != 202 || json.Unmarshal(data, &op) != nil || op.State != "accepted" || len(op.Result.Targets) != 2 {
		t.Fatal("batch admission", code)
	}
	admitted, _, e := s.Resource(ctx, "service")
	if e != nil {
		t.Fatal(e)
	}
	code, data = request("POST", "/api/v1/config/apply", batch)
	var replay resources.Operation
	if code != 202 || json.Unmarshal(data, &replay) != nil || replay.ID != op.ID {
		t.Fatal("idempotency recovery", code)
	}
	unchanged, _, e := s.Resource(ctx, "service")
	if e != nil || unchanged.ResourceVersion != admitted.ResourceVersion {
		t.Fatal("replay applied twice", e)
	}
	changed := batch
	changed.Manifests = []resources.Manifest{original}
	if code, _ = request("POST", "/api/v1/config/apply", changed); code != 409 {
		t.Fatal("idempotency key reused for another body", code)
	}
	slots := []monitor.Slot{{Agent: "apply-agent", Zone: "office"}}
	generation, e := s.BeginSession(ctx, "apply-agent")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.Applied(ctx, "apply-agent", generation, admitted.Revision); e != nil {
		t.Fatal(e)
	}
	proof := []monitor.AppliedAssignment{{Agent: "apply-agent", Zone: "office", Revision: admitted.Revision, Generation: generation}}
	if e = s.ReconcileOperations(ctx, "service", admitted.Revision, slots, proof); e != nil {
		t.Fatal(e)
	}
	pending, _, e := s.Operation(ctx, op.ID)
	if e != nil || pending.State != "running" || pending.Result.Targets[0].Ready {
		t.Fatal("unselected Monitor falsely completed", e)
	}
	single := resources.Batch{Manifests: []resources.Manifest{original}, ExpectedVersions: map[string]*string{"Monitor/service": &admitted.ResourceVersion}, RequestID: "apply-ready"}
	code, data = request("POST", "/api/v1/config/apply", single)
	var ready resources.Operation
	if code != 202 || json.Unmarshal(data, &ready) != nil {
		t.Fatal("single active batch", code)
	}
	newerGeneration, e := s.BeginSession(ctx, "apply-agent")
	if e != nil {
		t.Fatal(e)
	}
	if e = s.ReconcileOperations(ctx, "service", admitted.Revision, slots, proof); e != nil {
		t.Fatal(e)
	}
	pending, _, e = s.Operation(ctx, ready.ID)
	if e != nil || pending.State == "completed" {
		t.Fatal("fenced stream acknowledged operation", e)
	}
	superseded, _, e := s.Operation(ctx, op.ID)
	if e != nil || superseded.State != "failed" || superseded.Error == nil || superseded.Error.Code != "CONFIG_SUPERSEDED" {
		t.Fatal("superseded operation not terminated", e)
	}
	if e = s.Applied(ctx, "apply-agent", newerGeneration, admitted.Revision); e != nil {
		t.Fatal(e)
	}
	proof[0].Generation = newerGeneration
	if e = s.ReconcileOperations(ctx, "service", admitted.Revision, slots, proof); e != nil {
		t.Fatal(e)
	}
	code, data = request("GET", "/api/v1/operations/"+ready.ID, nil)
	if code != 200 || json.Unmarshal(data, &ready) != nil || ready.State != "completed" || ready.FinishedAt == nil || !ready.Result.Targets[0].Ready {
		t.Fatal("operation did not complete after current ACK", code)
	}
	code, data = request("POST", "/api/v1/config/apply", single)
	if code != 202 || json.Unmarshal(data, &replay) != nil || replay.ID != ready.ID || replay.State != "completed" {
		t.Fatal("recovery lost durable completed state", code)
	}
	code, data = request("GET", "/api/v1/operations?state=completed&limit=1", nil)
	var page resources.OperationPage
	if code != 200 || json.Unmarshal(data, &page) != nil || len(page.Items) != 1 || page.Items[0].ID != ready.ID {
		t.Fatal("operation filter", code)
	}
	if _, found, e := s.Resource(ctx, "batch-new"); e != nil || !found {
		t.Fatal("apply implicitly pruned another resource", e)
	}
}
