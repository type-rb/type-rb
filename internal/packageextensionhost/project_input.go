package packageextensionhost

import (
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type ProjectDeclarationInputOptions struct {
	PackageAliasesByModule map[string]map[string]string
	// KnownModulePaths resolves imports against modules deliberately excluded
	// from the exported declaration snapshot, without exposing their contents.
	KnownModulePaths []string
}

type projectImportBinding struct {
	importPath string
	modulePath string
}

type projectInputResolver struct {
	programs               map[string]*ast.Program
	aliases                map[string]map[string]*ast.TypeAliasStatement
	newtypes               map[string]map[string]*ast.NewtypeStatement
	definitions            map[string]map[string]bool
	imports                map[string]map[string]projectImportBinding
	knownModules           map[string]bool
	packageAliasesByModule map[string]map[string]string
}

// ExportProjectDeclarationInput copies the declaration-only portion of parsed
// project source into the versioned package-extension boundary. The returned
// value owns its data and does not retain compiler AST nodes.
func ExportProjectDeclarationInput(provider string, programs []*ast.Program, options ProjectDeclarationInputOptions) (packageextension.ProjectDeclarationInput, error) {
	result := packageextension.ProjectDeclarationInput{
		ProtocolVersion: packageextension.ProjectDeclarationInputProtocolVersion,
		Provider:        provider,
	}
	resolver, err := newProjectInputResolver(programs, options)
	if err != nil {
		return result, err
	}
	ordered := append([]*ast.Program(nil), programs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ModulePath < ordered[j].ModulePath })
	for _, program := range ordered {
		module := packageextension.ProjectModule{ModulePath: program.ModulePath}
		resolver.exportStatements(program.ModulePath, "", program.Statements, &module)
		result.Modules = append(result.Modules, module)
	}
	if err := packageextension.ValidateProjectDeclarationInput(result); err != nil {
		return packageextension.ProjectDeclarationInput{}, err
	}
	return result, nil
}

func (r projectInputResolver) exportStatements(modulePath, namespace string, statements []ast.Statement, module *packageextension.ProjectModule) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ModuleStatement:
			r.exportStatements(modulePath, projectQualifiedName(namespace, node.Name), node.Body, module)
		case *ast.ImportStatement:
			module.Imports = append(module.Imports, packageextension.ProjectImport{
				Path: node.Path, ModulePath: r.importModulePath(modulePath, node.Path),
				Symbols: append([]string(nil), node.Symbols...), Alias: node.Alias, Span: exportSourceSpan(node.Span()),
			})
		case *ast.TypeAliasStatement:
			module.TypeAliases = append(module.TypeAliases, packageextension.ProjectTypeAlias{
				Name: projectQualifiedName(namespace, node.Name), TypeParameters: projectTypeParameterNames(node.TypeParameters),
				Target: r.typeUse(modulePath, namespace, node.Target, projectTypeParameterSet(node.TypeParameters)),
				Span:   exportSourceSpan(node.Span()),
			})
		case *ast.NewtypeStatement:
			module.Newtypes = append(module.Newtypes, packageextension.ProjectNewtype{
				Name: projectQualifiedName(namespace, node.Name), Target: r.typeUse(modulePath, namespace, node.Target, nil), Span: exportSourceSpan(node.Span()),
			})
		case *ast.RecordStatement:
			module.Records = append(module.Records, r.exportRecord(modulePath, namespace, node))
		case *ast.ClassStatement:
			module.Classes = append(module.Classes, r.exportClass(modulePath, namespace, node))
		case *ast.EnumStatement:
			module.Enums = append(module.Enums, r.exportEnum(modulePath, namespace, node))
		case *ast.MethodStatement:
			method := r.exportMethod(modulePath, namespace, node, nil)
			method.Name = projectQualifiedName(namespace, method.Name)
			module.Functions = append(module.Functions, method)
		}
	}
}

