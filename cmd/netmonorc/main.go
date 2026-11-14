// netmonorc provides research operations and authenticated Controller access.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/boby-star/NetMonOrc/internal/config"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"github.com/boby-star/NetMonOrc/internal/probe"
)

func main() {
	if e := run(os.Args[1:]); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: netmonorc login|logout|whoami|validate|probe|assess [flags]")
	}
	if args[0] == "login" || args[0] == "logout" || args[0] == "whoami" {
		return authCommand(args)
	}
	f := flag.NewFlagSet(args[0], flag.ContinueOnError)
	manifest := f.String("config", "examples/http.yaml", "Monitor manifest")
	agent := f.String("agent", "", "local observation source ID (not authenticated)")
	zone := f.String("zone", "", "source zone")
	allow := f.String("allow-cidrs", "", "comma-separated explicit target CIDRs")
	slotsArg := f.String("slots", "", "planned agent=zone pairs, one per zone")
	blackboxBinary := f.String("blackbox-binary", "blackbox_exporter", "prepared Blackbox Exporter 0.27.0 executable")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	file, e := os.Open(*manifest)
	if e != nil {
		return e
	}
	b, e := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	closeErr := file.Close()
	if e != nil {
		return e
	}
	if closeErr != nil {
		return closeErr
	}
	p, e := config.Compile(b)
	if e != nil {
		return e
	}
	enc := json.NewEncoder(os.Stdout)
	switch args[0] {
	case "validate":
		return enc.Encode(p)
	case "probe":
		validZone := false
		for _, z := range p.Zones {
			if z == *zone {
				validZone = true
			}
		}
		if *agent == "" || !validZone || *allow == "" {
			return fmt.Errorf("probe requires agent, configured zone and explicit allow-cidrs")
		}
		var networks []netip.Prefix
		for _, part := range strings.Split(*allow, ",") {
			n, e := netip.ParsePrefix(part)
			if e != nil {
				return e
			}
			networks = append(networks, n)
		}
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		var exporter *probe.Exporter
		for _, c := range p.Checks {
			if c.Runner == "blackbox" {
				exporter, e = probe.StartExporter(ctx, *blackboxBinary, p)
				if e != nil {
					return e
				}
				defer exporter.Close()
				break
			}
		}
		for _, c := range p.Checks {
			var o monitor.Observation
			if c.Runner == "blackbox" {
				o, e = exporter.Run(ctx, p, c, monitor.Slot{Agent: *agent, Zone: *zone}, networks)
			} else {
				o, e = probe.HTTP(ctx, p, c, monitor.Slot{Agent: *agent, Zone: *zone}, networks)
			}
			if e != nil {
				return e
			}
			if e = enc.Encode(o); e != nil {
				return e
			}
		}
		return nil
	case "assess":
		var slots []monitor.Slot
		if *slotsArg == "" {
			return fmt.Errorf("assess requires explicit planned slots; received results cannot define coverage")
		}
		for _, entry := range strings.Split(*slotsArg, ",") {
			a, z, ok := strings.Cut(entry, "=")
			if !ok {
				return fmt.Errorf("slot must be agent=zone")
			}
			slots = append(slots, monitor.Slot{Agent: a, Zone: z})
		}
		dec := json.NewDecoder(io.LimitReader(os.Stdin, 32<<20))
		dec.DisallowUnknownFields()
		var history []monitor.Observation
		for {
			var o monitor.Observation
			e = dec.Decode(&o)
			if e == io.EOF {
				break
			}
			if e != nil {
				return e
			}
			history = append(history, o)
			if len(history) > 10000 {
				return fmt.Errorf("too many observations for local assessment")
			}
		}
		a, e := monitor.Evaluate(p, slots, history, time.Now().UTC())
		if e != nil {
			return e
		}
		return enc.Encode(a)
	default:
		return fmt.Errorf("unsupported command %q", args[0])
	}
}
