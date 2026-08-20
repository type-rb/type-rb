package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseMethodRecoversFromUnclosedParameterList(t *testing.T) {
	for _, source := range []string{"def test(", "def test(value: String"} {
		t.Run(source, func(t *testing.T) {
			program, diagnostics := Parse([]byte(source))
			found := false
			for _, item := range diagnostics {
				if item.Message == "unclosed parameter list" {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("diagnostics=%v", diagnostics)
			}
			if len(program.Statements) != 1 {
				t.Fatalf("statements=%#v", program.Statements)
			}
			method, ok := program.Statements[0].(*ast.MethodStatement)
			if !ok || method.Name != "test" {
				t.Fatalf("method=%#v", program.Statements[0])
			}
			if strings.Contains(source, "value") && (len(method.Parameters) != 1 || method.Parameters[0].Name != "value") {
				t.Fatalf("parameters=%#v", method.Parameters)
			}
		})
	}
}
