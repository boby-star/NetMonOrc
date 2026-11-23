package alerting

import (
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/celrefs"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"reflect"
)

func Environment(scope string) (*cel.Env, error) {
	opts := []cel.EnvOption{ext.NativeTypes(reflect.TypeOf(monitor.RuntimeMonitorView{}), reflect.TypeOf(monitor.TargetView{}), reflect.TypeOf(monitor.EvaluatedZoneView{}), reflect.TypeOf(monitor.CheckView{}), ext.ParseStructTags(true)), cel.CrossTypeNumericComparisons(true), cel.Variable("monitor", cel.ObjectType("monitor.RuntimeMonitorView")), cel.Variable("target", cel.ObjectType("monitor.TargetView")), cel.Variable("checks", cel.MapType(cel.StringType, cel.ObjectType("monitor.CheckView"))), cel.Variable("zones", cel.MapType(cel.StringType, cel.ObjectType("monitor.EvaluatedZoneView")))}
	if scope == "zone" {
		opts = append(opts, cel.Variable("zone", cel.ObjectType("monitor.EvaluatedZoneView")))
	}
	return cel.NewEnv(opts...)
}
func (r Rule) Matches(name string, labels map[string]string) bool {
	if r.Selector != nil {
		return r.Selector.Match(labels)
	}
	return r.Monitor == name
}
func ZoneContext(z monitor.ZoneAssessment) monitor.EvaluatedZoneView {
	coverage := "degraded"
	if z.Coverage == "complete" {
		coverage = "sufficient"
	}
	if len(z.Evidence) == 0 {
		coverage = "unknown"
	}
	return monitor.EvaluatedZoneView{Name: z.Zone, State: z.State, Coverage: coverage, Labels: map[string]string{}, Checks: z.Checks, WeightedScore: z.H}
}
func Activation(p monitor.Plan, whole, scoped monitor.Assessment, scope string) map[string]any {
	zones := map[string]monitor.EvaluatedZoneView{}
	for _, z := range whole.Zones {
		zones[z.Zone] = ZoneContext(z)
	}
	out := map[string]any{"monitor": monitor.MonitorContext(p, whole), "target": monitor.TargetCELContext(p), "zones": zones}
	if scope == "zone" && len(scoped.Zones) == 1 {
		z := ZoneContext(scoped.Zones[0])
		out["zone"] = z
		out["checks"] = z.Checks
	} else if len(whole.Zones) == 1 {
		out["checks"] = whole.Zones[0].Checks
	}
	return out
}
func ValidateMonitor(r Rule, p monitor.Plan) error {
	env, err := Environment(r.Scope)
	if err != nil {
		return err
	}
	checks := map[string]bool{}
	for _, c := range p.Checks {
		checks[c.ID] = true
	}
	zones := map[string]bool{}
	for _, z := range p.Zones {
		zones[z] = true
	}
	for _, expression := range []string{r.When, r.ResolveWhen} {
		ast, issues := env.Compile(expression)
		if issues.Err() != nil {
			return issues.Err()
		}
		refs := append(celrefs.StaticMapKeys(ast.Expr(), "checks"), celrefs.StaticMapKeys(ast.Expr(), "zones.*.checks")...)
		refs = append(refs, celrefs.StaticMapKeys(ast.Expr(), "zone.checks")...)
		for _, ref := range refs {
			if !checks[ref] {
				return fmt.Errorf("alert %s references unknown Monitor check %q", r.Name, ref)
			}
		}
		for _, ref := range celrefs.StaticMapKeys(ast.Expr(), "zones") {
			if !zones[ref] {
				return fmt.Errorf("alert %s references unknown zone %q", r.Name, ref)
			}
		}
	}
	return nil
}
