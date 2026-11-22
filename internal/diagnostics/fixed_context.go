package diagnostics

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
