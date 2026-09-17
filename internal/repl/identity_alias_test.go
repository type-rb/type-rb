package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestInteractiveGenericIdentityAliasesAcrossModes(t *testing.T) {
	source := []byte(`alias Identity<T> = T
alias Values<T> = Array<Identity<T>>
def keep<T>(value: Identity<T>): Identity<T>
 return value
end
values: Values<String> := ["held"]
keep<String>(values[0])
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "session.trb", ModulePath: "session", Package: "main", Source: source}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: mode, InteractiveModule: unit.ModulePath})
			if err != nil {
				t.Fatal(err)
			}
			program := artifacts[0].IR
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			evaluator.LoadDefinitions(program)
			result, err := evaluator.Evaluate(program.Statements, unit.ModulePath)
			if err != nil || !result.Display || Inspect(result.Value) != `"held"` {
				t.Fatalf("identity alias evaluation: %#v, %v", result, err)
			}
		})
	}
}

func TestInteractiveImportedAliasIdentitiesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			units := []compiler.SourceUnit{
				{Filename: "models/aliases.trb", ModulePath: "models/aliases", Package: "models", Source: []byte("alias Identity<T> = T\ndef keep<T>(value: T): Identity<T>\nreturn value\nend\n")},
				{Filename: "collections/aliases.trb", ModulePath: "collections/aliases", Package: "collections", Source: []byte("import { Identity as Item } from models/aliases\nalias Identity<T> = Array<Item<T>>\n")},
				{Filename: "session.trb", ModulePath: "session", Package: "main", Source: []byte("import { keep } from models/aliases\nimport { Identity as List } from collections/aliases\nrecord Packet\nvalue: String\nend\nvalues: List<String> := [\"held\"]\nitem := keep<Packet>(Packet.new(value: values[0]))\nitem.value\n")},
			}
			artifacts, err := compiler.CompileProject(units, compiler.Options{Mode: mode, InteractiveModule: "session"})
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
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, "session"); err != nil {
				t.Fatal(err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, "session")
			if err != nil || !result.Display || Inspect(result.Value) != `"held"` {
				t.Fatalf("imported alias evaluation: %#v, %v", result, err)
			}
		})
	}
}
