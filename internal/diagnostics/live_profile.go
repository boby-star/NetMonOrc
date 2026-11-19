package diagnostics

import (
	"fmt"
	"time"
)

// ValidateLiveProfile enforces the existing serial Agent/lease runtime limits;
// the standalone experiment compiler remains independent of this deployment.
func ValidateLiveProfile(profile Profile) error {
	for _, action := range profile.Actions {
		if profile.Budget.MaxEstimatedBytes > 0 {
			if _, known := EstimateCheckBytes(action.Check); !known {
				return fmt.Errorf("action %s has no supported traffic estimate", action.ID)
			}
		}
		if action.Check.Timeout > 15*time.Second {
			return fmt.Errorf("research diagnostic timeout must be <=15s to fit a renewed 30s lease with startup/clock bounds")
		}
	}
	return nil
}
