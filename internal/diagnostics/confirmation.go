package diagnostics

import (
	"fmt"
	"sort"
	"time"
)

// ConfirmationResult describes observed evidence, never a probability.
type ConfirmationResult struct {
	EvidenceDomains        map[string]string `json:"evidenceDomains,omitempty"`
	RequiredActions        []string          `json:"requiredActions"`
	ObservedActions        []string          `json:"observedActions"`
	Observations           int               `json:"observations"`
	DistinctFailureDomains int               `json:"distinctFailureDomains"`
	Satisfied              bool              `json:"satisfied"`
	CountedEvidence        []string          `json:"countedEvidence"`
}
type confirmationProof struct {
	id, action, domain, agent string
	outcome                   Outcome
	at, validUntil            time.Time
}
type confirmationTracker struct {
	maxSkew time.Duration
	policy  *Confirmation
	proofs  []confirmationProof
}

func validateConfirmation(p Profile, actions map[string]bool) error {
	c := p.Confirmation
	if c == nil {
		if p.ContinueAfterUnique {
			return fmt.Errorf("stopWhenUnique=false requires explicit confirmation")
		}
		return nil
	}
	if p.Mode != "adaptive" || len(c.ActionRefs) == 0 || len(c.ActionRefs) > 100 || c.MinObservations < 1 || c.MinObservations > 50 || c.MinDistinctFailureDomains < 1 || c.MinDistinctFailureDomains > c.MinObservations || c.MinSpacing < 0 || c.MinSpacing > p.Budget.MaxDuration {
		return fmt.Errorf("invalid adaptive confirmation limits")
	}
	seen := map[string]bool{}
	for _, id := range c.ActionRefs {
		if !actions[id] || seen[id] {
			return fmt.Errorf("invalid or duplicate confirmation actionRef")
		}
		seen[id] = true
	}
	return nil
}
func (t *confirmationTracker) add(id string, a Action, e Executor, o Outcome, at, validUntil time.Time) {
	if t.policy == nil || id == "" {
		return
	}
	t.proofs = append(t.proofs, confirmationProof{id: id, action: a.ID, domain: e.FailureDomain, agent: e.ID, outcome: o, at: at, validUntil: validUntil})
}
func (t *confirmationTracker) required(id string) bool {
	if t.policy != nil {
		for _, ref := range t.policy.ActionRefs {
			if ref == id {
				return true
			}
		}
	}
	return false
}

// All named actions must have a concrete compatible observation. Observation
// counts deduplicate the same baseline ID even when it matches several actions.
// Production evidence uses persisted physical completion times. Latest-pair
// validity, TTL and the shared coherent window also apply to confirmation.
func (t *confirmationTracker) state(h string, actions []Action, now time.Time) (*ConfirmationResult, map[string]bool, map[string]bool, time.Time) {
	r := &ConfirmationResult{EvidenceDomains: map[string]string{}, RequiredActions: append([]string(nil), t.policy.ActionRefs...), ObservedActions: []string{}, CountedEvidence: []string{}}
	sort.Strings(r.RequiredActions)
	expected := map[string]string{}
	for _, a := range actions {
		expected[a.ID] = a.Expected[h]
	}
	covered := map[string]bool{}
	domains := map[string]bool{}
	ids := map[string]bool{}
	latest := map[string]confirmationProof{}
	var newest time.Time
	for _, proof := range t.proofs {
		latest[proof.action+"/"+proof.agent] = proof
	}
	usable := func(p confirmationProof) bool {
		return p.outcome.Execution == "completed" && (p.outcome.Outcome == "success" || p.outcome.Outcome == "failure") && (p.validUntil.IsZero() || now.Before(p.validUntil)) && !p.at.After(now) && !p.at.IsZero()
	}
	for _, p := range latest {
		if usable(p) && p.at.After(newest) {
			newest = p.at
		}
	}
	proofs := append([]confirmationProof(nil), t.proofs...)
	sort.SliceStable(proofs, func(i, j int) bool { return proofs[i].at.Before(proofs[j].at) })
	var last time.Time
	for _, p := range proofs {
		current := latest[p.action+"/"+p.agent]
		if !usable(current) || !usable(p) || (t.maxSkew > 0 && newest.Sub(p.at) > t.maxSkew) {
			continue
		}
		if !t.required(p.action) || (!p.validUntil.IsZero() && !now.Before(p.validUntil)) || expected[p.action] != p.outcome.Outcome {
			continue
		}
		covered[p.action] = true
		if ids[p.id] || (!last.IsZero() && p.at.Sub(last) < t.policy.MinSpacing) {
			continue
		}
		ids[p.id] = true
		last = p.at
		r.Observations++
		r.CountedEvidence = append(r.CountedEvidence, p.id)
		r.EvidenceDomains[p.id] = p.domain
		if p.domain != "" {
			domains[p.domain] = true
		}
	}
	for id := range covered {
		r.ObservedActions = append(r.ObservedActions, id)
	}
	sort.Strings(r.ObservedActions)
	r.DistinctFailureDomains = len(domains)
	r.Satisfied = len(covered) == len(t.policy.ActionRefs) && r.Observations >= t.policy.MinObservations && len(domains) >= t.policy.MinDistinctFailureDomains
	return r, covered, domains, last
}

func (t *confirmationTracker) useful(a Action, e Executor, r *ConfirmationResult, covered, domains map[string]bool) bool {
	if !covered[a.ID] {
		return true
	}
	if r.DistinctFailureDomains < t.policy.MinDistinctFailureDomains {
		return e.FailureDomain != "" && !domains[e.FailureDomain]
	}
	return r.Observations < t.policy.MinObservations
}
