package languageservice

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

// BuildContext indexes only declarations visible from modulePath. Other
// project modules contribute names through explicit imports.
func BuildContext(programs []*ir.Program, modulePath string) Context {
	context := emptyContext()
	programsByPath := make(map[string]*ir.Program, len(programs))
	exportsByPath := make(map[string][]Symbol, len(programs))
	var session *ir.Program
	for _, program := range programs {
		if program == nil {
			continue
		}
		programsByPath[program.ModulePath] = program
		exportsByPath[program.ModulePath] = collectSymbols(program.Statements, "", context.TypeMembers)
		if program.ModulePath == modulePath {
			session = program
		}
	}
	if session == nil {
		return context
	}

	visible := map[string]Symbol{}
	for _, symbol := range exportsByPath[session.ModulePath] {
		visible[symbol.Name] = symbol
	}
	for _, statement := range session.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Implicit {
			continue
		}
		addImportSymbols(visible, imported, programsByPath, exportsByPath)
	}
	visible["puts"] = Symbol{
		Name:   "puts",
		Kind:   CompletionFunction,
		Detail: "puts(value: Any)",
		Type:   types.FromName("Void"),
		Call:   &CallInfo{ParameterCount: 1},
	}

	context.Symbols = make([]Symbol, 0, len(visible))
	for _, symbol := range visible {
		context.Symbols = append(context.Symbols, symbol)
	}
	sortSymbols(context.Symbols)
	return context
}

func addImportSymbols(visible map[string]Symbol, imported *ir.Import, programsByPath map[string]*ir.Program, exportsByPath map[string][]Symbol) {
	exports := exportsByPath[imported.Path]
	if definition, ok := stdlib.Lookup(imported.Path); ok {
		exports = append(exports, standardSymbols(definition)...)
	}
	if len(exports) == 0 {
		if program := programsByPath[imported.Path]; program != nil {
			exports = collectSymbols(program.Statements, "", map[string][]Symbol{})
		}
	}

	byName := map[string]Symbol{}
	for _, symbol := range exports {
		byName[symbol.Name] = symbol
	}
	for _, name := range imported.Symbols {
		if _, exists := byName[name]; exists {
			continue
		}
		kind := CompletionFunction
		if strings.HasPrefix(name, "_") {
			continue
		}
		if isTypeName(name) {
			kind = CompletionType
		}
		byName[name] = Symbol{Name: name, Kind: kind, Detail: string(kind), Type: inferredNamedType(name, kind)}
	}

	if imported.Namespace && imported.Alias != "" {
		members := make([]Symbol, 0, len(byName))
		if len(imported.Symbols) == 0 {
			for _, symbol := range byName {
				members = append(members, symbol)
			}
		} else {
			for _, name := range imported.Symbols {
				if symbol, exists := byName[name]; exists {
					members = append(members, symbol)
				}
			}
		}
		sortSymbols(members)
		visible[imported.Alias] = Symbol{Name: imported.Alias, Kind: CompletionModule, Detail: imported.Path, Members: members}
		return
	}

	for _, name := range imported.Symbols {
		if symbol, exists := byName[name]; exists {
			visible[name] = symbol
		}
	}
}

func standardSymbols(definition *stdlib.Package) []Symbol {
	if definition == nil {
		return nil
	}
	result := make([]Symbol, 0, len(definition.Symbols)+len(definition.RuntimeExports))
	for _, library := range definition.Symbols {
		result = append(result, Symbol{
			Name:   library.Name,
			Kind:   CompletionFunction,
			Detail: librarySignature(library),
			Type:   library.Return,
			Call:   &CallInfo{ParameterCount: len(library.Parameters)},
		})
	}
	for _, exported := range definition.RuntimeExports {
		kind := CompletionType
		result = append(result, Symbol{Name: exported.Name, Kind: kind, Detail: exported.Kind, Type: types.FromName(exported.Name)})
	}
	sortSymbols(result)
	return result
}

