package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestNullableLoopChecksAndRunsInREPL(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "/project/session.trb", ModulePath: "session", Package: "main", Source: []byte("mut text: String? := \"hello\"\nwhile text != nil\nputs(text.size())\ntext = nil\nend\n")}
			options := compiler.Options{Mode: mode, GoModule: "example.com/nullable-loop", RubyLoader: "require_relative", InteractiveModule: unit.ModulePath}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, options)
			if err != nil {
				t.Fatal(err)
			}
			var programs []*ir.Program
			var session *ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == unit.ModulePath {
					session = artifact.IR
				}
			}
			var out bytes.Buffer
			evaluator := NewEvaluator(&out, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, unit.ModulePath); err != nil {
				t.Fatal(err)
			}
			if _, err := evaluator.Evaluate(session.Statements, unit.ModulePath); err != nil {
				t.Fatal(err)
			}
			if got := out.String(); got != "5\n" {
				t.Fatalf("output=%q", got)
			}
			unit.Source = []byte("mut text: String? := \"hello\"\nif text != nil\n(0..1).each do |_i|\nputs(text.size())\ntext = nil\nend\nend\n")
			if _, err := compiler.CompileProject([]compiler.SourceUnit{unit}, options); err == nil {
				t.Fatal("REPL accepted a stale loop fact")
			}
		})
	}
}
