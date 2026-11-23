package config

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/boby-star/NetMonOrc/internal/celrefs"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/resourcegroup"
	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
	"google.golang.org/protobuf/types/known/anypb"

	"gopkg.in/yaml.v3"
)

type diagnosticDocument struct {
	APIVersion string `yaml:"apiVersion"`
	Kind       string `yaml:"kind"`
	Metadata   struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		Mode   string `yaml:"mode"`
		Budget struct {
			MaxChecks   int    `yaml:"maxAdditionalChecks"`
			MaxDuration string `yaml:"maxDuration"`
			MaxParallel *int   `yaml:"maxParallel"`
			MaxBytes    *int64 `yaml:"maxEstimatedBytes"`
		} `yaml:"budget"`
		Hypotheses []struct {
			ID          string `yaml:"id"`
			Description string `yaml:"description"`
		} `yaml:"hypotheses"`
		Actions []struct {
			ID    string `yaml:"id"`
			Retry *struct {
				MaxAttempts int `yaml:"maxAttempts"`
			} `yaml:"retry"`
			Check struct {
				Type    string        `yaml:"type"`
				Runner  string        `yaml:"runner"`
				Timeout string        `yaml:"timeout"`
				Config  yaml.Node     `yaml:"config"`
				Extract []extractSpec `yaml:"extract"`
			} `yaml:"check"`
			Expected  map[string]string `yaml:"expectedOutcomes"`
			Cost      *float64          `yaml:"costUnits"`
			Placement *struct {
				AgentRefs            []string                `yaml:"agentRefs"`
				Selector             *resourcegroup.Selector `yaml:"selector"`
				DistinctBy           string                  `yaml:"distinctBy"`
				Zones                []string                `yaml:"zones"`
				Strategy             string                  `yaml:"strategy"`
				Replicas             *int                    `yaml:"replicas"`
				OnInsufficientAgents string                  `yaml:"onInsufficientAgents"`
			} `yaml:"placement"`
		} `yaml:"actions"`
		Fixed *struct {
			Steps []struct {
				ID        string   `yaml:"id"`
				ActionRef string   `yaml:"actionRef"`
				DependsOn []string `yaml:"dependsOn"`
				When      *string  `yaml:"when"`
			} `yaml:"steps"`
		} `yaml:"fixed"`
		Adaptive *struct {
			Algorithm      string `yaml:"algorithm"`
			StopWhenUnique *bool  `yaml:"stopWhenUnique"`
			MaxNoGainSteps int    `yaml:"maxNoGainSteps"`
			TieBreak       string `yaml:"tieBreak"`
			Confirmation   *struct {
				ActionRefs                []string `yaml:"actionRefs"`
				MinObservations           *int     `yaml:"minObservations"`
				MinDistinctFailureDomains *int     `yaml:"minDistinctFailureDomains"`
				MinSpacing                *string  `yaml:"minSpacing"`
			} `yaml:"confirmation"`
		} `yaml:"adaptive"`
	} `yaml:"spec"`
}

