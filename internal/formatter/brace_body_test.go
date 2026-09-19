package formatter

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestFormatBraceBodiesRetainsControlsAndComments(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/iteration_brace_blocks.trb")
	if err != nil {
		t.Fatal(err)
	}
	source = bytes.Replace(source, []byte("if index == 0"), []byte("# keep this comment\nif index == 0"), 1)
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	if !strings.Contains(string(formatted), "# keep this comment") {
		t.Fatalf("lost comment: %s", formatted)
	}
	again, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, again) {
		t.Fatalf("not idempotent: %v\n%s\n%s", diagnostics, formatted, again)
	}
}
