package formatter

import (
	"bytes"
	"testing"
)

func TestFormatSymbolKeywordsDoesNotOpenBlocks(t *testing.T) {
	source := []byte("def main()\nvalue := :if\nputs(value)\nputs(:case)\nputs(:fn)\nvalues := {if: :case, end: :if}\nputs(values[\"end\"])\nend\n")
	want := []byte("def main()\n\tvalue := :if\n\tputs(value)\n\tputs(:case)\n\tputs(:fn)\n\tvalues := { if: :case, end: :if }\n\tputs(values[\"end\"])\nend\n")
	got, diagnostics := Format(source)
	if len(diagnostics) != 0 || !bytes.Equal(got, want) {
		t.Fatalf("formatted=%q want=%q diagnostics=%v", got, want, diagnostics)
	}
	again, diagnostics := Format(got)
	if len(diagnostics) != 0 || !bytes.Equal(again, got) {
		t.Fatalf("format is not idempotent: %q diagnostics=%v", again, diagnostics)
	}
	if got := NextLineIndent([]byte("def main()\nvalue := :if")); got != "\t" {
		t.Fatalf("next indentation=%q", got)
	}
	if got := ReindentPartial(source); !bytes.Equal(got, []byte("def main()\n\tvalue := :if\n\tputs(value)\n\tputs(:case)\n\tputs(:fn)\n\tvalues := {if: :case, end: :if}\n\tputs(values[\"end\"])\nend\n")) {
		t.Fatalf("partial reindent=%q", got)
	}
}
