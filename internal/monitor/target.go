package monitor

import (
	"fmt"
	"net"
	"strconv"
)

// ProbeTarget does not resolve names or perform any network operation.
func ProbeTarget(p Plan, c Check) (string, error) {
	switch c.Type {
	case "http":
		return c.URL, nil
	case "tcp":
		if c.TCP != nil {
			return net.JoinHostPort(p.TargetAddress, strconv.Itoa(c.TCP.Port)), nil
		}
	case "icmp":
		if c.ICMP != nil {
			return p.TargetAddress, nil
		}
	case "dns":
		if c.DNS != nil {
			return c.DNS.Resolver, nil
		}
	}
	return "", fmt.Errorf("unsupported or incomplete probe")
}
