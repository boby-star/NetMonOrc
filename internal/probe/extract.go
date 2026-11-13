package probe

import (
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/boby-star/NetMonOrc/internal/monitor"
)

func extract(body []byte, spec []monitor.Extractor) (map[string]monitor.Scalar, map[string]string) {
	if len(spec) == 0 {
		return nil, nil
	}
	out := map[string]monitor.Scalar{}
	status := map[string]string{}
	for _, x := range spec {
		status[x.Name] = "success"
		r, e := regexp.Compile(x.Pattern)
		if e != nil {
			status[x.Name] = "error"
			continue
		}
		matches := r.FindSubmatch(body)
		if x.Group < 1 || x.Group >= len(matches) || matches[x.Group] == nil {
			status[x.Name] = "missing"
			if x.OnMissing == "error" {
				status[x.Name] = "error"
			}
			continue
		}
		value := string(matches[x.Group])
		if !utf8.ValidString(value) {
			status[x.Name] = "error"
			continue
		}
		v := monitor.Scalar{Kind: x.As}
		numeric := strings.TrimSpace(value)
		switch x.As {
		case "string":
			v.Text = value
		case "integer":
			v.Integer, e = strconv.ParseInt(numeric, 10, 64)
		case "float":
			v.Number, e = strconv.ParseFloat(numeric, 64)
			if math.IsNaN(v.Number) || math.IsInf(v.Number, 0) {
				status[x.Name] = "error"
				continue
			}
			if v.Number == 0 {
				v.Number = 0
			}
		case "boolean":
			v.Boolean, e = strconv.ParseBool(numeric)
		default:
			status[x.Name] = "error"
			continue
		}
		if e != nil {
			status[x.Name] = "error"
			continue
		}
		out[x.Name] = v
		encoded, e := json.Marshal(out)
		if e != nil || len(encoded) > 4096 {
			delete(out, x.Name)
			status[x.Name] = "error"
		}
	}
	return out, status
}
