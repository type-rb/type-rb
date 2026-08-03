package repl

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

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
			Item:      "value",
			ItemType:  integer,
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
