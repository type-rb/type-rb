// Package lower converts checked syntax AST into the normalized IR. Keeping
// this pass explicit prevents backends from depending on parser node shapes.
package lower

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type lowerer struct {
	checked        checker.Result
	temporary      int
	generatedTypes map[string]map[string]resolver.Export
	usesJSX        bool
}

// resultBoundary describes the explicit Result returned by a try expression.
type resultBoundary struct {
	success types.Type
	fails   types.Type
	result  types.Type
}

func Program(checked checker.Result) *ir.Program {
	l := &lowerer{checked: checked, generatedTypes: map[string]map[string]resolver.Export{}}
	statements := l.statements(checked.Program.Statements)
	statements = l.generatedTypeImports(statements)
	statements = append(l.runtimeImports(statements), statements...)
	return &ir.Program{
		Mode:              checked.Program.Mode,
		Package:           checked.Program.Package,
		ModulePath:        checked.Program.ModulePath,
		GoModule:          checked.Program.GoModule,
		RubyLoader:        checked.Program.RubyLoader,
		TypeScriptRuntime: checked.Program.TypeScriptRuntime,
		UsesJSX:           l.usesJSX,
		Declarations:      checked.Resolution.Declarations,
		Statements:        statements,
	}
}

// requireGeneratedType records project types that lowering introduces even
// though the source did not import their defining modules. Result control flow
// is one such case: an imported function can expose a transparent Result alias
// owned by a third module, and the generated branch carries the expanded
// success and error types. Go and TypeScript must import those owner types for
// their generated annotations without making the names source-visible.
func (l *lowerer) requireGeneratedType(typ types.Type) {
	for _, argument := range typ.Args {
		l.requireGeneratedType(argument)
	}
	if typ.Kind != types.Named || typ.Name == "" || l.checked.Resolution.Catalog == nil {
		return
	}
	for modulePath, module := range l.checked.Resolution.Catalog.Modules {
		if module == nil || modulePath == l.checked.Program.ModulePath || module.CompilerOwned || module.Official {
			continue
		}
		exported, exists := module.Exports[typ.Name]
		if !exists || !contractTypeExport(exported.Kind) {
			continue
		}
		if l.generatedTypes[modulePath] == nil {
			l.generatedTypes[modulePath] = map[string]resolver.Export{}
		}
		l.generatedTypes[modulePath][typ.Name] = exported
		return
	}
}

// generatedTypeImports merges compiler-introduced project type dependencies
// into authored imports where possible and otherwise creates generated-only
// project imports.
// GeneratedTypeSymbols deliberately stay separate from Symbols so resolver and
// language-service visibility continue to match the authored program.
func (l *lowerer) generatedTypeImports(statements []ir.Statement) []ir.Statement {
	if len(l.generatedTypes) == 0 {
		return statements
	}
	loaded := map[string]*ir.Import{}
	for _, statement := range statements {
		if imported, ok := statement.(*ir.Import); ok {
			loaded[imported.Path] = imported
		}
	}
	paths := make([]string, 0, len(l.generatedTypes))
	for modulePath := range l.generatedTypes {
		paths = append(paths, modulePath)
	}
	sort.Strings(paths)
	generated := make([]ir.Statement, 0, len(paths))
	for _, modulePath := range paths {
		imported := loaded[modulePath]
		if imported == nil {
			imported = &ir.Import{
				Path:                      modulePath,
				Kind:                      string(resolver.ProjectImport),
				Implicit:                  true,
				IntrinsicSymbols:          map[string]bool{},
				RuntimeIndependentSymbols: map[string]bool{},
				RuntimeSymbols:            map[string]ir.RuntimeBinding{},
				SymbolKinds:               map[string]string{},
				SymbolTypes:               map[string]types.Type{},
				SymbolParameters:          map[string][]callsignature.Parameter{},
				SymbolTypeParameters:      map[string][]string{},
				RecordDefaults:            map[string]bool{},
			}
			generated = append(generated, imported)
		}
		names := make([]string, 0, len(l.generatedTypes[modulePath]))
		for name := range l.generatedTypes[modulePath] {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			exported := l.generatedTypes[modulePath][name]
			if !contains(imported.Symbols, name) && !contains(imported.GeneratedTypeSymbols, name) {
				imported.GeneratedTypeSymbols = append(imported.GeneratedTypeSymbols, name)
			}
			kind := string(exported.Kind)
			if exported.Kind == resolver.TypeAliasExport && exported.AliasEnum {
				kind = "enum_alias"
			}
			imported.SymbolKinds[name] = kind
			imported.SymbolTypes[name] = exported.Type
			imported.SymbolTypeParameters[name] = append([]string(nil), exported.TypeParameters...)
		}
	}
	return append(generated, statements...)
}

