package formatter

import (
	"bytes"
	"strings"
	"testing"
)

func TestFormatEmbeddedCollectionBlocks(t *testing.T) {
	source := []byte("def main()\nputs([1,2].map do |item|\n# retained\nitem+1 # tail\nend[1])\nputs([3].map{|item| item*2}[0])\nend\n")
	formatted, diagnostics := Format(source)
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for _, comment := range []string{"# retained", "# tail"} {
		if !strings.Contains(string(formatted), comment) {
			t.Fatalf("missing comment %s:\n%s", comment, formatted)
		}
	}
	again, diagnostics := Format(formatted)
	if len(diagnostics) != 0 || !bytes.Equal(formatted, again) {
		t.Fatalf("unstable format: %v\nfirst:\n%s\nsecond:\n%s", diagnostics, formatted, again)
	}
}
