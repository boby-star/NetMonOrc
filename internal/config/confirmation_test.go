package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfirmationSchemaAndRevision(t *testing.T) {
	b, e := os.ReadFile("../../deployment/lab/adaptive.yaml")
	if e != nil {
		t.Fatal(e)
	}
	base := strings.TrimSuffix(string(b), "  adaptive: {}\n")
	suffix := `  adaptive:
    stopWhenUnique: false
    confirmation:
      actionRefs: [d04-server-http, d01-office-dns]
      minObservations: 2
      minDistinctFailureDomains: 2
      minSpacing: 0s
`
	p, e := CompileDiagnostics([]byte(base + suffix))
	if e != nil {
		t.Fatal(e)
	}
	if !p.ContinueAfterUnique || p.Confirmation == nil || p.Confirmation.ActionRefs[0] != "d01-office-dns" {
		t.Fatal("confirmation not normalized")
	}
	for _, text := range []string{
		"  adaptive: {stopWhenUnique: false}\n",
		strings.Replace(suffix, "minSpacing: 0s", "minSpacing: ''", 1),
		strings.Replace(suffix, "      minSpacing: 0s\n", "", 1),
		strings.Replace(suffix, "minObservations: 2", "minObservations: 0", 1),
		strings.Replace(suffix, "minDistinctFailureDomains: 2", "minDistinctFailureDomains: 3", 1),
		strings.Replace(suffix, "d04-server-http, d01-office-dns", "d04-server-http, d04-server-http", 1),
		strings.Replace(suffix, "d04-server-http, d01-office-dns", "missing", 1),
		strings.Replace(suffix, "minSpacing: 0s", "minSpacing: 20m", 1),
	} {
		if _, e = CompileDiagnostics([]byte(base + text)); e == nil {
			t.Fatalf("invalid confirmation accepted: %s", text)
		}
	}
	if _, e = CompileDiagnostics([]byte(base + strings.Replace(suffix, "minSpacing: 0s", "minSpacing: 0ms", 1))); e != nil {
		t.Fatal("valid zero duration rejected", e)
	}
}
