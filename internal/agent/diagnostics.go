package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func prepareTask(task *pb.DiagnosticTask, st *state, dir string) (*assignment, monitor.Check, time.Time, error) {
	a := st.get()
	if task == nil || !validID(task.TaskId) || !validID(task.SessionId) || task.AttemptNo < 1 || task.AttemptNo > 50 || len(task.CompiledCheck) > 64<<10 || a == nil || a.plan.Diagnostics == nil || !a.plan.Diagnostics.Enabled || task.Revision != a.plan.Revision || !leaseValid(a) {
		return nil, monitor.Check{}, time.Time{}, fmt.Errorf("task not authorized by current assignment")
	}
	var c monitor.Check
	d := json.NewDecoder(bytes.NewReader(task.CompiledCheck))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		return nil, c, time.Time{}, e
	}
	if e := d.Decode(new(any)); e != io.EOF {
		return nil, c, time.Time{}, fmt.Errorf("trailing diagnostic data")
	}
	if e := config.VerifyDiagnosticCheck(c); e != nil {
		return nil, c, time.Time{}, e
	}
	remaining := time.Until(time.UnixMilli(task.ExpiresAtUnixMilli)) - 5*time.Second
	if remaining < 100*time.Millisecond || remaining > 60*time.Second {
		return nil, c, time.Time{}, fmt.Errorf("task deadline outside clock uncertainty bound")
	}
	deadline := time.Now().Add(remaining)
	if a.deadline.Before(deadline) {
		deadline = a.deadline
	}
	needed := c.Timeout
	if c.Runner == "blackbox" {
		needed += 5 * time.Second
	}
	if !leaseValid(a) || time.Until(deadline) < needed {
		return nil, c, time.Time{}, fmt.Errorf("insufficient safe time for configured probe and startup")
	}
	// Persist the receipt before any side effect. A lost result can be replayed
	// from the observation spool; a receipt without a result is never rerun.
	path := filepath.Join(dir, ".diagnostic-"+task.TaskId)
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, c, time.Time{}, fmt.Errorf("task already received or receipt unavailable")
	}
	if _, e = f.WriteString(time.UnixMilli(task.ExpiresAtUnixMilli).UTC().Format(time.RFC3339Nano)); e != nil {
		f.Close()
		return nil, c, time.Time{}, e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return nil, c, time.Time{}, e
	}
	if e = f.Close(); e != nil {
		return nil, c, time.Time{}, e
	}
	if e = syncTaskDir(dir); e != nil {
		return nil, c, time.Time{}, e
	}
	if !leaseValid(a) || time.Until(deadline) < needed {
		return nil, c, time.Time{}, fmt.Errorf("safe time expired while committing task receipt")
	}
	return a, c, deadline, nil
}
func syncTaskDir(dir string) error {
	f, e := os.Open(dir)
	if e != nil {
		return e
	}
	defer f.Close()
	return f.Sync()
}

func executeTask(ctx context.Context, task *pb.DiagnosticTask, a *assignment, c monitor.Check, deadline time.Time, st *state, spool *Spool, id, blackbox string, allowed []netip.Prefix) error {
	st.active.Add(1)
	defer st.active.Add(-1)
	child, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	p := a.plan
	p.Checks = []monitor.Check{c}
	slot := monitor.Slot{Agent: id, Zone: a.zone}
	var o monitor.Observation
	var e error
	if c.Runner == "blackbox" {
		var exporter *probe.Exporter
		exporter, e = probe.StartExporter(child, blackbox, p)
		if e == nil {
			defer exporter.Close()
			if !leaseValid(a) {
				e = fmt.Errorf("lease expired during diagnostic startup")
			} else {
				o, e = exporter.Run(child, p, c, slot, allowed)
			}
		}
	} else {
		o, e = probe.HTTP(child, p, c, slot, allowed)
	}
	if e != nil {
		now := time.Now().UTC()
		o = monitor.Observation{Monitor: p.Name, Revision: p.Revision, Check: c.ID, Agent: id, Zone: a.zone, StartedAt: now, FinishedAt: now, Execution: "execution_failed", Outcome: "unknown", Reason: "diagnostic_runner_or_policy_unavailable"}
	}
	o.ID = task.TaskId
	o.DiagnosticTaskID = task.TaskId
	return spool.AddReserved(o)
}

// Receipts have no long-term purpose after their absolute deadline. Expired
// redelivery fails the clock check first, so removing them permits no rerun.
func cleanupTaskReceipts(dir string) error {
	paths, e := filepath.Glob(filepath.Join(dir, ".diagnostic-*"))
	if e != nil {
		return e
	}
	for _, p := range paths {
		b, e := os.ReadFile(p)
		if e != nil {
			return e
		}
		t, e := time.Parse(time.RFC3339Nano, string(b))
		if e != nil {
			return fmt.Errorf("invalid diagnostic receipt")
		}
		if time.Now().After(t.Add(5 * time.Second)) {
			if e = os.Remove(p); e != nil {
				return e
			}
		}
	}
	return nil
}
