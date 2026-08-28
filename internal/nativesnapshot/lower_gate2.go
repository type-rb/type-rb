package nativesnapshot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type gate2ValueRef struct {
	id  string
	typ types.Type
}

type gate2FunctionLowerer struct {
	program   *ir.Program
	method    *ir.Method
	sourceID  string
	methodIDs map[string]string
	registry  *aggregateRegistry
	blocks    []*Block
	current   *Block
	env       map[string]gate2ValueRef
	locals    []string
	nextValue int
	nextBlock int
}

func lowerGate2Function(program *ir.Program, method *ir.Method, sourceID string, methodIDs map[string]string, registry *aggregateRegistry) (Function, error) {
	if len(method.TypeParameters) != 0 {
		return Function{}, unsupportedV3(program, method.SourceSpan(), "generic function "+method.Name)
	}
	lowerer := &gate2FunctionLowerer{
		program: program, method: method, sourceID: sourceID, methodIDs: methodIDs,
		registry: registry, env: map[string]gate2ValueRef{},
	}
	resultType, err := lowerer.typeName(method.ReturnType, method.SourceSpan())
	if err != nil {
		return Function{}, err
	}
	parameters := make([]Parameter, 0, len(method.Parameters))
	for _, item := range method.Parameters {
		parameterType, typeErr := lowerer.typeName(item.Type, method.SourceSpan())
		if typeErr != nil {
			return Function{}, typeErr
		}
		if parameterType == "Void" || item.Default != nil || item.NamedOnly || item.Rest || item.KeywordRest {
			return Function{}, unsupportedV3(program, method.SourceSpan(), "non-required-positional parameter "+item.Name)
		}
		id := lowerer.newValue()
		parameters = append(parameters, Parameter{ID: id, Type: parameterType, Origin: lowerer.origin(method.SourceSpan())})
		lowerer.env[item.Name] = gate2ValueRef{id: id, typ: item.Type}
		lowerer.locals = append(lowerer.locals, item.Name)
	}
	entry := lowerer.newBlock(method.SourceSpan())
	lowerer.current = entry
	terminated, err := lowerer.lowerStatements(method.Body)
	if err != nil {
		return Function{}, err
	}
	if !terminated {
		if resultType != "Void" {
			return Function{}, unsupportedV3(program, method.SourceSpan(), "fallthrough from a non-Void function")
		}
		lowerer.current.Terminator = Return{Op: "return", Value: nil, Origin: lowerer.origin(method.SourceSpan())}
	}
	blocks := make([]Block, len(lowerer.blocks))
	for index, block := range lowerer.blocks {
		if block.Terminator == nil {
			return Function{}, fmt.Errorf("%s: native snapshot lowering left block %s unterminated", program.SourcePath, block.ID)
		}
		blocks[index] = *block
	}
	return Function{
		ID: functionID(program, method), Name: method.Name, Parameters: parameters,
		Result: resultType, Entry: entry.ID, Origin: lowerer.origin(method.SourceSpan()), Blocks: blocks,
	}, nil
}

func (l *gate2FunctionLowerer) typeName(typ types.Type, span token.Span) (string, error) {
	return l.registry.typeName(l.program, typ, span)
}

func (l *gate2FunctionLowerer) origin(span token.Span) Origin {
	if span.Start.Line < 1 || span.Start.Column < 1 || span.End.Line < 1 || span.End.Column < 1 {
		span = l.method.SourceSpan()
	}
	return origin(l.sourceID, span)
}

func (l *gate2FunctionLowerer) lowerStatements(statements []ir.Statement) (bool, error) {
	for _, statement := range statements {
		if l.current.Terminator != nil {
			return true, nil
		}
		terminated, err := l.lowerStatement(statement)
		if err != nil {
			return false, err
		}
		if terminated {
			return true, nil
		}
	}
	return l.current.Terminator != nil, nil
}

