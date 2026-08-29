package bootstrapsnapshot

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type valueRef struct {
	id  string
	typ types.Type
}

type functionLowerer struct {
	program   *ir.Program
	method    *ir.Method
	sourceID  string
	methodIDs map[string]string
	blocks    []*Block
	current   *Block
	env       map[string]valueRef
	locals    []string
	nextValue int
	nextBlock int
}

func lowerFunction(program *ir.Program, method *ir.Method, sourceID string, methodIDs map[string]string) (Function, error) {
	resultType, ok := scalarTypeName(method.ReturnType)
	if !ok {
		return Function{}, unsupported(program, method.SourceSpan(), "function result type "+method.ReturnType.String())
	}
	lowerer := &functionLowerer{
		program: program, method: method, sourceID: sourceID, methodIDs: methodIDs,
		env: map[string]valueRef{},
	}
	parameters := make([]Parameter, 0, len(method.Parameters))
	for _, item := range method.Parameters {
		parameterType, supported := scalarTypeName(item.Type)
		if !supported || parameterType == "Void" || item.Default != nil || item.NamedOnly || item.Rest || item.KeywordRest {
			return Function{}, unsupported(program, method.SourceSpan(), "non-scalar or non-required-positional parameter "+item.Name)
		}
		id := lowerer.newValue()
		parameters = append(parameters, Parameter{ID: id, Type: parameterType, Origin: origin(sourceID, method.SourceSpan())})
		lowerer.env[item.Name] = valueRef{id: id, typ: item.Type}
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
			return Function{}, unsupported(program, method.SourceSpan(), "fallthrough from a non-Void function")
		}
		lowerer.current.Terminator = Return{Op: "return", Value: nil, Origin: origin(sourceID, method.SourceSpan())}
	}
	blocks := make([]Block, len(lowerer.blocks))
	for index, block := range lowerer.blocks {
		if block.Terminator == nil {
			return Function{}, fmt.Errorf("%s: bootstrap snapshot lowering left block %s unterminated", program.SourcePath, block.ID)
		}
		blocks[index] = *block
	}
	return Function{
		ID: functionID(program, method), Name: method.Name, Parameters: parameters,
		Result: resultType, Entry: entry.ID, Origin: origin(sourceID, method.SourceSpan()), Blocks: blocks,
	}, nil
}

func (l *functionLowerer) lowerStatements(statements []ir.Statement) (bool, error) {
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

func (l *functionLowerer) lowerStatement(statement ir.Statement) (bool, error) {
	switch node := statement.(type) {
	case *ir.Comment:
		return false, nil
	case *ir.Variable:
		if node.Constant || node.Value == nil {
			return false, unsupported(l.program, node.SourceSpan(), "constant or uninitialized local binding")
		}
		if _, exists := l.env[node.Name]; exists {
			return false, unsupported(l.program, node.SourceSpan(), "shadowed local binding "+node.Name)
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
			return false, unsupported(l.program, node.SourceSpan(), "non-local assignment")
		}
		current, exists := l.env[identifier.Name]
		if !exists {
			return false, unsupported(l.program, node.SourceSpan(), "assignment to an unavailable local")
		}
		var next valueRef
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
			return false, unsupported(l.program, node.SourceSpan(), "assignment operator "+node.Operator)
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
		l.current.Terminator = Return{Op: "return", Value: returned, Origin: origin(l.sourceID, node.SourceSpan())}
		return true, nil
	case *ir.If:
		return l.lowerIf(node)
	case *ir.While:
		return l.lowerWhile(node)
	case *ir.Break:
		return false, unsupported(l.program, node.SourceSpan(), "break")
	case *ir.Next:
		return false, unsupported(l.program, node.SourceSpan(), "next")
	default:
		return false, unsupported(l.program, statement.SourceSpan(), fmt.Sprintf("statement %T", statement))
	}
}

func (l *functionLowerer) lowerIf(node *ir.If) (bool, error) {
	if len(node.ElseIf) != 0 || node.ThenResult != nil || node.ElseResult != nil {
		return false, unsupported(l.program, node.SourceSpan(), "elsif or value-producing if")
	}
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	baseEnv := cloneEnv(l.env)
	baseLocals := append([]string(nil), l.locals...)
	branchOrigin := origin(l.sourceID, node.SourceSpan())
	thenBlock, thenEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	elseBlock, elseEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	l.current.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: thenBlock.ID, TrueArguments: environmentArguments(baseLocals, baseEnv),
		WhenFalse: elseBlock.ID, FalseArguments: environmentArguments(baseLocals, baseEnv), Origin: branchOrigin,
	}

	l.current, l.env, l.locals = thenBlock, thenEnv, append([]string(nil), baseLocals...)
	thenTerminated, err := l.lowerStatements(node.Then)
	if err != nil {
		return false, err
	}
	thenEnd, thenFinal := l.current, cloneEnv(l.env)

	l.current, l.env, l.locals = elseBlock, elseEnv, append([]string(nil), baseLocals...)
	elseTerminated := false
	if node.HasElse {
		elseTerminated, err = l.lowerStatements(node.Else)
		if err != nil {
			return false, err
		}
	}
	elseEnd, elseFinal := l.current, cloneEnv(l.env)
	if thenTerminated && elseTerminated {
		l.env, l.locals = baseEnv, baseLocals
		return true, nil
	}
	join, joinEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	if !thenTerminated {
		thenEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: environmentArguments(baseLocals, thenFinal), Origin: branchOrigin,
		}
	}
	if !elseTerminated {
		elseEnd.Terminator = Jump{
			Op: "jump", Target: join.ID, Arguments: environmentArguments(baseLocals, elseFinal), Origin: branchOrigin,
		}
	}
	l.current, l.env, l.locals = join, joinEnv, baseLocals
	return false, nil
}

