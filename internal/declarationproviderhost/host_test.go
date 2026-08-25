package declarationproviderhost

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/packageextension"
)

func TestReadImportsSafeFixedDeclarations(t *testing.T) {
	path := writeCatalog(t, packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
		Types: []packageextension.DeclaredType{{
			Name: "Pagy::Offset",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "page", Kind: "property", Return: packageextension.Type{Kind: "int", Name: "Integer"},
			}},
		}},
		Modules: []packageextension.DeclaredModule{{
			Name: "Pagy::Method",
			InstanceMembers: []packageextension.DeclaredMember{{
				Name: "pagy", Kind: "method", Return: packageextension.Type{Kind: "named", Name: "Pagy::Offset"},
			}},
		}},
	})
	catalog, err := Read(Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := catalog.Types["Pagy::Offset"]; !exists {
		t.Fatal("fixed type declaration was not imported")
	}
	if _, exists := catalog.Modules["Pagy::Method"]; !exists {
		t.Fatal("fixed module declaration was not imported")
	}
}

func TestReadRejectsPrivilegedOrMisownedDeclarations(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*packageextension.DeclarationCatalog)
		want   string
	}{
		{name: "provider", mutate: func(c *packageextension.DeclarationCatalog) { c.Provider = "other/package" }, want: "expected package"},
		{name: "runtime operation", mutate: func(c *packageextension.DeclarationCatalog) {
			c.Types[0].InstanceMembers[0].RuntimeOperation = "trb.internal.secret"
		}, want: "execution hooks"},
		{name: "source module", mutate: func(c *packageextension.DeclarationCatalog) { c.Types[0].SourceModule = "app/models/user" }, want: "project source module"},
		{name: "block", mutate: func(c *packageextension.DeclarationCatalog) {
			c.Types[0].InstanceMembers[0].Block = &packageextension.DeclaredBlock{}
		}, want: "block behavior"},
		{name: "representation", mutate: func(c *packageextension.DeclarationCatalog) {
			c.Types[0].InstanceMembers[0].Return.Representation = &packageextension.Type{Kind: "int", Name: "Integer"}
		}, want: "representation metadata"},
		{name: "any type", mutate: func(c *packageextension.DeclarationCatalog) {
			c.Types[0].InstanceMembers[0].Return = packageextension.Type{Kind: "any", Name: "Any"}
		}, want: "unsafe type kind any"},
		{name: "project rule", mutate: func(c *packageextension.DeclarationCatalog) {
			c.FunctionBlockRules = []packageextension.DeclaredFunctionBlockRule{{Package: c.Provider, Function: "run", TypeArgument: 0}}
		}, want: "project rules"},
		{name: "class-body declaration rule", mutate: func(c *packageextension.DeclarationCatalog) {
			c.ClassBodyDeclarationRules = []packageextension.DeclaredClassBodyDeclarationRule{{
				Package: c.Provider, Function: "run",
				Owner: packageextension.DeclaredReference{ModulePath: "app/job", Name: "ExampleJob"},
			}}
		}, want: "project rules"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			catalog := packageextension.DeclarationCatalog{
				ProtocolVersion: packageextension.DeclarationProtocolVersion,
				Provider:        "github.com/acme/pagy",
				Types: []packageextension.DeclaredType{{Name: "Pagy::Offset", InstanceMembers: []packageextension.DeclaredMember{{
					Name: "page", Kind: "property", Return: packageextension.Type{Kind: "int", Name: "Integer"},
				}}}},
			}
			test.mutate(&catalog)
			path := writeCatalog(t, catalog)
			_, err := Read(Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestReadRejectsAnEmptyFixedCatalog(t *testing.T) {
	path := writeCatalog(t, packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
	})
	_, err := Read(Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path})
	if err == nil || !strings.Contains(err.Error(), "contains no type or module declarations") {
		t.Fatalf("unexpected empty catalog error: %v", err)
	}
}

func TestReadExplainsDeclarationProtocolVersionTwoMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "declarations.json")
	if err := os.WriteFile(path, []byte(`{"protocolVersion":2,"provider":"github.com/acme/pagy"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Read(Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path})
	if err == nil || !strings.Contains(err.Error(), "set protocolVersion to 3") || !strings.Contains(err.Error(), "otherwise unchanged") {
		t.Fatalf("unexpected protocol migration error: %v", err)
	}
}

func TestReadRequiresThePackageRootModule(t *testing.T) {
	path := writeCatalog(t, packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        "github.com/acme/pagy",
	})
	_, err := Read(Source{
		Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy/internal", Path: path,
	})
	if err == nil || !strings.Contains(err.Error(), "must match package") {
		t.Fatalf("unexpected root module error: %v", err)
	}
}

func TestReadRejectsUnknownAndTrailingJSON(t *testing.T) {
	for name, source := range map[string]string{
		"unknown":  `{"protocolVersion":3,"provider":"github.com/acme/pagy","unknown":true}`,
		"trailing": `{"protocolVersion":3,"provider":"github.com/acme/pagy"} {}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "declarations.json")
			if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Read(Source{Package: "github.com/acme/pagy", Mode: "ruby", Module: "github.com/acme/pagy", Path: path}); err == nil {
				t.Fatal("invalid JSON was accepted")
			}
		})
	}
}

func writeCatalog(t *testing.T, catalog packageextension.DeclarationCatalog) string {
	t.Helper()
	data, err := json.Marshal(catalog)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "declarations.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