func (l *gate2FunctionLowerer) lowerStatement(statement ir.Statement) (bool, error) {
	switch node := statement.(type) {
	case *ir.Comment:
		return false, nil
	case *ir.Variable:
		if node.Constant || node.Value == nil {
			return false, unsupportedV3(l.program, node.SourceSpan(), "constant or uninitialized local binding")
		}
		if _, exists := l.env[node.Name]; exists {
			return false, unsupportedV3(l.program, node.SourceSpan(), "shadowed local binding "+node.Name)
		}
		value, err := l.lowerExpression(node.Value)
		if err != nil {
			return false, err
		}
		l.env[node.Name] = value
		l.locals = append(l.locals, node.Name)
		return false, nil
	case *ir.Assignment:
		identifier, ok := node.Target.(*ir.Identifier)
		if !ok || !identifier.Lexical {
			return false, unsupportedV3(l.program, node.SourceSpan(), "non-local assignment")
		}
		current, exists := l.env[identifier.Name]
		if !exists {
			return false, unsupportedV3(l.program, node.SourceSpan(), "assignment to an unavailable local")
		}
		var next gate2ValueRef
		var err error
		if node.Operator == "=" {
			next, err = l.lowerExpression(node.Value)
		} else if strings.HasSuffix(node.Operator, "=") {
			right, expressionErr := l.lowerExpression(node.Value)
			if expressionErr != nil {
				return false, expressionErr
			}
			next, err = l.emitBinary(strings.TrimSuffix(node.Operator, "="), current, right, node.SourceSpan())
		} else {
			return false, unsupportedV3(l.program, node.SourceSpan(), "assignment operator "+node.Operator)
		}
		if err != nil {
			return false, err
		}
		l.env[identifier.Name] = next
		return false, nil
	case *ir.ExpressionStatement:
		if call, ok := node.Expression.(*ir.Call); ok && l.isPuts(call) {
			return false, l.lowerPuts(call)
		}
		_, err := l.lowerExpression(node.Expression)
		return false, err
	case *ir.Return:
		var returned *string
		if node.Value != nil {
			value, err := l.lowerExpression(node.Value)
			if err != nil {
				return false, err
			}
			returned = &value.id
		}
		l.current.Terminator = Return{Op: "return", Value: returned, Origin: l.origin(node.SourceSpan())}
		return true, nil
	case *ir.If:
		return l.lowerIf(node)
	case *ir.While:
		return l.lowerWhile(node)
	case *ir.Case:
		_, terminated, err := l.lowerCase(node, false)
		return terminated, err
	case *ir.Break:
		return false, unsupportedV3(l.program, node.SourceSpan(), "break")
	case *ir.Next:
		return false, unsupportedV3(l.program, node.SourceSpan(), "next")
	default:
		return false, unsupportedV3(l.program, statement.SourceSpan(), fmt.Sprintf("statement %T", statement))
	}
}

func (l *gate2FunctionLowerer) lowerIf(node *ir.If) (bool, error) {
	if len(node.ElseIf) != 0 || node.ThenResult != nil || node.ElseResult != nil {
		return false, unsupportedV3(l.program, node.SourceSpan(), "elsif or value-producing if")
	}
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	baseEnv := cloneGate2Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	branchOrigin := l.origin(node.SourceSpan())
	thenBlock, thenEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	elseBlock, elseEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	l.current.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: thenBlock.ID, TrueArguments: gate2EnvironmentArguments(baseLocals, baseEnv),
		WhenFalse: elseBlock.ID, FalseArguments: gate2EnvironmentArguments(baseLocals, baseEnv), Origin: branchOrigin,
	}

	l.current, l.env, l.locals = thenBlock, thenEnv, append([]string(nil), baseLocals...)
	thenTerminated, err := l.lowerStatements(node.Then)
	if err != nil {
		return false, err
	}
	thenEnd, thenFinal := l.current, cloneGate2Env(l.env)

	l.current, l.env, l.locals = elseBlock, elseEnv, append([]string(nil), baseLocals...)
	elseTerminated := false
	if node.HasElse {
		elseTerminated, err = l.lowerStatements(node.Else)
		if err != nil {
			return false, err
		}
	}
	elseEnd, elseFinal := l.current, cloneGate2Env(l.env)
	if thenTerminated && elseTerminated {
		l.env, l.locals = baseEnv, baseLocals
		return true, nil
	}
	join, joinEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	if !thenTerminated {
		thenEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: gate2EnvironmentArguments(baseLocals, thenFinal), Origin: branchOrigin,
		}
	}
	if !elseTerminated {
		elseEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: gate2EnvironmentArguments(baseLocals, elseFinal), Origin: branchOrigin,
		}
	}
	l.current, l.env, l.locals = join, joinEnv, baseLocals
	return false, nil
}

