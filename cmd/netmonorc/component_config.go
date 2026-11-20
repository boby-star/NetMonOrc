package main

import (
	"flag"
	"fmt"
	"os"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
)

func componentConfigCommand(args []string) error {
	f := flag.NewFlagSet("component-config", flag.ContinueOnError)
	file := f.String("f", "-", "supplied YAML/JSON catalog, or - for stdin")
	slots := f.String("slots", "", "offline Agent=zone roster")
	labels := f.String("agent-labels-file", "", "trusted offline Agent labels")
	cidrs := f.String("allow-cidrs", "", "offline webhook validation CIDRs")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	b, err := readCLIInput(*file)
	if err != nil {
		return err
	}
	registry, compiled, _, err := resolveLocalCatalog(b, *slots, *cidrs, *labels)
	if err != nil {
		return err
	}
	keys := make([]string, 0, len(compiled))
	for k := range compiled {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	profiles := make([]string, 0, len(registry.Profiles))
	for name := range registry.Profiles {
		profiles = append(profiles, name)
	}
	sort.Strings(profiles)
	var plans []monitor.Plan
	for _, key := range keys {
		entry := compiled[key]
		if entry.Monitor == nil {
			continue
		}
		p := *entry.Monitor
		plans = append(plans, p)
		// Include every supplied diagnostic profile for every target context;
		// manual profile selection must not accidentally use a regular module.
		for _, name := range profiles {
			p.Checks = nil
			for _, a := range registry.Profiles[name].Actions {
				p.Checks = append(p.Checks, a.Check)
			}
			plans = append(plans, p)
		}
	}
	if len(plans) == 0 {
		return fmt.Errorf("at least one Monitor required for module target context")
	}
	config, err := probe.ExternalModules(plans)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(append(config, '\n'))
	return err
}
