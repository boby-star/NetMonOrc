package resources

import "context"

func (s *Service) MonitorDependencyContext(ctx context.Context, name string) (DependencyContext, error) {
	current, err := s.Repo.Catalog(ctx)
	if err != nil {
		return DependencyContext{}, err
	}
	exists := false
	explicit := map[string]bool{}
	for _, m := range current {
		if m.Kind == "Monitor" && m.Metadata.Name == name {
			exists = true
		}
		if m.Kind == "Dependency" {
			explicit[m.Metadata.Name] = true
		}
	}
	if !exists {
		return DependencyContext{}, ErrNotFound
	}
	_, compiled, err := ResolveCatalog(s.Registry, current, nil)
	if err != nil {
		return DependencyContext{}, err
	}
	p := compiled["Monitor/"+name].Monitor
	out := DependencyContext{Monitor: name, ConfigRevision: p.Revision, Items: []DependencyContextLink{}}
	for _, e := range p.Dependencies {
		stored := explicit[e.Name]
		declaredBy := "Dependency/" + e.Name
		if !stored {
			declaredBy = e.From.Key()
		}
		out.Items = append(out.Items, DependencyContextLink{Name: e.Name, Spec: e.Spec, Derived: !stored, DeclaredBy: declaredBy})
	}
	return out, nil
}
