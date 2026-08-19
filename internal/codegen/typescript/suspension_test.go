package typescript

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
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
		"export async function load(__trbScope: AbortSignal | undefined): Promise<Result<Array<Product>, DbError>>",
		"async function main(__trbScope: AbortSignal | undefined): Promise<void>",
	} {
		if !strings.Contains(strings.Join(generated, "\n"), want) {
			t.Fatalf("generated project is missing %q:\n%s", want, generated[index])
		}
	}
	if !strings.Contains(generated[1], "export async function forward(__trbScope: AbortSignal | undefined): Promise<Result<Array<Product>, DbError>>") ||
		!strings.Contains(generated[1], "return (await load(__trbScope));") ||
		!strings.Contains(generated[1], "await main(undefined);") {
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

func TestSuspendingResultLambdaIsAllowedOnlyAtPromiseRejectionBridge(t *testing.T) {
	integer := types.FromName("Integer")
	errorType := types.FromName("LoadError")
	resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{integer, errorType}}
	functionType := types.FunctionOf(nil, resultType)
	nativeType := types.FunctionOf(nil, integer)
	request := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, resultType),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(token.Span{}, resultType),
			Name:      "request",
			Reference: &ir.Reference{Intrinsic: "trb.platform.typescript.browser.request"},
		},
	}
	lambda := &ir.Lambda{
		ExprBase:    ir.NewExprBase(token.Span{}, functionType),
		SuccessType: resultType,
		ReturnType:  resultType,
		Fails:       types.Type{Kind: types.Never, Name: "Never"},
		Body:        []ir.Statement{&ir.Return{Value: request}},
	}
	variable := &ir.Variable{Name: "loader", Type: functionType, Value: lambda}
	identifier := &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, functionType), Name: "loader", Lexical: true}
	bridge := &ir.Conversion{
		ExprBase: ir.NewExprBase(token.Span{}, nativeType),
		Kind:     ir.ResultFunctionToPromiseRejectionConversion,
		Value:    identifier,
	}
	program := &ir.Program{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{variable, &ir.ExpressionStatement{Expression: bridge}}}
	plan, err := AnalyzeSuspension([]*ir.Program{program})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Lambdas[lambda] || !plan.ResultPromiseBridges[lambda] {
		t.Fatalf("suspending Result lambda was not associated with its native bridge: %#v", plan)
	}

	unbridged := &ir.Program{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{variable}}
	if _, err := AnalyzeSuspension([]*ir.Program{unbridged}); err == nil || !strings.Contains(err.Error(), "may suspend must omit") {
		t.Fatalf("expected an unbridged suspending Result lambda diagnostic, got %v", err)
	}
}

func TestORMRejectsUnsupportedTypeScriptRuntimes(t *testing.T) {
	for _, runtime := range []string{"", "browser", "node"} {
		program := &ir.Program{
			Mode:              "typescript",
			ModulePath:        "main",
			TypeScriptRuntime: runtime,
			Extensions:        []ir.Extension{&ormintegration.Manifest{}},
		}
		if err := ValidateProject([]*ir.Program{program}); err == nil || !strings.Contains(err.Error(), `typescript.runtime: "bun"`) {
			t.Fatalf("validation for runtime %q returned %v", runtime, err)
		}
		if _, err := GenerateProject([]*ir.Program{program}); err == nil || !strings.Contains(err.Error(), `typescript.runtime: "bun"`) {
			t.Fatalf("generation for runtime %q returned %v", runtime, err)
		}
	}
	program := &ir.Program{Mode: "typescript", ModulePath: "main", TypeScriptRuntime: "bun", Extensions: []ir.Extension{&ormintegration.Manifest{}}}
	if err := ValidateProject([]*ir.Program{program}); err != nil {
		t.Fatalf("Bun runtime validation failed: %v", err)
	}
	if _, err := GenerateProject([]*ir.Program{program}); err != nil {
		t.Fatalf("Bun runtime was rejected: %v", err)
	}
	repl := &ir.Program{Mode: "typescript", ModulePath: "__trb_repl__", TypeScriptRuntime: "node"}
	project := &ir.Program{Mode: "typescript", ModulePath: "main", TypeScriptRuntime: "node", Extensions: []ir.Extension{&ormintegration.Manifest{}}}
	if err := ValidateProject([]*ir.Program{project, repl}); err != nil {
		t.Fatalf("shared-host REPL project validation failed: %v", err)
	}
	if _, err := GenerateProject([]*ir.Program{project, repl}); err != nil {
		t.Fatalf("shared-host REPL project was rejected: %v", err)
	}
}

func TestBooleanHashKeysUseJavaScriptPropertyKeys(t *testing.T) {
	typ := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Boolean"), types.FromName("Integer")}}
	if generated := tsType(typ); generated != "Record<string, number>" {
		t.Fatalf("boolean-keyed Hash generated %q", generated)
	}
}
