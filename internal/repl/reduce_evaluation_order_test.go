package repl

import (
	"bytes"
	"os"
	"testing"
)

func TestReplReduceEvaluationOrder(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/reduce_evaluation_order.trb")
	if err != nil {
		t.Fatal(err)
	}
	want := "17\n1\n2\n3\n4\n17\n1\n2\n3\n4\n10\n1\n2\n16\n2\n17\n1\n2\n17\n100\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, string(source)+"\nmain()\n")
			var output bytes.Buffer
			e.stdout = &output
			if _, err := e.Evaluate(session.Statements, session.ModulePath); err != nil {
				t.Fatal(err)
			}
			if output.String() != want {
				t.Fatalf("output=%q, want %q", output.String(), want)
			}
		})
	}
}
