package diagnostics

import (
	"testing"
	"time"
)

func TestWorkingProofsCoherenceAndLatest(t *testing.T) {
	now := time.Now()
	a := Action{ID: "dns"}
	b := Action{ID: "http"}
	old := timedProof{action: a, agent: "branch", at: now.Add(-20 * time.Second), expires: now.Add(time.Minute), outcome: Outcome{"completed", "failure"}}
	recent := timedProof{action: b, agent: "dc", at: now.Add(-time.Second), expires: now.Add(time.Minute), outcome: Outcome{"completed", "success"}}
	if got := workingProofs([]timedProof{old, recent}, now, 5*time.Second); len(got) != 1 || got[0].action.ID != "http" {
		t.Fatal("mixed-time evidence admitted", got)
	}
	old.at = now.Add(-2 * time.Second)
	invalid := old
	invalid.at = now
	invalid.outcome = Outcome{"execution_failed", "unknown"}
	if got := workingProofs([]timedProof{old, recent, invalid}, now, 5*time.Second); len(got) != 1 || got[0].action.ID != "http" {
		t.Fatal("older convenient result resurrected", got)
	}
	future := old
	future.at = now.Add(time.Second)
	if got := workingProofs([]timedProof{old, recent, future}, now, 5*time.Second); len(got) != 1 {
		t.Fatal("future timestamp accepted or old result resurrected")
	}
	expired := old
	expired.expires = now.Add(-time.Millisecond)
	if got := workingProofs([]timedProof{expired}, now, 5*time.Second); len(got) != 0 {
		t.Fatal("expired result retained")
	}
	old.at = recent.at.Add(-5 * time.Second)
	if got := workingProofs([]timedProof{old, recent}, now, 5*time.Second); len(got) != 2 {
		t.Fatal("maxSkew inclusive boundary rejected")
	}
	old.at = old.at.Add(-time.Nanosecond)
	if got := workingProofs([]timedProof{old, recent}, now, 5*time.Second); len(got) != 1 {
		t.Fatal("maxSkew boundary exceeded")
	}
}

func TestConfirmationUsesSameTimeWindowAndPhysicalSpacing(t *testing.T) {
	now := time.Now()
	a := Action{ID: "dns", Expected: map[string]string{"fault": "failure"}}
	b := Action{ID: "http", Expected: map[string]string{"fault": "failure"}}
	tracker := confirmationTracker{policy: &Confirmation{ActionRefs: []string{"dns", "http"}, MinObservations: 2, MinDistinctFailureDomains: 2}, maxSkew: 5 * time.Second}
	tracker.add("old", a, Executor{ID: "branch", FailureDomain: "office"}, Outcome{"completed", "failure"}, now.Add(-20*time.Second), now.Add(time.Minute))
	tracker.add("new", b, Executor{ID: "dc", FailureDomain: "server"}, Outcome{"completed", "failure"}, now.Add(-time.Second), now.Add(time.Minute))
	state, _, _, _ := tracker.state("fault", []Action{a, b}, now)
	if state.Satisfied || state.Observations != 1 || state.DistinctFailureDomains != 1 {
		t.Fatal("confirmation combined incoherent sources", state)
	}
	tracker.policy.MinSpacing = time.Second
	tracker.maxSkew = time.Minute
	tracker.proofs[0].at = tracker.proofs[1].at
	state, _, _, _ = tracker.state("fault", []Action{a, b}, now)
	if state.Satisfied || state.Observations != 1 {
		t.Fatal("delivery spacing replaced physical spacing", state)
	}
}
