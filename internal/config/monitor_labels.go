package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func ResolveMonitorLabels(p monitor.Plan, labels map[string]string) (monitor.Plan, error) {
	if len(labels) == 0 {
		return p, nil
	}
	p.Labels = map[string]string{}
	for k, v := range labels {
		p.Labels[k] = v
	}
	p.Revision = ""
	b, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(b) > (1<<20)-4096 {
		return p, fmt.Errorf("plan exceeds transport budget")
	}
	sum := sha256.Sum256(b)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}
