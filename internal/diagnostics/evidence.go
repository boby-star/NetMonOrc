package diagnostics

import "time"

type timedProof struct {
	action      Action
	agent       string
	outcome     Outcome
	at, expires time.Time
}

// The latest physical result for a pair supersedes older results, including
// an unusable latest result. Never resurrect a convenient older success.
func workingProofs(proofs []timedProof, now time.Time, maxSkew time.Duration) []timedProof {
	latest := map[string]int{}
	for i, p := range proofs {
		latest[p.action.ID+"/"+p.agent] = i
	}
	var newest time.Time
	valid := func(p timedProof) bool {
		return p.outcome.Execution == "completed" && (p.expires.IsZero() || now.Before(p.expires)) && !p.at.After(now) && !p.at.IsZero()
	}
	for _, i := range latest {
		p := proofs[i]
		if valid(p) && p.at.After(newest) {
			newest = p.at
		}
	}
	var out []timedProof
	for i, p := range proofs {
		if latest[p.action.ID+"/"+p.agent] != i || !valid(p) {
			continue
		}
		if maxSkew > 0 && newest.Sub(p.at) > maxSkew {
			continue
		}
		out = append(out, p)
	}
	return out
}

func sameWorkingProofs(a, b []timedProof) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].action.ID != b[i].action.ID || a[i].agent != b[i].agent || a[i].at != b[i].at || a[i].outcome != b[i].outcome {
			return false
		}
	}
	return true
}
