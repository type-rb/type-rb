package languageservice

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

// BuildContext indexes only declarations visible from modulePath. Other
// project modules contribute names through explicit imports.
func BuildContext(programs []*ir.Program, modulePath string) Context {
	index := buildProjectIndex(programs)
	if session := index.programsByPath[modulePath]; session != nil {
		return index.buildContext(session)
	}
	return emptyContext()
}

// BuildContexts indexes project-wide type metadata once, then derives the
// declarations visible from each module through its explicit imports.
func BuildContexts(programs []*ir.Program) map[string]Context {
	index := buildProjectIndex(programs)
	result := make(map[string]Context, len(index.programsByPath))
	for _, session := range index.programsByPath {
		result[session.ModulePath] = index.buildContext(session)
	}
	return result
}

type projectIndex struct {
	metadata       Context
	programsByPath map[string]*ir.Program
	exportsByPath  map[string][]Symbol
}

type projectContextCache struct {
	modulePath string
	index      projectIndex
}

func newProjectContextCache(programs []*ir.Program, modulePath string) *projectContextCache {
	projectPrograms := make([]*ir.Program, 0, len(programs))
	for _, program := range programs {
		if program != nil && program.ModulePath != modulePath {
			projectPrograms = append(projectPrograms, program)
		}
	}
	return &projectContextCache{modulePath: modulePath, index: buildProjectMetadata(projectPrograms)}
}

func (cache *projectContextCache) reusable(programs []*ir.Program, modulePath string) bool {
	if cache == nil || cache.modulePath != modulePath {
		return false
	}
	current := make(map[string]*ir.Program, len(programs))
	for _, program := range programs {
		if program != nil && program.ModulePath != modulePath {
			current[program.ModulePath] = program
		}
	}
	if len(current) != len(cache.index.programsByPath) {
		return false
	}
	for path, program := range current {
		if cache.index.programsByPath[path] != program {
			return false
		}
	}
	return true
}

func (cache *projectContextCache) build(programs []*ir.Program) Context {
	var session *ir.Program
	for _, program := range programs {
		if program != nil && program.ModulePath == cache.modulePath {
			session = program
		}
	}
	if session == nil {
		return emptyContext()
	}
	index := cache.index.withSession(session, programs)
	return index.buildContext(session)
}

func buildProjectIndex(programs []*ir.Program) projectIndex {
	index := buildProjectMetadata(programs)
	indexImplementations(programs, &index.metadata)
	return index
}

func buildProjectMetadata(programs []*ir.Program) projectIndex {
	metadata := emptyContext()
	programsByPath := make(map[string]*ir.Program, len(programs))
	exportsByPath := make(map[string][]Symbol, len(programs))
	for _, program := range programs {
		if program == nil {
			continue
		}
		programsByPath[program.ModulePath] = program
		exportsByPath[program.ModulePath] = collectSymbols(program.Statements, "", program.SourcePath, &metadata)
	}
	for _, program := range programs {
		if program == nil {
			continue
		}
		addDeclarationMembers(&metadata, program.Declarations)
		for _, statement := range program.Statements {
			if imported, ok := statement.(*ir.Import); ok && !imported.Implicit {
				addImportContracts(&metadata, imported)
			}
		}
	}
	return projectIndex{metadata: metadata, programsByPath: programsByPath, exportsByPath: exportsByPath}
}

func (index projectIndex) withSession(session *ir.Program, programs []*ir.Program) projectIndex {
	metadata := cloneProjectMetadata(index.metadata)
	programsByPath := make(map[string]*ir.Program, len(index.programsByPath)+1)
	for path, program := range index.programsByPath {
		programsByPath[path] = program
	}
	programsByPath[session.ModulePath] = session
	exportsByPath := make(map[string][]Symbol, len(index.exportsByPath)+1)
	for path, exports := range index.exportsByPath {
		exportsByPath[path] = exports
	}
	exportsByPath[session.ModulePath] = collectSymbols(session.Statements, "", session.SourcePath, &metadata)
	addDeclarationMembers(&metadata, session.Declarations)
	for _, statement := range session.Statements {
		if imported, ok := statement.(*ir.Import); ok && !imported.Implicit {
			addImportContracts(&metadata, imported)
		}
	}
	indexImplementations(programs, &metadata)
	return projectIndex{metadata: metadata, programsByPath: programsByPath, exportsByPath: exportsByPath}
}

func cloneProjectMetadata(metadata Context) Context {
	result := emptyContext()
	for name, members := range metadata.TypeMembers {
		result.TypeMembers[name] = append([]Symbol(nil), members...)
	}
	for name, info := range metadata.Types {
		result.Types[name] = info
	}
	return result
}

