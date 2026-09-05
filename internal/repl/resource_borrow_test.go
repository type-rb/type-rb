package repl

import (
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/resourceborrow"
)

func TestEvaluateResourceBorrowAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			declarations, expression, expected := resourceborrow.Scenario(t)
			if got := evaluateDirBoundarySource(t, mode, declarations+"\n"+expression+"\n"); got != strconv.Quote(expected) {
				t.Fatalf("got %s, want %q", got, expected)
			}
		})
	}
}
