package httpapi

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestConfigurationSourceSharesStrictYAMLDecoder(t *testing.T) {
	source := "apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: example}\nspec: {type: logical}\n"
	for _, kind := range []string{"application/yaml", "application/x-yaml", "text/yaml"} {
		r := httptest.NewRequest("POST", "/api/v1/config/validate", strings.NewReader(source))
		r.Header.Set("Content-Type", kind)
		if batch, ok := configurationBatch(httptest.NewRecorder(), r); !ok || len(batch.Manifests) != 1 {
			t.Fatal("YAML source rejected", kind)
		}
	}
	for _, bad := range []string{source + "kind: Monitor\n", source + "---\nnull\n", strings.Repeat("x", (1<<20)+1)} {
		r := httptest.NewRequest("POST", "/api/v1/config/prepare", strings.NewReader(bad))
		r.Header.Set("Content-Type", "application/yaml")
		if _, ok := configurationBatch(httptest.NewRecorder(), r); ok {
			t.Fatal("invalid/bounded YAML accepted")
		}
	}
	r := httptest.NewRequest("POST", "/api/v1/config/validate", strings.NewReader(source))
	r.Header.Set("Content-Type", "text/plain")
	if _, ok := configurationBatch(httptest.NewRecorder(), r); ok {
		t.Fatal("unspecified source format accepted")
	}
}
