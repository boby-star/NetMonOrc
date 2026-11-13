//go:build linux

package agent

import (
	"golang.org/x/sys/unix"
	"time"
)

func bootTime() (time.Duration, error) {
	var t unix.Timespec
	err := unix.ClockGettime(unix.CLOCK_BOOTTIME, &t)
	return time.Duration(t.Sec)*time.Second + time.Duration(t.Nsec), err
}
