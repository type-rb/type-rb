package bootstrapsnapshot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type v3ValueRef struct {
	id  string
	typ types.Type
}

type v3FunctionLowerer struct {
	version   int
	program   *ir.Program
	method    *ir.Method
	sourceID  string
	methodIDs map[string]string
	registry  *aggregateRegistry
	builder   *v4Builder
	ownerID   string
	captures  map[string]bool
	blocks    []*Block
	current   *Block
	env       map[string]v3ValueRef
	locals    []string
	nextValue int
	nextBlock int
}

type v4CaptureInput struct {
	name string
	typ  types.Type
	span token.Span
}

func lowerV3Function(program *ir.Program, method *ir.Method, sourceID string, methodIDs map[string]string, registry *aggregateRegistry) (Function, error) {
	lowerer := &v3FunctionLowerer{
		version: Version3, program: program, method: method, sourceID: sourceID, methodIDs: methodIDs,
		registry: registry, ownerID: functionID(program, method), env: map[string]v3ValueRef{}, captures: map[string]bool{},
	}
	result, _, err := lowerer.lowerFunction(functionID(program, method), nil)
	return result, err
}

func (l *v3FunctionLowerer) lowerFunction(id string, captureInputs []v4CaptureInput) (Function, []Parameter, error) {
	program := l.program
	method := l.method
	if len(method.TypeParameters) != 0 {
		return Function{}, nil, l.unsupported(method.SourceSpan(), "generic function "+method.Name)
	}
	resultType, err := l.typeName(method.ReturnType, method.SourceSpan())
	if err != nil {
		return Function{}, nil, err
	}
	captures := make([]Parameter, 0, len(captureInputs))
	for _, item := range captureInputs {
		captureType, typeErr := l.typeName(item.typ, item.span)
		if typeErr != nil {
			return Function{}, nil, typeErr
		}
		if captureType == "Void" {
			return Function{}, nil, l.unsupported(item.span, "Void lexical capture "+item.name)
		}
		captureID := l.newValue()
		captures = append(captures, Parameter{ID: captureID, Type: captureType, Origin: l.origin(item.span)})
		l.env[item.name] = v3ValueRef{id: captureID, typ: item.typ}
		l.locals = append(l.locals, item.name)
		l.captures[item.name] = true
	}
	parameters := make([]Parameter, 0, len(method.Parameters))
	for _, item := range method.Parameters {
		parameterType, typeErr := l.typeName(item.Type, method.SourceSpan())
		if typeErr != nil {
			return Function{}, nil, typeErr
		}
		if parameterType == "Void" || item.Default != nil || item.NamedOnly || item.Rest || item.KeywordRest {
			return Function{}, nil, l.unsupported(method.SourceSpan(), "non-required-positional parameter "+item.Name)
		}
		parameterID := l.newValue()
		parameters = append(parameters, Parameter{ID: parameterID, Type: parameterType, Origin: l.origin(method.SourceSpan())})
		l.env[item.Name] = v3ValueRef{id: parameterID, typ: item.Type}
		l.locals = append(l.locals, item.Name)
	}
	entry := l.newBlock(method.SourceSpan())
	l.current = entry
	terminated, err := l.lowerStatements(method.Body)
	if err != nil {
		return Function{}, nil, err
	}
	if !terminated {
		if resultType != "Void" {
			return Function{}, nil, l.unsupported(method.SourceSpan(), "fallthrough from a non-Void function")
		}
		l.current.Terminator = Return{Op: "return", Value: nil, Origin: l.origin(method.SourceSpan())}
	}
	blocks := make([]Block, len(l.blocks))
	for index, block := range l.blocks {
		if block.Terminator == nil {
			return Function{}, nil, fmt.Errorf("%s: bootstrap snapshot lowering left block %s unterminated", program.SourcePath, block.ID)
		}
		blocks[index] = *block
	}
	return Function{
		ID: id, Name: method.Name, Parameters: parameters,
		Result: resultType, Entry: entry.ID, Origin: l.origin(method.SourceSpan()), Blocks: blocks,
	}, captures, nil
}

