package probe

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"net/netip"
	"testing"
)

func TestProtocolPolicies(t *testing.T) {
	p := monitor.Plan{TargetAddress: "169.254.169.254"}
	allow := []netip.Prefix{netip.MustParsePrefix("0.0.0.0/0")}
	for _, c := range []monitor.Check{
		{Type: "tcp", TCP: &monitor.TCPConfig{Port: 80, PreferredIPProtocol: "ip4"}},
		{Type: "icmp", ICMP: &monitor.ICMPConfig{PreferredIPProtocol: "ip4"}},
		{Type: "dns", DNS: &monitor.DNSConfig{Resolver: "169.254.169.254:53"}},
	} {
		if _, e := validateTarget(p, c, allow); e == nil {
			t.Fatalf("metadata target accepted: %s", c.Type)
		}
	}
	c := monitor.Check{Type: "dns", DNS: &monitor.DNSConfig{Resolver: "127.0.0.1:1053", QueryName: "outside.example"}}
	target, e := validateTarget(p, c, []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")})
	if e != nil || target != "127.0.0.1:1053" {
		t.Fatal("DNS policy must check resolver, not query answer", target, e)
	}
}
func TestICMPClassification(t *testing.T) {
	for _, tc := range []struct {
		logs     string
		negative bool
	}{
		{`msg="Timeout reading from socket" err="i/o timeout"`, true},
		{`msg="Error listening to socket" err="operation not permitted"`, false},
		{`msg="Error writing to socket" err="network is unreachable"`, true},
		{`msg="Error writing to socket" err="permission denied"`, false},
		{"", false},
	} {
		if icmpNegative(tc.logs) != tc.negative {
			t.Fatalf("incorrect ICMP classification %q", tc.logs)
		}
	}
}
