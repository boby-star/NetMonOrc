package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiagnosticExportNeverClobbers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "report.json")
	if err := writeDiagnosticExport(path, []byte("first")); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("export not private")
	}
	if err := writeDiagnosticExport(path, []byte("second")); err == nil {
		t.Fatal("existing evidence overwritten")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "first" {
		t.Fatal("original evidence changed")
	}
}
