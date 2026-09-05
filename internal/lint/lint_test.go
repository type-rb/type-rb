package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestPreferConditionalTransferReportsAndFixesSimpleGuard(t *testing.T) {
	source := []byte(`def classify(enabled: Boolean): String
	if enabled
		return "enabled"
	end
	return "disabled"
end
`)
	program, parseDiagnostics := parser.Parse(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", parseDiagnostics)
	}
	options, err := Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := Analyze(program, source, "/project/main.trb", options)
	if len(items) != 1 || items[0].Code != diagnostic.Code(PreferConditionalTransferRuleID) || items[0].Severity != diagnostic.Warning || len(items[0].Fixes) != 1 {
		t.Fatalf("diagnostics=%#v", items)
	}
	fixed, count, err := ApplyFixes(source, "/project/main.trb", items)
	if err != nil {
		t.Fatal(err)
	}
	want := "def classify(enabled: Boolean): String\n\treturn \"enabled\" if enabled\n\treturn \"disabled\"\nend\n"
	if count != 1 || string(fixed) != want {
		t.Fatalf("count=%d\nwant:\n%s\ngot:\n%s", count, want, fixed)
	}
}

func TestPreferConditionalTransferVisitsNewtypeBodies(t *testing.T) {
	source := []byte("newtype Label = String do\nprivate new\ndef self.parse(source: String): Label?\nif source.empty?()\nreturn nil\nend\nreturn self.new(source)\nend\nend\n")
	program, diagnostics := parser.Parse(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", diagnostics)
	}
	options, err := Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := Analyze(program, source, "/project/main.trb", options)
	if len(items) != 1 || items[0].Code != diagnostic.Code(PreferConditionalTransferRuleID) {
		t.Fatalf("diagnostics=%#v", items)
	}
}

func TestPreferConditionalTransferHonorsConfigurationAndSafetyBoundary(t *testing.T) {
	source := []byte(`def classify(enabled: Boolean): String
	if enabled # explanation
		return "enabled"
	end
	return "disabled"
end
`)
	program, diagnostics := parser.Parse(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", diagnostics)
	}
	recommended, _ := Resolve(Options{})
	if items := Analyze(program, source, "main.trb", recommended); len(items) != 0 {
		t.Fatalf("comment-preserving rewrite must not be offered: %#v", items)
	}
	disabled, err := Resolve(Options{Rules: map[string]string{PreferConditionalTransferRuleID: "off"}})
	if err != nil {
		t.Fatal(err)
	}
	if items := Analyze(program, source, "main.trb", disabled); len(items) != 0 {
		t.Fatalf("disabled diagnostics=%#v", items)
	}
}

func TestPreferConditionalTransferUsesRenderedUnicodeWidth(t *testing.T) {
	source := []byte("def label(enabled: Boolean): String\n\tif enabled\n\t\treturn \"" + strings.Repeat("界", 54) + "\"\n\tend\n\treturn \"disabled\"\nend\n")
	program, diagnostics := parser.Parse(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", diagnostics)
	}
	options, _ := Resolve(Options{})
	if items := Analyze(program, source, "main.trb", options); len(items) != 0 {
		t.Fatalf("display-wide rewrite must not be offered: %#v", items)
	}
}

func TestResolveRejectsUnknownRulesAndSupportsErrorLevel(t *testing.T) {
	if _, err := Resolve(Options{Rules: map[string]string{"trb/unknown": "warning"}}); err == nil || !strings.Contains(err.Error(), "unknown lint rule") {
		t.Fatalf("unknown rule error=%v", err)
	}
	options, err := Resolve(Options{Rules: map[string]string{
		PreferConditionalTransferRuleID: "error",
		OmitTerminalVoidReturnRuleID:    "off",
	}})
	if err != nil {
		t.Fatal(err)
	}
	source := []byte("def stop(enabled: Boolean)\n\tif enabled\n\t\treturn\n\tend\n\treturn\nend\n")
	program, _ := parser.Parse(source)
	items := Analyze(program, source, "main.trb", options)
	if len(items) != 1 || items[0].Severity != diagnostic.Error {
		t.Fatalf("diagnostics=%#v", items)
	}
}