func (index projectIndex) buildContext(session *ir.Program) Context {
	visible := map[string]Symbol{}
	for _, symbol := range index.exportsByPath[session.ModulePath] {
		visible[symbol.Name] = symbol
	}
	for _, statement := range session.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Implicit {
			continue
		}
		addImportSymbols(visible, imported, index.programsByPath, index.exportsByPath)
	}
	addFunctionArgumentReferences(visible, session, index.programsByPath, index.exportsByPath)
	visible["puts"] = Symbol{
		Name:   "puts",
		Kind:   CompletionFunction,
		Detail: "puts(value: Any)",
		Type:   types.FromName("Void"),
		Call:   &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: Any"}}},
	}

	context := Context{
		TypeMembers:     index.metadata.TypeMembers,
		Types:           index.metadata.Types,
		Implementations: index.metadata.Implementations,
		ModulePaths:     map[string]string{},
	}
	for _, statement := range session.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Implicit {
			continue
		}
		target := index.programsByPath[imported.Path]
		if target == nil || target.SourcePath == "" {
			continue
		}
		declaredPath := imported.DeclaredPath
		if declaredPath == "" {
			declaredPath = imported.Path
		}
		context.ModulePaths[declaredPath] = target.SourcePath
	}
	context.Symbols = make([]Symbol, 0, len(visible))
	for _, symbol := range visible {
		context.Symbols = append(context.Symbols, symbol)
	}
	sortSymbols(context.Symbols)
	return context
}

func addFunctionArgumentReferences(visible map[string]Symbol, session *ir.Program, programsByPath map[string]*ir.Program, exportsByPath map[string][]Symbol) {
	if session == nil || session.Declarations == nil {
		return
	}
	for _, rule := range session.Declarations.FunctionArgumentReferenceRules {
		if rule.Owner.ModulePath != session.ModulePath || !importsFunction(session, rule.Package, rule.Function) {
			continue
		}
		symbol, ok := visible[rule.Function]
		if !ok || symbol.Call == nil {
			continue
		}
		parameter := positionalCallParameter(symbol.Call, rule.Argument)
		if parameter == nil {
			continue
		}
		owner, found := declarationOwnerRange(session.Statements, rule.Owner.Name)
		if !found {
			continue
		}
		updated := cloneSymbolCall(symbol)
		parameter = positionalCallParameter(updated.Call, rule.Argument)
		scope := ReferenceScope{Owner: rule.Owner.Name, Range: owner}
		for _, reference := range rule.Targets {
			for _, candidate := range exportsByPath[reference.ModulePath] {
				if candidate.Name != reference.Name || candidate.Kind != CompletionType {
					continue
				}
				candidate.Import = nil
				scope.Symbols = appendUniqueSymbol(scope.Symbols, candidate)
				break
			}
		}
		sortSymbols(scope.Symbols)
		parameter.ReferenceScopes = append(parameter.ReferenceScopes, scope)
		visible[rule.Function] = updated
	}
}

func importsFunction(program *ir.Program, packagePath, function string) bool {
	for _, statement := range program.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Implicit || strings.TrimSuffix(imported.Path, "/index") != strings.TrimSuffix(packagePath, "/index") {
			continue
		}
		for _, symbol := range imported.Symbols {
			if symbol == function {
				return true
			}
		}
	}
	return false
}

func declarationOwnerRange(statements []ir.Statement, name string) (OffsetRange, bool) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			if node.Name == name {
				span := node.SourceSpan()
				return OffsetRange{Start: span.Start.Offset, End: span.End.Offset}, true
			}
			if result, found := declarationOwnerRange(node.Body, name); found {
				return result, true
			}
		case *ir.Module:
			if result, found := declarationOwnerRange(node.Body, name); found {
				return result, true
			}
		}
	}
	return OffsetRange{}, false
}

func positionalCallParameter(call *CallInfo, position int) *CallParameter {
	if call == nil || position < 0 {
		return nil
	}
	positional := 0
	for index := range call.Parameters {
		if call.Parameters[index].Keyword {
			continue
		}
		if positional == position {
			return &call.Parameters[index]
		}
		positional++
	}
	return nil
}

func cloneSymbolCall(symbol Symbol) Symbol {
	result := symbol
	if symbol.Call == nil {
		return result
	}
	call := *symbol.Call
	call.Parameters = append([]CallParameter(nil), symbol.Call.Parameters...)
	for index := range call.Parameters {
		call.Parameters[index].ReferenceScopes = append([]ReferenceScope(nil), call.Parameters[index].ReferenceScopes...)
		for scopeIndex := range call.Parameters[index].ReferenceScopes {
			call.Parameters[index].ReferenceScopes[scopeIndex].Symbols = append([]Symbol(nil), call.Parameters[index].ReferenceScopes[scopeIndex].Symbols...)
		}
	}
	result.Call = &call
	return result
}

type indexedInterface struct {
	definition DefinitionLocation
	methods    map[string]DefinitionLocation
}

