package monitor

// ComponentBinding is a Controller-owned snapshot. No installation commands,
// arbitrary module YAML or executable paths travel in an assignment.
type ComponentBinding struct {
	Ref       string
	Hash      string
	Endpoint  string
	ModuleSet string
	Enabled   bool
}
