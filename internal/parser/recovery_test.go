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

func TestParseMalformedCallExpressionFallsBackWithoutPanicking(t *testing.T) {
	program, diagnostics := Parse([]byte("E(;+O"))
	if len(diagnostics) != 0 {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
	if len(program.Statements) != 1 {
		t.Fatalf("statements=%#v", program.Statements)
	}
	statement, ok := program.Statements[0].(*ast.NativeStatement)
	if !ok || statement.Text != "E(;+O" {
		t.Fatalf("statement=%#v", program.Statements[0])
	}
}

func TestParseAliasAndNewtypeDeclarations(t *testing.T) {
	program, diagnostics := Parse([]byte("alias Users = Array<User>\nnewtype UserId = Integer\n"))
	if len(diagnostics) != 0 || len(program.Statements) != 2 {
		t.Fatalf("statements=%#v diagnostics=%v", program.Statements, diagnostics)
	}
	if alias, ok := program.Statements[0].(*ast.TypeAliasStatement); !ok || alias.Name != "Users" || alias.Target.Name != "Array" {
		t.Fatalf("alias=%#v", program.Statements[0])
	}
	if newtype, ok := program.Statements[1].(*ast.NewtypeStatement); !ok || newtype.Name != "UserId" || newtype.Target.Name != "Integer" {
		t.Fatalf("newtype=%#v", program.Statements[1])
	}
}

func TestParseLegacyTypeAliasReportsMigration(t *testing.T) {
	program, diagnostics := Parse([]byte("type UserId = Integer\n"))
	if len(program.Statements) != 1 {
		t.Fatalf("statements=%#v", program.Statements)
	}
	if _, ok := program.Statements[0].(*ast.TypeAliasStatement); !ok {
		t.Fatalf("recovered statement=%#v", program.Statements[0])
	}
	if len(diagnostics) != 1 || !strings.Contains(diagnostics[0].Message, "use alias") || !strings.Contains(diagnostics[0].Message, "or newtype") {
		t.Fatalf("diagnostics=%v", diagnostics)
	}
}
