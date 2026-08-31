package codegen

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestNonNullableSafeFunctionFieldCallKeepsCallResultType(t *testing.T) {
	span := token.Span{}
	stringType := types.FromName("String")
	receiverType := types.FromName("CallbackHolder")
	functionType := types.FunctionOf([]types.Type{stringType}, stringType)
	receiver := &ir.Identifier{
		ExprBase: ir.NewExprBase(span, receiverType), Name: "holder", Lexical: true,
	}
	member := &ir.Member{
		ExprBase: ir.NewExprBase(span, functionType), Receiver: receiver, Name: "callback",
		Safe: true, PresentType: functionType,
	}
	call := &ir.Call{
		ExprBase: ir.NewExprBase(span, stringType), Callee: member, PresentType: stringType,
		Arguments: []ir.CallArgument{{Value: &ir.Literal{
			ExprBase: ir.NewExprBase(span, stringType), Kind: "string", Raw: "value",
		}}},
	}
	program := normalizeDivergingControlFlow(&ir.Program{
		Statements: []ir.Statement{&ir.ExpressionStatement{Expression: call}},
	})
	statement, ok := program.Statements[0].(*ir.ExpressionStatement)
	if !ok {
		t.Fatalf("normalized statement is %T, want *ir.ExpressionStatement", program.Statements[0])
	}
	normalized, ok := statement.Expression.(*ir.Call)
	if !ok {
		t.Fatalf("normalized expression is %T, want *ir.Call", statement.Expression)
	}
	if !types.Equivalent(normalized.ExprType(), stringType) {
		t.Fatalf("normalized call type is %s, want String", normalized.ExprType())
	}
	normalizedMember, ok := normalized.Callee.(*ir.Member)
	if !ok || normalizedMember.Safe {
		t.Fatalf("normalized callee did not clear safe navigation: %#v", normalized.Callee)
	}
}