type indexedImplementation struct {
	definition DefinitionLocation
	interfaces []types.Type
	methods    map[string]DefinitionLocation
}

func indexImplementations(programs []*ir.Program, context *Context) {
	if context == nil {
		return
	}
	interfaces := map[string][]indexedInterface{}
	implementations := []indexedImplementation{}
	var visit func([]ir.Statement, string, string)
	visit = func(statements []ir.Statement, owner, sourcePath string) {
		if sourcePath == "" {
			return
		}
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ir.Interface:
				qualified := qualify(owner, node.Name)
				item := indexedInterface{
					definition: *sourceDefinition(sourcePath, node.Name, node.SourceSpan()),
					methods:    map[string]DefinitionLocation{},
				}
				for _, method := range node.Methods {
					item.methods[method.Name] = *sourceDefinition(sourcePath, method.Name, method.SourceSpan())
				}
				interfaces[qualified] = append(interfaces[qualified], item)
				if qualified != node.Name {
					interfaces[node.Name] = append(interfaces[node.Name], item)
				}
			case *ir.Class:
				qualified := qualify(owner, node.Name)
				item := indexedImplementation{
					definition: *sourceDefinition(sourcePath, node.Name, node.SourceSpan()),
					interfaces: append([]types.Type(nil), node.Implements...),
					methods:    map[string]DefinitionLocation{},
				}
				for _, member := range node.Body {
					if method, ok := member.(*ir.Method); ok && !method.Class {
						item.methods[method.Name] = *sourceDefinition(sourcePath, method.Name, method.SourceSpan())
					}
				}
				implementations = append(implementations, item)
				visitNestedDeclarations(node.Body, qualified, sourcePath, visit)
			case *ir.Module:
				visit(node.Body, qualify(owner, node.Name), sourcePath)
			}
		}
	}
	for _, program := range programs {
		if program != nil {
			visit(program.Statements, "", program.SourcePath)
		}
	}
	for _, implementation := range implementations {
		for _, implemented := range implementation.interfaces {
			candidates := interfaces[implemented.Name]
			if len(candidates) != 1 {
				continue
			}
			contract := candidates[0]
			appendImplementation(context, contract.definition.ID, implementation.definition)
			for name, declaration := range contract.methods {
				method, ok := implementation.methods[name]
				if ok {
					appendImplementation(context, declaration.ID, method)
				}
			}
		}
	}
	for id := range context.Implementations {
		sort.Slice(context.Implementations[id], func(left, right int) bool {
			items := context.Implementations[id]
			return items[left].Path < items[right].Path || items[left].Path == items[right].Path && items[left].Range.Start < items[right].Range.Start
		})
	}
}

func visitNestedDeclarations(statements []ir.Statement, owner, sourcePath string, visit func([]ir.Statement, string, string)) {
	for _, statement := range statements {
		switch statement.(type) {
		case *ir.Class, *ir.Interface, *ir.Module:
			visit([]ir.Statement{statement}, owner, sourcePath)
		}
	}
}

func appendImplementation(context *Context, id SymbolID, implementation DefinitionLocation) {
	if id == "" || implementation.ID == "" {
		return
	}
	for _, current := range context.Implementations[id] {
		if current.ID == implementation.ID {
			return
		}
	}
	context.Implementations[id] = append(context.Implementations[id], implementation)
}

// ProjectImportCandidates is an immutable project-wide index of declaration
// import origins. Ambiguity is resolved after excluding the requesting module.
type ProjectImportCandidates struct {
	byName map[string][]Symbol
}

// BuildProjectImportCandidates indexes declaration origins once for a complete
// project. Call ForModule to omit declarations owned by the module requesting
// completion and resolve the remaining names.
func BuildProjectImportCandidates(programs []*ir.Program) ProjectImportCandidates {
	byName := map[string][]Symbol{}
	modulePaths := make(map[string]bool, len(programs))
	for _, program := range programs {
		if program != nil {
			modulePaths[program.ModulePath] = true
		}
	}
	for _, program := range programs {
		if program == nil {
			continue
		}
		importPath := projectImportPath(program.ModulePath, modulePaths)
		metadata := emptyContext()
		for _, symbol := range collectSymbols(program.Statements, "", program.SourcePath, &metadata) {
			byName[symbol.Name] = append(byName[symbol.Name], withImportFromModule(symbol, importPath, program.ModulePath))
		}
	}
	return ProjectImportCandidates{byName: byName}
}

// ForModule returns names with exactly one origin after declarations from
// modulePath have been excluded.
func (c ProjectImportCandidates) ForModule(modulePath string) Context {
	result := emptyContext()
	for _, origins := range c.byName {
		var candidate Symbol
		count := 0
		for _, symbol := range origins {
			if symbol.Import != nil && symbol.Import.ModulePath == modulePath {
				continue
			}
			candidate = symbol
			count++
		}
		if count == 1 {
			result.Symbols = append(result.Symbols, candidate)
		}
	}
	sortSymbols(result.Symbols)
	return result
}

