package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateCompiledConditionalSyntaxAcrossModes(t *testing.T) {
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`def guard(enabled: Boolean): String
	return "early" if enabled
	return "late"
end

def total(): Integer
	mut count := 0
	mut result := 0
	while count < 5
		count += 1
		next if count < 2
		break if count == 4
		result += count
	end
	return result
end

[
	true ? 7 : (1 / 0),
	false ? (1 / 0) : 9,
	guard(true),
	guard(false),
	total(),
]
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/conditional-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
			})
			if err != nil {
				t.Fatalf("%s rejected executable conditional syntax: %v", mode, err)
			}
			programs := make([]*ir.Program, 0, len(artifacts))
			var session *ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == source.ModulePath {
					session = artifact.IR
				}
			}
			if session == nil {
				t.Fatalf("%s compilation did not produce the interactive session", mode)
			}
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, source.ModulePath); err != nil {
				t.Fatalf("%s could not load the conditional syntax project: %v", mode, err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
			if err != nil {
				t.Fatalf("%s conditional syntax evaluation failed: %v", mode, err)
			}
			if got, want := Inspect(result.Value), `[7, 9, "early", "late", 5]`; !result.Display || got != want {
				t.Fatalf("%s conditional syntax evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}
