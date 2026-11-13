package agent

import (
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"os"
	"testing"
	"time"
)

func TestDiagnosticAuthorizationReceiptAndDeadline(t *testing.T) {
	data, e := os.ReadFile("../../examples/http.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := config.Compile(data)
	if e != nil {
		t.Fatal(e)
	}
	p.Diagnostics = &monitor.DiagnosticBinding{Enabled: true}
	c := p.Checks[0]
	c.Timeout = 10 * time.Second
	data, e = json.Marshal(c)
	if e != nil {
		t.Fatal(e)
	}
	a := &assignment{plan: p, zone: "office", deadline: time.Now().Add(25 * time.Second)}
	st := &state{current: a}
	dir := t.TempDir()
	task := &pb.DiagnosticTask{TaskId: "0123456789abcdef0123456789abcdef", SessionId: "1123456789abcdef0123456789abcdef", Revision: p.Revision, CompiledCheck: data, AttemptNo: 1, ExpiresAtUnixMilli: time.Now().Add(30 * time.Second).UnixMilli()}
	_, _, deadline, e := prepareTask(task, st, dir)
	if e != nil {
		t.Fatal(e)
	}
	if deadline.After(time.Now().Add(25 * time.Second)) {
		t.Fatal("clock uncertainty was not subtracted")
	}
	if _, _, _, e = prepareTask(task, st, dir); e == nil {
		t.Fatal("duplicate task could execute again")
	}
	task.TaskId = "2123456789abcdef0123456789abcdef"
	task.ExpiresAtUnixMilli = time.Now().Add(10 * time.Second).UnixMilli()
	if _, _, _, e = prepareTask(task, st, dir); e == nil {
		t.Fatal("probe with insufficient safe time was accepted")
	}

	task.ExpiresAtUnixMilli = time.Now().Add(-time.Second).UnixMilli()
	if _, _, _, e = prepareTask(task, st, dir); e == nil {
		t.Fatal("expired task accepted")
	}
	task.ExpiresAtUnixMilli = time.Now().Add(10 * time.Second).UnixMilli()
	p.Diagnostics.Enabled = false
	if _, _, _, e = prepareTask(task, st, dir); e == nil {
		t.Fatal("disabled diagnostics accepted task")
	}
}
func TestSpoolReservationProtectsInFlightResults(t *testing.T) {
	s, e := OpenSpool(t.TempDir(), "branch")
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	for i := 0; i < MaxPending-1; i++ {
		s.items[string(rune(i))] = monitor.Observation{}
	}
	if !s.Reserve() || !s.Full() || s.Reserve() {
		t.Fatal("reserved slot not accounted")
	}
	o := monitor.Observation{Agent: "branch", ID: "0123456789abcdef0123456789abcdef"}
	if e = s.Add(o); e == nil {
		t.Fatal("regular add consumed reserved result slot")
	}
	if e = s.AddReserved(o); e != nil {
		t.Fatal(e)
	}
}
