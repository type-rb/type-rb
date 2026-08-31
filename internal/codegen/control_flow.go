package codegen

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

// normalizeDivergingControlFlow moves value-producing if/case expressions
// that contain an enclosing transfer into statement position. Go and
// TypeScript otherwise have to emit these expressions as closures, which
// would capture return, break, and next in the wrong control-flow owner.
func normalizeDivergingControlFlow(program *ir.Program) *ir.Program {
	n := &controlFlowNormalizer{reserved: map[string]bool{}}
	n.reserveStatements(program.Statements)
	result := *program
	result.Statements = n.statements(program.Statements)
	return &result
}

type controlFlowNormalizer struct {
	temporary  int
	reserved   map[string]bool
	executable int
}

type normalizedIfBranch struct {
	branch ir.IfBranch
	prefix []ir.Statement
}

func (n *controlFlowNormalizer) temporaryIdentifier(typ types.Type) (*ir.Temporary, *ir.Identifier) {
	name := ""
	for name == "" || n.reserved[name] {
		n.temporary++
		name = "__trbValue" + strconv.Itoa(n.temporary)
	}
	n.reserved[name] = true
	return &ir.Temporary{Name: name, Type: typ}, &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, typ), Name: name, Lexical: true, Generated: true}
}

// materialize evaluates a value before a later compiler-lifted statement
// prefix and returns a stable reference that can be used after that prefix.
// This preserves the source language's left-to-right evaluation order even
// when the later expression diverges or contains control flow that cannot stay
// nested in a target-language expression.
func (n *controlFlowNormalizer) materialize(value ir.Expression) ([]ir.Statement, ir.Expression) {
	if value == nil {
		return nil, nil
	}
	if identifier, ok := value.(*ir.Identifier); ok && identifier.Generated {
		return nil, value
	}
	temporary, identifier := n.temporaryIdentifier(value.ExprType())
	return []ir.Statement{temporary, assignment(identifier, value)}, identifier
}

// evaluate preserves an earlier value's evaluation when a later expression
// diverges and therefore removes the enclosing value-producing operation.
// The generated identifier use keeps target languages with unused-local
// diagnostics valid without adding observable behavior.
func (n *controlFlowNormalizer) evaluate(value ir.Expression) []ir.Statement {
	statements, identifier := n.materialize(value)
	if identifier == nil {
		return statements
	}
	statements = append(statements, &ir.ExpressionStatement{
		Base:       ir.Base{Span: value.SourceSpan()},
		Expression: identifier,
	})
	return statements
}

func (n *controlFlowNormalizer) finishExpression(prefix []ir.Statement, value ir.Expression) ([]ir.Statement, ir.Expression) {
	if value == nil || value.ExprType().Kind != types.Never {
		return prefix, value
	}
	return append(prefix, &ir.ExpressionStatement{
		Base:       ir.Base{Span: value.SourceSpan()},
		Expression: value,
	}), nil
}

func (n *controlFlowNormalizer) executableStatements(input []ir.Statement) []ir.Statement {
	n.executable++
	defer func() { n.executable-- }()
	return n.statements(input)
}

// deferredExpression retains statement prefixes inside a value expression
// whose surrounding declaration (for example, a parameter or field default)
// is emitted later inside an executable body. A compiler-owned zero-argument
// lambda provides that local statement owner without changing evaluation time.
func (n *controlFlowNormalizer) deferredExpression(value ir.Expression, expected types.Type) ir.Expression {
	if value == nil {
		return nil
	}
	prefix, result := n.expression(value)
	if len(prefix) == 0 {
		return result
	}
	body := append([]ir.Statement(nil), prefix...)
	if result != nil {
		body = append(body, &ir.Return{Base: ir.Base{Span: result.SourceSpan()}, Value: result})
	}
	functionType := types.FunctionOf(nil, expected)
	lambda := &ir.Lambda{
		ExprBase:   ir.NewExprBase(value.SourceSpan(), functionType),
		ReturnType: expected,
		Body:       body,
	}
	return &ir.Call{
		ExprBase: ir.NewExprBase(value.SourceSpan(), expected),
		Callee:   lambda,
	}
}

func (n *controlFlowNormalizer) parameters(input []ir.Parameter) []ir.Parameter {
	result := append([]ir.Parameter(nil), input...)
	for index := range result {
		result[index].Default = n.deferredExpression(result[index].Default, result[index].Type)
	}
	return result
}

func neverSurrogateType(typ types.Type) types.Type {
	if typ.Kind == types.Never {
		return types.FromName("Any")
	}
	copy := typ
	copy.Args = append([]types.Type(nil), typ.Args...)
	for index := range copy.Args {
		copy.Args[index] = neverSurrogateType(copy.Args[index])
	}
	return copy
}

func transformResultSurrogateType(transform *ir.Transform) types.Type {
	if transform == nil || transform.Result == nil {
		return types.FromName("Void")
	}
	result := transform.Result.ExprType()
	if result.Kind != types.Never {
		return neverSurrogateType(result)
	}
	switch transform.Operation {
	case "select", "any?", "all?", "none?", "find", "find_index":
		return types.FromName("Boolean")
	case "reduce":
		if transform.Initial != nil {
			return neverSurrogateType(transform.Initial.ExprType())
		}
	case "sort_by", "sort_by_descending":
		return types.FromName("String")
	}
	return types.FromName("Any")
}

func (n *controlFlowNormalizer) stabilizeCallee(value ir.Expression) ([]ir.Statement, ir.Expression) {
	switch node := value.(type) {
	case *ir.Identifier:
		if !node.Lexical {
			return nil, value
		}
	case *ir.Member:
		if node.Namespace {
			return nil, value
		}
		if !node.ClassField {
			prefix, receiver := n.materialize(node.Receiver)
			copy := *node
			copy.Receiver = receiver
			return prefix, &copy
		}
	case *ir.TypeApply:
		prefix, receiver := n.stabilizeCallee(node.Receiver)
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	}
	return n.materialize(value)
}

