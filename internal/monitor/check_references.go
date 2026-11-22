package monitor

import (
	"fmt"

	"github.com/boby-star/NetMonOrc/internal/celrefs"
	"github.com/google/cel-go/cel"
	exprpb "google.golang.org/genproto/googleapis/api/expr/v1alpha1"
)

// AssessmentCheckReferences returns statically named global checks. Reusable
// policies without a checks list resolve these names only when bound to a Monitor.
// Computed keys remain subject to bounded CEL evaluation and missing-data gates.
func AssessmentCheckReferences(expression string) ([]string, error) {
	if len(expression) == 0 || len(expression) > 4096 {
		return nil, fmt.Errorf("invalid assessment expression size")
	}
	env, err := assessmentEnvironment()
	if err != nil {
		return nil, err
	}
	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("assessment rule must return bool")
	}
	return assessmentCheckReferences(ast.Expr()), nil
}

func assessmentCheckReferences(root *exprpb.Expr) []string {
	return celrefs.StaticMapKeys(root, "checks")
}
