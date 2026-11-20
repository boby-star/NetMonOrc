// Package component defines the closed external exporter endpoint contract.
package component

import (
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"regexp"
	"strings"
)

const ModuleSet = "standard-network"

var localName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func LocalAddress(ip netip.Addr) bool {
	ip = ip.Unmap()
	return (ip.IsPrivate() || ip.IsLoopback()) && !ip.IsUnspecified() && !ip.IsLinkLocalUnicast() && !ip.IsMulticast() && ip.Zone() == ""
}

// NormalizeEndpoint admits local Docker names or explicit private/loopback IPs.
// DNS names are provisional: the Agent rechecks the actual IP before dialing.
func NormalizeEndpoint(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || u == nil || len(raw) > 2048 || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") || u.RawPath != "" || u.Opaque != "" {
		return "", fmt.Errorf("Component requires a local http endpoint without credentials, path, query or fragment")
	}
	host := strings.ToLower(u.Hostname())
	if ip, e := netip.ParseAddr(host); e == nil {
		if !LocalAddress(ip) {
			return "", fmt.Errorf("public or forbidden Component IP")
		}
		host = ip.Unmap().String()
	} else if !localName.MatchString(host) {
		return "", fmt.Errorf("Component hostname must be a local Docker service name")
	}
	port := u.Port()
	if port == "" {
		port = "80"
	}
	addr, err := netip.ParseAddrPort("127.0.0.1:" + port)
	if err != nil || addr.Port() == 0 {
		return "", fmt.Errorf("invalid Component port")
	}
	return "http://" + net.JoinHostPort(host, fmt.Sprint(addr.Port())), nil
}