func (l *gate2FunctionLowerer) lowerWhile(node *ir.While) (bool, error) {
	baseEnv := cloneGate2Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	loopOrigin := l.origin(node.SourceSpan())
	previous := l.current
	header, headerEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	body, bodyEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	done, doneEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	previous.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: gate2EnvironmentArguments(baseLocals, baseEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = header, headerEnv, append([]string(nil), baseLocals...)
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	header.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: body.ID, TrueArguments: gate2EnvironmentArguments(baseLocals, headerEnv),
		WhenFalse: done.ID, FalseArguments: gate2EnvironmentArguments(baseLocals, headerEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = body, bodyEnv, append([]string(nil), baseLocals...)
	terminated, err := l.lowerStatements(node.Body)
	if err != nil {
		return false, err
	}
	if terminated {
		return false, unsupportedV3(l.program, node.SourceSpan(), "control transfer from a Gate 2 while body")
	}
	l.current.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: gate2EnvironmentArguments(baseLocals, l.env), Origin: loopOrigin,
	}
	l.current, l.env, l.locals = done, doneEnv, baseLocals
	return false, nil
}

func (l *gate2FunctionLowerer) lowerExpression(expression ir.Expression) (gate2ValueRef, error) {
	switch node := expression.(type) {
	case *ir.Identifier:
		value, ok := l.env[node.Name]
		if !ok || !node.Lexical {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-local value "+node.Name)
		}
		return value, nil
	case *ir.Literal:
		return l.lowerLiteral(node)
	case *ir.Unary:
		return l.lowerUnary(node)
	case *ir.Binary:
		left, err := l.lowerExpression(node.Left)
		if err != nil {
			return gate2ValueRef{}, err
		}
		right, err := l.lowerExpression(node.Right)
		if err != nil {
			return gate2ValueRef{}, err
		}
		return l.emitBinary(node.Operator, left, right, node.SourceSpan())
	case *ir.Call:
		return l.lowerCall(node)
	case *ir.RecordConstruct:
		return l.lowerRecordConstruct(node)
	case *ir.Member:
		return l.lowerMember(node)
	case *ir.EnumConstruct:
		return l.lowerVariantConstruct(node)
	case *ir.Case:
		value, _, err := l.lowerCase(node, true)
		return value, err
	case *ir.Conversion:
		value, err := l.lowerExpression(node.Value)
		if err != nil {
			return gate2ValueRef{}, err
		}
		sourceType, sourceErr := l.typeName(value.typ, node.SourceSpan())
		targetType, targetErr := l.typeName(node.ExprType(), node.SourceSpan())
		if sourceErr == nil && targetErr == nil && sourceType == targetType {
			value.typ = node.ExprType()
			return value, nil
		}
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "representation-changing conversion "+string(node.Kind))
	case *ir.If:
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "value-producing if")
	default:
		return gate2ValueRef{}, unsupportedV3(l.program, expression.SourceSpan(), fmt.Sprintf("expression %T", expression))
	}
}

