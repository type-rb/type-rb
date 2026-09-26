package compiler

import (
	"strings"
	"testing"
)

func TestTRBModeAnalyzesWithoutGeneratingSource(t *testing.T) {
	sources := []SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte("def main()\n\tputs(\"ok\")\nend\n")}}
	artifacts, err := AnalyzeProject(sources, Options{Mode: "trb"})
	if err != nil {
		t.Fatalf("analyze mode trb: %v", err)
	}
	if len(artifacts) == 0 || artifacts[0].Mode != "trb" || artifacts[0].IR == nil || len(artifacts[0].Output) != 0 {
		t.Fatalf("unexpected analysis artifacts: %#v", artifacts)
	}
	if _, err := CompileProject(sources, Options{Mode: "trb"}); err == nil || !strings.Contains(err.Error(), "code generation is not available for mode trb in this implementation") {
		t.Fatalf("compile mode trb error = %v", err)
	}
	invalid := []SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte("def main()\n\tcount: Integer := \"one\"\n\tputs(count.to_s())\nend\n")}}
	if _, err := AnalyzeProject(invalid, Options{Mode: "trb"}); err == nil || !strings.Contains(err.Error(), "TRB3000") {
		t.Fatalf("mode trb must keep portable type errors, got %v", err)
	}
	if _, err := AnalyzeProject(sources, Options{Mode: "native"}); err == nil || !strings.Contains(err.Error(), `unsupported mode "native"`) {
		t.Fatalf("unknown mode error = %v", err)
	}
}

func TestTRBModeIncrementalAnalysisSkipsMissingBackendValidation(t *testing.T) {
	analyzer := NewAnalyzer()
	first := []SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte("def main()\n\tputs(\"first\")\nend\n")}}
	if _, err := analyzer.AnalyzeProject(first, Options{Mode: "trb"}); err != nil {
		t.Fatalf("initial analysis: %v", err)
	}
	changed := []SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte("def main()\n\tputs(\"changed\")\nend\n")}}
	artifacts, err := analyzer.AnalyzeProject(changed, Options{Mode: "trb"})
	if err != nil {
		t.Fatalf("incremental analysis of a valid edit: %v", err)
	}
	if len(artifacts) == 0 || artifacts[0].IR == nil {
		t.Fatalf("incremental analysis returned no artifacts: %#v", artifacts)
	}
}
