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
type TriggerMonitorView = monitor.RuntimeMonitorView

func CompileTrigger(expression string, plans ...monitor.Plan) (Trigger, error) {
	if len(expression) == 0 || len(expression) > 4096 {
		return nil, fmt.Errorf("invalid diagnostic trigger length")
	}
	env, e := cel.NewEnv(ext.NativeTypes(reflect.TypeOf(monitor.RuntimeMonitorView{}), reflect.TypeOf(monitor.TargetView{}), ext.ParseStructTags(true)), cel.Variable("monitor", cel.ObjectType("monitor.RuntimeMonitorView")), cel.Variable("target", cel.ObjectType("monitor.TargetView")))
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
		plan := monitor.Plan{}
		if len(plans) > 0 {
			plan = plans[0]
		}
		c, done := context.WithTimeout(ctx, 25*time.Millisecond)
		defer done()
		v, _, e := p.ContextEval(c, map[string]any{"monitor": monitor.MonitorContext(plan, a), "target": monitor.TargetCELContext(plan)})
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
