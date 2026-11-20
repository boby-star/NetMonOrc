package transport

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/protobuf/proto"
)

func TestComponentReadinessFencesAdmissionAndLiveProjection(t *testing.T) {
	now := time.Now()
	p := monitor.Plan{Name: "service", Revision: strings.Repeat("a", 64), Components: map[string]monitor.ComponentBinding{"branch": {Ref: "exporter", Hash: strings.Repeat("b", 64), Enabled: true}}}
	applied := map[string]bool{p.Revision: true, "withdrawn": true}
	report := &pb.ComponentReadiness{Revision: p.Revision, ComponentRef: "exporter", BindingHash: strings.Repeat("b", 64), State: "ready", Reason: "endpoint_ready", CheckedAtUnixMilli: now.UnixMilli()}
	reports, err := validateComponentReports("branch", []monitor.Plan{p}, applied, []*pb.ComponentReadiness{report}, now)
	if err != nil {
		t.Fatal(err)
	}
	s := admissionServer()
	s.Plan = p
	v := s.active["branch"]
	v.appliedRevisions = applied
	v.certExpiresAt = now.Add(time.Hour)
	v.components = reports
	s.active["branch"] = v
	runner := DiagnosticRunner{Server: s, Plan: p}
	action := diagnostics.Action{Check: monitor.Check{Runner: "blackbox"}}
	_, release, err := runner.Admit(context.Background(), action, diagnostics.Executor{ID: "branch"})
	if err != nil {
		t.Fatal("fresh readiness rejected", err)
	}
	release()
	if !s.ComponentReady(p, "branch", now) {
		t.Fatal("ready binding denied")
	}
	if statuses := s.AgentStatuses(now); len(statuses) != 0 { // admission fixture has no roster
		t.Fatal("unexpected roster")
	}
	s.Slots = []monitor.Slot{{Agent: "branch", Zone: "office"}}
	statuses := s.AgentStatuses(now.Add(16 * time.Second))
	if statuses[0].Components[0].State != "unknown" || s.ComponentReady(p, "branch", now.Add(16*time.Second)) {
		t.Fatal("stale readiness stayed ready")
	}
	report.State = "unavailable"
	report.Reason = "endpoint_unavailable"
	reports, err = validateComponentReports("branch", []monitor.Plan{p}, applied, []*pb.ComponentReadiness{report}, now)
	if err != nil {
		t.Fatal(err)
	}
	v.components = reports
	s.active["branch"] = v
	if _, _, err = runner.Admit(context.Background(), action, diagnostics.Executor{ID: "branch"}); err == nil || len(s.diagnosticAdmissions) != 0 {
		t.Fatal("unavailable runner admitted a physical task")
	}
	if s.AgentStatuses(now)[0].Components[0].State != "unavailable" {
		t.Fatal("public endpoint availability missing")
	}
	// A new session cannot inherit the previous session's positive sample.
	v.components = nil
	s.active["branch"] = v
	if s.ComponentReady(p, "branch", now) || s.AgentStatuses(now)[0].Components[0].State != "unknown" {
		t.Fatal("missing report inferred readiness")
	}
	for _, change := range []func(*pb.ComponentReadiness){
		func(r *pb.ComponentReadiness) { r.BindingHash = "forged" },
		func(r *pb.ComponentReadiness) { r.ComponentRef = "other" },
		func(r *pb.ComponentReadiness) { r.Revision = "unapplied" },
		func(r *pb.ComponentReadiness) { r.CheckedAtUnixMilli = now.Add(-16 * time.Second).UnixMilli() },
		func(r *pb.ComponentReadiness) { r.CheckedAtUnixMilli = now.Add(6 * time.Second).UnixMilli() },
		func(r *pb.ComponentReadiness) { r.State = "ready"; r.Reason = "response-body-must-not-pass" },
	} {
		copy := proto.Clone(report).(*pb.ComponentReadiness)
		change(copy)
		if _, err = validateComponentReports("branch", []monitor.Plan{p}, applied, []*pb.ComponentReadiness{copy}, now); err == nil {
			t.Fatal("invalid readiness accepted")
		}
	}
	if _, err = validateComponentReports("branch", []monitor.Plan{p}, applied, []*pb.ComponentReadiness{report, report}, now); err == nil {
		t.Fatal("duplicate sample accepted")
	}
	old := proto.Clone(report).(*pb.ComponentReadiness)
	old.Revision = "withdrawn"
	ignored, err := validateComponentReports("branch", []monitor.Plan{p}, applied, []*pb.ComponentReadiness{old}, now)
	if err != nil || len(ignored) != 0 {
		t.Fatal("withdrawn applied Plan interrupted stream", err)
	}
}
