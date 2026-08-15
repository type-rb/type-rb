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

func TestNarrowedNullableReceiverPreservesMemberPrecedence(t *testing.T) {
	nullableFormat := types.Type{Kind: types.Named, Name: "MemberFormat", Nullable: true}
	format := nullableFormat
	format.Nullable = false
	receiver := &ir.Identifier{
		ExprBase: ir.NewExprBase(token.Span{}, nullableFormat),
		Name:     "format",
		Lexical:  true,
	}
	narrowed := &ir.Conversion{
		ExprBase: ir.NewExprBase(token.Span{}, format),
		Kind:     ir.NullableToNonNullableConversion,
		Value:    receiver,
	}
	name := &ir.Member{
		ExprBase: ir.NewExprBase(token.Span{}, types.FromName("String")),
		Receiver: narrowed,
		Name:     "name",
	}
	program := &ir.Program{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/nullable",
		Statements: []ir.Statement{&ir.Method{
			Name: "read", Parameters: []ir.Parameter{{Name: "format", Type: nullableFormat}},
			SuccessType: types.FromName("String"), ReturnType: types.FromName("String"),
			Body: []ir.Statement{&ir.Return{Value: name}},
		}},
	}

	output := Generate(program)
	if !strings.Contains(output, "return (*(format)).Name") {
		t.Fatalf("nullable receiver was not parenthesized before member access:\n%s", output)
	}
	if _, err := parser.ParseFile(gotoken.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
		t.Fatalf("generated invalid Go: %v\n%s", err, output)
	}
}
