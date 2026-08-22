package projectintegration

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

type testExtension struct{ name string }

func (e *testExtension) ExtensionName() string { return e.name }

type reusableTestExtension struct {
	name    string
	version int
	modules map[string]bool
}

func (e *reusableTestExtension) ExtensionName() string { return e.name }

func (e *reusableTestExtension) EquivalentForIncrementalLowering(extension ir.Extension) bool {
	other, ok := extension.(*reusableTestExtension)
	return ok && e.name == other.name && e.version == other.version
}

func (e *reusableTestExtension) RequiresIncrementalRelowering(modulePath string) bool {
	return e.modules[modulePath]
}

func TestAnalysisAppliesPackageContributionsWithoutDomainKnowledge(t *testing.T) {
	first := &testExtension{name: "test.first"}
	second := &testExtension{name: "test.second"}
	analysis := Analysis{contributions: []Contribution{
		{
			Extension: first,
			MethodTargets: map[string]map[string]string{
				"routes/items": {"get": "generated_get"},
			},
		},
		{Extension: second, MethodTargets: map[string]map[string]string{}},
	}}
	program := &ir.Program{
		ModulePath: "routes/items",
		Statements: []ir.Statement{&ir.Method{Name: "get"}, &ir.Method{Name: "helper"}},
	}

	analysis.Apply(program, true)

	get := program.Statements[0].(*ir.Method)
	helper := program.Statements[1].(*ir.Method)
	if get.TargetName != "generated_get" || helper.TargetName != "" {
		t.Fatalf("unexpected method targets: get=%q helper=%q", get.TargetName, helper.TargetName)
	}
	if len(program.Extensions) != 2 || program.Extensions[0] != first || program.Extensions[1] != second {
		t.Fatalf("unexpected extensions: %#v", program.Extensions)
	}

	library := &ir.Program{ModulePath: "library"}
	analysis.Apply(library, false)
	if len(library.Extensions) != 0 {
		t.Fatalf("extensions were attached to a non-entrypoint module: %#v", library.Extensions)
	}
}

func TestAnalysisReusesLoweredProgramsOnlyForEquivalentSafeContributions(t *testing.T) {
	contribution := func(version int, targets map[string]map[string]string) Analysis {
		return Analysis{contributions: []Contribution{{
			Extension:     &reusableTestExtension{name: "test.reusable", version: version, modules: map[string]bool{"models/product": true}},
			MethodTargets: targets,
			AllPrograms:   true,
		}}}
	}
	previous := contribution(1, map[string]map[string]string{"routes/items": {"get": "generated_get"}})
	current := contribution(1, map[string]map[string]string{"routes/items": {"get": "generated_get"}})
	if !current.CanReuseLoweredPrograms(previous, map[string]bool{"__trb_repl__": true}) {
		t.Fatal("equivalent contribution rejected an unrelated incremental edit")
	}
	if current.CanReuseLoweredPrograms(previous, map[string]bool{"models/product": true}) {
		t.Fatal("extension-owned lowering input reused stale IR")
	}
	if contribution(2, current.contributions[0].MethodTargets).CanReuseLoweredPrograms(previous, map[string]bool{"__trb_repl__": true}) {
		t.Fatal("changed extension contribution reused stale IR")
	}
	if contribution(1, map[string]map[string]string{"routes/items": {"get": "different_get"}}).CanReuseLoweredPrograms(previous, map[string]bool{"__trb_repl__": true}) {
		t.Fatal("changed method target reused stale IR")
	}
}
