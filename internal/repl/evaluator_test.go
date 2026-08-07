package repl

import (
	"bytes"
	"context"
	"errors"
	"math"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func TestEvaluateFloatClassificationIntrinsics(t *testing.T) {
	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	floatType := types.FromName("Float")
	booleanType := types.FromName("Boolean")
	tests := []struct {
		name  string
		value float64
		want  bool
	}{
		{name: "trb.std.numbers.float_finite", value: 0.25, want: true},
		{name: "trb.std.numbers.float_finite", value: math.Inf(1), want: false},
		{name: "trb.std.numbers.float_infinite", value: math.Inf(-1), want: true},
		{name: "trb.std.numbers.float_nan", value: math.NaN(), want: true},
	}
	for _, test := range tests {
		result, err := evaluator.intrinsic(test.name, []evaluatedArgument{{Value: Value{Type: floatType, Data: test.value}}}, booleanType, nil)
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		if got, ok := result.Data.(bool); !ok || got != test.want {
			t.Fatalf("%s(%v)=%#v, want %t", test.name, test.value, result.Data, test.want)
		}
	}
}

func TestEvaluateContextStopsCanceledEvaluation(t *testing.T) {
	integer := types.FromName("Integer")
	statements := []ir.Statement{
		&ir.ExpressionStatement{Expression: &ir.Literal{
			ExprBase: ir.NewExprBase(token.Span{}, integer),
			Kind:     "integer",
			Raw:      "1",
		}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewEvaluator(&bytes.Buffer{}, "go").EvaluateContext(ctx, statements, "repl")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("EvaluateContext error=%v, want context.Canceled", err)
	}
}

func TestEvaluatePortableRangeIteration(t *testing.T) {
	integer := types.FromName("Integer")
	rangeType := types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{integer}}
	literal := func(raw string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: raw}
	}
	identifier := func(name string) *ir.Identifier {
		return &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, integer), Name: name}
	}
	statements := []ir.Statement{
		&ir.Variable{Name: "sum", Type: integer, Mutable: true, Value: literal("0")},
		&ir.Iterate{
			Source:    &ir.Range{ExprBase: ir.NewExprBase(token.Span{}, rangeType), Start: literal("0"), End: literal("4"), Exclusive: true},
			Operation: "each",
			Bindings:  []ir.IterationBinding{{Name: "value", Type: integer}},
			Body: []ir.Statement{&ir.Assignment{
				Target:   identifier("sum"),
				Operator: "+=",
				Value:    identifier("value"),
			}},
		},
		&ir.ExpressionStatement{Expression: identifier("sum")},
	}
	result, err := NewEvaluator(&bytes.Buffer{}, "go").Evaluate(statements, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != "6" {
		t.Fatalf("unexpected range iteration result: %#v", result)
	}
}

func TestEvaluatePortableHashIteration(t *testing.T) {
	integer := types.FromName("Integer")
	hashType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{integer, integer}}
	literal := func(raw string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, integer), Kind: "integer", Raw: raw}
	}
	identifier := func(name string) *ir.Identifier {
		return &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, integer), Name: name}
	}
	statements := []ir.Statement{
		&ir.Variable{Name: "sum", Type: integer, Mutable: true, Value: literal("0")},
		&ir.Iterate{
			Source: &ir.Hash{ExprBase: ir.NewExprBase(token.Span{}, hashType), Entries: []ir.HashEntry{
				{Key: literal("1"), Value: literal("2")},
				{Key: literal("3"), Value: literal("4")},
			}},
			Operation: "each",
			Bindings: []ir.IterationBinding{
				{Name: "key", Type: integer},
				{Name: "value", Type: integer},
			},
			Body: []ir.Statement{&ir.Assignment{
				Target:   identifier("sum"),
				Operator: "+=",
				Value: &ir.Binary{
					ExprBase: ir.NewExprBase(token.Span{}, integer),
					Left:     identifier("key"),
					Operator: "+",
					Right:    identifier("value"),
				},
			}},
		},
		&ir.ExpressionStatement{Expression: identifier("sum")},
	}
	result, err := NewEvaluator(&bytes.Buffer{}, "go").Evaluate(statements, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != "10" {
		t.Fatalf("unexpected Hash iteration result: %#v", result)
	}
}

