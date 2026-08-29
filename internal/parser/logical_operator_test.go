package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseCanonicalLogicalOperators(t *testing.T) {
	program, diagnostics := Parse([]byte("value := !false && true || false\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	value := program.Statements[0].(*ast.VariableStatement).Value
	orExpression, ok := value.(*ast.BinaryExpression)
	if !ok || orExpression.Operator != "||" {
		t.Fatalf("value=%#v", value)
	}
	andExpression, ok := orExpression.Left.(*ast.BinaryExpression)
	if !ok || andExpression.Operator != "&&" {
		t.Fatalf("left=%#v", orExpression.Left)
	}
	unary, ok := andExpression.Left.(*ast.UnaryExpression)
	if !ok || unary.Operator != "!" {
		t.Fatalf("unary=%#v", andExpression.Left)
	}
}

func TestRejectsWordLogicalOperators(t *testing.T) {
	for _, test := range []struct {
		source  string
		message string
	}{
		{source: "value := true and false\n", message: "unexpected token and"},
		{source: "value := true or false\n", message: "unexpected token or"},
		{source: "value := not false\n", message: "unexpected token not"},
	} {
		_, diagnostics := Parse([]byte(test.source))
		if len(diagnostics) != 1 || diagnostics[0].Message != test.message {
			t.Fatalf("source=%q diagnostics=%v", test.source, diagnostics)
		}
	}
}

func TestWordLogicalNamesRemainAvailableAsMembers(t *testing.T) {
	_, diagnostics := Parse([]byte("value := query.not(active: false).or(other)\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
}