func (n *controlFlowNormalizer) evaluateCallee(value ir.Expression) []ir.Statement {
	switch node := value.(type) {
	case *ir.Identifier:
		// IR callable identifiers carry the call's result type rather than a
		// first-class function-value type. Resolving a static callable has no
		// target-language effect of its own.
		return nil
	case *ir.Member:
		if node.Namespace {
			return nil
		}
		return n.evaluate(node.Receiver)
	case *ir.TypeApply:
		return n.evaluateCallee(node.Receiver)
	default:
		return n.evaluate(value)
	}
}

func safeCalleeMember(value ir.Expression) (*ir.Member, bool) {
	switch node := value.(type) {
	case *ir.Member:
		return node, node.Safe
	case *ir.TypeApply:
		return safeCalleeMember(node.Receiver)
	default:
		return nil, false
	}
}

func safePresentType(member *ir.Member) types.Type {
	if member.PresentType.Kind != "" {
		return member.PresentType
	}
	present := member.ExprType()
	present.Nullable = false
	return present
}

func safePresentReceiver(value ir.Expression) ir.Expression {
	typ := value.ExprType()
	if !typ.Nullable {
		return value
	}
	present := typ
	present.Nullable = false
	return &ir.Conversion{
		ExprBase: ir.NewExprBase(value.SourceSpan(), present),
		Kind:     ir.NullableToNonNullableConversion,
		Value:    value,
	}
}

func replaceSafeCalleeReceiver(value ir.Expression, receiver ir.Expression) (ir.Expression, types.Type) {
	switch node := value.(type) {
	case *ir.Member:
		present := safePresentType(node)
		copy := *node
		copy.Type = present
		copy.Receiver = receiver
		copy.Safe = false
		copy.PresentType = types.Type{}
		return &copy, present
	case *ir.TypeApply:
		inner, _ := replaceSafeCalleeReceiver(node.Receiver, receiver)
		copy := *node
		copy.Receiver = inner
		return &copy, node.ExprType()
	default:
		return value, value.ExprType()
	}
}

func safeNavigationNil(span token.Span) ir.Expression {
	return &ir.Literal{ExprBase: ir.NewExprBase(span, types.FromName("Nil")), Kind: "nil", Raw: "nil"}
}

func safeNavigationCondition(receiver ir.Expression, span token.Span) ir.Expression {
	return &ir.Binary{
		ExprBase: ir.NewExprBase(span, types.FromName("Boolean")),
		Left:     receiver,
		Operator: "!=",
		Right:    safeNavigationNil(span),
	}
}

func safeNavigationPresentValue(value ir.Expression, target types.Type) ir.Expression {
	if value == nil || types.Equivalent(value.ExprType(), target) {
		return value
	}
	if target.Nullable && !value.ExprType().Nullable && value.ExprType().Kind != types.Nil {
		return &ir.Conversion{
			ExprBase: ir.NewExprBase(value.SourceSpan(), target),
			Kind:     ir.NonNullableToNullableConversion,
			Value:    value,
		}
	}
	return value
}

func (n *controlFlowNormalizer) safeNavigationExpression(prefix []ir.Statement, receiver ir.Expression, present ir.Expression, resultType types.Type, span token.Span) ([]ir.Statement, ir.Expression) {
	flow := &ir.If{
		ExprBase:  ir.NewExprBase(span, resultType),
		Condition: safeNavigationCondition(receiver, span),
		HasElse:   true,
	}
	switch present.ExprType().Kind {
	case types.Void:
		flow.Then = []ir.Statement{&ir.ExpressionStatement{Base: ir.Base{Span: present.SourceSpan()}, Expression: present}}
	case types.Never:
		flow.ThenResult = present
		flow.ThenDiverges = true
		flow.ElseResult = safeNavigationNil(span)
	default:
		flow.ThenResult = safeNavigationPresentValue(present, resultType)
		flow.ElseResult = safeNavigationNil(span)
	}
	flowPrefix, result := n.ifExpression(flow)
	return append(prefix, flowPrefix...), result
}

func (n *controlFlowNormalizer) safeNavigationCall(node *ir.Call, member *ir.Member) ([]ir.Statement, ir.Expression) {
	if !member.Receiver.ExprType().Nullable {
		callee, calleeType := replaceSafeCalleeReceiver(node.Callee, member.Receiver)
		presentType := node.PresentType
		if presentType.Kind == "" {
			presentType = calleeType
		}
		copy := *node
		copy.Type = presentType
		copy.Callee = callee
		copy.PresentType = types.Type{}
		return n.expression(&copy)
	}
	prefix, receiver := n.expression(member.Receiver)
	if receiver == nil {
		return prefix, nil
	}
	stablePrefix, stable := n.materialize(receiver)
	prefix = append(prefix, stablePrefix...)
	callee, calleeType := replaceSafeCalleeReceiver(node.Callee, safePresentReceiver(stable))
	presentType := node.PresentType
	if presentType.Kind == "" {
		presentType = calleeType
	}
	present := *node
	present.Type = presentType
	present.Callee = callee
	present.PresentType = types.Type{}
	return n.safeNavigationExpression(prefix, stable, &present, node.ExprType(), node.SourceSpan())
}