func TestEvaluateEnumAndCase(t *testing.T) {
	enumType := types.FromName("State")
	definition := &ir.Enum{Name: "State", Body: []ir.Statement{
		&ir.EnumMember{Name: "Open"},
		&ir.EnumMember{Name: "Closed"},
	}}
	state := func(member string) *ir.Member {
		return &ir.Member{
			ExprBase:  ir.NewExprBase(token.Span{}, enumType),
			Receiver:  &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, enumType), Name: "State"},
			Name:      member,
			Namespace: true,
		}
	}
	text := func(value string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, types.FromName("String")), Kind: "string", Raw: `"` + value + `"`}
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.LoadDefinitions(&ir.Program{ModulePath: "repl", Statements: []ir.Statement{definition}})
	result, err := evaluator.Evaluate([]ir.Statement{&ir.Case{
		Value: state("Closed"),
		Branches: []ir.CaseBranch{
			{Value: state("Open"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: text("open")}}},
			{Value: state("Closed"), Body: []ir.Statement{&ir.ExpressionStatement{Expression: text("closed")}}},
		},
	}}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"closed"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected enum case result: %#v", result)
	}
}

func TestEvaluateCaseExpression(t *testing.T) {
	enumType := types.FromName("State")
	stringType := types.FromName("String")
	definition := &ir.Enum{Name: "State", Body: []ir.Statement{
		&ir.EnumMember{Name: "Open"},
		&ir.EnumMember{Name: "Closed"},
	}}
	state := func(member string) *ir.Member {
		return &ir.Member{
			ExprBase:  ir.NewExprBase(token.Span{}, enumType),
			Receiver:  &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, enumType), Name: "State"},
			Name:      member,
			Namespace: true,
		}
	}
	text := func(value string) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, stringType), Kind: "string", Raw: `"` + value + `"`}
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	evaluator.LoadDefinitions(&ir.Program{ModulePath: "repl", Statements: []ir.Statement{definition}})
	caseExpression := &ir.Case{
		ExprBase: ir.NewExprBase(token.Span{}, stringType),
		Value:    state("Closed"),
		Branches: []ir.CaseBranch{
			{Value: state("Open"), Result: text("open")},
			{Value: state("Closed"), Result: text("closed")},
		},
	}
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.Variable{Name: "message", Type: stringType, Value: caseExpression},
		&ir.ExpressionStatement{Expression: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "message"}},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"closed"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected case expression result: %#v", result)
	}
}

func TestEvaluateIfExpression(t *testing.T) {
	booleanType := types.FromName("Boolean")
	stringType := types.FromName("String")
	literal := func(kind, raw string, typ types.Type) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, typ), Kind: kind, Raw: raw}
	}
	ifExpression := &ir.If{
		ExprBase:   ir.NewExprBase(token.Span{}, stringType),
		Condition:  literal("boolean", "false", booleanType),
		ThenResult: literal("string", `"on"`, stringType),
		ElseIf: []ir.IfBranch{{
			Condition: literal("boolean", "true", booleanType),
			Result:    literal("string", `"secondary"`, stringType),
		}},
		ElseResult: literal("string", `"off"`, stringType),
		HasElse:    true,
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.Variable{Name: "message", Type: stringType, Value: ifExpression},
		&ir.ExpressionStatement{Expression: &ir.Identifier{ExprBase: ir.NewExprBase(token.Span{}, stringType), Name: "message"}},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display || Inspect(result.Value) != `"secondary"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected if expression result: %#v", result)
	}
}

func TestEvaluateDivergingIfExpressionPropagatesReturn(t *testing.T) {
	booleanType := types.FromName("Boolean")
	stringType := types.FromName("String")
	literal := func(kind, raw string, typ types.Type) *ir.Literal {
		return &ir.Literal{ExprBase: ir.NewExprBase(token.Span{}, typ), Kind: kind, Raw: raw}
	}
	ifExpression := &ir.If{
		ExprBase:     ir.NewExprBase(token.Span{}, stringType),
		Condition:    literal("boolean", "false", booleanType),
		ThenResult:   literal("string", `"value"`, stringType),
		Else:         []ir.Statement{&ir.Return{Value: literal("string", `"returned"`, stringType)}},
		ElseDiverges: true,
		HasElse:      true,
	}

	evaluator := NewEvaluator(&bytes.Buffer{}, "go")
	result, err := evaluator.Evaluate([]ir.Statement{
		&ir.ExpressionStatement{Expression: ifExpression},
		&ir.ExpressionStatement{Expression: literal("string", `"unreachable"`, stringType)},
	}, "repl")
	if err != nil {
		t.Fatal(err)
	}
	if result.Display || Inspect(result.Value) != `"returned"` || result.Value.Type.String() != "String" {
		t.Fatalf("unexpected propagated return: %#v", result)
	}
}
