package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// DecodeBatch shares the bounded YAML/JSON document parser across CLI and bootstrap.
func DecodeBatch(data []byte) (Batch, error) {
	if len(data) > 1<<20 {
		return Batch{}, fmt.Errorf("configuration exceeds 1 MiB")
	}
	d := yaml.NewDecoder(bytes.NewReader(data))
	batch := Batch{}
	for {
		var document yaml.Node
		err := d.Decode(&document)
		if err == io.EOF {
			break
		}
		if err != nil {
			return batch, fmt.Errorf("invalid YAML/JSON input")
		}
		encoded, err := YAMLJSON(&document)
		if err != nil {
			return batch, fmt.Errorf("JSON-compatible YAML required")
		}
		var envelope map[string]json.RawMessage
		if json.Unmarshal(encoded, &envelope) != nil {
			return batch, fmt.Errorf("resource or ConfigBatch object required")
		}
		if _, isBatch := envelope["manifests"]; isBatch {
			if len(batch.Manifests) > 0 {
				return batch, fmt.Errorf("ConfigBatch cannot be mixed with manifest documents")
			}
			decoder := json.NewDecoder(bytes.NewReader(encoded))
			decoder.DisallowUnknownFields()
			if err = decoder.Decode(&batch); err != nil {
				return batch, fmt.Errorf("invalid ConfigBatch")
			}
			if d.Decode(new(yaml.Node)) != io.EOF {
				return batch, fmt.Errorf("one ConfigBatch document required")
			}
			break
		}
		manifest, err := Decode(encoded)
		if err != nil {
			return batch, err
		}
		batch.Manifests = append(batch.Manifests, manifest)
		if len(batch.Manifests) > 20 {
			return batch, fmt.Errorf("batch exceeds 20 manifests")
		}
	}
	if len(batch.Manifests) < 1 || len(batch.Manifests) > 20 {
		return batch, fmt.Errorf("expected 1..20 manifests")
	}
	for _, m := range batch.Manifests {
		if err := validateMetadata(m); err != nil {
			return batch, fmt.Errorf("%s: %w", Key(m), err)
		}
	}
	return batch, nil
}