func (l *v3FunctionLowerer) typeName(typ types.Type, span token.Span) (string, error) {
	return l.registry.typeName(l.program, typ, span)
}

func (l *v3FunctionLowerer) origin(span token.Span) Origin {
	if span.Start.Line < 1 || span.Start.Column < 1 || span.End.Line < 1 || span.End.Column < 1 {
		span = l.method.SourceSpan()
	}
	return origin(l.sourceID, span)
}

func (l *v3FunctionLowerer) unsupported(span token.Span, feature string) error {
	return &UnsupportedError{Path: l.program.SourcePath, Span: span, Feature: feature, Version: l.version}
}

func (l *v3FunctionLowerer) lowerStatements(statements []ir.Statement) (bool, error) {
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

func (l *v3FunctionLowerer) lowerStatement(statement ir.Statement) (bool, error) {
	switch node := statement.(type) {
	case *ir.Comment:
		return false, nil
	case *ir.Variable:
		if node.Constant || node.Value == nil {
			return false, l.unsupported(node.SourceSpan(), "constant or uninitialized local binding")
		}
		if _, exists := l.env[node.Name]; exists {
			return false, l.unsupported(node.SourceSpan(), "shadowed local binding "+node.Name)
		}
		value, err := l.lowerExpression(node.Value)
		if err != nil {
			return false, err
		}
		l.env[node.Name] = value
		l.locals = append(l.locals, node.Name)
		return false, nil
	case *ir.Assignment:
		if index, ok := node.Target.(*ir.Index); ok && l.version >= Version4 {
			if node.Operator != "=" {
				return false, l.unsupported(node.SourceSpan(), "indexed assignment operator "+node.Operator)
			}
			return false, l.lowerArraySet(index, node.Value, node.SourceSpan())
		}
		identifier, ok := node.Target.(*ir.Identifier)
		if !ok || !identifier.Lexical {
			return false, l.unsupported(node.SourceSpan(), "non-local assignment")
		}
		if l.captures[identifier.Name] {
			return false, l.unsupported(node.SourceSpan(), "assignment to captured binding "+identifier.Name)
		}
		current, exists := l.env[identifier.Name]
		if !exists {
			return false, l.unsupported(node.SourceSpan(), "assignment to an unavailable local")
		}
		var next v3ValueRef
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
			return false, l.unsupported(node.SourceSpan(), "assignment operator "+node.Operator)
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
		return false, l.unsupported(node.SourceSpan(), "break")
	case *ir.Next:
		return false, l.unsupported(node.SourceSpan(), "next")
	default:
		return false, l.unsupported(statement.SourceSpan(), fmt.Sprintf("statement %T", statement))
	}
}

func (l *v3FunctionLowerer) lowerIf(node *ir.If) (bool, error) {
	if len(node.ElseIf) != 0 || node.ThenResult != nil || node.ElseResult != nil {
		return false, l.unsupported(node.SourceSpan(), "elsif or value-producing if")
	}
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	baseEnv := cloneV3Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	branchOrigin := l.origin(node.SourceSpan())
	thenBlock, thenEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	elseBlock, elseEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	l.current.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: thenBlock.ID, TrueArguments: v3EnvironmentArguments(baseLocals, baseEnv),
		WhenFalse: elseBlock.ID, FalseArguments: v3EnvironmentArguments(baseLocals, baseEnv), Origin: branchOrigin,
	}

	l.current, l.env, l.locals = thenBlock, thenEnv, append([]string(nil), baseLocals...)
	thenTerminated, err := l.lowerStatements(node.Then)
	if err != nil {
		return false, err
	}
	thenEnd, thenFinal := l.current, cloneV3Env(l.env)

	l.current, l.env, l.locals = elseBlock, elseEnv, append([]string(nil), baseLocals...)
	elseTerminated := false
	if node.HasElse {
		elseTerminated, err = l.lowerStatements(node.Else)
		if err != nil {
			return false, err
		}
	}
	elseEnd, elseFinal := l.current, cloneV3Env(l.env)
	if thenTerminated && elseTerminated {
		l.env, l.locals = baseEnv, baseLocals
		return true, nil
	}
	join, joinEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	if !thenTerminated {
		thenEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: v3EnvironmentArguments(baseLocals, thenFinal), Origin: branchOrigin,
		}
	}
	if !elseTerminated {
		elseEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: v3EnvironmentArguments(baseLocals, elseFinal), Origin: branchOrigin,
		}
	}
	l.current, l.env, l.locals = join, joinEnv, baseLocals
	return false, nil
}