func (l *gate2FunctionLowerer) lowerMember(node *ir.Member) (gate2ValueRef, error) {
	if !node.Namespace {
		return l.lowerRecordProject(node)
	}
	typeID, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-enum namespace member "+node.Name)
	}
	variant, ok := gate2Variant(definition, node.Name)
	if !ok || len(variant.Fields) != 0 {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-nullary enum member "+node.Name)
	}
	id := l.newValue()
	l.emit(VariantConstruct{
		Op: "variant_construct", Result: id, Type: typeID, Variant: node.Name, Arguments: []string{},
		Origin: l.origin(node.SourceSpan()),
	})
	return gate2ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *gate2FunctionLowerer) lowerRecordConstruct(node *ir.RecordConstruct) (gate2ValueRef, error) {
	typeID, err := l.registry.typeNameWithDeclaration(l.program, node.ExprType(), node.Declaration, node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "record" || definition.Fields == nil {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-record construction "+node.ExprType().String())
	}
	values := make(map[string]gate2ValueRef, len(node.Arguments))
	for _, argument := range node.Arguments {
		if argument.Splat != "" || argument.Name == "" {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "positional or splat record construction")
		}
		value, expressionErr := l.lowerExpression(argument.Value)
		if expressionErr != nil {
			return gate2ValueRef{}, expressionErr
		}
		values[argument.Name] = value
	}
	arguments := make([]string, 0, len(*definition.Fields))
	for _, field := range *definition.Fields {
		value, exists := values[field.Name]
		if !exists {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "omitted record default for field "+field.Name)
		}
		arguments = append(arguments, value.id)
		delete(values, field.Name)
	}
	if len(values) != 0 {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "unknown record construction field")
	}
	id := l.newValue()
	l.emit(RecordConstruct{
		Op: "record_construct", Result: id, Type: typeID, Arguments: arguments,
		Origin: l.origin(node.SourceSpan()),
	})
	return gate2ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *gate2FunctionLowerer) lowerRecordProject(node *ir.Member) (gate2ValueRef, error) {
	if node.Namespace || node.Safe || node.Receiver == nil {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-record member "+node.Name)
	}
	record, err := l.lowerExpression(node.Receiver)
	if err != nil {
		return gate2ValueRef{}, err
	}
	typeID, err := l.typeName(record.typ, node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "member "+node.Name+" on "+record.typ.String())
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "record" || definition.Fields == nil || !gate2HasField(*definition.Fields, node.Name) {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-record field "+node.Name)
	}
	id := l.newValue()
	l.emit(RecordProject{
		Op: "record_project", Result: id, Type: typeID, Record: record.id, Field: node.Name,
		Origin: l.origin(node.SourceSpan()),
	})
	return gate2ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *gate2FunctionLowerer) lowerVariantConstruct(node *ir.EnumConstruct) (gate2ValueRef, error) {
	declaration := node.Declaration
	if declaration.Empty() && node.Reference != nil {
		declaration = identityDeclarationForEnumReference(node.Reference.Package, node.EnumName)
	}
	typeID, err := l.registry.typeNameWithDeclaration(l.program, node.ExprType(), declaration, node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "non-tagged enum construction "+node.ExprType().String())
	}
	variant, ok := gate2Variant(definition, node.Member)
	if !ok {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "unknown enum member "+node.Member)
	}
	values := make(map[string]gate2ValueRef, len(node.Arguments))
	for _, argument := range node.Arguments {
		if argument.Splat != "" || argument.Field == "" {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "unbound or splat enum payload argument")
		}
		value, expressionErr := l.lowerExpression(argument.Value)
		if expressionErr != nil {
			return gate2ValueRef{}, expressionErr
		}
		values[argument.Field] = value
	}
	arguments := make([]string, 0, len(variant.Fields))
	for _, field := range variant.Fields {
		value, exists := values[field.Name]
		if !exists {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "missing enum payload field "+field.Name)
		}
		arguments = append(arguments, value.id)
		delete(values, field.Name)
	}
	if len(values) != 0 {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "unknown enum payload field")
	}
	id := l.newValue()
	l.emit(VariantConstruct{
		Op: "variant_construct", Result: id, Type: typeID, Variant: node.Member, Arguments: arguments,
		Origin: l.origin(node.SourceSpan()),
	})
	return gate2ValueRef{id: id, typ: node.ExprType()}, nil
}

type gate2CaseExit struct {
	block  *Block
	env    map[string]gate2ValueRef
	result gate2ValueRef
}

