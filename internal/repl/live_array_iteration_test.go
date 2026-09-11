package repl

import (
	"bytes"
	"os"
	"testing"
)

func TestReplLiveArrayIteration(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/live_array_iteration/main.trb")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../compiler/testdata/live_array_iteration/expected.txt")
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
			if !bytes.Equal(output.Bytes(), want) {
				t.Fatalf("want %q, got %q", want, output.String())
			}
		})
	}
}