func CompileDiagnostics(data []byte) (diagnostics.Profile, error) {
	p := diagnostics.Profile{}
	if len(data) > 1<<20 {
		return p, fmt.Errorf("diagnostic profile exceeds 1 MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	d.KnownFields(true)
	var m diagnosticDocument
	var tree yaml.Node
	if e := yaml.Unmarshal(data, &tree); e != nil {
		return p, e
	}
	if len(tree.Content) != 1 {
		return p, fmt.Errorf("empty DiagnosticProfile")
	}
	if e := scalarTypes(tree.Content[0], reflect.TypeOf(m)); e != nil {
		return p, e
	}
	if e := d.Decode(&m); e != nil {
		return p, e
	}
	var extra any
	if e := d.Decode(&extra); e != io.EOF {
		return p, fmt.Errorf("one DiagnosticProfile document required")
	}
	if m.APIVersion != "netmonorc.io/v1alpha1" || m.Kind != "DiagnosticProfile" || !namePattern.MatchString(m.Metadata.Name) {
		return p, fmt.Errorf("invalid DiagnosticProfile envelope")
	}
	s := m.Spec
	p.Name = m.Metadata.Name
	p.Mode = s.Mode
	budget, e := duration(s.Budget.MaxDuration, 0)
	if e != nil {
		return p, e
	}
	p.Budget = diagnostics.Budget{MaxChecks: s.Budget.MaxChecks, MaxDuration: budget, MaxParallel: 1}
	if s.Budget.MaxParallel != nil {
		p.Budget.MaxParallel = *s.Budget.MaxParallel
	}
	if s.Budget.MaxBytes != nil {
		if *s.Budget.MaxBytes <= 0 {
			return p, fmt.Errorf("maxEstimatedBytes must be positive")
		}
		p.Budget.MaxEstimatedBytes = *s.Budget.MaxBytes
	}
	for _, h := range s.Hypotheses {
		if !idPattern.MatchString(h.ID) || len(h.Description) > 1024 {
			return p, fmt.Errorf("invalid hypothesis")
		}
		p.Hypotheses = append(p.Hypotheses, h.ID)
	}
	for _, a := range s.Actions {
		if !idPattern.MatchString(a.ID) {
			return p, fmt.Errorf("invalid diagnostic action ID")
		}
		c, e := compileProbe(checkSpec{Type: a.Check.Type, Runner: a.Check.Runner, Config: a.Check.Config, Extract: a.Check.Extract}, "diagnostic-target")
		if e != nil {
			return p, e
		}
		c.ID = a.ID
		c.Timeout, e = duration(a.Check.Timeout, 5*time.Second)
		if e != nil || c.Timeout < 100*time.Millisecond || c.Timeout > 60*time.Second {
			return p, fmt.Errorf("invalid diagnostic timeout")
		}
		x := diagnostics.Action{ID: a.ID, Check: c, Cost: 1, Expected: a.Expected, Placement: diagnostics.Placement{Strategy: "leastLoaded", Inherit: a.Placement == nil}}
		if a.Retry != nil {
			x.Retry = &diagnostics.Retry{MaxAttempts: a.Retry.MaxAttempts}
		}
		if a.Cost != nil {
			x.Cost = *a.Cost
		}
		if v := a.Placement; v != nil {
			if v.Replicas != nil && (*v.Replicas < 1 || *v.Replicas > 10) {
				return p, fmt.Errorf("diagnostic replicas must be within 1..10")
			}
			if v.OnInsufficientAgents != "" && v.OnInsufficientAgents != "pending" {
				return p, fmt.Errorf("partial placement is disabled")
			}
			selector, err := resourcegroup.NormalizeAgentSelector(v.Selector)
			if err != nil {
				return p, err
			}
			x.Placement.Selector = selector
			if v.Replicas != nil && *v.Replicas > 1 {
				x.Placement.Replicas = *v.Replicas
			}
			if v.DistinctBy != "" && v.DistinctBy != "none" {
				if v.DistinctBy != "zone" && v.DistinctBy != "failureDomain" {
					return p, fmt.Errorf("invalid diagnostic distinctBy")
				}
				x.Placement.DistinctBy = v.DistinctBy
			}
			x.Placement.AgentRefs = v.AgentRefs
			x.Placement.Zones = v.Zones
			if v.Strategy != "" {
				x.Placement.Strategy = v.Strategy
			}
			for _, values := range [][]string{v.AgentRefs, v.Zones} {
				seen := map[string]bool{}
				for _, value := range values {
					if value == "" || len(value) > 63 || seen[value] {
						return p, fmt.Errorf("invalid diagnostic placement filter")
					}
					seen[value] = true
				}
			}
		}
		p.Actions = append(p.Actions, x)
	}
	if p.Mode == "adaptive" {
		if s.Adaptive == nil || s.Fixed != nil {
			return p, fmt.Errorf("adaptive profile requires adaptive block only")
		}
		v := s.Adaptive
		if v.Algorithm != "" && v.Algorithm != "distinguish-pairs-per-cost" {
			return p, fmt.Errorf("unsupported adaptive algorithm")
		}
		p.ContinueAfterUnique = v.StopWhenUnique != nil && !*v.StopWhenUnique
		if c := v.Confirmation; c != nil {
			if c.MinObservations == nil || c.MinDistinctFailureDomains == nil || c.MinSpacing == nil || *c.MinSpacing == "" {
				return p, fmt.Errorf("confirmation requires actionRefs, minObservations, minDistinctFailureDomains and minSpacing")
			}
			if !durationPattern.MatchString(*c.MinSpacing) {
				return p, fmt.Errorf("invalid confirmation minSpacing")
			}
			spacing, err := time.ParseDuration(*c.MinSpacing)
			if err != nil {
				return p, err
			}
			refs := append([]string(nil), c.ActionRefs...)
			sort.Strings(refs)
			p.Confirmation = &diagnostics.Confirmation{ActionRefs: refs, MinObservations: *c.MinObservations, MinDistinctFailureDomains: *c.MinDistinctFailureDomains, MinSpacing: spacing}
		}
		if v.MaxNoGainSteps < 0 || v.MaxNoGainSteps > 5 {
			return p, fmt.Errorf("adaptive.maxNoGainSteps must be within 0..5")
		}
		p.MaxNoGainSteps = v.MaxNoGainSteps
		if v.TieBreak != "" && v.TieBreak != "actionIdAsc" {
			return p, fmt.Errorf("invalid adaptive tieBreak")
		}
	} else if p.Mode == "fixed" {
		if s.Fixed == nil || s.Adaptive != nil {
			return p, fmt.Errorf("fixed profile requires fixed block only")
		}
		for _, v := range s.Fixed.Steps {
			if !idPattern.MatchString(v.ID) {
				return p, fmt.Errorf("invalid fixed step ID")
			}
			step := diagnostics.Step{ID: v.ID, ActionID: v.ActionRef, DependsOn: v.DependsOn}
			if v.When != nil {
				step.Expression = *v.When
			}
			p.Steps = append(p.Steps, step)
		}
	}
	if err := diagnostics.Validate(p); err != nil {
		return p, err
	}
	if p.Mode == "fixed" {
		predecessors := diagnostics.FixedPredecessors(p.Steps)
		for i, v := range s.Fixed.Steps {
			if v.When != nil {
				p.Steps[i].WhenContext, e = diagnosticContextCondition(*v.When, predecessors[v.ID])
				if e != nil {
					return p, fmt.Errorf("fixed step %s: %w", v.ID, e)
				}
			}
		}
	}
	return p, nil
}

// Fixed CEL views expose only fields currently supplied by the engine.
type PhysicalTaskView struct {
	Values          map[string]*anypb.Any `cel:"values"`
	ReasonCode      string                `cel:"reasonCode"`
	Outcome         string                `cel:"outcome"`
	ExecutionStatus string                `cel:"executionStatus"`
	AgentID         string                `cel:"agentId"`
	Zone            string                `cel:"zone"`
	FailureDomain   string                `cel:"failureDomain"`
	ObservationID   string                `cel:"observationId"`
}
type FixedTaskView struct {
	Values          map[string]*anypb.Any       `cel:"values"`
	ReasonCode      string                      `cel:"reasonCode"`
	Outcome         string                      `cel:"outcome"`
	ExecutionStatus string                      `cel:"executionStatus"`
	ReplicaResults  map[string]PhysicalTaskView `cel:"replicaResults"`
}
type FixedMonitorView = monitor.RuntimeMonitorView

func diagnosticCondition(expression string, predecessors ...map[string]bool) (func(context.Context, map[string]diagnostics.Outcome, string) (bool, error), error) {
	condition, err := diagnosticEvidenceCondition(expression, predecessors...)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tasks map[string]diagnostics.Outcome, state string) (bool, error) {
		input := map[string]diagnostics.TaskEvidence{}
		for id, outcome := range tasks {
			input[id] = diagnostics.TaskEvidence{Outcome: outcome}
		}
		return condition(ctx, input, state)
	}, nil
}

func diagnosticEvidenceCondition(expression string, predecessors ...map[string]bool) (diagnostics.EvidenceCondition, error) {
	condition, err := diagnosticContextCondition(expression, predecessors...)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context, tasks map[string]diagnostics.TaskEvidence, state string) (bool, error) {
		return condition(ctx, tasks, diagnostics.FixedContext{Monitor: monitor.RuntimeMonitorView{State: state}})
	}, nil
}

