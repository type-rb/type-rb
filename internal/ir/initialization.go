package ir

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/modulegraph"
)

type InitializationStep struct{ Module, Action string }
type InitializationAction struct {
	Name         string
	Preparation  bool
	Dependencies []InitializationStep
}

// Action identities use source positions, independently of target renaming.
func InitializationActionName(statement Statement) string {
	prefix := "value"
	switch statement.(type) {
	case *Class, *TypeAlias:
		prefix = "prepare"
	}
	return fmt.Sprintf("%s_%d", prefix, statement.SourceSpan().Start.Offset)
}

func InitializationComponents(programs []*Program, roots []string) []modulegraph.Component {
	byModule := map[string]*Program{}
	for _, program := range programs {
		byModule[program.ModulePath] = program
	}
	dependencies := func(module string) []string {
		var result []string
		program := byModule[module]
		if program == nil {
			return result
		}
		for _, statement := range program.Statements {
			if imported, ok := statement.(*Import); ok && !DeferredInitializationImport(program, imported) && !imported.Native && (!(imported.Standard || imported.Official) || imported.Runtime && imported.RuntimeRequired) && byModule[imported.Path] != nil {
				result = append(result, imported.Path)
			}
		}
		return result
	}
	// Discovery roots follow the explicit root's entire authored closure.
	var ordered []string
	for _, root := range roots {
		program := byModule[root]
		if program == nil {
			continue
		}
		ordered = append(ordered, root)
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
			if byModule[module] != nil {
				ordered = append(ordered, module)
			}
		}
	}
	return modulegraph.Components(ordered, dependencies)
}

func OrderedInitializationSteps(programs []*Program, roots []string) ([]InitializationStep, bool) {
	byModule := map[string]*Program{}
	for _, program := range programs {
		byModule[program.ModulePath] = program
	}
	var steps []InitializationStep
	for _, component := range InitializationComponents(programs, roots) {
		if !component.Cyclic {
			if byModule[component.Modules[0]] != nil {
				steps = append(steps, InitializationStep{Module: component.Modules[0]})
			}
			continue
		}
		actions := map[InitializationStep]InitializationAction{}
		var order []InitializationStep
		for _, preparation := range []bool{true, false} {
			for _, module := range component.Modules {
				if byModule[module] == nil {
					continue
				}
				for _, action := range byModule[module].InitializationActions {
					if action.Preparation == preparation {
						step := InitializationStep{Module: module, Action: action.Name}
						order = append(order, step)
						actions[step] = action
					}
				}
			}
		}
		emitted := map[InitializationStep]bool{}
		for len(emitted) < len(order) {
			ready := false
			for _, step := range order {
				if emitted[step] {
					continue
				}
				blocked := false
				for _, dependency := range actions[step].Dependencies {
					if _, internal := actions[dependency]; internal && !emitted[dependency] {
						blocked = true
						break
					}
				}
				if blocked {
					continue
				}
				emitted[step], ready = true, true
				steps = append(steps, step)
				break
			}
			if !ready {
				return nil, false
			}
		}
	}
	return steps, true
}

// InitializationDependencies contributes explicitly discovered runtime roots
// without turning every type-checked source into an execution root.
type InitializationDependencies interface {
	InitializationDependencies(module string) []string
}

// InitializationOrder is dependency-first DFS in authored import order.
// The caller supplies ordered roots; shared modules occur exactly once.
func InitializationOrder(programs []*Program, roots []string) []string {
	var order []string
	for _, component := range InitializationComponents(programs, roots) {
		order = append(order, component.Modules...)
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
			clone.InitializationSteps, _ = OrderedInitializationSteps(programs, []string{program.ModulePath})
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
