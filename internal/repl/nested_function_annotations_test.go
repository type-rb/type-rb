package repl

import (
	"bytes"
	"os"
	"testing"
)

func TestReplNestedFunctionAnnotations(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/nested_function_annotations.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, string(source)+"\nmain()\n")
			var output bytes.Buffer
			e.stdout = &output
			if _, err := e.Evaluate(session.Statements, session.ModulePath); err != nil {
				t.Fatal(err)
			}
			if want := "called\n5\n9\n13\n17\ncalled\n"; output.String() != want {
				t.Fatalf("output=%q, want %q", output.String(), want)
			}
		})
	}
}
