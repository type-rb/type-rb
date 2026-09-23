package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestLogicalAssignmentSkipsRHSAndEvaluatesIndexOnce(t *testing.T) {
	const helpers = `def tick(mut calls: Array<Integer>): Boolean
  calls[0] += 1
  return true
end
def at(mut indices: Array<Integer>): Integer
  indices[0] += 1
  return 0
end
`
	tests := []struct {
		name   string
		body   string
		result string
	}{
		{
			name: "bindings",
			body: `mut calls := [0]
mut yes := true
mut no := false
yes ||= tick(calls)
no &&= tick(calls)
no ||= tick(calls)
yes &&= tick(calls)
calls[0]
`,
			result: "2",
		},
		{
			name: "indexed target",
			body: `mut calls := [0]
mut indices := [0]
mut flags := [true]
flags[at(indices)] ||= tick(calls)
flags[at(indices)] &&= tick(calls)
calls[0] * 10 + indices[0]
`,
			result: "12",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				unit := compiler.SourceUnit{
					Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
					Source: []byte(helpers + test.body),
				}
				artifacts, err := compiler.CompileProject([]compiler.SourceUnit{unit}, compiler.Options{Mode: mode, InteractiveModule: unit.ModulePath})
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
				if err != nil || !result.Display || Inspect(result.Value) != test.result {
					t.Fatalf("logical assignment result = %#v, %v; want %s", result, err, test.result)
				}
			})
		}
	}
}