func (l *v3FunctionLowerer) lowerWhile(node *ir.While) (bool, error) {
	baseEnv := cloneV3Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	loopOrigin := l.origin(node.SourceSpan())
	previous := l.current
	header, headerEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	body, bodyEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	done, doneEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	previous.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: v3EnvironmentArguments(baseLocals, baseEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = header, headerEnv, append([]string(nil), baseLocals...)
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	header.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: body.ID, TrueArguments: v3EnvironmentArguments(baseLocals, headerEnv),
		WhenFalse: done.ID, FalseArguments: v3EnvironmentArguments(baseLocals, headerEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = body, bodyEnv, append([]string(nil), baseLocals...)
	terminated, err := l.lowerStatements(node.Body)
	if err != nil {
		return false, err
	}
	if terminated {
		return false, l.unsupported(node.SourceSpan(), "control transfer from a while body")
	}
	l.current.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: v3EnvironmentArguments(baseLocals, l.env), Origin: loopOrigin,
	}
	l.current, l.env, l.locals = done, doneEnv, baseLocals
	return false, nil
}

func (l *v3FunctionLowerer) lowerExpression(expression ir.Expression) (v3ValueRef, error) {
	switch node := expression.(type) {
	case *ir.Identifier:
		value, ok := l.env[node.Name]
		if ok && node.Lexical {
			return value, nil
		}
		if l.version >= Version4 && node.ExprType().Kind == types.Function {
			return l.lowerFunctionValue(node)
		}
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-local value "+node.Name)
	case *ir.Literal:
		return l.lowerLiteral(node)
	case *ir.Unary:
		return l.lowerUnary(node)
	case *ir.Binary:
		left, err := l.lowerExpression(node.Left)
		if err != nil {
			return v3ValueRef{}, err
		}
		right, err := l.lowerExpression(node.Right)
		if err != nil {
			return v3ValueRef{}, err
		}
		return l.emitBinary(node.Operator, left, right, node.SourceSpan())
	case *ir.Array:
		if l.version >= Version4 {
			return l.lowerArray(node)
		}
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "Array literal expression")
	case *ir.Index:
		if l.version >= Version4 {
			return l.lowerIndex(node)
		}
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "index expression")
	case *ir.Lambda:
		if l.version >= Version4 {
			return l.lowerLambda(node)
		}
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "lambda expression")
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
			return v3ValueRef{}, err
		}
		sourceType, sourceErr := l.typeName(value.typ, node.SourceSpan())
		targetType, targetErr := l.typeName(node.ExprType(), node.SourceSpan())
		if sourceErr == nil && targetErr == nil && sourceType == targetType {
			value.typ = node.ExprType()
			return value, nil
		}
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "representation-changing conversion "+string(node.Kind))
	case *ir.If:
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "value-producing if")
	default:
		return v3ValueRef{}, l.unsupported(expression.SourceSpan(), fmt.Sprintf("expression %T", expression))
	}
}

