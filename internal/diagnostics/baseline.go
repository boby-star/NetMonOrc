package diagnostics

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"reflect"
	"sort"
	"time"
)

func equivalent(a, b monitor.Check) bool {
	a.Fallback = nil
	b.Fallback = nil
	a.ID = ""
	b.ID = ""
	a.Interval = 0
	b.Interval = 0
	a.MaxAge = 0
	b.MaxAge = 0
	a.Weight = 0
	b.Weight = 0
	a.Required = false
	b.Required = false
	a.Priority, b.Priority = 0, 0
	a.Jitter, b.Jitter = 0, 0
	return reflect.DeepEqual(a, b)
}

// MatchBaseline must receive authenticated Controller observations, not raw
// user-supplied NDJSON. No explicit evidenceCheckRefs or guessed matches exist.
func MatchBaseline(plan monitor.Plan, p Profile, history []monitor.Observation, agents []Executor, now time.Time) []Evidence {
	checks := map[string]monitor.Check{}
	for _, c := range plan.Checks {
		checks[c.ID] = c
	}
	sources := map[string]Executor{}
	for _, a := range agents {
		sources[a.ID] = a
	}
	latest := map[string]monitor.Observation{}
	for _, o := range history {
		c, ok := checks[o.Check]
		a, known := sources[o.Agent]
		age := c.MaxAge
		if age == 0 {
			age = plan.MaxAge
		}
		if (!a.EvidenceNotBefore.IsZero() && o.StartedAt.Before(a.EvidenceNotBefore)) || o.DiagnosticTaskID != "" || !ok || !known || o.ID == "" || o.Monitor != plan.Name || o.Revision != plan.Revision || o.Zone != a.Zone || o.FinishedAt.Before(o.StartedAt) || o.FinishedAt.After(now) || now.Sub(o.FinishedAt) > age {
			continue
		}
		if o.FallbackTaskID != "" {
			origin := monitor.Slot{Agent: o.FallbackForAgent, Zone: o.Zone}
			if o.FallbackRunID == "" || o.FallbackForAgent == "" || o.ID != o.FallbackTaskID || !plan.FallbackCandidate(c, origin, monitor.Slot{Agent: o.Agent, Zone: o.Zone}, plan.FallbackDomains) {
				continue
			}
		} else if o.FallbackForAgent != "" || o.FallbackRunID != "" {
			continue
		}
		key := o.Check + "/" + o.Agent
		previous, found := latest[key]
		if !found || o.FinishedAt.After(previous.FinishedAt) || (o.FinishedAt.Equal(previous.FinishedAt) && o.ID > previous.ID) {
			latest[key] = o
		}
	}
	type matched struct {
		proof Evidence
		at    time.Time
	}
	var matches []matched
	var newest time.Time
	for _, o := range latest {
		if o.Execution != "completed" || (o.Outcome != "success" && o.Outcome != "failure") {
			continue
		}
		for _, action := range p.Actions {
			if !placementMatch(action.Placement, sources[o.Agent]) || !equivalent(action.Check, checks[o.Check]) {
				continue
			}
			age := checks[o.Check].MaxAge
			if age == 0 {
				age = plan.MaxAge
			}
			matches = append(matches, matched{Evidence{FallbackRunID: o.FallbackRunID, FallbackTaskID: o.FallbackTaskID, FallbackOriginAgentID: o.FallbackForAgent, CheckID: o.Check, ConfigRevision: o.Revision, FinishedAt: o.FinishedAt, ActionID: action.ID, AgentID: o.Agent, Outcome: Outcome{o.Execution, o.Outcome}, ObservationID: o.ID, ValidUntil: o.FinishedAt.Add(age)}, o.FinishedAt})
			if newest.IsZero() || o.FinishedAt.After(newest) {
				newest = o.FinishedAt
			}
		}
	}
	// One physical action/Agent pair has one initial fact, even when multiple
	// equivalent regular check IDs exist. Time/ID, never map order, breaks ties.
	pairs := map[string]Evidence{}
	for _, m := range matches {
		if newest.Sub(m.at) > plan.MaxSkew {
			continue
		}
		key := m.proof.ActionID + "/" + m.proof.AgentID
		old, found := pairs[key]
		if !found || m.proof.FinishedAt.After(old.FinishedAt) || (m.proof.FinishedAt.Equal(old.FinishedAt) && m.proof.ObservationID > old.ObservationID) {
			pairs[key] = m.proof
		}
	}
	var out []Evidence
	for _, proof := range pairs {
		out = append(out, proof)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActionID == out[j].ActionID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].ActionID < out[j].ActionID
	})
	return out
}
