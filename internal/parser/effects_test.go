package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseRemovedAttemptAroundCallBlockForRecovery(t *testing.T) {
	program, diagnostics := Parse([]byte(`result := attempt users.find_each do |user|
	puts(user)
end
`))
	if len(diagnostics) != 1 || diagnostics[0].Message != attemptRemovedMessage {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if diagnostics[0].Span.Start.Line != 1 || diagnostics[0].Span.Start.Column != 11 || diagnostics[0].Span.End.Column != 18 {
		t.Fatalf("diagnostic span=%#v", diagnostics[0].Span)
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

func TestParseRemovedMethodFailsForRecovery(t *testing.T) {
	program, diagnostics := Parse([]byte(`def save() fails DbError
	return
end
`))
	if len(diagnostics) != 1 || diagnostics[0].Message != failsRemovedMessage {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if diagnostics[0].Span.Start.Line != 1 || diagnostics[0].Span.Start.Column != 12 || diagnostics[0].Span.End.Column != 17 {
		t.Fatalf("diagnostic span=%#v", diagnostics[0].Span)
	}
	method, ok := program.Statements[0].(*ast.MethodStatement)
	if !ok || method.Fails.Name != "DbError" || !method.ReturnType.Empty() {
		t.Fatalf("method=%#v", program.Statements[0])
	}
}

func TestParseRemovedDirectAndBlockAttempt(t *testing.T) {
	source := []byte(`direct := attempt read_value()
grouped := attempt do
	read_value()
end
`)
	_, diagnostics := Parse(source)
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	for index, expectedColumn := range []int{11, 12} {
		item := diagnostics[index]
		if item.Message != attemptRemovedMessage || item.Span.Start.Line != index+1 || item.Span.Start.Column != expectedColumn || item.Span.End.Column != expectedColumn+len("attempt") {
			t.Fatalf("diagnostic[%d]=%#v", index, item)
		}
	}
}

func TestParseAttemptAndFailsAsOrdinaryIdentifiers(t *testing.T) {
	program, diagnostics := Parse([]byte(`attempt := operation()
called := attempt()
indexed := attempt[0]
combined := attempt + 1
member := service.attempt()
fails := recover_value()
checked := fails()
other := service.fails()
typed: fails := recover_value()
def fails(attempt: String, fails: String): String
	return attempt
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	if len(program.Statements) != 10 {
		t.Fatalf("statements=%d", len(program.Statements))
	}
	called := program.Statements[1].(*ast.VariableStatement).Value
	call, ok := called.(*ast.CallExpression)
	if !ok {
		t.Fatalf("attempt()=%T", called)
	}
	callee, ok := call.Callee.(*ast.Identifier)
	if !ok || callee.Name != "attempt" {
		t.Fatalf("attempt() callee=%#v", call.Callee)
	}
	typed := program.Statements[8].(*ast.VariableStatement)
	if typed.Type.Name != "fails" {
		t.Fatalf("ordinary fails type=%#v", typed.Type)
	}
	method, ok := program.Statements[9].(*ast.MethodStatement)
	if !ok || method.Name != "fails" || len(method.Parameters) != 2 || method.Parameters[0].Name != "attempt" || method.Parameters[1].Name != "fails" {
		t.Fatalf("method=%#v", program.Statements[9])
	}
}

func TestParseRemovedAttemptInInterpolationReportsSourceSpan(t *testing.T) {
	_, diagnostics := Parse([]byte("text := \"value #{attempt read_value()}\"\n"))
	if len(diagnostics) != 1 || diagnostics[0].Message != attemptRemovedMessage {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	span := diagnostics[0].Span
	if span.Start.Line != 1 || span.Start.Column != 18 || span.End.Column != 25 || span.Start.Offset != 17 || span.End.Offset != 24 {
		t.Fatalf("diagnostic span=%#v", span)
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
