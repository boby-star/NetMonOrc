package config

import (
	"os"
	"strings"
	"testing"
)

func TestResponseSchemaAndRevision(t *testing.T) {
	base, err := os.ReadFile("../../examples/http.yaml")
	if err != nil {
		t.Fatal(err)
	}
	addition := `  responses:
    - id: isolate
      when: 'monitor.state == "down"'
      blockPort: {agentRef: branch, protocol: tcp, port: 18088, sourceCIDR: 188.137.254.31/32, duration: 60s}
`
	original, err := Compile(base)
	if err != nil {
		t.Fatal(err)
	}
	source := string(base) + addition
	plan, err := Compile([]byte(source))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Responses) != 1 || plan.Revision == original.Revision {
		t.Fatal("response not bound to revision")
	}
	if err = VerifyPlan(plan, plan.Zones[0]); err != nil {
		t.Fatal(err)
	}
	for _, change := range [][2]string{{"port: 18088", "port: 22"}, {"port: 18088", "port: 9443"}, {"port: 18088", "port: 8443"}, {"protocol: tcp", "protocol: udp"}, {"duration: 60s", "duration: 1h"}, {"duration: 60s", "duration: 1s"}, {"188.137.254.31/32", "0.0.0.0/0"}, {"188.137.254.31/32", "127.0.0.1/32"}, {"monitor.state", "monitor.sttae"}, {"agentRef: branch", "agentRef: branch, command: 'rm -rf /'"}, {"port: 18088", "port: '18088'"}} {
		if _, err = Compile([]byte(strings.Replace(source, change[0], change[1], 1))); err == nil {
			t.Fatalf("accepted invalid change %v", change)
		}
	}
	tampered := plan
	tampered.Responses[0].Port = 22
	if VerifyPlan(tampered, plan.Zones[0]) == nil {
		t.Fatal("tampered IR accepted")
	}
}
