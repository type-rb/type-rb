// Package initialization verifies the portable initialization contract before
// any backend or interactive evaluator executes source-module values.
package initialization

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/modulegraph"
)

type value struct {
	program     *ir.Program
	variable    *ir.Variable
	step        ir.InitializationStep
	declaration identity.Declaration
}

func Analyze(programs []*ir.Program) ([]*ir.Program, []diagnostic.Diagnostic) {
	result := make([]*ir.Program, len(programs))
	byModule := map[string]*ir.Program{}
	var roots []string
	for index, program := range programs {
		result[index] = program
		byModule[program.ModulePath] = program
		roots = append(roots, program.ModulePath)
	}
	sort.Strings(roots)
	components := ir.InitializationComponents(result, roots)
	anyCycles := false
	for _, component := range components {
		anyCycles = anyCycles || component.Cyclic
	}
	if !anyCycles {
		return result, nil
	}
	for _, component := range components {
		if !component.Cyclic {
			continue
		}
		for _, module := range component.Modules {
			clone := *byModule[module]
			clone.InitializationActions = nil
			clone.CyclicInitialization = true
			byModule[module] = &clone
		}
	}
	for index, program := range result {
		result[index] = byModule[program.ModulePath]
	}
	opaque := effectplan.InitializationSafety(result)
	diagnostics := validateLayouts(result)
	report := func(program *ir.Program, statement ir.Statement, message string) {
		diagnostics = append(diagnostics, diagnostic.Diagnostic{Code: diagnostic.TypeError, Severity: diagnostic.Error, Path: program.SourcePath, Span: statement.SourceSpan(), Message: message})
	}
	for _, component := range components {
		if !component.Cyclic {
			continue
		}
		var values []value
		origins := map[ir.InitializationStep]ir.Statement{}
		preparations := map[identity.Declaration]ir.InitializationStep{}
		classes := map[*ir.Class]*ir.Program{}
		aliases := map[*ir.TypeAlias]*ir.Program{}
		for _, module := range component.Modules {
			program := byModule[module]
			program.CyclicInitialization = true
			previous := ir.InitializationStep{}
			var collect func([]ir.Statement, ir.InitializationStep)
			collect = func(statements []ir.Statement, containing ir.InitializationStep) {
				for _, statement := range statements {
					action := ir.InitializationAction{Name: ir.InitializationActionName(statement)}
					step := ir.InitializationStep{Module: module, Action: action.Name}
					origins[step] = statement
					switch node := statement.(type) {
					case *ir.Variable:
						declaration := node.Declaration
						if declaration.Empty() {
							declaration = identity.Declaration{Module: module, Name: identity.Qualify(node.Owner, node.Name), Kind: identity.Value}
						}
						values = append(values, value{program, node, step, declaration})
						if previous.Module != "" {
							action.Dependencies = append(action.Dependencies, previous)
						}
						previous = step
						program.InitializationActions = append(program.InitializationActions, action)
					case *ir.Class:
						if !node.External {
							classes[node] = program
							if containing.Module != "" {
								action.Dependencies = append(action.Dependencies, containing)
							}
							action.Preparation = true
							preparations[node.Declaration] = step
							program.InitializationActions = append(program.InitializationActions, action)
						}
						collect(node.Body, step)
					case *ir.TypeAlias:
						if len(node.Variants) > 0 {
							aliases[node] = program
							action.Preparation = true
							preparations[node.Declaration] = step
							program.InitializationActions = append(program.InitializationActions, action)
						}
					case *ir.Module:
						collect(node.Body, containing)
					}
				}
			}
			collect(program.Statements, ir.InitializationStep{})
		}
		addDependency := func(program *ir.Program, name string, dependency ir.InitializationStep) {
			for index := range program.InitializationActions {
				action := &program.InitializationActions[index]
				if action.Name == name {
					action.Dependencies = append(action.Dependencies, dependency)
					return
				}
			}
		}
		for class, program := range classes {
			if class.Superclass == nil {
				continue
			}
			parent := ir.ExpressionDeclaration(class.Superclass)
			if dependency, ok := preparations[parent]; ok {
				addDependency(program, ir.InitializationActionName(class), dependency)
			} else {
				known := false
				for _, candidate := range result {
					var find func([]ir.Statement)
					find = func(statements []ir.Statement) {
						for _, statement := range statements {
							switch node := statement.(type) {
							case *ir.Class:
								if node.Declaration == parent && !node.External {
									known = true
								}
								find(node.Body)
							case *ir.Module:
								find(node.Body)
							}
						}
					}
					find(candidate.Statements)
				}
				if !known {
					report(program, class, "class preparation in an import cycle may invoke an unverified native superclass hook")
				}
			}
		}
		for alias, program := range aliases {
			if dependency, ok := preparations[alias.Target.Declaration]; ok {
				addDependency(program, ir.InitializationActionName(alias), dependency)
			}
		}
		for _, target := range values {
			// All authored class declarations are ready before source values execute.
			// Native superclass hooks are rejected above, so preparation cannot move an
			// observable native operation ahead of a source initializer.
			for _, module := range component.Modules {
				for _, action := range byModule[module].InitializationActions {
					if action.Preparation {
						addDependency(target.program, target.step.Action, ir.InitializationStep{Module: module, Action: action.Name})
					}
				}
			}
			if opaque.Expressions[target.variable.Value] {
				report(target.program, target.variable, "initializer in an import cycle reaches an indirect, native, or unverified operation; move the operation into main or use a checked source helper")
			}
		}
		for _, dependency := range values {
			reads := effectplan.Analyze(result, effectplan.Options{Expression: func(expression ir.Expression, module string) bool {
				declaration := valueDeclaration(expression, module)
				return declaration == dependency.declaration
			}})
			for _, target := range values {
				if reads.Expressions[target.variable.Value] {
					addDependency(target.program, target.step.Action, dependency.step)
				}
			}
		}
		// Report only declarations that participate in an actual cycle, with
		// stable source origins even for components outside the runtime roots.
		actions := map[string]ir.InitializationAction{}
		steps := map[string]ir.InitializationStep{}
		var names []string
		key := func(step ir.InitializationStep) string { return step.Module + "#" + step.Action }
		for _, module := range component.Modules {
			for _, action := range byModule[module].InitializationActions {
				step := ir.InitializationStep{Module: module, Action: action.Name}
				name := key(step)
				names = append(names, name)
				steps[name], actions[name] = step, action
			}
		}
		for _, cycle := range modulegraph.Components(names, func(name string) []string {
			var result []string
			for _, dependency := range actions[name].Dependencies {
				result = append(result, key(dependency))
			}
			return result
		}) {
			if !cycle.Cyclic {
				continue
			}
			first := steps[cycle.Modules[0]]
			message := "value initialization cycle"
			if actions[cycle.Modules[0]].Preparation {
				message = "inheritance cycle in class preparation"
			}
			report(byModule[first.Module], origins[first], message+" within source modules "+strings.Join(component.Modules, " -> "))
			item := &diagnostics[len(diagnostics)-1]
			for _, name := range cycle.Modules[1:] {
				step := steps[name]
				item.Related = append(item.Related, diagnostic.RelatedInformation{Message: "participating declaration", Location: diagnostic.Location{Path: byModule[step.Module].SourcePath, Span: origins[step].SourceSpan()}})
			}
		}
	}
	sort.SliceStable(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		return diagnostics[i].Span.Start.Offset < diagnostics[j].Span.Start.Offset
	})
	return result, diagnostics
}

func valueDeclaration(expression ir.Expression, module string) identity.Declaration {
	var reference *ir.Reference
	switch node := expression.(type) {
	case *ir.Identifier:
		reference = node.Reference
	case *ir.Member:
		reference = node.Reference
	}
	if reference != nil && reference.ExportKind == "value" && reference.ClassMember {
		owner := reference.Dispatch.Owner
		if owner.Empty() {
			owner = reference.Declaration
		}
		return identity.Declaration{Module: owner.Module, Name: identity.Qualify(owner.Name, reference.Symbol), Kind: identity.Value}
	}
	declaration := ir.ExpressionDeclaration(expression)
	if declaration.Kind == identity.Value {
		return declaration
	}
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Reference != nil && node.Reference.Declaration.Kind == identity.Value {
			return node.Reference.Declaration
		}
		if !node.Lexical && node.Name != "" && node.Name[0] >= 'A' && node.Name[0] <= 'Z' {
			return identity.Declaration{Module: module, Name: identity.Qualify(node.Owner, node.Name), Kind: identity.Value}
		}
	case *ir.Member:
		if node.Reference != nil && node.Reference.Declaration.Kind == identity.Value {
			return node.Reference.Declaration
		}
	}
	return identity.Declaration{}
}
