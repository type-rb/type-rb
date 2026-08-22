package packageextensionhost

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type ProjectDeclarationInputOptions struct {
	PackageAliasesByModule map[string]map[string]string
}

type projectImportBinding struct {
	importPath string
	modulePath string
}

type projectInputResolver struct {
	programs               map[string]*ast.Program
	aliases                map[string]map[string]*ast.TypeAliasStatement
	definitions            map[string]map[string]bool
	imports                map[string]map[string]projectImportBinding
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
		for _, statement := range program.Statements {
			switch node := statement.(type) {
			case *ast.ImportStatement:
				module.Imports = append(module.Imports, packageextension.ProjectImport{
					Path: node.Path, ModulePath: resolver.importModulePath(program.ModulePath, node.Path),
					Symbols: append([]string(nil), node.Symbols...), Alias: node.Alias, Span: exportSourceSpan(node.Span()),
				})
			case *ast.TypeAliasStatement:
				module.TypeAliases = append(module.TypeAliases, packageextension.ProjectTypeAlias{
					Name: node.Name, TypeParameters: projectTypeParameterNames(node.TypeParameters),
					Target: resolver.typeUse(program.ModulePath, node.Target, projectTypeParameterSet(node.TypeParameters)),
					Span:   exportSourceSpan(node.Span()),
				})
			case *ast.ClassStatement:
				module.Classes = append(module.Classes, resolver.exportClass(program.ModulePath, node))
			case *ast.EnumStatement:
				module.Enums = append(module.Enums, resolver.exportEnum(program.ModulePath, node))
			}
		}
		result.Modules = append(result.Modules, module)
	}
	if err := packageextension.ValidateProjectDeclarationInput(result); err != nil {
		return packageextension.ProjectDeclarationInput{}, err
	}
	return result, nil
}

func newProjectInputResolver(programs []*ast.Program, options ProjectDeclarationInputOptions) (projectInputResolver, error) {
	result := projectInputResolver{
		programs:               map[string]*ast.Program{},
		aliases:                map[string]map[string]*ast.TypeAliasStatement{},
		definitions:            map[string]map[string]bool{},
		imports:                map[string]map[string]projectImportBinding{},
		packageAliasesByModule: options.PackageAliasesByModule,
	}
	for _, program := range programs {
		if program == nil {
			return result, fmt.Errorf("project declaration input contains an empty program")
		}
		if _, exists := result.programs[program.ModulePath]; exists {
			return result, fmt.Errorf("project declaration input contains duplicate module %q", program.ModulePath)
		}
		result.programs[program.ModulePath] = program
		result.aliases[program.ModulePath] = map[string]*ast.TypeAliasStatement{}
		result.definitions[program.ModulePath] = map[string]bool{}
		result.imports[program.ModulePath] = map[string]projectImportBinding{}
	}
	for _, program := range programs {
		for _, statement := range program.Statements {
			switch node := statement.(type) {
			case *ast.ImportStatement:
				for _, symbol := range node.Symbols {
					result.imports[program.ModulePath][symbol] = projectImportBinding{
						importPath: node.Path, modulePath: result.importModulePath(program.ModulePath, node.Path),
					}
				}
			case *ast.TypeAliasStatement:
				result.aliases[program.ModulePath][node.Name] = node
				result.definitions[program.ModulePath][node.Name] = true
			case *ast.ClassStatement:
				result.definitions[program.ModulePath][node.Name] = true
			case *ast.RecordStatement:
				result.definitions[program.ModulePath][node.Name] = true
			case *ast.EnumStatement:
				result.definitions[program.ModulePath][node.Name] = true
			case *ast.InterfaceStatement:
				result.definitions[program.ModulePath][node.Name] = true
			}
		}
	}
	return result, nil
}

func (r projectInputResolver) exportEnum(modulePath string, enum *ast.EnumStatement) packageextension.ProjectEnum {
	result := packageextension.ProjectEnum{
		Name: enum.Name, TypeParameters: projectTypeParameterNames(enum.TypeParameters), Span: exportSourceSpan(enum.Span()),
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
				Name: parameter.Name, Type: r.typeUse(modulePath, parameter.Type, typeParameters),
				Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest,
				Optional: parameter.Default != nil, Span: exportSourceSpan(parameter.Span()),
			})
		}
		if member.RawValue != nil {
			value := r.exportProjectValue(modulePath, member.RawValue, true)
			converted.RawValue = &value
		}
		result.Members = append(result.Members, converted)
	}
	return result
}

func (r projectInputResolver) exportClass(modulePath string, class *ast.ClassStatement) packageextension.ProjectClass {
	result := packageextension.ProjectClass{
		Name: class.Name, TypeParameters: projectTypeParameterNames(class.TypeParameters), Span: exportSourceSpan(class.Span()),
	}
	classTypeParameters := projectTypeParameterSet(class.TypeParameters)
	if superclass, ok := class.Superclass.(*ast.Identifier); ok {
		ref := ast.TypeRef{Base: superclass.Base, Name: superclass.Name}
		converted := r.typeUse(modulePath, ref, classTypeParameters)
		result.Superclass = &converted
	}
	for _, statement := range class.Body {
		switch node := statement.(type) {
		case *ast.MethodStatement:
			methodTypeParameters := cloneStringSet(classTypeParameters)
			for _, parameter := range node.TypeParameters {
				methodTypeParameters[parameter.Name] = true
			}
			method := packageextension.ProjectMethod{
				Name: node.Name, Class: node.Class, TypeParameters: projectTypeParameterNames(node.TypeParameters), Span: exportSourceSpan(node.Span()),
			}
			for _, parameter := range node.Parameters {
				method.Parameters = append(method.Parameters, packageextension.ProjectParameter{
					Name: parameter.Name, Type: r.typeUse(modulePath, parameter.Type, methodTypeParameters),
					Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest,
					Optional: parameter.Default != nil, Span: exportSourceSpan(parameter.Span()),
				})
			}
			if !node.ReturnType.Empty() {
				converted := r.typeUse(modulePath, node.ReturnType, methodTypeParameters)
				method.Return = &converted
			}
			result.Methods = append(result.Methods, method)
		case *ast.ExpressionStatement:
			if directive, ok := r.exportProjectDirective(modulePath, node); ok {
				result.Directives = append(result.Directives, directive)
			}
		}
	}
	return result
}

