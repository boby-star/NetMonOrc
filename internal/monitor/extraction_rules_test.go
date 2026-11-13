package monitor

import (
	"testing"
	"time"
)

func TestExtractedScalarsInCEL(t *testing.T) {
	now := time.Now().UTC()
	p := Plan{Name: "web", Revision: "current", Zones: []string{"office"}, MaxAge: time.Minute, MaxSkew: time.Second, MinCoverage: 1, HealthyMin: .9, DegradedMin: .5, Checks: []Check{{ID: "http", Type: "http", Weight: 1, Required: true}}, Rules: []AssessmentRule{{ID: "typed-values", Priority: 100, When: `has(checks["http"].values.ready) && !checks["http"].values.ready && checks["http"].values.queue_depth == 0 && checks["http"].values.serial == 9007199254740993 && checks["http"].values.version == "v1" && checks["http"].values.ratio == 1.0`, State: "down", Reason: "APPLICATION_NOT_READY"}}}
	o := Observation{ID: "one", Monitor: p.Name, Revision: p.Revision, Check: "http", Agent: "branch", Zone: "office", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success", StatusCode: 200, Values: map[string]Scalar{"ready": {Kind: "boolean", Boolean: false}, "queue_depth": {Kind: "integer", Integer: 0}, "serial": {Kind: "integer", Integer: 9007199254740993}, "version": {Kind: "string", Text: "v1"}, "ratio": {Kind: "float", Number: 1}}}
	o.ExtractionStatus = map[string]string{}
	for name := range o.Values {
		o.ExtractionStatus[name] = "success"
	}
	a, e := Evaluate(p, []Slot{{Agent: "branch", Zone: "office"}}, []Observation{o}, now)
	if e != nil {
		t.Fatal(e)
	}
	if a.State != "down" || a.Zones[0].Reason != "APPLICATION_NOT_READY" {
		t.Fatalf("CEL lost scalar types or precision: %+v", a)
	}
}
