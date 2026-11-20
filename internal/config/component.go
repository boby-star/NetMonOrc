package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/boby-star/NetMonOrc/internal/component"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func VerifyComponents(bindings map[string]monitor.ComponentBinding) error {
	if len(bindings) > 100 {
		return fmt.Errorf("too many Component bindings")
	}
	for agent, b := range bindings {
		endpoint, err := component.NormalizeEndpoint(b.Endpoint)
		hash, e := hex.DecodeString(b.Hash)
		if !namePattern.MatchString(agent) || !namePattern.MatchString(b.Ref) || err != nil || endpoint != b.Endpoint || e != nil || len(hash) != 32 || b.ModuleSet != component.ModuleSet {
			return fmt.Errorf("invalid resolved Component binding")
		}
	}
	return nil
}

func ResolveComponents(p monitor.Plan, bindings map[string]monitor.ComponentBinding) (monitor.Plan, error) {
	if err := VerifyComponents(bindings); err != nil {
		return p, err
	}
	p.Components = bindings
	p.Revision = ""
	b, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved plan too large")
	}
	sum := sha256.Sum256(b)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}
