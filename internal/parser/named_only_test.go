package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseBareStarNamedOnlyParameters(t *testing.T) {
	program, diagnostics := Parse([]byte(`def request(url: String, *, timeout: Integer, retries: Integer = 2): String
	return url
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	if len(method.Parameters) != 3 || method.Parameters[0].NamedOnly || !method.Parameters[1].NamedOnly || !method.Parameters[2].NamedOnly {
		t.Fatalf("unexpected parameter regions: %#v", method.Parameters)
	}
	if method.Parameters[1].Name != "timeout" || method.Parameters[1].Default != nil || method.Parameters[2].Default == nil {
		t.Fatalf("named-only presence and default expression were not preserved: %#v", method.Parameters)
	}
}

func TestParseRejectsRemovedDoubleColonParameterSyntax(t *testing.T) {
	_, diagnostics := Parse([]byte("def describe(label:: String): String\n\treturn label\nend\n"))
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "name:: Type was removed") {
		t.Fatalf("expected :: migration diagnostic, got %v", diagnostics)
	}
}