func collectSymbols(statements []ir.Statement, owner string, typeMembers map[string][]Symbol) []Symbol {
	result := []Symbol{}
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Variable:
			if privateName(node.Name) {
				continue
			}
			kind := CompletionVariable
			if node.Constant {
				kind = CompletionConstant
			}
			result = append(result, Symbol{Name: node.Name, Kind: kind, Detail: node.Type.String(), Type: node.Type})
		case *ir.Method:
			if privateName(node.Name) {
				continue
			}
			result = append(result, Symbol{Name: node.Name, Kind: CompletionFunction, Detail: methodSignature(node), Type: node.ReturnType, Call: methodCallInfo(node)})
		case *ir.Class:
			qualified := qualify(owner, node.Name)
			instance, namespace := classMembers(node.Body, qualified, typeMembers)
			typeMembers[qualified] = append(typeMembers[qualified], instance...)
			typeMembers[node.Name] = append(typeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "class " + qualified, Type: types.FromName(qualified), Members: namespace})
		case *ir.Record:
			qualified := qualify(owner, node.Name)
			instance, namespace := recordMembers(node.Body, qualified)
			typeMembers[qualified] = append(typeMembers[qualified], instance...)
			typeMembers[node.Name] = append(typeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "record " + qualified, Type: types.FromName(qualified), Members: namespace})
		case *ir.Enum:
			qualified := qualify(owner, node.Name)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "enum " + qualified, Type: types.FromName(qualified), Members: enumMembers(node.Body)})
		case *ir.TypeAlias:
			qualified := qualify(owner, node.Name)
			members := make([]Symbol, 0, len(node.Variants))
			for _, variant := range node.Variants {
				members = append(members, Symbol{Name: variant.Name, Kind: CompletionConstant, Detail: node.Target.String()})
			}
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "type " + qualified + " = " + node.Target.String(), Type: types.FromName(qualified), Members: members})
		case *ir.Interface:
			qualified := qualify(owner, node.Name)
			methods := make([]Symbol, 0, len(node.Methods))
			for _, method := range node.Methods {
				if !privateName(method.Name) {
					methods = append(methods, Symbol{Name: method.Name, Kind: CompletionMethod, Detail: methodSignature(method), Type: method.ReturnType, Call: methodCallInfo(method)})
				}
			}
			typeMembers[qualified] = methods
			typeMembers[node.Name] = methods
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "interface " + qualified, Type: types.FromName(qualified)})
		case *ir.Module:
			qualified := qualify(owner, node.Name)
			members := collectSymbols(node.Body, qualified, typeMembers)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionModule, Detail: "module " + qualified, Members: members})
		}
	}
	sortSymbols(result)
	return result
}

func classMembers(statements []ir.Statement, owner string, typeMembers map[string][]Symbol) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	namespace := []Symbol{}
	constructor := "new()"
	constructorCall := &CallInfo{}
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Field:
			name := strings.TrimPrefix(node.Name, "@")
			if privateName(name) {
				continue
			}
			instance = append(instance, Symbol{Name: name, Kind: CompletionField, Detail: node.Type.String(), Type: node.Type})
		case *ir.Method:
			if node.Name == "initialize" {
				constructor = constructorSignature(node, owner)
				constructorCall = methodCallInfo(node)
				continue
			}
			if privateName(node.Name) {
				continue
			}
			symbol := Symbol{Name: node.Name, Kind: CompletionMethod, Detail: methodSignature(node), Type: node.ReturnType, Call: methodCallInfo(node)}
			if node.Class {
				namespace = append(namespace, symbol)
			} else {
				instance = append(instance, symbol)
			}
		case *ir.Class, *ir.Record, *ir.Enum, *ir.Module, *ir.Interface:
			namespace = append(namespace, collectSymbols([]ir.Statement{statement}, owner, typeMembers)...)
		}
	}
	namespace = append(namespace, Symbol{Name: "new", Kind: CompletionMethod, Detail: constructor, Type: types.FromName(owner), Call: constructorCall})
	sortSymbols(instance)
	sortSymbols(namespace)
	return instance, namespace
}

func recordMembers(statements []ir.Statement, owner string) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	parameters := []string{}
	for _, statement := range statements {
		field, ok := statement.(*ir.RecordField)
		if !ok || privateName(field.Name) {
			continue
		}
		instance = append(instance, Symbol{Name: field.Name, Kind: CompletionField, Detail: field.Type.String(), Type: field.Type})
		parameters = append(parameters, field.Name+": "+field.Type.String())
	}
	sortSymbols(instance)
	namespace := []Symbol{{Name: "new", Kind: CompletionMethod, Detail: "new(" + strings.Join(parameters, ", ") + "): " + owner, Type: types.FromName(owner), Call: &CallInfo{ParameterCount: len(parameters)}}}
	return instance, namespace
}

