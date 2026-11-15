package config

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func ResolveFixedPlacement(p monitor.Plan, placement *monitor.FixedPlacement) (monitor.Plan, error) {
	p.FixedPlacement = placement
	p.Revision = ""
	data, err := json.Marshal(p)
	if err != nil {
		return p, err
	}
	if len(data) > (1<<20)-4096 {
		return p, fmt.Errorf("resolved plan exceeds transport budget")
	}
	sum := sha256.Sum256(data)
	p.Revision = hex.EncodeToString(sum[:])
	return p, nil
}

var rosterAgentID = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)

func verifyFixedPlacement(placement *monitor.FixedPlacement, agent, zone string) error {
	hash, err := hex.DecodeString(placement.Hash)
	if err != nil || len(hash) != 32 || (placement.Ref != "" && !namePattern.MatchString(placement.Ref)) || placement.Replicas < 1 || placement.Replicas > 10 || len(placement.Roster) > 100 || len(placement.Selected) > placement.Replicas {
		return fmt.Errorf("invalid fixed placement bounds")
	}
	if len(placement.Selected) != 0 && len(placement.Selected) != placement.Replicas {
		return fmt.Errorf("partial fixed placement is forbidden")
	}
	registered := map[string]string{}
	for _, slot := range placement.Roster {
		if !rosterAgentID.MatchString(slot.Agent) || slot.Zone == "" || len(slot.Zone) > 63 || registered[slot.Agent] != "" {
			return fmt.Errorf("invalid roster binding")
		}
		registered[slot.Agent] = slot.Zone
	}
	if agent == "" || registered[agent] != zone {
		return fmt.Errorf("placement requires the authenticated Agent identity and zone")
	}
	agents, zones := map[string]bool{}, map[string]bool{}
	for _, slot := range placement.Selected {
		if registered[slot.Agent] != slot.Zone || agents[slot.Agent] || zones[slot.Zone] {
			return fmt.Errorf("invalid selected placement source")
		}
		agents[slot.Agent] = true
		zones[slot.Zone] = true
	}
	return nil
}
