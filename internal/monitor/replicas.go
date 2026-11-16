package monitor

import (
	"fmt"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
	"sort"
	"time"
)

func voteThreshold(mode, outcome string, n int) int {
	if n == 0 {
		return 1
	}
	switch mode {
	case "any":
		if outcome == "success" {
			return 1
		}
		return n
	case "quorum":
		return n/2 + 1
	default:
		if outcome == "failure" {
			return 1
		}
		return n
	}
}
func validOutcome(o Observation) bool {
	return o.Execution == "completed" && (o.Outcome == "success" || o.Outcome == "failure")
}

// A decisive required failure can stand independently of other checks, but its
// own physical witnesses must fit maxSkew. Missing votes never reduce planned N.
func failureWitness(mode string, n int, observations []Observation, skew time.Duration) []Observation {
	var failures []Observation
	for _, o := range observations {
		if validOutcome(o) && o.Outcome == "failure" {
			failures = append(failures, o)
		}
	}
	sort.Slice(failures, func(i, j int) bool {
		if failures[i].FinishedAt.Equal(failures[j].FinishedAt) {
			return failures[i].ID < failures[j].ID
		}
		return failures[i].FinishedAt.Before(failures[j].FinishedAt)
	})
	k := voteThreshold(mode, "failure", n)
	for end := len(failures) - 1; end >= k-1; end-- {
		if failures[end].FinishedAt.Sub(failures[end-k+1].FinishedAt) <= skew {
			return failures[end-k+1 : end+1]
		}
	}
	return nil
}
func aggregateReplicas(mode string, n int, observations []Observation) string {
	if n == 0 {
		return "unknown"
	}
	successes, failures := 0, 0
	for _, o := range observations {
		if !validOutcome(o) {
			continue
		}
		if o.Outcome == "success" {
			successes++
		} else {
			failures++
		}
	}
	if failures >= voteThreshold(mode, "failure", n) {
		return "failure"
	}
	if successes >= voteThreshold(mode, "success", n) {
		return "success"
	}
	return "unknown"
}
func (a *Assessor) ValidateSources(slots []Slot) error {
	if !a.replicaValues {
		return nil
	}
	counts := map[string]int{}
	for _, slot := range slots {
		counts[slot.Zone]++
		if counts[slot.Zone] > 1 {
			return fmt.Errorf("CEL values/metrics for multiple same-zone replicas are deferred; use outcomes or separate Monitor resources")
		}
	}
	return nil
}

// Inspect the checked CEL tree, including macros and bracket notation. String
// literals containing the word metrics alone are not interpreted as field access.
func replicaValueAccess(e *exprpb.Expr, types map[int64]*exprpb.Type) bool {
	if e == nil {
		return false
	}
	switch v := e.ExprKind.(type) {
	case *exprpb.Expr_SelectExpr:
		return ((v.SelectExpr.Field == "values" || v.SelectExpr.Field == "metrics") && types[v.SelectExpr.Operand.Id].GetMessageType() == "monitor.CheckView") || replicaValueAccess(v.SelectExpr.Operand, types)
	case *exprpb.Expr_CallExpr:
		if v.CallExpr.Function == "_[_]" && len(v.CallExpr.Args) == 2 && types[v.CallExpr.Args[0].Id].GetMessageType() == "monitor.CheckView" {
			return true
		}
		if replicaValueAccess(v.CallExpr.Target, types) {
			return true
		}
		for _, arg := range v.CallExpr.Args {
			if replicaValueAccess(arg, types) {
				return true
			}
		}
	case *exprpb.Expr_ListExpr:
		for _, value := range v.ListExpr.Elements {
			if replicaValueAccess(value, types) {
				return true
			}
		}
	case *exprpb.Expr_StructExpr:
		for _, entry := range v.StructExpr.Entries {
			if replicaValueAccess(entry.GetMapKey(), types) || replicaValueAccess(entry.Value, types) {
				return true
			}
		}
	case *exprpb.Expr_ComprehensionExpr:
		c := v.ComprehensionExpr
		return replicaValueAccess(c.IterRange, types) || replicaValueAccess(c.AccuInit, types) || replicaValueAccess(c.LoopCondition, types) || replicaValueAccess(c.LoopStep, types) || replicaValueAccess(c.Result, types)
	}
	return false
}
