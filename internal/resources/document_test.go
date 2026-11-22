package resources

import (
	"strings"
	"testing"
)

func TestManifestDocumentPreservesNumericTypesAndEffectiveHash(t *testing.T) {
	m, err := DecodeYAML([]byte("apiVersion: netmonorc.io/v1alpha1\nkind: Target\nmetadata: {name: numeric-target}\nspec: {type: host, address: 127.0.0.1, attributes: {ratio: 1.0, counter: 9007199254740993}}\n"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := ManifestDocument(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(d.Source, `"ratio": 1.0`) || !strings.Contains(d.Source, `9007199254740993`) {
		t.Fatal("document lost numeric spelling")
	}
	decoded, err := DecodeYAML([]byte(d.Source))
	if err != nil {
		t.Fatal(err)
	}
	original, err := (Registry{}).CompileResource(m)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := (Registry{}).CompileResource(decoded)
	if err != nil || restored.Revision != original.Revision {
		t.Fatal("document changed target hash", err)
	}
	again, err := ManifestDocument(decoded)
	if err != nil || again.Source != d.Source {
		t.Fatal("document formatting not stable", err)
	}
}

func TestManifestDocumentDoesNotInflateHTMLTextPastRequestLimit(t *testing.T) {
	m := Manifest{APIVersion: "netmonorc.io/v1alpha1", Kind: "Target", Metadata: Metadata{Name: "html-note", Annotations: map[string]string{"note": strings.Repeat("<", 180000)}}, Spec: []byte(`{"type":"logical"}`)}
	d, err := ManifestDocument(m)
	if err != nil {
		t.Fatal("valid compact resource became unavailable", err)
	}
	if len(d.Source) > 1<<20 || !strings.Contains(d.Source, strings.Repeat("<", 100)) {
		t.Fatal("document cannot be submitted within original limit")
	}
}
