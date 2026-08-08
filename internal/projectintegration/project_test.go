package projectintegration

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

type testExtension struct{ name string }

func (e *testExtension) ExtensionName() string { return e.name }

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
