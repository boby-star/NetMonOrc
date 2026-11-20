package probe

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sort"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

var ErrRunnerUnavailable = errors.New("runner_unavailable")

// StartAgentExporter preserves the prepared local runner for legacy assignments.
// An explicit external binding never falls back to spawning a different runner.
func StartAgentExporter(ctx context.Context, binary string, p monitor.Plan, agent string) (*Exporter, error) {
	b, explicit := p.Components[agent]
	if !explicit {
		return StartExporter(ctx, binary, p)
	}
	endpoint, err := component.NormalizeEndpoint(b.Endpoint)
	if !b.Enabled || err != nil || b.ModuleSet != component.ModuleSet {
		return nil, ErrRunnerUnavailable
	}
	return &Exporter{endpoint: endpoint, external: true, stop: func() {}}, nil
}

// standard-network is the release's closed module translator. Operators prepare
// these exact content-addressed modules offline; Component apply never reloads
// or writes an external exporter. A missing module is a technical failure.
func externalModule(p monitor.Plan, c monitor.Check) (string, map[string]any, error) {
	m, err := blackboxModule(p, c)
	if err != nil {
		return "", nil, err
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "", nil, err
	}
	sum := sha256.Sum256(append([]byte(component.ModuleSet+"/v1\x00"), b...))
	return "nmo_" + hex.EncodeToString(sum[:]), m, nil
}

// ExternalModules renders exporter config without network or process side effects.
func ExternalModules(plans []monitor.Plan) ([]byte, error) {
	modules := map[string]any{}
	for _, p := range plans {
		for _, c := range p.Checks {
			if c.Runner != "blackbox" {
				continue
			}
			id, m, err := externalModule(p, c)
			if err != nil {
				return nil, err
			}
			modules[id] = m
		}
	}
	if len(modules) == 0 {
		return nil, fmt.Errorf("no Blackbox checks in supplied catalog")
	}
	b, err := json.MarshalIndent(map[string]any{"modules": modules}, "", "  ")
	if len(b) > 8<<20 {
		return nil, fmt.Errorf("module catalog exceeds 8 MiB")
	}
	return b, err
}

func dialLocalExporter(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, ErrRunnerUnavailable
	}
	var ips []netip.Addr
	if ip, e := netip.ParseAddr(host); e == nil {
		ips = []netip.Addr{ip}
	} else {
		ips, err = net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if err != nil {
			return nil, ErrRunnerUnavailable
		}
	}
	sort.Slice(ips, func(i, j int) bool { return ips[i].Unmap().Less(ips[j].Unmap()) })
	last := ErrRunnerUnavailable
	for _, ip := range ips {
		if !component.LocalAddress(ip) {
			continue
		}
		conn, e := (&net.Dialer{}).DialContext(ctx, network, net.JoinHostPort(ip.Unmap().String(), port))
		if e == nil {
			return conn, nil
		}
		last = e
	}
	return nil, last
}
