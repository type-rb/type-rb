package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestNestedGenericClosersPreserveParameterAndExpressionDelimiters(t *testing.T) {
	program, diagnostics := Parse([]byte(`def pick(rows: Array<Array<Array<Integer>>>, index: Integer = add(8 >> 1, 2)): Integer
	return index
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	if len(method.Parameters) != 2 || method.Parameters[1].Name != "index" {
		t.Fatalf("following parameter was not preserved: %#v", method.Parameters)
	}
	current := method.Parameters[0].Type
	for range 3 {
		if current.Name != "Array" || len(current.Arguments) != 1 {
			t.Fatalf("nested Array type was not preserved: %#v", current)
		}
		current = current.Arguments[0]
	}
	if current.Name != "Integer" {
		t.Fatalf("nested element type: %#v", current)
	}
	call, ok := method.Parameters[1].Default.(*ast.CallExpression)
	if !ok || len(call.Arguments) != 2 {
		t.Fatalf("default call arguments were not preserved: %#v", method.Parameters[1].Default)
	}
	shift, ok := call.Arguments[0].Value.(*ast.BinaryExpression)
	if !ok || shift.Operator != ">>" {
		t.Fatalf("shift operator was not preserved: %#v", call.Arguments[0].Value)
	}
}