func (l *functionLowerer) lowerWhile(node *ir.While) (bool, error) {
	baseEnv := cloneEnv(l.env)
	baseLocals := append([]string(nil), l.locals...)
	loopOrigin := origin(l.sourceID, node.SourceSpan())
	previous := l.current
	header, headerEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	body, bodyEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	done, doneEnv := l.newBlockWithLocals(node.SourceSpan(), baseLocals, baseEnv)
	previous.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: environmentArguments(baseLocals, baseEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = header, headerEnv, append([]string(nil), baseLocals...)
	condition, err := l.lowerExpression(node.Condition)
	if err != nil {
		return false, err
	}
	header.Terminator = Branch{
		Op: "branch", Condition: condition.id,
		WhenTrue: body.ID, TrueArguments: environmentArguments(baseLocals, headerEnv),
		WhenFalse: done.ID, FalseArguments: environmentArguments(baseLocals, headerEnv), Origin: loopOrigin,
	}

	l.current, l.env, l.locals = body, bodyEnv, append([]string(nil), baseLocals...)
	terminated, err := l.lowerStatements(node.Body)
	if err != nil {
		return false, err
	}
	if terminated {
		return false, unsupported(l.program, node.SourceSpan(), "control transfer from a while body")
	}
	l.current.Terminator = Jump{
		Op: "jump", Target: header.ID, Arguments: environmentArguments(baseLocals, l.env), Origin: loopOrigin,
	}
	l.current, l.env, l.locals = done, doneEnv, baseLocals
	return false, nil
}

func (l *functionLowerer) lowerExpression(expression ir.Expression) (valueRef, error) {
	switch node := expression.(type) {
	case *ir.Identifier:
		value, ok := l.env[node.Name]
		if !ok || !node.Lexical {
			return valueRef{}, unsupported(l.program, node.SourceSpan(), "non-local value "+node.Name)
		}
		return value, nil
	case *ir.Literal:
		return l.lowerLiteral(node)
	case *ir.Unary:
		return l.lowerUnary(node)
	case *ir.Binary:
		left, err := l.lowerExpression(node.Left)
		if err != nil {
			return valueRef{}, err
		}
		right, err := l.lowerExpression(node.Right)
		if err != nil {
			return valueRef{}, err
		}
		return l.emitBinary(node.Operator, left, right, node.SourceSpan())
	case *ir.Call:
		return l.lowerCall(node)
	case *ir.Conversion:
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "scalar conversion "+string(node.Kind))
	case *ir.If:
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "value-producing if")
	default:
		return valueRef{}, unsupported(l.program, expression.SourceSpan(), fmt.Sprintf("expression %T", expression))
	}
}

