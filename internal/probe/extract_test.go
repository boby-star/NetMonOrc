package probe

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	"testing"
)

func TestTypedExtraction(t *testing.T) {
	spec := []monitor.Extractor{{Name: "depth", Pattern: `depth=(\d+)`, Group: 1, As: "integer", OnMissing: "unknown"}, {Name: "ready", Pattern: `ready=(\w+)`, Group: 1, As: "boolean", OnMissing: "unknown"}, {Name: "ratio", Pattern: `ratio=(\S+)`, Group: 1, As: "float", OnMissing: "error"}}
	v, status := extract([]byte("depth=0 ready=false ratio=1.0"), spec)
	if status["depth"] != "success" || status["ready"] != "success" || status["ratio"] != "success" || v["depth"].Kind != "integer" || v["depth"].Integer != 0 || v["ready"].Kind != "boolean" || v["ready"].Boolean || v["ratio"].Kind != "float" {
		t.Fatalf("zero/false/type lost: %+v %+v", v, status)
	}
	v, status = extract([]byte("depth=1 ratio=1.0"), spec)
	if status["ready"] != "missing" || status["depth"] != "success" {
		t.Fatal(status)
	}
	if _, exists := v["ready"]; exists {
		t.Fatal("missing value fabricated")
	}
	_, status = extract([]byte("depth=1 ready=invalid ratio=NaN"), spec)
	if status["ready"] != "error" || status["ratio"] != "error" {
		t.Fatal("invalid conversions accepted", status)
	}
	v, status = extract([]byte("depth=999999999999999999999999 ready=true ratio=1.0"), spec)
	if status["depth"] != "error" {
		t.Fatal("integer overflow accepted")
	}
	if _, ok := v["depth"]; ok {
		t.Fatal("overflow value fabricated")
	}
}
