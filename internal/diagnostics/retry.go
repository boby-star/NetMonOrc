package diagnostics

import (
	"context"
	"time"
)

// A chain retries only a technical execution failure for one planned activation
// on the same executor. It cannot reselect an action or replenish episode budget.
type attemptChain struct {
	engine   Engine
	profile  Profile
	ledger   *Ledger
	result   *Result
	deadline time.Time
}

func (c attemptChain) execute(ctx context.Context, a Action, executor Executor, estimatedBytes int64, phase string, gain int) (Outcome, string) {
	limit := 1
	if a.Retry != nil {
		limit = a.Retry.MaxAttempts
	}
	before := append([]string(nil), c.result.Hypotheses...)
	var outcome Outcome
	for n := 1; n <= limit; n++ {
		if ctx.Err() != nil {
			return outcome, "deadline_or_cancelled"
		}
		if c.ledger.UsedChecks >= c.profile.Budget.MaxChecks {
			return outcome, "check_budget_exhausted"
		}
		if time.Now().Add(a.Check.Timeout + c.engine.TaskOverhead).After(c.deadline) {
			return outcome, "no_time_for_retry"
		}
		if c.profile.Budget.MaxEstimatedBytes > 0 && estimatedBytes > c.profile.Budget.MaxEstimatedBytes-c.ledger.EstimatedBytes {
			return outcome, "estimated_byte_budget_exhausted"
		}
		if !c.engine.Eligible(a, executor) {
			return outcome, "executor_no_longer_eligible"
		}
		// Mark before dispatch. A cancelled chain cannot make an already
		// attempted Adaptive action free to select in another Run.
		if c.result.Mode == "adaptive" {
			c.ledger.adaptiveAttempted[a.ID] = true
		}
		c.ledger.UsedChecks++
		c.ledger.EstimatedBytes += estimatedBytes
		c.result.Attempts++
		c.result.Cost += a.Cost
		dispatched := a
		dispatched.AttemptNo = n
		attempt, done := context.WithTimeout(ctx, a.Check.Timeout+c.engine.TaskOverhead)
		observed, err := c.engine.Runner.Execute(attempt, dispatched, executor)
		valid := (observed.Execution == "completed" && (observed.Outcome == "success" || observed.Outcome == "failure" || observed.Outcome == "unknown")) || (observed.Execution == "execution_failed" && observed.Outcome == "unknown")
		if err != nil || attempt.Err() != nil || !valid {
			observed = Outcome{"execution_failed", "unknown"}
		}
		done()
		outcome = observed
		c.result.Decisions = append(c.result.Decisions, Decision{ActionID: a.ID, AgentID: executor.ID, HypothesesBefore: before, Gain: gain, Score: float64(gain) / a.Cost, Outcome: outcome, Phase: phase, AttemptNo: n})
		if outcome.Execution != "execution_failed" {
			break
		}
	}
	return outcome, ""
}
