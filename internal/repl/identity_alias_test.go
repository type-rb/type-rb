package repl

import (
	"bytes"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
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
