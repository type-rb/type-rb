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
}

type rubyFunctionDeclaration struct {
	modulePath string
	sourceName string
	targetName string
}

func analyzeRubyProjectNames(programs []*ir.Program) *rubyProjectNames {
	result := &rubyProjectNames{functions: map[string]map[string]string{}}
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

func rubyFunctionFallback(declaration rubyFunctionDeclaration) string {
	identity := declaration.modulePath + "\x00" + declaration.sourceName + "\x00" + declaration.targetName
	return "__trb_function_" + naming.PrivateSuffix(identity)
}
