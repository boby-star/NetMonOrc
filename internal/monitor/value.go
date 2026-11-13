package monitor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Scalar preserves integer precision and zero/false presence. JSON stays a
// normal scalar; floats retain a decimal scale through PostgreSQL jsonb.
type Scalar struct {
	Kind    string
	Text    string
	Integer int64
	Number  float64
	Boolean bool
}

func (v Scalar) Native() (any, error) {
	switch v.Kind {
	case "string":
		return v.Text, nil
	case "integer":
		return v.Integer, nil
	case "boolean":
		return v.Boolean, nil
	case "float":
		if math.IsNaN(v.Number) || math.IsInf(v.Number, 0) {
			return nil, fmt.Errorf("nonfinite extracted value")
		}
		return v.Number, nil
	}
	return nil, fmt.Errorf("invalid scalar type")
}
func (v Scalar) MarshalJSON() ([]byte, error) {
	if v.Kind == "float" {
		if _, e := v.Native(); e != nil {
			return nil, e
		}
		n := v.Number
		if n == 0 {
			n = 0
		}
		s := strconv.FormatFloat(n, 'f', -1, 64)
		if !strings.Contains(s, ".") {
			s += ".0"
		}
		return []byte(s), nil
	}
	n, e := v.Native()
	if e != nil {
		return nil, e
	}
	return json.Marshal(n)
}
func (v *Scalar) UnmarshalJSON(b []byte) error {
	if !json.Valid(b) {
		return fmt.Errorf("invalid JSON scalar")
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return fmt.Errorf("empty scalar")
	}
	var x Scalar
	switch b[0] {
	case '"':
		x.Kind = "string"
		if e := json.Unmarshal(b, &x.Text); e != nil {
			return e
		}
	case 't', 'f':
		x.Kind = "boolean"
		if e := json.Unmarshal(b, &x.Boolean); e != nil {
			return e
		}
	default:
		var number json.Number
		d := json.NewDecoder(bytes.NewReader(b))
		d.UseNumber()
		if e := d.Decode(&number); e != nil {
			return e
		}
		if strings.ContainsAny(number.String(), ".eE") {
			x.Kind = "float"
			n, e := strconv.ParseFloat(number.String(), 64)
			if e != nil || math.IsNaN(n) || math.IsInf(n, 0) {
				return fmt.Errorf("invalid floating scalar")
			}
			if n == 0 {
				n = 0
			}
			x.Number = n
		} else {
			x.Kind = "integer"
			n, e := strconv.ParseInt(number.String(), 10, 64)
			if e != nil {
				return e
			}
			x.Integer = n
		}
	}
	*v = x
	return nil
}

type Extractor struct {
	Name      string
	Pattern   string
	Group     int
	As        string
	OnMissing string
}
