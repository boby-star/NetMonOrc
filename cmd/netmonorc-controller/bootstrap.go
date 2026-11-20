package main

import (
	"fmt"
	"io"
	"os"

	"github.com/boby-star/NetMonOrc/internal/resources"
)

// Bootstrap is a create-once catalog, not an implicit apply on every restart.
func readBootstrap(path, selected string) ([]resources.Manifest, string, error) {
	data, err := readBootstrapFile(path)
	if err != nil {
		return nil, "", err
	}
	batch, err := resources.DecodeBatch(data)
	if err != nil {
		return nil, "", err
	}
	if batch.RequestID != "" || len(batch.ExpectedVersions) > 0 {
		return nil, "", fmt.Errorf("bootstrap forbids apply requestId/expectedVersions; use authenticated CLI apply")
	}
	monitors := map[string]bool{}
	seen := map[string]bool{}
	for _, m := range batch.Manifests {
		key := resources.Key(m)
		if seen[key] {
			return nil, "", fmt.Errorf("duplicate bootstrap resource %s", key)
		}
		seen[key] = true
		if m.Kind == "Monitor" {
			monitors[m.Metadata.Name] = true
		}
	}
	if selected == "" {
		if len(monitors) != 1 {
			return nil, "", fmt.Errorf("bootstrap requires one Monitor or explicit -monitor NAME")
		}
		for name := range monitors {
			selected = name
		}
	}
	if selected == "" || !monitors[selected] {
		return nil, "", fmt.Errorf("selected bootstrap Monitor is absent")
	}
	return batch.Manifests, selected, nil
}

func readBootstrapFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 1<<20 {
		return nil, fmt.Errorf("bootstrap file exceeds 1 MiB")
	}
	return data, nil
}

// Legacy separate files participate in the same catalog transaction.
func appendBootstrapFile(manifests []resources.Manifest, path, kind string) ([]resources.Manifest, error) {
	if path == "" {
		return manifests, nil
	}
	data, err := readBootstrapFile(path)
	if err != nil {
		return nil, err
	}
	batch, err := resources.DecodeBatch(data)
	if err != nil {
		return nil, err
	}
	if len(batch.Manifests) != 1 || batch.RequestID != "" || len(batch.ExpectedVersions) > 0 {
		return nil, fmt.Errorf("legacy bootstrap flag requires one resource without apply metadata")
	}
	m := batch.Manifests[0]
	if m.Kind != kind {
		return nil, fmt.Errorf("bootstrap flag requires %s", kind)
	}
	return append(manifests, m), nil
}
