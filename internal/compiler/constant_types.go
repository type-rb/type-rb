package compiler

import (
	"maps"
	"reflect"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

// Check dependencies before consumers so exported value types come from the
// ordinary checker. Cyclic components additionally converge their checked types.
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

// Signatures and type declarations are already catalogued for the entire unit.
// Inferred exported values reach a fixed point within each cyclic component;
// no provisional Any escapes into lowering or backend execution.
func checkProjectComponents(analyzer *Analyzer, units []SourceUnit, programs map[string]*ast.Program, catalog *resolver.Catalog, resolutions map[string]resolver.Result, options Options, checkedPrograms map[string]checker.Result, diagnostics map[string][]diagnostic.Diagnostic) {
	byModule := map[string]SourceUnit{}
	for _, unit := range units {
		byModule[unit.ModulePath] = unit
	}
	values := map[identity.Declaration]types.Type{}
	for _, component := range resolver.ImportComponents(catalog, resolutions) {
		limit := 1
		if component.Cyclic {
			limit = 2
			for _, module := range component.Modules {
				if program := programs[module]; program != nil {
					limit += countConstants(program.Statements)
				}
			}
		}
		for attempt := 0; attempt < limit; attempt++ {
			previous := maps.Clone(values)
			for _, module := range component.Modules {
				source, program := byModule[module], programs[module]
				if program == nil {
					continue
				}
				resolutions[module] = resolutions[module].WithCheckedValues(values)
				checked, items := analyzer.checkProgram(program, resolutions[module], checker.Options{
					AllowUnusedImports:     options.AllowUnusedImports,
					InteractiveTopLevel:    options.InteractiveModule != "" && options.InteractiveModule == module,
					InteractiveFlowResets:  options.InteractiveFlowResets,
					RunnableMain:           topLevelMethod(program, MainFunction),
					CompilerGeneratedStart: compilerGeneratedStart(source),
				})
				checkedPrograms[module], diagnostics[module] = checked, items
				known := map[identity.Declaration]types.Type{}
				collectCheckedConstants(checked, known)
				for declaration, typ := range known {
					if !unresolvedValueType(typ) {
						values[declaration] = typ
					}
				}
			}
			if reflect.DeepEqual(previous, values) {
				break
			}
		}
		if component.Cyclic {
			for _, module := range component.Modules {
				checked := checkedPrograms[module]
				for variable := range checked.ConstantOwners {
					if variable.Type.Empty() && unresolvedValueType(checked.Variables[variable]) {
						diagnostics[module] = append(diagnostics[module], diagnostic.Diagnostic{Code: diagnostic.TypeError, Severity: diagnostic.Error, Span: variable.Span(), Message: "type inference cycle for constant " + variable.Name + "; add an explicit type annotation"})
					}
				}
			}
		}
	}
}

func unresolvedValueType(typ types.Type) bool {
	if typ.Kind == "" || typ.Kind == types.Any || typ.Kind == types.Invalid {
		return true
	}
	return false
}

func countConstants(statements []ast.Statement) int {
	count := 0
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.VariableStatement:
			if node.Constant {
				count++
			}
		case *ast.ClassStatement:
			count += countConstants(node.Body)
		case *ast.ModuleStatement:
			count += countConstants(node.Body)
		}
	}
	return count
}
