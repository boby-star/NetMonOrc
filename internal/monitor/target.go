package monitor

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
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

func (c Check) NeedsHTTPPolicy() bool {
	if c.Type != "http" {
		return false
	}
	if c.ConnectIP != "" || c.FollowRedirects || len(c.Headers) > 0 {
		return true
	}
	u, err := url.Parse(c.URL)
	if err != nil || u == nil {
		return false
	}
	_, err = netip.ParseAddr(u.Hostname())
	return c.Runner == "native-http" && err != nil
}
