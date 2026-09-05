package repl

import (
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/regularfile"
)

func TestEvaluateRegularFileAcquisitionAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			declarations, expression, expected := regularfile.Scenario(t)
			if got := evaluateDirBoundarySource(t, mode, declarations+"\n"+expression+"\n"); got != strconv.Quote(expected) {
				t.Fatalf("got %s, want %q", got, expected)
			}
		})
	}
}
