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
