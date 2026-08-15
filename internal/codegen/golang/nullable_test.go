package golang

import (
	"go/parser"
	gotoken "go/token"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestLocalNullableVariableInitializedWithTypedNil(t *testing.T) {
	nullableString := types.FromName("String")
	nullableString.Nullable = true
	program := &ir.Program{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/nullable",
		Statements: []ir.Statement{&ir.Method{
			Name: "main", SuccessType: types.FromName("Void"), ReturnType: types.FromName("Void"),
			Body: []ir.Statement{&ir.Variable{
				Name: "value", Type: nullableString, Mutable: true,
				Value: &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("Nil")), Kind: "nil", Raw: "nil"},
			}},
		}},
	}

	output := Generate(program)
	if !strings.Contains(output, "value := (*string)(nil)") {
		t.Fatalf("nullable nil initializer was not emitted with its declared type:\n%s", output)
	}
	if _, err := parser.ParseFile(gotoken.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid Go: %v\n%s", err, output)
	}
}
