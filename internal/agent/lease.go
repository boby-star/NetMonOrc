package agent

import "time"

// CLOCK_MONOTONIC may pause during OS suspend. Both bounds must hold;
// unavailable suspend-aware time fails closed until a new valid assignment.
func leaseValid(a *assignment) bool {
	boot, err := bootTime()
	return err == nil && leaseValidAt(a, time.Now(), boot)
}

func leaseValidAt(a *assignment, now time.Time, boot time.Duration) bool {
	return a != nil && a.bootDeadline > 0 && now.Before(a.deadline) && boot < a.bootDeadline
}
