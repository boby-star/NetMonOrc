package diagnostics

import (
	"context"
	"time"
)

// executeAttempt has no shared ledger/journal mutations and is safe for a wave
// of independent fixed steps. The scheduler owns admission and budget ordering.
func (e Engine) executeAttempt(ctx context.Context, a Action, executor Executor, deadline time.Time, phase string, gain int, before []string) Decision {
	attempt, done := context.WithTimeout(ctx, a.Check.Timeout+e.TaskOverhead)
	defer done()
	var evidence ObservedOutcome
	var err error
	if runner, ok := e.Runner.(EvidenceRunner); ok {
		evidence, err = runner.ExecuteEvidence(attempt, a, executor)
	} else {
		evidence.Outcome, err = e.Runner.Execute(attempt, a, executor)
	}
	observed := evidence.Outcome
	valid := (observed.Execution == "completed" && (observed.Outcome == "success" || observed.Outcome == "failure" || observed.Outcome == "unknown")) || (observed.Execution == "execution_failed" && observed.Outcome == "unknown")
	if err != nil || attempt.Err() != nil || !valid {
		observed = Outcome{"execution_failed", "unknown"}
	}
	var finished, validUntil *time.Time
	if !evidence.FinishedAt.IsZero() {
		at := evidence.FinishedAt
		finished = &at
		expires := deadline
		if e.EvidenceMaxAge > 0 && at.Add(e.EvidenceMaxAge).Before(expires) {
			expires = at.Add(e.EvidenceMaxAge)
		}
		validUntil = &expires
	}
	data := TaskEvidence{Outcome: observed}
	if err == nil && attempt.Err() == nil && valid && evidence.ObservationID != "" && finished != nil {
		data.ReasonCode = evidence.ReasonCode
		if observed.Execution == "completed" {
			data.Values = evidence.Values
		}
		if validUntil != nil {
			data.ValidUntil = *validUntil
		}
	}
	return Decision{TaskEvidence: data, StepID: a.StepID, ObservationID: evidence.ObservationID, FinishedAt: finished, ValidUntil: validUntil, FailureDomain: executor.FailureDomain, ActionID: a.ID, AgentID: executor.ID, HypothesesBefore: append([]string(nil), before...), Gain: gain, Score: float64(gain) / a.Cost, Outcome: observed, Phase: phase, AttemptNo: a.AttemptNo}
}