// BuildImportCandidates indexes declarations from other project modules. A
// name is offered only when its import path is unambiguous.
func BuildImportCandidates(programs []*ir.Program, modulePath string) Context {
	return BuildProjectImportCandidates(programs).ForModule(modulePath)
}

// StandardImportCandidates returns portable runtime types that can be made
// visible by inserting one explicit import. Duplicate export names are omitted.
func StandardImportCandidates(mode string) Context {
	byName := map[string][]Symbol{}
	for _, definition := range stdlib.RuntimeExportPackages(mode) {
		for _, symbol := range standardSymbols(definition) {
			for _, exported := range definition.RuntimeExports {
				if symbol.Name == exported.Name {
					byName[symbol.Name] = append(byName[symbol.Name], withImport(symbol, definition.Path))
					break
				}
			}
		}
	}
	result := emptyContext()
	for _, origins := range byName {
		if len(origins) == 1 {
			result.Symbols = append(result.Symbols, origins[0])
		}
	}
	sortSymbols(result.Symbols)
	return result
}

// MergeImportCandidateSets keeps only names with one possible source package.
// Editors must not silently choose between a project and standard declaration.
func MergeImportCandidateSets(contexts ...Context) Context {
	byName := map[string][]Symbol{}
	for _, context := range contexts {
		for _, symbol := range context.Symbols {
			byName[symbol.Name] = append(byName[symbol.Name], symbol)
		}
	}
	result := emptyContext()
	for _, origins := range byName {
		if len(origins) == 1 {
			result.Symbols = append(result.Symbols, origins[0])
		}
	}
	sortSymbols(result.Symbols)
	return result
}

// MergeImportCandidates replaces symbols retained from a stale checked
// snapshot when the current source no longer declares or imports that name.
// This lets completion repair a missing import without weakening diagnostics.
func MergeImportCandidates(current, candidates Context, source string) Context {
	visible := sourceVisibleNames(source)
	result := current
	byName := make(map[string]Symbol, len(current.Symbols)+len(candidates.Symbols))
	for _, symbol := range current.Symbols {
		byName[symbol.Name] = symbol
	}
	for _, symbol := range candidates.Symbols {
		if !visible[symbol.Name] {
			byName[symbol.Name] = symbol
		}
	}
	result.Symbols = make([]Symbol, 0, len(byName))
	for _, symbol := range byName {
		result.Symbols = append(result.Symbols, symbol)
	}
	sortSymbols(result.Symbols)
	return result
}

func sourceVisibleNames(source string) map[string]bool {
	visible := map[string]bool{}
	program, _ := parser.Parse([]byte(source))
	for name := range resolver.CollectExports(program.Statements) {
		visible[name] = true
	}
	for _, statement := range program.Statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		for _, name := range imported.Symbols {
			visible[name] = true
		}
		if imported.Alias != "" {
			visible[imported.Alias] = true
		}
	}
	return visible
}

func withImport(symbol Symbol, path string) Symbol {
	return withImportFromModule(symbol, path, path)
}

func withImportFromModule(symbol Symbol, path, modulePath string) Symbol {
	result := symbol
	result.Import = &Import{Path: path, ModulePath: modulePath, Symbol: symbol.Name}
	result.Members = append([]Symbol(nil), symbol.Members...)
	for index := range result.Members {
		result.Members[index] = withImportFromModule(result.Members[index], path, modulePath)
		result.Members[index].Import.Symbol = symbol.Name
	}
	return result
}

func projectImportPath(modulePath string, modulePaths map[string]bool) string {
	return resolver.CanonicalProjectImportPath(modulePath, modulePaths, nil)
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
	valueType := member.Return
	if member.Kind == declaration.Property {
		return Symbol{Name: member.Name, Kind: CompletionField, Detail: displayType(valueType), Type: valueType}
	}
	parameters := make([]string, 0, len(member.Parameters)+1)
	callParameters := make([]CallParameter, len(member.Parameters))
	namedBoundary := false
	for index, parameter := range member.Parameters {
		if parameter.Keyword && !namedBoundary {
			parameters = append(parameters, "*")
			namedBoundary = true
		}
		label := parameter.Name + ": " + displayType(parameter.Type)
		parameters = append(parameters, label)
		callParameters[index] = CallParameter{Name: parameter.Name, Label: label, NamedOnly: parameter.Keyword, Keyword: parameter.Keyword, Optional: parameter.Optional}
	}
	detail := member.Name + "(" + strings.Join(parameters, ", ") + "): " + displayType(valueType)
	return Symbol{
		Name: member.Name, Kind: CompletionMethod, Detail: detail, Type: valueType,
		Call: &CallInfo{ParameterCount: len(member.Parameters), Parameters: callParameters},
	}
}