func (l *gate2FunctionLowerer) lowerCase(node *ir.Case, wantValue bool) (gate2ValueRef, bool, error) {
	if node.TypeUnion || len(node.Branches) == 0 {
		return gate2ValueRef{}, false, unsupportedV3(l.program, node.SourceSpan(), "union or empty case")
	}
	if terminated, err := l.lowerStatements(node.Leading); err != nil || terminated {
		return gate2ValueRef{}, terminated, err
	}
	selector, err := l.lowerExpression(node.Value)
	if err != nil {
		return gate2ValueRef{}, false, err
	}
	typeID, err := l.typeName(selector.typ, node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, false, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return gate2ValueRef{}, false, unsupportedV3(l.program, node.SourceSpan(), "case selector "+selector.typ.String())
	}
	for _, branch := range node.Branches {
		if _, found := gate2Variant(definition, branch.Member); !found || len(branch.Alternatives) != 0 || branch.TypePattern {
			return gate2ValueRef{}, false, unsupportedV3(l.program, branch.SourceSpan(), "non-enum case pattern "+branch.Member)
		}
	}

	baseEnv := cloneGate2Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	selectorName := fmt.Sprintf("\x00case-selector-%d", l.nextBlock)
	caseEnv := cloneGate2Env(baseEnv)
	caseEnv[selectorName] = selector
	caseLocals := append(append([]string(nil), baseLocals...), selectorName)
	branchBlocks := make([]*Block, len(node.Branches))
	branchEnvs := make([]map[string]gate2ValueRef, len(node.Branches))
	for index, branch := range node.Branches {
		branchBlocks[index], branchEnvs[index] = l.newBlockWithLocals(branch.SourceSpan(), caseLocals, caseEnv)
	}
	var elseBlock *Block
	var elseEnv map[string]gate2ValueRef
	if node.HasElse {
		elseBlock, elseEnv = l.newBlockWithLocals(node.SourceSpan(), caseLocals, caseEnv)
	}

	dispatch := l.current
	dispatchEnv := caseEnv
	caseOrigin := l.origin(node.SourceSpan())
	for index, branch := range node.Branches {
		last := index == len(node.Branches)-1
		if last && !node.HasElse {
			dispatch.Terminator = Jump{
				Op: "jump", Target: branchBlocks[index].ID,
				Arguments: gate2EnvironmentArguments(caseLocals, dispatchEnv), Origin: caseOrigin,
			}
			break
		}
		l.current = dispatch
		testID := l.newValue()
		l.emit(VariantTest{
			Op: "variant_test", Result: testID, Type: typeID, Value: dispatchEnv[selectorName].id, Variant: branch.Member,
			Origin: l.origin(branch.SourceSpan()),
		})
		var falseBlock *Block
		var falseEnv map[string]gate2ValueRef
		if last {
			falseBlock, falseEnv = elseBlock, elseEnv
		} else {
			falseBlock, falseEnv = l.newBlockWithLocals(node.SourceSpan(), caseLocals, caseEnv)
		}
		dispatch.Terminator = Branch{
			Op: "branch", Condition: testID,
			WhenTrue: branchBlocks[index].ID, TrueArguments: gate2EnvironmentArguments(caseLocals, dispatchEnv),
			WhenFalse: falseBlock.ID, FalseArguments: gate2EnvironmentArguments(caseLocals, dispatchEnv), Origin: caseOrigin,
		}
		dispatch, dispatchEnv = falseBlock, falseEnv
	}

	exits := []gate2CaseExit{}
	for index, branch := range node.Branches {
		l.current, l.env, l.locals = branchBlocks[index], branchEnvs[index], append([]string(nil), caseLocals...)
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			if _, exists := l.env[binding.Name]; exists {
				return gate2ValueRef{}, false, unsupportedV3(l.program, branch.SourceSpan(), "shadowed case binding "+binding.Name)
			}
			fieldType, typeErr := l.typeName(binding.Type, branch.SourceSpan())
			if typeErr != nil || fieldType == "Void" {
				if typeErr != nil {
					return gate2ValueRef{}, false, typeErr
				}
				return gate2ValueRef{}, false, unsupportedV3(l.program, branch.SourceSpan(), "Void enum binding "+binding.Name)
			}
			id := l.newValue()
			l.emit(VariantProject{
				Op: "variant_project", Result: id, Type: typeID, Value: l.env[selectorName].id,
				Variant: branch.Member, Field: binding.Field, Origin: l.origin(branch.SourceSpan()),
			})
			l.env[binding.Name] = gate2ValueRef{id: id, typ: binding.Type}
			l.locals = append(l.locals, binding.Name)
		}
		terminated, bodyErr := l.lowerStatements(branch.Body)
		if bodyErr != nil {
			return gate2ValueRef{}, false, bodyErr
		}
		if terminated {
			continue
		}
		exit := gate2CaseExit{block: l.current, env: cloneGate2Env(l.env)}
		if wantValue {
			if branch.Result == nil {
				return gate2ValueRef{}, false, unsupportedV3(l.program, branch.SourceSpan(), "case branch without a value")
			}
			exit.result, err = l.lowerExpression(branch.Result)
			if err != nil {
				return gate2ValueRef{}, false, err
			}
			exit.block = l.current
			exit.env = cloneGate2Env(l.env)
		}
		exits = append(exits, exit)
	}
	if node.HasElse {
		l.current, l.env, l.locals = elseBlock, elseEnv, append([]string(nil), caseLocals...)
		terminated, bodyErr := l.lowerStatements(node.Else)
		if bodyErr != nil {
			return gate2ValueRef{}, false, bodyErr
		}
		if !terminated {
			exit := gate2CaseExit{block: l.current, env: cloneGate2Env(l.env)}
			if wantValue {
				if node.ElseResult == nil {
					return gate2ValueRef{}, false, unsupportedV3(l.program, node.SourceSpan(), "case else without a value")
				}
				exit.result, err = l.lowerExpression(node.ElseResult)
				if err != nil {
					return gate2ValueRef{}, false, err
				}
				exit.block = l.current
				exit.env = cloneGate2Env(l.env)
			}
			exits = append(exits, exit)
		}
	}
	if len(exits) == 0 {
		l.env, l.locals = baseEnv, baseLocals
		return gate2ValueRef{}, true, nil
	}

	join := l.newBlock(node.SourceSpan())
	joinEnv := cloneGate2Env(baseEnv)
	result := gate2ValueRef{}
	if wantValue {
		resultType, typeErr := l.typeName(node.ExprType(), node.SourceSpan())
		if typeErr != nil || resultType == "Void" {
			if typeErr != nil {
				return gate2ValueRef{}, false, typeErr
			}
			return gate2ValueRef{}, false, unsupportedV3(l.program, node.SourceSpan(), "Void case expression")
		}
		result = gate2ValueRef{id: l.newValue(), typ: node.ExprType()}
		join.Parameters = append(join.Parameters, Parameter{ID: result.id, Type: resultType, Origin: caseOrigin})
	}
	for _, name := range baseLocals {
		value := baseEnv[name]
		typeName, typeErr := l.typeName(value.typ, node.SourceSpan())
		if typeErr != nil || typeName == "Void" {
			continue
		}
		id := l.newValue()
		join.Parameters = append(join.Parameters, Parameter{ID: id, Type: typeName, Origin: caseOrigin})
		joinEnv[name] = gate2ValueRef{id: id, typ: value.typ}
	}
	for _, exit := range exits {
		arguments := []string{}
		if wantValue {
			arguments = append(arguments, exit.result.id)
		}
		arguments = append(arguments, gate2EnvironmentArguments(baseLocals, exit.env)...)
		exit.block.Terminator = Jump{Op: "jump", Target: join.ID, Arguments: arguments, Origin: caseOrigin}
	}
	l.current, l.env, l.locals = join, joinEnv, baseLocals
	return result, false, nil
}

