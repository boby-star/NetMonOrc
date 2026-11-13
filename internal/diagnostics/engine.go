package diagnostics

import (
	"context"
	"fmt"
	"math"
	"sort"
	"time"
)

func pattern(s string) uint8 {
	switch s {
	case "success":
		return 1
	case "failure":
		return 2
	case "unknown":
		return 4
	case "either":
		return 7
	}
	return 0
}
func Gain(hypotheses []string, expected map[string]string) int {
	gain := 0
	for i, h := range hypotheses {
		for _, other := range hypotheses[i+1:] {
			a, b := pattern(expected[h]), pattern(expected[other])
			if a != 0 && b != 0 && a&b == 0 {
				gain++
			}
		}
	}
	return gain
}
func Validate(p Profile) error {
	if p.Mode != "fixed" && p.Mode != "adaptive" {
		return fmt.Errorf("profile mode must be fixed/adaptive")
	}
	if p.Budget.MaxChecks < 1 || p.Budget.MaxChecks > 50 || p.Budget.MaxDuration < time.Second || p.Budget.MaxDuration > 10*time.Minute || p.Budget.MaxParallel < 1 || p.Budget.MaxParallel > 10 || p.Budget.MaxEstimatedBytes < 0 {
		return fmt.Errorf("invalid diagnostic budget")
	}
	if len(p.Actions) == 0 || len(p.Actions) > 100 || len(p.Hypotheses) > 50 || (p.Mode == "adaptive" && len(p.Hypotheses) < 2) {
		return fmt.Errorf("invalid catalog size")
	}
	h := map[string]bool{}
	for _, id := range p.Hypotheses {
		if id == "" || h[id] {
			return fmt.Errorf("invalid hypothesis ID")
		}
		h[id] = true
	}
	actions := map[string]bool{}
	for _, a := range p.Actions {
		if a.ID == "" || actions[a.ID] || math.IsNaN(a.Cost) || math.IsInf(a.Cost, 0) || a.Cost <= 0 {
			return fmt.Errorf("invalid action ID/cost")
		}
		actions[a.ID] = true
		maxPairs := len(p.Hypotheses) * (len(p.Hypotheses) - 1) / 2
		if a.Cost > math.MaxFloat64/float64(p.Budget.MaxChecks) || (maxPairs > 0 && a.Cost < float64(maxPairs)/math.MaxFloat64) {
			return fmt.Errorf("cost outside supported finite score/report range")
		}
		for id, outcome := range a.Expected {
			if !h[id] || pattern(outcome) == 0 {
				return fmt.Errorf("invalid expected outcome")
			}
		}
		if p.Mode == "adaptive" && len(a.Expected) != len(h) {
			return fmt.Errorf("adaptive action must cover every hypothesis")
		}
		if a.Placement.Strategy != "fixed" && a.Placement.Strategy != "leastLoaded" && a.Placement.Strategy != "roundRobin" {
			return fmt.Errorf("unsupported placement strategy")
		}
		if a.Placement.Strategy == "fixed" && len(a.Placement.AgentRefs) == 0 {
			return fmt.Errorf("fixed placement requires agentRefs")
		}
	}
	if err := validateConfirmation(p, actions); err != nil {
		return err
	}
	if p.Mode == "fixed" {
		if len(p.Steps) == 0 || len(p.Steps) > 100 {
			return fmt.Errorf("fixed steps required")
		}
		steps := map[string]bool{}
		for _, s := range p.Steps {
			if s.ID == "" || steps[s.ID] || !actions[s.ActionID] {
				return fmt.Errorf("invalid fixed step")
			}
			steps[s.ID] = true
		}
		finished := map[string]bool{}
		for len(finished) < len(steps) {
			progress := false
			for _, s := range p.Steps {
				if finished[s.ID] {
					continue
				}
				ready := true
				for _, dep := range s.DependsOn {
					if !steps[dep] {
						return fmt.Errorf("missing fixed dependency")
					}
					if !finished[dep] {
						ready = false
					}
				}
				if ready {
					finished[s.ID] = true
					progress = true
				}
			}
			if !progress {
				return fmt.Errorf("fixed DAG cycle")
			}
		}
	}
	return nil
}
func placementMatch(p Placement, a Executor) bool {
	if len(p.Zones) > 0 {
		found := false
		for _, z := range p.Zones {
			if z == a.Zone {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if len(p.AgentRefs) > 0 {
		found := false
		for _, id := range p.AgentRefs {
			if id == a.ID {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	return true
}
func chooseExecutor(p Placement, agents []Executor, round int) Executor {
	sort.Slice(agents, func(i, j int) bool {
		if p.Strategy == "leastLoaded" {
			left, right := int64(agents[i].Active)*int64(agents[j].Capacity), int64(agents[j].Active)*int64(agents[i].Capacity)
			if left != right {
				return left < right
			}
		}
		if p.Strategy == "fixed" {
			rank := func(id string) int {
				for i, x := range p.AgentRefs {
					if x == id {
						return i
					}
				}
				return len(p.AgentRefs)
			}
			if rank(agents[i].ID) != rank(agents[j].ID) {
				return rank(agents[i].ID) < rank(agents[j].ID)
			}
		}
		return agents[i].ID < agents[j].ID
	})
	if p.Strategy == "roundRobin" {
		return agents[round%len(agents)]
	}
	return agents[0]
}

// Run executes serially within maxParallel. Static is an experiment mode, not a
// new YAML enum. All modes share catalog, eligibility and the same budget rules.
func (e Engine) Run(ctx context.Context, p Profile, mode string, agents []Executor, ledger *Ledger, initial []Evidence, monitorState string) (r Result, err error) {
	started := time.Now()
	defer func() { elapsed := time.Since(started).Milliseconds(); r.DurationMillis = &elapsed }()
	r = Result{Mode: mode, Status: "inconclusive", Hypotheses: append([]string(nil), p.Hypotheses...), Decisions: []Decision{}}
	if err := Validate(p); err != nil {
		return r, err
	}
	if mode != "static" && mode != "fixed" && mode != "adaptive" {
		return r, fmt.Errorf("invalid execution mode")
	}
	if mode == "adaptive" && p.Mode != "adaptive" {
		return r, fmt.Errorf("adaptive requires adaptive profile")
	}
	if mode == "fixed" && p.Mode != "fixed" {
		return r, fmt.Errorf("fixed requires a fixed profile")
	}
	if e.Runner == nil || e.Eligible == nil || ledger == nil {
		return r, fmt.Errorf("runner, eligibility and episode ledger required")
	}
	seenAgents := map[string]bool{}
	for _, a := range agents {
		if a.ID == "" || seenAgents[a.ID] || a.Active < 0 || a.Capacity < 0 || a.Capacity > 10000 {
			return r, fmt.Errorf("invalid executor catalog")
		}
		seenAgents[a.ID] = true
	}
	ledger.mu.Lock()
	if ledger.running {
		ledger.mu.Unlock()
		return r, fmt.Errorf("episode already has an active diagnostic session")
	}
	if ledger.UsedChecks < 0 || ledger.EstimatedBytes < 0 || ledger.StartedAt.After(time.Now()) {
		ledger.mu.Unlock()
		return r, fmt.Errorf("invalid episode ledger")
	}
	if ledger.policy != nil && *ledger.policy != p.Budget {
		ledger.mu.Unlock()
		return r, fmt.Errorf("episode budget policy cannot be replaced")
	}
	ledger.running = true
	if ledger.policy == nil {
		policy := p.Budget
		ledger.policy = &policy
		ledger.adaptiveAttempted = map[string]bool{}
	}
	if ledger.StartedAt.IsZero() {
		ledger.StartedAt = time.Now()
	}
	ledger.mu.Unlock()
	defer func() { ledger.mu.Lock(); ledger.running = false; ledger.mu.Unlock() }()
	deadline := ledger.StartedAt.Add(p.Budget.MaxDuration)
	child, cancel := context.WithDeadline(ctx, deadline)
	defer cancel()
	sort.Strings(r.Hypotheses)
	tracker := confirmationTracker{policy: p.Confirmation}
	if p.Confirmation != nil {
		defer func() {
			h := ""
			if len(r.Hypotheses) == 1 {
				h = r.Hypotheses[0]
			}
			r.Confirmation, _, _, _ = tracker.state(h, p.Actions, time.Now())
			if r.Status == "completed" && !r.Confirmation.Satisfied {
				r.Status = "inconclusive"
				r.Reason = "confirmation_incomplete"
			}
		}()
	}
	unusablePairs := map[string]bool{}
	fixedIncomplete := false
	support := map[string]bool{}
	accept := func(a Action, o Outcome) {
		if o.Execution != "completed" || pattern(o.Outcome) == 0 || o.Outcome == "either" || len(a.Expected) != len(p.Hypotheses) {
			return
		}
		var remaining []string
		for _, h := range r.Hypotheses {
			if pattern(a.Expected[h])&pattern(o.Outcome) != 0 {
				remaining = append(remaining, h)
				if (o.Outcome == "success" || o.Outcome == "failure") && a.Expected[h] == o.Outcome {
					support[h] = true
				}
			}
		}
		r.Hypotheses = remaining
	}
	var usedExpiries []time.Time
	defer func() {
		if r.Status == "completed" {
			for _, expiry := range usedExpiries {
				if !time.Now().Before(expiry) {
					r.Status = "inconclusive"
					r.Reason = "initial_evidence_expired"
					break
				}
			}
		}
	}()
	for _, proof := range initial {
		if !proof.ValidUntil.IsZero() && !time.Now().Before(proof.ValidUntil) {
			continue
		}
		for _, a := range p.Actions {
			if a.ID != proof.ActionID {
				continue
			}
			for _, agent := range agents {
				if agent.ID == proof.AgentID && placementMatch(a.Placement, agent) {
					accept(a, proof.Outcome)
					if !proof.ValidUntil.IsZero() && proof.Outcome.Execution == "completed" && len(a.Expected) == len(p.Hypotheses) {
						usedExpiries = append(usedExpiries, proof.ValidUntil)
					}
					if proof.ObservationID != "" {
						tracker.add("observation:"+proof.ObservationID, a, agent, proof.Outcome, started, proof.ValidUntil)
					}
				}
			}
		}
	}
	finish := func(reason string) Result {
		r.Reason = reason
		if len(r.Hypotheses) == 0 && len(p.Hypotheses) > 0 {
			r.Reason = "evidence_conflict"
		}
		if len(r.Hypotheses) == 1 && support[r.Hypotheses[0]] && !(mode == "fixed" && fixedIncomplete) {
			r.Status = "completed"
			r.Reason = "supported_class"
			if p.Confirmation != nil {
				r.Reason = "confirmed_class"
			}
		}
		if mode == "fixed" && fixedIncomplete && len(r.Hypotheses) > 0 {
			r.Reason = "fixed_incomplete_evidence"
		}
		return r
	}
	visited := map[string]bool{}
	if mode == "adaptive" {
		for id := range ledger.adaptiveAttempted {
			visited[id] = true
		}
	}
	terminal := map[string]bool{}
	tasks := map[string]Outcome{}
	rounds := map[string]int{}
	actions := append([]Action(nil), p.Actions...)
	sort.Slice(actions, func(i, j int) bool { return actions[i].ID < actions[j].ID })
	steps := append([]Step(nil), p.Steps...)
	sort.Slice(steps, func(i, j int) bool { return steps[i].ID < steps[j].ID })
	for {
		if child.Err() != nil {
			r.Reason = "deadline_or_cancelled"
			return r, nil
		}
		if mode == "adaptive" {
			if len(r.Hypotheses) == 0 {
				r.Reason = "evidence_conflict"
				return r, nil
			}
			if len(r.Hypotheses) == 1 && p.Confirmation == nil {
				return finish("insufficient_support"), nil
			}
		}

		confirming := mode == "adaptive" && len(r.Hypotheses) == 1 && p.Confirmation != nil
		var confirmation *ConfirmationResult
		var covered, domains map[string]bool
		var lastProof time.Time
		if confirming {
			confirmation, covered, domains, lastProof = tracker.state(r.Hypotheses[0], actions, time.Now())
			if confirmation.Satisfied {
				return finish("insufficient_support"), nil
			}
			for _, a := range actions {
				if tracker.required(a.ID) && a.Expected[r.Hypotheses[0]] != "success" && a.Expected[r.Hypotheses[0]] != "failure" {
					r.Reason = "confirmation_pattern_unavailable"
					return r, nil
				}
			}
		}

		var candidates []Action
		stepID := ""
		if mode == "fixed" {
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
				ok := true
				var err error
				if step.When != nil {
					ok, err = step.When(child, tasks, monitorState)
				}
				if err != nil {
					fixedIncomplete = true
					r.BlockedSteps = append(r.BlockedSteps, step.ID)
				}
				if err != nil || !ok {
					terminal[step.ID] = true
					tasks[step.ID] = Outcome{"skipped", "unknown"}
					r.SkippedSteps = append(r.SkippedSteps, step.ID)
					continue
				}
				stepID = step.ID
				for _, a := range actions {
					if a.ID == step.ActionID {
						candidates = []Action{a}
					}
				}
				break
			}
			if stepID == "" {
				if len(terminal) < len(steps) {
					continue
				}
				return finish("fixed_finished_without_class"), nil
			}
		} else {
			for _, a := range actions {
				if confirming {
					if tracker.required(a.ID) && (len(covered) == len(p.Confirmation.ActionRefs) || !covered[a.ID]) {
						candidates = append(candidates, a)
					}
				} else if !visited[a.ID] {
					candidates = append(candidates, a)
				}
			}
			if len(candidates) == 0 {
				if confirming {
					r.Reason = "confirmation_unavailable"
					return r, nil
				}
				return finish("catalog_exhausted"), nil
			}
		}
		if ledger.UsedChecks >= p.Budget.MaxChecks {
			r.Reason = "check_budget_exhausted"
			return r, nil
		}
		var selected *Action
		var executor Executor
		best := -1.0
		selectedGain := 0
		var selectedBytes int64
		for i := range candidates {
			a := &candidates[i]
			if time.Now().Add(a.Check.Timeout).After(deadline) {
				continue
			}
			var eligible []Executor
			estimates := map[string]int64{}
			for _, agent := range agents {
				if !agent.Online || agent.Capacity <= agent.Active || agent.Capacity <= 0 || !agent.Supported[a.Check.Runner] || !agent.Supported[a.Check.Type] || !placementMatch(a.Placement, agent) || !e.Eligible(*a, agent) {
					continue
				}
				if confirming && (unusablePairs[a.ID+"/"+agent.ID] || !tracker.useful(*a, agent, confirmation, covered, domains)) {
					continue
				}
				if p.Budget.MaxEstimatedBytes > 0 {
					if e.EstimateBytes == nil {
						continue
					}
					n, known := e.EstimateBytes(*a, agent)
					if !known || n < 0 || n > p.Budget.MaxEstimatedBytes-ledger.EstimatedBytes {
						continue
					}
					estimates[agent.ID] = n
				}
				eligible = append(eligible, agent)
			}
			if len(eligible) == 0 {
				continue
			}
			gain := Gain(r.Hypotheses, a.Expected)
			score := float64(gain) / a.Cost
			if mode != "adaptive" || confirming {
				score = 0
			}
			if selected == nil || score > best {
				selected = a
				executor = chooseExecutor(a.Placement, eligible, rounds[a.ID])
				best = score
				selectedGain = gain
				selectedBytes = estimates[executor.ID]
			}
			if (mode != "adaptive" || confirming) && selected != nil {
				break
			}
		}
		if selected == nil {
			r.Reason = "no_admissible_action"
			if confirming {
				r.Reason = "confirmation_unavailable"
			}
			return r, nil
		}
		if mode == "adaptive" && !confirming && best <= 0 {
			r.Reason = "no_gain"
			return r, nil
		}
		if confirming && !lastProof.IsZero() {
			wait := time.Until(lastProof.Add(p.Confirmation.MinSpacing))
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-child.Done():
					timer.Stop()
					r.Reason = "deadline_or_cancelled"
					return r, nil
				case <-timer.C:
				}
				continue
			}
		}
		before := append([]string(nil), r.Hypotheses...)
		ledger.UsedChecks++
		ledger.EstimatedBytes += selectedBytes
		r.Attempts++
		r.Cost += selected.Cost
		attempt, done := context.WithTimeout(child, selected.Check.Timeout)
		outcome, err := e.Runner.Execute(attempt, *selected, executor)
		validExecution := (outcome.Execution == "completed" && (outcome.Outcome == "success" || outcome.Outcome == "failure" || outcome.Outcome == "unknown")) || (outcome.Execution == "execution_failed" && outcome.Outcome == "unknown")
		if err != nil || attempt.Err() != nil || !validExecution {
			outcome = Outcome{"execution_failed", "unknown"}
		}
		done()
		phase := ""
		if p.Confirmation != nil {
			phase = "primary"
			if confirming {
				phase = "confirmation"
			}
		}
		r.Decisions = append(r.Decisions, Decision{ActionID: selected.ID, AgentID: executor.ID, HypothesesBefore: before, Gain: selectedGain, Score: float64(selectedGain) / selected.Cost, Outcome: outcome, Phase: phase})
		if outcome.Execution != "completed" || (outcome.Outcome != "success" && outcome.Outcome != "failure") {
			unusablePairs[selected.ID+"/"+executor.ID] = true
			if mode == "fixed" {
				fixedIncomplete = true
			}
		}
		tracker.add(fmt.Sprintf("attempt:%d", ledger.UsedChecks), *selected, executor, outcome, time.Now(), deadline)
		accept(*selected, outcome)
		visited[selected.ID] = true
		if mode == "adaptive" {
			ledger.adaptiveAttempted[selected.ID] = true
		}
		rounds[selected.ID]++
		if stepID != "" {
			terminal[stepID] = true
			tasks[stepID] = outcome
		}
	}
}
