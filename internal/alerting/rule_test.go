package alerting

import (
	"context"
	"testing"
	"time"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func evaluator(t *testing.T, when, resolve string, pending time.Duration) *Evaluator {
	t.Helper()
	e, err := New(Rule{Name: "outage", Monitor: "service", When: when, ResolveWhen: resolve, Enabled: true, For: pending, Cooldown: time.Minute, NotifyEvents: []string{"firing", "resolved"}})
	if err != nil {
		t.Fatal(err)
	}
	return e
}
func assessment(at time.Time, state, revision string) monitor.Assessment {
	return monitor.Assessment{At: at, Monitor: "service", State: state, Revision: revision}
}

func TestPendingRequiresContinuity(t *testing.T) {
	for _, reset := range []string{"gap", "revision", "restart", "error", "false"} {
		t.Run(reset, func(t *testing.T) {
			e := evaluator(t, `monitor.state == "down" && (monitor.name != "error" || int(monitor.state) > 0)`, `monitor.state == "healthy"`, 2*time.Second)
			base := time.Now()
			s := e.Step(context.Background(), State{}, assessment(base, "down", "r1")).State
			if s.Status != "pending" {
				t.Fatalf("%+v", s)
			}
			a := assessment(base.Add(time.Second), "down", "r1")
			switch reset {
			case "gap":
				a.At = base.Add(4 * time.Second)
			case "revision":
				a.Revision = "r2"
			case "restart":
				e.startedAt = a.At
			case "error":
				a.Monitor = "error"
			case "false":
				a.State = "unknown"
			}
			s = e.Step(context.Background(), s, a).State
			if reset == "error" || reset == "false" {
				if s.Status != "normal" || s.PendingSince != nil {
					t.Fatalf("pending not reset: %+v", s)
				}
			} else if s.Status != "pending" || !s.PendingSince.Equal(a.At) {
				t.Fatalf("timer not restarted: %+v", s)
			}
		})
	}
}

func TestFiringHoldsAndRequiresFalseBeforeRearming(t *testing.T) {
	e := evaluator(t, `monitor.state == "down"`, `monitor.state != "unknown"`, 0)
	base := time.Now()
	step := func(s State, n int, state string) Evaluation {
		return e.Step(context.Background(), s, assessment(base.Add(time.Duration(n)*time.Second), state, "r1"))
	}
	r := step(State{}, 0, "down")
	if r.Notify != "firing" {
		t.Fatalf("no firing: %+v", r)
	}
	r = step(r.State, 1, "unknown")
	if r.State.Status != "firing" || r.Transition != nil {
		t.Fatal("unknown resolved firing")
	}
	r = step(r.State, 2, "down") // Both expressions are true.
	if r.State.Status != "resolved" || r.State.Armed || r.Notify != "resolved" {
		t.Fatalf("%+v", r)
	}
	r = step(r.State, 3, "down")
	if r.Transition != nil {
		t.Fatal("unchanged condition fired again")
	}
	r = step(r.State, 4, "healthy")
	if !r.State.Armed {
		t.Fatal("false did not rearm")
	}
	r = step(r.State, 5, "down")
	if r.Notify != "firing" {
		t.Fatalf("new edge did not fire: %+v", r)
	}
	if again := step(r.State, 5, "healthy"); again.Transition != nil || again.State.Status != "firing" {
		t.Fatal("same assessment evaluated twice")
	}
}

func TestCooldownIsSeparateForBothDirections(t *testing.T) {
	e := evaluator(t, `monitor.state == "down"`, `monitor.state == "healthy"`, 0)
	base := time.Now()
	r := e.Step(context.Background(), State{}, assessment(base, "down", "r1"))
	r.State.LastFiringQueuedAt = &base // Store records actual enqueue, not suppression.
	r = e.Step(context.Background(), r.State, assessment(base.Add(time.Second), "healthy", "r1"))
	if r.Notify != "resolved" || r.Suppressed || !r.State.Armed {
		t.Fatalf("resolve suppressed by firing cooldown: %+v", r)
	}
	r = e.Step(context.Background(), r.State, assessment(base.Add(2*time.Second), "down", "r1"))
	if r.Notify != "firing" || !r.Suppressed || r.Transition == nil {
		t.Fatalf("transition must persist during cooldown: %+v", r)
	}
	r = e.Step(context.Background(), r.State, assessment(base.Add(61*time.Second), "down", "r1"))
	if r.Notify != "" {
		t.Fatal("suppressed event replayed later")
	}
}