func (l *v3FunctionLowerer) lowerMember(node *ir.Member) (v3ValueRef, error) {
	if !node.Namespace {
		return l.lowerRecordProject(node)
	}
	typeID, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-enum namespace member "+node.Name)
	}
	variant, ok := v3Variant(definition, node.Name)
	if !ok || len(variant.Fields) != 0 {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-nullary enum member "+node.Name)
	}
	id := l.newValue()
	l.emit(VariantConstruct{
		Op: "variant_construct", Result: id, Type: typeID, Variant: node.Name, Arguments: []string{},
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerRecordConstruct(node *ir.RecordConstruct) (v3ValueRef, error) {
	typeID, err := l.registry.typeNameWithDeclaration(l.program, node.ExprType(), node.Declaration, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "record" || definition.Fields == nil {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-record construction "+node.ExprType().String())
	}
	values := make(map[string]v3ValueRef, len(node.Arguments))
	for _, argument := range node.Arguments {
		if argument.Splat != "" || argument.Name == "" {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "positional or splat record construction")
		}
		value, expressionErr := l.lowerExpression(argument.Value)
		if expressionErr != nil {
			return v3ValueRef{}, expressionErr
		}
		values[argument.Name] = value
	}
	arguments := make([]string, 0, len(*definition.Fields))
	for _, field := range *definition.Fields {
		value, exists := values[field.Name]
		if !exists {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "omitted record default for field "+field.Name)
		}
		arguments = append(arguments, value.id)
		delete(values, field.Name)
	}
	if len(values) != 0 {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "unknown record construction field")
	}
	id := l.newValue()
	l.emit(RecordConstruct{
		Op: "record_construct", Result: id, Type: typeID, Arguments: arguments,
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerRecordProject(node *ir.Member) (v3ValueRef, error) {
	if node.Namespace || node.Safe || node.Receiver == nil {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-record member "+node.Name)
	}
	record, err := l.lowerExpression(node.Receiver)
	if err != nil {
		return v3ValueRef{}, err
	}
	typeID, err := l.typeName(record.typ, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "member "+node.Name+" on "+record.typ.String())
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "record" || definition.Fields == nil || !v3HasField(*definition.Fields, node.Name) {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-record field "+node.Name)
	}
	id := l.newValue()
	l.emit(RecordProject{
		Op: "record_project", Result: id, Type: typeID, Record: record.id, Field: node.Name,
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerVariantConstruct(node *ir.EnumConstruct) (v3ValueRef, error) {
	declaration := node.Declaration
	if declaration.Empty() && node.Reference != nil {
		declaration = identityDeclarationForEnumReference(node.Reference.Package, node.EnumName)
	}
	typeID, err := l.registry.typeNameWithDeclaration(l.program, node.ExprType(), declaration, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "non-tagged enum construction "+node.ExprType().String())
	}
	variant, ok := v3Variant(definition, node.Member)
	if !ok {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "unknown enum member "+node.Member)
	}
	values := make(map[string]v3ValueRef, len(node.Arguments))
	for _, argument := range node.Arguments {
		if argument.Splat != "" || argument.Field == "" {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "unbound or splat enum payload argument")
		}
		value, expressionErr := l.lowerExpression(argument.Value)
		if expressionErr != nil {
			return v3ValueRef{}, expressionErr
		}
		values[argument.Field] = value
	}
	arguments := make([]string, 0, len(variant.Fields))
	for _, field := range variant.Fields {
		value, exists := values[field.Name]
		if !exists {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "missing enum payload field "+field.Name)
		}
		arguments = append(arguments, value.id)
		delete(values, field.Name)
	}
	if len(values) != 0 {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "unknown enum payload field")
	}
	id := l.newValue()
	l.emit(VariantConstruct{
		Op: "variant_construct", Result: id, Type: typeID, Variant: node.Member, Arguments: arguments,
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

type v3CaseExit struct {
	block  *Block
	env    map[string]v3ValueRef
	result v3ValueRef
}

func (l *v3FunctionLowerer) lowerCase(node *ir.Case, wantValue bool) (v3ValueRef, bool, error) {
	if node.TypeUnion || len(node.Branches) == 0 {
		return v3ValueRef{}, false, l.unsupported(node.SourceSpan(), "union or empty case")
	}
	if terminated, err := l.lowerStatements(node.Leading); err != nil || terminated {
		return v3ValueRef{}, terminated, err
	}
	selector, err := l.lowerExpression(node.Value)
	if err != nil {
		return v3ValueRef{}, false, err
	}
	typeID, err := l.typeName(selector.typ, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, false, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "tagged" {
		return v3ValueRef{}, false, l.unsupported(node.SourceSpan(), "case selector "+selector.typ.String())
	}
	for _, branch := range node.Branches {
		if _, found := v3Variant(definition, branch.Member); !found || len(branch.Alternatives) != 0 || branch.TypePattern {
			return v3ValueRef{}, false, l.unsupported(branch.SourceSpan(), "non-enum case pattern "+branch.Member)
		}
	}

	baseEnv := cloneV3Env(l.env)
	baseLocals := append([]string(nil), l.locals...)
	selectorName := fmt.Sprintf("\x00case-selector-%d", l.nextBlock)
	caseEnv := cloneV3Env(baseEnv)
	caseEnv[selectorName] = selector
	caseLocals := append(append([]string(nil), baseLocals...), selectorName)
	branchBlocks := make([]*Block, len(node.Branches))
	branchEnvs := make([]map[string]v3ValueRef, len(node.Branches))
	for index, branch := range node.Branches {
		branchBlocks[index], branchEnvs[index] = l.newBlockWithLocals(branch.SourceSpan(), caseLocals, caseEnv)
	}
	var elseBlock *Block
	var elseEnv map[string]v3ValueRef
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
				Arguments: v3EnvironmentArguments(caseLocals, dispatchEnv), Origin: caseOrigin,
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
		var falseEnv map[string]v3ValueRef
		if last {
			falseBlock, falseEnv = elseBlock, elseEnv
		} else {
			falseBlock, falseEnv = l.newBlockWithLocals(node.SourceSpan(), caseLocals, caseEnv)
		}
		dispatch.Terminator = Branch{
			Op: "branch", Condition: testID,
			WhenTrue: branchBlocks[index].ID, TrueArguments: v3EnvironmentArguments(caseLocals, dispatchEnv),
			WhenFalse: falseBlock.ID, FalseArguments: v3EnvironmentArguments(caseLocals, dispatchEnv), Origin: caseOrigin,
		}
		dispatch, dispatchEnv = falseBlock, falseEnv
	}

	exits := []v3CaseExit{}
	for index, branch := range node.Branches {
		l.current, l.env, l.locals = branchBlocks[index], branchEnvs[index], append([]string(nil), caseLocals...)
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			if _, exists := l.env[binding.Name]; exists {
				return v3ValueRef{}, false, l.unsupported(branch.SourceSpan(), "shadowed case binding "+binding.Name)
			}
			fieldType, typeErr := l.typeName(binding.Type, branch.SourceSpan())
			if typeErr != nil || fieldType == "Void" {
				if typeErr != nil {
					return v3ValueRef{}, false, typeErr
				}
				return v3ValueRef{}, false, l.unsupported(branch.SourceSpan(), "Void enum binding "+binding.Name)
			}
			id := l.newValue()
			l.emit(VariantProject{
				Op: "variant_project", Result: id, Type: typeID, Value: l.env[selectorName].id,
				Variant: branch.Member, Field: binding.Field, Origin: l.origin(branch.SourceSpan()),
			})
			l.env[binding.Name] = v3ValueRef{id: id, typ: binding.Type}
			l.locals = append(l.locals, binding.Name)
		}
		terminated, bodyErr := l.lowerStatements(branch.Body)
		if bodyErr != nil {
			return v3ValueRef{}, false, bodyErr
		}
		if terminated {
			continue
		}
		exit := v3CaseExit{block: l.current, env: cloneV3Env(l.env)}
		if wantValue {
			if branch.Result == nil {
				return v3ValueRef{}, false, l.unsupported(branch.SourceSpan(), "case branch without a value")
			}
			exit.result, err = l.lowerExpression(branch.Result)
			if err != nil {
				return v3ValueRef{}, false, err
			}
			exit.block = l.current
			exit.env = cloneV3Env(l.env)
		}
		exits = append(exits, exit)
	}
	if node.HasElse {
		l.current, l.env, l.locals = elseBlock, elseEnv, append([]string(nil), caseLocals...)
		terminated, bodyErr := l.lowerStatements(node.Else)
		if bodyErr != nil {
			return v3ValueRef{}, false, bodyErr
		}
		if !terminated {
			exit := v3CaseExit{block: l.current, env: cloneV3Env(l.env)}
			if wantValue {
				if node.ElseResult == nil {
					return v3ValueRef{}, false, l.unsupported(node.SourceSpan(), "case else without a value")
				}
				exit.result, err = l.lowerExpression(node.ElseResult)
				if err != nil {
					return v3ValueRef{}, false, err
				}
				exit.block = l.current
				exit.env = cloneV3Env(l.env)
			}
			exits = append(exits, exit)
		}
	}
	if len(exits) == 0 {
		l.env, l.locals = baseEnv, baseLocals
		return v3ValueRef{}, true, nil
	}

	join := l.newBlock(node.SourceSpan())
	joinEnv := cloneV3Env(baseEnv)
	result := v3ValueRef{}
	if wantValue {
		resultType, typeErr := l.typeName(node.ExprType(), node.SourceSpan())
		if typeErr != nil || resultType == "Void" {
			if typeErr != nil {
				return v3ValueRef{}, false, typeErr
			}
			return v3ValueRef{}, false, l.unsupported(node.SourceSpan(), "Void case expression")
		}
		result = v3ValueRef{id: l.newValue(), typ: node.ExprType()}
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
		joinEnv[name] = v3ValueRef{id: id, typ: value.typ}
	}
	for _, exit := range exits {
		arguments := []string{}
		if wantValue {
			arguments = append(arguments, exit.result.id)
		}
		arguments = append(arguments, v3EnvironmentArguments(baseLocals, exit.env)...)
		exit.block.Terminator = Jump{Op: "jump", Target: join.ID, Arguments: arguments, Origin: caseOrigin}
	}
	l.current, l.env, l.locals = join, joinEnv, baseLocals
	return result, false, nil
}

func v3HasField(fields []Field, name string) bool {
	for _, field := range fields {
		if field.Name == name {
			return true
		}
	}
	return false
}

func v3Variant(definition TypeDefinition, name string) (Variant, bool) {
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

func (l *v3FunctionLowerer) lowerLiteral(node *ir.Literal) (v3ValueRef, error) {
	id := l.newValue()
	at := l.origin(node.SourceSpan())
	switch node.Kind {
	case "boolean":
		value, err := strconv.ParseBool(node.Raw)
		if err != nil {
			return v3ValueRef{}, err
		}
		l.emit(BooleanLiteral{Op: "boolean_literal", Result: id, Value: value, Origin: at})
	case "integer":
		value, ok := types.ParsePortableIntegerLiteral(node.Raw)
		if !ok {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "Integer literal "+node.Raw)
		}
		l.emit(IntegerLiteral{Op: "integer_literal", Result: id, Value: value, Origin: at})
	case "float":
		value, ok := types.ParsePortableFloatLiteral(node.Raw)
		if !ok {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "Float literal "+node.Raw)
		}
		l.emit(FloatLiteral{Op: "float_literal", Result: id, Value: value, Origin: at})
	case "string":
		if l.version < Version4 {
			return v3ValueRef{}, l.unsupported(node.SourceSpan(), "string literal expression")
		}
		value, err := strconv.Unquote(node.Raw)
		if err != nil {
			return v3ValueRef{}, fmt.Errorf("%s: decode TypeRB String literal: %w", l.program.SourcePath, err)
		}
		if _, err := l.typeName(node.ExprType(), node.SourceSpan()); err != nil {
			return v3ValueRef{}, err
		}
		l.emit(StringLiteral{Op: "string_literal", Result: id, Value: value, Origin: at})
	default:
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), node.Kind+" literal expression")
	}
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerUnary(node *ir.Unary) (v3ValueRef, error) {
	operand, err := l.lowerExpression(node.Operand)
	if err != nil {
		return v3ValueRef{}, err
	}
	if node.Operator == "+" {
		return operand, nil
	}
	if node.Operator == "!" && node.ExprType().Kind == types.Bool {
		id := l.newValue()
		l.emit(BooleanNot{Op: "boolean_not", Result: id, Value: operand.id, Origin: l.origin(node.SourceSpan())})
		return v3ValueRef{id: id, typ: node.ExprType()}, nil
	}
	if node.Operator == "-" && (node.ExprType().Kind == types.Int || node.ExprType().Kind == types.Float) {
		zeroID := l.newValue()
		at := l.origin(node.SourceSpan())
		if node.ExprType().Kind == types.Int {
			l.emit(IntegerLiteral{Op: "integer_literal", Result: zeroID, Value: 0, Origin: at})
		} else {
			l.emit(FloatLiteral{Op: "float_literal", Result: zeroID, Value: 0, Origin: at})
		}
		return l.emitBinary("-", v3ValueRef{id: zeroID, typ: node.ExprType()}, operand, node.SourceSpan())
	}
	return v3ValueRef{}, l.unsupported(node.SourceSpan(), "unary operator "+node.Operator)
}

func (l *v3FunctionLowerer) emitBinary(operator string, left, right v3ValueRef, span token.Span) (v3ValueRef, error) {
	resultType := left.typ
	if operator == "==" || operator == "!=" || operator == "<" || operator == "<=" || operator == ">" || operator == ">=" {
		resultType = types.FromName("Boolean")
	}
	typeName, typeErr := l.typeName(left.typ, span)
	supported := typeErr == nil
	if !supported || typeName == "Void" {
		if typeErr != nil {
			return v3ValueRef{}, typeErr
		}
		return v3ValueRef{}, l.unsupported(span, "binary operand type "+left.typ.String())
	}
	id := l.newValue()
	at := l.origin(span)
	if l.version >= Version4 && typeName == "String" {
		switch operator {
		case "+":
			l.emit(StringBinary{Op: "string_concat", Result: id, Left: left.id, Right: right.id, Origin: at})
			return v3ValueRef{id: id, typ: left.typ}, nil
		case "==", "!=":
			l.emit(StringBinary{Op: "string_equal", Result: id, Left: left.id, Right: right.id, Origin: at})
			if operator == "!=" {
				equalID := id
				id = l.newValue()
				l.emit(BooleanNot{Op: "boolean_not", Result: id, Value: equalID, Origin: at})
			}
			return v3ValueRef{id: id, typ: types.FromName("Boolean")}, nil
		default:
			return v3ValueRef{}, l.unsupported(span, "String operator "+operator)
		}
	}
	if operatorName, ok := v3ComparisonOperator(operator); ok {
		op := "integer_compare"
		if typeName == "Float" {
			op = "float_compare"
		} else if typeName != "Integer" {
			return v3ValueRef{}, l.unsupported(span, "comparison of "+typeName)
		}
		l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
		return v3ValueRef{id: id, typ: resultType}, nil
	}
	operatorName, ok := v3ArithmeticOperator(operator)
	if !ok {
		return v3ValueRef{}, l.unsupported(span, "binary operator "+operator)
	}
	op := "integer_binary"
	if typeName == "Float" {
		if operator == "%" || operator == "**" {
			return v3ValueRef{}, l.unsupported(span, "Float operator "+operator)
		}
		op = "float_binary"
	} else if typeName != "Integer" {
		return v3ValueRef{}, l.unsupported(span, "arithmetic on "+typeName)
	}
	l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
	return v3ValueRef{id: id, typ: resultType}, nil
}

func (l *v3FunctionLowerer) lowerCall(node *ir.Call) (v3ValueRef, error) {
	if l.isPuts(node) {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "puts() in a value position")
	}
	if l.version >= Version4 {
		if result, handled, err := l.lowerV4IntrinsicCall(node); handled || err != nil {
			return result, err
		}
		if identifier, ok := node.Callee.(*ir.Identifier); !ok || identifier.Lexical || l.callFunctionID(identifier) == "" {
			return l.lowerClosureCall(node)
		}
	}
	callee, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "indirect call")
	}
	function := l.callFunctionID(callee)
	if function == "" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "call to "+callee.Name)
	}
	arguments := make([]string, 0, len(node.Arguments))
	for _, argument := range node.Arguments {
		value, err := l.lowerExpression(argument.Value)
		if err != nil {
			return v3ValueRef{}, err
		}
		arguments = append(arguments, value.id)
	}
	resultType, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	var result *string
	ref := v3ValueRef{typ: node.ExprType()}
	if resultType != "Void" {
		ref.id = l.newValue()
		result = &ref.id
	}
	l.emit(Call{Op: "call", Result: result, Function: function, Arguments: arguments, Origin: l.origin(node.SourceSpan())})
	return ref, nil
}

