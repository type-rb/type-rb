package compiler

import (
	"strings"
	"testing"
)

func TestAnalyzerInvalidatesImportedInferredConstants(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			sources := []SourceUnit{
				{Filename: "/project/values.trb", ModulePath: "values", Source: []byte("VALUE := 1 + 2\n")},
				{Filename: "/project/middle.trb", ModulePath: "middle", Source: []byte("import { VALUE } from values\nCOPY := VALUE\n")},
				{Filename: "/project/main.trb", ModulePath: "main", Source: []byte("import { COPY } from middle\ndef main()\nputs(COPY + 1)\nend\n")},
			}
			options := Options{Mode: mode, GoModule: "example.com/constants", RubyLoader: "require_relative", ProjectRoot: "/project", SourceRoot: "/project"}
			analyzer := NewAnalyzer()
			initial, err := analyzer.AnalyzeProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			requireAnalysisMatchesFullCompilation(t, initial, sources, options)
			// Both syntax-only exports are Any; checked String must invalidate
			// even a transitive consumer, without mutating the accepted snapshot.
			sources[0].Source = []byte("VALUE := \"a\" + \"b\"\n")
			_, incremental := analyzer.AnalyzeProject(sources, options)
			_, fresh := AnalyzeProject(sources, options)
			if incremental == nil || fresh == nil || incremental.Error() != fresh.Error() || !strings.Contains(incremental.Error(), "String and Integer") {
				t.Fatalf("incremental=%v; fresh=%v", incremental, fresh)
			}
			sources[0].Source = []byte("VALUE := 3 + 4\n")
			recovered, err := analyzer.AnalyzeProject(sources, options)
			if err != nil {
				t.Fatal(err)
			}
			requireAnalysisMatchesFullCompilation(t, recovered, sources, options)
		})
	}
}

func TestImportedInferredConstantsPreserveCycleRejection(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/first.trb", ModulePath: "first", Source: []byte("import { SECOND } from second\nFIRST := SECOND\n")},
		{Filename: "/project/second.trb", ModulePath: "second", Source: []byte("import { FIRST } from first\nSECOND := FIRST\n")},
	}
	_, err := AnalyzeProject(sources, Options{Mode: "go", GoModule: "example.com/constants", ProjectRoot: "/project", SourceRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "import cycle") {
		t.Fatalf("cycle error=%v", err)
	}
}