func contractTypeSymbols(imported *resolver.Import, native bool) []string {
	if imported == nil {
		return nil
	}
	sourceSymbols := map[string]bool{}
	for _, name := range imported.Symbols {
		sourceSymbols[name] = true
	}
	generated := map[string]bool{}
	visiting := map[string]bool{}
	var visitType func(types.Type)
	var visitExport func(string, resolver.Export)
	visitExport = func(name string, exported resolver.Export) {
		if visiting[name] {
			return
		}
		visiting[name] = true
		if !sourceSymbols[name] {
			if native && exported.NativeExported || !native && contractTypeExport(exported.Kind) {
				generated[name] = true
			}
		}
		visitType(exported.Type)
		visitType(exported.AliasTarget)
		visitType(exported.NewtypeTarget)
		for _, parameter := range exported.Parameters {
			visitType(parameter.Type)
		}
		for _, field := range exported.Fields {
			visitType(field.Type)
		}
		delete(visiting, name)
	}
	visitType = func(typ types.Type) {
		for _, argument := range typ.Args {
			visitType(argument)
		}
		if typ.Kind != types.Named || typ.Name == "" {
			return
		}
		if exported, exists := imported.Exports[typ.Name]; exists {
			visitExport(typ.Name, exported)
		}
	}
	for _, name := range imported.Symbols {
		if exported, exists := imported.Exports[name]; exists {
			visitExport(name, exported)
		}
	}
	result := make([]string, 0, len(generated))
	for name := range generated {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func contractTypeExport(kind resolver.ExportKind) bool {
	switch kind {
	case resolver.ClassExport, resolver.RecordExport, resolver.EnumExport, resolver.TypeAliasExport, resolver.NewtypeExport, resolver.InterfaceExport:
		return true
	default:
		return false
	}
}

func typeContracts(imported *resolver.Import, generated []string) map[string]ir.TypeContract {
	if imported == nil {
		return nil
	}
	names := append([]string(nil), generated...)
	for _, name := range imported.Symbols {
		if exported, ok := imported.Exports[name]; ok && contractTypeExport(exported.Kind) {
			names = append(names, name)
		}
	}
	result := map[string]ir.TypeContract{}
	for _, name := range names {
		exported, ok := imported.Exports[name]
		if !ok || !contractTypeExport(exported.Kind) {
			continue
		}
		contract := ir.TypeContract{
			TypeParameters: append([]string(nil), exported.TypeParameters...),
			Members:        map[string]ir.MemberContract{},
		}
		if exported.AliasTarget.Kind != "" {
			target := exported.AliasTarget
			contract.AliasTarget = &target
		}
		for memberName, member := range exported.Members {
			contract.Members[memberName] = ir.MemberContract{
				Kind:           string(member.Kind),
				Type:           member.Type,
				TypeParameters: append([]string(nil), member.TypeParameters...),
				Parameters:     append([]callsignature.Parameter(nil), member.Parameters...),
				Variadic:       member.Variadic,
				Class:          member.Class,
				Readonly:       member.Readonly,
			}
		}
		result[name] = contract
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func (l *lowerer) runtimeImports(statements []ir.Statement) []ir.Statement {
	loaded := map[string]*ir.Import{}
	for _, statement := range statements {
		if imported, ok := statement.(*ir.Import); ok {
			loaded[imported.Path] = imported
		}
	}
	paths := make([]string, 0, len(l.checked.RuntimeDependencies))
	for packagePath, definition := range l.checked.RuntimeDependencies {
		if definition == nil || definition.ModulePath == "" {
			continue
		}
		if imported := loaded[definition.ModulePath]; imported != nil {
			imported.RuntimeRequired = true
			for _, exported := range definition.RuntimeExports {
				if !contains(imported.Symbols, exported.Name) {
					imported.Symbols = append(imported.Symbols, exported.Name)
				}
				imported.SymbolKinds[exported.Name] = exported.Kind
			}
			continue
		}
		paths = append(paths, packagePath)
	}
	sort.Strings(paths)
	imports := make([]ir.Statement, 0, len(paths))
	for _, packagePath := range paths {
		definition := l.checked.RuntimeDependencies[packagePath]
		imported := &ir.Import{
			Path:                      definition.ModulePath,
			Alias:                     definition.RuntimeAlias,
			Kind:                      "standard",
			Standard:                  true,
			Runtime:                   true,
			RuntimeRequired:           true,
			Implicit:                  true,
			IntrinsicSymbols:          map[string]bool{},
			RuntimeIndependentSymbols: map[string]bool{},
			RuntimeSymbols:            map[string]ir.RuntimeBinding{},
			SymbolKinds:               map[string]string{},
			SymbolTypes:               map[string]types.Type{},
			SymbolParameters:          map[string][]callsignature.Parameter{},
			SymbolTypeParameters:      map[string][]string{},
			RecordDefaults:            map[string]bool{},
		}
		for _, exported := range definition.RuntimeExports {
			imported.Symbols = append(imported.Symbols, exported.Name)
			imported.SymbolKinds[exported.Name] = exported.Kind
		}
		imports = append(imports, imported)
	}
	return imports
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func (l *lowerer) statements(nodes []ast.Statement) []ir.Statement {
	result := make([]ir.Statement, 0, len(nodes))
	for _, node := range nodes {
		if lowered, ok := l.structuredResultStatement(node); ok {
			result = append(result, lowered...)
			continue
		}
		if lowered := l.statement(node); lowered != nil {
			result = append(result, lowered)
		}
	}
	return result
}

// structuredResultStatement preserves a value-producing structured block when
// its raw Result is consumed immediately by try or catch. Structured blocks are
// statement IR because backends must control the resource lifetime, so the raw
// Result is first assigned to a compiler-owned temporary and the ordinary
// Result lowering consumes that temporary in the authored statement.
func (l *lowerer) structuredResultStatement(node ast.Statement) ([]ir.Statement, bool) {
	var value ast.Expression
	switch n := node.(type) {
	case *ast.VariableStatement:
		value = n.Value
	case *ast.ReturnStatement:
		value = n.Value
	default:
		return nil, false
	}

	prefix, loweredValue, ok := l.structuredResultValue(value)
	if !ok {
		return nil, false
	}

	base := func(b ast.Base) ir.Base { return ir.Base{Span: b.SourceSpan, TrailingComment: b.TrailingComment} }
	var statement ir.Statement
	switch n := node.(type) {
	case *ast.VariableStatement:
		statement = &ir.Variable{
			Base:     base(n.Base),
			Name:     n.Name,
			Type:     l.checked.Variables[n],
			Value:    loweredValue,
			Mutable:  n.Mutable,
			Constant: n.Constant,
			Owner:    l.checked.ConstantOwners[n],
		}
	case *ast.ReturnStatement:
		statement = &ir.Return{Base: base(n.Base), Value: loweredValue}
	}
	return append(prefix, statement), true
}

func (l *lowerer) structuredResultValue(expression ast.Expression) ([]ir.Statement, ir.Expression, bool) {
	if expression == nil {
		return nil, nil, false
	}

	var operand ast.Expression
	switch node := expression.(type) {
	case *ast.TryExpression:
		operand = node.Value
	case *ast.CatchExpression:
		operand = node.Value
	default:
		return nil, nil, false
	}
	call, ok := operand.(*ast.CallExpression)
	if !ok {
		return nil, nil, false
	}
	semantic, ok := l.checked.StructuredBlocks[call]
	if !ok || semantic.ResultBoundary.Kind == "" || semantic.ResultBoundary.Kind == types.Never || semantic.ResultType.Kind == "" {
		return nil, nil, false
	}
	l.temporary++
	name := "__trbStructuredResult" + strconv.Itoa(l.temporary)
	resultType := semantic.ResultType
	temporary := &ir.Temporary{Base: ir.Base{Span: expression.Span()}, Name: name, Type: resultType}
	identifier := &ir.Identifier{
		ExprBase:  ir.NewExprBase(expression.Span(), resultType),
		Name:      name,
		Lexical:   true,
		Generated: true,
	}
	var structured ir.Statement
	if block, blockOK := l.structuredBlock(operand); blockOK {
		block.Result = &ir.StructuredBlockResult{Target: identifier, Type: resultType}
		structured = block
	} else if iteration, iterationOK := l.structuredIteration(operand); iterationOK {
		iteration.Result = &ir.IterationResult{Target: identifier, Type: resultType}
		structured = iteration
	} else {
		return nil, nil, false
	}

	var lowered ir.Expression
	switch node := expression.(type) {
	case *ast.TryExpression:
		lowered = l.resultTryValue(node, identifier)
	case *ast.CatchExpression:
		lowered = l.resultCatchValue(node, identifier)
	}
	lowered = l.expressionConversions(expression, lowered)
	return []ir.Statement{temporary, structured}, lowered, true
}

func (l *lowerer) statement(node ast.Statement) ir.Statement {
	base := func(b ast.Base) ir.Base { return ir.Base{Span: b.SourceSpan, TrailingComment: b.TrailingComment} }
	switch n := node.(type) {
	case *ast.CommentStatement:
		return &ir.Comment{Base: base(n.Base), Text: n.Text}
	case *ast.BlankStatement:
		return nil
	case *ast.ImportStatement:
		result := &ir.Import{
			Base:                      base(n.Base),
			Path:                      n.Path,
			DeclaredPath:              n.Path,
			Symbols:                   append([]string(nil), n.Symbols...),
			Alias:                     n.Alias,
			SymbolKinds:               map[string]string{},
			SymbolTypes:               map[string]types.Type{},
			SymbolParameters:          map[string][]callsignature.Parameter{},
			SymbolTypeParameters:      map[string][]string{},
			RecordDefaults:            map[string]bool{},
			IntrinsicSymbols:          map[string]bool{},
			RuntimeIndependentSymbols: map[string]bool{},
			RuntimeSymbols:            map[string]ir.RuntimeBinding{},
			Implicit:                  l.checked.CompilerGeneratedStart > 0 && n.Span().Start.Offset >= l.checked.CompilerGeneratedStart,
		}
		if resolved := l.checked.Resolution.Imports[n]; resolved != nil {
			result.Path = resolved.RuntimePath()
			result.Symbols = append([]string(nil), resolved.Symbols...)
			result.Alias = resolved.Alias
			result.Namespace = len(n.Symbols) == 0 && resolved.Alias != ""
			result.Kind = string(resolved.Kind)
			result.Standard = resolved.Kind == resolver.StandardImport
			result.Official = resolved.Kind == resolver.OfficialImport
			result.Native = resolved.Kind == resolver.NativeImport
			result.Platform = resolved.Definition != nil && resolved.Definition.Kind == "platform"
			result.Runtime = resolved.Definition != nil && resolved.Definition.Source != ""
			selectedRuntimeSymbols := map[string]bool{}
			if result.Namespace {
				for name := range resolved.Exports {
					selectedRuntimeSymbols[name] = true
				}
			} else {
				for _, name := range resolved.Symbols {
					selectedRuntimeSymbols[name] = true
				}
			}
			for name, exported := range resolved.Exports {
				kind := string(exported.Kind)
				if exported.Kind == resolver.TypeAliasExport && exported.AliasEnum {
					kind = "enum_alias"
				}
				result.SymbolKinds[name] = kind
				result.SymbolTypes[name] = exported.Type
				result.SymbolParameters[name] = append([]callsignature.Parameter(nil), exported.Parameters...)
				result.SymbolTypeParameters[name] = append([]string(nil), exported.TypeParameters...)
				if exported.Kind == resolver.RecordExport {
					for _, field := range exported.Fields {
						if field.HasDefault {
							result.RecordDefaults[name] = true
							break
						}
					}
				}
				if exported.Runtime != nil && selectedRuntimeSymbols[name] {
					result.RuntimeSymbols[name] = *lowerRuntimeBinding(exported.Runtime)
				}
			}
			result.GeneratedTypeSymbols = contractTypeSymbols(resolved, resolved.Kind == resolver.NativeImport)
			result.TypeContracts = typeContracts(resolved, result.GeneratedTypeSymbols)
			if resolved.Definition != nil {
				for name, symbol := range resolved.Definition.Symbols {
					if result.SymbolTypes[name].Kind == "" {
						result.SymbolKinds[name] = "function"
						result.SymbolTypes[name] = symbol.Return
						parameters := make([]callsignature.Parameter, len(symbol.Parameters))
						for index, parameter := range symbol.Parameters {
							kind := callsignature.Positional
							label := ""
							if parameter.Keyword {
								kind = callsignature.NamedOnly
								label = parameter.Name
							}
							presence := callsignature.Required
							if parameter.Optional {
								presence = callsignature.Omittable
							}
							parameters[index] = callsignature.Parameter{Kind: kind, Label: label, Type: parameter.Type, Presence: presence}
						}
						result.SymbolParameters[name] = parameters
						result.SymbolTypeParameters[name] = append([]string(nil), symbol.TypeParameters...)
					}
					if symbol.Intrinsic != "" {
						if _, hasRuntimeExport := resolved.Exports[name]; symbol.RuntimeIndependent || !hasRuntimeExport {
							result.IntrinsicSymbols[name] = true
						}
						if symbol.RuntimeIndependent {
							result.RuntimeIndependentSymbols[name] = true
						}
					}
				}
				for name := range l.checked.ImportUses[n] {
					if name != "" && !result.RuntimeIndependentSymbols[name] {
						result.RuntimeRequired = true
					}
				}
			}
		}
		return result
	case *ast.ClassStatement:
		result := &ir.Class{Base: base(n.Base), Name: n.Name, Superclass: l.expression(n.Superclass), Body: l.statements(n.Body)}
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		for index, implemented := range n.Implements {
			typ := lowerType(implemented)
			var authoredReference *ir.Reference
			if binding, ok := l.checked.Resolution.ImportedType(implemented.Name); ok {
				authoredReference = referenceFromBinding(&binding)
			}
			result.Implements = append(result.Implements, typ)
			result.ImplementReferences = append(result.ImplementReferences, authoredReference)
			if semantic := l.checked.InterfaceImplementations[n]; index < len(semantic) {
				result.ResolvedImplements = append(result.ResolvedImplements, semantic[index].Type)
				result.ResolvedImplementReferences = append(result.ResolvedImplementReferences, referenceFromBinding(semantic[index].TargetBinding))
			} else {
				result.ResolvedImplements = append(result.ResolvedImplements, typ)
				result.ResolvedImplementReferences = append(result.ResolvedImplementReferences, authoredReference)
			}
		}
		return result
	case *ast.RecordStatement:
		result := &ir.Record{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		return result
	case *ast.RecordFieldStatement:
		attributes := make([]ir.Attribute, len(n.Attributes))
		for index, attribute := range n.Attributes {
			arguments := make([]ir.CallArgument, len(attribute.Arguments))
			for argumentIndex, argument := range attribute.Arguments {
				arguments[argumentIndex] = ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat}
			}
			attributes[index] = ir.Attribute{Name: attribute.Name, Arguments: arguments}
		}
		return &ir.RecordField{Base: base(n.Base), Name: n.Name, Type: lowerType(n.Type), Default: l.expression(n.Default), Attributes: attributes}
	case *ast.EnumStatement:
		result := &ir.Enum{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
		if raw, ok := l.checked.RawEnums[n]; ok {
			result.RawType = raw.Type
		}
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		return result
	case *ast.EnumMemberStatement:
		member := &ir.EnumMember{Base: base(n.Base), Name: n.Name}
		member.RawValue = l.expression(n.RawValue)
		for _, field := range n.Parameters {
			member.Fields = append(member.Fields, ir.Parameter{Name: field.Name, Type: lowerType(field.Type)})
		}
		for _, attribute := range n.Attributes {
			lowered := ir.Attribute{Name: attribute.Name}
			for _, argument := range attribute.Arguments {
				lowered.Arguments = append(lowered.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
			}
			member.Attributes = append(member.Attributes, lowered)
		}
		return member
	case *ast.TypeAliasStatement:
		semantic := l.checked.TypeAliases[n]
		result := &ir.TypeAlias{Base: base(n.Base), Name: n.Name, AuthoredTarget: lowerType(n.Target), Target: semantic.Target}
		result.AuthoredTargetReference = referenceFromBinding(semantic.AuthoredTargetBinding)
		result.TargetReference = referenceFromBinding(semantic.TargetBinding)
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		for _, variant := range semantic.Variants {
			member := ir.EnumMember{Name: variant.Name}
			for _, field := range variant.Fields {
				member.Fields = append(member.Fields, ir.Parameter{Name: field.Name, Type: field.Type})
			}
			result.Variants = append(result.Variants, member)
		}
		return result
	case *ast.NewtypeStatement:
		semantic := l.checked.Newtypes[n]
		return &ir.Newtype{Base: base(n.Base), Name: n.Name, Target: semantic.Target}
	case *ast.ModuleStatement:
		return &ir.Module{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
	case *ast.InterfaceStatement:
		result := &ir.Interface{Base: base(n.Base), Name: n.Name}
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		for _, method := range n.Methods {
			if lowered, ok := l.statement(method).(*ir.Method); ok {
				result.Methods = append(result.Methods, lowered)
			}
		}
		return result
	case *ast.FieldStatement:
		return &ir.Field{Base: base(n.Base), Name: n.Name, Type: lowerType(n.Type), Value: l.expression(n.Value), ReadOnly: n.ReadOnly}
	case *ast.MethodStatement:
		returnType := lowerType(n.ReturnType)
		if n.ReturnType.Empty() {
			returnType = types.Type{Kind: types.Void, Name: "Void"}
		}
		method := &ir.Method{Base: base(n.Base), Name: n.Name, ReturnType: returnType, Class: n.Class}
		for _, parameter := range n.TypeParameters {
			method.TypeParameters = append(method.TypeParameters, parameter.Name)
		}
		method.Body = l.statements(n.Body)
		for _, parameter := range n.Parameters {
			typ := lowerType(parameter.Type)
			if parameter.Type.Empty() {
				typ = types.Type{Kind: types.Any, Name: "Any"}
			}
			method.Parameters = append(method.Parameters, ir.Parameter{Name: parameter.Name, Type: typ, Default: l.expression(parameter.Default), NamedOnly: parameter.NamedOnly, Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest})
		}
		return method
	case *ast.VariableStatement:
		typ := l.checked.Variables[n]
		// Backends annotate local variables with their inferred types. A value
		// returned by an imported function can therefore introduce a project type
		// whose owner was not named by the source import itself.
		l.requireGeneratedType(typ)
		if block, ok := l.structuredBlock(n.Value); ok {
			block.Result = &ir.StructuredBlockResult{
				Variable: &ir.Variable{Base: base(n.Base), Name: n.Name, Type: typ, Mutable: n.Mutable, Constant: n.Constant, Owner: l.checked.ConstantOwners[n]},
				Type:     typ,
			}
			return block
		}
		if iteration, ok := l.structuredIteration(n.Value); ok {
			iteration.Result = &ir.IterationResult{
				Variable: &ir.Variable{Base: base(n.Base), Name: n.Name, Type: typ, Mutable: n.Mutable, Constant: n.Constant, Owner: l.checked.ConstantOwners[n]},
				Type:     typ,
			}
			return iteration
		}
		return &ir.Variable{Base: base(n.Base), Name: n.Name, Type: typ, Value: l.expression(n.Value), Mutable: n.Mutable, Constant: n.Constant, Owner: l.checked.ConstantOwners[n]}
	case *ast.AssignmentStatement:
		if block, ok := l.structuredBlock(n.Value); ok {
			block.Result = &ir.StructuredBlockResult{Target: l.expression(n.Target), Type: l.checked.Expressions[n.Value]}
			return block
		}
		if iteration, ok := l.structuredIteration(n.Value); ok {
			iteration.Result = &ir.IterationResult{Target: l.expression(n.Target), Type: l.checked.Expressions[n.Value]}
			return iteration
		}
		return &ir.Assignment{Base: base(n.Base), Target: l.expression(n.Target), Operator: n.Operator, Value: l.expression(n.Value)}
	case *ast.ReturnStatement:
		if block, ok := l.structuredBlock(n.Value); ok {
			block.Result = &ir.StructuredBlockResult{Return: true, Type: l.checked.Expressions[n.Value]}
			return block
		}
		if iteration, ok := l.structuredIteration(n.Value); ok {
			iteration.Result = &ir.IterationResult{Return: true, Type: l.checked.Expressions[n.Value]}
			return iteration
		}
		value := l.expression(n.Value)
		return &ir.Return{Base: base(n.Base), Value: value}
	case *ast.BreakStatement:
		return &ir.Break{Base: base(n.Base)}
	case *ast.NextStatement:
		return &ir.Next{Base: base(n.Base)}
	case *ast.ExpressionStatement:
		if iteration, ok := l.structuredIteration(n.Expression); ok {
			return iteration
		}
		if iteration, ok := n.Expression.(*ast.IterationExpression); ok {
			if iteration.Operation == "map" || iteration.Operation == "select" || iteration.Operation == "reduce" || iteration.Operation == "any?" || iteration.Operation == "all?" || iteration.Operation == "none?" || iteration.Operation == "find" || iteration.Operation == "find_index" || iteration.Operation == "sort_by" || iteration.Operation == "sort_by_descending" {
				return &ir.ExpressionStatement{Base: base(n.Base), Expression: l.expression(iteration)}
			}
			result := &ir.Iterate{
				Base:      base(n.Base),
				Source:    l.expression(iteration.Source),
				Operation: iteration.Operation,
				SliceSize: l.expression(iteration.SliceSize),
				WithIndex: iteration.WithIndex,
			}
			if iteration.Block != nil {
				bindingTypes := l.checked.IterationBindings[iteration]
				for index, name := range iteration.Block.Parameters {
					typ := types.Type{Kind: types.Any, Name: "Any"}
					if index < len(bindingTypes) {
						typ = bindingTypes[index]
					}
					result.Bindings = append(result.Bindings, ir.IterationBinding{Name: name, Type: typ})
				}
				result.Body = l.statements(iteration.Block.Body)
			}
			return result
		}
		return &ir.ExpressionStatement{Base: base(n.Base), Expression: l.expression(n.Expression)}
	case *ast.IfStatement:
		return l.ifNode(n, false)
	case *ast.CaseStatement:
		return l.caseNode(n, false)
	case *ast.WhileStatement:
		return &ir.While{Base: base(n.Base), Condition: l.expression(n.Condition), Body: l.statements(n.Body)}
	case *ast.NativeStatement:
		return &ir.Native{Base: base(n.Base), Text: n.Text}
	case *ast.NativeBlock:
		return &ir.NativeBlock{Base: base(n.Base), Header: n.Header, Body: l.statements(n.Body), Closer: n.Closer}
	default:
		return nil
	}
}

func (l *lowerer) structuredIteration(expression ast.Expression) (*ir.Iterate, bool) {
	call, ok := expression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return nil, false
	}
	member, external := l.checked.ExternalMembers[call.Callee]
	callee, method := call.Callee.(*ast.MemberExpression)
	if !external || !method || member.Block == nil || !member.Block.Structured || member.Block.Return.Name != "" {
		return nil, false
	}
	semantic, checked := l.checked.StructuredBlocks[call]
	resultBoundary := checked && semantic.ResultBoundary.Kind != "" && semantic.ResultBoundary.Kind != types.Never && semantic.ResultType.Kind != ""
	if !resultBoundary {
		return nil, false
	}
	result := &ir.Iterate{
		Base:           ir.Base{Span: expression.Span()},
		Source:         l.expression(callee.Receiver),
		Operation:      callee.Name,
		Intrinsic:      member.Intrinsic,
		Fails:          semantic.ResultBoundary,
		EffectSuccess:  semantic.Return,
		CaptureEffect:  true,
		ResultBoundary: true,
	}
	for _, argument := range call.Arguments {
		if argument.Name == "batch_size" || argument.Name == "" {
			result.SliceSize = l.expression(argument.Value)
			break
		}
	}
	for index, name := range call.Block.Parameters {
		typ := types.Type{Kind: types.Any, Name: "Any"}
		if index < len(member.Block.Parameters) {
			typ = member.Block.Parameters[index]
		}
		result.Bindings = append(result.Bindings, ir.IterationBinding{Name: name, Type: typ})
	}
	result.Body = l.statements(call.Block.Body)
	return result, true
}

func (l *lowerer) structuredBlock(expression ast.Expression) (*ir.StructuredBlock, bool) {
	call, ok := expression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return nil, false
	}
	member, external := l.checked.ExternalMembers[call.Callee]
	semantic, checked := l.checked.StructuredBlocks[call]
	if !external || !checked || member.Block == nil || !member.Block.Structured || member.Block.Return.Name == "" {
		return nil, false
	}
	resultBoundary := semantic.ResultBoundary.Kind != "" && semantic.ResultBoundary.Kind != types.Never && semantic.ResultType.Kind != ""
	if !resultBoundary {
		return nil, false
	}
	loweredCall := &ir.Call{
		ExprBase: ir.NewExprBase(call.Span(), semantic.ResultType),
		Callee:   l.expression(call.Callee),
	}
	if codec, ok := l.checked.CodecApplications[call]; ok {
		loweredCall.Codec = l.lowerCodecSchema(codec.Schema)
	}
	for _, argument := range call.Arguments {
		loweredCall.Arguments = append(loweredCall.Arguments, ir.CallArgument{
			Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat,
		})
	}
	resultIndex, resultExpression := lowerControlFlowBranchExpression(call.Block.Body)
	if resultExpression == nil {
		return nil, false
	}
	result := &ir.StructuredBlock{
		Base:          ir.Base{Span: call.Span()},
		Call:          loweredCall,
		Intrinsic:     member.Intrinsic,
		Fails:         semantic.ResultBoundary,
		EffectSuccess: semantic.Return,
		CaptureEffect: true,
	}
	result.Body = l.statements(call.Block.Body[:resultIndex])
	result.Value = l.expression(semantic.Result)
	result.Body = append(result.Body, l.statements(call.Block.Body[resultIndex+1:])...)
	for index, name := range call.Block.Parameters {
		typ := types.Type{Kind: types.Any, Name: "Any"}
		if index < len(semantic.Parameters) {
			typ = semantic.Parameters[index]
		}
		result.Bindings = append(result.Bindings, ir.IterationBinding{Name: name, Type: typ})
	}
	return result, true
}

func (l *lowerer) expression(node ast.Expression) ir.Expression {
	if node == nil {
		return nil
	}
	return l.expressionConversions(node, l.expressionWithoutConversion(node))
}

func (l *lowerer) expressionConversions(node ast.Expression, result ir.Expression) ir.Expression {
	if source, ok := l.checked.NullableUnwraps[node]; ok && result != nil {
		if value := expressionWithType(result, source); value != nil {
			result = &ir.Conversion{
				ExprBase: ir.NewExprBase(node.Span(), result.ExprType()),
				Kind:     ir.NullableToNonNullableConversion,
				Value:    value,
			}
		}
	}
	if target, ok := l.checked.Conversions[node]; ok && result != nil {
		kind := ir.IntegerToFloatConversion
		if target.Kind == types.Iterable && result.ExprType().Kind == types.Range {
			kind = ir.RangeToIterableConversion
		} else if target.Nullable && !result.ExprType().Nullable && result.ExprType().Kind != types.Nil {
			kind = ir.NonNullableToNullableConversion
		} else if result.ExprType().Kind == types.Union {
			kind = ir.UnionIntegerToFloatConversion
		}
		result = &ir.Conversion{
			ExprBase: ir.NewExprBase(node.Span(), target),
			Kind:     kind,
			Value:    result,
		}
	}
	if bridge, ok := l.checked.NativeResultBridges[node]; ok && result != nil {
		if _, _, valid := types.FunctionSignature(bridge.Type); valid {
			result = &ir.Conversion{
				ExprBase: ir.NewExprBase(node.Span(), bridge.Type),
				Kind:     ir.ResultFunctionToPromiseRejectionConversion,
				Value:    result,
			}
		}
	}
	if call, ok := node.(*ast.CallExpression); ok && result != nil {
		if bridge, bridged := l.checked.NativeCallResultBridges[call]; bridged {
			if nativeCall := expressionWithType(result, bridge.Success); nativeCall != nil {
				result = &ir.Conversion{
					ExprBase: ir.NewExprBase(node.Span(), bridge.ResultType),
					Kind:     ir.PromiseRejectionToResultConversion,
					Value:    nativeCall,
				}
			}
		}
	}
	return result
}

func expressionWithType(expression ir.Expression, typ types.Type) ir.Expression {
	switch node := expression.(type) {
	case *ir.Identifier:
		copy := *node
		copy.ExprBase.Type = typ
		return &copy
	case *ir.Member:
		copy := *node
		copy.ExprBase.Type = typ
		return &copy
	case *ir.Call:
		copy := *node
		copy.ExprBase.Type = typ
		return &copy
	default:
		return nil
	}
}

func (l *lowerer) expressionWithoutConversion(node ast.Expression) ir.Expression {
	typ := l.checked.Expressions[node]
	base := ir.NewExprBase(node.Span(), typ)
	switch n := node.(type) {
	case *ast.IfStatement:
		return l.ifNode(n, true)
	case *ast.CaseStatement:
		return l.caseNode(n, true)
	case *ast.Identifier:
		return &ir.Identifier{ExprBase: base, Name: n.Name, Owner: l.checked.Constants[n], Lexical: l.checked.LexicalBindings[n], Reference: l.reference(n)}
	case *ast.Literal:
		return &ir.Literal{ExprBase: base, Kind: string(n.Kind), Raw: n.Raw}
	case *ast.InterpolatedString:
		result := &ir.InterpolatedString{ExprBase: base, Raw: n.Raw}
		for _, part := range n.Parts {
			result.Parts = append(result.Parts, ir.StringPart{Text: part.Text, Expression: l.expression(part.Expression)})
		}
		return result
	case *ast.SymbolLiteral:
		return &ir.Symbol{ExprBase: base, Name: n.Name, Raw: n.Raw}
	case *ast.ArrayLiteral:
		l.requireGeneratedType(typ)
		result := &ir.Array{ExprBase: base}
		for _, element := range n.Elements {
			result.Elements = append(result.Elements, l.expression(element))
		}
		return result
	case *ast.HashLiteral:
		l.requireGeneratedType(typ)
		result := &ir.Hash{ExprBase: base}
		for _, entry := range n.Entries {
			result.Entries = append(result.Entries, ir.HashEntry{Key: l.expression(entry.Key), Value: l.expression(entry.Value)})
		}
		return result
	case *ast.JSXElement:
		l.usesJSX = true
		result := &ir.JSXElement{ExprBase: base, Name: n.Name, Component: l.expression(n.Component), Fragment: n.Fragment}
		for _, attribute := range n.Attributes {
			result.Attributes = append(result.Attributes, ir.JSXAttribute{Name: attribute.Name, Value: l.expression(attribute.Value), Boolean: attribute.Boolean})
		}
		for _, child := range n.Children {
			switch item := child.(type) {
			case *ast.JSXElement:
				result.Children = append(result.Children, l.expression(item).(*ir.JSXElement))
			case *ast.JSXText:
				result.Children = append(result.Children, &ir.JSXText{Text: item.Text})
			case *ast.JSXExpression:
				result.Children = append(result.Children, &ir.JSXExpression{Value: l.expression(item.Value)})
			}
		}
		return result
	case *ast.UnaryExpression:
		return &ir.Unary{ExprBase: base, Operator: n.Operator, Operand: l.expression(n.Operand)}
	case *ast.BinaryExpression:
		return &ir.Binary{ExprBase: base, Left: l.expression(n.Left), Operator: n.Operator, Right: l.expression(n.Right)}
	case *ast.RangeExpression:
		return &ir.Range{ExprBase: base, Start: l.expression(n.Start), End: l.expression(n.End), Exclusive: n.Exclusive}
	case *ast.TryExpression:
		return l.resultTry(n)
	case *ast.CatchExpression:
		return l.resultCatch(n)
	case *ast.AttemptExpression:
		// The parser retains this node only to recover and report pre-0.3 source.
		return nil
	case *ast.LambdaExpression:
		result := &ir.Lambda{ExprBase: base, ReturnType: types.FromName("Void")}
		if !n.ReturnType.Empty() {
			result.ReturnType = lowerType(n.ReturnType)
		}
		for _, parameter := range n.Parameters {
			result.Parameters = append(result.Parameters, ir.Parameter{Name: parameter.Name, Type: lowerType(parameter.Type)})
		}
		result.Body = l.statements(n.Body)
		return result
	case *ast.IterationExpression:
		source := l.expression(n.Source)
		initial := l.expression(n.Initial)
		limit := l.expression(n.Limit)
		result := &ir.Transform{
			ExprBase:  base,
			Source:    source,
			Operation: n.Operation,
			Initial:   initial,
			Limit:     limit,
			WithIndex: n.WithIndex,
			ItemType:  l.checked.Iterations[n],
		}
		if n.Block != nil {
			if n.Operation == "reduce" {
				if len(n.Block.Parameters) > 0 {
					result.Accumulator = n.Block.Parameters[0]
				}
				if len(n.Block.Parameters) > 1 {
					result.Item = n.Block.Parameters[1]
				}
			} else {
				if len(n.Block.Parameters) > 0 {
					result.Item = n.Block.Parameters[0]
				}
				if len(n.Block.Parameters) > 1 {
					result.Index = n.Block.Parameters[1]
				}
			}
			if len(n.Block.Body) > 0 {
				last := len(n.Block.Body) - 1
				if expression, ok := n.Block.Body[last].(*ast.ExpressionStatement); ok {
					result.Body = l.statements(n.Block.Body[:last])
					result.Result = l.expression(expression.Expression)
				} else if expression, ok := n.Block.Body[last].(ast.Expression); ok {
					result.Body = l.statements(n.Block.Body[:last])
					result.Result = l.expression(expression)
				} else {
					result.Body = l.statements(n.Block.Body)
				}
			}
		}
		return result
	case *ast.CallExpression:
		if semantic, ok := l.checked.NewtypeCalls[n]; ok {
			kind := ir.NewtypeConstructionConversion
			var value ir.Expression
			if semantic.Operation == "new" {
				if len(n.Arguments) > 0 {
					value = l.expression(n.Arguments[0].Value)
				}
			} else {
				kind = ir.NewtypeValueConversion
				if member, memberCall := n.Callee.(*ast.MemberExpression); memberCall {
					value = l.expression(member.Receiver)
				}
			}
			return &ir.Conversion{ExprBase: base, Kind: kind, Value: value}
		}
		if semantic, ok := l.checked.EnumCalls[n]; ok {
			result := &ir.EnumCall{
				ExprBase:      base,
				EnumName:      semantic.EnumName,
				Owner:         semantic.Owner,
				Method:        semantic.Method,
				CallSignature: append([]callsignature.Parameter(nil), l.checked.CallSignatures[n]...),
				Reference:     l.reference(n.Callee),
			}
			if semantic.Receiver != nil {
				result.Receiver = l.expression(semantic.Receiver)
			} else {
				result.Receiver = &ir.Identifier{ExprBase: ir.NewExprBase(n.Span(), types.FromName(semantic.EnumName)), Name: "self", Lexical: true}
			}
			for _, argument := range n.Arguments {
				result.Arguments = append(result.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
			}
			if semantic.Raw != nil {
				result.RawType = semantic.Raw.Type
				names := make([]string, 0, len(semantic.Raw.Values))
				for name := range semantic.Raw.Values {
					names = append(names, name)
				}
				sort.Strings(names)
				for _, name := range names {
					result.RawValues = append(result.RawValues, ir.EnumRawValue{Member: name, Raw: semantic.Raw.Values[name].Raw})
				}
			}
			return result
		}
		if variant, ok := l.checked.EnumConstructors[n]; ok {
			result := &ir.EnumConstruct{ExprBase: base, EnumName: variant.EnumName, Member: variant.Name, TypeArguments: append([]types.Type(nil), variant.TypeArguments...), Reference: l.reference(n.Callee)}
			for _, argument := range n.Arguments {
				result.Arguments = append(result.Arguments, l.expression(argument.Value))
			}
			return result
		}
		if specialization, ok := l.checked.CallSpecializations[n]; ok {
			arguments := make([]ir.CallArgument, 0, len(specialization.Arguments))
			for _, argument := range specialization.Arguments {
				value := l.expression(argument)
				arguments = append(arguments, ir.CallArgument{Value: value})
			}
			return &ir.Call{
				ExprBase:        base,
				DeclarationOnly: l.checked.DeclarationOnlyCalls[n],
				Callee: &ir.Identifier{
					// Source-level named calls carry the declared return type on
					// their callee identifier. Keep the same representation so Ruby
					// emits a method invocation rather than a first-class fn .call.
					ExprBase: ir.NewExprBase(n.Callee.Span(), base.Type),
					Name:     specialization.Callee,
				},
				Arguments: arguments,
			}
		}
		result := &ir.Call{
			ExprBase: base, Callee: l.expression(n.Callee),
			CallSignature:   append([]callsignature.Parameter(nil), l.checked.CallSignatures[n]...),
			DeclarationOnly: l.checked.DeclarationOnlyCalls[n],
		}
		if construction, ok := l.checked.RecordConstructions[n]; ok {
			result.RecordTarget = l.expression(construction.Target)
			for _, field := range construction.Fields {
				result.RecordFields = append(result.RecordFields, ir.RecordFieldContract{Name: field.Name, Type: field.Type, HasDefault: field.HasDefault})
			}
		}
		if codec, ok := l.checked.CodecApplications[n]; ok {
			result.Codec = l.lowerCodecSchema(codec.Schema)
		}
		for _, argument := range n.Arguments {
			result.Arguments = append(result.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
		}
		if n.Block != nil {
			result.Block = l.expression(n.Block).(*ir.Block)
		}
		return result
	case *ast.MemberExpression:
		receiver := n.Receiver
		name := n.Name
		reference := l.reference(n)
		if reference != nil && strings.HasPrefix(reference.Intrinsic, "trb.orm.association.") && !strings.Contains(reference.Intrinsic, ".value.") {
			if association, ok := n.Receiver.(*ast.MemberExpression); ok {
				receiver = association.Receiver
				name = association.Name
			}
		}
		member := &ir.Member{ExprBase: base, Receiver: l.expression(receiver), Name: name, Safe: n.Safe, Namespace: n.Namespace, ClassField: l.checked.ClassFieldAccesses[n], Reference: reference}
		for _, alternative := range l.checked.UnionMemberAccesses[n] {
			member.UnionAlternatives = append(member.UnionAlternatives, ir.UnionMemberAlternative{Type: alternative.Alternative, MemberType: alternative.Member})
		}
		if reference != nil && strings.HasPrefix(reference.Intrinsic, "trb.orm.association.value.") {
			return &ir.Call{ExprBase: base, Callee: member}
		}
		return member
	case *ast.GenericExpression:
		application := l.checked.GenericApplications[n]
		result := &ir.TypeApply{ExprBase: base, Receiver: l.expression(n.Receiver), Owner: application.Owner, OwnerArguments: append([]types.Type(nil), application.OwnerArguments...), Kind: application.Kind}
		for _, argument := range n.Arguments {
			result.Arguments = append(result.Arguments, lowerType(argument))
		}
		return result
	case *ast.IndexExpression:
		return &ir.Index{ExprBase: base, Receiver: l.expression(n.Receiver), Index: l.expression(n.Index)}
	case *ast.BlockExpression:
		return &ir.Block{ExprBase: base, Parameters: append([]string(nil), n.Parameters...), Body: l.statements(n.Body), Brace: n.Brace}
	case *ast.NativeExpression:
		return &ir.NativeExpression{ExprBase: base, Text: n.Text}
	default:
		return nil
	}
}

func (l *lowerer) ifNode(node *ast.IfStatement, expression bool) *ir.If {
	typ := types.FromName("Void")
	if expression {
		typ = l.checked.Expressions[node]
	}
	exprBase := ir.NewExprBase(node.Span(), typ)
	exprBase.TrailingComment = node.TrailingComment
	result := &ir.If{
		ExprBase:  exprBase,
		Condition: l.expression(node.Condition),
		HasElse:   node.HasElse,
	}
	result.Then, result.ThenResult, result.ThenDiverges = l.controlFlowBranch(node.Then, expression)
	result.Else, result.ElseResult, result.ElseDiverges = l.controlFlowBranch(node.Else, expression)
	for _, branch := range node.ElseIf {
		body, branchResult, diverges := l.controlFlowBranch(branch.Body, expression)
		result.ElseIf = append(result.ElseIf, ir.IfBranch{
			Condition: l.expression(branch.Condition),
			Body:      body,
			Result:    branchResult,
			Diverges:  diverges,
		})
	}
	return result
}

func (l *lowerer) caseNode(node *ast.CaseStatement, expression bool) *ir.Case {
	typ := types.FromName("Void")
	if expression {
		typ = l.checked.Expressions[node]
	}
	exprBase := ir.NewExprBase(node.Span(), typ)
	exprBase.TrailingComment = node.TrailingComment
	result := &ir.Case{
		ExprBase: exprBase,
		Value:    l.expression(node.Value),
		Leading:  l.statements(node.Leading),
		HasElse:  node.HasElse,
	}
	result.Else, result.ElseResult, result.ElseDiverges = l.controlFlowBranch(node.Else, expression)
	narrowing, narrows := l.checked.CaseNarrowings[node]
	if narrows && narrowing.Else.Kind != "" && narrowing.Else.Kind != types.Invalid {
		result.ElseNarrowings = append(result.ElseNarrowings, ir.CaseBinding{Name: narrowing.Name, Type: narrowing.Else})
	}
	for _, branch := range node.Branches {
		body, branchResult, diverges := l.controlFlowBranch(branch.Body, expression)
		lowered := ir.CaseBranch{
			Base:     ir.Base{Span: branch.Span(), TrailingComment: branch.TrailingComment},
			Value:    l.expression(branch.Value),
			Body:     body,
			Result:   branchResult,
			Diverges: diverges,
		}
		for _, alternative := range branch.Alternatives {
			lowered.Alternatives = append(lowered.Alternatives, l.expression(alternative))
		}
		if pattern, ok := l.checked.CasePatterns[branch.Value]; ok {
			lowered.TypePattern = pattern.TypeUnion
			lowered.MatchType = pattern.MatchType
			result.TypeUnion = result.TypeUnion || pattern.TypeUnion
			lowered.EnumName = pattern.Variant.EnumName
			lowered.Member = pattern.Variant.Name
			lowered.PayloadEnum = pattern.PayloadEnum
			for _, binding := range pattern.Bindings {
				lowered.Bindings = append(lowered.Bindings, ir.CaseBinding{Name: binding.Name, Field: binding.Field.Name, Type: binding.Field.Type})
			}
		}
		if narrows {
			if narrowed, ok := narrowing.Branches[branch.Value]; ok {
				lowered.Narrowings = append(lowered.Narrowings, ir.CaseBinding{Name: narrowing.Name, Type: narrowed})
			}
		}
		result.Branches = append(result.Branches, lowered)
	}
	return result
}

func (l *lowerer) controlFlowBranch(body []ast.Statement, expression bool) ([]ir.Statement, ir.Expression, bool) {
	if !expression {
		return l.statements(body), nil, false
	}
	resultIndex, result := lowerControlFlowBranchExpression(body)
	if result == nil {
		return l.statements(body), nil, terminalControlFlowTransfer(body)
	}
	statements := l.statements(body[:resultIndex])
	statements = append(statements, l.statements(body[resultIndex+1:])...)
	return statements, l.expression(result), l.checked.Expressions[result].Kind == types.Never
}

func terminalControlFlowTransfer(body []ast.Statement) bool {
	for index := len(body) - 1; index >= 0; index-- {
		switch body[index].(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		case *ast.ReturnStatement, *ast.BreakStatement, *ast.NextStatement:
			return true
		default:
			return false
		}
	}
	return false
}

func lowerControlFlowBranchExpression(body []ast.Statement) (int, ast.Expression) {
	for index := len(body) - 1; index >= 0; index-- {
		switch statement := body[index].(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		case *ast.ExpressionStatement:
			return index, statement.Expression
		default:
			if expression, ok := statement.(ast.Expression); ok {
				return index, expression
			}
			return index, nil
		}
	}
	return -1, nil
}

func (l *lowerer) lowerCodecSchema(schema checker.CodecSchema) *ir.CodecSchema {
	l.requireGeneratedType(schema.Type)
	result := &ir.CodecSchema{Type: schema.Type, Kind: schema.Kind, Module: schema.Module, RawType: schema.RawType}
	for _, value := range schema.RawValues {
		result.RawValues = append(result.RawValues, ir.EnumRawValue{Member: value.Member, Raw: value.Raw})
	}
	if schema.Reference != nil {
		result.Reference = &ir.Reference{Package: schema.Reference.Import.RuntimePath(), Alias: schema.Reference.Import.Alias, Symbol: schema.Reference.Name, ExportKind: string(schema.Reference.Export.Kind)}
	}
	if schema.Element != nil {
		result.Element = l.lowerCodecSchema(*schema.Element)
	}
	for _, field := range schema.Fields {
		result.Fields = append(result.Fields, ir.CodecField{Name: field.Name, WireName: field.WireName, Schema: l.lowerCodecSchema(*field.Schema)})
	}
	return result
}

func (l *lowerer) reference(node ast.Expression) *ir.Reference {
	binding, ok := l.checked.References[node]
	if !ok || binding.Import == nil {
		member, external := l.checked.ExternalMembers[node]
		if !external || member.Intrinsic == "" {
			return nil
		}
		receiver := false
		switch value := node.(type) {
		case *ast.MemberExpression:
			receiver = true
		case *ast.GenericExpression:
			_, receiver = value.Receiver.(*ast.MemberExpression)
		}
		receiver = receiver && !member.Class
		return &ir.Reference{Intrinsic: member.Intrinsic, Symbol: member.Name, ExportKind: string(member.Kind), ReceiverMethod: receiver}
	}
	result := &ir.Reference{Package: binding.Import.RuntimePath(), Alias: binding.Import.Alias, Symbol: binding.Name}
	if binding.Library != nil {
		result.Intrinsic = binding.Library.Intrinsic
		result.ReceiverMethod = binding.Library.HasReceiver()
		if !result.ReceiverMethod {
			result.ExportKind = string(resolver.FunctionExport)
		}
	}
	if binding.Export != nil {
		result.ExportKind = string(binding.Export.Kind)
		result.Runtime = lowerRuntimeBinding(binding.Export.Runtime)
	}
	if binding.Member != nil {
		result.ExportKind = string(binding.Member.Kind)
		result.ClassMember = binding.Member.Class
		if binding.Export != nil {
			result.Owner = binding.Export.Name
		}
	}
	return result
}

func referenceFromBinding(binding *resolver.Binding) *ir.Reference {
	if binding == nil || binding.Import == nil {
		return nil
	}
	result := &ir.Reference{Package: binding.Import.RuntimePath(), Alias: binding.Import.Alias, Symbol: binding.Name}
	if binding.Library != nil {
		result.Intrinsic = binding.Library.Intrinsic
		result.ReceiverMethod = binding.Library.HasReceiver()
		if !result.ReceiverMethod {
			result.ExportKind = string(resolver.FunctionExport)
		}
	}
	if binding.Export != nil {
		result.ExportKind = string(binding.Export.Kind)
		result.Runtime = lowerRuntimeBinding(binding.Export.Runtime)
	}
	if binding.Member != nil {
		result.ExportKind = string(binding.Member.Kind)
		result.ClassMember = binding.Member.Class
		if binding.Export != nil {
			result.Owner = binding.Export.Name
		}
	}
	return result
}

func lowerRuntimeBinding(binding *resolver.RuntimeBinding) *ir.RuntimeBinding {
	if binding == nil {
		return nil
	}
	return &ir.RuntimeBinding{
		Identity: binding.Identity, Dependency: binding.Dependency, Module: binding.Module,
		Symbol: binding.Symbol, CallConvention: binding.CallConvention, MaySuspend: binding.MaySuspend,
		PropagatesExecutionScope: binding.PropagatesExecutionScope,
	}
}

func resultSuccessType(typ types.Type) types.Type {
	if typ.Kind == types.Void {
		return types.FromName("Unit")
	}
	return typ
}

func resultType(success, failure types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{success, failure}}
}

func resultReference() *ir.Reference {
	return &ir.Reference{Package: "trb/std/result/index", Alias: "__trb_result", Symbol: "Result", ExportKind: "enum"}
}

func (l *lowerer) resultPattern(span token.Span, result types.Type, member string) ir.Expression {
	reference := resultReference()
	receiver := &ir.Identifier{ExprBase: ir.NewExprBase(span, result), Name: "Result", Reference: reference}
	return &ir.Member{ExprBase: ir.NewExprBase(span, result), Receiver: receiver, Name: member, Namespace: true, Reference: reference}
}

func (l *lowerer) resultFailure(span token.Span, boundary resultBoundary, value ir.Expression) ir.Expression {
	return &ir.EnumConstruct{
		ExprBase:      ir.NewExprBase(span, boundary.result),
		EnumName:      "Result",
		Member:        "Err",
		TypeArguments: []types.Type{boundary.success, boundary.fails},
		Arguments:     []ir.Expression{value},
		Reference:     resultReference(),
	}
}

func (l *lowerer) resultPropagation(span token.Span, value ir.Expression, success, failure types.Type, boundary resultBoundary) ir.Expression {
	l.temporary++
	suffix := strconv.Itoa(l.temporary)
	valueName := "__trbEffectValue" + suffix
	errorName := "__trbEffectError" + suffix
	rawResult := resultType(resultSuccessType(success), failure)

	valueIdentifier := &ir.Identifier{ExprBase: ir.NewExprBase(span, resultSuccessType(success)), Name: valueName, Lexical: true, Generated: true}
	errorIdentifier := &ir.Identifier{ExprBase: ir.NewExprBase(span, failure), Name: errorName, Lexical: true, Generated: true}
	returnFailure := l.resultFailure(span, boundary, assignableConversion(span, errorIdentifier, boundary.fails))

	return &ir.Case{
		ExprBase: ir.NewExprBase(span, resultSuccessType(success)),
		Value:    value,
		Branches: []ir.CaseBranch{
			{
				Value:       l.resultPattern(span, rawResult, "Ok"),
				EnumName:    "Result",
				Member:      "Ok",
				Bindings:    []ir.CaseBinding{{Name: valueName, Field: "value", Type: resultSuccessType(success), Generated: true}},
				PayloadEnum: true,
				Result:      valueIdentifier,
			},
			{
				Value:       l.resultPattern(span, rawResult, "Err"),
				EnumName:    "Result",
				Member:      "Err",
				Bindings:    []ir.CaseBinding{{Name: errorName, Field: "error", Type: failure, Generated: true}},
				PayloadEnum: true,
				Body:        []ir.Statement{&ir.Return{Value: returnFailure}},
				Diverges:    true,
			},
		},
	}
}

func assignableConversion(span token.Span, value ir.Expression, target types.Type) ir.Expression {
	source := value.ExprType()
	if types.Equivalent(target, source) {
		return value
	}
	conversionType := target
	kind := ir.ConversionKind("")
	switch {
	case target.Kind == types.Iterable && source.Kind == types.Range:
		kind = ir.RangeToIterableConversion
	case target.Nullable && !source.Nullable && source.Kind != types.Nil:
		kind = ir.NonNullableToNullableConversion
	case target.Kind == types.Union && source.Kind == types.Union &&
		unionContainsTypeKind(target, types.Float) && unionContainsTypeKind(source, types.Int):
		kind = ir.UnionIntegerToFloatConversion
	case target.Kind == types.Union && !target.Nullable && !source.Nullable &&
		(source.Kind == types.Int || source.Kind == types.IntLiteral) && unionContainsNonNullableTypeKind(target, types.Float):
		kind = ir.IntegerToFloatConversion
		conversionType = types.FromName("Float")
	case target.Kind == types.Float && (source.Kind == types.Int || source.Kind == types.IntLiteral):
		kind = ir.IntegerToFloatConversion
	}
	if kind == "" {
		return value
	}
	return &ir.Conversion{ExprBase: ir.NewExprBase(span, conversionType), Kind: kind, Value: value}
}

func unionContainsTypeKind(typ types.Type, kind types.Kind) bool {
	for _, alternative := range typ.Args {
		if alternative.Kind == kind {
			return true
		}
	}
	return false
}

func unionContainsNonNullableTypeKind(typ types.Type, kind types.Kind) bool {
	for _, alternative := range typ.Args {
		if alternative.Kind == kind && !alternative.Nullable {
			return true
		}
	}
	return false
}

func (l *lowerer) resultTry(node *ast.TryExpression) ir.Expression {
	_, ok := l.checked.ResultTries[node]
	if !ok {
		return l.expression(node.Value)
	}
	return l.resultTryValue(node, l.expression(node.Value))
}

func (l *lowerer) resultTryValue(node *ast.TryExpression, value ir.Expression) ir.Expression {
	semantic, ok := l.checked.ResultTries[node]
	if !ok {
		return value
	}
	// The generated case IIFE returns the operand success type and constructs
	// the boundary Result on error. Raw operand errors are only values and do
	// not appear in a generated target-language type annotation.
	l.requireGeneratedType(semantic.SuccessType)
	l.requireGeneratedType(semantic.ReturnType)
	boundary := resultBoundary{
		success: semantic.ReturnSuccessType,
		fails:   semantic.ReturnErrorType,
		result:  semantic.ReturnType,
	}
	return l.resultPropagation(node.Span(), value, semantic.SuccessType, semantic.ErrorType, boundary)
}

func (l *lowerer) resultCatch(node *ast.CatchExpression) ir.Expression {
	_, ok := l.checked.ResultCatches[node]
	if !ok {
		return l.expression(node.Value)
	}
	return l.resultCatchValue(node, l.expression(node.Value))
}

func (l *lowerer) resultCatchValue(node *ast.CatchExpression, value ir.Expression) ir.Expression {
	semantic, ok := l.checked.ResultCatches[node]
	if !ok {
		return value
	}
	// catch lowers to an IIFE whose generated return annotation is the unwrapped
	// success type. The operand error remains an inferred local value.
	l.requireGeneratedType(semantic.SuccessType)
	result := semantic.ResultType
	success := semantic.SuccessType
	failure := semantic.ErrorType

	l.temporary++
	valueName := "__trbCatchValue" + strconv.Itoa(l.temporary)
	valueIdentifier := &ir.Identifier{
		ExprBase:  ir.NewExprBase(node.Span(), success),
		Name:      valueName,
		Lexical:   true,
		Generated: true,
	}
	handlerBody, handlerResult, _ := l.controlFlowBranch(node.Body, true)
	handlerDiverges := semantic.HandlerDiverges
	exprBase := ir.NewExprBase(node.Span(), success)
	exprBase.TrailingComment = node.TrailingComment

	return &ir.Case{
		ExprBase: exprBase,
		Value:    value,
		Branches: []ir.CaseBranch{
			{
				Value:       l.resultPattern(node.Span(), result, "Ok"),
				EnumName:    "Result",
				Member:      "Ok",
				Bindings:    []ir.CaseBinding{{Name: valueName, Field: "value", Type: success, Generated: true}},
				PayloadEnum: true,
				Result:      valueIdentifier,
			},
			{
				Value:       l.resultPattern(node.Span(), result, "Err"),
				EnumName:    "Result",
				Member:      "Err",
				Bindings:    []ir.CaseBinding{{Name: node.Binding.Name, Field: "error", Type: failure}},
				PayloadEnum: true,
				Body:        handlerBody,
				Result:      handlerResult,
				Diverges:    handlerDiverges,
			},
		},
	}
}

func (l *lowerer) unitValue(span token.Span) ir.Expression {
	typ := types.FromName("Unit")
	reference := &ir.Reference{Package: "trb/std/unit/index", Alias: "unit", Symbol: "Unit", ExportKind: "record"}
	receiver := &ir.Identifier{ExprBase: ir.NewExprBase(span, typ), Name: "Unit", Reference: reference}
	callee := &ir.Member{ExprBase: ir.NewExprBase(span, typ), Receiver: receiver, Name: "new", Reference: reference}
	return &ir.Call{ExprBase: ir.NewExprBase(span, typ), Callee: callee}
}

func lowerType(ref ast.TypeRef) types.Type {
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = lowerType(alternative)
		}
		return types.UnionOf(alternatives...)
	}
	if ref.FunctionReturn != nil {
		parameters := make([]types.Type, len(ref.FunctionParameters))
		for index, parameter := range ref.FunctionParameters {
			parameters[index] = lowerType(parameter)
		}
		result := types.FunctionOf(parameters, lowerType(*ref.FunctionReturn))
		result.Nullable = ref.Nullable
		return result
	}
	t := types.FromName(ref.Name)
	t.Nullable = ref.Nullable
	for _, arg := range ref.Arguments {
		t.Args = append(t.Args, lowerType(arg))
	}
	if ref.Array {
		t = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{t}, Nullable: ref.Nullable}
	}
	return t
}
