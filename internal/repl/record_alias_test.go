package repl

import (
	"bytes"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"testing"
)

func TestInteractiveRecordAliasConstructionAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			unit := compiler.SourceUnit{Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte("import { Holder } from models/box\nbox := Holder<Integer>.new(value: 7)\nbox.copy\n")}
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{
				{Filename: "/project/models/box.trb", ModulePath: "models/box", Package: "models", Source: []byte("record Box<T>\nvalue: T\ncopy: T = value\nend\nalias Holder<T> = Box<T>\n")}, unit,
			}, compiler.Options{Mode: mode, InteractiveModule: unit.ModulePath, SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			var session *ir.Program
			var programs []*ir.Program
			for _, artifact := range artifacts {
				programs = append(programs, artifact.IR)
				if artifact.IR.ModulePath == unit.ModulePath {
					session = artifact.IR
				}
			}
			if session == nil {
				t.Fatal("missing interactive program")
			}
			evaluator := NewEvaluator(&bytes.Buffer{}, mode)
			t.Cleanup(func() { _ = evaluator.Close() })
			if err := evaluator.LoadProject(programs, unit.ModulePath); err != nil {
				t.Fatal(err)
			}
			evaluator.LoadDefinitions(session)
			result, err := evaluator.Evaluate(session.Statements, unit.ModulePath)
			if err != nil || !result.Display || Inspect(result.Value) != "7" {
				t.Fatalf("alias record evaluation: %#v, %v", result, err)
			}
		})
	}
}
