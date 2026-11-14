package diagnostics

import (
	"fmt"
	"time"
)

// ValidateLiveProfile enforces the existing serial Agent/lease runtime limits;
// the standalone experiment compiler remains independent of this deployment.
func ValidateLiveProfile(profile Profile) error {
	if profile.Budget.MaxEstimatedBytes != 0 {
		return fmt.Errorf("live diagnostic byte estimates are not measured; byte budget unsupported")
	}
	for _, action := range profile.Actions {
		if action.Check.Timeout > 15*time.Second {
			return fmt.Errorf("research diagnostic timeout must be <=15s to fit a renewed 30s lease with startup/clock bounds")
		}
	}
	return nil
}
