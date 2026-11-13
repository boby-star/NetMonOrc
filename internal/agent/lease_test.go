package agent

import (
	"testing"
	"time"
)

func TestSuspendAwareLease(t *testing.T) {
	now := time.Now()
	a := &assignment{deadline: now.Add(25 * time.Second), bootDeadline: 125 * time.Second}
	if !leaseValidAt(a, now, 100*time.Second) {
		t.Fatal("valid lease rejected")
	}
	// The process monotonic clock has barely advanced, but the OS suspended.
	if leaseValidAt(a, now.Add(time.Second), 130*time.Second) {
		t.Fatal("suspend extended authorization")
	}
	if leaseValidAt(a, now.Add(26*time.Second), 101*time.Second) {
		t.Fatal("ordinary expiry ignored")
	}
	if leaseValidAt(nil, now, 0) || leaseValidAt(&assignment{deadline: a.deadline}, now, 0) {
		t.Fatal("missing time bound authorized work")
	}
}