func (l *functionLowerer) lowerLiteral(node *ir.Literal) (valueRef, error) {
	id := l.newValue()
	at := origin(l.sourceID, node.SourceSpan())
	switch node.Kind {
	case "boolean":
		value, err := strconv.ParseBool(node.Raw)
		if err != nil {
			return valueRef{}, err
		}
		l.emit(BooleanLiteral{Op: "boolean_literal", Result: id, Value: value, Origin: at})
	case "integer":
		value, ok := types.ParsePortableIntegerLiteral(node.Raw)
		if !ok {
			return valueRef{}, unsupported(l.program, node.SourceSpan(), "Integer literal "+node.Raw)
		}
		l.emit(IntegerLiteral{Op: "integer_literal", Result: id, Value: value, Origin: at})
	case "float":
		value, ok := types.ParsePortableFloatLiteral(node.Raw)
		if !ok {
			return valueRef{}, unsupported(l.program, node.SourceSpan(), "Float literal "+node.Raw)
		}
		l.emit(FloatLiteral{Op: "float_literal", Result: id, Value: value, Origin: at})
	default:
		return valueRef{}, unsupported(l.program, node.SourceSpan(), node.Kind+" literal expression")
	}
	return valueRef{id: id, typ: node.ExprType()}, nil
}

func (l *functionLowerer) lowerUnary(node *ir.Unary) (valueRef, error) {
	operand, err := l.lowerExpression(node.Operand)
	if err != nil {
		return valueRef{}, err
	}
	if node.Operator == "+" {
		return operand, nil
	}
	if node.Operator == "!" && node.ExprType().Kind == types.Bool {
		id := l.newValue()
		l.emit(BooleanNot{Op: "boolean_not", Result: id, Value: operand.id, Origin: origin(l.sourceID, node.SourceSpan())})
		return valueRef{id: id, typ: node.ExprType()}, nil
	}
	if node.Operator == "-" && (node.ExprType().Kind == types.Int || node.ExprType().Kind == types.Float) {
		zeroID := l.newValue()
		at := origin(l.sourceID, node.SourceSpan())
		if node.ExprType().Kind == types.Int {
			l.emit(IntegerLiteral{Op: "integer_literal", Result: zeroID, Value: 0, Origin: at})
		} else {
			l.emit(FloatLiteral{Op: "float_literal", Result: zeroID, Value: 0, Origin: at})
		}
		return l.emitBinary("-", valueRef{id: zeroID, typ: node.ExprType()}, operand, node.SourceSpan())
	}
	return valueRef{}, unsupported(l.program, node.SourceSpan(), "unary operator "+node.Operator)
}

func (l *functionLowerer) emitBinary(operator string, left, right valueRef, span token.Span) (valueRef, error) {
	resultType := left.typ
	if operator == "==" || operator == "!=" || operator == "<" || operator == "<=" || operator == ">" || operator == ">=" {
		resultType = types.FromName("Boolean")
	}
	typeName, supported := scalarTypeName(left.typ)
	if !supported || typeName == "Void" {
		return valueRef{}, unsupported(l.program, span, "binary operand type "+left.typ.String())
	}
	id := l.newValue()
	at := origin(l.sourceID, span)
	if operatorName, ok := comparisonOperator(operator); ok {
		op := "integer_compare"
		if typeName == "Float" {
			op = "float_compare"
		} else if typeName != "Integer" {
			return valueRef{}, unsupported(l.program, span, "comparison of "+typeName)
		}
		l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
		return valueRef{id: id, typ: resultType}, nil
	}
	operatorName, ok := arithmeticOperator(operator)
	if !ok {
		return valueRef{}, unsupported(l.program, span, "binary operator "+operator)
	}
	op := "integer_binary"
	if typeName == "Float" {
		if operator == "%" || operator == "**" {
			return valueRef{}, unsupported(l.program, span, "Float operator "+operator)
		}
		op = "float_binary"
	} else if typeName != "Integer" {
		return valueRef{}, unsupported(l.program, span, "arithmetic on "+typeName)
	}
	l.emit(BinaryInstruction{Op: op, Result: id, Operator: operatorName, Left: left.id, Right: right.id, Origin: at})
	return valueRef{id: id, typ: resultType}, nil
}