func TestOmitTerminalVoidReturnReportsAndFixesDefsAndFunctionValues(t *testing.T) {
	source := []byte(`def log_value(value: String)
	puts(value)
	return
	# Keep this closing comment.
end

callback := fn()
	puts("callback")
	return
end
`)
	program, parseDiagnostics := parser.Parse(source)
	if len(parseDiagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", parseDiagnostics)
	}
	options, err := Resolve(Options{})
	if err != nil {
		t.Fatal(err)
	}
	items := Analyze(program, source, "/project/main.trb", options)
	if len(items) != 2 {
		t.Fatalf("diagnostics=%#v", items)
	}
	for _, item := range items {
		if item.Code != diagnostic.Code(OmitTerminalVoidReturnRuleID) || item.Severity != diagnostic.Warning || len(item.Fixes) != 1 {
			t.Fatalf("diagnostic=%#v", item)
		}
	}
	fixed, count, err := ApplyFixes(source, "/project/main.trb", items)
	if err != nil {
		t.Fatal(err)
	}
	want := `def log_value(value: String)
	puts(value)
	# Keep this closing comment.
end

callback := fn()
	puts("callback")
end
`
	if count != 2 || string(fixed) != want {
		t.Fatalf("count=%d\nwant:\n%s\ngot:\n%s", count, want, fixed)
	}
}

func TestOmitTerminalVoidReturnKeepsSemanticAndCommentedReturns(t *testing.T) {
	source := []byte(`def stop_if(should_stop: Boolean)
	return if should_stop
	puts("continued")
end

def described()
	return # documents an intentional boundary
end

def label(): String
	return "ready"
end
`)
	program, diagnostics := parser.Parse(source)
	if len(diagnostics) != 0 {
		t.Fatalf("parse diagnostics=%v", diagnostics)
	}
	options, _ := Resolve(Options{})
	if items := Analyze(program, source, "main.trb", options); len(items) != 0 {
		t.Fatalf("diagnostics=%#v", items)
	}
}

func TestOmitTerminalVoidReturnHonorsConfiguration(t *testing.T) {
	source := []byte("def stop()\n\treturn\nend\n")
	program, _ := parser.Parse(source)
	disabled, err := Resolve(Options{Rules: map[string]string{OmitTerminalVoidReturnRuleID: "off"}})
	if err != nil {
		t.Fatal(err)
	}
	if items := Analyze(program, source, "main.trb", disabled); len(items) != 0 {
		t.Fatalf("disabled diagnostics=%#v", items)
	}
	errors, err := Resolve(Options{Preset: NoRulesPreset, Rules: map[string]string{OmitTerminalVoidReturnRuleID: "error"}})
	if err != nil {
		t.Fatal(err)
	}
	items := Analyze(program, source, "main.trb", errors)
	if len(items) != 1 || items[0].Severity != diagnostic.Error {
		t.Fatalf("diagnostics=%#v", items)
	}
}

func TestRegistryRulesHaveDedicatedDocumentation(t *testing.T) {
	index, err := os.ReadFile(filepath.Join("..", "..", "docs", "lint-rules", "index.md"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, rule := range Registry() {
		if rule.ID == "" || rule.Documentation == "" || seen[rule.ID] {
			t.Fatalf("invalid registry metadata: %#v", rule)
		}
		seen[rule.ID] = true
		if !strings.Contains(string(index), rule.ID) || !strings.Contains(string(index), rule.Documentation+".md") {
			t.Fatalf("rule %s is missing from the index", rule.ID)
		}
		page, err := os.ReadFile(filepath.Join("..", "..", "docs", "lint-rules", rule.Documentation+".md"))
		if err != nil {
			t.Fatalf("rule %s documentation: %v", rule.ID, err)
		}
		if !strings.Contains(string(page), rule.ID) || !strings.Contains(string(page), rule.Since) {
			t.Fatalf("rule %s documentation is missing its ID or since version", rule.ID)
		}
	}
}
