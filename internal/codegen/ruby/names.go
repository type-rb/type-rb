package ruby

import (
	"sort"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
)

// rubyProjectNames resolves top-level functions that share Ruby's Object
// method namespace. TypeRB modules have separate function namespaces, so only
// declarations that collide after lowering receive compiler-owned names.
type rubyProjectNames struct {
	functions map[string]map[string]string
	constants map[string]map[string]string
}

type rubyFunctionDeclaration struct {
	modulePath string
	sourceName string
	targetName string
}

func analyzeRubyProjectNames(programs []*ir.Program) *rubyProjectNames {
	result := &rubyProjectNames{functions: map[string]map[string]string{}, constants: analyzeRubyConstantNames(programs)}
	occupied := map[string]bool{}
	reserved := map[string]bool{}
	functions := map[string][]rubyFunctionDeclaration{}

	for _, program := range programs {
		for _, statement := range program.Statements {
			method, ok := statement.(*ir.Method)
			if !ok {
				continue
			}
			target := method.TargetName
			if target == "" {
				target = method.Name
			}
			if method.External {
				if result.functions[program.ModulePath] == nil {
					result.functions[program.ModulePath] = map[string]string{}
				}
				result.functions[program.ModulePath][method.Name] = target
				result.functions[program.ModulePath][target] = target
				occupied[target] = true
				reserved[target] = true
				continue
			}
			if method.TargetName == "" && rubyPrivateFunction(method.Name) {
				target = rubyPrivateFunctionName(program.ModulePath, method.Name)
			}
			occupied[target] = true
			functions[target] = append(functions[target], rubyFunctionDeclaration{
				modulePath: program.ModulePath,
				sourceName: method.Name,
				targetName: target,
			})
		}
	}

	candidates := make([]string, 0, len(functions))
	for candidate := range functions {
		candidates = append(candidates, candidate)
	}
	sort.Strings(candidates)
	for _, candidate := range candidates {
		declarations := functions[candidate]
		sort.Slice(declarations, func(i, j int) bool {
			if declarations[i].modulePath != declarations[j].modulePath {
				return declarations[i].modulePath < declarations[j].modulePath
			}
			return declarations[i].sourceName < declarations[j].sourceName
		})
		for _, declaration := range declarations {
			name := candidate
			if len(declarations) > 1 || reserved[candidate] {
				name = rubyFunctionFallback(declaration)
				for occupied[name] {
					name += "_"
				}
			}
			occupied[name] = true
			if result.functions[declaration.modulePath] == nil {
				result.functions[declaration.modulePath] = map[string]string{}
			}
			result.functions[declaration.modulePath][declaration.sourceName] = name
			result.functions[declaration.modulePath][declaration.targetName] = name
		}
	}
	return result
}

// Source files share Ruby's root constant namespace. Retain source identity
// for collisions instead of letting a later require replace an earlier value.
func analyzeRubyConstantNames(programs []*ir.Program) map[string]map[string]string {
	declarations := map[string][]string{}
	occupied := map[string]bool{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			switch node := statement.(type) {
			case *ir.Variable:
				occupied[node.Name] = true
				if node.Constant && node.Owner == "" {
					declarations[node.Name] = append(declarations[node.Name], program.ModulePath)
				}
			case *ir.Class:
				occupied[node.Name] = true
			case *ir.Record:
				occupied[node.Name] = true
			case *ir.Enum:
				occupied[node.Name] = true
			case *ir.Module:
				occupied[node.Name] = true
			case *ir.Newtype:
				occupied[node.Name] = true
			case *ir.TypeAlias:
				occupied[node.Name] = true
			}
		}
	}
	ordered := make([]string, 0, len(declarations))
	for name := range declarations {
		ordered = append(ordered, name)
	}
	sort.Strings(ordered)
	result := map[string]map[string]string{}
	for _, name := range ordered {
		modules := declarations[name]
		if len(modules) < 2 {
			continue
		}
		sort.Strings(modules)
		for _, module := range modules {
			target := "TrbConstant_" + naming.PrivateSuffix(module+"\x00"+name)
			for occupied[target] {
				target += "_"
			}
			occupied[target] = true
			if result[module] == nil {
				result[module] = map[string]string{}
			}
			result[module][name] = target
		}
	}
	return result
}

func rubyFunctionFallback(declaration rubyFunctionDeclaration) string {
	identity := declaration.modulePath + "\x00" + declaration.sourceName + "\x00" + declaration.targetName
	return "__trb_function_" + naming.PrivateSuffix(identity)
}
