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

func TestCompilerSupportDependencyConflictDoesNotRewriteModule(t *testing.T) {
	config := project.New(t.TempDir(), "go")
	config.Go.Module = "example.com/support"
	filename := filepath.Join(config.Root, "go.mod")
	original := "module example.com/support\n\ngo 1.27\n\nrequire example.com/native v1.0.0\n"
	if err := os.WriteFile(filename, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	compiled := map[string]*compiler.Artifact{"main.trb": {NativeDependencies: map[string]string{"example.com/native": "v2.0.0"}}}
	if err := writeCompiledGoDependencies(config, config.Root, compiled); err == nil || !strings.Contains(err.Error(), "module requires") {
		t.Fatalf("got %v", err)
	}
	data, err := os.ReadFile(filename)
	if err != nil || string(data) != original {
		t.Fatalf("changed module on failure: %q, %v", data, err)
	}
}
