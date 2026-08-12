package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
)

func nativeComponentCatalog() *nativepackage.Catalog {
	propsName := "Native_react_spinners_ClipLoaderProps"
	return &nativepackage.Catalog{
		FormatVersion: 1,
		Dependencies:  map[string]string{"react-spinners": "^0.17.0"},
		Modules: map[string]nativepackage.Module{
			"react-spinners": {
				Exports: map[string]nativepackage.Export{
					"ClipLoader": {
						Kind:       "component",
						Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
						Parameters: []nativepackage.Type{{Kind: "named", Name: propsName}},
						Required:   1,
						UnsupportedFields: map[string]string{
							"cssOverride": "uses unsupported TypeScript type CSSProperties",
						},
					},
				},
				Records: map[string]nativepackage.Export{
					propsName: {
						Kind: "record",
						Type: nativepackage.Type{Kind: "named", Name: propsName},
						Fields: []nativepackage.Field{
							{Name: "color", Type: nativepackage.Type{Kind: "string", Name: "String", Nullable: true}, Optional: true},
							{Name: "loading", Type: nativepackage.Type{Kind: "bool", Name: "Boolean", Nullable: true}, Optional: true},
							{Name: "size", Type: nativepackage.Type{Kind: "union", Name: "Union", Args: []nativepackage.Type{{Kind: "string", Name: "String"}, {Kind: "float", Name: "Float"}}, Nullable: true}, Optional: true},
						},
					},
				},
			},
		},
	}
}

func TestCompileTypeScriptImportsIndexedNativeReactComponent(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { ClipLoader } from "react-spinners"

def Loading(): ReactNode
	return <ClipLoader color="#4f46e5" loading size={24} />
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "loading.trb", ModulePath: "app/loading", Source: source}}, Options{
		Mode:              "typescript",
		TypeScriptRuntime: "browser",
		NativePackages:    nativeComponentCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "loading.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { ClipLoader } from "react-spinners";`,
		`return <ClipLoader color={"#4f46e5"} loading size={24} />;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated native component TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptDiagnosesUnsupportedNativeProp(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { ClipLoader } from react-spinners

def Loading(): ReactNode
	return <ClipLoader cssOverride="display: block" />
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "loading.trb", ModulePath: "app/loading", Source: source}}, Options{
		Mode:              "typescript",
		TypeScriptRuntime: "browser",
		NativePackages:    nativeComponentCatalog(),
	})
	if err == nil || !strings.Contains(err.Error(), "JSX prop cssOverride from native component ClipLoader cannot be represented safely") {
		t.Fatalf("expected unsupported native prop diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptRequiresInstalledNativeTypeIndex(t *testing.T) {
	catalog := nativepackage.Empty(map[string]string{"react-spinners": "^0.17.0"})
	catalog.UnavailableReason = "native TypeScript package types are not indexed; run trb install"
	_, err := CompileProject([]SourceUnit{{
		Filename: "loading.trb", ModulePath: "app/loading", Source: []byte("import { ClipLoader } from react-spinners\n"),
	}}, Options{Mode: "typescript", NativePackages: catalog, AllowUnusedImports: true})
	if err == nil || !strings.Contains(err.Error(), "run trb install") {
		t.Fatalf("expected missing native index diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptCallsIndexedNativeFunction(t *testing.T) {
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"tiny-format": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"tiny-format": {Exports: map[string]nativepackage.Export{
				"format": {
					Kind:       "function",
					Type:       nativepackage.Type{Kind: "string", Name: "String"},
					Parameters: []nativepackage.Type{{Kind: "string", Name: "String"}},
					Required:   1,
				},
			}},
		},
	}
	artifact, err := CompileWithOptions("main.trb", []byte("import { format } from tiny-format\nvalue := format(\"hello\")\nputs(value)\n"), Options{
		Mode: "typescript", NativePackages: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`import { format } from "tiny-format";`, `const value: string = format("hello");`} {
		if !strings.Contains(string(artifact.Output), expected) {
			t.Fatalf("generated native function call is missing %q:\n%s", expected, artifact.Output)
		}
	}
}

func TestCompileTypeScriptRejectsUnsafeNativeExportInsteadOfUsingAny(t *testing.T) {
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"unsafe-package": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"unsafe-package": {Exports: map[string]nativepackage.Export{}, Unsupported: map[string]string{"run": "parameter value uses TypeScript any"}},
		},
	}
	_, err := CompileWithOptions("main.trb", []byte("import { run } from unsafe-package\nrun(1)\n"), Options{
		Mode: "typescript", NativePackages: catalog,
	})
	if err == nil || !strings.Contains(err.Error(), "cannot be represented safely") || !strings.Contains(err.Error(), "TypeScript any") {
		t.Fatalf("expected unsafe native export diagnostic, got %v", err)
	}
}