func newProjectInputResolver(programs []*ast.Program, options ProjectDeclarationInputOptions) (projectInputResolver, error) {
	result := projectInputResolver{
		programs:               map[string]*ast.Program{},
		aliases:                map[string]map[string]*ast.TypeAliasStatement{},
		newtypes:               map[string]map[string]*ast.NewtypeStatement{},
		definitions:            map[string]map[string]bool{},
		imports:                map[string]map[string]projectImportBinding{},
		knownModules:           map[string]bool{},
		packageAliasesByModule: options.PackageAliasesByModule,
	}
	for _, modulePath := range options.KnownModulePaths {
		result.knownModules[modulePath] = true
	}
	for _, program := range programs {
		if program == nil {
			return result, fmt.Errorf("project declaration input contains an empty program")
		}
		if _, exists := result.programs[program.ModulePath]; exists {
			return result, fmt.Errorf("project declaration input contains duplicate module %q", program.ModulePath)
		}
		result.programs[program.ModulePath] = program
		result.knownModules[program.ModulePath] = true
		result.aliases[program.ModulePath] = map[string]*ast.TypeAliasStatement{}
		result.newtypes[program.ModulePath] = map[string]*ast.NewtypeStatement{}
		result.definitions[program.ModulePath] = map[string]bool{}
		result.imports[program.ModulePath] = map[string]projectImportBinding{}
	}
	for _, program := range programs {
		result.collectStatements(program.ModulePath, "", program.Statements)
	}
	return result, nil
}

func (r projectInputResolver) collectStatements(modulePath, namespace string, statements []ast.Statement) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ModuleStatement:
			r.collectStatements(modulePath, projectQualifiedName(namespace, node.Name), node.Body)
		case *ast.ImportStatement:
			for _, symbol := range node.Symbols {
				r.imports[modulePath][symbol] = projectImportBinding{
					importPath: node.Path, modulePath: r.importModulePath(modulePath, node.Path),
				}
			}
		case *ast.TypeAliasStatement:
			name := projectQualifiedName(namespace, node.Name)
			r.aliases[modulePath][name] = node
			r.definitions[modulePath][name] = true
		case *ast.NewtypeStatement:
			name := projectQualifiedName(namespace, node.Name)
			r.newtypes[modulePath][name] = node
			r.definitions[modulePath][name] = true
		case *ast.ClassStatement:
			r.definitions[modulePath][projectQualifiedName(namespace, node.Name)] = true
		case *ast.RecordStatement:
			r.definitions[modulePath][projectQualifiedName(namespace, node.Name)] = true
		case *ast.EnumStatement:
			r.definitions[modulePath][projectQualifiedName(namespace, node.Name)] = true
		case *ast.InterfaceStatement:
			r.definitions[modulePath][projectQualifiedName(namespace, node.Name)] = true
		}
	}
}

func (r projectInputResolver) exportRecord(modulePath, namespace string, record *ast.RecordStatement) packageextension.ProjectRecord {
	result := packageextension.ProjectRecord{
		Name: projectQualifiedName(namespace, record.Name), TypeParameters: projectTypeParameterNames(record.TypeParameters), Span: exportSourceSpan(record.Span()),
	}
	typeParameters := projectTypeParameterSet(record.TypeParameters)
	for _, statement := range record.Body {
		field, ok := statement.(*ast.RecordFieldStatement)
		if !ok {
			continue
		}
		converted := packageextension.ProjectRecordField{
			Name: field.Name, Type: r.typeUse(modulePath, namespace, field.Type, typeParameters), HasDefault: field.Default != nil, Span: exportSourceSpan(field.Span()),
		}
		for _, attribute := range field.Attributes {
			projectAttribute := packageextension.ProjectAttribute{Name: attribute.Name, Span: exportSourceSpan(attribute.Span())}
			for _, argument := range attribute.Arguments {
				projectAttribute.Arguments = append(projectAttribute.Arguments, r.exportProjectArgument(modulePath, namespace, argument))
			}
			converted.Attributes = append(converted.Attributes, projectAttribute)
		}
		result.Fields = append(result.Fields, converted)
	}
	return result
}

