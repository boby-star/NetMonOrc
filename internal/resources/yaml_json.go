package resources

import (
	"encoding/json"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// YAMLJSON preserves integer/float identity for typed profile parameters and
// bounds alias expansion. It performs no environment or shell substitution.
func YAMLJSON(node *yaml.Node) ([]byte, error) {
	budget := 65536
	active := map[*yaml.Node]bool{}
	var convert func(*yaml.Node, int) (any, error)
	convert = func(n *yaml.Node, depth int) (any, error) {
		budget--
		if n == nil || depth > 64 || budget < 0 || active[n] {
			return nil, fmt.Errorf("YAML nesting, alias cycle or expansion limit")
		}
		active[n] = true
		defer delete(active, n)
		switch n.Kind {
		case yaml.DocumentNode:
			if len(n.Content) != 1 {
				return nil, fmt.Errorf("one YAML value required")
			}
			return convert(n.Content[0], depth+1)
		case yaml.AliasNode:
			return convert(n.Alias, depth+1)
		case yaml.MappingNode:
			out := map[string]any{}
			for i := 0; i < len(n.Content); i += 2 {
				key := n.Content[i]
				if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || key.Value == "<<" {
					return nil, fmt.Errorf("string mapping keys required; YAML merge is forbidden")
				}
				if _, exists := out[key.Value]; exists {
					return nil, fmt.Errorf("duplicate YAML key %s", key.Value)
				}
				value, err := convert(n.Content[i+1], depth+1)
				if err != nil {
					return nil, err
				}
				out[key.Value] = value
			}
			return out, nil
		case yaml.SequenceNode:
			out := make([]any, 0, len(n.Content))
			for _, item := range n.Content {
				v, err := convert(item, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, v)
			}
			return out, nil
		case yaml.ScalarNode:
			switch n.Tag {
			case "!!str":
				return n.Value, nil
			case "!!null":
				return nil, nil
			case "!!bool":
				var v bool
				err := n.Decode(&v)
				return v, err
			case "!!int":
				if len(n.Value) > 4096 {
					return nil, fmt.Errorf("YAML numeric scalar exceeds limit")
				}
				integer, ok := new(big.Int).SetString(strings.ReplaceAll(n.Value, "_", ""), 0)
				if !ok {
					return nil, fmt.Errorf("invalid YAML integer")
				}
				return json.Number(integer.String()), nil
			case "!!float":
				var v float64
				if err := n.Decode(&v); err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
					return nil, fmt.Errorf("finite YAML number required")
				}
				text := strconv.FormatFloat(v, 'g', -1, 64)
				if !strings.ContainsAny(text, ".eE") {
					text += ".0"
				}
				return json.Number(text), nil
			}
		}
		return nil, fmt.Errorf("unsupported YAML node or scalar tag")
	}
	value, err := convert(node, 0)
	if err != nil {
		return nil, err
	}
	return json.Marshal(value)
}