func addImportContracts(context *Context, imported *ir.Import) {
	if context == nil || imported == nil {
		return
	}
	for name, contract := range imported.TypeContracts {
		context.Types[name] = TypeInfo{
			TypeParameters: append([]string(nil), contract.TypeParameters...),
			AliasTarget:    contract.AliasTarget,
		}
		for memberName, member := range contract.Members {
			if member.Class {
				continue
			}
			context.TypeMembers[name] = appendUniqueSymbol(context.TypeMembers[name], contractMemberSymbol(memberName, member))
		}
		sortSymbols(context.TypeMembers[name])
	}
}

func contractMemberSymbol(name string, member ir.MemberContract) Symbol {
	valueType := member.Type
	kind := CompletionMethod
	if member.Kind == string(resolver.ValueExport) {
		kind = CompletionField
	}
	parameters := make([]string, 0, len(member.Parameters)+1)
	callParameters := make([]CallParameter, len(member.Parameters))
	namedBoundary := false
	for index, parameter := range member.Parameters {
		parameterName := "arg" + strconv.Itoa(index)
		parameterLabel := displayType(parameter.Type)
		keyword := parameter.Kind == callsignature.NamedOnly
		if keyword {
			if !namedBoundary {
				parameters = append(parameters, "*")
				namedBoundary = true
			}
			parameterName = parameter.Label
			parameterLabel = parameter.Label + ": " + parameterLabel
		}
		parameters = append(parameters, parameterLabel)
		callParameters[index] = CallParameter{Name: parameterName, Label: parameterLabel, NamedOnly: keyword, Keyword: keyword, Optional: parameter.Presence == callsignature.Omittable}
	}
	detail := displayType(valueType)
	var call *CallInfo
	if kind == CompletionMethod {
		genericSuffix := ""
		if len(member.TypeParameters) > 0 {
			genericSuffix = "<" + strings.Join(member.TypeParameters, ", ") + ">"
		}
		detail = name + genericSuffix + "(" + strings.Join(parameters, ", ") + "): " + displayType(valueType)
		call = &CallInfo{
			ParameterCount:        len(member.Parameters),
			ExplicitTypeArguments: len(member.TypeParameters) > 0,
			TypeParameters:        append([]string(nil), member.TypeParameters...),
			Parameters:            callParameters,
		}
	}
	return Symbol{Name: name, Kind: kind, Detail: detail, Type: valueType, Call: call}
}

func appendUniqueSymbol(symbols []Symbol, candidate Symbol) []Symbol {
	for index, symbol := range symbols {
		if symbol.Name == candidate.Name {
			symbols[index] = mergeMemberSymbols(symbol, candidate)
			return symbols
		}
	}
	return append(symbols, candidate)
}