func (n *controlFlowNormalizer) safeNavigationMember(node *ir.Member) ([]ir.Statement, ir.Expression) {
	presentType := safePresentType(node)
	if !node.Receiver.ExprType().Nullable {
		copy := *node
		copy.Type = presentType
		copy.Safe = false
		copy.PresentType = types.Type{}
		return n.expression(&copy)
	}
	prefix, receiver := n.expression(node.Receiver)
	if receiver == nil {
		return prefix, nil
	}
	stablePrefix, stable := n.materialize(receiver)
	prefix = append(prefix, stablePrefix...)
	present := *node
	present.Type = presentType
	present.Receiver = safePresentReceiver(stable)
	present.Safe = false
	present.PresentType = types.Type{}
	return n.safeNavigationExpression(prefix, stable, &present, node.ExprType(), node.SourceSpan())
}

func (n *controlFlowNormalizer) stabilizeAssignmentTarget(value ir.Expression) ([]ir.Statement, ir.Expression) {
	switch node := value.(type) {
	case *ir.Identifier:
		return nil, value
	case *ir.Member:
		prefix, receiver := n.materialize(node.Receiver)
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	case *ir.Index:
		prefix, receiver := n.materialize(node.Receiver)
		indexPrefix, index := n.materialize(node.Index)
		prefix = append(prefix, indexPrefix...)
		copy := *node
		copy.Receiver = receiver
		copy.Index = index
		return prefix, &copy
	default:
		return n.materialize(value)
	}
}

func (n *controlFlowNormalizer) evaluateAssignmentTarget(value ir.Expression) []ir.Statement {
	switch node := value.(type) {
	case *ir.Identifier:
		return nil
	case *ir.Member:
		return n.evaluate(node.Receiver)
	case *ir.Index:
		result := n.evaluate(node.Receiver)
		return append(result, n.evaluate(node.Index)...)
	default:
		return n.evaluate(value)
	}
}

func (n *controlFlowNormalizer) lazyAssignment(prefix []ir.Statement, node *ir.Assignment, target ir.Expression, valuePrefix []ir.Statement, value ir.Expression) []ir.Statement {
	targetPrefix, stable := n.stabilizeAssignmentTarget(target)
	prefix = append(prefix, targetPrefix...)
	body := append([]ir.Statement(nil), valuePrefix...)
	if value != nil {
		body = append(body, &ir.Assignment{
			Base:     node.Base,
			Target:   stable,
			Operator: "=",
			Value:    value,
		})
	}
	condition := stable
	if node.Operator == "||=" {
		condition = &ir.Unary{
			ExprBase: ir.NewExprBase(node.SourceSpan(), types.FromName("Boolean")),
			Operator: "!",
			Operand:  stable,
		}
	}
	flow := &ir.If{
		ExprBase:  ir.NewExprBase(node.SourceSpan(), types.FromName("Void")),
		Condition: condition,
		Then:      body,
	}
	return append(prefix, flow)
}

func (n *controlFlowNormalizer) reserveStatements(statements []ir.Statement) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			n.reserveStatements(node.Body)
		case *ir.Record:
			n.reserveStatements(node.Body)
		case *ir.Enum:
			n.reserveStatements(node.Body)
		case *ir.Module:
			n.reserveStatements(node.Body)
		case *ir.Method:
			for _, parameter := range node.Parameters {
				n.reserved[parameter.Name] = true
			}
			n.reserveStatements(node.Body)
		case *ir.Variable:
			n.reserved[node.Name] = true
		case *ir.If:
			n.reserveStatements(node.Then)
			for _, branch := range node.ElseIf {
				n.reserveStatements(branch.Body)
			}
			n.reserveStatements(node.Else)
		case *ir.Case:
			for _, branch := range node.Branches {
				for _, binding := range branch.Bindings {
					n.reserved[binding.Name] = true
				}
				n.reserveStatements(branch.Body)
			}
			n.reserveStatements(node.Else)
		case *ir.While:
			n.reserveStatements(node.Body)
		case *ir.Iterate:
			if node.Result != nil && node.Result.Variable != nil {
				n.reserved[node.Result.Variable.Name] = true
			}
			for _, binding := range node.Bindings {
				n.reserved[binding.Name] = true
			}
			n.reserveStatements(node.Body)
		case *ir.StructuredBlock:
			if node.Result != nil && node.Result.Variable != nil {
				n.reserved[node.Result.Variable.Name] = true
			}
			for _, binding := range node.Bindings {
				n.reserved[binding.Name] = true
			}
			n.reserveStatements(node.Body)
		case *ir.NativeBlock:
			n.reserveStatements(node.Body)
		}
	}
}

func (n *controlFlowNormalizer) statements(input []ir.Statement) []ir.Statement {
	result := make([]ir.Statement, 0, len(input))
	for _, statement := range input {
		normalized := n.statement(statement)
		result = append(result, normalized...)
		if n.executable > 0 {
			variable, ok := statement.(*ir.Variable)
			if ok && variable.Value != nil {
				declared := false
				for _, candidate := range normalized {
					if value, declaration := candidate.(*ir.Variable); declaration && value.Name == variable.Name {
						declared = true
						break
					}
				}
				if !declared {
					break
				}
			}
		}
	}
	return result
}

