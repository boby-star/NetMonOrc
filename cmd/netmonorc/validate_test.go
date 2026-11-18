package main

import (
	"os"
	"strings"
	"testing"
)

func TestLocalValidationUsesCatalogAndNormativeInlineTarget(t *testing.T) {
	source := `apiVersion: netmonorc.io/v1alpha1
kind: Monitor
metadata: {name: simple-http}
spec:
  target: {type: endpoint, address: example.com}
  checks:
    - id: https
      type: http
      config: {url: 'https://example.com/', expectedStatus: [200]}
`
	if err := localValidate([]byte(source), "", ""); err != nil {
		t.Fatal("normative minimal example rejected", err)
	}
	profile, err := os.ReadFile("../../deployment/lab/adaptive.yaml")
	if err != nil {
		t.Fatal(err)
	}
	monitor, err := os.ReadFile("../../deployment/lab/monitor.yaml")
	if err != nil {
		t.Fatal(err)
	}
	data := append(append(profile, []byte("\n---\n")...), monitor...)
	if err := localValidate(data, "branch=office,dc=server", ""); err != nil {
		t.Fatal("offline refs/batch rejected", err)
	}
	if err := localValidate(monitor, "branch=office,dc=server", ""); err == nil {
		t.Fatal("missing external ref accepted")
	}
	if _, err := parseBatch([]byte(strings.Replace(source, "metadata:", "kind: Target\nmetadata:", 1))); err == nil {
		t.Fatal("duplicate YAML key accepted")
	}
}
