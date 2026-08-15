package testsuite

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestPrepareDiscoversNestedTestsAndCreatesRegistration(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(1 + 2).to_equal(3)
	end

	describe("negative values") do
		test("keeps the sign") do
			expect(0 - 2).to_equal(0 - 2)
		end
	end
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	items, issues := Prepare(program, "/project/src/calculator_test.trb", "trb_test_register_abc")
	if len(issues) != 0 {
		t.Fatalf("test diagnostics: %#v", issues)
	}
	if len(items) != 4 {
		t.Fatalf("got %d test items, want 4", len(items))
	}
	if items[0].Kind != Suite || items[0].FullName != "Calculator" {
		t.Fatalf("unexpected suite: %#v", items[0])
	}
	if items[1].Kind != Case || items[1].FullName != "Calculator / adds numbers" || items[1].ParentID != items[0].ID {
		t.Fatalf("unexpected first case: %#v", items[1])
	}
	if items[3].FullName != "Calculator / negative values / keeps the sign" || items[3].ParentID != items[2].ID {
		t.Fatalf("unexpected nested case: %#v", items[3])
	}
	method, ok := program.Statements[len(program.Statements)-1].(*ast.MethodStatement)
	if !ok || method.Name != "trb_test_register_abc" || len(method.Body) != 1 {
		t.Fatalf("missing generated registration method: %#v", program.Statements)
	}
}

func TestPrepareRejectsNamespacedTestImports(t *testing.T) {
	program, diagnostics := parser.Parse([]byte(`import trb/std/test as testing

testing.describe("Calculator") do
	testing.test("adds numbers") do
	end
end
`))
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics: %#v", diagnostics)
	}
	items, issues := Prepare(program, "/project/src/calculator_test.trb", "trb_test_register_abc")
	if len(items) != 0 || len(issues) != 1 || !strings.Contains(issues[0].Message, "named imports") {
		t.Fatalf("unexpected namespace result: items=%#v diagnostics=%#v", items, issues)
	}
}

func TestPrepareRejectsAmbiguousTestDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"top-level test", `import { describe, test } from trb/std/test

test("outside") do
end
`, "top-level describe"},
		{"missing suite", `def helper(): Integer
	return 1
end
`, "top-level describe"},
		{"dynamic name", `import { describe, test } from trb/std/test

name := "dynamic"
describe(name) do
	test("case") do
	end
end
`, "String literal"},
		{"statements in suite", `import { describe, test } from trb/std/test

describe("suite") do
	value := 1
	test("case") do
	end
end
`, "only nested"},
		{"duplicate", `import { describe, test } from trb/std/test

describe("suite") do
	test("case") do
	end
	test("case") do
	end
end
`, "already declared"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			program, parseDiagnostics := parser.Parse([]byte(test.source))
			if len(parseDiagnostics) != 0 {
				t.Fatalf("parse diagnostics: %#v", parseDiagnostics)
			}
			_, diagnostics := Prepare(program, "/project/src/example_test.trb", "")
			var messages []string
			for _, item := range diagnostics {
				messages = append(messages, item.Message)
			}
			if !strings.Contains(strings.Join(messages, "\n"), test.want) {
				t.Fatalf("diagnostics %q do not contain %q", messages, test.want)
			}
		})
	}
}