func (r projectInputResolver) exportEnum(modulePath, namespace string, enum *ast.EnumStatement) packageextension.ProjectEnum {
	result := packageextension.ProjectEnum{
		Name: projectQualifiedName(namespace, enum.Name), TypeParameters: projectTypeParameterNames(enum.TypeParameters), Span: exportSourceSpan(enum.Span()),
	}
	typeParameters := projectTypeParameterSet(enum.TypeParameters)
	for _, statement := range enum.Body {
		member, ok := statement.(*ast.EnumMemberStatement)
		if !ok {
			continue
		}
		converted := packageextension.ProjectEnumMember{Name: member.Name, Span: exportSourceSpan(member.Span())}
		for _, parameter := range member.Parameters {
			converted.Parameters = append(converted.Parameters, packageextension.ProjectParameter{
				Name: parameter.Name, Type: r.typeUse(modulePath, namespace, parameter.Type, typeParameters),
				NamedOnly: parameter.NamedOnly, Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest,
				Optional: parameter.Default != nil, Span: exportSourceSpan(parameter.Span()),
			})
		}
		if member.RawValue != nil {
			value := r.exportProjectValue(modulePath, namespace, member.RawValue, true)
			converted.RawValue = &value
		}
		for _, attribute := range member.Attributes {
			projectAttribute := packageextension.ProjectAttribute{Name: attribute.Name, Span: exportSourceSpan(attribute.Span())}
			for _, argument := range attribute.Arguments {
				projectAttribute.Arguments = append(projectAttribute.Arguments, r.exportProjectArgument(modulePath, namespace, argument))
			}
			converted.Attributes = append(converted.Attributes, projectAttribute)
		}
		result.Members = append(result.Members, converted)
	}
	return result
}

func (r projectInputResolver) exportClass(modulePath, namespace string, class *ast.ClassStatement) packageextension.ProjectClass {
	className := projectQualifiedName(namespace, class.Name)
	result := packageextension.ProjectClass{
		Name: className, TypeParameters: projectTypeParameterNames(class.TypeParameters), Span: exportSourceSpan(class.Span()),
	}
	classTypeParameters := projectTypeParameterSet(class.TypeParameters)
	if superclass, ok := class.Superclass.(*ast.Identifier); ok {
		ref := ast.TypeRef{Base: superclass.Base, Name: superclass.Name}
		converted := r.typeUse(modulePath, namespace, ref, classTypeParameters)
		result.Superclass = &converted
	}
	for _, statement := range class.Body {
		switch node := statement.(type) {
		case *ast.MethodStatement:
			result.Methods = append(result.Methods, r.exportMethod(modulePath, className, node, classTypeParameters))
		case *ast.ExpressionStatement:
			if directive, ok := r.exportProjectDirective(modulePath, className, node, classTypeParameters); ok {
				result.Directives = append(result.Directives, directive)
			}
		}
	}
	return result
}

func (r projectInputResolver) exportMethod(modulePath, namespace string, node *ast.MethodStatement, ownerTypeParameters map[string]bool) packageextension.ProjectMethod {
	methodTypeParameters := cloneStringSet(ownerTypeParameters)
	for _, parameter := range node.TypeParameters {
		methodTypeParameters[parameter.Name] = true
	}
	result := packageextension.ProjectMethod{
		Name: node.Name, Class: node.Class, TypeParameters: projectTypeParameterNames(node.TypeParameters), Span: exportSourceSpan(node.Span()),
	}
	for _, parameter := range node.Parameters {
		result.Parameters = append(result.Parameters, packageextension.ProjectParameter{
			Name: parameter.Name, Type: r.typeUse(modulePath, namespace, parameter.Type, methodTypeParameters),
			NamedOnly: parameter.NamedOnly, Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest,
			Optional: parameter.Default != nil, Span: exportSourceSpan(parameter.Span()),
		})
	}
	if !node.ReturnType.Empty() {
		converted := r.typeUse(modulePath, namespace, node.ReturnType, methodTypeParameters)
		result.Return = &converted
	}
	return result
}

