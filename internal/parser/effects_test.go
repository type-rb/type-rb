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

func TestParseTryExpression(t *testing.T) {
	program, diagnostics := Parse([]byte(`value := try read_value()
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	tryExpression, ok := variable.Value.(*ast.TryExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	if _, ok := tryExpression.Value.(*ast.CallExpression); !ok {
		t.Fatalf("try operand=%T", tryExpression.Value)
	}
}

func TestParseTryBindsBeforeBinaryOperators(t *testing.T) {
	program, diagnostics := Parse([]byte(`value := try read_value() + 1
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable := program.Statements[0].(*ast.VariableStatement)
	binary, ok := variable.Value.(*ast.BinaryExpression)
	if !ok || binary.Operator != "+" {
		t.Fatalf("value=%#v", variable.Value)
	}
	tryExpression, ok := binary.Left.(*ast.TryExpression)
	if !ok {
		t.Fatalf("binary left=%T", binary.Left)
	}
	if _, ok := tryExpression.Value.(*ast.CallExpression); !ok {
		t.Fatalf("try operand=%T", tryExpression.Value)
	}
}

func TestParseCatchExpression(t *testing.T) {
	program, diagnostics := Parse([]byte(`value := read_value() catch |error|
	return recover(error)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	catchExpression, ok := variable.Value.(*ast.CatchExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	if catchExpression.Binding.Name != "error" {
		t.Fatalf("binding=%#v", catchExpression.Binding)
	}
	if _, ok := catchExpression.Value.(*ast.CallExpression); !ok {
		t.Fatalf("catch operand=%T", catchExpression.Value)
	}
	if len(catchExpression.Body) != 1 {
		t.Fatalf("catch body=%#v", catchExpression.Body)
	}
	if _, ok := catchExpression.Body[0].(*ast.ReturnStatement); !ok {
		t.Fatalf("catch body statement=%T", catchExpression.Body[0])
	}
}

func TestParseCatchAfterCallBlock(t *testing.T) {
	program, diagnostics := Parse([]byte(`result := Database.transaction() do |tx|
	try save(tx)
end catch |error|
	return recover(error)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	catchExpression, ok := variable.Value.(*ast.CatchExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	call, ok := catchExpression.Value.(*ast.CallExpression)
	if !ok || call.Block == nil || len(call.Block.Parameters) != 1 || call.Block.Parameters[0] != "tx" {
		t.Fatalf("catch operand=%#v", catchExpression.Value)
	}
	if len(call.Block.Body) != 1 {
		t.Fatalf("call body=%#v", call.Block.Body)
	}
	statement, ok := call.Block.Body[0].(*ast.ExpressionStatement)
	if !ok {
		t.Fatalf("call body statement=%T", call.Block.Body[0])
	}
	if _, ok := statement.Expression.(*ast.TryExpression); !ok {
		t.Fatalf("call body expression=%T", statement.Expression)
	}
	if catchExpression.Binding.Name != "error" || len(catchExpression.Body) != 1 {
		t.Fatalf("catch=%#v", catchExpression)
	}
}

func TestParseTryAroundCallBlock(t *testing.T) {
	program, diagnostics := Parse([]byte(`result := try Database.transaction() do |tx|
	save(tx)
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable := program.Statements[0].(*ast.VariableStatement)
	tryExpression, ok := variable.Value.(*ast.TryExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	call, ok := tryExpression.Value.(*ast.CallExpression)
	if !ok || call.Block == nil || len(call.Block.Parameters) != 1 || call.Block.Parameters[0] != "tx" {
		t.Fatalf("try operand=%#v", tryExpression.Value)
	}
}

func TestParseCatchRequiresOneBinding(t *testing.T) {
	_, diagnostics := Parse([]byte(`value := read_value() catch |first, second|
	return first
end
`))
	if len(diagnostics) != 1 || diagnostics[0].Message != "catch binding must be written as |error|" {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}
