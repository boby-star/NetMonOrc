package diagnostics

import (
	"context"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"reflect"
	"time"
)

type Trigger func(context.Context, monitor.Assessment) (bool, error)

// TriggerMonitorView is the closed context actually supplied by Assessment.
type TriggerMonitorView struct {
	Name          string `cel:"name"`
	State         string `cel:"state"`
	CoverageState string `cel:"coverageState"`
}

func CompileTrigger(expression string) (Trigger, error) {
	if len(expression) == 0 || len(expression) > 2048 {
		return nil, fmt.Errorf("invalid diagnostic trigger length")
	}
	env, e := cel.NewEnv(ext.NativeTypes(reflect.TypeOf(TriggerMonitorView{}), ext.ParseStructTags(true)), cel.Variable("monitor", cel.ObjectType("diagnostics.TriggerMonitorView")))
	if e != nil {
		return nil, e
	}
	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("diagnostic when must return bool")
	}
	p, e := env.Program(ast, cel.CostLimit(1024), cel.InterruptCheckFrequency(50))
	if e != nil {
		return nil, e
	}
	return func(ctx context.Context, a monitor.Assessment) (bool, error) {
		coverage := "sufficient"
		evidence := 0
		for _, z := range a.Zones {
			evidence += len(z.Evidence)
			if z.Coverage != "complete" {
				coverage = "degraded"
			}
		}
		if len(a.Zones) == 0 || evidence == 0 {
			coverage = "unknown"
		}
		c, done := context.WithTimeout(ctx, 25*time.Millisecond)
		defer done()
		v, _, e := p.ContextEval(c, map[string]any{"monitor": TriggerMonitorView{Name: a.Monitor, State: a.State, CoverageState: coverage}})
		if e != nil {
			return false, e
		}
		b, ok := v.Value().(bool)
		if !ok {
			return false, fmt.Errorf("trigger returned unknown")
		}
		return b, nil
	}, nil
}