func gate2HasField(fields []Field, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

func gate2Variant(definition TypeDefinition, name string) (Variant, bool) {
	if definition.Variants == nil {
		return Variant{}, false
	}
	for _, variant := range *definition.Variants {
		if variant.Name == name {
			return variant, true
		}
	}
	return Variant{}, false
}

func identityDeclarationForEnumReference(module, name string) identity.Declaration {
	return identity.Declaration{Module: module, Name: name, Kind: identity.Enum}
}

func (l *gate2FunctionLowerer) lowerLiteral(node *ir.Literal) (gate2ValueRef, error) {
	id := l.newValue()
	at := l.origin(node.SourceSpan())
	switch node.Kind {
	case "boolean":
		value, err := strconv.ParseBool(node.Raw)
		if err != nil {
			return gate2ValueRef{}, err
		}
		l.emit(BooleanLiteral{Op: "boolean_literal", Result: id, Value: value, Origin: at})
	case "integer":
		value, ok := types.ParsePortableIntegerLiteral(node.Raw)
		if !ok {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "Integer literal "+node.Raw)
		}
		l.emit(IntegerLiteral{Op: "integer_literal", Result: id, Value: value, Origin: at})
	case "float":
		value, ok := types.ParsePortableFloatLiteral(node.Raw)
		if !ok {
			return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "Float literal "+node.Raw)
		}
		l.emit(FloatLiteral{Op: "float_literal", Result: id, Value: value, Origin: at})
	default:
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), node.Kind+" literal expression")
	}
	return gate2ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *gate2FunctionLowerer) lowerUnary(node *ir.Unary) (gate2ValueRef, error) {
	operand, err := l.lowerExpression(node.Operand)
	if err != nil {
		return gate2ValueRef{}, err
	}
	if node.Operator == "+" {
		return operand, nil
	}
	if node.Operator == "!" && node.ExprType().Kind == types.Bool {
		id := l.newValue()
		l.emit(BooleanNot{Op: "boolean_not", Result: id, Value: operand.id, Origin: l.origin(node.SourceSpan())})
		return gate2ValueRef{id: id, typ: node.ExprType()}, nil
	}
	if node.Operator == "-" && (node.ExprType().Kind == types.Int || node.ExprType().Kind == types.Float) {
		zeroID := l.newValue()
		at := l.origin(node.SourceSpan())
		if node.ExprType().Kind == types.Int {
			l.emit(IntegerLiteral{Op: "integer_literal", Result: zeroID, Value: 0, Origin: at})
		} else {
			l.emit(FloatLiteral{Op: "float_literal", Result: zeroID, Value: 0, Origin: at})
		}
		return l.emitBinary("-", gate2ValueRef{id: zeroID, typ: node.ExprType()}, operand, node.SourceSpan())
	}
	return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "unary operator "+node.Operator)
}

