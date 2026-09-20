package compiler

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

// Import cycles have already been rejected. Check dependencies before consumers
// so exported value types come from the ordinary checker, not syntax guesses.
func checkedModuleOrder(units []SourceUnit, resolutions map[string]resolver.Result) []SourceUnit {
	byPath := make(map[string]SourceUnit, len(units))
	paths := make([]string, 0, len(units))
	for _, unit := range units {
		byPath[unit.ModulePath] = unit
		paths = append(paths, unit.ModulePath)
	}
	sort.Strings(paths)
	seen := map[string]bool{}
	ordered := make([]SourceUnit, 0, len(units))
	var visit func(string)
	visit = func(module string) {
		if seen[module] {
			return
		}
		seen[module] = true
		var dependencies []string
		for _, imported := range resolutions[module].Imports {
			if imported != nil {
				if _, present := byPath[imported.RuntimePath()]; present {
					dependencies = append(dependencies, imported.RuntimePath())
				}
			}
		}
		sort.Strings(dependencies)
		for _, dependency := range dependencies {
			visit(dependency)
		}
		ordered = append(ordered, byPath[module])
	}
	for _, module := range paths {
		visit(module)
	}
	return ordered
}

func collectCheckedConstants(checked checker.Result, values map[identity.Declaration]types.Type) {
	for variable, owner := range checked.ConstantOwners {
		if typ, found := checked.Variables[variable]; found {
			declaration := identity.Declaration{Module: checked.Program.ModulePath, Name: identity.Qualify(owner, variable.Name), Kind: identity.Value}
			values[declaration] = typ
		}
	}
}

func hasInferredConstants(statements []ast.Statement) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.VariableStatement:
			if node.Constant && node.Type.Empty() {
				return true
			}
		case *ast.ModuleStatement:
			if hasInferredConstants(node.Body) {
				return true
			}
		case *ast.ClassStatement:
			if hasInferredConstants(node.Body) {
				return true
			}
		}
	}
	return false
}
