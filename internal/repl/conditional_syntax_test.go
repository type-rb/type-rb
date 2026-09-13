package repl

import (
	"bytes"
	"strings"
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

def grouped_conditions(): Integer
	mut count := 0
	if (false) || (true)
		count += 1
	end
	while (count < 2) && (true)
		count += 1
	end
	if false
		count += 10
	elsif (false) || (count == 2)
		count += 1
	end
	return count
end

[
	true ? 7 : (1 / 0),
	false ? (1 / 0) : 9,
	guard(true),
	guard(false),
	total(),
	grouped_conditions(),
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
			if got, want := Inspect(result.Value), `[7, 9, "early", "late", 5, 3]`; !result.Display || got != want {
				t.Fatalf("%s conditional syntax evaluation=%s display=%t, want %s", mode, got, result.Display, want)
			}
		})
	}
}

func TestRunReadsAndEvaluatesConditionalTransfersAcrossModes(t *testing.T) {
	const input = `def total(): Integer
mut count := 0
mut result := 0
while count < 5
count += 1
next if count == 1
break if count == 4
result += count
end
return result if true
return 99
end
def stop()
return if true
puts(99)
end
total()
stop()
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{
				Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr,
				Compile: conditionalSessionCompiler(mode),
			})
			if err != nil || stderr.Len() != 0 || stdout.String() != "5 : Integer\n" {
				t.Fatalf("Run conditional transfers: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}

func conditionalSessionCompiler(mode string) CompileFunc {
	return func(text string) (*Compilation, error) {
		const module = "__trb_repl__"
		artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
			Filename: "/project/.trb-repl.trb", ModulePath: module, Package: "main", Source: []byte(text),
		}}, compiler.Options{Mode: mode, InteractiveModule: module, GoModule: "example.com/repl", RubyLoader: "require_relative"})
		if err != nil {
			return nil, err
		}
		programs := make([]*ir.Program, 0, len(artifacts))
		for _, artifact := range artifacts {
			programs = append(programs, artifact.IR)
		}
		return &Compilation{Session: artifacts[0], Artifacts: artifacts, Programs: programs}, nil
	}
}

func TestRunReportsInvalidConditionalTransfersAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{
				Mode: mode, Stdin: strings.NewReader("def broken(): Integer\nreturn 1 if\nend\n7\n:quit\n"),
				Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode),
			})
			if err != nil || stdout.String() != "7 : Integer\n" ||
				!strings.Contains(stderr.String(), "conditional return requires a valid condition after if") ||
				strings.Contains(stderr.String(), "incomplete input") {
				t.Fatalf("Run invalid transfer: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}
