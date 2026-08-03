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
