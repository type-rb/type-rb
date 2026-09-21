package resolver

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestReopenedModuleExportsRetainEarlierContractsWithoutChangingSourceOrder(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`module Store
  record Item
    value: Integer = 7
  end
end
module Store
  def self.make(): Item
    return Item.new()
  end
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	exported := CollectExports(program.Statements)["Store"]
	if got := exported.Members["make"].Type.Name; got != "Store::Item" {
		t.Fatalf("reopened member lost its owned return type: %q", got)
	}
	fields := exported.Nested["Item"].Fields
	if len(fields) != 1 || !fields[0].HasDefault {
		t.Fatalf("earlier record default contract was lost: %#v", fields)
	}
	if len(program.Statements) != 2 {
		t.Fatal("export collection changed authored statement order")
	}
	for _, statement := range program.Statements {
		if len(statement.(*ast.ModuleStatement).Body) != 1 {
			t.Fatal("export collection modified a source module body")
		}
	}
}
