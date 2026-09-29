package web

import "sort"

// File-based routes and middleware are explicitly discovered runtime roots.
func (m *Manifest) InitializationDependencies(_ string) []string {
	modules := map[string]bool{}
	for _, route := range m.Routes {
		modules[route.ModulePath] = true
	}
	for _, middleware := range m.Middlewares {
		modules[middleware.ModulePath] = true
	}
	var paths []string
	for module := range modules {
		paths = append(paths, module)
	}
	sort.Strings(paths)
	return paths
}
