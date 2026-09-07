package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestInteractiveRecordAssignmentAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, body := range []string{"value.value = 2", "value.value += 2", "value.values = []", "value.values.push(3)\nvalue = Sample.new(value: 7, values: value.values)\nvalue.value + value.values[0]"} {
			t.Run(mode+"/"+body, func(t *testing.T) {
				valid := strings.HasPrefix(body, "value.values.push")
				unit := compiler.SourceUnit{
					Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
					Source: []byte("record Sample\nvalue: Integer\nvalues: Array<Integer>\nend\nmut value := Sample.new(value: 1, values: [])\n" + body + "\n"),
				}
				artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: mode, InteractiveModule: unit.ModulePath})
				if !valid {
					if err == nil || !strings.Contains(err.Error(), "is readonly") {
						t.Fatalf("interactive record write diagnostic = %v", err)
					}
					return
				}
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
				if err != nil || !result.Display || Inspect(result.Value) != "10" {
					t.Fatalf("record evaluation = %#v, %v", result, err)
				}
			})
		}
	}
}