func (l *v3FunctionLowerer) lowerPuts(node *ir.Call) error {
	if len(node.Arguments) != 1 {
		return l.unsupported(node.SourceSpan(), "puts() arity")
	}
	if l.version >= Version4 {
		value, err := l.lowerExpression(node.Arguments[0].Value)
		if err != nil {
			return err
		}
		name, err := l.typeName(value.typ, node.SourceSpan())
		if err != nil {
			return err
		}
		if name != "String" {
			return l.unsupported(node.SourceSpan(), "puts() argument type "+name)
		}
		l.emit(WriteString{Op: "write_string", Value: value.id, Newline: true, Origin: l.origin(node.SourceSpan())})
		return nil
	}
	literal, ok := node.Arguments[0].Value.(*ir.Literal)
	if !ok || literal.Kind != "string" {
		return l.unsupported(node.SourceSpan(), "dynamic puts() output")
	}
	value, err := strconv.Unquote(literal.Raw)
	if err != nil {
		return fmt.Errorf("%s: decode TypeRB String literal: %w", l.program.SourcePath, err)
	}
	l.emit(WriteStatic{Op: "write_static", Value: value + "\n", Origin: l.origin(node.SourceSpan())})
	return nil
}

func (l *v3FunctionLowerer) isPuts(node *ir.Call) bool {
	identifier, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return false
	}
	if identifier.Reference != nil && identifier.Reference.Intrinsic == "trb.std.io.puts" {
		return true
	}
	return identifier.Name == "puts" && identifier.Reference != nil && identifier.Reference.Intrinsic != ""
}

