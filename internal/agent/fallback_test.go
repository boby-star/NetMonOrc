package agent

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/protobuf/proto"
)

func TestFallbackRequiresExactAssignedCheckAndTrustedSpare(t *testing.T) {
	primary := monitor.Slot{Agent: "primary", Zone: "office"}
	spare := monitor.Slot{Agent: "spare", Zone: "office"}
	c := monitor.Check{ID: "http", Type: "http", Runner: "native-http", Method: "GET", URL: "http://127.0.0.1/", ExpectedStatus: []int{200}, MaxBodyBytes: 16384, Timeout: time.Second, Interval: 10 * time.Second, Fallback: &monitor.FallbackPolicy{On: []string{"agent_unreachable"}, Strategy: "same-zone", MaxAttempts: 1}}
	p := monitor.Plan{Name: "fallback", Revision: "revision", Checks: []monitor.Check{c}, FallbackRoster: []monitor.Slot{primary, spare}, FixedPlacement: &monitor.FixedPlacement{Selected: []monitor.Slot{primary}}}
	boot, err := bootTime()
	if err != nil {
		t.Fatal(err)
	}
	a := &assignment{plan: p, zone: "office", deadline: time.Now().Add(30 * time.Second), bootDeadline: boot + 30*time.Second}
	st := &state{current: a, agentID: "spare"}
	wire, err := json.Marshal(config.FallbackCheck(c))
	if err != nil {
		t.Fatal(err)
	}
	task := pb.DiagnosticTask{TaskId: "0123456789abcdef0123456789abcdef", SessionId: "1123456789abcdef0123456789abcdef", FallbackRunId: "1123456789abcdef0123456789abcdef", FallbackOriginAgent: "primary", FallbackOriginZone: "office", Revision: p.Revision, CompiledCheck: wire, AttemptNo: 1, ExpiresAtUnixMilli: time.Now().Add(20 * time.Second).UnixMilli()}
	for _, tc := range []struct {
		name   string
		change func(*pb.DiagnosticTask)
	}{
		{"unknown origin", func(x *pb.DiagnosticTask) { x.FallbackOriginAgent = "unknown" }},
		{"wrong zone", func(x *pb.DiagnosticTask) { x.FallbackOriginZone = "server" }},
		{"extra attempt", func(x *pb.DiagnosticTask) { x.AttemptNo = 2 }},
		{"wrong correlation", func(x *pb.DiagnosticTask) { x.FallbackRunId = x.TaskId }},
		{"altered target", func(x *pb.DiagnosticTask) {
			other := config.FallbackCheck(c)
			other.URL = "http://127.0.0.2/"
			x.CompiledCheck, _ = json.Marshal(other)
		}},
		{"ordinary diagnostic disabled", func(x *pb.DiagnosticTask) { x.FallbackRunId = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			copy := proto.Clone(&task).(*pb.DiagnosticTask)
			tc.change(copy)
			if _, _, _, err := prepareTask(copy, st, t.TempDir()); err == nil {
				t.Fatal("unauthorized task accepted")
			}
		})
	}
	dir := t.TempDir()
	if _, _, _, err = prepareTask(&task, st, dir); err != nil {
		t.Fatal("authorized fallback requires no diagnostic binding", err)
	}
	if _, _, _, err = prepareTask(&task, st, dir); err == nil {
		t.Fatal("fallback physical attempt replayed")
	}
}
