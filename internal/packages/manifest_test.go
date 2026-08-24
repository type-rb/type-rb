package packages

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestSyncRubyGemfile(t *testing.T) {
	config := project.New(t.TempDir(), "ruby")
	config.Dependencies["rails"] = "~> 8.0"
	config.DevDependencies["rspec-rails"] = "~> 7.0"
	path, err := Sync(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{`source "https://rubygems.org"`, `ruby "` + project.DefaultRubyVersion + `"`, `gem "rails", "~> 8.0"`, `gem "rspec-rails", "~> 7.0"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("Gemfile does not contain %q:\n%s", want, text)
		}
	}
	version, err := os.ReadFile(filepath.Join(config.Root, ".ruby-version"))
	if err != nil {
		t.Fatal(err)
	}
	if string(version) != project.DefaultRubyVersion+"\n" {
		t.Fatalf("unexpected .ruby-version: %q", version)
	}
}

func TestSyncGoMod(t *testing.T) {
	config := project.New(t.TempDir(), "go")
	config.Go.Module = "example.com/acme/service"
	config.Dependencies["golang.org/x/text"] = "v0.27.0"
	path, err := Sync(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"module example.com/acme/service", "golang.org/x/text v0.27.0"} {
		if !strings.Contains(text, want) {
			t.Fatalf("go.mod does not contain %q:\n%s", want, text)
		}
	}
}

func TestSyncAddsImportedTypeRBPackageDependencies(t *testing.T) {
	config := project.New(t.TempDir(), "go")
	config.Go.Module = "example.com/acme/service"
	path, err := SyncWithDependencies(config, map[string]string{"modernc.org/sqlite": "v1.53.0"})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "modernc.org/sqlite v1.53.0") {
		t.Fatalf("go.mod does not contain package-owned dependency:\n%s", data)
	}
	config.Dependencies["modernc.org/sqlite"] = "v1.52.0"
	if _, err := SyncWithDependencies(config, map[string]string{"modernc.org/sqlite": "v1.53.0"}); err == nil || !strings.Contains(err.Error(), "requires v1.53.0") {
		t.Fatalf("expected package dependency conflict, got %v", err)
	}
}

func TestSyncNpmPackage(t *testing.T) {
	config := project.New(t.TempDir(), "typescript")
	config.Dependencies["zod"] = "^4.0.0"
	config.TypeScript.Scripts["check"] = "tsc && bun test < fixtures/input.ts"
	path, err := Sync(config)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) != "package.json" {
		t.Fatalf("unexpected manifest path %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Type            string            `json:"type"`
		Scripts         map[string]string `json:"scripts"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Type != "module" || manifest.Dependencies["zod"] != "^4.0.0" || manifest.DevDependencies["typescript"] != project.DefaultTypeScriptVersion {
		t.Fatalf("unexpected package.json: %s", data)
	}
	if manifest.Scripts["check"] != config.TypeScript.Scripts["check"] {
		t.Fatalf("unexpected package.json script: %s", data)
	}
	if !strings.Contains(string(data), `"check": "tsc && bun test < fixtures/input.ts"`) {
		t.Fatalf("package.json HTML-escapes its shell script:\n%s", data)
	}
}

func TestSyncNpmPackageAddsRequiredTypeDeclarationsAsDevelopmentDependencies(t *testing.T) {
	config := project.New(t.TempDir(), "typescript")
	path, err := SyncWithDependencies(config, map[string]string{
		"@types/react": "latest",
		"react":        "latest",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Dependencies["react"] != "latest" || manifest.DevDependencies["@types/react"] != "latest" {
		t.Fatalf("unexpected package.json: %s", data)
	}
	if _, exists := manifest.Dependencies["@types/react"]; exists {
		t.Fatalf("type declarations were emitted as a runtime dependency: %s", data)
	}

	config.DevDependencies["@types/react"] = "^18.0.0"
	if _, err := SyncWithDependencies(config, map[string]string{"@types/react": "latest"}); err == nil || !strings.Contains(err.Error(), "requires latest") {
		t.Fatalf("expected package-owned type dependency conflict, got %v", err)
	}
}

func TestSyncBunPackage(t *testing.T) {
	config := project.New(t.TempDir(), "typescript")
	config.TypeScript.Runtime = project.TypeScriptRuntimeBun
	config.TypeScript.PackageManager = "bun"
	path, err := Sync(config)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		PackageManager string `json:"packageManager"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.PackageManager != "bun" {
		t.Fatalf("unexpected Bun package.json: %s", data)
	}
}

func TestInstallCommandUsesConfiguredTypeScriptPackageManager(t *testing.T) {
	for _, name := range []string{"bun", "npm"} {
		t.Run(name, func(t *testing.T) {
			config := project.New(t.TempDir(), "typescript")
			config.TypeScript.PackageManager = name
			command, err := installCommand(config)
			if err != nil {
				t.Fatal(err)
			}
			if len(command.Args) != 2 || command.Args[0] != name || command.Args[1] != "install" {
				t.Fatalf("unexpected install command: %#v", command.Args)
			}
		})
	}
}

func TestInstallCommandRunsBundlerThroughRuby(t *testing.T) {
	command, err := installCommand(project.New(t.TempDir(), "ruby"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ruby", "-e", `load Gem.bin_path("bundler", "bundle")`, "--", "install"}
	if strings.Join(command.Args, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("unexpected Ruby install command: want %#v, got %#v", want, command.Args)
	}
}

func TestExternalPackageManagementDoesNotWriteManifest(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.PackageManagement = project.ExternalPackages
	if _, err := Sync(config); err == nil || !strings.Contains(err.Error(), "package management is external") {
		t.Fatalf("expected external package management error, got %v", err)
	}
	for _, name := range []string{"Gemfile", ".ruby-version"} {
		if _, err := os.Stat(filepath.Join(root, name)); !os.IsNotExist(err) {
			t.Fatalf("external package management wrote %s: %v", name, err)
		}
	}
}