func (l *v3FunctionLowerer) callFunctionID(identifier *ir.Identifier) string {
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

func (l *v3FunctionLowerer) newBlock(span token.Span) *Block {
	block := &Block{
		ID: fmt.Sprintf("b%d", l.nextBlock), Parameters: []Parameter{},
		Origin: l.origin(span), Instructions: []any{},
	}
	l.nextBlock++
	l.blocks = append(l.blocks, block)
	return block
}

func (l *v3FunctionLowerer) newBlockWithLocals(span token.Span, names []string, template map[string]v3ValueRef) (*Block, map[string]v3ValueRef) {
	block := l.newBlock(span)
	environment := cloneV3Env(template)
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
		environment[name] = v3ValueRef{id: id, typ: value.typ}
	}
	return block, environment
}

func (l *v3FunctionLowerer) newValue() string {
	id := fmt.Sprintf("v%d", l.nextValue)
	l.nextValue++
	return id
}

func (l *v3FunctionLowerer) emit(instruction any) {
	l.current.Instructions = append(l.current.Instructions, instruction)
}

func cloneV3Env(source map[string]v3ValueRef) map[string]v3ValueRef {
	result := make(map[string]v3ValueRef, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func v3EnvironmentArguments(names []string, environment map[string]v3ValueRef) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, environment[name].id)
	}
	return result
}

func v3ArithmeticOperator(operator string) (string, bool) {
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

func v3ComparisonOperator(operator string) (string, bool) {
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
