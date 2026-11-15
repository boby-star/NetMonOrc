package resources

import (
	"github.com/boby-star/NetMonOrc/internal/dependency"
)

func compileDependency(m Manifest) (dependency.Edge, error) {
	var spec dependency.Spec
	if err := closedJSON(m.Spec, &spec); err != nil {
		return dependency.Edge{}, err
	}
	if err := spec.Normalize(); err != nil {
		return dependency.Edge{}, err
	}
	return dependency.Edge{Name: m.Metadata.Name, Spec: spec}, nil
}
