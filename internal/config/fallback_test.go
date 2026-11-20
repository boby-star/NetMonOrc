package config

import (
	"os"
	"strings"
	"testing"
)

func TestFallbackClosedSchemaAndCanonicalDefaults(t *testing.T) {
	b, err := os.ReadFile("../../examples/http.yaml")
	if err != nil {
		t.Fatal(err)
	}
	base := string(b)
	with := func(text string) []byte {
		return []byte(strings.Replace(base, "      config:", "      fallback: "+text+"\n      config:", 1))
	}
	p, err := Compile(with(`{enabled: true, on: [runner_unavailable, local_execution_error]}`))
	if err != nil {
		t.Fatal(err)
	}
	q, err := Compile(with(`{on: [local_execution_error, runner_unavailable], enabled: true, strategy: same-zone, maxAttempts: 1}`))
	if err != nil || q.Revision != p.Revision {
		t.Fatal("fallback normalization changed hash", err)
	}
	old, err := Compile(b)
	disabled, e := Compile(with(`{enabled: false}`))
	if err != nil || e != nil || old.Revision != disabled.Revision {
		t.Fatal("disabled fallback changed default IR")
	}
	for _, bad := range []string{`null`, `{enabled: true}`, `{enabled: false, on: []}`, `{enabled: true, on: [http_503]}`, `{enabled: true, on: [runner_unavailable, runner_unavailable]}`, `{enabled: true, on: [runner_unavailable], maxAttempts: 3}`, `{enabled: true, on: [runner_unavailable], maxAttempts: 1.5}`, `{enabled: true, on: [runner_unavailable], strategy: arbitrary}`, `{enabled: true, on: [runner_unavailable], command: sh}`, `{enabled: "true", on: [runner_unavailable]}`} {
		if _, err = Compile(with(bad)); err == nil {
			t.Fatal("invalid fallback accepted", bad)
		}
	}
}