func (l *functionLowerer) lowerCall(node *ir.Call) (valueRef, error) {
	if l.isPuts(node) {
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "puts() in a value position")
	}
	callee, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "indirect call")
	}
	function := l.callFunctionID(callee)
	if function == "" {
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "call to "+callee.Name)
	}
	arguments := make([]string, 0, len(node.Arguments))
	for _, argument := range node.Arguments {
		value, err := l.lowerExpression(argument.Value)
		if err != nil {
			return valueRef{}, err
		}
		arguments = append(arguments, value.id)
	}
	resultType, supported := scalarTypeName(node.ExprType())
	if !supported {
		return valueRef{}, unsupported(l.program, node.SourceSpan(), "call result type "+node.ExprType().String())
	}
	var result *string
	ref := valueRef{typ: node.ExprType()}
	if resultType != "Void" {
		ref.id = l.newValue()
		result = &ref.id
	}
	l.emit(Call{Op: "call", Result: result, Function: function, Arguments: arguments, Origin: origin(l.sourceID, node.SourceSpan())})
	return ref, nil
}

func (l *functionLowerer) lowerPuts(node *ir.Call) error {
	if len(node.Arguments) != 1 {
		return unsupported(l.program, node.SourceSpan(), "puts() arity")
	}
	literal, ok := node.Arguments[0].Value.(*ir.Literal)
	if !ok || literal.Kind != "string" {
		return unsupported(l.program, node.SourceSpan(), "dynamic puts() output")
	}
	value, err := strconv.Unquote(literal.Raw)
	if err != nil {
		return fmt.Errorf("%s: decode TypeRB String literal: %w", l.program.SourcePath, err)
	}
	l.emit(WriteStatic{Op: "write_static", Value: value + "\n", Origin: origin(l.sourceID, node.SourceSpan())})
	return nil
}

func (l *functionLowerer) isPuts(node *ir.Call) bool {
	identifier, ok := node.Callee.(*ir.Identifier)
	if !ok {
		return false
	}
	if identifier.Reference != nil && identifier.Reference.Intrinsic == "trb.std.io.puts" {
		return true
	}
	return identifier.Name == "puts" && identifier.Reference != nil && identifier.Reference.Intrinsic != ""
}

func (l *functionLowerer) callFunctionID(identifier *ir.Identifier) string {
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

func (l *functionLowerer) newBlock(span token.Span) *Block {
	block := &Block{
		ID: fmt.Sprintf("b%d", l.nextBlock), Parameters: []Parameter{},
		Origin: origin(l.sourceID, span), Instructions: []any{},
	}
	l.nextBlock++
	l.blocks = append(l.blocks, block)
	return block
}

func (l *functionLowerer) newBlockWithLocals(span token.Span, names []string, template map[string]valueRef) (*Block, map[string]valueRef) {
	block := l.newBlock(span)
	environment := cloneEnv(template)
	for _, name := range names {
		value := template[name]
		typeName, ok := scalarTypeName(value.typ)
		if !ok || typeName == "Void" {
			continue
		}
		id := l.newValue()
		block.Parameters = append(block.Parameters, Parameter{ID: id, Type: typeName, Origin: origin(l.sourceID, span)})
		environment[name] = valueRef{id: id, typ: value.typ}
	}
	return block, environment
}

func (l *functionLowerer) newValue() string {
	id := fmt.Sprintf("v%d", l.nextValue)
	l.nextValue++
	return id
}

func (l *functionLowerer) emit(instruction any) {
	l.current.Instructions = append(l.current.Instructions, instruction)
}

func cloneEnv(source map[string]valueRef) map[string]valueRef {
	result := make(map[string]valueRef, len(source))
	for name, value := range source {
		result[name] = value
	}
	return result
}

func environmentArguments(names []string, environment map[string]valueRef) []string {
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, environment[name].id)
	}
	return result
}

func arithmeticOperator(operator string) (string, bool) {
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

func comparisonOperator(operator string) (string, bool) {
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

func unsupported(program *ir.Program, span token.Span, feature string) error {
	return &UnsupportedError{Path: program.SourcePath, Span: span, Feature: feature}
}