func enumMembers(statements []ir.Statement) []Symbol {
	result := []Symbol{}
	for _, statement := range statements {
		member, ok := statement.(*ir.EnumMember)
		if !ok || privateName(member.Name) {
			continue
		}
		parameters := make([]string, 0, len(member.Fields))
		for _, field := range member.Fields {
			parameters = append(parameters, field.Name+": "+field.Type.String())
		}
		detail := member.Name
		if len(parameters) > 0 {
			detail += "(" + strings.Join(parameters, ", ") + ")"
		}
		result = append(result, Symbol{Name: member.Name, Kind: CompletionEnumMember, Detail: detail})
	}
	sortSymbols(result)
	return result
}

func methodSignature(method *ir.Method) string {
	parameters := make([]string, 0, len(method.Parameters))
	for _, parameter := range method.Parameters {
		text := parameter.Name + ": " + parameter.Type.String()
		if parameter.Rest {
			text = "*" + text
		} else if parameter.KeywordRest {
			text = "**" + text
		}
		parameters = append(parameters, text)
	}
	result := method.Name + "(" + strings.Join(parameters, ", ") + ")"
	if method.ReturnType.Kind != types.Void {
		result += ": " + method.ReturnType.String()
	}
	return result
}

func methodCallInfo(method *ir.Method) *CallInfo {
	result := &CallInfo{
		ParameterCount:        len(method.Parameters),
		ExplicitTypeArguments: len(method.TypeParameters) > 0,
	}
	for _, parameter := range method.Parameters {
		result.Parameters = append(result.Parameters, callParameter(parameter))
	}
	for _, alternative := range method.Alternatives {
		signature := CallSignature{}
		for _, parameter := range alternative.Parameters {
			signature.Parameters = append(signature.Parameters, callParameter(parameter))
		}
		result.Alternatives = append(result.Alternatives, signature)
	}
	return result
}

func callParameter(parameter ir.Parameter) CallParameter {
	return CallParameter{
		Name: parameter.Name, Keyword: parameter.Keyword,
		LiteralValues: append([]string(nil), parameter.LiteralValues...),
	}
}

func constructorSignature(method *ir.Method, owner string) string {
	parameters := make([]string, 0, len(method.Parameters))
	for _, parameter := range method.Parameters {
		parameters = append(parameters, parameter.Name+": "+parameter.Type.String())
	}
	return "new(" + strings.Join(parameters, ", ") + "): " + owner
}

func librarySignature(symbol stdlib.Symbol) string {
	parameters := make([]string, 0, len(symbol.Parameters))
	for _, parameter := range symbol.Parameters {
		parameters = append(parameters, parameter.Name+": "+parameter.Type.String())
	}
	result := symbol.Name + "(" + strings.Join(parameters, ", ") + ")"
	if symbol.Return.Kind != types.Void {
		result += ": " + symbol.Return.String()
	}
	return result
}

func sortSymbols(symbols []Symbol) {
	sort.SliceStable(symbols, func(left, right int) bool {
		if symbols[left].Kind != symbols[right].Kind {
			return completionPriority(symbols[left].Kind) < completionPriority(symbols[right].Kind)
		}
		return symbols[left].Name < symbols[right].Name
	})
}

func completionPriority(kind CompletionKind) int {
	switch kind {
	case CompletionVariable, CompletionParameter:
		return 0
	case CompletionField, CompletionMethod, CompletionFunction:
		return 1
	case CompletionConstant, CompletionType, CompletionEnumMember, CompletionModule, CompletionValue:
		return 2
	case CompletionKeyword:
		return 3
	default:
		return 4
	}
}

func qualify(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + "::" + name
}

func privateName(name string) bool {
	return strings.HasPrefix(strings.TrimPrefix(name, "@"), "_")
}

func inferredNamedType(name string, kind CompletionKind) types.Type {
	if kind != CompletionType {
		return types.Type{}
	}
	return types.FromName(name)
}
