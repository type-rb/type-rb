package compiler

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestAnalyzerInvalidatesCachedInteractiveFlowAndCopiesBoundaries(t *testing.T) {
	const prefix = "mut value: String? := nil\nvalue = \"kept\"\n"
	units := []SourceUnit{{Filename: "/project/session.trb", ModulePath: "session", Package: "main", Source: []byte(prefix + "value\n")}}
	options := Options{Mode: "go", GoModule: "example.com/interactive", InteractiveModule: "session"}
	analyzer := NewAnalyzer()
	check := func(want string) {
		t.Helper()
		artifacts, err := analyzer.AnalyzeProject(units, options)
		if err != nil {
			t.Fatal(err)
		}
		statements := artifacts[0].IR.Statements
		expression := statements[len(statements)-1].(*ir.ExpressionStatement).Expression
		if got := expression.ExprType().String(); got != want {
			t.Fatalf("expression type=%s, want %s", got, want)
		}
	}
	check("String")
	options.InteractiveFlowResets = []int{len(prefix)}
	check("String?")
	// Changing a caller-owned slice must not rewrite the cached option identity.
	options.InteractiveFlowResets[0] = 0
	check("String")
	options.InteractiveFlowResets[0] = len(prefix)
	check("String?")
}
