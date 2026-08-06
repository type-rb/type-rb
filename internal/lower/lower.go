// Package lower converts checked syntax AST into the normalized IR. Keeping
// this pass explicit prevents backends from depending on parser node shapes.
package lower

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type lowerer struct{ checked checker.Result }

func Program(checked checker.Result) *ir.Program {
	l := &lowerer{checked: checked}
	statements := l.statements(checked.Program.Statements)
	statements = append(l.runtimeImports(statements), statements...)
	return &ir.Program{
		Mode:       checked.Program.Mode,
		Package:    checked.Program.Package,
		ModulePath: checked.Program.ModulePath,
		GoModule:   checked.Program.GoModule,
		RubyLoader: checked.Program.RubyLoader,
		Statements: statements,
	}
}

func (l *lowerer) runtimeImports(statements []ir.Statement) []ir.Statement {
	loaded := map[string]bool{}
	for _, statement := range statements {
		if imported, ok := statement.(*ir.Import); ok {
			loaded[imported.Path] = true
		}
	}
	paths := make([]string, 0, len(l.checked.RuntimeDependencies))
	for packagePath, definition := range l.checked.RuntimeDependencies {
		if definition == nil || definition.ModulePath == "" || loaded[definition.ModulePath] {
			continue
		}
		paths = append(paths, packagePath)
	}
	sort.Strings(paths)
	imports := make([]ir.Statement, 0, len(paths))
	for _, packagePath := range paths {
		definition := l.checked.RuntimeDependencies[packagePath]
		imported := &ir.Import{
			Path:             definition.ModulePath,
			Alias:            definition.RuntimeAlias,
			Kind:             "standard",
			Standard:         true,
			Runtime:          true,
			Implicit:         true,
			IntrinsicSymbols: map[string]bool{},
			SymbolKinds:      map[string]string{},
		}
		for _, exported := range definition.RuntimeExports {
			imported.Symbols = append(imported.Symbols, exported.Name)
			imported.SymbolKinds[exported.Name] = exported.Kind
		}
		imports = append(imports, imported)
	}
	return imports
}

func (l *lowerer) statements(nodes []ast.Statement) []ir.Statement {
	result := make([]ir.Statement, 0, len(nodes))
	for _, node := range nodes {
		if lowered := l.statement(node); lowered != nil {
			result = append(result, lowered)
		}
	}
	return result
}

