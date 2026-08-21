package compiler

import (
	"errors"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
)

func TestCompileRejectsInvalidUTF8AcrossModes(t *testing.T) {
	source := append([]byte("value := 1\n"), 0xff)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("invalid.trb", source, mode)
			var compilation *CompileError
			if !errors.As(err, &compilation) {
				t.Fatalf("error=%v", err)
			}
			if len(compilation.Diagnostics) != 1 {
				t.Fatalf("diagnostics=%#v", compilation.Diagnostics)
			}
			item := compilation.Diagnostics[0]
			if item.Path != "invalid.trb" || item.Code != diagnostic.SyntaxError || item.Message != "source is not valid UTF-8" || item.Span.Start.Line != 2 || item.Span.Start.Column != 1 {
				t.Fatalf("unexpected diagnostic: %#v", item)
			}
		})
	}
}
