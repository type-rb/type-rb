package typescript

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestSuspensionPropagatesOnlyThroughTypeScriptProjectGeneration(t *testing.T) {
	dbError := types.FromName("DbError")
	products := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Product")}}
	result := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{products, dbError}}
	loadCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, result),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(token.Span{}, result),
			Name:      "all",
			Reference: &ir.Reference{Intrinsic: "trb.orm.query.all", Symbol: "all"},
		},
		Fails: dbError,
	}
	load := &ir.Method{Name: "load", SuccessType: products, ReturnType: result, Fails: dbError, Body: []ir.Statement{&ir.Return{Value: loadCall}}}
	repository := &ir.Program{Mode: "typescript", ModulePath: "repository", Statements: []ir.Statement{load}}

	forwardCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, result),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(token.Span{}, result),
			Name:      "load",
			Reference: &ir.Reference{Package: "repository", Symbol: "load", ExportKind: "function"},
		},
		Fails: dbError,
	}
	forward := &ir.Method{Name: "forward", SuccessType: products, ReturnType: result, Fails: dbError, Body: []ir.Statement{&ir.Return{Value: forwardCall}}}
	mainCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, result),
		Callee:   &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, result), Name: "forward"},
		Fails:    dbError,
	}
	main := &ir.Method{Name: "main", SuccessType: types.FromName("Void"), ReturnType: types.FromName("Void"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: mainCall}}}
	application := &ir.Program{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{
		&ir.Import{Path: "repository", Symbols: []string{"load"}, SymbolKinds: map[string]string{"load": "function"}, IntrinsicSymbols: map[string]bool{}, RuntimeIndependentSymbols: map[string]bool{}},
		forward,
		main,
	}}

	generated, err := GenerateProject([]*ir.Program{repository, application})
	if err != nil {
		t.Fatal(err)
	}
	for index, want := range []string{
		"export async function load(): Promise<Result<Array<Product>, DbError>>",
		"async function main(): Promise<void>",
	} {
		if !strings.Contains(strings.Join(generated, "\n"), want) {
			t.Fatalf("generated project is missing %q:\n%s", want, generated[index])
		}
	}
	if !strings.Contains(generated[1], "export async function forward(): Promise<Result<Array<Product>, DbError>>") ||
		!strings.Contains(generated[1], "return (await load());") ||
		!strings.Contains(generated[1], "await main();") {
		t.Fatalf("suspension did not propagate through imported and entry calls:\n%s", generated[1])
	}
}

func TestPureTypeScriptFunctionsRemainSynchronous(t *testing.T) {
	integer := types.FromName("Integer")
	value := &ir.Method{Name: "value", SuccessType: integer, ReturnType: integer, Body: []ir.Statement{
		&ir.Return{Value: &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: "1"}},
	}}
	generated, err := GenerateProject([]*ir.Program{{Mode: "typescript", ModulePath: "values", Statements: []ir.Statement{value}}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(generated[0], "async function value") || !strings.Contains(generated[0], "export function value(): number") {
		t.Fatalf("pure function unexpectedly became asynchronous:\n%s", generated[0])
	}
}