func (r projectInputResolver) exportProjectDirective(modulePath, namespace string, statement *ast.ExpressionStatement, typeParameters map[string]bool) (packageextension.ProjectDirective, bool) {
	call, ok := statement.Expression.(*ast.CallExpression)
	if !ok {
		return packageextension.ProjectDirective{}, false
	}
	callee := call.Callee
	var typeArguments []ast.TypeRef
	if generic, genericCall := callee.(*ast.GenericExpression); genericCall {
		callee = generic.Receiver
		typeArguments = generic.Arguments
	}
	identifier, ok := callee.(*ast.Identifier)
	if !ok {
		return packageextension.ProjectDirective{}, false
	}
	result := packageextension.ProjectDirective{Name: identifier.Name, Span: exportSourceSpan(call.Span())}
	for _, argument := range typeArguments {
		result.TypeArguments = append(result.TypeArguments, r.typeUse(modulePath, namespace, argument, typeParameters))
	}
	for _, argument := range call.Arguments {
		result.Arguments = append(result.Arguments, r.exportProjectArgument(modulePath, namespace, argument))
	}
	if call.Block != nil {
		result.Block = &packageextension.ProjectDirectiveBlock{
			Parameters: append([]string(nil), call.Block.Parameters...), StatementCount: len(call.Block.Body),
			Span: exportSourceSpan(call.Block.Span()),
		}
		if len(call.Block.Body) == 1 {
			_, result.Block.ResultExpression = call.Block.Body[0].(*ast.ExpressionStatement)
		}
	}
	return result, true
}

func (r projectInputResolver) exportProjectArgument(modulePath, namespace string, argument ast.CallArgument) packageextension.ProjectDirectiveArgument {
	return packageextension.ProjectDirectiveArgument{
		Name: argument.Name, Splat: argument.Splat,
		Value: r.exportProjectValue(modulePath, namespace, argument.Value, false), Span: exportSourceSpan(argument.Value.Span()),
	}
}

func (r projectInputResolver) exportProjectValue(modulePath, namespace string, expression ast.Expression, negativeInteger bool) packageextension.ProjectValue {
	result := exportStandaloneProjectValue(expression, negativeInteger)
	if result.Kind != "reference" {
		return result
	}
	if reference, ok := r.reference(modulePath, namespace, result.Name); ok {
		copy := reference
		result.Reference = &copy
	}
	return result
}

func exportStandaloneProjectValue(expression ast.Expression, negativeInteger bool) packageextension.ProjectValue {
	switch value := expression.(type) {
	case *ast.Literal:
		return packageextension.ProjectValue{Kind: string(value.Kind), Raw: value.Raw}
	case *ast.SymbolLiteral:
		return packageextension.ProjectValue{Kind: "symbol", Raw: value.Raw, Name: value.Name}
	case *ast.Identifier:
		return packageextension.ProjectValue{Kind: "reference", Name: value.Name}
	case *ast.UnaryExpression:
		literal, ok := value.Operand.(*ast.Literal)
		if negativeInteger && value.Operator == "-" && ok && literal.Kind == ast.IntegerLiteral {
			return packageextension.ProjectValue{Kind: "integer", Raw: "-" + literal.Raw}
		}
	}
	return packageextension.ProjectValue{Kind: "unsupported"}
}

func (r projectInputResolver) typeUse(modulePath, namespace string, ref ast.TypeRef, typeParameters map[string]bool) packageextension.ProjectTypeUse {
	authored := exportType(projectInputTypeRef(ref))
	r.attachDefinitions(modulePath, namespace, &authored, typeParameters)
	resolved, path := r.resolveType(modulePath, namespace, authored, map[string]bool{})
	result := packageextension.ProjectTypeUse{
		Authored: authored, Resolved: resolved, ResolutionPath: path, Span: exportSourceSpan(ref.Span()),
	}
	if representation, newtype := r.resolveRepresentation(modulePath, namespace, authored, map[string]bool{}); newtype {
		result.Representation = &representation
	}
	return result
}

