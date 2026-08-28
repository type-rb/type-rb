package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParsePostfixCallAfterDoEndIteration(t *testing.T) {
	program, diagnostics := Parse([]byte(`def count(ids: Array<Integer>): Integer
	count := ids.map do |id|
		id * 2
	end.size()
	return count
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	variable := method.Body[0].(*ast.VariableStatement)
	call, ok := variable.Value.(*ast.CallExpression)
	if !ok {
		t.Fatalf("value=%T", variable.Value)
	}
	member, ok := call.Callee.(*ast.MemberExpression)
	if !ok || member.Name != "size" {
		t.Fatalf("callee=%#v", call.Callee)
	}
	iteration, ok := member.Receiver.(*ast.IterationExpression)
	if !ok || iteration.Operation != "map" {
		t.Fatalf("receiver=%#v", member.Receiver)
	}
}

func TestParseChainedDoEndIterations(t *testing.T) {
	program, diagnostics := Parse([]byte(`def total(ids: Array<Integer>): Integer
	return ids.concurrent_map do |id|
		id * 2
	end.reduce(0) do |sum, value|
		sum + value
	end
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	returned := method.Body[0].(*ast.ReturnStatement)
	reduce, ok := returned.Value.(*ast.IterationExpression)
	if !ok || reduce.Operation != "reduce" {
		t.Fatalf("return=%#v", returned.Value)
	}
	concurrentMap, ok := reduce.Source.(*ast.IterationExpression)
	if !ok || concurrentMap.Operation != "concurrent_map" {
		t.Fatalf("reduce source=%#v", reduce.Source)
	}
}

func TestParseBraceIterationChainedToDoEndIteration(t *testing.T) {
	program, diagnostics := Parse([]byte(`def total(ids: Array<Integer>): Integer
	return ids.map { |id| id * 2 }.reduce(0) do |sum, value|
		sum + value
	end
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	returned := method.Body[0].(*ast.ReturnStatement)
	reduce, ok := returned.Value.(*ast.IterationExpression)
	if !ok || reduce.Operation != "reduce" {
		t.Fatalf("return=%#v", returned.Value)
	}
	mapped, ok := reduce.Source.(*ast.IterationExpression)
	if !ok || mapped.Operation != "map" || mapped.Block == nil || !mapped.Block.Brace {
		t.Fatalf("reduce source=%#v", reduce.Source)
	}
}
