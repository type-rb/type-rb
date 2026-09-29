package jobs

import "sort"

func (m *Manifest) InitializationDependencies(module string) []string {
	if module == "trb_test_main" {
		return nil
	}
	modules := map[string]bool{}
	for _, job := range m.Jobs {
		modules[job.ModulePath] = true
	}
	var paths []string
	for module := range modules {
		paths = append(paths, module)
	}
	sort.Strings(paths)
	return paths
}
