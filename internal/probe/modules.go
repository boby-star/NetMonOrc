package probe

import (
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func blackboxModule(p monitor.Plan, c monitor.Check) (map[string]any, error) {
	target, e := monitor.ProbeTarget(p, c)
	if e != nil {
		return nil, e
	}
	protocol := "ip4"
	var ip netip.Addr
	switch c.Type {
	case "http":
		u, e := url.Parse(target)
		if e != nil {
			return nil, e
		}
		ip, _ = netip.ParseAddr(u.Hostname())
	case "tcp", "dns":
		a, e := netip.ParseAddrPort(target)
		if e == nil {
			ip = a.Addr()
		}
	case "icmp":
		ip, _ = netip.ParseAddr(target)
	}
	if ip.Is6() && !ip.Is4In6() {
		protocol = "ip6"
	}
	m := map[string]any{"prober": c.Type, "timeout": c.Timeout.String()}
	switch c.Type {
	case "http":
		m["http"] = map[string]any{"method": c.Method, "valid_status_codes": c.ExpectedStatus, "follow_redirects": false, "preferred_ip_protocol": protocol, "ip_protocol_fallback": false, "body_size_limit": strconv.FormatInt(c.MaxBodyBytes, 10) + "B", "tls_config": map[string]bool{"insecure_skip_verify": false}}
	case "tcp":
		m["tcp"] = map[string]any{"preferred_ip_protocol": c.TCP.PreferredIPProtocol, "ip_protocol_fallback": c.TCP.IPProtocolFallback}
	case "icmp":
		m["icmp"] = map[string]any{"preferred_ip_protocol": c.ICMP.PreferredIPProtocol, "ip_protocol_fallback": c.ICMP.IPProtocolFallback, "payload_size": c.ICMP.PayloadSize, "dont_fragment": c.ICMP.DontFragment}
	case "dns":
		d := c.DNS
		v := map[string]any{"preferred_ip_protocol": protocol, "ip_protocol_fallback": false, "query_name": d.QueryName, "query_type": d.RecordType, "transport_protocol": d.Transport, "valid_rcodes": d.ExpectedRcodes}
		if d.ExpectedRecordRegex != "" {
			v["validate_answer_rrs"] = map[string]any{"fail_if_none_matches_regexp": []string{d.ExpectedRecordRegex}}
		}
		m["dns"] = v
	default:
		return nil, fmt.Errorf("unsupported Blackbox prober")
	}
	return m, nil
}

func validateTarget(p monitor.Plan, c monitor.Check, allowed []netip.Prefix) (string, error) {
	target, e := monitor.ProbeTarget(p, c)
	if e != nil {
		return "", e
	}
	var ip netip.Addr
	switch c.Type {
	case "http":
		if e = allowedTarget(target, allowed); e != nil {
			return "", e
		}
		return target, nil
	case "tcp", "dns":
		a, err := netip.ParseAddrPort(target)
		if err != nil || a.Port() == 0 {
			return "", fmt.Errorf("IP-literal endpoint with port required")
		}
		ip = a.Addr()
	case "icmp":
		ip, e = netip.ParseAddr(target)
		if e != nil {
			return "", fmt.Errorf("IP-literal ICMP target required")
		}
	default:
		return "", fmt.Errorf("unsupported target type")
	}
	if e = allowedAddress(ip, allowed); e != nil {
		return "", e
	}
	family := "ip4"
	if ip.Unmap().Is6() {
		family = "ip6"
	}
	if c.Type == "tcp" && c.TCP.PreferredIPProtocol != family && !c.TCP.IPProtocolFallback {
		return "", fmt.Errorf("TCP target and IP protocol differ without fallback")
	}
	if c.Type == "icmp" && ((c.ICMP.PreferredIPProtocol != family && !c.ICMP.IPProtocolFallback) || (c.ICMP.DontFragment && family != "ip4")) {
		return "", fmt.Errorf("ICMP target and IP options incompatible")
	}
	return target, nil
}

func icmpDebug(body string) (string, string, error) {
	const metrics = "Metrics that would have been returned:"
	const module = "Module configuration:"
	before, after, ok := strings.Cut(body, metrics)
	if !ok || !strings.HasPrefix(body, "Logs for the probe:") {
		return "", "", fmt.Errorf("invalid ICMP debug response")
	}
	data, _, ok := strings.Cut(after, module)
	if !ok {
		return "", "", fmt.Errorf("incomplete ICMP debug response")
	}
	return before, data, nil
}
func icmpNegative(logs string) bool {
	if strings.Contains(logs, "operation not permitted") || strings.Contains(logs, "permission denied") {
		return false
	}
	if strings.Contains(logs, `msg="Timeout reading from socket"`) {
		return true
	}
	return strings.Contains(logs, `msg="Error writing to socket"`) && (strings.Contains(logs, "network is unreachable") || strings.Contains(logs, "no route to host") || strings.Contains(logs, "host is unreachable"))
}
