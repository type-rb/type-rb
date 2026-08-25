package ruby

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestPrivateTopLevelFunctionsUseModuleScopedNames(t *testing.T) {
	generate := func(modulePath string) string {
		return Generate(&ir.Program{
			Mode:       "ruby",
			ModulePath: modulePath,
			Statements: []ir.Statement{
				&ir.Method{Name: "_invalid"},
				&ir.Method{Name: "__trb_specialize_bind_42"},
			},
		})
	}

	firstTarget := rubyPrivateFunctionName("alpha", "_invalid")
	secondTarget := rubyPrivateFunctionName("beta", "_invalid")
	if firstTarget == secondTarget {
		t.Fatalf("private Ruby function targets collided: %q", firstTarget)
	}
	for modulePath, target := range map[string]string{"alpha": firstTarget, "beta": secondTarget} {
		output := generate(modulePath)
		if !strings.Contains(output, "def "+target+"(") || strings.Contains(output, "def _invalid(") {
			t.Fatalf("generated %s private function is not module scoped:\n%s", modulePath, output)
		}
		if !strings.Contains(output, "def __trb_specialize_bind_42(") {
			t.Fatalf("generated %s compiler-owned function was renamed:\n%s", modulePath, output)
		}
	}
}

func TestCollidingTopLevelFunctionsUseModuleScopedNames(t *testing.T) {
	programs := []*ir.Program{
		{Mode: "ruby", ModulePath: "alpha", Statements: []ir.Statement{&ir.Method{Name: "render"}, &ir.Method{Name: "alpha_only"}}},
		{Mode: "ruby", ModulePath: "beta", Statements: []ir.Statement{&ir.Method{Name: "render"}}},
	}
	names := analyzeRubyProjectNames(programs)
	alpha := names.functions["alpha"]["render"]
	beta := names.functions["beta"]["render"]
	if alpha == "" || beta == "" || alpha == beta || alpha == "render" || beta == "render" {
		t.Fatalf("colliding Ruby functions were not module scoped: alpha=%q beta=%q", alpha, beta)
	}
	if got := names.functions["alpha"]["alpha_only"]; got != "alpha_only" {
		t.Fatalf("non-colliding Ruby function changed from its source name: %q", got)
	}
}

func TestGeneratedTopLevelFunctionDoesNotReplaceExternalTarget(t *testing.T) {
	programs := []*ir.Program{
		{Mode: "ruby", ModulePath: "native", Statements: []ir.Statement{&ir.Method{Name: "render", External: true}}},
		{Mode: "ruby", ModulePath: "portable", Statements: []ir.Statement{&ir.Method{Name: "render"}}},
	}
	names := analyzeRubyProjectNames(programs)
	if got := names.functions["native"]["render"]; got != "render" {
		t.Fatalf("external Ruby function target changed: %q", got)
	}
	if got := names.functions["portable"]["render"]; got == "" || got == "render" {
		t.Fatalf("generated Ruby function did not move away from external target: %q", got)
	}
}

func TestNamespaceImportedCollidingFunctionUsesModuleScopedName(t *testing.T) {
	programs := []*ir.Program{
		{Mode: "ruby", ModulePath: "alpha", Statements: []ir.Statement{&ir.Method{Name: "render"}}},
		{Mode: "ruby", ModulePath: "beta", Statements: []ir.Statement{&ir.Method{Name: "render"}}},
	}
	names := analyzeRubyProjectNames(programs)
	g := &generator{modulePath: "main", projectNames: names, topTargets: map[string]string{}}
	reference := &ir.Reference{Package: "alpha", Alias: "alpha", Symbol: "render", ExportKind: "function"}
	member := &ir.Member{Receiver: &ir.Identifier{Name: "alpha"}, Name: "render", Reference: reference}
	if got, want := g.expr(member), names.functions["alpha"]["render"]; got != want {
		t.Fatalf("namespace-imported Ruby function target = %q, want %q", got, want)
	}
}
