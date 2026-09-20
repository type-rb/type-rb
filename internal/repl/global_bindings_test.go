package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestGlobalBindingsRetainIdentityAcrossClosuresAndSubmissions(t *testing.T) {
	const input = `mut value := 3
def read(): Integer
  saved := fn(): Integer; return value; end
  value := 17
  return saved() + value
end
def update()
  saved := fn(); value += 1; end
  mut value := 23
  saved()
  puts(value)
end
read()
value = 5
read()
update()
value
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			want := "3 : Integer [mut]\n20 : Integer\n5 : Integer [mut]\n22 : Integer\n23\n6 : Integer [mut]\n"
			if stdout.String() != want {
				t.Fatalf("retained global binding: got %q, want %q", stdout.String(), want)
			}
		})
	}
}

func TestGlobalBindingsRemainIndependentOfSessionBindings(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			units := []compiler.SourceUnit{
				{Filename: "/project/left.trb", ModulePath: "left", Source: []byte("mut value := 11\ndef read_left(): Integer\nvalue += 1\nreturn value\nend\n")},
				{Filename: "/project/right.trb", ModulePath: "right", Source: []byte("value := 23\ndef read_right(): Integer\nreturn value\nend\n")},
				{Filename: "/project/session.trb", ModulePath: "session", Source: []byte("import { read_left } from left\nimport { read_right } from right\nmut value := 31\nputs(read_left())\nputs(read_right())\nputs(read_left())\nputs(value)\n")},
			}
			artifacts, err := compiler.CompileProject(units, compiler.Options{Mode: mode, GoModule: "example.com/bindings", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project", InteractiveModule: "session"})
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
			if session == nil {
				t.Fatal("missing session")
			}
			var output bytes.Buffer
			evaluator := NewEvaluator(&output, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, "session"); err != nil {
				t.Fatal(err)
			}
			if _, err := evaluator.Evaluate(session.Statements, "session"); err != nil {
				t.Fatal(err)
			}
			if output.String() != "12\n23\n13\n31\n" {
				t.Fatalf("project storage: %q", output.String())
			}
		})
	}
}

func TestGlobalNullableFactsAreCheckedBeforeExecutingSubmission(t *testing.T) {
	const input = `mut value: Integer? := 1
def clear()
  value = nil
end
if value != nil
  clear()
  puts(value + 1)
end
value
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || !strings.Contains(stderr.String(), "operator + does not support Integer? and Integer") {
				t.Fatalf("expected static stale proof rejection: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if stdout.String() != "1 : Integer? [mut]\n1 : Integer? [mut]\n" {
				t.Fatalf("invalid submission changed global storage: %q", stdout.String())
			}
		})
	}
}
