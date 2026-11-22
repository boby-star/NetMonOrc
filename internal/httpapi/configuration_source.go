package httpapi

import (
	"io"
	"mime"
	"net/http"

	"github.com/boby-star/NetMonOrc/internal/resources"
)

func configurationBatch(w http.ResponseWriter, r *http.Request) (resources.Batch, bool) {
	var batch resources.Batch
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || (kind != "application/json" && kind != "application/yaml" && kind != "application/x-yaml" && kind != "text/yaml") {
		failure(w, 400, "INVALID_ARGUMENT", "JSON or YAML configuration required")
		return batch, false
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Configuration exceeds request limit")
		return batch, false
	}
	if kind == "application/json" && r.URL.Path != "/api/v1/config/prepare" {
		err = resources.DecodeJSON(data, &batch)
	} else {
		batch, err = resources.DecodeBatch(data)
	}
	if err != nil {
		failure(w, 400, "INVALID_ARGUMENT", "Invalid YAML/JSON configuration, duplicate key or unknown field")
		return batch, false
	}
	if r.URL.Path == "/api/v1/config/apply" && kind != "application/json" && batch.RequestID == "" {
		failure(w, 400, "INVALID_ARGUMENT", "YAML apply requires a stable requestId; prepare the configuration first")
		return batch, false
	}
	return batch, true
}

// Single-resource CRUD does not silently discard ConfigBatch conditions.
func resourceManifest(w http.ResponseWriter, r *http.Request, out *resources.Manifest) bool {
	kind, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err == nil && kind == "application/json" {
		return resourceJSON(w, r, out)
	}
	batch, ok := configurationBatch(w, r)
	if !ok {
		return false
	}
	if len(batch.Manifests) != 1 || batch.RequestID != "" || len(batch.ExpectedVersions) > 0 {
		failure(w, 400, "INVALID_ARGUMENT", "One manifest without batch conditions required; use config/apply for packages")
		return false
	}
	*out = batch.Manifests[0]
	return true
}
