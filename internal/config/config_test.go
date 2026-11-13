package config

import (
	"os"
	"strings"
	"testing"
)

func TestCompileClosedSubset(t *testing.T) {
	b, e := os.ReadFile("../../examples/http.yaml")
	if e != nil {
		t.Fatal(e)
	}
	p, e := Compile(b)
	if e != nil {
		t.Fatal(e)
	}
	if len(p.Revision) != 64 || len(p.Checks) != 1 || !p.Checks[0].Required {
		t.Fatalf("bad plan %+v", p)
	}
	x, e := Compile([]byte(strings.ReplaceAll(string(b), "expectedStatus: [200]", "expectedStatus: [200]\n        method: GET")))
	if e != nil || x.Revision != p.Revision {
		t.Fatal("explicit defaults changed revision", e)
	}
	for _, change := range [][2]string{{"runner: native-http", "runner: blackbox"}, {"expectedStatus: [200]", "expectedStatus: []"}, {"expectedStatus: [200]", "followRedirects: true"}, {"timeout: 2s", "timeout: 30s"}, {"weight: 1", "weight: .nan"}, {"address: 127.0.0.1", "address: 127.0.0.1\n    unsupported: true"}, {"weight: 1", "weight: 0"}} {
		if _, e = Compile([]byte(strings.ReplaceAll(string(b), change[0], change[1]))); e == nil {
			t.Fatalf("accepted invalid change %v", change)
		}
	}
}