func fixedEnvironment() (*cel.Env, error) {
	return cel.NewEnv(ext.NativeTypes(reflect.TypeOf(FixedTaskView{}), reflect.TypeOf(PhysicalTaskView{}), reflect.TypeOf(monitor.RuntimeMonitorView{}), reflect.TypeOf(monitor.TargetView{}), reflect.TypeOf(monitor.EvaluatedZoneView{}), reflect.TypeOf(monitor.CheckView{}), reflect.TypeOf(diagnostics.DiagnosticCounters{}), ext.ParseStructTags(true)), cel.CrossTypeNumericComparisons(true),
		cel.Variable("tasks", cel.MapType(cel.StringType, cel.ObjectType("config.FixedTaskView"))),
		cel.Variable("monitor", cel.ObjectType("monitor.RuntimeMonitorView")), cel.Variable("target", cel.ObjectType("monitor.TargetView")),
		cel.Variable("checks", cel.MapType(cel.StringType, cel.ObjectType("monitor.CheckView"))),
		cel.Variable("zone", cel.ObjectType("monitor.EvaluatedZoneView")), cel.Variable("zones", cel.MapType(cel.StringType, cel.ObjectType("monitor.EvaluatedZoneView"))),
		cel.Variable("diagnostic", cel.ObjectType("diagnostics.DiagnosticCounters")))
}

