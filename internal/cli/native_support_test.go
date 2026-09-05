package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/project"
)

func TestCompilerSupportRejectsUnsafePathsAndCollisions(t *testing.T) {
	config := project.New(t.TempDir(), "go")
	config.SourceDir = "src"
	for _, path := range []string{"", ".", "..", "../escape.go", "/absolute.go", "a/../escape.go", "C:/absolute.go", "a\\b.go", "a\x00b.go"} {
		compiled := map[string]*compiler.Artifact{"main.trb": {SupportFiles: []codegen.SupportFile{{Path: path}}}}
		if _, err := compiledSupportFiles(config, compiled); err == nil {
			t.Fatalf("accepted path %q", path)
		}
	}
	file := codegen.SupportFile{Path: "trb/runtime/nativefs/lock.go"}
	compiled := map[string]*compiler.Artifact{"main.trb": {IR: &ir.Program{ModulePath: "main"}, SupportFiles: []codegen.SupportFile{file, file}}}
	if _, err := compiledSupportFiles(config, compiled); err == nil {
		t.Fatal("accepted duplicate support file")
	}
	compiled["main.trb"].SupportFiles = []codegen.SupportFile{file}
	compiled["collision.trb"] = &compiler.Artifact{CompilerOwned: true, IR: &ir.Program{ModulePath: "trb/runtime/nativefs/injected"}}
	if _, err := compiledSupportFiles(config, compiled); err == nil {
		t.Fatal("accepted authored support package")
	}
	delete(compiled, "collision.trb")
	if err := os.MkdirAll(filepath.Join(config.Root, "trb/runtime/nativefs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := compiledSupportFiles(config, compiled); err == nil {
		t.Fatal("accepted native-copy injection")
	}
}

func TestCompilerSupportUsesMinimumDependencyVersions(t *testing.T) {
	for _, fixture := range []struct{ name, existing, minimum, wanted string }{
		{"new", "", "v1.2.0", "v1.2.0"},
		{"older", "v1.1.0", "v1.2.0", "v1.2.0"},
		{"same", "v1.2.0", "v1.2.0", "v1.2.0"},
		{"newer", "v1.3.0", "v1.2.0", "v1.3.0"},
		{"prerelease", "v1.2.0-rc.1", "v1.2.0", "v1.2.0"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			config := project.New(t.TempDir(), "go")
			config.Go.Module = "example.com/support"
			filename := filepath.Join(config.Root, "go.mod")
			original := "module example.com/support\n\ngo 1.27\n"
			if fixture.existing != "" {
				original += "\nrequire example.com/native " + fixture.existing + "\n"
			}
			directives := "\nreplace example.com/native => ./native\nexclude example.com/native v1.0.0\n"
			if err := os.WriteFile(filename, []byte(original+directives), 0o600); err != nil {
				t.Fatal(err)
			}
			compiled := map[string]*compiler.Artifact{
				"main.trb":   {NativeDependencies: map[string]string{"example.com/native": fixture.minimum}},
				"helper.trb": {NativeDependencies: map[string]string{"example.com/native": "v1.0.1"}},
			}
			if err := writeCompiledGoDependencies(config, config.Root, compiled); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(filename)
			if err != nil {
				t.Fatal(err)
			}
			for _, wanted := range []string{"require example.com/native " + fixture.wanted, "replace example.com/native => ./native", "exclude example.com/native v1.0.0"} {
				if !strings.Contains(string(data), wanted) {
					t.Fatalf("missing %q in %s", wanted, data)
				}
			}
		})
	}
}
