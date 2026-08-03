package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadJSONCWithCommentsAndTrailingCommas(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  // One mode is selected for the whole project.
  "name": "web-app",
  "mode": "ruby",
  "sourceDir": "app",
  "dependencies": {
    "rails": "~> 8.0",
  },
  "ruby": {
    /* URLs containing // are strings, not comments. */
    "source": "https://rubygems.org",
  },
}
`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.Mode != "ruby" || config.Dependencies["rails"] != "~> 8.0" {
		t.Fatalf("unexpected config: %#v", config)
	}
	if config.Ruby == nil || config.Ruby.Source != "https://rubygems.org" {
		t.Fatalf("unexpected Ruby config: %#v", config.Ruby)
	}
}

func TestSaveAndFindConfig(t *testing.T) {
	root := t.TempDir()
	config := New(root, "go")
	config.Go.Module = "example.com/acme/app"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(config.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "// TypeRB project configuration") {
		t.Fatalf("saved config is not JSONC:\n%s", data)
	}

	nested := filepath.Join(root, "src", "commands")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	found, err := Find(nested)
	if err != nil {
		t.Fatal(err)
	}
	if found.Root != root || found.Go == nil || found.Go.Module != "example.com/acme/app" {
		t.Fatalf("unexpected discovered config: %#v", found)
	}
}

func TestExternalPackageManagement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "existing-rails-app",
  "mode": "ruby",
  "packageManagement": "external"
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.ManagesPackages() {
		t.Fatal("external project must not be treated as a trb-managed package project")
	}
}
