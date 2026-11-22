// Package celrefs inspects global map references in checked CEL expressions.
package celrefs

import (
	"sort"

	"github.com/google/cel-go/common/operators"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// StaticMapKeys returns sorted literal keys and respects macro variable scopes.
// Computed keys remain a runtime concern.
func StaticMapKeys(root *exprpb.Expr, variable string) []string {
	names := map[string]bool{}
	var visit func(*exprpb.Expr, bool)
	visit = func(e *exprpb.Expr, shadowed bool) {
		if e == nil {
			return
		}
		global := func(x *exprpb.Expr) bool { return !shadowed && x.GetIdentExpr().GetName() == variable }
		literal := func(x *exprpb.Expr) {
			if c, ok := x.GetConstExpr().GetConstantKind().(*exprpb.Constant_StringValue); ok {
				names[c.StringValue] = true
			}
		}
		switch n := e.ExprKind.(type) {
		case *exprpb.Expr_SelectExpr:
			if global(n.SelectExpr.Operand) {
				names[n.SelectExpr.Field] = true
			}
			visit(n.SelectExpr.Operand, shadowed)
		case *exprpb.Expr_CallExpr:
			c := n.CallExpr
			if len(c.Args) == 2 {
				if c.Function == operators.Index && global(c.Args[0]) {
					literal(c.Args[1])
				}
				if c.Function == operators.In && global(c.Args[1]) {
					literal(c.Args[0])
				}
			}
			visit(c.Target, shadowed)
			for _, arg := range c.Args {
				visit(arg, shadowed)
			}
		case *exprpb.Expr_ListExpr:
			for _, item := range n.ListExpr.Elements {
				visit(item, shadowed)
			}
		case *exprpb.Expr_StructExpr:
			for _, entry := range n.StructExpr.Entries {
				visit(entry.GetMapKey(), shadowed)
				visit(entry.Value, shadowed)
			}
		case *exprpb.Expr_ComprehensionExpr:
			c := n.ComprehensionExpr
			visit(c.IterRange, shadowed)
			visit(c.AccuInit, shadowed)
			loopShadow := shadowed || c.IterVar == variable || c.IterVar2 == variable || c.AccuVar == variable
			visit(c.LoopCondition, loopShadow)
			visit(c.LoopStep, loopShadow)
			visit(c.Result, shadowed || c.AccuVar == variable)
		}
	}
	visit(root, false)
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}
