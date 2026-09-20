package repl

import "testing"

func TestCommonUnionFieldWidensNumericValueInREPL(t *testing.T) {
	const declarations = `record Whole
value: Integer
end
record Decimal
value: Float
end
record Exact
value: 2
end

def rounded(value: Whole | Decimal): Integer
return value.value.floor()
end

def exact(value: Exact | Decimal): Integer
return value.value.floor()
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			source := declarations + `[rounded(Whole.new(value: 2)), rounded(Decimal.new(value: 3.5)), exact(Exact.new(value: 2))]
`
			if got := evaluateDirBoundarySource(t, mode, source); got != "[2, 3, 2]" {
				t.Fatalf("result=%s", got)
			}
		})
	}
}