func (l *gate2FunctionLowerer) emitBinary(operator string, left, right gate2ValueRef, span token.Span) (gate2ValueRef, error) {
	resultType := left.typ
	if operator == "==" || operator == "!=" || operator == "<" || operator == "<=" || operator == ">" || operator == ">=" {
		resultType = types.FromName("Boolean")
	}
	typeName, supported := scalarTypeName(left.typ)
	if !supported || typeName == "Void" {
		return gate2ValueRef{}, unsupportedV3(l.program, span, "binary operand type "+left.typ.String())
	}
	id := l.newValue()
	at := l.origin(span)
	if operatorName, ok := gate2ComparisonOperator(operator); ok {
		op := "integer_compare"
		if typeName == "Float" {
			op = "float_compare"
		} else if typeName != "Integer" {
			return gate2ValueRef{}, unsupportedV3(l.program, span, "comparison of "+typeName)
		}
		l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
		return gate2ValueRef{id: id, typ: resultType}, nil
	}
	operatorName, ok := gate2ArithmeticOperator(operator)
	if !ok {
		return gate2ValueRef{}, unsupportedV3(l.program, span, "binary operator "+operator)
	}
	op := "integer_binary"
	if typeName == "Float" {
		if operator == "%" || operator == "**" {
			return gate2ValueRef{}, unsupportedV3(l.program, span, "Gate 2 Float operator "+operator)
		}
		op = "float_binary"
	} else if typeName != "Integer" {
		return gate2ValueRef{}, unsupportedV3(l.program, span, "arithmetic on "+typeName)
	}
	l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
	return gate2ValueRef{id: id, typ: resultType}, nil
}

func (l *gate2FunctionLowerer) lowerCall(node *ir.Call) (gate2ValueRef, error) {
	if l.isPuts(node) {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "puts() in a value position")
	}
	callee, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "indirect call")
	}
	function := l.callFunctionID(callee)
	if function == "" {
		return gate2ValueRef{}, unsupportedV3(l.program, node.SourceSpan(), "call to "+callee.Name)
	}
	arguments := make([]string, 0, len(node.Arguments))
	for _, argument := range node.Arguments {
		value, err := l.lowerExpression(argument.Value)
		if err != nil {
			return gate2ValueRef{}, err
		}
		arguments = append(arguments, value.id)
	}
	resultType, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return gate2ValueRef{}, err
	}
	var result *string
	ref := gate2ValueRef{typ: node.ExprType()}
	if resultType != "Void" {
		ref.id = l.newValue()
		result = &ref.id
	}
	l.emit(Call{Op: "call", Result: result, Function: function, Arguments: arguments, Origin: l.origin(node.SourceSpan())})
	return ref, nil
}