func (n *controlFlowNormalizer) statement(statement ir.Statement) []ir.Statement {
	switch node := statement.(type) {
	case *ir.Class:
		copy := *node
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	case *ir.Record:
		copy := *node
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	case *ir.Enum:
		copy := *node
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	case *ir.Module:
		copy := *node
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	case *ir.Method:
		copy := *node
		copy.Parameters = n.parameters(node.Parameters)
		copy.Body = n.executableStatements(node.Body)
		return []ir.Statement{&copy}
	case *ir.RecordField:
		copy := *node
		copy.Default = n.deferredExpression(node.Default, node.Type)
		return []ir.Statement{&copy}
	case *ir.Field:
		copy := *node
		copy.Value = n.deferredExpression(node.Value, node.Type)
		return []ir.Statement{&copy}
	case *ir.Variable:
		if n.executable == 0 {
			copy := *node
			copy.Type = neverSurrogateType(node.Type)
			copy.Value = n.deferredExpression(node.Value, copy.Type)
			return []ir.Statement{&copy}
		}
		prefix, value := n.expression(node.Value)
		if value == nil {
			return prefix
		}
		if node.Value != nil && node.Value.ExprType().Kind == types.Never {
			failure := &ir.ExpressionStatement{Base: ir.Base{Span: node.SourceSpan()}, Expression: value}
			return append(prefix, failure)
		}
		copy := *node
		copy.Type = neverSurrogateType(node.Type)
		copy.Value = value
		return append(prefix, &copy)
	case *ir.Assignment:
		prefix, target := n.expression(node.Target)
		if target == nil {
			return prefix
		}
		valuePrefix, value := n.expression(node.Value)
		if node.Operator == "&&=" || node.Operator == "||=" {
			return n.lazyAssignment(prefix, node, target, valuePrefix, value)
		}
		if value == nil {
			prefix = append(prefix, n.evaluateAssignmentTarget(target)...)
			return append(prefix, valuePrefix...)
		}
		if len(valuePrefix) > 0 {
			targetPrefix, stable := n.stabilizeAssignmentTarget(target)
			prefix = append(prefix, targetPrefix...)
			target = stable
			prefix = append(prefix, valuePrefix...)
		}
		copy := *node
		copy.Target = target
		copy.Value = value
		return append(prefix, &copy)
	case *ir.Return:
		if node.Value == nil {
			copy := *node
			return []ir.Statement{&copy}
		}
		prefix, value := n.expression(node.Value)
		if value == nil {
			return prefix
		}
		copy := *node
		copy.Value = value
		return append(prefix, &copy)
	case *ir.ExpressionStatement:
		prefix, expression := n.expression(node.Expression)
		if expression == nil {
			return prefix
		}
		copy := *node
		copy.Expression = expression
		return append(prefix, &copy)
	case *ir.If:
		return n.ifStatement(node)
	case *ir.Case:
		return n.caseStatement(node)
	case *ir.While:
		return n.whileStatement(node)
	case *ir.Iterate:
		expressions := []ir.Expression{node.Source}
		if node.SliceSize != nil {
			expressions = append(expressions, node.SliceSize)
		}
		prefix, values, ok := n.expressions(expressions)
		if !ok {
			return prefix
		}
		copy := *node
		copy.Source = values[0]
		if node.SliceSize != nil {
			copy.SliceSize = values[1]
		}
		copy.Body = n.statements(node.Body)
		if node.Result != nil && node.Result.Target != nil {
			resultPrefix, target := n.expression(node.Result.Target)
			prefix = append(prefix, resultPrefix...)
			if target == nil {
				return prefix
			}
			result := *node.Result
			result.Target = target
			copy.Result = &result
		}
		return append(prefix, &copy)
	case *ir.StructuredBlock:
		callPrefix, callExpression := n.expression(node.Call)
		if callExpression == nil {
			return callPrefix
		}
		call, ok := callExpression.(*ir.Call)
		if !ok {
			return callPrefix
		}
		copy := *node
		copy.Call = call
		copy.Body = n.statements(node.Body)
		valuePrefix, value := n.expression(node.Value)
		copy.Body = append(copy.Body, valuePrefix...)
		copy.Value = value
		if node.Result != nil && node.Result.Target != nil {
			resultPrefix, target := n.expression(node.Result.Target)
			callPrefix = append(callPrefix, resultPrefix...)
			if target == nil {
				return callPrefix
			}
			result := *node.Result
			result.Target = target
			copy.Result = &result
		}
		return append(callPrefix, &copy)
	case *ir.NativeBlock:
		copy := *node
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	default:
		return []ir.Statement{statement}
	}
}

func (n *controlFlowNormalizer) ifStatement(node *ir.If) []ir.Statement {
	prefix, condition := n.expression(node.Condition)
	if condition == nil {
		return prefix
	}
	copy := *node
	copy.Condition = condition
	copy.Then = n.statements(node.Then)
	copy.Else = n.statements(node.Else)
	copy.ElseIf = make([]ir.IfBranch, len(node.ElseIf))
	normalized := make([]normalizedIfBranch, len(node.ElseIf))
	hasPrefix := false
	for index, branch := range node.ElseIf {
		branchCopy := branch
		branchPrefix, branchCondition := n.expression(branch.Condition)
		hasPrefix = hasPrefix || len(branchPrefix) > 0
		if branchCondition == nil {
			branchCopy.Condition = falseLiteral()
			branchCopy.Body = nil
		} else {
			branchCopy.Condition = branchCondition
			branchCopy.Body = n.statements(branch.Body)
		}
		copy.ElseIf[index] = branchCopy
		normalized[index] = normalizedIfBranch{branch: branchCopy, prefix: branchPrefix}
	}
	if hasPrefix {
		copy.Else = nestedElseIf(normalized, copy.Else, node.Span)
		copy.ElseIf = nil
	}
	return append(prefix, &copy)
}

func (n *controlFlowNormalizer) caseStatement(node *ir.Case) []ir.Statement {
	leading := n.statements(node.Leading)
	prefix, value := n.expression(node.Value)
	leading = append(leading, prefix...)
	if value == nil {
		return leading
	}
	copy := *node
	copy.Value = value
	copy.Leading = nil
	copy.Branches = make([]ir.CaseBranch, len(node.Branches))
	for index, branch := range node.Branches {
		branchCopy := branch
		branchCopy.Body = n.statements(branch.Body)
		copy.Branches[index] = branchCopy
	}
	copy.Else = n.statements(node.Else)
	return append(leading, &copy)
}

