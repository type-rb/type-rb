package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseTypedLambdaAndFunctionType(t *testing.T) {
	program, diagnostics := Parse([]byte(`formatter: (Integer) -> String := fn(value: Integer): String
	return value.to_s()
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	if variable.Type.String() != "(Integer) -> String" {
		t.Fatalf("type=%s", variable.Type.String())
	}
	lambda, ok := variable.Value.(*ast.LambdaExpression)
	if !ok || len(lambda.Parameters) != 1 || lambda.Parameters[0].Name != "value" || lambda.ReturnType.String() != "String" {
		t.Fatalf("lambda=%#v", variable.Value)
	}
}

func TestParseInlineSemicolonLambda(t *testing.T) {
	program, diagnostics := Parse([]byte("identity := fn(value: Integer): Integer; return value; end\n"))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable, ok := program.Statements[0].(*ast.VariableStatement)
	if !ok {
		t.Fatalf("statement=%T", program.Statements[0])
	}
	lambda, ok := variable.Value.(*ast.LambdaExpression)
	if !ok || len(lambda.Body) != 1 {
		t.Fatalf("lambda=%#v", variable.Value)
	}
}

func TestParseResultLambdaAndFunctionType(t *testing.T) {
	program, diagnostics := Parse([]byte(`loader: () -> Result<String, LoadError> := fn(): Result<String, LoadError>
	return read()
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable := program.Statements[0].(*ast.VariableStatement)
	if variable.Type.String() != "() -> Result<String, LoadError>" {
		t.Fatalf("type=%s", variable.Type.String())
	}
	lambda := variable.Value.(*ast.LambdaExpression)
	if lambda.ReturnType.String() != "Result<String, LoadError>" || !lambda.Fails.Empty() {
		t.Fatalf("lambda=%#v", lambda)
	}
}

func TestParseRemovedLambdaAndFunctionTypeFailsForRecovery(t *testing.T) {
	program, diagnostics := Parse([]byte(`loader: () -> String fails LoadError := fn(): String fails LoadError
	return read()
end
`))
	if len(diagnostics) != 2 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	wantColumns := map[int]bool{22: false, 54: false}
	for _, item := range diagnostics {
		if item.Message != failsRemovedMessage || item.Span.Start.Line != 1 || item.Span.End.Column != item.Span.Start.Column+len("fails") {
			t.Fatalf("diagnostic=%#v", item)
		}
		if _, ok := wantColumns[item.Span.Start.Column]; !ok {
			t.Fatalf("unexpected diagnostic column: %#v", item.Span)
		}
		wantColumns[item.Span.Start.Column] = true
	}
	for column, seen := range wantColumns {
		if !seen {
			t.Fatalf("missing diagnostic at column %d: %v", column, diagnostics)
		}
	}
	variable := program.Statements[0].(*ast.VariableStatement)
	if variable.Type.FunctionFails == nil {
		t.Fatalf("function type recovery=%#v", variable.Type)
	}
	if lambda, ok := variable.Value.(*ast.LambdaExpression); !ok || lambda.Fails.Empty() {
		t.Fatalf("lambda recovery=%#v", variable.Value)
	}
}