func (r projectInputResolver) exportProjectDirective(modulePath string, statement *ast.ExpressionStatement) (packageextension.ProjectDirective, bool) {
	call, ok := statement.Expression.(*ast.CallExpression)
	if !ok {
		return packageextension.ProjectDirective{}, false
	}
	callee, ok := call.Callee.(*ast.Identifier)
	if !ok {
		return packageextension.ProjectDirective{}, false
	}
	result := packageextension.ProjectDirective{Name: callee.Name, Span: exportSourceSpan(call.Span())}
	for _, argument := range call.Arguments {
		converted := packageextension.ProjectDirectiveArgument{
			Name: argument.Name, Splat: argument.Splat,
			Value: r.exportProjectValue(modulePath, argument.Value, false), Span: exportSourceSpan(argument.Value.Span()),
		}
		result.Arguments = append(result.Arguments, converted)
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

func (r projectInputResolver) exportProjectValue(modulePath string, expression ast.Expression, negativeInteger bool) packageextension.ProjectValue {
	result := exportStandaloneProjectValue(expression, negativeInteger)
	if result.Kind != "reference" {
		return result
	}
	if reference, ok := r.reference(modulePath, result.Name); ok {
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

func (r projectInputResolver) typeUse(modulePath string, ref ast.TypeRef, typeParameters map[string]bool) packageextension.ProjectTypeUse {
	authored := exportType(projectInputTypeRef(ref))
	r.attachDefinitions(modulePath, &authored, typeParameters)
	resolved, path := r.resolveType(modulePath, authored, map[string]bool{})
	return packageextension.ProjectTypeUse{
		Authored: authored, Resolved: resolved, ResolutionPath: path, Span: exportSourceSpan(ref.Span()),
	}
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

func (r projectInputResolver) attachDefinitions(modulePath string, typ *packageextension.Type, typeParameters map[string]bool) {
	if typ.Kind == "named" && !typeParameters[typ.Name] {
		if reference, ok := r.reference(modulePath, typ.Name); ok {
			typ.Definition = &packageextension.Definition{ModulePath: reference.ModulePath, ImportPath: reference.ImportPath}
		}
	}
	for index := range typ.Arguments {
		r.attachDefinitions(modulePath, &typ.Arguments[index], typeParameters)
	}
}

func (r projectInputResolver) resolveType(modulePath string, typ packageextension.Type, visiting map[string]bool) (packageextension.Type, []packageextension.ProjectTypeReference) {
	for index := range typ.Arguments {
		resolved, _ := r.resolveType(modulePath, typ.Arguments[index], cloneBoolMap(visiting))
		typ.Arguments[index] = resolved
	}
	if typ.Kind != "named" || typ.Nullable || len(typ.Arguments) != 0 {
		return typ, nil
	}
	reference, ok := projectTypeReference(typ)
	if !ok {
		return typ, nil
	}
	path := []packageextension.ProjectTypeReference{reference}
	alias := r.aliases[reference.ModulePath][typ.Name]
	if alias == nil || len(alias.TypeParameters) != 0 {
		return typ, path
	}
	key := reference.ModulePath + "\x00" + typ.Name
	if visiting[key] {
		return typ, path
	}
	visiting[key] = true
	target := exportType(projectInputTypeRef(alias.Target))
	r.attachDefinitions(reference.ModulePath, &target, nil)
	resolved, nestedPath := r.resolveType(reference.ModulePath, target, visiting)
	return resolved, append(path, nestedPath...)
}

func (r projectInputResolver) reference(modulePath, name string) (packageextension.ProjectTypeReference, bool) {
	if r.definitions[modulePath][name] {
		return packageextension.ProjectTypeReference{Name: name, ModulePath: modulePath}, true
	}
	if imported, ok := r.imports[modulePath][name]; ok {
		return packageextension.ProjectTypeReference{Name: name, ModulePath: imported.modulePath, ImportPath: imported.importPath}, true
	}
	return packageextension.ProjectTypeReference{}, false
}

func projectTypeReference(typ packageextension.Type) (packageextension.ProjectTypeReference, bool) {
	if typ.Definition == nil || typ.Definition.ModulePath == "" || typ.Name == "" {
		return packageextension.ProjectTypeReference{}, false
	}
	return packageextension.ProjectTypeReference{
		Name: typ.Name, ModulePath: typ.Definition.ModulePath, ImportPath: typ.Definition.ImportPath,
	}, true
}

func (r projectInputResolver) modulePath(importPath string) string {
	if _, exists := r.programs[importPath]; exists {
		return importPath
	}
	if _, exists := r.programs[importPath+"/index"]; exists {
		return importPath + "/index"
	}
	return importPath
}

func (r projectInputResolver) importModulePath(modulePath, importPath string) string {
	return r.modulePath(resolver.CanonicalPackageImport(importPath, r.packageAliasesByModule[modulePath]))
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