func (n *controlFlowNormalizer) whileStatement(node *ir.While) []ir.Statement {
	prefix, condition := n.expression(node.Condition)
	if condition == nil {
		return prefix
	}
	copy := *node
	copy.Body = n.statements(node.Body)
	if len(prefix) == 0 {
		copy.Condition = condition
		return []ir.Statement{&copy}
	}
	boolean := types.FromName("Boolean")
	copy.Condition = &ir.Literal{ExprBase: ir.NewExprBase(node.Span, boolean), Kind: "boolean", Raw: "true"}
	guard := &ir.If{
		ExprBase:  ir.NewExprBase(node.Span, types.FromName("Void")),
		Condition: &ir.Unary{ExprBase: ir.NewExprBase(node.Span, boolean), Operator: "!", Operand: condition},
		Then:      []ir.Statement{&ir.Break{}},
	}
	copy.Body = append(prefix, append([]ir.Statement{guard}, copy.Body...)...)
	return []ir.Statement{&copy}
}

func (n *controlFlowNormalizer) valueBranch(body []ir.Statement, result ir.Expression, diverges bool) ([]ir.Statement, ir.Expression, bool) {
	statements := n.statements(body)
	if result == nil {
		return statements, nil, diverges
	}
	prefix, value := n.expression(result)
	statements = append(statements, prefix...)
	if value == nil {
		return statements, nil, true
	}
	return statements, value, diverges
}

func (n *controlFlowNormalizer) ifExpression(node *ir.If) ([]ir.Statement, ir.Expression) {
	prefix, condition := n.expression(node.Condition)
	if condition == nil {
		return prefix, nil
	}
	copy := *node
	copy.Condition = condition
	copy.Then, copy.ThenResult, copy.ThenDiverges = n.valueBranch(node.Then, node.ThenResult, node.ThenDiverges)
	copy.Else, copy.ElseResult, copy.ElseDiverges = n.valueBranch(node.Else, node.ElseResult, node.ElseDiverges)
	copy.ElseIf = make([]ir.IfBranch, len(node.ElseIf))
	normalized := make([]normalizedIfBranch, len(node.ElseIf))
	hasConditionPrefix := false
	for index, branch := range node.ElseIf {
		branchCopy := branch
		branchPrefix, branchCondition := n.expression(branch.Condition)
		hasConditionPrefix = hasConditionPrefix || len(branchPrefix) > 0
		if branchCondition == nil {
			branchCopy.Condition = falseLiteral()
			branchCopy.Body = nil
			branchCopy.Result = nil
			branchCopy.Diverges = true
		} else {
			branchCopy.Condition = branchCondition
			branchCopy.Body, branchCopy.Result, branchCopy.Diverges = n.valueBranch(branch.Body, branch.Result, branch.Diverges)
		}
		copy.ElseIf[index] = branchCopy
		normalized[index] = normalizedIfBranch{branch: branchCopy, prefix: branchPrefix}
	}
	if node.ExprType().Kind == types.Never {
		copy.Type = types.FromName("Void")
		clearIfResults(&copy, nil)
		if hasConditionPrefix {
			for index := range normalized {
				normalized[index].branch = copy.ElseIf[index]
			}
			copy.Else = nestedElseIf(normalized, copy.Else, node.Span)
			copy.ElseIf = nil
		}
		return append(prefix, &copy), nil
	}
	if !hasConditionPrefix && !containsTransferIf(&copy) {
		return prefix, &copy
	}
	copy.Type = types.FromName("Void")
	temporary, identifier := n.temporaryIdentifier(node.ExprType())
	clearIfResults(&copy, identifier)
	if hasConditionPrefix {
		for index := range normalized {
			normalized[index].branch = copy.ElseIf[index]
		}
		copy.Else = nestedElseIf(normalized, copy.Else, node.Span)
		copy.ElseIf = nil
	}
	return append(prefix, temporary, &copy), identifier
}

func nestedElseIf(branches []normalizedIfBranch, finalElse []ir.Statement, span token.Span) []ir.Statement {
	result := finalElse
	for index := len(branches) - 1; index >= 0; index-- {
		branch := branches[index]
		nested := &ir.If{
			ExprBase:  ir.NewExprBase(span, types.FromName("Void")),
			Condition: branch.branch.Condition,
			Then:      branch.branch.Body,
			Else:      result,
			HasElse:   len(result) > 0,
		}
		result = append(append([]ir.Statement(nil), branch.prefix...), nested)
	}
	return result
}

func (n *controlFlowNormalizer) caseExpression(node *ir.Case) ([]ir.Statement, ir.Expression) {
	leading := n.statements(node.Leading)
	prefix, value := n.expression(node.Value)
	leading = append(leading, prefix...)
	if value == nil {
		return leading, nil
	}
	copy := *node
	copy.Value = value
	copy.Leading = nil
	copy.Branches = make([]ir.CaseBranch, len(node.Branches))
	for index, branch := range node.Branches {
		branchCopy := branch
		branchCopy.Body, branchCopy.Result, branchCopy.Diverges = n.valueBranch(branch.Body, branch.Result, branch.Diverges)
		copy.Branches[index] = branchCopy
	}
	copy.Else, copy.ElseResult, copy.ElseDiverges = n.valueBranch(node.Else, node.ElseResult, node.ElseDiverges)
	if node.ExprType().Kind == types.Never {
		copy.Type = types.FromName("Void")
		clearCaseResults(&copy, nil)
		return append(leading, &copy), nil
	}
	if !containsTransferCase(&copy) {
		return leading, &copy
	}
	copy.Type = types.FromName("Void")
	temporary, identifier := n.temporaryIdentifier(node.ExprType())
	clearCaseResults(&copy, identifier)
	return append(leading, temporary, &copy), identifier
}