func addImportSymbols(visible map[string]Symbol, imported *ir.Import, programsByPath map[string]*ir.Program, exportsByPath map[string][]Symbol) {
	exports := exportsByPath[imported.Path]
	if definition, ok := stdlib.Lookup(imported.Path); ok {
		exports = append(exports, standardSymbols(definition)...)
	}
	if len(exports) == 0 {
		if program := programsByPath[imported.Path]; program != nil {
			metadata := emptyContext()
			exports = collectSymbols(program.Statements, "", program.SourcePath, &metadata)
		}
	}

	byName := map[string]Symbol{}
	for _, symbol := range exports {
		symbol.Members = append([]Symbol(nil), symbol.Members...)
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
		case "class", "record", "enum", "type_alias", "newtype", "enum_alias", "interface":
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
			parts := make([]string, 0, len(parameters)+1)
			callParameters := make([]CallParameter, len(parameters))
			namedBoundary := false
			for index, parameter := range parameters {
				parameterName := "arg" + strconv.Itoa(index)
				parameterLabel := displayType(parameter.Type)
				keyword := parameter.Kind == callsignature.NamedOnly
				if keyword {
					if !namedBoundary {
						parts = append(parts, "*")
						namedBoundary = true
					}
					parameterName = parameter.Label
					parameterLabel = parameter.Label + ": " + parameterLabel
				}
				parts = append(parts, parameterLabel)
				callParameters[index] = CallParameter{Name: parameterName, Label: parameterLabel, NamedOnly: keyword, Keyword: keyword, Optional: parameter.Presence == callsignature.Omittable}
			}
			detail = name + genericSuffix + "(" + strings.Join(parts, ", ") + "): " + displayType(typ)
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
	for name, contract := range imported.TypeContracts {
		symbol, exists := byName[name]
		if !exists {
			continue
		}
		for memberName, member := range contract.Members {
			if member.Class {
				symbol.Members = appendUniqueSymbol(symbol.Members, contractMemberSymbol(memberName, member))
			}
		}
		sortSymbols(symbol.Members)
		byName[name] = symbol
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
		if library.CompilerOnly {
			continue
		}
		parameters := make([]CallParameter, len(library.Parameters))
		for index, parameter := range library.Parameters {
			parameters[index] = CallParameter{
				Name: parameter.Name, Label: parameter.Name + ": " + displayType(parameter.Type),
				NamedOnly: parameter.Keyword, Keyword: parameter.Keyword, Optional: parameter.Optional,
			}
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

func collectSymbols(statements []ir.Statement, owner, sourcePath string, context *Context) []Symbol {
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
			result = append(result, Symbol{Name: node.Name, Kind: kind, Detail: displayType(node.Type), Type: node.Type, Definition: sourceDefinition(sourcePath, node.Name, node.SourceSpan())})
		case *ir.Method:
			if privateName(node.Name) {
				continue
			}
			result = append(result, methodSymbol(node, CompletionFunction, sourcePath))
		case *ir.Class:
			qualified := qualify(owner, node.Name)
			definition := sourceDefinition(sourcePath, node.Name, node.SourceSpan())
			rememberType(context, qualified, node.Name, node.TypeParameters, nil)
			instance, namespace := classMembers(node.Body, qualified, sourcePath, definition, context)
			context.TypeMembers[qualified] = append(context.TypeMembers[qualified], instance...)
			context.TypeMembers[node.Name] = append(context.TypeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "class " + qualified, Type: types.FromName(qualified), Members: namespace, Definition: definition})
		case *ir.Record:
			qualified := qualify(owner, node.Name)
			definition := sourceDefinition(sourcePath, node.Name, node.SourceSpan())
			rememberType(context, qualified, node.Name, node.TypeParameters, nil)
			instance, namespace := recordMembers(node.Body, qualified, sourcePath, definition)
			context.TypeMembers[qualified] = append(context.TypeMembers[qualified], instance...)
			context.TypeMembers[node.Name] = append(context.TypeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "record " + qualified, Type: types.FromName(qualified), Members: namespace, Definition: definition})
		case *ir.Enum:
			qualified := qualify(owner, node.Name)
			definition := sourceDefinition(sourcePath, node.Name, node.SourceSpan())
			rememberType(context, qualified, node.Name, node.TypeParameters, nil)
			instance, namespace := enumMembers(node, qualified, sourcePath, definition)
			context.TypeMembers[qualified] = append(context.TypeMembers[qualified], instance...)
			context.TypeMembers[node.Name] = append(context.TypeMembers[node.Name], instance...)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "enum " + qualified, Type: types.FromName(qualified), Members: namespace, Definition: definition})
		case *ir.TypeAlias:
			qualified := qualify(owner, node.Name)
			target := node.Target
			rememberType(context, qualified, node.Name, node.TypeParameters, &target)
			members := make([]Symbol, 0, len(node.Variants))
			for _, variant := range node.Variants {
				members = append(members, Symbol{Name: variant.Name, Kind: CompletionConstant, Detail: displayType(node.Target), Definition: sourceDefinition(sourcePath, variant.Name, variant.SourceSpan())})
			}
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "alias " + qualified + " = " + displayType(node.Target), Type: types.FromName(qualified), Members: members, Definition: sourceDefinition(sourcePath, node.Name, node.SourceSpan())})
		case *ir.Newtype:
			qualified := qualify(owner, node.Name)
			definition := sourceDefinition(sourcePath, node.Name, node.SourceSpan())
			rememberType(context, qualified, node.Name, nil, nil)
			value := Symbol{
				Name: "value", Kind: CompletionMethod, Detail: "value(): " + displayType(node.Target), Type: node.Target,
				Call: &CallInfo{}, Definition: definition,
			}
			context.TypeMembers[qualified] = []Symbol{value}
			context.TypeMembers[node.Name] = []Symbol{value}
			constructor := Symbol{
				Name: "new", Kind: CompletionMethod, Detail: "new(value: " + displayType(node.Target) + "): " + qualified,
				Type: types.FromName(qualified), Call: &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: " + displayType(node.Target)}}}, Definition: definition,
			}
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "newtype " + qualified + " = " + displayType(node.Target), Type: types.FromName(qualified), Members: []Symbol{constructor}, Definition: definition})
		case *ir.Interface:
			qualified := qualify(owner, node.Name)
			rememberType(context, qualified, node.Name, node.TypeParameters, nil)
			displayName := qualified
			if len(node.TypeParameters) > 0 {
				displayName += "<" + strings.Join(node.TypeParameters, ", ") + ">"
			}
			methods := make([]Symbol, 0, len(node.Methods))
			for _, method := range node.Methods {
				if !privateName(method.Name) {
					methods = append(methods, methodSymbol(method, CompletionMethod, sourcePath))
				}
			}
			context.TypeMembers[qualified] = methods
			context.TypeMembers[node.Name] = methods
			result = append(result, Symbol{Name: node.Name, Kind: CompletionType, Detail: "interface " + displayName, Type: types.FromName(qualified), Definition: sourceDefinition(sourcePath, node.Name, node.SourceSpan())})
		case *ir.Module:
			qualified := qualify(owner, node.Name)
			members := collectSymbols(node.Body, qualified, sourcePath, context)
			result = append(result, Symbol{Name: node.Name, Kind: CompletionModule, Detail: "module " + qualified, Members: members, Definition: sourceDefinition(sourcePath, node.Name, node.SourceSpan())})
		}
	}
	sortSymbols(result)
	return result
}

