package golang

import (
	"encoding/hex"
	pathpkg "path"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// goProjectNames resolves the package-level namespace after typed IR exists.
// TypeRB keeps type and callable names distinct, while Go places both in one
// namespace and normalizes snake_case identifiers. Only colliding functions
// receive a compiler-owned fallback, keeping ordinary generated names stable.
type goProjectNames struct {
	functions map[string]map[string]string
}

type goFunctionDeclaration struct {
	modulePath string
	sourceName string
	targetName string
}

func analyzeGoProjectNames(programs []*ir.Program) *goProjectNames {
	result := &goProjectNames{functions: map[string]map[string]string{}}
	occupied := map[string]map[string]bool{}
	functions := map[string]map[string][]goFunctionDeclaration{}

	for _, program := range programs {
		group := goPackageGroup(program.ModulePath)
		if occupied[group] == nil {
			occupied[group] = map[string]bool{}
		}
		if functions[group] == nil {
			functions[group] = map[string][]goFunctionDeclaration{}
		}
		collectGoProjectDeclarations(program.ModulePath, program.Statements, occupied[group], functions[group])
	}

	groups := make([]string, 0, len(functions))
	for group := range functions {
		groups = append(groups, group)
	}
	sort.Strings(groups)
	for _, group := range groups {
		candidates := make([]string, 0, len(functions[group]))
		for candidate := range functions[group] {
			candidates = append(candidates, candidate)
		}
		sort.Strings(candidates)
		for _, candidate := range candidates {
			declarations := functions[group][candidate]
			sort.Slice(declarations, func(i, j int) bool {
				if declarations[i].modulePath != declarations[j].modulePath {
					return declarations[i].modulePath < declarations[j].modulePath
				}
				return declarations[i].sourceName < declarations[j].sourceName
			})
			collision := occupied[group][candidate] || len(declarations) > 1
			for _, declaration := range declarations {
				name := candidate
				if collision {
					name = goFunctionFallback(declaration)
					for occupied[group][name] {
						name += "_"
					}
				}
				occupied[group][name] = true
				if result.functions[declaration.modulePath] == nil {
					result.functions[declaration.modulePath] = map[string]string{}
				}
				result.functions[declaration.modulePath][declaration.sourceName] = name
				result.functions[declaration.modulePath][declaration.targetName] = name
			}
		}
	}
	return result
}

func collectGoProjectDeclarations(modulePath string, statements []ir.Statement, occupied map[string]bool, functions map[string][]goFunctionDeclaration) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			name := goIdentifier(node.Name, true)
			occupied[name] = true
			occupied["New"+name] = true
		case *ir.Record:
			occupied[goIdentifier(node.Name, true)] = true
		case *ir.Enum:
			occupied[goIdentifier(node.Name, true)] = true
		case *ir.TypeAlias:
			occupied[goIdentifier(node.Name, true)] = true
		case *ir.Interface:
			occupied[goIdentifier(node.Name, true)] = true
		case *ir.Variable:
			name := goBindingIdentifier(node.Name)
			if node.Constant {
				name = goConstantIdentifier(node.Owner, node.Name)
			}
			occupied[name] = true
		case *ir.Method:
			target := node.Name
			if node.TargetName != "" {
				target = node.TargetName
			}
			candidate := goMethodName(target)
			if node.Name == "main" {
				candidate = "main"
			}
			functions[candidate] = append(functions[candidate], goFunctionDeclaration{
				modulePath: modulePath,
				sourceName: node.Name,
				targetName: target,
			})
		case *ir.Module:
			collectGoProjectDeclarations(modulePath, node.Body, occupied, functions)
		}
	}
}

func goPackageGroup(modulePath string) string {
	directory := pathpkg.Dir(modulePath)
	if directory == "." {
		return ""
	}
	return directory
}

func goFunctionFallback(declaration goFunctionDeclaration) string {
	prefix := "TrbFunction_"
	if strings.HasPrefix(declaration.sourceName, "_") {
		prefix = "trbFunction_"
	}
	identity := declaration.modulePath + "\x00" + declaration.sourceName + "\x00" + declaration.targetName
	return prefix + hex.EncodeToString([]byte(identity))
}

// analyzeGoBindingNames reserves the package identifiers required by one
// generated file. Go imports live in the file block and can otherwise be
// shadowed by a source-visible local in any generated function. Only actual
// collisions receive a compiler-owned target name, so ordinary output remains
// stable.
func analyzeGoBindingNames(sourceNames map[string]bool, imports map[string]string) map[string]string {
	importNames := map[string]bool{}
	for importPath, alias := range imports {
		if alias == "_" {
			continue
		}
		if alias == "" {
			alias = pathpkg.Base(importPath)
		}
		importNames[goImportAlias(alias)] = true
	}
	if len(importNames) == 0 {
		return nil
	}

	occupied := map[string]bool{}
	for sourceName := range sourceNames {
		occupied[goBindingIdentifier(sourceName)] = true
	}

	ordered := make([]string, 0, len(sourceNames))
	for sourceName := range sourceNames {
		ordered = append(ordered, sourceName)
	}
	sort.Strings(ordered)
	result := map[string]string{}
	for _, sourceName := range ordered {
		if !importNames[goBindingIdentifier(sourceName)] {
			continue
		}
		target := "__trbBinding_" + hex.EncodeToString([]byte(sourceName))
		for occupied[target] || importNames[target] {
			target += "_"
		}
		occupied[target] = true
		result[sourceName] = target
	}
	return result
}
