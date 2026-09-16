package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestImportedGenericEnumPatternsInTypedIREvaluator(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			units := []compiler.SourceUnit{
				{Filename: "/project/item.trb", ModulePath: "item", Package: "item", Source: []byte("enum Item<T>\nValue(value: T)\nend\n")},
				{Filename: "/project/session.trb", ModulePath: "session", Package: "main", Source: []byte(`import { Item as Choice } from item
value := Choice<String>::Value("held")
case value
when Choice::Value(text)
puts(text)
end
case Choice<Integer>::Value(7)
when Choice::Value(number)
puts(number + 1)
end
`)},
			}
			artifacts, err := compiler.CompileProject(units, compiler.Options{Mode: mode, GoModule: "example.com/generic-enums", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project", InteractiveModule: "session"})
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
			if out.String() != "held\n8\n" {
				t.Fatalf("got %q", out.String())
			}
		})
	}
}
