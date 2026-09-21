package formatter

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/parser"
)

func TestFormatNestedFunctionValues(t *testing.T) {
	for _, source := range []string{
		"invoke(fn(value:Integer):Integer;return value+1;end, 3)\n",
		"callbacks := [fn():Integer;return 1;end, fn():Integer;return 2;end]\n",
		"holder := Holder.new(callback: fn(value:Integer):Integer\nif value>0\nreturn value\nend\nreturn 0\nend)\n",
	} {
		formatted, diagnostics := Format([]byte(source))
		if len(diagnostics) != 0 {
			t.Fatalf("format diagnostics: %v\n%s", diagnostics, source)
		}
		program, diagnostics := parser.Parse(formatted)
		if len(diagnostics) != 0 || len(program.NativeIslands) != 0 {
			t.Fatalf("formatted function expression lost portable syntax: %v\n%s", diagnostics, formatted)
		}
		again, diagnostics := Format(formatted)
		if len(diagnostics) != 0 || !bytes.Equal(formatted, again) {
			t.Fatalf("format is not idempotent: %v\n%s\n%s", diagnostics, formatted, again)
		}
	}
}
