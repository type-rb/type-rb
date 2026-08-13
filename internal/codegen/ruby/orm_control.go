package ruby

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormStructuredBlock(block *ir.StructuredBlock) {
	if block == nil || block.Intrinsic != "trb.orm.transaction" || block.Result == nil {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trb_transaction_result" + id
	transaction := "__trb_transaction" + id
	parent := "nil"
	if member, ok := block.Call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Name == "Transaction" {
		parent = g.expr(member.Receiver)
	}

	g.line(raw+" = TrbOrmRuntime.with_scope(__trb_scope) do", block.TrailingComment)
	g.indent++
	g.line("TrbOrmRuntime.transaction_result("+parent+") do |"+transaction+"|", "")
	g.indent++
	g.line("-> do", "")
	g.indent++
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		g.line(block.Bindings[0].Name+" = "+transaction, "")
	}
	g.statements(block.Body)
	g.line("Result::Ok.new("+g.expr(block.Value)+")", "")
	g.indent--
	g.line("end.call", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.ormAssignStructuredResult(raw, block.Result, block.CaptureEffect, block.PropagateSuccess)
}

func (g *generator) ormBatchIterate(iteration *ir.Iterate) {
	if iteration == nil || iteration.Result == nil {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	raw := "__trb_batch_result" + id
	processed := "__trb_batch_processed" + id
	batch := "__trb_batch" + id
	item := "__trb_item" + id
	breakTarget := "__trb_batch_break" + id
	query := g.expr(iteration.Source)
	if _, ok := g.orm.Model(iteration.Source.ExprType().Name); ok {
		query = "TrbOrmRuntime.query(" + iteration.Source.ExprType().Name + ")"
	}
	batchSize := "1000"
	if iteration.SliceSize != nil {
		batchSize = g.expr(iteration.SliceSize)
	}

	g.line(raw+" = TrbOrmRuntime.with_scope(__trb_scope) do", iteration.TrailingComment)
	g.indent++
	g.line("-> do", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line(processed+" = 0", "")
	g.line("catch("+strconv.Quote(breakTarget)+") do", "")
	g.indent++
	g.line(query+".each_batch("+batchSize+") do |"+batch+"|", "")
	g.indent++
	if iteration.Operation == "find_each" {
		g.line(batch+".each do |"+item+"|", "")
		g.indent++
		g.line(processed+" += 1", "")
		if len(iteration.Bindings) > 0 && iteration.Bindings[0].Name != "_" {
			g.line(iteration.Bindings[0].Name+" = "+item, "")
		}
		previous := g.breakTarget
		g.breakTarget = breakTarget
		g.statements(iteration.Body)
		g.breakTarget = previous
		g.indent--
		g.line("end", "")
	} else {
		g.line(processed+" += "+batch+".length", "")
		if len(iteration.Bindings) > 0 && iteration.Bindings[0].Name != "_" {
			g.line(iteration.Bindings[0].Name+" = "+batch, "")
		}
		previous := g.breakTarget
		g.breakTarget = breakTarget
		g.statements(iteration.Body)
		g.breakTarget = previous
	}
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("Result::Ok.new("+processed+")", "")
	g.indent--
	g.line("rescue TrbOrmRuntime::Failure => error", "")
	g.indent++
	g.line("Result::Err.new(error.db_error)", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end.call", "")
	g.indent--
	g.line("end", "")
	g.ormAssignIterationResult(raw, iteration)
}

func (g *generator) ormAssignStructuredResult(raw string, result *ir.StructuredBlockResult, capture bool, propagate types.Type) {
	if result == nil {
		return
	}
	if capture {
		g.ormAssignResultTarget(raw, result.Variable, result.Target, result.Return)
		return
	}
	if result.Return {
		g.line("return "+raw, "")
		return
	}
	g.line("return "+raw+" if "+raw+".is_a?(Result::Err)", "")
	g.ormAssignResultTarget(raw+".value", result.Variable, result.Target, false)
}

func (g *generator) ormAssignIterationResult(raw string, iteration *ir.Iterate) {
	result := iteration.Result
	if iteration.CaptureEffect {
		g.ormAssignIterationTarget(raw, result)
		return
	}
	if result.Return {
		g.line("return "+raw, "")
		return
	}
	g.line("return "+raw+" if "+raw+".is_a?(Result::Err)", "")
	g.ormAssignIterationTarget(raw+".value", result)
}

func (g *generator) ormAssignResultTarget(value string, variable *ir.Variable, target ir.Expression, returned bool) {
	switch {
	case returned:
		g.line("return "+value, "")
	case variable != nil:
		g.line(variable.Name+" = "+value, variable.TrailingComment)
	case target != nil:
		g.line(g.assignmentTarget(target)+" = "+value, "")
	}
}

func (g *generator) ormAssignIterationTarget(value string, result *ir.IterationResult) {
	if result == nil {
		return
	}
	switch {
	case result.Return:
		g.line("return "+value, "")
	case result.Variable != nil:
		g.line(result.Variable.Name+" = "+value, result.Variable.TrailingComment)
	case result.Target != nil:
		g.line(g.assignmentTarget(result.Target)+" = "+value, "")
	}
}
