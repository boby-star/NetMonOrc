//go:build !linux

package agent

import (
	"fmt"
	"time"
)

func bootTime() (time.Duration, error) {
	return 0, fmt.Errorf("Agent lease requires Linux CLOCK_BOOTTIME")
}