func clearIfResults(node *ir.If, target *ir.Identifier) {
	if !node.ThenDiverges && node.ThenResult != nil && target != nil {
		node.Then = append(node.Then, assignment(target, node.ThenResult))
	}
	node.ThenResult = nil
	for index := range node.ElseIf {
		branch := &node.ElseIf[index]
		if !branch.Diverges && branch.Result != nil && target != nil {
			branch.Body = append(branch.Body, assignment(target, branch.Result))
		}
		branch.Result = nil
	}
	if !node.ElseDiverges && node.ElseResult != nil && target != nil {
		node.Else = append(node.Else, assignment(target, node.ElseResult))
	}
	node.ElseResult = nil
}

func clearCaseResults(node *ir.Case, target *ir.Identifier) {
	for index := range node.Branches {
		branch := &node.Branches[index]
		if !branch.Diverges && branch.Result != nil && target != nil {
			branch.Body = append(branch.Body, assignment(target, branch.Result))
		}
		branch.Result = nil
	}
	if !node.ElseDiverges && node.ElseResult != nil && target != nil {
		node.Else = append(node.Else, assignment(target, node.ElseResult))
	}
	node.ElseResult = nil
}

func assignment(target *ir.Identifier, value ir.Expression) *ir.Assignment {
	copy := *target
	return &ir.Assignment{Target: &copy, Operator: "=", Value: value}
}

func falseLiteral() ir.Expression {
	return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("Boolean")), Kind: "boolean", Raw: "false"}
}

func unreachableNeverExpression(span token.Span) ir.Expression {
	never := types.Type{Kind: types.Never, Name: "Never"}
	return &ir.Call{
		ExprBase: ir.NewExprBase(span, never),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(span, never),
			Name:      "fail",
			Reference: &ir.Reference{Intrinsic: "trb.internal.runtime.fail"},
		},
		Arguments: []ir.CallArgument{{Value: &ir.Literal{
			ExprBase: ir.NewExprBase(span, types.FromName("String")),
			Kind:     "string",
			Raw:      `"unreachable Never value"`,
		}}},
	}
}

func containsTransferIf(node *ir.If) bool {
	if containsTransfer(node.Then) || containsTransfer(node.Else) {
		return true
	}
	for _, branch := range node.ElseIf {
		if containsTransfer(branch.Body) {
			return true
		}
	}
	return false
}

func containsTransferCase(node *ir.Case) bool {
	if containsTransfer(node.Else) {
		return true
	}
	for _, branch := range node.Branches {
		if containsTransfer(branch.Body) {
			return true
		}
	}
	return false
}

func containsTransfer(statements []ir.Statement) bool {
	return containsEscapingTransfer(statements, 0)
}

func containsEscapingTransfer(statements []ir.Statement, loopDepth int) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Return:
			return true
		case *ir.Break, *ir.Next:
			if loopDepth == 0 {
				return true
			}
		case *ir.If:
			if containsEscapingTransfer(node.Then, loopDepth) || containsEscapingTransfer(node.Else, loopDepth) {
				return true
			}
			for _, branch := range node.ElseIf {
				if containsEscapingTransfer(branch.Body, loopDepth) {
					return true
				}
			}
		case *ir.Case:
			if containsEscapingTransfer(node.Else, loopDepth) {
				return true
			}
			for _, branch := range node.Branches {
				if containsEscapingTransfer(branch.Body, loopDepth) {
					return true
				}
			}
		case *ir.While:
			if containsEscapingTransfer(node.Body, loopDepth+1) {
				return true
			}
		case *ir.Iterate:
			if node.ResultBoundary {
				// A Result-boundary structured iteration owns propagation returns.
				// Authored return is rejected by the checker before typed IR.
				continue
			}
			if containsEscapingTransfer(node.Body, loopDepth+1) {
				return true
			}
		case *ir.StructuredBlock:
			// A value-producing structured block owns return and propagation.
			// Transfers inside it do not escape into the surrounding method.
		case *ir.NativeBlock:
			if containsEscapingTransfer(node.Body, loopDepth) {
				return true
			}
		}
	}
	return false
}

