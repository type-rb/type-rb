package compiler

import (
	"fmt"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/parser"
)

func TestAnalyzerReusesOnlyCompilerIdenticalParsedUnits(t *testing.T) {
	analyzer := NewAnalyzer()
	parseCalls := 0
	analyzer.parse = func(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
		parseCalls++
		return parser.Parse(source)
	}
	sources := []SourceUnit{
		{Filename: "/project/src/alpha.trb", ModulePath: "alpha", Package: "main", Source: []byte("def alpha(): Integer\n\treturn 1\nend\n")},
		{Filename: "/project/src/beta.trb", ModulePath: "beta", Package: "main", Source: []byte("def beta(): Integer\n\treturn 2\nend\n")},
	}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 2 {
		t.Fatalf("initial parse calls=%d, want 2", parseCalls)
	}
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 2 {
		t.Fatalf("unchanged parse calls=%d, want 2", parseCalls)
	}

	sources[1].Source = []byte("def beta(): Integer\n\treturn 3\nend\n")
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 3 {
		t.Fatalf("one changed unit parse calls=%d, want 3", parseCalls)
	}

	options.Mode = "ruby"
	options.GoModule = ""
	if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
		t.Fatal(err)
	}
	if parseCalls != 5 {
		t.Fatalf("changed compiler options parse calls=%d, want 5", parseCalls)
	}
}

func TestAnalyzerReusesNormalizedSyntaxDiagnostics(t *testing.T) {
	analyzer := NewAnalyzer()
	parseCalls := 0
	analyzer.parse = func(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
		parseCalls++
		return parser.Parse(source)
	}
	sources := []SourceUnit{{
		Filename: "/project/src/broken.trb", ModulePath: "broken", Package: "main", Source: []byte("def broken(\n"),
	}}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	for iteration := range 2 {
		diagnostics := compileErrorDiagnostics(t, func() error {
			_, err := analyzer.AnalyzeProject(sources, options)
			return err
		})
		if len(diagnostics) == 0 || diagnostics[0].Path != sources[0].Filename || diagnostics[0].Code != diagnostic.SyntaxError {
			t.Fatalf("unexpected cached diagnostics: %#v", diagnostics)
		}
		if iteration == 0 {
			diagnostics[0].Path = "/mutated/by/caller.trb"
		}
	}
	if parseCalls != 1 {
		t.Fatalf("syntax-error parse calls=%d, want 1", parseCalls)
	}
}

func BenchmarkProjectAnalysisAfterOneFileEdit(b *testing.B) {
	const modules = 64
	base := make([]SourceUnit, 0, modules)
	for index := 0; index < modules; index++ {
		base = append(base, SourceUnit{
			Filename:   fmt.Sprintf("/project/src/module_%03d.trb", index),
			ModulePath: fmt.Sprintf("module_%03d", index),
			Package:    "main",
			Source:     []byte(fmt.Sprintf("def value_%03d(): Integer\n\treturn %d\nend\n", index, index)),
		})
	}
	options := Options{Mode: "go", GoModule: "example.com/analyzer", SourceRoot: "/project/src", ProjectRoot: "/project"}

	b.Run("one-shot", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		iteration := 0
		for b.Loop() {
			iteration++
			sources[modules-1].Source = []byte(fmt.Sprintf("def value_063(): Integer\n\treturn %d\nend\n", iteration))
			if _, err := AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("reused-syntax", func(b *testing.B) {
		sources := append([]SourceUnit(nil), base...)
		analyzer := NewAnalyzer()
		if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
			b.Fatal(err)
		}
		iteration := 0
		b.ResetTimer()
		for b.Loop() {
			iteration++
			sources[modules-1].Source = []byte(fmt.Sprintf("def value_063(): Integer\n\treturn %d\nend\n", iteration))
			if _, err := analyzer.AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
}
