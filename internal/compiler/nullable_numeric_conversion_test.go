package compiler

import (
	"os"
	"strings"
	"testing"
)

func TestNullableNumericConversionRunsAcrossBackends(t *testing.T) {
	source, err := os.ReadFile("testdata/nullable_numeric_conversion.trb")
	if err != nil {
		t.Fatal(err)
	}
	want := ""
	for _, label := range []string{"nil", "0.0", "3.5"} {
		// Float rendering differs by backend for a whole value; normalize only that label below.
		want += strings.Repeat("source\n"+label+"\n", 6) + label + "\n"
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{Mode: mode, GoModule: "example.com/nullable-numeric", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project"})
			if err != nil {
				t.Fatal(err)
			}
			got := runEffectProject(t, mode, artifacts, "example.com/nullable-numeric")
			got = strings.ReplaceAll(got, "\n0\n", "\n0.0\n")
			if strings.TrimSpace(got) != strings.TrimSpace(want) {
				t.Fatalf("output=%q, want %q", got, want)
			}
		})
	}
}