func (n *controlFlowNormalizer) expression(expression ir.Expression) ([]ir.Statement, ir.Expression) {
	if expression == nil {
		return nil, nil
	}
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Lexical && node.ExprType().Kind == types.Never {
			return n.finishExpression(nil, unreachableNeverExpression(node.SourceSpan()))
		}
		return nil, expression
	case *ir.Lambda:
		copy := *node
		inner := &controlFlowNormalizer{temporary: n.temporary, reserved: n.reserved, executable: n.executable}
		copy.Parameters = inner.parameters(node.Parameters)
		copy.Body = inner.executableStatements(node.Body)
		n.temporary = inner.temporary
		return nil, &copy
	case *ir.If:
		return n.ifExpression(node)
	case *ir.Case:
		return n.caseExpression(node)
	case *ir.InterpolatedString:
		copy := *node
		copy.Parts = append([]ir.StringPart(nil), node.Parts...)
		expressions := make([]ir.Expression, 0, len(copy.Parts))
		indexes := make([]int, 0, len(copy.Parts))
		for index, part := range copy.Parts {
			if part.Expression != nil {
				expressions = append(expressions, part.Expression)
				indexes = append(indexes, index)
			}
		}
		prefix, values, ok := n.expressions(expressions)
		if !ok {
			return prefix, nil
		}
		for index, value := range values {
			copy.Parts[indexes[index]].Expression = value
		}
		return n.finishExpression(prefix, &copy)
	case *ir.Array:
		copy := *node
		prefix, values, ok := n.expressions(node.Elements)
		if !ok {
			return prefix, nil
		}
		copy.Elements = values
		return n.finishExpression(prefix, &copy)
	case *ir.Hash:
		copy := *node
		copy.Entries = make([]ir.HashEntry, len(node.Entries))
		expressions := make([]ir.Expression, 0, len(node.Entries)*2)
		for _, entry := range node.Entries {
			expressions = append(expressions, entry.Key, entry.Value)
		}
		prefix, values, ok := n.expressions(expressions)
		if !ok {
			return prefix, nil
		}
		for index := range copy.Entries {
			copy.Entries[index] = ir.HashEntry{Key: values[index*2], Value: values[index*2+1]}
		}
		return n.finishExpression(prefix, &copy)
	case *ir.JSXElement:
		copy := *node
		prefix, component := n.expression(node.Component)
		if node.Component != nil && component == nil {
			return prefix, nil
		}
		copy.Component = component
		copy.Attributes = append([]ir.JSXAttribute(nil), node.Attributes...)
		for index := range copy.Attributes {
			attributePrefix, value := n.expression(copy.Attributes[index].Value)
			prefix = append(prefix, attributePrefix...)
			if copy.Attributes[index].Value != nil && value == nil {
				return prefix, nil
			}
			copy.Attributes[index].Value = value
		}
		copy.Children = append([]ir.JSXChild(nil), node.Children...)
		for index, child := range copy.Children {
			switch item := child.(type) {
			case *ir.JSXElement:
				childPrefix, value := n.expression(item)
				prefix = append(prefix, childPrefix...)
				if value == nil {
					return prefix, nil
				}
				copy.Children[index] = value.(*ir.JSXElement)
			case *ir.JSXExpression:
				childPrefix, value := n.expression(item.Value)
				prefix = append(prefix, childPrefix...)
				if value == nil {
					return prefix, nil
				}
				copy.Children[index] = &ir.JSXExpression{Value: value}
			}
		}
		return prefix, &copy
	case *ir.Unary:
		prefix, operand := n.expression(node.Operand)
		if operand == nil {
			return prefix, nil
		}
		copy := *node
		copy.Operand = operand
		return n.finishExpression(prefix, &copy)
	case *ir.Conversion:
		prefix, value := n.expression(node.Value)
		if value == nil {
			return prefix, nil
		}
		copy := *node
		copy.Value = value
		return n.finishExpression(prefix, &copy)
	case *ir.Binary:
		prefix, left := n.expression(node.Left)
		if left == nil {
			return prefix, nil
		}
		rightPrefix, right := n.expression(node.Right)
		if right == nil {
			if isShortCircuitOperator(node.Operator) {
				return n.shortCircuitExpression(node, prefix, left, rightPrefix, nil)
			}
			prefix = append(prefix, n.evaluate(left)...)
			return append(prefix, rightPrefix...), nil
		}
		if len(rightPrefix) > 0 && isShortCircuitOperator(node.Operator) {
			return n.shortCircuitExpression(node, prefix, left, rightPrefix, right)
		}
		if len(rightPrefix) > 0 {
			materialized, stable := n.materialize(left)
			prefix = append(prefix, materialized...)
			left = stable
			prefix = append(prefix, rightPrefix...)
		}
		copy := *node
		copy.Left = left
		copy.Right = right
		return n.finishExpression(prefix, &copy)
	case *ir.Range:
		prefix, values, ok := n.expressions([]ir.Expression{node.Start, node.End})
		if !ok {
			return prefix, nil
		}
		copy := *node
		copy.Start = values[0]
		copy.End = values[1]
		return n.finishExpression(prefix, &copy)
	case *ir.RecordConstruct:
		prefix, target := n.expression(node.Target)
		if target == nil {
			return prefix, nil
		}
		copy := *node
		copy.Target = target
		copy.Arguments = append([]ir.CallArgument(nil), node.Arguments...)
		arguments := make([]ir.Expression, len(copy.Arguments))
		for index, argument := range copy.Arguments {
			arguments[index] = argument.Value
		}
		argumentPrefix, values, ok := n.expressions(arguments)
		prefix = append(prefix, argumentPrefix...)
		if !ok {
			return prefix, nil
		}
		for index, value := range values {
			copy.Arguments[index].Value = value
		}
		return n.finishExpression(prefix, &copy)
	case *ir.Call:
		if member, safe := safeCalleeMember(node.Callee); safe {
			return n.safeNavigationCall(node, member)
		}
		prefix, callee := n.expression(node.Callee)
		if callee == nil {
			return prefix, nil
		}
		copy := *node
		copy.Callee = callee
		copy.Arguments = append([]ir.CallArgument(nil), node.Arguments...)
		arguments := make([]ir.Expression, len(copy.Arguments))
		for index, argument := range copy.Arguments {
			arguments[index] = argument.Value
		}
		argumentPrefix, values, ok := n.expressions(arguments)
		if len(argumentPrefix) > 0 {
			if !ok {
				prefix = append(prefix, n.evaluateCallee(callee)...)
				prefix = append(prefix, argumentPrefix...)
				return prefix, nil
			}
			calleePrefix, stable := n.stabilizeCallee(callee)
			prefix = append(prefix, calleePrefix...)
			callee = stable
			prefix = append(prefix, argumentPrefix...)
		}
		for index, value := range values {
			copy.Arguments[index].Value = value
		}
		copy.Callee = callee
		if node.Block != nil {
			block := *node.Block
			block.Body = n.executableStatements(node.Block.Body)
			copy.Block = &block
		}
		return n.finishExpression(prefix, &copy)
	case *ir.EnumConstruct:
		copy := *node
		copy.Arguments = append([]ir.CallArgument(nil), node.Arguments...)
		arguments := make([]ir.Expression, len(copy.Arguments))
		for index, argument := range copy.Arguments {
			arguments[index] = argument.Value
		}
		prefix, values, ok := n.expressions(arguments)
		if !ok {
			return prefix, nil
		}
		for index, value := range values {
			copy.Arguments[index].Value = value
		}
		return n.finishExpression(prefix, &copy)
	case *ir.EnumCall:
		copy := *node
		prefix := []ir.Statement{}
		if node.Receiver != nil {
			receiverPrefix, receiver := n.expression(node.Receiver)
			prefix = append(prefix, receiverPrefix...)
			if receiver == nil {
				return prefix, nil
			}
			copy.Receiver = receiver
		}
		copy.Arguments = append([]ir.CallArgument(nil), node.Arguments...)
		arguments := make([]ir.Expression, len(copy.Arguments))
		for index, argument := range copy.Arguments {
			arguments[index] = argument.Value
		}
		argumentPrefix, values, ok := n.expressions(arguments)
		if len(argumentPrefix) > 0 && copy.Receiver != nil {
			if !ok {
				prefix = append(prefix, n.evaluate(copy.Receiver)...)
			} else {
				receiverPrefix, receiver := n.materialize(copy.Receiver)
				prefix = append(prefix, receiverPrefix...)
				copy.Receiver = receiver
			}
		}
		prefix = append(prefix, argumentPrefix...)
		if !ok {
			return prefix, nil
		}
		for index, value := range values {
			copy.Arguments[index].Value = value
		}
		return n.finishExpression(prefix, &copy)
	case *ir.TypeApply:
		prefix, receiver := n.expression(node.Receiver)
		if receiver == nil {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	case *ir.Member:
		if node.Safe {
			return n.safeNavigationMember(node)
		}
		prefix, receiver := n.expression(node.Receiver)
		if receiver == nil {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	case *ir.Index:
		prefix, values, ok := n.expressions([]ir.Expression{node.Receiver, node.Index})
		if !ok {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = values[0]
		copy.Index = values[1]
		return n.finishExpression(prefix, &copy)
	case *ir.Block:
		copy := *node
		copy.Body = n.executableStatements(node.Body)
		return nil, &copy
	case *ir.Transform:
		copy := *node
		expressions := []ir.Expression{node.Source}
		initialIndex := -1
		limitIndex := -1
		if node.Initial != nil {
			initialIndex = len(expressions)
			expressions = append(expressions, node.Initial)
		}
		if node.Limit != nil {
			limitIndex = len(expressions)
			expressions = append(expressions, node.Limit)
		}
		prefix, values, ok := n.expressions(expressions)
		if !ok {
			return prefix, nil
		}
		copy.Source = values[0]
		if initialIndex >= 0 {
			copy.Initial = values[initialIndex]
		}
		if limitIndex >= 0 {
			copy.Limit = values[limitIndex]
		}
		copy.Body = n.executableStatements(node.Body)
		resultType := transformResultSurrogateType(node)
		copy.Result = n.deferredExpression(node.Result, resultType)
		if !types.Equivalent(resultType, node.Result.ExprType()) {
			copy.Type = neverSurrogateType(node.ExprType())
		}
		return n.finishExpression(prefix, &copy)
	default:
		return nil, expression
	}
}

func isShortCircuitOperator(operator string) bool {
	return operator == "and" || operator == "&&" || operator == "or" || operator == "||"
}

func (n *controlFlowNormalizer) shortCircuitExpression(node *ir.Binary, prefix []ir.Statement, left ir.Expression, rightPrefix []ir.Statement, right ir.Expression) ([]ir.Statement, ir.Expression) {
	boolean := types.FromName("Boolean")
	trueValue := &ir.Literal{ExprBase: ir.NewExprBase(node.Span, boolean), Kind: "boolean", Raw: "true"}
	falseValue := &ir.Literal{ExprBase: ir.NewExprBase(node.Span, boolean), Kind: "boolean", Raw: "false"}
	flow := &ir.If{ExprBase: ir.NewExprBase(node.Span, boolean), Condition: left, HasElse: true}
	if node.Operator == "and" || node.Operator == "&&" {
		flow.Then = rightPrefix
		flow.ThenResult = right
		flow.ThenDiverges = right == nil
		flow.ElseResult = falseValue
	} else {
		flow.ThenResult = trueValue
		flow.Else = rightPrefix
		flow.ElseResult = right
		flow.ElseDiverges = right == nil
	}
	flowPrefix, value := n.ifExpression(flow)
	return append(prefix, flowPrefix...), value
}

func (n *controlFlowNormalizer) expressions(input []ir.Expression) ([]ir.Statement, []ir.Expression, bool) {
	prefix := []ir.Statement{}
	result := make([]ir.Expression, len(input))
	for index, expression := range input {
		expressionPrefix, value := n.expression(expression)
		if len(expressionPrefix) > 0 {
			if value == nil {
				for previous := 0; previous < index; previous++ {
					prefix = append(prefix, n.evaluate(result[previous])...)
				}
				prefix = append(prefix, expressionPrefix...)
				return prefix, nil, false
			}
			for previous := 0; previous < index; previous++ {
				materialized, stable := n.materialize(result[previous])
				prefix = append(prefix, materialized...)
				result[previous] = stable
			}
			prefix = append(prefix, expressionPrefix...)
		}
		if value == nil {
			return prefix, nil, false
		}
		result[index] = value
	}
	return prefix, result, true
}
