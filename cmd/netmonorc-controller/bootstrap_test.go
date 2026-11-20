package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBootstrapMonitorSelectionAndApplySeparation(t *testing.T) {
	original, err := os.ReadFile("../../examples/bootstrap-catalog.yaml")
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "bootstrap.yaml")
	write := func(data []byte) {
		t.Helper()
		if err := os.WriteFile(file, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(original)
	ms, name, err := readBootstrap(file, "")
	if err != nil || name != "shared-http" || len(ms) != 5 {
		t.Fatal(name, len(ms), err)
	}
	other := []byte("\n---\napiVersion: netmonorc.io/v1alpha1\nkind: Monitor\nmetadata: {name: alternate}\nspec: {}\n")
	write(append(append([]byte(nil), original...), other...))
	if _, _, err = readBootstrap(file, ""); err == nil {
		t.Fatal("ambiguous active Monitor follows input order")
	}
	if _, name, err = readBootstrap(file, "alternate"); err != nil || name != "alternate" {
		t.Fatal(name, err)
	}
	if _, _, err = readBootstrap(file, "missing"); err == nil {
		t.Fatal("missing Monitor accepted")
	}
	write([]byte(string(original) + "\n---\n" + string(original)))
	if _, _, err = readBootstrap(file, ""); err == nil {
		t.Fatal("duplicate resources accepted")
	}
	write([]byte(`{"requestId":"apply-id","manifests":[{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{}}]}`))
	if _, _, err = readBootstrap(file, ""); err == nil {
		t.Fatal("apply requestId silently discarded")
	}
	write([]byte(`{"expectedVersions":{"Monitor/service":null},"manifests":[{"apiVersion":"netmonorc.io/v1alpha1","kind":"Monitor","metadata":{"name":"service"},"spec":{}}]}`))
	if _, _, err = readBootstrap(file, ""); err == nil {
		t.Fatal("apply precondition silently discarded")
	}
	for _, bad := range []string{
		strings.Replace(string(original), "apiVersion: netmonorc.io/v1alpha1", "apiVersion: netmonorc.io/invalid", 1),
		strings.Replace(string(original), "kind: Monitor", "kind: Unknown", 1),
	} {
		write([]byte(bad))
		if _, _, err = readBootstrap(file, ""); err == nil {
			t.Fatal("invalid metadata accepted")
		}
	}
	write(original)
	if _, err = appendBootstrapFile(ms, file, "DiagnosticProfile"); err == nil {
		t.Fatal("legacy flag accepted wrong kind/documents")
	}
}
