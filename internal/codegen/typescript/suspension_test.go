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
	}
	load := &ir.Method{Name: "load", ReturnType: result, Body: []ir.Statement{&ir.Return{Value: loadCall}}}
	repository := &ir.Program{Mode: "typescript", ModulePath: "repository", Statements: []ir.Statement{load}}

	forwardCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, result),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(token.Span{}, result),
			Name:      "load",
			Reference: &ir.Reference{Package: "repository", Symbol: "load", ExportKind: "function"},
		},
	}
	forward := &ir.Method{Name: "forward", ReturnType: result, Body: []ir.Statement{&ir.Return{Value: forwardCall}}}
	mainCall := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, result),
		Callee:   &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, result), Name: "forward"},
	}
	main := &ir.Method{Name: "main", ReturnType: types.FromName("Void"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: mainCall}}}
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
	value := &ir.Method{Name: "value", ReturnType: integer, Body: []ir.Statement{
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

func TestORMResultStreamingIntrinsicsSuspendWithoutLegacyEffects(t *testing.T) {
	for _, intrinsic := range []string{"trb.orm.query.find_each", "trb.orm.query.find_in_batches"} {
		t.Run(intrinsic, func(t *testing.T) {
			iteration := &ir.Iterate{Operation: "stream", Intrinsic: intrinsic}
			method := &ir.Method{
				Name:       "stream",
				ReturnType: types.FromName("Void"),
				Body:       []ir.Statement{iteration},
			}
			plan, err := AnalyzeSuspension([]*ir.Program{{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{method}}})
			if err != nil {
				t.Fatal(err)
			}
			if !plan.Methods[method] || !plan.Iterations[iteration] {
				t.Fatalf("Result streaming intrinsic %s was not classified as suspending: %#v", intrinsic, plan)
			}
		})
	}
}

func TestORMTransactionSuspendsEvenWhenItsBlockIsPure(t *testing.T) {
	block := &ir.StructuredBlock{Intrinsic: "trb.orm.transaction", Call: &ir.Call{}}
	method := &ir.Method{
		Name:       "transaction",
		ReturnType: types.FromName("Void"),
		Body:       []ir.Statement{block},
	}
	plan, err := AnalyzeSuspension([]*ir.Program{{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{method}}})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Methods[method] || !plan.StructuredBlocks[block] {
		t.Fatalf("transaction lifecycle was not classified as suspending: %#v", plan)
	}
}

func TestSuspendingResultLambdaUsesPortableBoundaryWithoutRequiringNativeBridge(t *testing.T) {
	integer := types.FromName("Integer")
	errorType := types.FromName("LoadError")
	resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{integer, errorType}}
	functionType := types.FunctionOf(nil, resultType)
	request := &ir.Call{
		ExprBase: ir.NewExprBase(token.Span{}, resultType),
		Callee: &ir.Identifier{
			ExprBase:  ir.NewExprBase(token.Span{}, resultType),
			Name:      "request",
			Reference: &ir.Reference{Intrinsic: "trb.platform.typescript.browser.request"},
		},
	}
	lambda := &ir.Lambda{
		ExprBase:   ir.NewExprBase(token.Span{}, functionType),
		ReturnType: resultType,
		Body:       []ir.Statement{&ir.Return{Value: request}},
	}
	variable := &ir.Variable{Name: "loader", Type: functionType, Value: lambda}
	ordinary := &ir.Program{Mode: "typescript", ModulePath: "main", Statements: []ir.Statement{variable}}
	plan, err := AnalyzeSuspension([]*ir.Program{ordinary})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Lambdas[lambda] || plan.LambdaModules[lambda] != "main" {
		t.Fatalf("ordinary suspending Result lambda was not classified independently of native bridging: %#v", plan)
	}

	shadowed := &ir.Program{Mode: "typescript", ModulePath: "shadowed", Statements: []ir.Statement{&ir.Enum{Name: "Result"}, variable}}
	if _, err := AnalyzeSuspension([]*ir.Program{shadowed}); err == nil || !strings.Contains(err.Error(), "may suspend must omit") {
		t.Fatalf("expected a user-defined Result to remain an ordinary non-Void boundary, got %v", err)
	}

	pureType := types.FunctionOf(nil, types.FromName("String"))
	pure := &ir.Lambda{
		ExprBase:   ir.NewExprBase(token.Span{}, pureType),
		ReturnType: types.FromName("String"),
		Body:       []ir.Statement{&ir.Return{Value: request}},
	}
	pureProgram := &ir.Program{Mode: "typescript", ModulePath: "pure", Statements: []ir.Statement{&ir.Variable{Name: "loader", Type: pureType, Value: pure}}}
	if _, err := AnalyzeSuspension([]*ir.Program{pureProgram}); err == nil || !strings.Contains(err.Error(), "may suspend must omit") {
		t.Fatalf("expected a pure non-Void suspending lambda diagnostic, got %v", err)
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
