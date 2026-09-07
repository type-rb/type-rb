package compiler

import (
	"strings"
	"testing"
)

func TestStringEscapeDiagnosticsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, literal := range []string{`"aaa#\{s}"`, `"\q"`, `"\q #{"ok"}"`, `"#{"ok"}\q"`, `"#{"\q"}"`, `"\xGG"`, `"\uZZZZ"`} {
			t.Run(mode+"/"+literal, func(t *testing.T) {
				source := "def main()\n puts(" + literal + ")\nend\n"
				_, err := Compile("escape.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), "invalid String escape or literal") {
					t.Fatalf("expected frontend String escape error, got %v", err)
				}
			})
		}
	}
}

func TestStringEscapesInLiteralTypes(t *testing.T) {
	const source = `alias Marker = "\#tag"
def marker(): Marker
 return "\#tag"
end
def main()
 puts(marker())
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := Compile("marker.trb", []byte(source), mode); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}
