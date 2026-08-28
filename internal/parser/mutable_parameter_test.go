package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseMutableMethodAndFunctionValueParameters(t *testing.T) {
	program, diagnostics := Parse([]byte(`def update(value: Integer, *, mut count: Integer = 1): Integer
	return count
end

callable := fn(mut value: Integer): Integer
	return value
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	if method.Parameters[0].Mutable || !method.Parameters[1].Mutable || !method.Parameters[1].NamedOnly {
		t.Fatalf("method parameter mutability was not preserved: %#v", method.Parameters)
	}
	variable := program.Statements[1].(*ast.VariableStatement)
	lambda := variable.Value.(*ast.LambdaExpression)
	if len(lambda.Parameters) != 1 || !lambda.Parameters[0].Mutable {
		t.Fatalf("fn parameter mutability was not preserved: %#v", lambda.Parameters)
	}
}

func TestParseRejectsMalformedMutableParameters(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{source: "def invalid(mut)\n\treturn\nend\n", want: "mut parameter requires a name"},
		{source: "def invalid(mut: Integer)\n\treturn\nend\n", want: "parameter name must be an identifier"},
		{source: "def invalid(mut mut value: Integer)\n\treturn\nend\n", want: "parameter may declare mut only once"},
	} {
		_, diagnostics := Parse([]byte(test.source))
		found := false
		for _, item := range diagnostics {
			if strings.Contains(item.Message, test.want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("%q: expected %q, got %v", test.source, test.want, diagnostics)
		}
	}
}
