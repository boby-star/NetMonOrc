package resources

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Document is an exact JSON text representation for editors. Numeric literals
// never pass through a browser's floating-point object representation.
type Document struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Source string `json:"source"`
}

func ManifestDocument(m Manifest) (Document, error) {
	data, err := json.Marshal(m)
	if err != nil {
		return Document{}, err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err = decoder.Decode(&value); err != nil {
		return Document{}, err
	}
	var canonical bytes.Buffer
	encoder := json.NewEncoder(&canonical)
	encoder.SetEscapeHTML(false)
	if err = encoder.Encode(value); err != nil {
		return Document{}, err
	}
	data = bytes.TrimSuffix(canonical.Bytes(), []byte("\n"))
	if len(data) > 1<<20 {
		return Document{}, fmt.Errorf("resource document exceeds 1 MiB")
	}
	var pretty bytes.Buffer
	if err = json.Indent(&pretty, data, "", "  "); err != nil {
		return Document{}, err
	}
	if pretty.Len() <= 1<<20 {
		data = pretty.Bytes()
	}
	return Document{Kind: m.Kind, Name: m.Metadata.Name, Source: string(data)}, nil
}
