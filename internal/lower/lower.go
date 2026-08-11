// Package lower converts checked syntax AST into the normalized IR. Keeping
// this pass explicit prevents backends from depending on parser node shapes.
package lower

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type lowerer struct {
	checked          checker.Result
	temporary        int
	effectBoundaries []effectBoundary
	usesJSX          bool
}

type effectBoundary struct {
	success types.Type
	fails   types.Type
	result  types.Type
}

func Program(checked checker.Result) *ir.Program {
	l := &lowerer{checked: checked}
	statements := l.statements(checked.Program.Statements)
	statements = append(l.runtimeImports(statements), statements...)
	return &ir.Program{
		Mode:              checked.Program.Mode,
		Package:           checked.Program.Package,
		ModulePath:        checked.Program.ModulePath,
		GoModule:          checked.Program.GoModule,
		RubyLoader:        checked.Program.RubyLoader,
		TypeScriptRuntime: checked.Program.TypeScriptRuntime,
		UsesJSX:           l.usesJSX,
		Statements:        statements,
	}
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
			SymbolKinds:               map[string]string{},
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
		result := &ir.Import{
			Base:                      base(n.Base),
			Path:                      n.Path,
			Symbols:                   append([]string(nil), n.Symbols...),
			Alias:                     n.Alias,
			SymbolKinds:               map[string]string{},
			IntrinsicSymbols:          map[string]bool{},
			RuntimeIndependentSymbols: map[string]bool{},
		}
		if resolved := l.checked.Resolution.Imports[n]; resolved != nil {
			result.Path = resolved.RuntimePath()
			result.Symbols = append([]string(nil), resolved.Symbols...)
			result.Alias = resolved.Alias
			result.Namespace = len(n.Symbols) == 0 && resolved.Alias != ""
			result.Kind = string(resolved.Kind)
			result.Standard = resolved.Kind == resolver.StandardImport
			result.Official = resolved.Kind == resolver.OfficialImport
			result.Platform = resolved.Definition != nil && resolved.Definition.Kind == "platform"
			result.Runtime = resolved.Definition != nil && resolved.Definition.Source != ""
			for name, exported := range resolved.Exports {
				kind := string(exported.Kind)
				if exported.Kind == resolver.TypeAliasExport && exported.AliasEnum {
					kind = "enum_alias"
				}
				result.SymbolKinds[name] = kind
			}
			if resolved.Definition != nil {
				for name, symbol := range resolved.Definition.Symbols {
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
		return member
	case *ast.TypeAliasStatement:
		semantic := l.checked.TypeAliases[n]
		result := &ir.TypeAlias{Base: base(n.Base), Name: n.Name, Target: semantic.Target}
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
		successType := lowerType(n.ReturnType)
		if n.ReturnType.Empty() {
			successType = types.Type{Kind: types.Void, Name: "Void"}
		}
		failsType := lowerFailureType(n.Fails)
		method := &ir.Method{Base: base(n.Base), Name: n.Name, SuccessType: successType, ReturnType: successType, Fails: failsType, Class: n.Class}
		for _, parameter := range n.TypeParameters {
			method.TypeParameters = append(method.TypeParameters, parameter.Name)
		}
		if failsType.Kind != types.Never {
			internalSuccess := effectSuccessType(successType)
			boundary := effectBoundary{success: internalSuccess, fails: failsType, result: resultType(internalSuccess, failsType)}
			method.ReturnType = boundary.result
			l.effectBoundaries = append(l.effectBoundaries, boundary)
			method.Body = l.statements(n.Body)
			l.effectBoundaries = l.effectBoundaries[:len(l.effectBoundaries)-1]
			if successType.Kind == types.Void {
				method.Body = append(method.Body, &ir.Return{Value: l.resultSuccess(n.Span(), boundary, l.unitValue(n.Span()))})
			}
		} else {
			method.Body = l.statements(n.Body)
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
		if boundary, ok := l.currentEffectBoundary(); ok {
			if value == nil {
				value = l.unitValue(n.Span())
			}
			value = l.resultSuccess(n.Span(), boundary, value)
		}
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
			if iteration.Operation == "map" || iteration.Operation == "select" || iteration.Operation == "reduce" {
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
	captureEffect := false
	callExpression := expression
	var captureBoundary *effectBoundary
	if attempt, ok := expression.(*ast.AttemptExpression); ok {
		callExpression = attempt.Value
		captureEffect = true
		if semantic, exists := l.checked.Attempts[attempt]; exists {
			boundary := effectBoundary{success: semantic.SuccessType, fails: semantic.ErrorType, result: semantic.ResultType}
			captureBoundary = &boundary
		}
	}
	call, ok := callExpression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return nil, false
	}
	member, external := l.checked.ExternalMembers[call.Callee]
	callee, method := call.Callee.(*ast.MemberExpression)
	if !external || !method || member.Block == nil || !member.Block.Structured || member.Block.Return.Name != "" {
		return nil, false
	}
	result := &ir.Iterate{
		Base:            ir.Base{Span: expression.Span()},
		Source:          l.expression(callee.Receiver),
		Operation:       callee.Name,
		Intrinsic:       member.Intrinsic,
		Fails:           l.checked.ExpressionEffects[call],
		CaptureEffect:   captureEffect,
		UnhandledEffect: l.checked.UnhandledEffects[call],
	}
	if captureBoundary != nil {
		result.EffectSuccess = captureBoundary.success
	} else if boundary, exists := l.currentEffectBoundary(); exists {
		result.EffectSuccess = boundary.success
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
	if captureBoundary != nil {
		l.effectBoundaries = append(l.effectBoundaries, *captureBoundary)
	}
	result.Body = l.statements(call.Block.Body)
	if captureBoundary != nil {
		l.effectBoundaries = l.effectBoundaries[:len(l.effectBoundaries)-1]
	}
	return result, true
}

func (l *lowerer) structuredBlock(expression ast.Expression) (*ir.StructuredBlock, bool) {
	captureEffect := false
	if attempt, ok := expression.(*ast.AttemptExpression); ok {
		expression = attempt.Value
		captureEffect = true
	}
	call, ok := expression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return nil, false
	}
	member, external := l.checked.ExternalMembers[call.Callee]
	semantic, checked := l.checked.StructuredBlocks[call]
	if !external || !checked || member.Block == nil || !member.Block.Structured || member.Block.Return.Name == "" {
		return nil, false
	}
	fails := l.checked.ExpressionEffects[call]
	success := l.checked.Expressions[call]
	callType := success
	if fails.Kind != "" && fails.Kind != types.Never {
		callType = resultType(effectSuccessType(success), fails)
	}
	loweredCall := &ir.Call{
		ExprBase: ir.NewExprBase(call.Span(), callType),
		Callee:   l.expression(call.Callee),
		Fails:    fails,
	}
	if codec, ok := l.checked.CodecApplications[call]; ok {
		loweredCall.Codec = lowerCodecSchema(codec.Schema)
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
		Base:            ir.Base{Span: call.Span()},
		Call:            loweredCall,
		Intrinsic:       member.Intrinsic,
		Fails:           fails,
		EffectSuccess:   success,
		CaptureEffect:   captureEffect,
		UnhandledEffect: l.checked.UnhandledEffects[call],
	}
	if fails.Kind != "" && fails.Kind != types.Never {
		if !captureEffect {
			if outer, ok := l.currentEffectBoundary(); ok {
				result.PropagateSuccess = outer.success
			}
		}
		boundary := effectBoundary{success: effectSuccessType(success), fails: fails, result: callType}
		l.effectBoundaries = append(l.effectBoundaries, boundary)
		result.Body = l.statements(call.Block.Body[:resultIndex])
		result.Value = l.expression(semantic.Result)
		result.Body = append(result.Body, l.statements(call.Block.Body[resultIndex+1:])...)
		l.effectBoundaries = l.effectBoundaries[:len(l.effectBoundaries)-1]
	} else {
		result.Body = l.statements(call.Block.Body[:resultIndex])
		result.Value = l.expression(semantic.Result)
		result.Body = append(result.Body, l.statements(call.Block.Body[resultIndex+1:])...)
	}
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
	result := l.expressionWithoutConversion(node)
	if target, ok := l.checked.Conversions[node]; ok && result != nil {
		kind := ir.IntegerToFloatConversion
		if target.Nullable && !result.ExprType().Nullable && result.ExprType().Kind != types.Nil {
			kind = ir.NonNullableToNullableConversion
		} else if result.ExprType().Kind == types.Union {
			kind = ir.UnionIntegerToFloatConversion
		}
		return &ir.Conversion{
			ExprBase: ir.NewExprBase(node.Span(), target),
			Kind:     kind,
			Value:    result,
		}
	}
	return result
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
	case *ast.AttemptExpression:
		semantic, ok := l.checked.Attempts[n]
		if !ok {
			return nil
		}
		boundary := effectBoundary{success: semantic.SuccessType, fails: semantic.ErrorType, result: semantic.ResultType}
		attempt := &ir.Attempt{
			ExprBase: ir.NewExprBase(n.Span(), semantic.ResultType),
			Success:  semantic.SuccessType,
			Fails:    semantic.ErrorType,
		}
		l.effectBoundaries = append(l.effectBoundaries, boundary)
		if n.Value != nil {
			attempt.Value = l.resultSuccess(n.Span(), boundary, l.expression(n.Value))
		}
		if n.Value == nil {
			resultIndex, resultExpression := lowerControlFlowBranchExpression(n.Body)
			if resultExpression == nil {
				attempt.Body = l.statements(n.Body)
				attempt.BodyResult = l.resultSuccess(n.Span(), boundary, l.unitValue(n.Span()))
			} else {
				attempt.Body = l.statements(n.Body[:resultIndex])
				attempt.BodyResult = l.resultSuccess(n.Span(), boundary, l.expression(semantic.Result))
				attempt.Body = append(attempt.Body, l.statements(n.Body[resultIndex+1:])...)
			}
		}
		l.effectBoundaries = l.effectBoundaries[:len(l.effectBoundaries)-1]
		return attempt
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
		if semantic, ok := l.checked.EnumCalls[n]; ok {
			fails := l.checked.ExpressionEffects[n]
			callBase := base
			if fails.Kind != "" && fails.Kind != types.Never {
				callBase = ir.NewExprBase(n.Span(), resultType(effectSuccessType(typ), fails))
			}
			result := &ir.EnumCall{ExprBase: callBase, EnumName: semantic.EnumName, Method: semantic.Method, Reference: l.reference(n.Callee), Fails: fails}
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
			if fails.Kind != "" && fails.Kind != types.Never {
				if boundary, ok := l.currentEffectBoundary(); ok {
					return l.effectPropagation(n.Span(), result, typ, fails, boundary)
				}
				if l.checked.UnhandledEffects[n] {
					return &ir.UnhandledEffect{ExprBase: base, Value: result, Fails: fails}
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
		fails := l.checked.ExpressionEffects[n]
		callBase := base
		if fails.Kind != "" && fails.Kind != types.Never {
			callBase = ir.NewExprBase(n.Span(), resultType(effectSuccessType(typ), fails))
		}
		result := &ir.Call{ExprBase: callBase, Callee: l.expression(n.Callee), Fails: fails}
		if codec, ok := l.checked.CodecApplications[n]; ok {
			result.Codec = lowerCodecSchema(codec.Schema)
		}
		for _, argument := range n.Arguments {
			result.Arguments = append(result.Arguments, ir.CallArgument{Name: argument.Name, Value: l.expression(argument.Value), Splat: argument.Splat})
		}
		if n.Block != nil {
			result.Block = l.expression(n.Block).(*ir.Block)
		}
		if fails.Kind != "" && fails.Kind != types.Never {
			if boundary, ok := l.currentEffectBoundary(); ok {
				return l.effectPropagation(n.Span(), result, typ, fails, boundary)
			}
			if l.checked.UnhandledEffects[n] {
				return &ir.UnhandledEffect{ExprBase: base, Value: result, Fails: fails}
			}
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
		fails := l.checked.ExpressionEffects[n]
		if fails.Kind != "" && fails.Kind != types.Never {
			raw := &ir.Call{
				ExprBase: ir.NewExprBase(n.Span(), resultType(effectSuccessType(typ), fails)),
				Callee:   member,
				Fails:    fails,
			}
			if boundary, ok := l.currentEffectBoundary(); ok {
				return l.effectPropagation(n.Span(), raw, typ, fails, boundary)
			}
			if l.checked.UnhandledEffects[n] {
				return &ir.UnhandledEffect{ExprBase: base, Value: raw, Fails: fails}
			}
			return raw
		}
		return member
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
	for _, branch := range node.Branches {
		body, branchResult, diverges := l.controlFlowBranch(branch.Body, expression)
		lowered := ir.CaseBranch{
			Base:     ir.Base{Span: branch.Span(), TrailingComment: branch.TrailingComment},
			Value:    l.expression(branch.Value),
			Body:     body,
			Result:   branchResult,
			Diverges: diverges,
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

func lowerCodecSchema(schema checker.CodecSchema) *ir.CodecSchema {
	result := &ir.CodecSchema{Type: schema.Type, Kind: schema.Kind, Module: schema.Module, RawType: schema.RawType}
	for _, value := range schema.RawValues {
		result.RawValues = append(result.RawValues, ir.EnumRawValue{Member: value.Member, Raw: value.Raw})
	}
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
		member, external := l.checked.ExternalMembers[node]
		if !external || member.Intrinsic == "" {
			return nil
		}
		_, receiver := node.(*ast.MemberExpression)
		receiver = receiver && !member.Class
		return &ir.Reference{Intrinsic: member.Intrinsic, Symbol: member.Name, ExportKind: string(member.Kind), ReceiverMethod: receiver}
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

func referenceFromBinding(binding *resolver.Binding) *ir.Reference {
	if binding == nil || binding.Import == nil {
		return nil
	}
	result := &ir.Reference{Package: binding.Import.RuntimePath(), Alias: binding.Import.Alias, Symbol: binding.Name}
	if binding.Export != nil {
		result.ExportKind = string(binding.Export.Kind)
	}
	if binding.Member != nil {
		result.ExportKind = string(binding.Member.Kind)
	}
	return result
}

func (l *lowerer) currentEffectBoundary() (effectBoundary, bool) {
	if len(l.effectBoundaries) == 0 {
		return effectBoundary{}, false
	}
	return l.effectBoundaries[len(l.effectBoundaries)-1], true
}

func effectSuccessType(typ types.Type) types.Type {
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

func (l *lowerer) resultSuccess(span token.Span, boundary effectBoundary, value ir.Expression) ir.Expression {
	return &ir.EnumConstruct{
		ExprBase:      ir.NewExprBase(span, boundary.result),
		EnumName:      "Result",
		Member:        "Ok",
		TypeArguments: []types.Type{boundary.success, boundary.fails},
		Arguments:     []ir.Expression{value},
		Reference:     resultReference(),
	}
}

func (l *lowerer) resultFailure(span token.Span, boundary effectBoundary, value ir.Expression) ir.Expression {
	return &ir.EnumConstruct{
		ExprBase:      ir.NewExprBase(span, boundary.result),
		EnumName:      "Result",
		Member:        "Err",
		TypeArguments: []types.Type{boundary.success, boundary.fails},
		Arguments:     []ir.Expression{value},
		Reference:     resultReference(),
	}
}

func (l *lowerer) effectPropagation(span token.Span, value ir.Expression, success, failure types.Type, boundary effectBoundary) ir.Expression {
	l.temporary++
	suffix := strconv.Itoa(l.temporary)
	valueName := "__trbEffectValue" + suffix
	errorName := "__trbEffectError" + suffix
	rawResult := resultType(effectSuccessType(success), failure)

	valueIdentifier := &ir.Identifier{ExprBase: ir.NewExprBase(span, effectSuccessType(success)), Name: valueName, Lexical: true, Generated: true}
	errorIdentifier := &ir.Identifier{ExprBase: ir.NewExprBase(span, failure), Name: errorName, Lexical: true, Generated: true}
	returnFailure := l.resultFailure(span, boundary, errorIdentifier)

	return &ir.Case{
		ExprBase: ir.NewExprBase(span, success),
		Value:    value,
		Branches: []ir.CaseBranch{
			{
				Value:       l.resultPattern(span, rawResult, "Ok"),
				EnumName:    "Result",
				Member:      "Ok",
				Bindings:    []ir.CaseBinding{{Name: valueName, Field: "value", Type: effectSuccessType(success), Generated: true}},
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

func lowerFailureType(ref ast.TypeRef) types.Type {
	if ref.Empty() {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	return lowerType(ref)
}
