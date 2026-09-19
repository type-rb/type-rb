package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestNestedFunctionAnnotationsPreserveTypeStructureAndOrigins(t *testing.T) {
	source := "callbacks: Array<Hash<String, (Integer, Array<String>) -> Void>> := []\n"
	program, diagnostics := Parse([]byte(source))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	ref := program.Statements[0].(*ast.VariableStatement).Type
	if got, want := ref.String(), "Array<Hash<String, (Integer, Array<String>) -> Void>>"; got != want {
		t.Fatalf("type=%q, want %q", got, want)
	}
	function := ref.Arguments[0].Arguments[1]
	if len(function.FunctionParameters) != 2 || function.FunctionReturn == nil || function.FunctionReturn.Name != "Void" {
		t.Fatalf("function type not retained: %#v", function)
	}
	if got, want := function.FunctionReturn.Span().Start.Column, strings.Index(source, "Void")+1; got != want {
		t.Fatalf("return type column=%d, want %d", got, want)
	}
}

func TestNestedFunctionAnnotationsRetainRemovedFailsDiagnostic(t *testing.T) {
	for _, source := range []string{
		"callback: Array<() -> String fails Error> := []\n",
		"callback: () -> Array<() -> String fails Error> := value\n",
	} {
		_, diagnostics := Parse([]byte(source))
		if len(diagnostics) != 1 || diagnostics[0].Message != failsRemovedMessage || diagnostics[0].Span.Start.Column != strings.Index(source, "fails")+1 {
			t.Fatalf("source=%q, diagnostics=%v", source, diagnostics)
		}
	}
}
