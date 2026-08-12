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

func TestParseFallibleLambdaAndFunctionType(t *testing.T) {
	program, diagnostics := Parse([]byte(`loader: () -> String fails LoadError := fn(): String fails LoadError
	return read()
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	variable := program.Statements[0].(*ast.VariableStatement)
	if variable.Type.String() != "() -> String fails LoadError" {
		t.Fatalf("type=%s", variable.Type.String())
	}
	lambda := variable.Value.(*ast.LambdaExpression)
	if lambda.ReturnType.String() != "String" || lambda.Fails.String() != "LoadError" {
		t.Fatalf("lambda=%#v", lambda)
	}
}
