package codegen

import (
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Keep initial values, slice sizes, limits and the body inside the present
// branch. The retained receiver is shared by the test and the actual traversal.
func (n *controlFlowNormalizer) safeIteration(node *ir.Iterate) []ir.Statement {
	present := *node
	present.Safe = false
	if !node.Source.ExprType().Nullable {
		return n.statement(&present)
	}
	prefix, source := n.expression(node.Source)
	if source == nil {
		return prefix
	}
	stablePrefix, stable := n.materialize(source)
	prefix = append(prefix, stablePrefix...)
	present.Source = safePresentReceiver(stable)
	flow := &ir.If{
		ExprBase:  ir.NewExprBase(node.SourceSpan(), types.FromName("Void")),
		Condition: safeNavigationCondition(stable, node.SourceSpan()),
		Then:      n.statement(&present),
	}
	return append(prefix, flow)
}

func (n *controlFlowNormalizer) safeTransform(node *ir.Transform) ([]ir.Statement, ir.Expression) {
	present := *node
	present.Safe = false
	present.Type = node.PresentType
	present.PresentType = types.Type{}
	if !node.Source.ExprType().Nullable {
		return n.expression(&present)
	}
	prefix, source := n.expression(node.Source)
	if source == nil {
		return prefix, nil
	}
	stablePrefix, stable := n.materialize(source)
	prefix = append(prefix, stablePrefix...)
	present.Source = safePresentReceiver(stable)
	return n.safeNavigationExpression(prefix, stable, &present, node.ExprType(), node.SourceSpan())
}