func rememberType(context *Context, qualified, name string, parameters []string, alias *types.Type) {
	if context == nil {
		return
	}
	info := TypeInfo{TypeParameters: append([]string(nil), parameters...)}
	if alias != nil {
		copy := *alias
		info.AliasTarget = &copy
	}
	context.Types[qualified] = info
	context.Types[name] = info
}

func classMembers(statements []ir.Statement, owner, sourcePath string, ownerDefinition *DefinitionLocation, context *Context) ([]Symbol, []Symbol) {
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
			instance = append(instance, Symbol{Name: name, Kind: CompletionField, Detail: displayType(node.Type), Type: node.Type, Definition: sourceDefinition(sourcePath, name, node.SourceSpan())})
		case *ir.Method:
			if node.Name == "initialize" {
				constructor = constructorSignature(node, owner)
				constructorCall = methodCallInfo(node)
				continue
			}
			if privateName(node.Name) {
				continue
			}
			symbol := methodSymbol(node, CompletionMethod, sourcePath)
			if node.Class {
				namespace = append(namespace, symbol)
			} else {
				instance = append(instance, symbol)
			}
		case *ir.Class, *ir.Record, *ir.Enum, *ir.Module, *ir.Interface:
			namespace = append(namespace, collectSymbols([]ir.Statement{statement}, owner, sourcePath, context)...)
		}
	}
	constructorDefinition := ownerDefinition
	for _, statement := range statements {
		if method, ok := statement.(*ir.Method); ok && method.Name == "initialize" {
			constructorDefinition = sourceDefinition(sourcePath, method.Name, method.SourceSpan())
			break
		}
	}
	namespace = append(namespace, Symbol{Name: "new", Kind: CompletionMethod, Detail: constructor, Type: types.FromName(owner), Call: constructorCall, Definition: constructorDefinition})
	sortSymbols(instance)
	sortSymbols(namespace)
	return instance, namespace
}

func methodSymbol(method *ir.Method, kind CompletionKind, sourcePath string) Symbol {
	symbol := Symbol{Name: method.Name, Kind: kind, Detail: methodSignature(method), Type: methodValueType(method), Call: methodCallInfo(method), Definition: sourceDefinition(sourcePath, method.Name, method.SourceSpan())}
	if method.Property {
		symbol.Kind = CompletionField
		symbol.Call = nil
	}
	if method.Loadable {
		symbol.Members = loadablePropertyMembers(symbol.Type, symbol.Definition)
	}
	return symbol
}

func loadablePropertyMembers(valueType types.Type, definition *DefinitionLocation) []Symbol {
	load := func(name string) Symbol {
		return Symbol{
			Name: name, Kind: CompletionMethod, Detail: name + "(): " + displayType(valueType),
			Type: valueType, Call: &CallInfo{}, Definition: definition,
		}
	}
	return []Symbol{
		load("load"),
		{Name: "loaded?", Kind: CompletionMethod, Detail: "loaded?(): Boolean", Type: types.FromName("Boolean"), Call: &CallInfo{}, Definition: definition},
		load("reload"),
	}
}

func recordMembers(statements []ir.Statement, owner, sourcePath string, ownerDefinition *DefinitionLocation) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	parameters := []string{}
	callParameters := []CallParameter{}
	for _, statement := range statements {
		field, ok := statement.(*ir.RecordField)
		if !ok || privateName(field.Name) {
			continue
		}
		definition := sourceDefinition(sourcePath, field.Name, field.SourceSpan())
		instance = append(instance, Symbol{Name: field.Name, Kind: CompletionField, Detail: displayType(field.Type), Type: field.Type, Definition: definition})
		label := field.Name + ": " + displayType(field.Type)
		parameters = append(parameters, label)
		callParameters = append(callParameters, CallParameter{Name: field.Name, Label: label, Keyword: true, Definition: definition})
	}
	sortSymbols(instance)
	namespace := []Symbol{{Name: "new", Kind: CompletionMethod, Detail: "new(" + strings.Join(parameters, ", ") + "): " + owner, Type: types.FromName(owner), Call: &CallInfo{ParameterCount: len(parameters), Parameters: callParameters}, Definition: ownerDefinition}}
	return instance, namespace
}

