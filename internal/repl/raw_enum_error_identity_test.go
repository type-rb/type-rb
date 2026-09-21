package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestRawEnumStandardErrorIdentityInTypedIREvaluator(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			units := []compiler.SourceUnit{
				{Filename: "/project/model/index.trb", ModulePath: "model/index", Package: "model", Source: []byte("enum Status\nReady = \"ready\"\nend\n")},
				{Filename: "/project/session.trb", ModulePath: "session", Package: "main", Source: []byte(`import { Status } from model
import { Result as Outcome } from trb/std/result
import { EnumValueError as RawError } from trb/std/errors
record EnumValueError
flag: Boolean
end
def parse(text: String): Outcome<Status, RawError>
value := try Status.from_raw(text)
return Outcome<Status, RawError>::Ok(value)
end
value := EnumValueError.new(flag: true)
state := parse("missing") catch |error|
puts(error.message)
Status::Ready
end
puts(state.raw_value())
puts(value.flag)
`)},
			}
			artifacts, err := compiler.CompileProject(units, compiler.Options{Mode: mode, GoModule: "example.com/enum-errors", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project", InteractiveModule: "session"})
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
			if out.String() != "unknown raw value for Status\nready\ntrue\n" {
				t.Fatalf("got %q", out.String())
			}
		})
	}
}