func (r projectInputResolver) resolveRepresentation(modulePath, namespace string, typ packageextension.Type, visiting map[string]bool) (packageextension.Type, bool) {
	foundNewtype := false
	for index := range typ.Arguments {
		resolved, found := r.resolveRepresentation(modulePath, namespace, typ.Arguments[index], cloneBoolMap(visiting))
		typ.Arguments[index] = resolved
		foundNewtype = foundNewtype || found
	}
	if typ.Kind != "named" || len(typ.Arguments) != 0 {
		return typ, foundNewtype
	}
	reference, ok := r.reference(modulePath, namespace, typ.Name)
	if !ok {
		return typ, foundNewtype
	}
	key := reference.ModulePath + "\x00" + reference.Name
	if visiting[key] {
		return typ, foundNewtype
	}
	visiting[key] = true
	targetNamespace := projectDeclarationOwner(reference.Name)
	if alias := r.aliases[reference.ModulePath][reference.Name]; alias != nil && len(alias.TypeParameters) == 0 {
		target := exportType(projectInputTypeRef(alias.Target))
		target.Nullable = target.Nullable || typ.Nullable
		r.attachDefinitions(reference.ModulePath, targetNamespace, &target, nil)
		resolved, found := r.resolveRepresentation(reference.ModulePath, targetNamespace, target, visiting)
		return resolved, foundNewtype || found
	}
	if newtype := r.newtypes[reference.ModulePath][reference.Name]; newtype != nil {
		target := exportType(projectInputTypeRef(newtype.Target))
		target.Nullable = target.Nullable || typ.Nullable
		r.attachDefinitions(reference.ModulePath, targetNamespace, &target, nil)
		resolved, _ := r.resolveRepresentation(reference.ModulePath, targetNamespace, target, visiting)
		return resolved, true
	}
	typ.Name = reference.Name
	return typ, foundNewtype
}

func projectInputTypeRef(ref ast.TypeRef) types.Type {
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = projectInputTypeRef(alternative)
		}
		return types.UnionOf(alternatives...)
	}
	if ref.FunctionReturn != nil {
		parameters := make([]types.Type, len(ref.FunctionParameters))
		for index, parameter := range ref.FunctionParameters {
			parameters[index] = projectInputTypeRef(parameter)
		}
		result := types.FunctionOf(parameters, projectInputTypeRef(*ref.FunctionReturn))
		result.Nullable = ref.Nullable
		return result
	}
	result := types.FromName(ref.Name)
	result.Nullable = ref.Nullable
	for _, argument := range ref.Arguments {
		result.Args = append(result.Args, projectInputTypeRef(argument))
	}
	if ref.Array {
		result = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{result}, Nullable: ref.Nullable}
	}
	return result
}

func (r projectInputResolver) attachDefinitions(modulePath, namespace string, typ *packageextension.Type, typeParameters map[string]bool) {
	if typ.Kind == "named" && !typeParameters[typ.Name] {
		if reference, ok := r.reference(modulePath, namespace, typ.Name); ok {
			typ.Definition = &packageextension.Definition{ModulePath: reference.ModulePath, ImportPath: reference.ImportPath}
		}
	}
	for index := range typ.Arguments {
		r.attachDefinitions(modulePath, namespace, &typ.Arguments[index], typeParameters)
	}
}

