// Package lower converts checked syntax AST into the normalized IR. Keeping
// this pass explicit prevents backends from depending on parser node shapes.
package lower

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type lowerer struct{ checked checker.Result }

func Program(checked checker.Result) *ir.Program {
	l := &lowerer{checked: checked}
	return &ir.Program{
		Mode:       checked.Program.Mode,
		Package:    checked.Program.Package,
		EntryPoint: checked.Program.EntryPoint,
		ModulePath: checked.Program.ModulePath,
		GoModule:   checked.Program.GoModule,
		RubyLoader: checked.Program.RubyLoader,
		Statements: l.statements(checked.Program.Statements),
	}
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
		result := &ir.Import{Base: base(n.Base), Path: n.Path, Symbols: append([]string(nil), n.Symbols...), Alias: n.Alias, SymbolKinds: map[string]string{}}
		if resolved := l.checked.Resolution.Imports[n]; resolved != nil {
			result.Path = resolved.Path
			result.Symbols = append([]string(nil), resolved.Symbols...)
			result.Alias = resolved.Alias
			result.Kind = string(resolved.Kind)
			result.Standard = resolved.Definition != nil
			result.Platform = resolved.Definition != nil && resolved.Definition.Kind == "platform"
			for name, exported := range resolved.Exports {
				result.SymbolKinds[name] = string(exported.Kind)
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
		return &ir.Variable{Base: base(n.Base), Name: n.Name, Type: typ, Value: l.expression(n.Value), Mutable: n.Mutable, Constant: n.Constant}
	case *ast.AssignmentStatement:
		return &ir.Assignment{Base: base(n.Base), Target: l.expression(n.Target), Operator: n.Operator, Value: l.expression(n.Value)}
	case *ast.ReturnStatement:
		return &ir.Return{Base: base(n.Base), Value: l.expression(n.Value)}
	case *ast.ExpressionStatement:
		if iteration, ok := n.Expression.(*ast.IterationExpression); ok {
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
	typ := l.checked.Expressions[node]
	base := ir.NewExprBase(node.Span(), typ)
	switch n := node.(type) {
	case *ast.Identifier:
		return &ir.Identifier{ExprBase: base, Name: n.Name, Reference: l.reference(n)}
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
	case *ast.CallExpression:
		result := &ir.Call{ExprBase: base, Callee: l.expression(n.Callee)}
		for _, argument := range n.Arguments {
			result.Arguments = append(result.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
		}
		if n.Block != nil {
			result.Block = l.expression(n.Block).(*ir.Block)
		}
		return result
	case *ast.MemberExpression:
		return &ir.Member{ExprBase: base, Receiver: l.expression(n.Receiver), Name: n.Name, Safe: n.Safe, Namespace: n.Namespace, Reference: l.reference(n)}
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

func (l *lowerer) reference(node ast.Expression) *ir.Reference {
	binding, ok := l.checked.References[node]
	if !ok || binding.Import == nil {
		return nil
	}
	result := &ir.Reference{Package: binding.Import.Path, Alias: binding.Import.Alias, Symbol: binding.Name}
	if binding.Library != nil {
		result.Intrinsic = binding.Library.Intrinsic
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
