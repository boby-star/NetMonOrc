package report

import (
	"github.com/boby-star/NetMonOrc/internal/diagnostics"
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"strings"
	"testing"
	"time"
)

func TestReportEscapesEvidenceAndMarksHistoricalDiagnostic(t *testing.T) {
	hostile := `<script>alert("evidence")</script>`
	a := monitor.Assessment{Monitor: hostile, Revision: "new", At: time.Now(), State: "unknown", Zones: []monitor.ZoneAssessment{{Zone: "office", State: "unknown", Coverage: "coverage_degraded", Reason: hostile}}}
	d := &diagnostics.Report{Monitor: hostile, Revision: "old", Result: diagnostics.Result{Status: "inconclusive", Reason: hostile}}
	b, e := Render(a, d)
	if e != nil {
		t.Fatal(e)
	}
	s := string(b)
	if strings.Contains(s, hostile) || !strings.Contains(s, "&lt;script&gt;") {
		t.Fatal("untrusted evidence reached HTML unescaped")
	}
	if !strings.Contains(s, "попередній ревізії") || !strings.Contains(s, "не означає успішне виконання") {
		t.Fatal("historical/unknown diagnostic could be mistaken for a current conclusion")
	}
	d.Monitor = "other"
	if _, e = Render(a, d); e == nil {
		t.Fatal("unrelated monitor diagnostic rendered")
	}
}
