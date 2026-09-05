package pathvalue

import (
	"testing"

	"github.com/type-rb/type-rb/internal/pathvalue/pathfixture"
)

func TestJoinPreservesParentForBothHostGrammars(t *testing.T) {
	for _, windows := range []bool{false, true} {
		for _, fixture := range pathfixture.Cases {
			if got := Join(fixture.Parent, fixture.Child, windows); got != fixture.Expected(windows) {
				t.Errorf("Join(%q, %q, windows=%v) = %q, want %q", fixture.Parent, fixture.Child, windows, got, fixture.Expected(windows))
			}
		}
	}
}