func (r projectInputResolver) resolveType(modulePath, namespace string, typ packageextension.Type, visiting map[string]bool) (packageextension.Type, []packageextension.ProjectTypeReference) {
	for index := range typ.Arguments {
		resolved, _ := r.resolveType(modulePath, namespace, typ.Arguments[index], cloneBoolMap(visiting))
		typ.Arguments[index] = resolved
	}
	if typ.Kind != "named" {
		return typ, nil
	}
	reference, ok := r.reference(modulePath, namespace, typ.Name)
	if !ok {
		return typ, nil
	}
	path := []packageextension.ProjectTypeReference{reference}
	if typ.Nullable || len(typ.Arguments) != 0 {
		typ.Name = reference.Name
		return typ, path
	}
	alias := r.aliases[reference.ModulePath][reference.Name]
	if alias == nil || len(alias.TypeParameters) != 0 {
		typ.Name = reference.Name
		return typ, path
	}
	key := reference.ModulePath + "\x00" + reference.Name
	if visiting[key] {
		return typ, path
	}
	visiting[key] = true
	targetNamespace := projectDeclarationOwner(reference.Name)
	target := exportType(projectInputTypeRef(alias.Target))
	r.attachDefinitions(reference.ModulePath, targetNamespace, &target, nil)
	resolved, nestedPath := r.resolveType(reference.ModulePath, targetNamespace, target, visiting)
	return resolved, append(path, nestedPath...)
}

func (r projectInputResolver) reference(modulePath, namespace, name string) (packageextension.ProjectTypeReference, bool) {
	for _, candidate := range projectNameCandidates(namespace, name) {
		if r.definitions[modulePath][candidate] {
			return packageextension.ProjectTypeReference{Name: candidate, ModulePath: modulePath}, true
		}
	}
	if imported, ok := r.imports[modulePath][name]; ok {
		return packageextension.ProjectTypeReference{Name: name, ModulePath: imported.modulePath, ImportPath: imported.importPath}, true
	}
	return packageextension.ProjectTypeReference{}, false
}

func (r projectInputResolver) modulePath(importPath string) string {
	if r.knownModules[importPath] {
		return importPath
	}
	if r.knownModules[importPath+"/index"] {
		return importPath + "/index"
	}
	return importPath
}

func (r projectInputResolver) importModulePath(modulePath, importPath string) string {
	return r.modulePath(resolver.CanonicalPackageImport(importPath, r.packageAliasesByModule[modulePath]))
}

func projectQualifiedName(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "::" + name
}

func projectDeclarationOwner(name string) string {
	separator := strings.LastIndex(name, "::")
	if separator < 0 {
		return ""
	}
	return name[:separator]
}

func projectNameCandidates(namespace, name string) []string {
	result := []string{}
	seen := map[string]bool{}
	for current := namespace; ; current = projectDeclarationOwner(current) {
		candidate := projectQualifiedName(current, name)
		if !seen[candidate] {
			result = append(result, candidate)
			seen[candidate] = true
		}
		if current == "" {
			break
		}
	}
	return result
}

func projectTypeParameterNames(parameters []ast.TypeParameter) []string {
	if len(parameters) == 0 {
		return nil
	}
	result := make([]string, len(parameters))
	for index, parameter := range parameters {
		result[index] = parameter.Name
	}
	return result
}

func projectTypeParameterSet(parameters []ast.TypeParameter) map[string]bool {
	result := make(map[string]bool, len(parameters))
	for _, parameter := range parameters {
		result[parameter.Name] = true
	}
	return result
}

func cloneStringSet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneBoolMap(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func exportSourceSpan(span token.Span) packageextension.SourceSpan {
	return ExportSourceSpan(span)
}

// ExportSourceSpan copies a compiler source span into the data-only package
// extension representation.
func ExportSourceSpan(span token.Span) packageextension.SourceSpan {
	return packageextension.SourceSpan{
		Start: packageextension.SourcePosition{Offset: span.Start.Offset, Line: span.Start.Line, Column: span.Start.Column},
		End:   packageextension.SourcePosition{Offset: span.End.Offset, Line: span.End.Line, Column: span.End.Column},
	}
}

// ImportSourceSpan copies a package extension span back into compiler-owned
// source coordinates.
func ImportSourceSpan(span packageextension.SourceSpan) token.Span {
	return token.Span{
		Start: token.Position{Offset: span.Start.Offset, Line: span.Start.Line, Column: span.Start.Column},
		End:   token.Position{Offset: span.End.Offset, Line: span.End.Line, Column: span.End.Column},
	}
}
