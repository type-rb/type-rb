package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestNestedFunctionBodyEndsBeforeEnclosingArguments(t *testing.T) {
	source := "invoke(fn(value: Integer): Integer; return value; end, 9)\n"
	program, diagnostics := Parse([]byte(source))
	if len(diagnostics) != 0 || len(program.NativeIslands) != 0 {
		t.Fatalf("function argument not parsed: %v", diagnostics)
	}
	call := program.Statements[0].(*ast.ExpressionStatement).Expression.(*ast.CallExpression)
	if len(call.Arguments) != 2 {
		t.Fatalf("function consumed following argument: %#v", call.Arguments)
	}
	function := call.Arguments[0].Value.(*ast.LambdaExpression)
	if got := source[function.Span().Start.Offset:function.Span().End.Offset]; got != "fn(value: Integer): Integer; return value; end" {
		t.Fatalf("function diagnostic span includes enclosing syntax: %q", got)
	}
}

func TestNestedFunctionValuesReportIncompleteSyntax(t *testing.T) {
	for _, source := range []string{
		"invoke(fn(value: Integer\n",
		"invoke(fn(): Integer; return 1\n",
	} {
		_, diagnostics := Parse([]byte(source))
		if len(diagnostics) == 0 {
			t.Fatalf("incomplete function accepted: %s", source)
		}
		for _, item := range diagnostics {
			if strings.Contains(item.Message, "unsupported") {
				t.Fatalf("function lost its structured diagnostic: %v", diagnostics)
			}
		}
	}
}
