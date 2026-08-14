package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/project"
)

func TestGeneratedRelativeUsesTSXOnlyForJSXModules(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(config.SourcePath(), "view.trb")
	jsxArtifact := &compiler.Artifact{IR: &ir.Program{ModulePath: "app/view", UsesJSX: true}}
	relative, _ := generatedRelative(config, filename, jsxArtifact)
	if relative != "view.tsx" {
		t.Fatalf("JSX output = %q", relative)
	}
	plainArtifact := &compiler.Artifact{IR: &ir.Program{ModulePath: "app/view"}}
	relative, _ = generatedRelative(config, filename, plainArtifact)
	if relative != "view.ts" {
		t.Fatalf("plain TypeScript output = %q", relative)
	}
}

func TestGeneratedRelativeUsesCanonicalPathForExternalPackage(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	filename := filepath.Join(config.SourcePath(), ".trb", "packages", "checksum", "src", "index.trb")
	artifact := &compiler.Artifact{
		ExternalPackage: true,
		IR:              &ir.Program{ModulePath: "github.com/acme/contracts/index"},
	}
	relative, packageOutput := generatedRelative(config, filename, artifact)
	if !packageOutput {
		t.Fatal("external package artifact was classified as application source")
	}
	if relative != filepath.FromSlash("github.com/acme/contracts/index.ts") {
		t.Fatalf("external package output = %q", relative)
	}
}

func TestReactImportContributesManagedNativeDependencies(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(config.SourcePath(), "main.trb")
	if err := os.WriteFile(filename, []byte("import trb/platform/typescript/react\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dependencies, err := projectPackageDependencies(config, []string{filename})
	if err != nil {
		t.Fatal(err)
	}
	if dependencies["react"] != "latest" || dependencies["react-dom"] != "latest" {
		t.Fatalf("unexpected managed React dependencies: %#v", dependencies)
	}
}