func (l *lowerer) statement(node ast.Statement) ir.Statement {
	base := func(b ast.Base) ir.Base { return ir.Base{Span: b.SourceSpan, TrailingComment: b.TrailingComment} }
	switch n := node.(type) {
	case *ast.CommentStatement:
		return &ir.Comment{Base: base(n.Base), Text: n.Text}
	case *ast.BlankStatement:
		return nil
	case *ast.ImportStatement:
		result := &ir.Import{Base: base(n.Base), Path: n.Path, Symbols: append([]string(nil), n.Symbols...), Alias: n.Alias, SymbolKinds: map[string]string{}, IntrinsicSymbols: map[string]bool{}}
		if resolved := l.checked.Resolution.Imports[n]; resolved != nil {
			result.Path = resolved.RuntimePath()
			result.Symbols = append([]string(nil), resolved.Symbols...)
			result.Alias = resolved.Alias
			result.Namespace = len(n.Symbols) == 0 && resolved.Alias != ""
			result.Kind = string(resolved.Kind)
			result.Standard = resolved.Definition != nil
			result.Platform = resolved.Definition != nil && resolved.Definition.Kind == "platform"
			result.Runtime = resolved.Definition != nil && resolved.Definition.Source != ""
			for name, exported := range resolved.Exports {
				result.SymbolKinds[name] = string(exported.Kind)
			}
			if resolved.Definition != nil {
				for name, symbol := range resolved.Definition.Symbols {
					if symbol.Intrinsic != "" {
						if _, hasRuntimeExport := resolved.Exports[name]; !hasRuntimeExport {
							result.IntrinsicSymbols[name] = true
						}
					}
				}
			}
		}
		return result
	case *ast.ClassStatement:
		return &ir.Class{Base: base(n.Base), Name: n.Name, Superclass: l.expression(n.Superclass), Implements: append([]string(nil), n.Implements...), Body: l.statements(n.Body)}
	case *ast.RecordStatement:
		return &ir.Record{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
	case *ast.RecordFieldStatement:
		attributes := make([]ir.Attribute, len(n.Attributes))
		for index, attribute := range n.Attributes {
			arguments := make([]ir.CallArgument, len(attribute.Arguments))
			for argumentIndex, argument := range attribute.Arguments {
				arguments[argumentIndex] = ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat}
			}
			attributes[index] = ir.Attribute{Name: attribute.Name, Arguments: arguments}
		}
		return &ir.RecordField{Base: base(n.Base), Name: n.Name, Type: lowerType(n.Type), Attributes: attributes}
	case *ast.EnumStatement:
		result := &ir.Enum{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
		for _, parameter := range n.TypeParameters {
			result.TypeParameters = append(result.TypeParameters, parameter.Name)
		}
		return result
	case *ast.EnumMemberStatement:
		member := &ir.EnumMember{Base: base(n.Base), Name: n.Name}
		for _, field := range n.Parameters {
			member.Fields = append(member.Fields, ir.Parameter{Name: field.Name, Type: lowerType(field.Type)})
		}
		return member
	case *ast.ModuleStatement:
		return &ir.Module{Base: base(n.Base), Name: n.Name, Body: l.statements(n.Body)}
	case *ast.InterfaceStatement:
		result := &ir.Interface{Base: base(n.Base), Name: n.Name}
		for _, method := range n.Methods {
			if lowered, ok := l.statement(method).(*ir.Method); ok {
				result.Methods = append(result.Methods, lowered)
			}
		}
		return result
	case *ast.FieldStatement:
		return &ir.Field{Base: base(n.Base), Name: n.Name, Type: lowerType(n.Type), Value: l.expression(n.Value), ReadOnly: n.ReadOnly}
	case *ast.MethodStatement:
		method := &ir.Method{Base: base(n.Base), Name: n.Name, ReturnType: lowerType(n.ReturnType), Body: l.statements(n.Body), Class: n.Class}
		for _, parameter := range n.TypeParameters {
			method.TypeParameters = append(method.TypeParameters, parameter.Name)
		}
		if n.ReturnType.Empty() {
			method.ReturnType = types.Type{Kind: types.Void, Name: "Void"}
		}
		for _, parameter := range n.Parameters {
			typ := lowerType(parameter.Type)
			if parameter.Type.Empty() {
				typ = types.Type{Kind: types.Any, Name: "Any"}
			}
			method.Parameters = append(method.Parameters, ir.Parameter{Name: parameter.Name, Type: typ, Default: l.expression(parameter.Default), Keyword: parameter.Keyword, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest})
		}
		return method
	case *ast.VariableStatement:
		typ := l.checked.Variables[n]
		return &ir.Variable{Base: base(n.Base), Name: n.Name, Type: typ, Value: l.expression(n.Value), Mutable: n.Mutable, Constant: n.Constant, Owner: l.checked.ConstantOwners[n]}
	case *ast.AssignmentStatement:
		return &ir.Assignment{Base: base(n.Base), Target: l.expression(n.Target), Operator: n.Operator, Value: l.expression(n.Value)}
	case *ast.ReturnStatement:
		return &ir.Return{Base: base(n.Base), Value: l.expression(n.Value)}
	case *ast.BreakStatement:
		return &ir.Break{Base: base(n.Base)}
	case *ast.NextStatement:
		return &ir.Next{Base: base(n.Base)}
	case *ast.ExpressionStatement:
		if iteration, ok := n.Expression.(*ast.IterationExpression); ok {
			if iteration.Operation == "map" || iteration.Operation == "select" || iteration.Operation == "reduce" {
				return &ir.ExpressionStatement{Base: base(n.Base), Expression: l.expression(iteration)}
			}
			result := &ir.Iterate{
				Base:      base(n.Base),
				Source:    l.expression(iteration.Source),
				Operation: iteration.Operation,
				SliceSize: l.expression(iteration.SliceSize),
				WithIndex: iteration.WithIndex,
				ItemType:  l.checked.Iterations[iteration],
			}
			if iteration.Block != nil {
				if len(iteration.Block.Parameters) > 0 {
					result.Item = iteration.Block.Parameters[0]
				}
				if len(iteration.Block.Parameters) > 1 {
					result.Index = iteration.Block.Parameters[1]
				}
				result.Body = l.statements(iteration.Block.Body)
			}
			return result
		}
		return &ir.ExpressionStatement{Base: base(n.Base), Expression: l.expression(n.Expression)}
	case *ast.IfStatement:
		result := &ir.If{Base: base(n.Base), Condition: l.expression(n.Condition), Then: l.statements(n.Then), Else: l.statements(n.Else)}
		for _, branch := range n.ElseIf {
			result.ElseIf = append(result.ElseIf, ir.IfBranch{Condition: l.expression(branch.Condition), Body: l.statements(branch.Body)})
		}
		return result
	case *ast.CaseStatement:
		result := &ir.Case{
			Base:    base(n.Base),
			Value:   l.expression(n.Value),
			Leading: l.statements(n.Leading),
			Else:    l.statements(n.Else),
			HasElse: n.HasElse,
		}
		for _, branch := range n.Branches {
			lowered := ir.CaseBranch{
				Base:  base(branch.Base),
				Value: l.expression(branch.Value),
				Body:  l.statements(branch.Body),
			}
			if pattern, ok := l.checked.CasePatterns[branch.Value]; ok {
				lowered.EnumName = pattern.Variant.EnumName
				lowered.Member = pattern.Variant.Name
				lowered.PayloadEnum = pattern.PayloadEnum
				for _, binding := range pattern.Bindings {
					lowered.Bindings = append(lowered.Bindings, ir.CaseBinding{Name: binding.Name, Field: binding.Field.Name, Type: binding.Field.Type})
				}
			}
			result.Branches = append(result.Branches, lowered)
		}
		return result
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

func (l *lowerer) expression(node ast.Expression) ir.Expression {
	if node == nil {
		return nil
	}
	result := l.expressionWithoutConversion(node)
	if target, ok := l.checked.Conversions[node]; ok && result != nil {
		return &ir.Conversion{
			ExprBase: ir.NewExprBase(node.Span(), target),
			Kind:     ir.IntegerToFloatConversion,
			Value:    result,
		}
	}
	return result
}

func (l *lowerer) expressionWithoutConversion(node ast.Expression) ir.Expression {
	typ := l.checked.Expressions[node]
	base := ir.NewExprBase(node.Span(), typ)
	switch n := node.(type) {
	case *ast.Identifier:
		return &ir.Identifier{ExprBase: base, Name: n.Name, Owner: l.checked.Constants[n], Reference: l.reference(n)}
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
		result := &ir.Array{ExprBase: base}
		for _, element := range n.Elements {
			result.Elements = append(result.Elements, l.expression(element))
		}
		return result
	case *ast.HashLiteral:
		result := &ir.Hash{ExprBase: base}
		for _, entry := range n.Entries {
			result.Entries = append(result.Entries, ir.HashEntry{Key: l.expression(entry.Key), Value: l.expression(entry.Value)})
		}
		return result
	case *ast.UnaryExpression:
		return &ir.Unary{ExprBase: base, Operator: n.Operator, Operand: l.expression(n.Operand)}
	case *ast.BinaryExpression:
		return &ir.Binary{ExprBase: base, Left: l.expression(n.Left), Operator: n.Operator, Right: l.expression(n.Right)}
	case *ast.RangeExpression:
		return &ir.Range{ExprBase: base, Start: l.expression(n.Start), End: l.expression(n.End), Exclusive: n.Exclusive}
	case *ast.IterationExpression:
		result := &ir.Transform{
			ExprBase:  base,
			Source:    l.expression(n.Source),
			Operation: n.Operation,
			Initial:   l.expression(n.Initial),
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
				} else {
					result.Body = l.statements(n.Block.Body)
				}
			}
		}
		return result
	case *ast.CallExpression:
		if variant, ok := l.checked.EnumConstructors[n]; ok {
			result := &ir.EnumConstruct{ExprBase: base, EnumName: variant.EnumName, Member: variant.Name, TypeArguments: append([]types.Type(nil), variant.TypeArguments...), Reference: l.reference(n.Callee)}
			for _, argument := range n.Arguments {
				result.Arguments = append(result.Arguments, l.expression(argument.Value))
			}
			return result
		}
		result := &ir.Call{ExprBase: base, Callee: l.expression(n.Callee)}
		if codec, ok := l.checked.CodecApplications[n]; ok {
			result.Codec = lowerCodecSchema(codec.Schema)
		}
		for _, argument := range n.Arguments {
			result.Arguments = append(result.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
		}
		if n.Block != nil {
			result.Block = l.expression(n.Block).(*ir.Block)
		}
		return result
	case *ast.MemberExpression:
		return &ir.Member{ExprBase: base, Receiver: l.expression(n.Receiver), Name: n.Name, Safe: n.Safe, Namespace: n.Namespace, Reference: l.reference(n)}
	case *ast.GenericExpression:
		result := &ir.TypeApply{ExprBase: base, Receiver: l.expression(n.Receiver)}
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

func lowerCodecSchema(schema checker.CodecSchema) *ir.CodecSchema {
	result := &ir.CodecSchema{Type: schema.Type, Kind: schema.Kind, Module: schema.Module}
	if schema.Reference != nil {
		result.Reference = &ir.Reference{Package: schema.Reference.Import.RuntimePath(), Alias: schema.Reference.Import.Alias, Symbol: schema.Reference.Name, ExportKind: string(schema.Reference.Export.Kind)}
	}
	if schema.Element != nil {
		result.Element = lowerCodecSchema(*schema.Element)
	}
	for _, field := range schema.Fields {
		result.Fields = append(result.Fields, ir.CodecField{Name: field.Name, WireName: field.WireName, Schema: lowerCodecSchema(*field.Schema)})
	}
	return result
}

func (l *lowerer) reference(node ast.Expression) *ir.Reference {
	binding, ok := l.checked.References[node]
	if !ok || binding.Import == nil {
		return nil
	}
	result := &ir.Reference{Package: binding.Import.RuntimePath(), Alias: binding.Import.Alias, Symbol: binding.Name}
	if binding.Library != nil {
		result.Intrinsic = binding.Library.Intrinsic
		result.ReceiverMethod = binding.Library.HasReceiver()
	}
	if binding.Export != nil {
		result.ExportKind = string(binding.Export.Kind)
	}
	if binding.Member != nil {
		result.ExportKind = string(binding.Member.Kind)
	}
	return result
}

func lowerType(ref ast.TypeRef) types.Type {
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
