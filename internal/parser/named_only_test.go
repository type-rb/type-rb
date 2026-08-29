package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseBareStarNamedOnlyParameters(t *testing.T) {
	program, diagnostics := Parse([]byte(`def request(url: String, *, timeout: Integer, retries: Integer = 2): String
	return url
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	if len(method.Parameters) != 3 || method.Parameters[0].NamedOnly || !method.Parameters[1].NamedOnly || !method.Parameters[2].NamedOnly {
		t.Fatalf("unexpected parameter regions: %#v", method.Parameters)
	}
	if method.Parameters[1].Name != "timeout" || method.Parameters[1].Default != nil || method.Parameters[2].Default == nil {
		t.Fatalf("named-only presence and default expression were not preserved: %#v", method.Parameters)
	}
}

func TestParseRejectsRemovedDoubleColonParameterSyntax(t *testing.T) {
	_, diagnostics := Parse([]byte("def describe(label:: String): String\n\treturn label\nend\n"))
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "name:: Type was removed") {
		t.Fatalf("expected :: migration diagnostic, got %v", diagnostics)
	}
}

func TestParseNamedOnlyEnumPayloadsAndPatterns(t *testing.T) {
	program, diagnostics := Parse([]byte(`enum Change
	Renamed(id: Integer, *, before: String, after: String)
end

def describe(change: Change): String
	case change
	when Change::Renamed(id, after: current, before: previous)
		return previous + current + id.to_s()
	end
end
`))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	enum := program.Statements[0].(*ast.EnumStatement)
	member := enum.Body[0].(*ast.EnumMemberStatement)
	if len(member.Parameters) != 3 || member.Parameters[0].NamedOnly || !member.Parameters[1].NamedOnly || !member.Parameters[2].NamedOnly {
		t.Fatalf("unexpected enum payload parameter regions: %#v", member.Parameters)
	}
	method := program.Statements[1].(*ast.MethodStatement)
	caseStatement := method.Body[0].(*ast.CaseStatement)
	bindings := caseStatement.Branches[0].Bindings
	if len(bindings) != 3 || bindings[0].Label != "" || bindings[1].Label != "after" || bindings[1].Name != "current" || bindings[2].Label != "before" || bindings[2].Name != "previous" {
		t.Fatalf("named enum pattern bindings were not retained: %#v", bindings)
	}
}