func enumMembers(enum *ir.Enum, owner, sourcePath string, ownerDefinition *DefinitionLocation) ([]Symbol, []Symbol) {
	instance := []Symbol{}
	namespace := []Symbol{}
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.EnumMember:
			if privateName(member.Name) {
				continue
			}
			parameters := make([]string, 0, len(member.Fields)+1)
			callParameters := make([]CallParameter, 0, len(member.Fields))
			namedBoundary := false
			for _, field := range member.Fields {
				if field.NamedOnly && !namedBoundary {
					parameters = append(parameters, "*")
					namedBoundary = true
				}
				parameters = append(parameters, field.Name+": "+displayType(field.Type))
				callParameters = append(callParameters, callParameter(field))
			}
			detail := member.Name
			if len(parameters) > 0 {
				detail += "(" + strings.Join(parameters, ", ") + ")"
			}
			var call *CallInfo
			if len(member.Fields) > 0 {
				call = &CallInfo{ParameterCount: len(member.Fields), Parameters: callParameters}
			}
			namespace = append(namespace, Symbol{Name: member.Name, Kind: CompletionEnumMember, Detail: detail, Type: types.FromName(owner), Call: call, Definition: sourceDefinition(sourcePath, member.Name, member.SourceSpan())})
		case *ir.Method:
			if !privateName(member.Name) {
				instance = append(instance, methodSymbol(member, CompletionMethod, sourcePath))
			}
		}
	}
	if enum.RawType.Kind != "" {
		instance = append(instance, Symbol{Name: "raw_value", Kind: CompletionMethod, Detail: "raw_value(): " + displayType(enum.RawType), Type: enum.RawType, Call: &CallInfo{}, Definition: ownerDefinition})
		resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName(owner), types.FromName("EnumValueError")}}
		namespace = append(namespace, Symbol{Name: "from_raw", Kind: CompletionMethod, Detail: "from_raw(value: " + displayType(enum.RawType) + "): " + displayType(resultType), Type: resultType, Call: &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: " + displayType(enum.RawType)}}}, Definition: ownerDefinition})
	}
	sortSymbols(instance)
	sortSymbols(namespace)
	return instance, namespace
}

func methodSignature(method *ir.Method) string {
	parameters := make([]string, 0, len(method.Parameters)+1)
	namedBoundary := false
	for _, parameter := range method.Parameters {
		if parameter.NamedOnly && !namedBoundary {
			parameters = append(parameters, "*")
			namedBoundary = true
		}
		text := parameter.Name + ": " + displayType(parameter.Type)
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
		result += ": " + displayType(valueType)
	}
	return result
}

func methodValueType(method *ir.Method) types.Type {
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
	label := parameter.Name + ": " + displayType(parameter.Type)
	if parameter.Rest {
		label = "*" + label
	} else if parameter.KeywordRest {
		label = "**" + label
	}
	return CallParameter{
		Name: parameter.Name, Label: label, NamedOnly: parameter.NamedOnly, Keyword: parameter.Keyword || parameter.NamedOnly,
		Optional:             parameter.Default != nil,
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
	parameters := make([]string, 0, len(method.Parameters)+1)
	namedBoundary := false
	for _, parameter := range method.Parameters {
		if parameter.NamedOnly && !namedBoundary {
			parameters = append(parameters, "*")
			namedBoundary = true
		}
		parameters = append(parameters, parameter.Name+": "+displayType(parameter.Type))
	}
	return "new(" + strings.Join(parameters, ", ") + "): " + owner
}

func librarySignature(symbol stdlib.Symbol) string {
	parameters := make([]string, 0, len(symbol.Parameters)+1)
	namedBoundary := false
	for _, parameter := range symbol.Parameters {
		if parameter.Keyword && !namedBoundary {
			parameters = append(parameters, "*")
			namedBoundary = true
		}
		parameters = append(parameters, parameter.Name+": "+displayType(parameter.Type))
	}
	result := symbol.Name + "(" + strings.Join(parameters, ", ") + ")"
	valueType := symbol.Return
	if valueType.Kind != types.Void {
		result += ": " + displayType(valueType)
	}
	return result
}

func displayType(input types.Type) string {
	return input.String()
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

func sourceDefinition(path, name string, span token.Span) *DefinitionLocation {
	if path == "" {
		return nil
	}
	return &DefinitionLocation{
		ID:   SymbolID(path + "#" + strconv.Itoa(span.Start.Offset) + ":" + strconv.Itoa(span.End.Offset)),
		Name: name,
		Path: path,
		Range: OffsetRange{
			Start: span.Start.Offset,
			End:   span.End.Offset,
		},
	}
}
