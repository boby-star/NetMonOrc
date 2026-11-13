package transport

import (
	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
	"google.golang.org/protobuf/proto"
	"strings"
	"testing"
	"time"
)

func TestExtractedValueWireRoundtrip(t *testing.T) {
	now := time.Now().UTC()
	o := monitor.Observation{ID: strings.Repeat("a", 32), Monitor: "web", Revision: strings.Repeat("b", 64), Check: "http", Agent: "branch", Zone: "office", StartedAt: now, FinishedAt: now, Execution: "completed", Outcome: "success", Values: map[string]monitor.Scalar{"zero": {Kind: "integer", Integer: 0}, "large": {Kind: "integer", Integer: 9007199254740993}, "flag": {Kind: "boolean", Boolean: false}, "ratio": {Kind: "float", Number: 1}}}
	o.ExtractionStatus = map[string]string{}
	for name := range o.Values {
		o.ExtractionStatus[name] = "success"
	}
	b, e := proto.Marshal(Encode(o))
	if e != nil {
		t.Fatal(e)
	}
	var wire pb.Observation
	if e = proto.Unmarshal(b, &wire); e != nil {
		t.Fatal(e)
	}
	got, e := Decode(&wire, "branch", "office", "web")
	if e != nil {
		t.Fatal(e)
	}
	for k, v := range o.Values {
		if got.Values[k] != v || got.ExtractionStatus[k] != "success" {
			t.Fatalf("wire changed %s: %+v", k, got.Values)
		}
	}
}

func TestRejectInconsistentExtractionEvidence(t *testing.T) {
	for _, w := range []*pb.Observation{
		{ExtractionStatus: map[string]string{"ready": "success"}},
		{Values: map[string]*pb.ScalarValue{"ready": {Value: &pb.ScalarValue_BooleanValue{BooleanValue: false}}}},
		{ExtractionStatus: map[string]string{"ready": "unknown"}},
	} {
		if _, e := decodeValues(w); e == nil {
			t.Fatal("inconsistent status/value accepted")
		}
	}
	if _, e := decodeValues(&pb.Observation{ExtractionStatus: map[string]string{"ready": "missing"}}); e != nil {
		t.Fatal(e)
	}
}
