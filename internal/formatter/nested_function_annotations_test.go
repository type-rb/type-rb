package formatter

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestFormatNestedFunctionAnnotations(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/nested_function_annotations.trb")
	if err != nil {
		t.Fatal(err)
	}
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for _, annotation := range []string{"Array<() -> Void>", "Hash<String, (Integer) -> Integer>", "Array<() -> (Integer) -> Integer>", "Array<((Integer) -> Integer) -> Integer>"} {
		if !strings.Contains(string(formatted), annotation) {
			t.Fatalf("lost annotation %q: %s", annotation, formatted)
		}
	}
	again, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, again) {
		t.Fatalf("not idempotent: %v\n%s\n%s", diagnostics, formatted, again)
	}
}
