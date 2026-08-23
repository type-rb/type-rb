package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/packageextension"
	packageManager "github.com/type-rb/type-rb/internal/packages"
	"github.com/type-rb/type-rb/internal/project"
)

func TestBuildUsesInstalledNativeTypeScriptPackageIndex(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.PackageManagement = project.ExternalPackages
	config.TypeScript.Runtime = project.TypeScriptRuntimeBrowser
	config.Dependencies["react-spinners"] = "^0.17.0"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { ReactNode } from trb/platform/typescript/react
import { ClipLoader } from "react-spinners"

def Loading(): ReactNode
	return <ClipLoader color="#4f46e5" loading size={24} />
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "loading.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := nativepackage.Write(root, cliNativeComponentCatalog()); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	generated, err := os.ReadFile(filepath.Join(config.OutputPath(), "loading.tsx"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`import { ClipLoader } from "react-spinners";`,
		`return <ClipLoader color={"#4f46e5"} loading size={24} />;`,
	} {
		if !strings.Contains(string(generated), expected) {
			t.Fatalf("generated native component TSX is missing %q:\n%s", expected, generated)
		}
	}
}

func TestBuildNarrowsNullableAliasThroughNativeDiscriminatedUnion(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.OutDir = "build"
	config.PackageManagement = project.ExternalPackages
	config.TypeScript.Runtime = project.TypeScriptRuntimeBrowser
	config.Dependencies["query"] = "1.0.0"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { QueryResult } from "query"

alias MemberId = Integer

record UserView
	member_id: MemberId?
end

def member_id_text(query: QueryResult<UserView>): String
	case query.status
	when "pending"
		return ""
	when "success"
		if query.data.member_id == nil
			return ""
		end
		return query.data.member_id.to_s()
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	typeParameter := nativepackage.Type{Kind: "named", Name: "TData"}
	pending := nativepackage.Type{Kind: "named", Name: "QueryPending", Args: []nativepackage.Type{typeParameter}}
	success := nativepackage.Type{Kind: "named", Name: "QuerySuccess", Args: []nativepackage.Type{typeParameter}}
	aliasTarget := nativepackage.Type{Kind: "union", Name: "Union", Args: []nativepackage.Type{pending, success}}
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"query": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"query": {
				Exports: map[string]nativepackage.Export{
					"QueryResult": {Kind: "type_alias", Type: nativepackage.Type{Kind: "named", Name: "QueryResult", Args: []nativepackage.Type{typeParameter}}, AliasTarget: &aliasTarget, TypeParameters: []string{"TData"}},
				},
				Records: map[string]nativepackage.Export{
					"QueryPending": {Kind: "record", Type: pending, TypeParameters: []string{"TData"}, Fields: []nativepackage.Field{{Name: "status", Type: nativepackage.Type{Kind: "string_literal", Name: `"pending"`}}}},
					"QuerySuccess": {Kind: "record", Type: success, TypeParameters: []string{"TData"}, Fields: []nativepackage.Field{{Name: "status", Type: nativepackage.Type{Kind: "string_literal", Name: `"success"`}}, {Name: "data", Type: typeParameter}}},
				},
			},
		},
	}
	if err := nativepackage.Write(root, catalog); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	generated, err := os.ReadFile(filepath.Join(config.OutputPath(), "main.ts"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(generated), "String(query.data.member_id)") {
		t.Fatalf("narrowed alias did not use the Integer receiver intrinsic:\n%s", generated)
	}
}

