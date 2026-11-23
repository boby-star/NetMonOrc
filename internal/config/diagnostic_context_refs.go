package config

import (
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/celrefs"
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func ValidateDiagnosticMonitor(p monitor.Plan, profile diagnostics.Profile) error {
	checks := map[string]bool{}
	for _, c := range p.Checks {
		checks[c.ID] = true
	}
	zones := map[string]bool{}
	for _, z := range p.Zones {
		zones[z] = true
	}
	env, err := fixedEnvironment()
	if err != nil {
		return err
	}
	for _, step := range profile.Steps {
		if step.Expression == "" {
			continue
		}
		ast, issues := env.Compile(step.Expression)
		if issues.Err() != nil {
			return issues.Err()
		}
		refs := append(celrefs.StaticMapKeys(ast.Expr(), "checks"), celrefs.StaticMapKeys(ast.Expr(), "zones.*.checks")...)
		for _, ref := range refs {
			if !checks[ref] {
				return fmt.Errorf("fixed step %s references unknown Monitor check %q", step.ID, ref)
			}
		}
		for _, ref := range celrefs.StaticMapKeys(ast.Expr(), "zones") {
			if !zones[ref] {
				return fmt.Errorf("fixed step %s references unknown zone %q", step.ID, ref)
			}
		}
	}
	return nil
}