func (l *gate2FunctionLowerer) lowerPuts(node *ir.Call) error {
	if len(node.Arguments) != 1 {
		return unsupportedV3(l.program, node.SourceSpan(), "puts() arity")
	}
	literal, ok := node.Arguments[0].Value.(*ir.Literal)
	if !ok || literal.Kind != "string" {
		return unsupportedV3(l.program, node.SourceSpan(), "dynamic puts() output")
	}
	value, err := strconv.Unquote(literal.Raw)
	if err != nil {
		return fmt.Errorf("%s: decode TypeRB String literal: %w", l.program.SourcePath, err)
	}
	l.emit(WriteStatic{Op: "write_static", Value: value + "\n", Origin: l.origin(node.SourceSpan())})
	return nil
}

func (l *gate2FunctionLowerer) isPuts(node *ir.Call) bool {
	identifier, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return false
	}
	if identifier.Reference != nil && identifier.Reference.Intrinsic == "trb.std.io.puts" {
		return true
	}
	return identifier.Name == "puts" && identifier.Reference != nil && identifier.Reference.Intrinsic != ""
}

func (l *gate2FunctionLowerer) callFunctionID(identifier *ir.Identifier) string {
	if id := l.methodIDs[identifier.Declaration.Key()]; id != "" {
		return id
	}
	if identifier.Reference != nil {
		if id := l.methodIDs[identifier.Reference.Declaration.Key()]; id != "" {
			return id
		}
	}
	return l.methodIDs["name:"+l.program.ModulePath+"#"+identifier.Name]
}

func (l *gate2FunctionLowerer) newBlock(span token.Span) *Block {
	block := &Block{
		ID: fmt.Sprintf("b%d", l.nextBlock), Parameters: []Parameter{},
		Origin: l.origin(span), Instructions: []any{},
	}
	l.nextBlock++
	l.blocks = append(l.blocks, block)
	return block
}

func (l *gate2FunctionLowerer) newBlockWithLocals(span token.Span, names []string, template map[string]gate2ValueRef) (*Block, map[string]gate2ValueRef) {
	block := l.newBlock(span)
	environment := cloneGate2Env(template)
	for _, name := range names {
		value := template[name]
		typeName, err := l.typeName(value.typ, span)
		if err != nil || typeName == "Void" {
			// Locals have already been validated when their values were emitted.
			// A Void local cannot be created by checked TypeRB source.
			continue
		}
		id := l.newValue()
		block.Parameters = append(block.Parameters, Parameter{ID: id, Type: typeName, Origin: l.origin(span)})
		environment[name] = gate2ValueRef{id: id, typ: value.typ}
	}
	return block, environment
}

func (l *gate2FunctionLowerer) newValue() string {
	id := fmt.Sprintf("v%d", l.nextValue)
	l.nextValue++
	return id
}

func (l *gate2FunctionLowerer) emit(instruction any) {
	l.current.Instructions = append(l.current.Instructions, instruction)
}

func cloneGate2Env(source map[string]gate2ValueRef) map[string]gate2ValueRef {
	result := make(map[string]gate2ValueRef, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func gate2EnvironmentArguments(names []string, environment map[string]gate2ValueRef) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, environment[name].id)
	}
	return result
}

func gate2ArithmeticOperator(operator string) (string, bool) {
	switch operator {
	case "+":
		return "add", true
	case "-":
		return "subtract", true
	case "*":
		return "multiply", true
	case "/":
		return "divide", true
	case "%":
		return "remainder", true
	case "**":
		return "power", true
	default:
		return "", false
	}
}

func gate2ComparisonOperator(operator string) (string, bool) {
	switch operator {
	case "==":
		return "equal", true
	case "!=":
		return "not_equal", true
	case "<":
		return "less_than", true
	case "<=":
		return "less_than_or_equal", true
	case ">":
		return "greater_than", true
	case ">=":
		return "greater_than_or_equal", true
	default:
		return "", false
	}
}
