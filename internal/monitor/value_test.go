package monitor

import (
	"encoding/json"
	"testing"
)

func TestScalarJSONPreservesTypes(t *testing.T) {
	for _, v := range []Scalar{{Kind: "integer", Integer: 9007199254740993}, {Kind: "integer", Integer: 0}, {Kind: "float", Number: 1}, {Kind: "float", Number: 1e100}, {Kind: "boolean", Boolean: false}, {Kind: "string", Text: ""}} {
		b, e := json.Marshal(v)
		if e != nil {
			t.Fatal(e)
		}
		var got Scalar
		if e = json.Unmarshal(b, &got); e != nil {
			t.Fatal(e)
		}
		if got != v {
			t.Fatalf("type/value lost: %+v -> %s -> %+v", v, b, got)
		}
	}
}
