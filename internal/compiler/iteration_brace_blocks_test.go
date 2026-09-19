package compiler

import (
	"os"
	"testing"
)

func TestIterationBraceBlocksRunAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/iteration_brace_blocks.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "iteration_brace_blocks.trb", source, "24\nfirst\nsecond\ntwo\nthree")
		})
	}
}
