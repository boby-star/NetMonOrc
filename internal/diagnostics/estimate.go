package diagnostics

import "github.com/boby-star/NetMonOrc/internal/monitor"

// EstimateCheckBytes is the v1 planning model, not wire accounting. It includes
// protocol envelopes and configured response bounds. Retransmits, DNS/TLS and
// exporter traffic can differ on the wire. Unknown checks have no estimate.
func EstimateCheckBytes(c monitor.Check) (int64, bool) {
	if c.Runner != "blackbox" && c.Runner != "native-http" {
		return 0, false
	}
	switch c.Type {
	case "http":
		if c.MaxBodyBytes < 1 || c.MaxBodyBytes > 65536 {
			return 0, false
		}
		// Request/response headers and TLS setup; one body bound per hop.
		n := int64(32768) + c.MaxBodyBytes + int64(len(c.URL))
		for key, value := range c.Headers {
			n += int64(len(key) + len(value) + 4)
		}
		if c.FollowRedirects {
			n *= 10
		}
		return n, true
	case "tcp":
		return 4096, c.Runner == "blackbox" && c.TCP != nil
	case "dns":
		return 8192, c.Runner == "blackbox" && c.DNS != nil
	case "icmp":
		if c.ICMP == nil || c.ICMP.PayloadSize < 0 || c.ICMP.PayloadSize > 65507 {
			return 0, false
		}
		return 2 * (int64(c.ICMP.PayloadSize) + 128), c.Runner == "blackbox"
	}
	return 0, false
}

func EstimateBytes(a Action, _ Executor) (int64, bool) { return EstimateCheckBytes(a.Check) }
