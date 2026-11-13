package diagnostics

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"reflect"
	"sort"
	"time"
)

func equivalent(a, b monitor.Check) bool {
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
		if o.DiagnosticTaskID != "" || !ok || !known || o.ID == "" || o.Monitor != plan.Name || o.Revision != plan.Revision || o.Zone != a.Zone || o.FinishedAt.Before(o.StartedAt) || o.FinishedAt.After(now) || now.Sub(o.FinishedAt) > age {
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
			matches = append(matches, matched{Evidence{action.ID, o.Agent, Outcome{o.Execution, o.Outcome}}, o.FinishedAt})
			if newest.IsZero() || o.FinishedAt.After(newest) {
				newest = o.FinishedAt
			}
		}
	}
	var out []Evidence
	for _, m := range matches {
		if newest.Sub(m.at) <= plan.MaxSkew {
			out = append(out, m.proof)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ActionID == out[j].ActionID {
			return out[i].AgentID < out[j].AgentID
		}
		return out[i].ActionID < out[j].ActionID
	})
	return out
}
