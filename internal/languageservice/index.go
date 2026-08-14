package languageservice

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/declaration"
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
	addDeclarationMembers(&context, session.Declarations)

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
		Call:   &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: Any"}}},
	}

	context.Symbols = make([]Symbol, 0, len(visible))
	for _, symbol := range visible {
		context.Symbols = append(context.Symbols, symbol)
	}
	sortSymbols(context.Symbols)
	return context
}

func addDeclarationMembers(context *Context, catalog *declaration.Catalog) {
	if context == nil || catalog == nil {
		return
	}
	typeNames := make([]string, 0, len(catalog.Types))
	for name := range catalog.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, typeName := range typeNames {
		declared := catalog.Types[typeName]
		existing := map[string]bool{}
		for _, member := range context.TypeMembers[typeName] {
			existing[member.Name] = true
		}
		memberNames := make([]string, 0, len(declared.InstanceMembers))
		for name := range declared.InstanceMembers {
			memberNames = append(memberNames, name)
		}
		sort.Strings(memberNames)
		for _, name := range memberNames {
			member := declared.InstanceMembers[name]
			if privateName(member.Name) || existing[member.Name] {
				continue
			}
			context.TypeMembers[typeName] = append(context.TypeMembers[typeName], declarationSymbol(member))
		}
	}
}

func declarationSymbol(member declaration.Member) Symbol {
	if member.Kind == declaration.Property {
		return Symbol{Name: member.Name, Kind: CompletionField, Detail: member.Return.String(), Type: member.Return}
	}
	parameters := make([]string, len(member.Parameters))
	callParameters := make([]CallParameter, len(member.Parameters))
	for index, parameter := range member.Parameters {
		parameters[index] = parameter.Name + ": " + parameter.Type.String()
		callParameters[index] = CallParameter{Name: parameter.Name, Label: parameters[index], Keyword: parameter.Keyword}
	}
	detail := member.Name + "(" + strings.Join(parameters, ", ") + "): " + member.Return.String()
	if member.Fails.Kind != "" && member.Fails.Kind != types.Never {
		detail += " fails " + member.Fails.String()
	}
	return Symbol{
		Name: member.Name, Kind: CompletionMethod, Detail: detail, Type: member.Return,
		Call: &CallInfo{ParameterCount: len(member.Parameters), Parameters: callParameters},
	}
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
		switch imported.SymbolKinds[name] {
		case "class", "record", "enum", "type_alias", "enum_alias", "interface":
			kind = CompletionType
		}
		typ := imported.SymbolTypes[name]
		if typ.Kind == "" {
			typ = inferredNamedType(name, kind)
		}
		detail := string(kind)
		typeParameters := imported.SymbolTypeParameters[name]
		genericSuffix := ""
		if len(typeParameters) > 0 {
			genericSuffix = "<" + strings.Join(typeParameters, ", ") + ">"
		}
		var call *CallInfo
		if kind == CompletionFunction {
			parameters := imported.SymbolParameters[name]
			parts := make([]string, len(parameters))
			callParameters := make([]CallParameter, len(parameters))
			for index, parameter := range parameters {
				parts[index] = parameter.String()
				callParameters[index] = CallParameter{Name: "arg" + strconv.Itoa(index), Label: parts[index]}
			}
			detail = name + genericSuffix + "(" + strings.Join(parts, ", ") + "): " + typ.String()
			call = &CallInfo{
				ParameterCount:        len(parameters),
				ExplicitTypeArguments: len(typeParameters) > 0,
				TypeParameters:        append([]string(nil), typeParameters...),
				Parameters:            callParameters,
			}
		} else if genericSuffix != "" {
			detail += " " + name + genericSuffix
		}
		byName[name] = Symbol{Name: name, Kind: kind, Detail: detail, Type: typ, Call: call}
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
		parameters := make([]CallParameter, len(library.Parameters))
		for index, parameter := range library.Parameters {
			parameters[index] = CallParameter{Name: parameter.Name, Label: parameter.Name + ": " + parameter.Type.String()}
		}
		result = append(result, Symbol{
			Name:   library.Name,
			Kind:   CompletionFunction,
			Detail: librarySignature(library),
			Type:   library.Return,
			Call:   &CallInfo{ParameterCount: len(library.Parameters), Parameters: parameters},
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
			result = append(result, methodSymbol(node, CompletionFunction))
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
			instance, namespace := enumMembers(node, qualified)
			typeMembers[qualified] = append(typeMembers[qualified], instance...)
			typeMembers[node.Name] = append(typeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "enum " + qualified, Type: types.FromName(qualified), Members: namespace})
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
					methods = append(methods, Symbol{Name: method.Name, Kind: CompletionMethod, Detail: methodSignature(method), Type: methodValueType(method), Call: methodCallInfo(method)})
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
			symbol := methodSymbol(node, CompletionMethod)
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

func methodSymbol(method *ir.Method, kind CompletionKind) Symbol {
	symbol := Symbol{Name: method.Name, Kind: kind, Detail: methodSignature(method), Type: methodValueType(method), Call: methodCallInfo(method)}
	if method.Property {
		symbol.Kind = CompletionField
		symbol.Call = nil
	}
	if method.Loadable {
		symbol.Members = loadablePropertyMembers(symbol.Type, method.Fails)
	}
	return symbol
}

func loadablePropertyMembers(valueType, failureType types.Type) []Symbol {
	load := func(name string) Symbol {
		return Symbol{
			Name: name, Kind: CompletionMethod, Detail: name + "(): " + valueType.String() + " fails " + failureType.String(),
			Type: valueType, Call: &CallInfo{},
		}
	}
	return []Symbol{
		load("load"),
		{Name: "loaded?", Kind: CompletionMethod, Detail: "loaded?(): Boolean", Type: types.FromName("Boolean"), Call: &CallInfo{}},
		load("reload"),
	}
}

func recordMembers(statements []ir.Statement, owner string) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	parameters := []string{}
	callParameters := []CallParameter{}
	for _, statement := range statements {
		field, ok := statement.(*ir.RecordField)
		if !ok || privateName(field.Name) {
			continue
		}
		instance = append(instance, Symbol{Name: field.Name, Kind: CompletionField, Detail: field.Type.String(), Type: field.Type})
		label := field.Name + ": " + field.Type.String()
		parameters = append(parameters, label)
		callParameters = append(callParameters, CallParameter{Name: field.Name, Label: label, Keyword: true})
	}
	sortSymbols(instance)
	namespace := []Symbol{{Name: "new", Kind: CompletionMethod, Detail: "new(" + strings.Join(parameters, ", ") + "): " + owner, Type: types.FromName(owner), Call: &CallInfo{ParameterCount: len(parameters), Parameters: callParameters}}}
	return instance, namespace
}

func enumMembers(enum *ir.Enum, owner string) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	namespace := []Symbol{}
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.EnumMember:
			if privateName(member.Name) {
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
			namespace = append(namespace, Symbol{Name: member.Name, Kind: CompletionEnumMember, Detail: detail})
		case *ir.Method:
			if !privateName(member.Name) {
				instance = append(instance, methodSymbol(member, CompletionMethod))
			}
		}
	}
	if enum.RawType.Kind != "" {
		instance = append(instance, Symbol{Name: "raw_value", Kind: CompletionMethod, Detail: "raw_value(): " + enum.RawType.String(), Type: enum.RawType, Call: &CallInfo{}})
		resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName(owner), types.FromName("EnumValueError")}}
		namespace = append(namespace, Symbol{Name: "from_raw", Kind: CompletionMethod, Detail: "from_raw(value: " + enum.RawType.String() + "): " + resultType.String(), Type: resultType, Call: &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: " + enum.RawType.String()}}}})
	}
	sortSymbols(instance)
	sortSymbols(namespace)
	return instance, namespace
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
	if method.Property {
		result = method.Name
	}
	valueType := methodValueType(method)
	if valueType.Kind != types.Void {
		result += ": " + valueType.String()
	}
	if method.Fails.Kind != "" && method.Fails.Kind != types.Never {
		result += " fails " + method.Fails.String()
	}
	return result
}

