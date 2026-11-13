package config

import (
	"strings"
	"testing"
)

const extractManifest = `apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata: {name: extract-demo}
spec:
  target: {address: 127.0.0.1}
  checks:
    - id: http
      type: http
      config: {url: 'http://127.0.0.1/'}
      extract:
        - {name: queue_depth, from: response.body, pattern: 'depth=(?P<value>[0-9]+)', group: value, as: integer}
  assessment: {requiredZones: [office]}
`

func TestExtractionCompile(t *testing.T) {
	p, e := Compile([]byte(extractManifest))
	if e != nil {
		t.Fatal(e)
	}
	c := p.Checks[0]
	if c.Runner != "native-http" || len(c.Extract) != 1 || c.Extract[0].Group != 1 {
		t.Fatal(c)
	}
	if e = VerifyPlan(p, "office"); e != nil {
		t.Fatal(e)
	}
	for _, change := range [][2]string{{"type: http", "type: http\n      runner: blackbox"}, {"group: value", "group: 0"}, {"group: value", "group: missing"}, {"name: queue_depth", "name: queueDepth"}, {"as: integer", "as: arbitrary"}, {"from: response.body", "from: filesystem"}} {
		if _, e = Compile([]byte(strings.ReplaceAll(extractManifest, change[0], change[1]))); e == nil {
			t.Fatalf("invalid extractor accepted: %v", change)
		}
	}
}
