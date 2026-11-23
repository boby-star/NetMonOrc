package diagnostics

import (
	"context"
	"time"
)

// FixedPredecessors returns transitive dependency closures. Callers validate the
// DAG first; the visited set also bounds traversal for malformed input.
func FixedPredecessors(steps []Step) map[string]map[string]bool {
	byID := map[string]Step{}
	for _, step := range steps {
		byID[step.ID] = step
	}
	out := map[string]map[string]bool{}
	for _, step := range steps {
		seen := map[string]bool{}
		var visit func(string)
		visit = func(id string) {
			if seen[id] {
				return
			}
			seen[id] = true
			for _, dep := range byID[id].DependsOn {
				visit(dep)
			}
		}
		for _, dep := range step.DependsOn {
			visit(dep)
		}
		out[step.ID] = seen
	}
	return out
}

func fixedTaskContext(predecessors map[string]bool, tasks map[string]Outcome) map[string]Outcome {
	out := map[string]Outcome{}
	for id := range predecessors {
		if task, ok := tasks[id]; ok {
			out[id] = task
		}
	}
	return out
}

func (e Engine) fixedCondition(ctx context.Context, step Step, predecessors map[string]bool, tasks map[string]Outcome, evidence map[string]TaskEvidence, state string, p Profile, ledger *Ledger) (bool, error) {
	if step.WhenEvidence != nil || step.WhenContext != nil {
		input := map[string]TaskEvidence{}
		for id, outcome := range fixedTaskContext(predecessors, tasks) {
			data := evidence[id]
			data.Outcome = outcome
			if !data.ValidUntil.IsZero() && !time.Now().Before(data.ValidUntil) {
				data.Values = nil
			}
			input[id] = data
		}
		if step.WhenContext != nil {
			action := Action{}
			for _, candidate := range p.Actions {
				if candidate.ID == step.ActionID {
					action = candidate
					break
				}
			}
			frame := e.CELContext.contextFor(action, state)
			frame.Diagnostic = DiagnosticCounters{Attempts: int64(ledger.UsedChecks), RemainingChecks: int64(max(0, p.Budget.MaxChecks-ledger.UsedChecks)), ElapsedSeconds: time.Since(ledger.StartedAt).Seconds()}
			return step.WhenContext(ctx, input, frame)
		}
		return step.WhenEvidence(ctx, input, state)
	}
	if step.When != nil {
		return step.When(ctx, fixedTaskContext(predecessors, tasks), state)
	}
	return true, nil
}
