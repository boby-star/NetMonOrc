package diagnostics

import (
	"context"
	"sort"
	"sync"
	"time"
)

type fixedHooks struct {
	refresh    func()
	hypotheses func() []string
	proofs     func() []timedProof
	record     func(Action, Executor, Decision, int)
}
type fixedActivation struct {
	action   Action
	executor Executor
	next     int
}
type fixedJob struct {
	step       string
	activation *fixedActivation
	bytes      int64
	engine     Engine
	release    func()
	decision   Decision
	attempted  bool
	reason     string
}

// runFixedParallel uses deterministic barrier waves. Budget quotas and journal
// publication are serial; only independent physical execution is concurrent.
// One diagnostic activation per Agent matches the live shared admission model.
func (e Engine) runFixedParallel(ctx context.Context, p Profile, agents []Executor, ledger *Ledger, r *Result, deadline time.Time, monitorState string, hooks fixedHooks) (string, bool, bool) {
	steps := append([]Step(nil), p.Steps...)
	predecessors := FixedPredecessors(steps)
	sort.Slice(steps, func(i, j int) bool { return steps[i].ID < steps[j].ID })
	actions := map[string]Action{}
	for _, a := range p.Actions {
		actions[a.ID] = a
	}
	terminal := map[string]bool{}
	defer func() {
		blocked := map[string]bool{}
		for _, id := range r.BlockedSteps {
			blocked[id] = true
		}
		for _, step := range steps {
			if !terminal[step.ID] {
				blocked[step.ID] = true
			}
		}
		r.BlockedSteps = nil
		for id := range blocked {
			r.BlockedSteps = append(r.BlockedSteps, id)
		}
		sort.Strings(r.BlockedSteps)
	}()
	tasks := map[string]Outcome{}
	taskEvidence := map[string]TaskEvidence{}
	activations := map[string]*fixedActivation{}
	rounds := map[string]int{}
	incomplete := false
	for {
		hooks.refresh()
		if ctx.Err() != nil {
			return "deadline_or_cancelled", false, incomplete
		}
		if len(terminal) == len(steps) {
			return "fixed_finished_without_class", true, incomplete
		}
		before := hooks.hypotheses()
		proofsBefore := hooks.proofs()
		usedAgents := map[string]bool{}
		var jobs []*fixedJob
		var skipped []Step
		reservedBytes := int64(0)
		_, managed := e.Runner.(AdmissionRunner)
		for _, step := range steps {
			if terminal[step.ID] {
				continue
			}
			ready := true
			for _, dep := range step.DependsOn {
				if !terminal[dep] {
					ready = false
				}
			}
			if !ready {
				continue
			}
			activation, started := activations[step.ID]
			if !started && (step.When != nil || step.WhenEvidence != nil) {
				yes, err := fixedCondition(ctx, step, predecessors[step.ID], tasks, taskEvidence, monitorState)
				if err != nil {
					incomplete = true
					r.BlockedSteps = append(r.BlockedSteps, step.ID)
				}
				if err != nil || !yes {
					skipped = append(skipped, step)
					continue
				}
			}
			if len(jobs) >= p.Budget.MaxParallel || len(jobs) >= p.Budget.MaxChecks-ledger.UsedChecks {
				continue
			}
			a := actions[step.ActionID]
			if started {
				a = activation.action
			}
			if time.Now().Add(a.Check.Timeout + e.TaskOverhead).After(deadline) {
				continue
			}
			var eligible []Executor
			estimates := map[string]int64{}
			for _, agent := range agents {
				if usedAgents[agent.ID] || !agent.Online || agent.Capacity <= 0 || (!managed && agent.Active >= agent.Capacity) || !agent.Supported[a.Check.Runner] || !agent.Supported[a.Check.Type] || !placementMatch(a.Placement, agent) || !e.Eligible(a, agent) || (started && agent.ID != activation.executor.ID) {
					continue
				}
				if p.Budget.MaxEstimatedBytes > 0 {
					if e.EstimateBytes == nil {
						continue
					}
					n, known := e.EstimateBytes(a, agent)
					if !known || n < 0 || n > p.Budget.MaxEstimatedBytes-ledger.EstimatedBytes-reservedBytes {
						continue
					}
					estimates[agent.ID] = n
				}
				eligible = append(eligible, agent)
			}
			if len(eligible) == 0 {
				continue
			}
			executor := chooseExecutor(a.Placement, eligible, rounds[a.ID])
			if !started {
				a.StepID = step.ID
				activation = &fixedActivation{action: a, executor: executor, next: 1}
				activations[step.ID] = activation
				rounds[a.ID]++
			}
			usedAgents[executor.ID] = true
			n := estimates[executor.ID]
			reservedBytes += n
			jobs = append(jobs, &fixedJob{step: step.ID, activation: activation, bytes: n, engine: e, release: func() {}})
		}
		for _, step := range skipped {
			terminal[step.ID] = true
			tasks[step.ID] = Outcome{"skipped", "unknown"}
			r.SkippedSteps = append(r.SkippedSteps, step.ID)
		}
		if len(jobs) == 0 {
			if len(skipped) > 0 {
				continue
			}
			if ledger.UsedChecks >= p.Budget.MaxChecks {
				return "check_budget_exhausted", false, incomplete
			}
			for _, step := range steps {
				if !terminal[step.ID] {
					r.BlockedSteps = append(r.BlockedSteps, step.ID)
				}
			}
			return "no_admissible_action", false, incomplete
		}
		// Acquire permits in a global Agent order, preventing hold-and-wait
		// cycles between concurrent DAG sessions. No budget is spent while waiting.
		admission := append([]*fixedJob(nil), jobs...)
		sort.Slice(admission, func(i, j int) bool { return admission[i].activation.executor.ID < admission[j].activation.executor.ID })
		for _, job := range admission {
			if runner, ok := e.Runner.(AdmissionRunner); ok {
				a := job.activation.action
				wait, stop := context.WithDeadline(ctx, deadline.Add(-a.Check.Timeout-e.TaskOverhead))
				bound, release, err := runner.Admit(wait, a, job.activation.executor)
				stop()
				if err != nil {
					job.reason = "executor_admission_unavailable"
					continue
				}
				job.engine.Runner = bound
				job.release = release
			}
		}
		if !sameWorkingProofs(proofsBefore, hooks.proofs()) {
			for _, job := range jobs {
				job.release()
			}
			continue
		}
		var workers sync.WaitGroup
		for _, job := range jobs {
			workers.Add(1)
			go func() {
				defer workers.Done()
				defer job.release()
				if job.reason != "" {
					return
				}
				a := job.activation.action
				a.AttemptNo = job.activation.next
				if ctx.Err() != nil {
					job.reason = "deadline_or_cancelled"
					return
				}
				if time.Now().Add(a.Check.Timeout + e.TaskOverhead).After(deadline) {
					job.reason = "no_time_for_retry"
					return
				}
				if !e.Eligible(a, job.activation.executor) {
					job.reason = "executor_no_longer_eligible"
					return
				}
				phase := ""
				if p.Confirmation != nil {
					phase = "primary"
				}
				job.attempted = true
				job.decision = job.engine.executeAttempt(ctx, a, job.activation.executor, deadline, phase, Gain(before, a.Expected), before)
			}()
		}
		workers.Wait()
		stopReason := ""
		// Only this goroutine publishes outcomes/spends preallocated quotas.
		// Physical completion order cannot change retry budget or journal order.
		for _, job := range jobs {
			if !job.attempted {
				if stopReason == "" {
					stopReason = job.reason
				}
				continue
			}
			a := job.activation.action
			d := job.decision
			ledger.UsedChecks++
			ledger.EstimatedBytes += job.bytes
			r.Attempts++
			r.Cost += a.Cost
			r.Decisions = append(r.Decisions, d)
			hooks.record(a, job.activation.executor, d, ledger.UsedChecks)
			limit := 1
			if a.Retry != nil {
				limit = a.Retry.MaxAttempts
			}
			if d.Outcome.Execution == "execution_failed" && job.activation.next < limit {
				job.activation.next++
				continue
			}
			terminal[job.step] = true
			tasks[job.step] = d.Outcome
			taskEvidence[job.step] = d.TaskEvidence
			if d.Outcome.Execution != "completed" || (d.Outcome.Outcome != "success" && d.Outcome.Outcome != "failure") {
				incomplete = true
			}
		}
		if ctx.Err() != nil {
			return "deadline_or_cancelled", false, incomplete
		}
		if stopReason != "" {
			return stopReason, false, incomplete
		}
	}
}
