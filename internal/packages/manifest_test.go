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
	for _, want := range []string{`source "https://rubygems.org"`, `gem "rails", "~> 8.0"`, `gem "rspec-rails", "~> 7.0"`} {
		if !strings.Contains(text, want) {
			t.Fatalf("Gemfile does not contain %q:\n%s", want, text)
		}
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

func TestSyncNpmPackage(t *testing.T) {
	config := project.New(t.TempDir(), "typescript")
	config.Dependencies["zod"] = "^4.0.0"
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
		Type         string            `json:"type"`
		Dependencies map[string]string `json:"dependencies"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Type != "module" || manifest.Dependencies["zod"] != "^4.0.0" {
		t.Fatalf("unexpected package.json: %s", data)
	}
}

func TestExternalPackageManagementDoesNotWriteManifest(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.PackageManagement = project.ExternalPackages
	if _, err := Sync(config); err == nil || !strings.Contains(err.Error(), "package management is external") {
		t.Fatalf("expected external package management error, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "Gemfile")); !os.IsNotExist(err) {
		t.Fatalf("external package management wrote Gemfile: %v", err)
	}
}
