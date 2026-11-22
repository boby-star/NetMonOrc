package httpapi

import (
	"encoding/json"
	"github.com/boby-star/NetMonOrc/internal/resources"
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

func TestSingleManifestRejectsBatchConditionsAndBindsSourceVersion(t *testing.T) {
	source := "apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: example}\nspec: {type: logical, attributes: {ratio: 1.0, counter: 9007199254740993}}\n"
	r := httptest.NewRequest("PUT", "/api/v1/resources/Target/example", strings.NewReader(source))
	r.Header.Set("Content-Type", "application/yaml")
	var m resources.Manifest
	if !resourceManifest(httptest.NewRecorder(), r, &m) {
		t.Fatal("single YAML manifest rejected")
	}
	recorder := httptest.NewRecorder()
	resourceView(recorder, 200, resources.View{Manifest: m, ResourceVersion: "owned:7"})
	if recorder.Header().Get("ETag") != `"owned:7"` {
		t.Fatal("source lost loaded version")
	}
	var response struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response.Source, `9007199254740993`) || !strings.Contains(response.Source, `"ratio": 1.0`) {
		t.Fatal("HTTP source rounded numbers")
	}
	for _, bad := range []string{source + "---\n" + source, `{"manifests":[{"apiVersion":"netmonorc.io/v1alpha1","kind":"Target","metadata":{"name":"example"},"spec":{"type":"logical"}}],"requestId":"do-not-discard"}`} {
		req := httptest.NewRequest("POST", "/api/v1/resources/Target", strings.NewReader(bad))
		req.Header.Set("Content-Type", "application/yaml")
		if resourceManifest(httptest.NewRecorder(), req, &m) {
			t.Fatal("single route discarded batch context")
		}
	}
}
