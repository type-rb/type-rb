package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindReportsMissingConfiguration(t *testing.T) {
	_, err := Find(t.TempDir())
	if !errors.Is(err, ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got %v", err)
	}
}

func TestLoadJSONCWithComments(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  // One mode is selected for the whole project.
  "name": "web-app",
  "mode": "ruby",
  "sourceDir": "app",
  "dependencies": {
    "rails": "~> 8.0"
  },
  "ruby": {
    /* URLs containing // are strings, not comments. */
    "source": "https://rubygems.org"
  }
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
	if config.Ruby == nil || config.Ruby.Source != "https://rubygems.org" || config.Ruby.Version != DefaultRubyVersion {
		t.Fatalf("unexpected Ruby config: %#v", config.Ruby)
	}
}

func TestLoadPreservesOfficialPackageOptions(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "orm-app",
  "mode": "go",
  "packageOptions": {
    "trb/orm": {
      "adapter": "sqlite",
      "database": "database/application.sqlite3"
    }
  },
  "go": {
    "module": "example.com/orm-app"
  }
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	var options struct {
		Adapter  string `json:"adapter"`
		Database string `json:"database"`
	}
	if err := json.Unmarshal(config.PackageOptions["trb/orm"], &options); err != nil {
		t.Fatal(err)
	}
	if options.Adapter != "sqlite" || options.Database != "database/application.sqlite3" {
		t.Fatalf("unexpected package options: %#v", options)
	}
}

func TestLoadJSONCRejectsTrailingCommas(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := "{\n  \"name\": \"web-app\",\n  \"mode\": \"ruby\",\n}\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("trailing comma must be rejected")
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
	if strings.Contains(string(data), "entrypoint") {
		t.Fatalf("main is conventional and must not be written to config:\n%s", data)
	}
	jsonDocument := data[strings.IndexByte(string(data), '\n')+1:]
	var decoded map[string]any
	if err := json.Unmarshal(jsonDocument, &decoded); err != nil {
		t.Fatalf("saved config must remain strict JSON after its comment header: %v\n%s", err, data)
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

func TestNewTypeScriptProjectUsesLatestCompiler(t *testing.T) {
	config := New(t.TempDir(), "typescript")
	if got := config.DevDependencies["typescript"]; got != DefaultTypeScriptVersion {
		t.Fatalf("unexpected TypeScript version: %q", got)
	}
	if config.TypeScript.Runtime != TypeScriptRuntimeNode || config.TypeScript.PackageManager != "npm" {
		t.Fatalf("unexpected default TypeScript toolchain: %#v", config.TypeScript)
	}
}

func TestLoadLegacyTypeScriptConfigDefaultsToNodeAndNpm(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "legacy-app",
  "mode": "typescript",
  "typescript": {
    "moduleType": "module"
  }
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.TypeScript.Runtime != TypeScriptRuntimeNode || config.TypeScript.PackageManager != "npm" {
		t.Fatalf("unexpected legacy defaults: %#v", config.TypeScript)
	}
}

func TestLoadBunRuntimeDefaultsToBunPackageManager(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "bun-api",
  "mode": "typescript",
  "typescript": {
    "runtime": "bun"
  }
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if config.TypeScript.Runtime != TypeScriptRuntimeBun || config.TypeScript.PackageManager != "bun" {
		t.Fatalf("unexpected Bun defaults: %#v", config.TypeScript)
	}
}

func TestLoadRejectsUnknownTypeScriptRuntime(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "deno-app",
  "mode": "typescript",
  "typescript": {
    "runtime": "deno"
  }
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "typescript.runtime must be browser, bun, or node") {
		t.Fatalf("unexpected invalid-runtime result: %v", err)
	}
}
