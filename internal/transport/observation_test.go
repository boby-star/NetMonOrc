package transport

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"strings"
	"testing"
	"time"
)

func TestProvenanceBinding(t *testing.T) {
	now := time.Now().UTC()
	o := monitor.Observation{ID: strings.Repeat("a", 32), Monitor: "web", Revision: strings.Repeat("b", 64), Check: "http", Agent: "branch", Zone: "office", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "failure"}
	w := Encode(o)
	if _, e := Decode(w, "branch", "office", "web"); e != nil {
		t.Fatal(e)
	}
	if _, e := Decode(w, "other", "office", "web"); e == nil {
		t.Fatal("spoofed Agent accepted")
	}
	w.Execution = "execution_failed"
	if _, e := Decode(w, "branch", "office", "web"); e == nil {
		t.Fatal("execution failure accepted as negative probe")
	}
	w.Outcome = "unknown"
	if _, e := Decode(w, "branch", "office", "web"); e != nil {
		t.Fatal(e)
	}
}
