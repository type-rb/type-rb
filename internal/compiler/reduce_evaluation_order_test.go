package compiler

import (
	"os"
	"testing"
)

func TestReduceEvaluationOrderAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/reduce_evaluation_order.trb")
	if err != nil {
		t.Fatal(err)
	}
	want := "17\n1\n2\n3\n4\n17\n1\n2\n3\n4\n10\n1\n2\n16\n2\n17\n1\n2\n17\n100"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runEffectSource(t, mode, "reduce_evaluation_order.trb", source, want)
		})
	}
}
