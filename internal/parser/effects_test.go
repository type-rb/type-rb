package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseAttemptAroundCallBlock(t *testing.T) {
	program, diagnostics := Parse([]byte(`result := attempt users.find_each do |user|
	puts(user)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	attempt, ok := variable.Value.(*ast.AttemptExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	call, ok := attempt.Value.(*ast.CallExpression)
	if !ok || call.Block == nil || len(call.Block.Parameters) != 1 || call.Block.Parameters[0] != "user" {
		t.Fatalf("attempt call=%#v", attempt.Value)
	}
}

func TestParseFailsWithoutReturnType(t *testing.T) {
	program, diagnostics := Parse([]byte(`def save() fails DbError
	return
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method, ok := program.Statements[0].(*ast.MethodStatement)
	if !ok || method.Fails.Name != "DbError" || !method.ReturnType.Empty() {
		t.Fatalf("method=%#v", program.Statements[0])
	}
}
