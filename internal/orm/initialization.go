package orm

import "sort"

// Model registration retains its documented directory-group discovery rules.
func (m *Manifest) InitializationDependencies(_ string) []string {
	modules := map[string]bool{}
	for _, model := range m.Models {
		modules[model.ModulePath] = true
	}
	var paths []string
	for module := range modules {
		paths = append(paths, module)
	}
	sort.Strings(paths)
	return paths
}
