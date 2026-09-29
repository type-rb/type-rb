package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestInitializationRetryDoesNotRepeatCompletedSideEffects(t *testing.T) {
	for _, cyclic := range []bool{false, true} {
		dependency := ""
		if cyclic {
			dependency = "import { VALUE } from a\ndef later(): Integer\nreturn VALUE\nend\n"
		}
		sources := []compiler.SourceUnit{
			{Filename: "/project/a.trb", ModulePath: "a", Source: []byte("import { ZERO, mark } from b\nFIRST := mark()\nVALUE := FIRST / ZERO\n")},
			{Filename: "/project/b.trb", ModulePath: "b", Source: []byte(dependency + "def mark(): Integer\nputs(\"once\")\nreturn 1\nend\nZERO := 0\n")},
			{Filename: "/project/session.trb", ModulePath: "session", Source: []byte("import { VALUE } from a\nputs(VALUE)\n")},
		}
		artifacts, err := compiler.AnalyzeProject(sources, compiler.Options{Mode: "go", GoModule: "example.com/retry", InteractiveModule: "session"})
		if err != nil {
			t.Fatal(err)
		}
		var programs []*ir.Program
		for _, artifact := range artifacts {
			programs = append(programs, artifact.IR)
		}
		var output bytes.Buffer
		evaluator := NewEvaluator(&output, "go")
		t.Cleanup(func() { _ = evaluator.Close() })
		for attempt := 0; attempt < 2; attempt++ {
			if err := evaluator.LoadProject(programs, "session"); err == nil {
				t.Fatal("expected division failure")
			}
		}
		want := "once\n"
		// Acyclic modules have an indivisible existing initialization boundary.
		if !cyclic {
			want = "once\nonce\n"
		}
		if output.String() != want {
			t.Fatalf("cyclic=%v: got %q want %q", cyclic, output.String(), want)
		}
	}
}
