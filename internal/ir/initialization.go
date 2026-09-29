package ir

import "sort"

// InitializationDependencies contributes explicitly discovered runtime roots
// without turning every type-checked source into an execution root.
type InitializationDependencies interface {
	InitializationDependencies(module string) []string
}

// InitializationOrder is dependency-first DFS in authored import order.
// The caller supplies ordered roots; shared modules occur exactly once.
func InitializationOrder(programs []*Program, roots []string) []string {
	byModule := map[string]*Program{}
	for _, program := range programs {
		byModule[program.ModulePath] = program
	}
	seen := map[string]bool{}
	var order []string
	var visit func(string)
	visit = func(module string) {
		program := byModule[module]
		if program == nil || seen[module] {
			return
		}
		seen[module] = true
		for _, statement := range program.Statements {
			if imported, ok := statement.(*Import); ok && !DeferredInitializationImport(program, imported) && !imported.Native && (!(imported.Standard || imported.Official) || imported.Runtime && imported.RuntimeRequired) {
				visit(imported.Path)
			}
		}

		order = append(order, module)
	}
	for _, root := range roots {
		visit(root)
		if program := byModule[root]; program != nil {
			var discovered []string
			for _, statement := range program.Statements {
				if imported, ok := statement.(*Import); ok && DeferredInitializationImport(program, imported) {
					discovered = append(discovered, imported.Path)
				}
			}
			for _, extension := range program.Extensions {
				if contributor, ok := extension.(InitializationDependencies); ok {
					discovered = append(discovered, contributor.InitializationDependencies(root)...)
				}
			}
			sort.Strings(discovered)
			for _, module := range discovered {
				visit(module)
			}
		}
	}
	return order
}

// Entry-owned provider glue imports discovered sources for generated functions.
// Those edges must not turn discovery into an authored dependency of the entry.
func DeferredInitializationImport(program *Program, imported *Import) bool {
	return program.CompilationUnit != "" && HasEntrypoint(program) && imported.Implicit && !imported.Native && !imported.Standard && !imported.Official
}

func HasEntrypoint(program *Program) bool {
	for _, statement := range program.Statements {
		if method, ok := statement.(*Method); ok && method.Name == "main" {
			return true
		}
	}
	return false
}

// PlanInitialization records the same plan for each backend without changing
// the cached checked programs or their source-module identities.
func PlanInitialization(programs []*Program) []*Program {
	result := make([]*Program, len(programs))
	for index, program := range programs {
		clone := *program
		if HasEntrypoint(program) {
			clone.InitializationOrder = InitializationOrder(programs, []string{program.ModulePath})
		}
		result[index] = &clone
	}
	active := map[string]bool{}
	for _, program := range result {
		for _, module := range program.InitializationOrder {
			active[module] = true
		}
	}
	if len(active) > 0 {
		for _, program := range result {
			program.RuntimeInactive = !active[program.ModulePath]
		}
	}
	return result
}