func TestBuildRejectsStaleNativeTypeScriptPackageIndex(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.PackageManagement = project.ExternalPackages
	config.Dependencies["react-spinners"] = "^0.18.0"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("import { ClipLoader } from react-spinners\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := nativepackage.Write(root, cliNativeComponentCatalog()); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status == 0 {
		t.Fatal("stale native package index unexpectedly compiled")
	}
	if !strings.Contains(stderr.String(), "native TypeScript package types are stale; run trb install") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestInstallAppliesTypeRBPackageDeclarationAdapter(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.TypeScript.PackageManager = "npm"
	config.Dependencies["ui"] = "1.0.0"
	adapterPath := filepath.Join(root, "declarations.json")
	adapter := packageextension.DeclarationAdapterCatalog{
		ProtocolVersion: packageextension.DeclarationAdapterProtocolVersion,
		Modules: map[string]packageextension.DeclarationAdapterModule{
			"ui": {Exports: map[string]packageextension.DeclarationAdapterExport{
				"Button": {Kind: "component", Type: packageextension.DeclarationAdapterType{Kind: "named", Name: "ReactNode"}},
				"identity": {
					Kind: "function", Type: packageextension.DeclarationAdapterType{Kind: "named", Name: "T"}, Parameters: []packageextension.DeclarationAdapterType{{Kind: "named", Name: "T"}}, Required: 1, TypeParameters: []string{"T"},
				},
			}},
		},
	}
	adapterData, err := json.Marshal(adapter)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(adapterPath, append(adapterData, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	indexerOutput := `{"typescriptVersion":"6.0.3","modules":{"ui":{"exports":{},"unsupported":{"Button":"uses a conditional type","identity":"uses generic call signatures"}}}}`
	node := filepath.Join(bin, "node")
	if err := os.WriteFile(node, []byte("#!/bin/sh\nprintf '%s' '"+indexerOutput+"'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	resolved := &packageManager.TypeRBPackages{DeclarationAdapters: []packageManager.DeclarationAdapter{{
		Package: "github.com/acme/ui-types", Mode: "typescript", Path: adapterPath, Dependencies: map[string]string{"ui": "1.0.0"},
	}}}
	var stdout bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &bytes.Buffer{}}
	if err := command.indexNativeTypeScriptPackages(config, resolved); err != nil {
		t.Fatal(err)
	}
	loaded, err := nativepackage.Load(root, map[string]string{"ui": "1.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Modules["ui"].Exports["Button"].Kind != "component" {
		t.Fatalf("declaration adapter correction is missing: %#v", loaded.Modules["ui"])
	}
	if parameters := loaded.Modules["ui"].Exports["identity"].TypeParameters; len(parameters) != 1 || parameters[0] != "T" {
		t.Fatalf("generic declaration adapter correction is missing: %#v", loaded.Modules["ui"])
	}
	if !strings.Contains(stdout.String(), "indexed 1 native TypeScript module(s) from 1 package(s)") {
		t.Fatalf("unexpected install output: %s", stdout.String())
	}
}

func TestNativeTypeScriptModulesIncludeImportedDependencySubpaths(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.SourceDir = "src"
	config.Dependencies["@acme/ui"] = "1.0.0"
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "import { Button } from \"@acme/ui/components\"\n"
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	modules, err := nativeTypeScriptModules(config, &packageManager.TypeRBPackages{}, config.Dependencies)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(modules, ",") != "@acme/ui,@acme/ui/components" {
		t.Fatalf("unexpected native modules: %#v", modules)
	}
}

func cliNativeComponentCatalog() *nativepackage.Catalog {
	propsName := "Native_react_spinners_ClipLoaderProps"
	return &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"react-spinners": "^0.17.0"},
		Modules: map[string]nativepackage.Module{
			"react-spinners": {
				Exports: map[string]nativepackage.Export{
					"ClipLoader": {
						Kind:       "component",
						Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
						Parameters: []nativepackage.Type{{Kind: "named", Name: propsName}},
						Required:   1,
					},
				},
				Records: map[string]nativepackage.Export{
					propsName: {
						Kind: "record",
						Type: nativepackage.Type{Kind: "named", Name: propsName},
						Fields: []nativepackage.Field{
							{Name: "color", Type: nativepackage.Type{Kind: "union", Name: "Union", Args: []nativepackage.Type{{Kind: "string_literal", Name: `"#4f46e5"`}, {Kind: "string_literal", Name: `"#ffffff"`}}, Nullable: true}, Optional: true},
							{Name: "loading", Type: nativepackage.Type{Kind: "bool", Name: "Boolean", Nullable: true}, Optional: true},
							{Name: "size", Type: nativepackage.Type{Kind: "union", Name: "Union", Args: []nativepackage.Type{{Kind: "string", Name: "String"}, {Kind: "float", Name: "Float"}}, Nullable: true}, Optional: true},
						},
					},
				},
			},
		},
	}
}