func methodValueType(method *ir.Method) types.Type {
	if method.SuccessType.Kind != "" {
		return method.SuccessType
	}
	return method.ReturnType
}

func methodCallInfo(method *ir.Method) *CallInfo {
	result := &CallInfo{
		ParameterCount:        len(method.Parameters),
		ExplicitTypeArguments: len(method.TypeParameters) > 0,
		TypeParameters:        append([]string(nil), method.TypeParameters...),
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
	label := parameter.Name + ": " + parameter.Type.String()
	if parameter.Rest {
		label = "*" + label
	} else if parameter.KeywordRest {
		label = "**" + label
	}
	return CallParameter{
		Name: parameter.Name, Label: label, Keyword: parameter.Keyword,
		LiteralValues:        append([]string(nil), parameter.LiteralValues...),
		LiteralArrays:        copyLiteralArrays(parameter.LiteralArrays),
		LiteralArrayElements: append([]string(nil), parameter.LiteralArrayElements...),
	}
}

func copyLiteralArrays(values [][]string) [][]string {
	result := make([][]string, len(values))
	for index, value := range values {
		result[index] = append([]string(nil), value...)
	}
	return result
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
	if symbol.Fails.Kind != "" && symbol.Fails.Kind != types.Never {
		result += " fails " + symbol.Fails.String()
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
