package transport

import (
	"encoding/json"
	"fmt"
	"math"

	"github.com/boby-star/NetMonOrc/internal/monitor"
	pb "github.com/boby-star/NetMonOrc/proto/netmonorc/agent/v1"
)

func encodeValues(values map[string]monitor.Scalar) map[string]*pb.ScalarValue {
	if values == nil {
		return nil
	}
	out := map[string]*pb.ScalarValue{}
	for name, v := range values {
		w := &pb.ScalarValue{}
		switch v.Kind {
		case "string":
			w.Value = &pb.ScalarValue_StringValue{StringValue: v.Text}
		case "integer":
			w.Value = &pb.ScalarValue_IntegerValue{IntegerValue: v.Integer}
		case "float":
			w.Value = &pb.ScalarValue_FloatValue{FloatValue: v.Number}
		case "boolean":
			w.Value = &pb.ScalarValue_BooleanValue{BooleanValue: v.Boolean}
		}
		out[name] = w
	}
	return out
}
func decodeValues(w *pb.Observation) (map[string]monitor.Scalar, error) {
	if len(w.Values) > 16 {
		return nil, fmt.Errorf("too many extracted values")
	}
	if len(w.ExtractionStatus) > 16 {
		return nil, fmt.Errorf("too many extraction statuses")
	}
	for name, status := range w.ExtractionStatus {
		if !checkID.MatchString(name) || (status != "success" && status != "missing" && status != "error" && status != "not_applicable") {
			return nil, fmt.Errorf("invalid extraction status")
		}
		if _, exists := w.Values[name]; status == "success" && !exists {
			return nil, fmt.Errorf("successful extraction requires value")
		}
	}
	if w.Values == nil {
		return nil, nil
	}
	out := map[string]monitor.Scalar{}
	for name, value := range w.Values {
		if !checkID.MatchString(name) || value == nil || w.ExtractionStatus[name] != "success" {
			return nil, fmt.Errorf("invalid extracted value")
		}
		var v monitor.Scalar
		switch x := value.Value.(type) {
		case *pb.ScalarValue_StringValue:
			v = monitor.Scalar{Kind: "string", Text: x.StringValue}
		case *pb.ScalarValue_IntegerValue:
			v = monitor.Scalar{Kind: "integer", Integer: x.IntegerValue}
		case *pb.ScalarValue_FloatValue:
			if math.IsNaN(x.FloatValue) || math.IsInf(x.FloatValue, 0) {
				return nil, fmt.Errorf("nonfinite extracted value")
			}
			n := x.FloatValue
			if n == 0 {
				n = 0
			}
			v = monitor.Scalar{Kind: "float", Number: n}
		case *pb.ScalarValue_BooleanValue:
			v = monitor.Scalar{Kind: "boolean", Boolean: x.BooleanValue}
		default:
			return nil, fmt.Errorf("missing scalar variant")
		}
		out[name] = v
	}
	b, e := json.Marshal(out)
	if e != nil || len(b) > 4096 {
		return nil, fmt.Errorf("extracted values exceed payload bound")
	}
	return out, nil
}
