package monitor

import (
	"context"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"time"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Views are an explicit CEL contract, separate from storage and wire DTOs.
// Native tags keep DSL field names and reject recursive zone.state at compile.
type CheckView struct {
	Values          map[string]*anypb.Any `cel:"values"`
	Outcome         string                `cel:"outcome"`
	ExecutionStatus string                `cel:"executionStatus"`
	ReasonCode      string                `cel:"reasonCode"`
	Metrics         map[string]int64      `cel:"metrics"`
	AgeSeconds      float64               `cel:"ageSeconds"`
}
type ZoneView struct {
	Name     string            `cel:"name"`
	Coverage string            `cel:"coverage"`
	Labels   map[string]string `cel:"labels"`
}
type TargetView struct {
	Address     string            `cel:"address"`
	Type        string            `cel:"type"`
	NetworkZone string            `cel:"networkZone"`
	Criticality int               `cel:"criticality"`
	Labels      map[string]string `cel:"labels"`
}
type compiledRule struct {
	rule    AssessmentRule
	program cel.Program
}
type Assessor struct {
	plan  Plan
	rules []compiledRule
}

var ruleID = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,63}$`)
var ruleReason = regexp.MustCompile(`^[A-Z0-9_]+$`)

func NewAssessor(p Plan) (*Assessor, error) {
	a := &Assessor{plan: p}
	if len(p.Rules) == 0 {
		return a, nil
	}
	if len(p.Rules) > 100 {
		return nil, fmt.Errorf("at most 100 assessment rules supported")
	}
	env, e := cel.NewEnv(ext.NativeTypes(reflect.TypeOf(CheckView{}), reflect.TypeOf(ZoneView{}), reflect.TypeOf(TargetView{}), ext.ParseStructTags(true)), cel.CrossTypeNumericComparisons(true), cel.Variable("checks", cel.MapType(cel.StringType, cel.ObjectType("monitor.CheckView"))), cel.Variable("zone", cel.ObjectType("monitor.ZoneView")), cel.Variable("target", cel.ObjectType("monitor.TargetView")))
	if e != nil {
		return nil, e
	}
	rules := append([]AssessmentRule(nil), p.Rules...)
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].Priority == rules[j].Priority {
			return rules[i].ID < rules[j].ID
		}
		return rules[i].Priority > rules[j].Priority
	})
	seen := map[string]bool{}
	for _, r := range rules {
		if !ruleID.MatchString(r.ID) || seen[r.ID] || r.Priority < 1 || r.Priority > 1000 || !ruleReason.MatchString(r.Reason) || len(r.Reason) > 128 || len(r.When) == 0 || len(r.When) > 4096 {
			return nil, fmt.Errorf("invalid assessment rule %q", r.ID)
		}
		seen[r.ID] = true
		if r.State != "healthy" && r.State != "degraded" && r.State != "down" && r.State != "unknown" {
			return nil, fmt.Errorf("invalid rule state")
		}
		ast, issues := env.Compile(r.When)
		if issues.Err() != nil {
			return nil, fmt.Errorf("rule %s: %w", r.ID, issues.Err())
		}
		if ast.OutputType() != cel.BoolType {
			return nil, fmt.Errorf("rule %s must return bool", r.ID)
		}
		program, e := env.Program(ast, cel.CostLimit(1024), cel.InterruptCheckFrequency(50))
		if e != nil {
			return nil, e
		}
		a.rules = append(a.rules, compiledRule{r, program})
	}
	return a, nil
}

func (a *Assessor) applyRules(z *ZoneAssessment, latest map[string]Observation, now time.Time, coherent, requiredFailure, requiredMissing bool) {
	if len(a.rules) == 0 || requiredFailure || !coherent {
		return
	}
	checks := map[string]CheckView{}
	for _, c := range a.plan.Checks {
		o, ok := latest[c.ID]
		if !ok {
			continue
		}
		v := CheckView{Outcome: "unknown", ExecutionStatus: "failed", ReasonCode: o.Reason, AgeSeconds: now.Sub(o.FinishedAt).Seconds(), Metrics: map[string]int64{}, Values: map[string]*anypb.Any{}}
		if o.Execution == "completed" && (o.Outcome == "success" || o.Outcome == "failure") {
			for name, value := range o.Values {
				var wrapped *anypb.Any
				var err error
				switch value.Kind {
				case "integer":
					wrapped, err = anypb.New(wrapperspb.Int64(value.Integer))
				case "float":
					wrapped, err = anypb.New(wrapperspb.Double(value.Number))
				case "boolean":
					wrapped, err = anypb.New(wrapperspb.Bool(value.Boolean))
				case "string":
					wrapped, err = anypb.New(wrapperspb.String(value.Text))
				}
				if err == nil && wrapped != nil {
					v.Values[name] = wrapped
				}
			}
			v.Outcome = o.Outcome
			v.ExecutionStatus = "completed"
			if c.Type == "http" && o.StatusCode != 0 {
				v.Metrics["httpStatus"] = int64(o.StatusCode)
			}
		}
		checks[c.ID] = v
	}
	coverage := "degraded"
	if z.Coverage == "complete" {
		coverage = "sufficient"
	}
	if len(z.Evidence) == 0 {
		coverage = "unknown"
	}
	activation := map[string]any{"checks": checks, "zone": ZoneView{Name: z.Zone, Coverage: coverage, Labels: map[string]string{}}, "target": TargetView{Address: a.plan.TargetAddress, Type: "endpoint", NetworkZone: "unspecified", Criticality: 3, Labels: map[string]string{}}}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	for _, r := range a.rules {
		value, _, e := r.program.ContextEval(ctx, activation)
		if e != nil {
			z.RuleErrors = append(z.RuleErrors, r.rule.ID)
			continue
		}
		matched, ok := value.Value().(bool)
		if !ok {
			z.RuleErrors = append(z.RuleErrors, r.rule.ID)
			continue
		}
		if !matched {
			continue
		}
		z.State = r.rule.State
		z.Reason = r.rule.Reason
		break
	}
	// An evaluation error is not ordinary false. Even a lower-priority
	// healthy rule cannot erase missing context or mandatory proof gates.
	if z.State == "healthy" && (requiredMissing || z.Q < a.plan.MinCoverage || len(z.RuleErrors) > 0) {
		z.State = "unknown"
		z.Reason = "rule_evaluation_incomplete"
	}
}
