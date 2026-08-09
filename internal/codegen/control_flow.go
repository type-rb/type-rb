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
	temporary int
	reserved  map[string]bool
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
		case *ir.NativeBlock:
			n.reserveStatements(node.Body)
		}
	}
}

func (n *controlFlowNormalizer) statements(input []ir.Statement) []ir.Statement {
	result := make([]ir.Statement, 0, len(input))
	for _, statement := range input {
		result = append(result, n.statement(statement)...)
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
		copy.Body = n.statements(node.Body)
		return []ir.Statement{&copy}
	case *ir.Field:
		copy := *node
		prefix, value := n.expression(node.Value)
		if value == nil && node.Value != nil {
			return prefix
		}
		copy.Value = value
		return append(prefix, &copy)
	case *ir.Variable:
		prefix, value := n.expression(node.Value)
		if value == nil {
			return prefix
		}
		copy := *node
		copy.Value = value
		return append(prefix, &copy)
	case *ir.Assignment:
		prefix, target := n.expression(node.Target)
		if target == nil {
			return prefix
		}
		valuePrefix, value := n.expression(node.Value)
		prefix = append(prefix, valuePrefix...)
		if value == nil {
			return prefix
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
		prefix, source := n.expression(node.Source)
		if source == nil {
			return prefix
		}
		sizePrefix, size := n.expression(node.SliceSize)
		prefix = append(prefix, sizePrefix...)
		if node.SliceSize != nil && size == nil {
			return prefix
		}
		copy := *node
		copy.Source = source
		copy.SliceSize = size
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
	if !hasConditionPrefix && !containsTransferIf(&copy) {
		return prefix, &copy
	}
	copy.Type = types.FromName("Void")
	if node.ExprType().Kind == types.Never {
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
	if !containsTransferCase(&copy) {
		return leading, &copy
	}
	copy.Type = types.FromName("Void")
	if node.ExprType().Kind == types.Never {
		clearCaseResults(&copy, nil)
		return append(leading, &copy), nil
	}
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
			if containsEscapingTransfer(node.Body, loopDepth+1) {
				return true
			}
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
	case *ir.If:
		return n.ifExpression(node)
	case *ir.Case:
		return n.caseExpression(node)
	case *ir.InterpolatedString:
		copy := *node
		copy.Parts = append([]ir.StringPart(nil), node.Parts...)
		prefix := []ir.Statement{}
		for index := range copy.Parts {
			partPrefix, value := n.expression(copy.Parts[index].Expression)
			prefix = append(prefix, partPrefix...)
			if copy.Parts[index].Expression != nil && value == nil {
				return prefix, nil
			}
			copy.Parts[index].Expression = value
		}
		return prefix, &copy
	case *ir.Array:
		copy := *node
		prefix, values, ok := n.expressions(node.Elements)
		if !ok {
			return prefix, nil
		}
		copy.Elements = values
		return prefix, &copy
	case *ir.Hash:
		copy := *node
		copy.Entries = make([]ir.HashEntry, len(node.Entries))
		prefix := []ir.Statement{}
		for index, entry := range node.Entries {
			keyPrefix, key := n.expression(entry.Key)
			prefix = append(prefix, keyPrefix...)
			if key == nil {
				return prefix, nil
			}
			valuePrefix, value := n.expression(entry.Value)
			prefix = append(prefix, valuePrefix...)
			if value == nil {
				return prefix, nil
			}
			copy.Entries[index] = ir.HashEntry{Key: key, Value: value}
		}
		return prefix, &copy
	case *ir.Unary:
		prefix, operand := n.expression(node.Operand)
		if operand == nil {
			return prefix, nil
		}
		copy := *node
		copy.Operand = operand
		return prefix, &copy
	case *ir.Conversion:
		prefix, value := n.expression(node.Value)
		if value == nil {
			return prefix, nil
		}
		copy := *node
		copy.Value = value
		return prefix, &copy
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
			return append(prefix, rightPrefix...), nil
		}
		if len(rightPrefix) > 0 && isShortCircuitOperator(node.Operator) {
			return n.shortCircuitExpression(node, prefix, left, rightPrefix, right)
		}
		prefix = append(prefix, rightPrefix...)
		copy := *node
		copy.Left = left
		copy.Right = right
		return prefix, &copy
	case *ir.Range:
		prefix, start := n.expression(node.Start)
		if start == nil {
			return prefix, nil
		}
		endPrefix, end := n.expression(node.End)
		prefix = append(prefix, endPrefix...)
		if end == nil {
			return prefix, nil
		}
		copy := *node
		copy.Start = start
		copy.End = end
		return prefix, &copy
	case *ir.Call:
		prefix, callee := n.expression(node.Callee)
		if callee == nil {
			return prefix, nil
		}
		copy := *node
		copy.Callee = callee
		copy.Arguments = append([]ir.CallArgument(nil), node.Arguments...)
		for index := range copy.Arguments {
			argumentPrefix, value := n.expression(copy.Arguments[index].Value)
			prefix = append(prefix, argumentPrefix...)
			if value == nil {
				return prefix, nil
			}
			copy.Arguments[index].Value = value
		}
		return prefix, &copy
	case *ir.EnumConstruct:
		copy := *node
		prefix, values, ok := n.expressions(node.Arguments)
		if !ok {
			return prefix, nil
		}
		copy.Arguments = values
		return prefix, &copy
	case *ir.TypeApply:
		prefix, receiver := n.expression(node.Receiver)
		if receiver == nil {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	case *ir.Member:
		prefix, receiver := n.expression(node.Receiver)
		if receiver == nil {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = receiver
		return prefix, &copy
	case *ir.Index:
		prefix, receiver := n.expression(node.Receiver)
		if receiver == nil {
			return prefix, nil
		}
		indexPrefix, index := n.expression(node.Index)
		prefix = append(prefix, indexPrefix...)
		if index == nil {
			return prefix, nil
		}
		copy := *node
		copy.Receiver = receiver
		copy.Index = index
		return prefix, &copy
	case *ir.Block:
		copy := *node
		copy.Body = n.statements(node.Body)
		return nil, &copy
	case *ir.Transform:
		copy := *node
		prefix, source := n.expression(node.Source)
		if source == nil {
			return prefix, nil
		}
		initialPrefix, initial := n.expression(node.Initial)
		prefix = append(prefix, initialPrefix...)
		if node.Initial != nil && initial == nil {
			return prefix, nil
		}
		copy.Source = source
		copy.Initial = initial
		copy.Body = n.statements(node.Body)
		resultPrefix, result := n.expression(node.Result)
		copy.Body = append(copy.Body, resultPrefix...)
		if result == nil {
			return prefix, nil
		}
		copy.Result = result
		return prefix, &copy
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
		prefix = append(prefix, expressionPrefix...)
		if value == nil {
			return prefix, nil, false
		}
		result[index] = value
	}
	return prefix, result, true
}