func diagnosticContextCondition(expression string, predecessors ...map[string]bool) (diagnostics.ContextCondition, error) {
	if len(expression) > 4096 || strings.TrimSpace(expression) == "" {
		return nil, fmt.Errorf("invalid CEL expression size")
	}
	env, e := fixedEnvironment()
	if e != nil {
		return nil, e
	}
	ast, issues := env.Compile(expression)
	if issues.Err() != nil {
		return nil, issues.Err()
	}
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("fixed when must return bool")
	}
	if len(predecessors) > 0 {
		for _, ref := range celrefs.StaticMapKeys(ast.Expr(), "tasks") {
			if !predecessors[0][ref] {
				return nil, fmt.Errorf("task %q must be a dependsOn predecessor", ref)
			}
		}
	}
	program, e := env.Program(ast, cel.CostLimit(1024), cel.InterruptCheckFrequency(50))
	if e != nil {
		return nil, e
	}
	return func(ctx context.Context, tasks map[string]diagnostics.TaskEvidence, frame diagnostics.FixedContext) (bool, error) {
		input := map[string]FixedTaskView{}
		for id, t := range tasks {
			physical := physicalTaskView(t)
			view := FixedTaskView{Values: physical.Values, ReasonCode: physical.ReasonCode, Outcome: physical.Outcome, ExecutionStatus: physical.ExecutionStatus, ReplicaResults: map[string]PhysicalTaskView{}}
			for actor, result := range t.ReplicaResults {
				view.ReplicaResults[actor] = physicalTaskView(result)
			}
			input[id] = view
		}
		activation := map[string]any{"tasks": input, "monitor": frame.Monitor, "target": frame.Target, "diagnostic": frame.Diagnostic, "zones": frame.Zones}
		if frame.Zone != nil {
			activation["zone"] = *frame.Zone
			activation["checks"] = frame.Checks
		}

		out, _, e := program.ContextEval(ctx, activation)
		if e != nil {
			return false, e
		}
		value, ok := out.Value().(bool)
		if !ok {
			return false, fmt.Errorf("unknown CEL result")
		}
		return value, nil
	}, nil
}

func physicalTaskView(t diagnostics.TaskEvidence) PhysicalTaskView {
	execution := t.Outcome.Execution
	if execution == "execution_failed" {
		execution = "failed"
	}
	if execution == "skipped" {
		execution = "not_executed"
	}
	values := t.Values
	if !t.ValidUntil.IsZero() && !time.Now().Before(t.ValidUntil) {
		values = nil
	}
	return PhysicalTaskView{Values: monitor.ScalarValues(values), ReasonCode: t.ReasonCode, Outcome: t.Outcome.Outcome, ExecutionStatus: execution, AgentID: t.AgentID, Zone: t.Zone, FailureDomain: t.FailureDomain, ObservationID: t.ObservationID}
}
