package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/dependency"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func ResolveDependencyContext(p monitor.Plan, edges []dependency.Edge) (monitor.Plan, error) {
	if len(edges) == 0 {
		return p, nil
	}
	if err := dependency.ValidateGraph(edges, nil); err != nil {
		return p, err
	}
	p.Dependencies = edges
	p.Revision = ""
	raw, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(raw) > (1<<20)-4096 {
		return p, fmt.Errorf("dependency context exceeds plan transport budget")
	}
	sum := sha256.Sum256(raw)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}
