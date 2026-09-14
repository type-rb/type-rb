package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestControlExpressionSpanExcludesEnclosingCall(t *testing.T) {
	for _, construct := range []string{"if ready\n1\nelse\n2\nend", "case value\nwhen 1\n1\nelse\n2\nend"} {
		source := "chosen := pair(" + construct + ", 3)\n"
		program, diagnostics := Parse([]byte(source))
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		variable, ok := program.Statements[0].(*ast.VariableStatement)
		if !ok {
			t.Fatalf("expected variable, got %T", program.Statements[0])
		}
		call, ok := variable.Value.(*ast.CallExpression)
		if !ok || len(call.Arguments) != 2 {
			t.Fatalf("enclosing call was lost: %#v", variable.Value)
		}
		control := call.Arguments[0].Value
		if got, want := control.Span().End.Offset, strings.Index(source, ", 3)"); got != want {
			t.Fatalf("control end=%d, want %d before the remaining argument", got, want)
		}
	}
}
