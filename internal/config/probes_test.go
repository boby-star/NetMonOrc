package config

import (
	"os"
	"strings"
	"testing"
)

func TestNetworkPlan(t *testing.T) {
	b, e := os.ReadFile("../../examples/network.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := Compile(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Checks) != 4 {
		t.Fatal("missing network checks")
	}
	if e = VerifyPlan(p, "office"); e != nil {
		t.Fatal(e)
	}
	if p.Checks[0].DNS == nil || p.Checks[0].DNS.RecordType != "A" || p.Checks[2].ICMP.PayloadSize != 32 || p.Checks[3].TCP.PreferredIPProtocol != "ip4" {
		t.Fatalf("incorrect defaults: %+v", p.Checks)
	}
	for _, change := range [][2]string{
		{"port: 18080", "port: 1.5"},
		{"port: 18080", "port: 65536"},
		{"port: 18080", "port: 80\n        url: http://127.0.0.1/"},
		{"payloadSize: 32", "payloadSize: 1401"},
		{"payloadSize: 32", "payloadSize: 32\n        dontFragment: yes"},
		{"preferredIPProtocol: ip4", "preferredIPProtocol: ip6\n        dontFragment: true"},
		{"resolver: 127.0.0.1:1053", "resolver: dns.example:53"},
		{"queryName: health.example", "queryName: -invalid.example"},
		{"recordType: A", "recordType: ANY"}, // outside the DSL's supported record types
		{"transport: udp", "transport: tls"},
		{"recordType: A", "recordType: A\n        expectedRcodes: []"},
	} {
		if _, e = Compile([]byte(strings.ReplaceAll(string(b), change[0], change[1]))); e == nil {
			t.Fatalf("accepted invalid network configuration %v", change)
		}
	}
}
