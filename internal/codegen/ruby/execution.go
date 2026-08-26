package ruby

import "github.com/type-rb/type-rb/internal/ir"

func (g *generator) programUsesExecutionScope(statements []ir.Statement) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Field:
			if g.expressionUsesExecutionScope(node.Value) {
				return true
			}
		case *ir.Variable:
			if g.expressionUsesExecutionScope(node.Value) {
				return true
			}
		case *ir.Assignment:
			if g.expressionUsesExecutionScope(node.Target) || g.expressionUsesExecutionScope(node.Value) {
				return true
			}
		case *ir.Return:
			if g.expressionUsesExecutionScope(node.Value) {
				return true
			}
		case *ir.ExpressionStatement:
			if g.expressionUsesExecutionScope(node.Expression) {
				return true
			}
		case *ir.If:
			if g.expressionUsesExecutionScope(node) {
				return true
			}
		case *ir.Case:
			if g.expressionUsesExecutionScope(node) {
				return true
			}
		case *ir.While:
			if g.expressionUsesExecutionScope(node.Condition) || g.programUsesExecutionScope(node.Body) {
				return true
			}
		case *ir.Iterate:
			if g.execution != nil && g.execution.Iterations[node] {
				return true
			}
		case *ir.StructuredBlock:
			if g.execution != nil && g.execution.StructuredBlocks[node] {
				return true
			}
		case *ir.NativeBlock:
			if g.programUsesExecutionScope(node.Body) {
				return true
			}
		case *ir.Method:
			if g.methodUsesExecutionScope(node) {
				return true
			}
		case *ir.Class:
			if g.programUsesExecutionScope(node.Body) {
				return true
			}
		case *ir.Enum:
			if g.programUsesExecutionScope(node.Body) {
				return true
			}
		case *ir.Record:
			if g.execution != nil && g.execution.RecordDefault(g.modulePath, node.Name) {
				return true
			}
		case *ir.Module:
			if g.programUsesExecutionScope(node.Body) {
				return true
			}
		case *ir.Interface:
			for _, method := range node.Methods {
				if g.methodUsesExecutionScope(method) {
					return true
				}
			}
		}
	}
	return false
}

func (g *generator) expressionUsesExecutionScope(expression ir.Expression) bool {
	return expression != nil && g.execution != nil && g.execution.Expressions[expression]
}

func (g *generator) executionScopeRuntime() {
	g.line("unless defined?(TrbExecutionCancelled)", "")
	g.indent++
	g.line("class TrbExecutionCancelled < StandardError; end", "")
	g.line("class TrbExecutionScope", "")
	g.indent++
	g.line("def self.root", "")
	g.indent++
	g.line("new", "")
	g.indent--
	g.line("end", "")
	g.line("def initialize", "")
	g.indent++
	g.line("@mutex = Mutex.new", "")
	g.line("@cancelled = false", "")
	g.line("@callbacks = []", "")
	g.indent--
	g.line("end", "")
	g.line("def cancelled?", "")
	g.indent++
	g.line("@mutex.synchronize { @cancelled }", "")
	g.indent--
	g.line("end", "")
	g.line("def check!", "")
	g.indent++
	g.line(`raise TrbExecutionCancelled, "TypeRB execution was cancelled" if cancelled?`, "")
	g.indent--
	g.line("end", "")
	g.line("def on_cancel(&callback)", "")
	g.indent++
	g.line("call_now = @mutex.synchronize do", "")
	g.indent++
	g.line("if @cancelled then true else @callbacks << callback; false end", "")
	g.indent--
	g.line("end", "")
	g.line("callback.call if call_now", "")
	g.indent--
	g.line("end", "")
	g.line("def cancel", "")
	g.indent++
	g.line("callbacks = @mutex.synchronize do", "")
	g.indent++
	g.line("next [] if @cancelled", "")
	g.line("@cancelled = true", "")
	g.line("values = @callbacks", "")
	g.line("@callbacks = []", "")
	g.line("values", "")
	g.indent--
	g.line("end", "")
	g.line("callbacks.each(&:call)", "")
	g.indent--
	g.line("end", "")
	g.line("def child", "")
	g.indent++
	g.line("value = TrbExecutionScope.new", "")
	g.line("on_cancel { value.cancel }", "")
	g.line("value", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.b.WriteByte('\n')
}
