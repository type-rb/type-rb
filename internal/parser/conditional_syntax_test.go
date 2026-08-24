package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseConditionalExpressionAndTransfers(t *testing.T) {
	program, diagnostics := Parse([]byte(`def choose(ready: Boolean): String
	label := ready ? "ready" : "waiting"
	return label if ready
	while ready
		next if false
		break if true
	end
	return "waiting"
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	variable := method.Body[0].(*ast.VariableStatement)
	conditional, ok := variable.Value.(*ast.IfStatement)
	if !ok || !conditional.Ternary || !conditional.HasElse || len(conditional.Then) != 1 || len(conditional.Else) != 1 {
		t.Fatalf("conditional=%#v", variable.Value)
	}
	guard, ok := method.Body[1].(*ast.IfStatement)
	if !ok || !guard.ConditionalTransfer {
		t.Fatalf("conditional return=%#v", method.Body[1])
	}
	if _, ok := guard.Then[0].(*ast.ReturnStatement); !ok {
		t.Fatalf("conditional return body=%T", guard.Then[0])
	}
	loop := method.Body[2].(*ast.WhileStatement)
	for index, expected := range []any{&ast.NextStatement{}, &ast.BreakStatement{}} {
		transfer, ok := loop.Body[index].(*ast.IfStatement)
		if !ok || !transfer.ConditionalTransfer || len(transfer.Then) != 1 {
			t.Fatalf("conditional loop transfer[%d]=%#v", index, loop.Body[index])
		}
		switch expected.(type) {
		case *ast.NextStatement:
			if _, ok := transfer.Then[0].(*ast.NextStatement); !ok {
				t.Fatalf("next body=%T", transfer.Then[0])
			}
		case *ast.BreakStatement:
			if _, ok := transfer.Then[0].(*ast.BreakStatement); !ok {
				t.Fatalf("break body=%T", transfer.Then[0])
			}
		}
	}
}

func TestParseConditionalExpressionAfterPredicateCall(t *testing.T) {
	program, diagnostics := Parse([]byte("value := ready?() ? 1 : 2\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	conditional := program.Statements[0].(*ast.VariableStatement).Value.(*ast.IfStatement)
	call, ok := conditional.Condition.(*ast.CallExpression)
	if !ok {
		t.Fatalf("condition=%T", conditional.Condition)
	}
	callee, ok := call.Callee.(*ast.Identifier)
	if !ok || callee.Name != "ready?" {
		t.Fatalf("callee=%#v", call.Callee)
	}
}

func TestParseConditionalExpressionRequiresParenthesizedNesting(t *testing.T) {
	_, diagnostics := Parse([]byte("value := first ? second ? 1 : 2 : 3\n"))
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "nested conditional expressions must be parenthesized") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}

	program, diagnostics := Parse([]byte("value := first ? (second ? 1 : 2) : 3\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("parenthesized diagnostics=%v", diagnostics)
	}
	outer := program.Statements[0].(*ast.VariableStatement).Value.(*ast.IfStatement)
	inner := outer.Then[0].(*ast.ExpressionStatement).Expression.(*ast.IfStatement)
	if !inner.TernaryParenthesized {
		t.Fatalf("parenthesized nested conditional was not retained: %#v", inner)
	}
}

func TestParseRejectsGeneralTrailingCondition(t *testing.T) {
	for _, source := range []string{"notify() if ready\n", "value := fallback if ready\n", "value = fallback if ready\n"} {
		_, diagnostics := Parse([]byte(source))
		if len(diagnostics) != 1 || diagnostics[0].Message != "trailing if is only allowed on return, break, or next" {
			t.Fatalf("source=%q diagnostics=%v", source, diagnostics)
		}
	}

	_, diagnostics := Parse([]byte("return value unless ready\n"))
	if len(diagnostics) != 1 || diagnostics[0].Message != "conditional transfers use trailing if; unless is not supported" {
		t.Fatalf("unless diagnostics=%v", diagnostics)
	}
}
