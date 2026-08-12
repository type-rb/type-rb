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

func TestLoadTypeRBPackageRequirements(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ConfigName)
	source := `{
  "name": "package-app",
  "mode": "ruby",
  "packages": {
    "acme/contracts": "v1.2.3",
    "private/contracts": {
      "source": "gitlab.example.com/team/contracts",
      "version": "v2.0.0"
    },
    "workspace/ui": {
      "path": "../ui"
    }
  }
}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	config, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.Packages["acme/contracts"].Version; got != "v1.2.3" {
		t.Fatalf("unexpected shorthand requirement: %q", got)
	}
	if got := config.Packages["private/contracts"].Source; got != "gitlab.example.com/team/contracts" {
		t.Fatalf("unexpected explicit source: %q", got)
	}
	if got := config.Packages["workspace/ui"].Path; got != "../ui" {
		t.Fatalf("unexpected path requirement: %q", got)
	}
}

func TestSaveUsesPackageVersionShorthand(t *testing.T) {
	config := New(t.TempDir(), "ruby")
	config.Packages["acme/contracts"] = PackageRequirement{Version: "v1.2.3"}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(config.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"acme/contracts": "v1.2.3"`) {
		t.Fatalf("version shorthand was not preserved:\n%s", data)
	}
}

func TestTypeRBPackageRequirementRejectsMixedPathAndVersion(t *testing.T) {
	config := New(t.TempDir(), "ruby")
	config.Packages["acme/contracts"] = PackageRequirement{Path: "../contracts", Version: "v1.0.0"}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected validation result: %v", err)
	}
}

func TestTypeScriptDependencyCannotAlsoBeATypeRBPackage(t *testing.T) {
	config := New(t.TempDir(), "typescript")
	config.Dependencies["acme/ui"] = "1.0.0"
	config.Packages["acme/ui"] = PackageRequirement{Version: "v1.0.0"}
	if err := config.Validate(); err == nil || !strings.Contains(err.Error(), "both a TypeRB package and a native TypeScript package") {
		t.Fatalf("unexpected validation result: %v", err)
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

func TestDatabaseConfigDefaultsArePortable(t *testing.T) {
	for _, test := range []struct {
		adapter, command string
	}{
		{adapter: "sqlite", command: "sqlite3def"},
		{adapter: "postgresql", command: "psqldef"},
		{adapter: "mysql", command: "mysqldef"},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigName)
			source := `{
  "name": "database-app",
  "mode": "ruby",
  "db": {
    "adapter": "` + test.adapter + `",
    "database": {"environment": "DATABASE_URL"}
  }
}`
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			config, err := Load(path)
			if err != nil {
				t.Fatal(err)
			}
			if config.Database.Schema != "db/schema.sql" || config.Database.Lock != "db/schema.lock.json" {
				t.Fatalf("unexpected schema paths: %#v", config.Database)
			}
			if config.Database.Sqldef.Command != test.command || config.Database.Sqldef.Version != DefaultSqldefVersion {
				t.Fatalf("unexpected sqldef defaults: %#v", config.Database.Sqldef)
			}
		})
	}
}

func TestDatabaseConfigRejectsEscapingPathsAndUnknownAdapters(t *testing.T) {
	for name, database := range map[string]string{
		"adapter": `{"adapter":"oracle"}`,
		"schema":  `{"adapter":"sqlite","schema":"../schema.sql"}`,
		"lock":    `{"adapter":"sqlite","lock":"/tmp/schema.lock.json"}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, ConfigName)
			source := `{"name":"invalid-db","mode":"ruby","db":` + database + `}`
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(path); err == nil {
				t.Fatal("invalid database configuration was accepted")
			}
		})
	}
}
