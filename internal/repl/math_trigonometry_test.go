package repl

import (
	"bytes"
	"os"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestPortableTrigonometryInTypedIR(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/math_trigonometry.trb")
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../compiler/testdata/math_trigonometry.out")
	if err != nil {
		t.Fatal(err)
	}
	source = append(source, []byte("\nmain()\n")...)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "/project/session.trb", ModulePath: "session", Package: "main", Source: source}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: mode, GoModule: "example.com/math", RubyLoader: "require_relative", InteractiveModule: "session"})
			if err != nil {
				t.Fatal(err)
			}
			var programs []*ir.Program
			var session *ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == "session" {
					session = artifact.IR
				}
			}
			var out bytes.Buffer
			evaluator := NewEvaluator(&out, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, "session"); err != nil {
				t.Fatal(err)
			}
			evaluator.LoadDefinitions(session)
			if _, err := evaluator.Evaluate(session.Statements, "session"); err != nil {
				t.Fatal(err)
			}
			if out.String() != string(want) {
				t.Fatalf("output=%q, want %q", out.String(), want)
			}
		})
	}
}
