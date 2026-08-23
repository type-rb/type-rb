package compiler

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/declarationadapterhost"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/nativepackage"
	packageManager "github.com/type-rb/type-rb/internal/packages"
)

func nativeComponentCatalog() *nativepackage.Catalog {
	propsName := "Native_react_spinners_ClipLoaderProps"
	tablePropsName := "Native_ui_TableProps"
	childrenPropsName := "Native_ui_ChildrenProps"
	labelPropsName := "Native_ui_LabelProps"
	return &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"react-spinners": "^0.17.0", "ui": "1.0.0"},
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
			"ui": {
				Exports: map[string]nativepackage.Export{
					"Label": {
						Kind:       "component",
						Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
						Parameters: []nativepackage.Type{{Kind: "named", Name: labelPropsName}},
						Required:   1,
					},
					"Table": {
						Kind:       "component",
						Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
						Parameters: []nativepackage.Type{{Kind: "named", Name: tablePropsName}},
						Required:   1,
						Members: map[string]nativepackage.Export{
							"Row": {
								Kind:       "component",
								Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
								Parameters: []nativepackage.Type{{Kind: "named", Name: childrenPropsName}},
								Required:   1,
							},
							"Cell": {
								Kind:       "component",
								Type:       nativepackage.Type{Kind: "named", Name: "ReactNode"},
								Parameters: []nativepackage.Type{{Kind: "named", Name: childrenPropsName}},
								Required:   1,
							},
						},
					},
				},
				Records: map[string]nativepackage.Export{
					tablePropsName: {
						Kind:   "record",
						Type:   nativepackage.Type{Kind: "named", Name: tablePropsName},
						Fields: []nativepackage.Field{{Name: "children", Type: nativepackage.Type{Kind: "named", Name: "ReactNode"}}},
					},
					childrenPropsName: {
						Kind:   "record",
						Type:   nativepackage.Type{Kind: "named", Name: childrenPropsName},
						Fields: []nativepackage.Field{{Name: "children", Type: nativepackage.Type{Kind: "named", Name: "ReactNode"}}},
					},
					labelPropsName: {
						Kind:   "record",
						Type:   nativepackage.Type{Kind: "named", Name: labelPropsName},
						Fields: []nativepackage.Field{{Name: "label", Type: nativepackage.Type{Kind: "named", Name: "ReactNode"}}},
					},
				},
			},
		},
	}
}

func TestCompileTypeScriptAcceptsRenderableValuesForNativeReactNodeProps(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { Label } from ui

def Page(): ReactNode
	return <div>
		<Label label="Name" />
		<Label label={42} />
	</div>
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "page.trb", ModulePath: "app/page", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeComponentCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "page.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{`<Label label={"Name"} />`, `<Label label={42} />`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated native component TSX is missing %q:\n%s", expected, output)
		}
	}
}

func tanStackQueryAdapterExampleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "examples", "adapters", "tanstack-query"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func tanStackQueryAdapterCatalog(t *testing.T) *nativepackage.Catalog {
	t.Helper()
	root := tanStackQueryAdapterExampleRoot(t)
	manifest, err := packageManager.ReadTypeRBManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := manifest.NativeDependenciesFor("typescript")
	catalog := nativepackage.Empty(dependencies)
	if err := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: manifest.Name, Mode: "typescript",
		Path: filepath.Join(root, manifest.DeclarationAdapterFor("typescript")), Dependencies: dependencies,
	}}); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func tanStackRouterAdapterExampleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "examples", "adapters", "tanstack-router"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func tanStackRouterAdapterCatalog(t *testing.T) *nativepackage.Catalog {
	t.Helper()
	root := tanStackRouterAdapterExampleRoot(t)
	manifest, err := packageManager.ReadTypeRBManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := manifest.NativeDependenciesFor("typescript")
	catalog := nativepackage.Empty(dependencies)
	if err := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: manifest.Name, Mode: "typescript",
		Path: filepath.Join(root, manifest.DeclarationAdapterFor("typescript")), Dependencies: dependencies,
	}}); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func auth0ReactAdapterExampleRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "examples", "adapters", "auth0-react"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func auth0ReactAdapterCatalog(t *testing.T) *nativepackage.Catalog {
	t.Helper()
	root := auth0ReactAdapterExampleRoot(t)
	manifest, err := packageManager.ReadTypeRBManifest(root)
	if err != nil {
		t.Fatal(err)
	}
	dependencies := manifest.NativeDependenciesFor("typescript")
	catalog := nativepackage.Empty(dependencies)
	if err := nativepackage.ApplyDeclarationAdapterFiles(catalog, []declarationadapterhost.Source{{
		Package: manifest.Name, Mode: "typescript",
		Path: filepath.Join(root, manifest.DeclarationAdapterFor("typescript")), Dependencies: dependencies,
	}}); err != nil {
		t.Fatal(err)
	}
	return catalog
}

func nativeGenericQueryCatalog(t *testing.T) *nativepackage.Catalog {
	t.Helper()
	catalog := tanStackQueryAdapterCatalog(t)
	tData := nativepackage.Type{Kind: "named", Name: "TData"}
	tError := nativepackage.Type{Kind: "named", Name: "TError"}
	queryCallback := nativepackage.Type{
		Kind: "function", Name: "Function", Args: []nativepackage.Type{tData},
		ResultBridge: &nativepackage.ResultBridge{Kind: "result_to_promise_rejection", Error: tError},
	}
	module := catalog.Modules["@tanstack/react-query"]
	module.Exports["runQuery"] = nativepackage.Export{
		Kind: "function", Type: tData, Parameters: []nativepackage.Type{queryCallback}, Required: 1,
		TypeParameters: []string{"TData", "TError"},
	}
	module.Exports["runVoid"] = nativepackage.Export{
		Kind: "function", Type: nativepackage.Type{Kind: "void", Name: "Void"},
		Parameters: []nativepackage.Type{{
			Kind: "function", Name: "Function", Args: []nativepackage.Type{{Kind: "void", Name: "Void"}},
			ResultBridge: &nativepackage.ResultBridge{Kind: "result_to_promise_rejection", Error: tError},
		}},
		Required: 1, TypeParameters: []string{"TError"},
	}
	catalog.Modules["@tanstack/react-query"] = module
	return catalog
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

func TestCompileTypeScriptBridgesDOMFileFromNativeComponent(t *testing.T) {
	propsName := "Native_ui_FileUploadProps"
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"ui": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"ui": {
				Exports: map[string]nativepackage.Export{
					"FileUpload": {
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
						Fields: []nativepackage.Field{{
							Name: "onFileSelect",
							Type: nativepackage.Type{Kind: "function", Name: "Function", Args: []nativepackage.Type{
								{Kind: "named", Name: "File"},
								{Kind: "void", Name: "Void"},
							}},
						}},
					},
				},
			},
		},
	}
	source := []byte(`import { File } from trb/platform/typescript/browser
import { ReactNode } from trb/platform/typescript/react
import { FileUpload } from ui

def Upload(): ReactNode
	on_select := fn(file: File)
		puts(file.name)
		return
	end
	return <FileUpload onFileSelect={on_select} />
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "upload.trb", ModulePath: "app/upload", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: catalog,
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "upload.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { FileUpload } from "ui";`,
		`const on_select: (arg0: __trb_browser.File) => void`,
		`return <FileUpload onFileSelect={on_select} />;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated DOM File bridge is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptUsesIndexedCompoundNativeComponent(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { Table } from ui

def Results(): ReactNode
	return <Table><Table.Row><Table.Cell>value</Table.Cell></Table.Row></Table>
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "results.trb", ModulePath: "app/results", Source: source}}, Options{
		Mode:              "typescript",
		TypeScriptRuntime: "browser",
		NativePackages:    nativeComponentCatalog(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "results.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { Table } from "ui";`,
		`return <Table><Table.Row><Table.Cell>value</Table.Cell></Table.Row></Table>;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated compound native component TSX is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptDiagnosesUnknownCompoundNativeComponent(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { Table } from ui

def Results(): ReactNode
	return <Table.Unknown />
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "results.trb", ModulePath: "app/results", Source: source}}, Options{
		Mode:              "typescript",
		TypeScriptRuntime: "browser",
		NativePackages:    nativeComponentCatalog(),
	})
	if err == nil || !strings.Contains(err.Error(), "native component Table has no member Unknown") {
		t.Fatalf("expected unknown compound component diagnostic, got %v", err)
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

func TestCompileTypeScriptDistinguishesNativeClassAndInstanceMembers(t *testing.T) {
	stringType := nativepackage.Type{Kind: "string", Name: "String"}
	clientType := nativepackage.Type{Kind: "named", Name: "Client"}
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"client-library": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"client-library": {Exports: map[string]nativepackage.Export{
				"Client": {
					Kind: "class", Type: clientType,
					InstanceMembers: map[string]nativepackage.Export{
						"run": {Kind: "function", Type: stringType},
					},
					ClassMembers: map[string]nativepackage.Export{
						"create": {Kind: "function", Type: clientType},
					},
				},
			}},
		},
	}
	source := []byte(`import { Client } from client-library

client := Client.new()
puts(client.run())
other := Client.create()
puts(other.run())
`)
	artifact, err := CompileWithOptions("main.trb", source, Options{Mode: "typescript", NativePackages: catalog})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`new Client()`, `client.run()`, `Client.create()`, `other.run()`} {
		if !strings.Contains(string(artifact.Output), expected) {
			t.Fatalf("generated native class access is missing %q:\n%s", expected, artifact.Output)
		}
	}

	_, err = CompileWithOptions("invalid.trb", []byte("import { Client } from client-library\nclient := Client.new()\nclient.create()\n"), Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "class Client has no instance member create; create is a class member") {
		t.Fatalf("expected class/instance member diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptUsesNativeInterfaceInstanceMembers(t *testing.T) {
	catalog := &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"router-library": "1.0.0"},
		Modules: map[string]nativepackage.Module{
			"router-library": {Exports: map[string]nativepackage.Export{
				"AnyRouter": {
					Kind: "interface", Type: nativepackage.Type{Kind: "named", Name: "AnyRouter"},
					Fields: []nativepackage.Field{{Name: "ready", Type: nativepackage.Type{Kind: "bool", Name: "Boolean"}}},
					UnsupportedFields: map[string]string{
						"history": "uses a target-specific history object",
					},
					InstanceMembers: map[string]nativepackage.Export{
						"navigate": {
							Kind: "function", Type: nativepackage.Type{Kind: "void", Name: "Void"},
							Parameters: []nativepackage.Type{{Kind: "string", Name: "String"}}, Required: 1,
						},
					},
				},
				"useRouter": {Kind: "function", Type: nativepackage.Type{Kind: "named", Name: "AnyRouter"}},
			}},
		},
	}
	source := []byte("import { useRouter } from router-library\nrouter := useRouter()\nputs(router.ready.to_s())\nrouter.navigate(\"/todos/42\")\n")
	artifact, err := CompileWithOptions("main.trb", source, Options{Mode: "typescript", NativePackages: catalog})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`import { useRouter } from "router-library";`,
		`import type { AnyRouter } from "router-library";`,
		`const router: AnyRouter = useRouter();`,
		`console.log(String(router.ready));`,
		`router.navigate("/todos/42");`,
	} {
		if !strings.Contains(string(artifact.Output), expected) {
			t.Fatalf("generated native interface access is missing %q:\n%s", expected, artifact.Output)
		}
	}

	_, err = CompileWithOptions("invalid.trb", []byte("import { AnyRouter } from router-library\nrouter := AnyRouter.new()\n"), Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "type AnyRouter imported from router-library has no member new") {
		t.Fatalf("expected non-constructible interface diagnostic, got %v", err)
	}

	_, err = CompileWithOptions("invalid.trb", []byte("import { useRouter } from router-library\nrouter := useRouter()\nputs(router.history)\n"), Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "member history from native type AnyRouter cannot be represented safely: uses a target-specific history object") {
		t.Fatalf("expected unsupported native interface field diagnostic, got %v", err)
	}

	_, err = CompileWithOptions("invalid.trb", []byte("import { useRouter } from router-library\nrouter := useRouter()\nputs(router.missing)\n"), Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "type AnyRouter imported from router-library has no member missing") {
		t.Fatalf("expected unknown inferred native interface member diagnostic, got %v", err)
	}

	_, err = CompileWithOptions("invalid.trb", []byte("import { useRouter } from router-library\nmut router := useRouter()\nrouter.ready = false\n"), Options{Mode: "typescript", NativePackages: catalog})
	if err == nil || !strings.Contains(err.Error(), "field ready is readonly") {
		t.Fatalf("expected readonly native interface field diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptTanStackRouterAdapterExample(t *testing.T) {
	root := tanStackRouterAdapterExampleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "conformance", "src", "app.trb"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := CompileProject([]SourceUnit{{Filename: "app.trb", ModulePath: "app", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: tanStackRouterAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "app.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { Outlet, useRouter } from "@tanstack/react-router";`,
		`import type { NavigateOptions, AnyRouter } from "@tanstack/react-router";`,
		`const router: AnyRouter = useRouter();`,
		`router.navigate(({to: "/todos/42"} satisfies NavigateOptions));`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TanStack Router adapter output is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptAuth0ReactAdapterExample(t *testing.T) {
	root := auth0ReactAdapterExampleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "conformance", "src", "app.trb"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := CompileProject([]SourceUnit{{Filename: "app.trb", ModulePath: "app", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: auth0ReactAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "app.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { Auth0Provider, useAuth0 } from "@auth0/auth0-react";`,
		`import type { AuthorizationParams, Auth0ContextInterface } from "@auth0/auth0-react";`,
		`const auth: Auth0ContextInterface = useAuth0();`,
		`auth.isLoading`,
		`auth.isAuthenticated`,
		`<Auth0Provider domain={"tenant.example.test"} clientId={"client-id"}`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Auth0 adapter output is missing %q:\n%s", expected, output)
		}
	}

	invalid := []byte("import { useAuth0 } from \"@auth0/auth0-react\"\nauth := useAuth0()\nauth.getAccessTokenSilently()\n")
	_, err = CompileWithOptions("invalid.trb", invalid, Options{Mode: "typescript", NativePackages: auth0ReactAdapterCatalog(t)})
	if err == nil || !strings.Contains(err.Error(), "native Promise rejection cannot yet be converted into a checked TypeRB Result") {
		t.Fatalf("expected unsupported native Promise diagnostic, got %v", err)
	}
}

func TestCompileTypeScriptUsesGenericNativeQueryContracts(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
import { Result } from trb/std/result
import {
	QueryClient,
	QueryClientProvider,
	UseQueryOptions,
	queryOptions,
	useQuery,
} from "@tanstack/react-query"

record Todo
	id: Integer
	title: String
end

CLIENT := QueryClient.new()

def Todos(): ReactNode
	query_fn := fn(): Result<Todo, String>
		return Result<Todo, String>::Ok(Todo.new(id: 1, title: "Ship TypeRB"))
	end
	raw_options := UseQueryOptions<Todo, String>.new(
		queryKey: ["todos"],
		queryFn: query_fn,
	)
	options := queryOptions<Todo, String>(raw_options)
	query := useQuery<Todo, String>(options)
	label := case query.status
	when "pending"
		"Loading"
	when "error"
		query.error
	when "success"
		query.data.title
	end
	return <p>{label}</p>
end

def App(): ReactNode
	return <QueryClientProvider client={CLIENT}><Todos /></QueryClientProvider>
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "app.trb", ModulePath: "app", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: tanStackQueryAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	var queryImport *ir.Import
	for _, artifact := range artifacts {
		if artifact.Filename == "app.trb" {
			output = string(artifact.Output)
			for _, statement := range artifact.IR.Statements {
				if imported, ok := statement.(*ir.Import); ok && imported.Path == "@tanstack/react-query" {
					queryImport = imported
				}
			}
		}
	}
	if queryImport == nil || queryImport.TypeContracts["UseQueryResult"].AliasTarget == nil || queryImport.TypeContracts["QueryObserverSuccessResult"].Members["data"].Type.Name != "TData" {
		t.Fatalf("native query import is missing editor type contracts: %#v", queryImport)
	}
	for _, expected := range []string{
		`import { QueryClient, QueryClientProvider, queryOptions, useQuery } from "@tanstack/react-query";`,
		`import type { UseQueryOptions, QueryObserverLoadingErrorResult`,
		`QueryObserverSuccessResult, UseQueryResult } from "@tanstack/react-query";`,
		`const CLIENT: QueryClient = new QueryClient();`,
		`satisfies UseQueryOptions<Todo, string>`,
		`queryOptions<Todo, string>`,
		`useQuery<Todo, string>(options)`,
		`if (__trbResult.kind === "Err") { throw __trbResult.error; }`,
		`return query.data.title;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated generic native package output is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptTanStackQueryAdapterExample(t *testing.T) {
	root := tanStackQueryAdapterExampleRoot(t)
	source, err := os.ReadFile(filepath.Join(root, "conformance", "src", "app.trb"))
	if err != nil {
		t.Fatal(err)
	}
	artifacts, err := CompileProject([]SourceUnit{{Filename: "app.trb", ModulePath: "app", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: tanStackQueryAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "app.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`import { QueryClient, QueryClientProvider, queryOptions, useQuery } from "@tanstack/react-query";`,
		`QueryObserverLoadingResult`,
		`const QUERY_CLIENT: QueryClient = new QueryClient();`,
		`globalThis.fetch`,
		`queryOptions<Todo, __trb_browser.RequestError>`,
		`useQuery<Todo, __trb_browser.RequestError>`,
		`const __trbResult = await __trbCallback()`,
		`throw __trbResult.error`,
		`return query.error.__trb_message;`,
		`return query.data.title;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated TanStack Query example is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptBridgesSuspendingHTTPResultToNativePromiseRejection(t *testing.T) {
	source := []byte(`import { HttpClient, RequestError } from trb/platform/typescript/browser
import { Result } from trb/std/result
import { UseQueryOptions } from "@tanstack/react-query"

record Todo
	id: Integer
	title: String
end

def options(client: HttpClient): UseQueryOptions<Todo, RequestError>
	query_fn := fn(): Result<Todo, RequestError>
		raw := try client.request("/todos/1")
		response := try raw.json<Todo>()
		return Result<Todo, RequestError>::Ok(response.body)
	end
	return UseQueryOptions<Todo, RequestError>.new(
		queryKey: ["todos"],
		queryFn: query_fn,
	)
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "query.trb", ModulePath: "app/query", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: tanStackQueryAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "query.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`const query_fn: () => Promise<Result<Todo, __trb_browser.RequestError>> = async ()`,
		`globalThis.fetch`,
		`const __trbResult = await __trbCallback()`,
		`if (__trbResult.kind === "Err") { throw __trbResult.error; }`,
		`return __trbResult.value;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated native callback bridge is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptBridgesDirectNativeCallbackParameters(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result
import { runQuery } from "@tanstack/react-query"

record LoadError
	message: String
end

def load(): Result<Integer, LoadError>
	return Result<Integer, LoadError>::Ok(7)
end

def main()
	loader := fn(): Result<Integer, LoadError>
		return load()
	end
	puts(runQuery<Integer, LoadError>(loader))
	return
end
`)
	artifact, err := CompileWithOptions("query.trb", source, Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{`runQuery<number, LoadError>`, `const __trbResult = await __trbCallback()`, `throw __trbResult.error`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated direct callback bridge is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptBridgesStandardResultCallbacksAtDirectAndRecordBoundaries(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result
import { UseQueryOptions, runQuery } from "@tanstack/react-query"

record LoadError
	message: String
end

alias LoadResult<T> = Result<T, LoadError>

def main()
	loader := fn(): LoadResult<Integer>
		return LoadResult<Integer>::Ok(7)
	end
	_options := UseQueryOptions<Integer, LoadError>.new(
		queryKey: ["value"],
		queryFn: loader,
	)
	puts(runQuery<Integer, LoadError>(loader))
	return
end
`)
	artifact, err := CompileWithOptions("query.trb", source, Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{
		`(__trbCallback: () => Result<number, LoadError> | Promise<Result<number, LoadError>>)`,
		`runQuery<number, LoadError>`,
		`const __trbResult = await __trbCallback()`,
		`if (__trbResult.kind === "Err") { throw __trbResult.error; }`,
		`return __trbResult.value;`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Result callback bridge is missing %q:\n%s", expected, output)
		}
	}
	if count := strings.Count(output, `const __trbResult = await __trbCallback()`); count != 2 {
		t.Fatalf("expected direct and record callback conversions, got %d:\n%s", count, output)
	}
}

func TestCompileTypeScriptBridgesSuspendingStandardResultCallback(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result
import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { UseQueryOptions } from "@tanstack/react-query"

record Todo
	id: Integer
	title: String
end

def options(client: HttpClient): UseQueryOptions<Response<Todo>, RequestError>
	query_fn := fn(): Result<Response<Todo>, RequestError>
		raw := try client.request("/todos/1")
		return raw.json<Todo>()
	end
	return UseQueryOptions<Response<Todo>, RequestError>.new(
		queryKey: ["todos"],
		queryFn: query_fn,
	)
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "query.trb", ModulePath: "app/query", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: tanStackQueryAdapterCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	var output string
	for _, artifact := range artifacts {
		if artifact.Filename == "query.trb" {
			output = string(artifact.Output)
		}
	}
	for _, expected := range []string{
		`const query_fn: () => Promise<Result<__trb_browser.Response<Todo>, __trb_browser.RequestError>> = async ()`,
		`() => Result<__trb_browser.Response<Todo>, __trb_browser.RequestError> | Promise<Result<__trb_browser.Response<Todo>, __trb_browser.RequestError>>`,
		`globalThis.fetch`,
		`const __trbResult = await __trbCallback()`,
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated suspending Result callback bridge is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptNativeResultBridgeMapsVoidWithoutCollapsingGenericUnit(t *testing.T) {
	source := []byte(`import { Result } from trb/std/result
import { Unit } from trb/std/unit
import { runQuery, runVoid } from "@tanstack/react-query"

record LoadError
	message: String
end

def main()
	work := fn(): Result<Unit, LoadError>
		return Result<Unit, LoadError>::Ok(Unit.new())
	end
	runVoid<LoadError>(work)
	runQuery<Unit, LoadError>(work)
	return
end
`)
	artifact, err := CompileWithOptions("query.trb", source, Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(t)})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, expected := range []string{`): Promise<void>`, `): Promise<Unit>`, `runVoid<LoadError>`, `runQuery<Unit, LoadError>`} {
		if !strings.Contains(output, expected) {
			t.Fatalf("generated Unit/Void Result callback bridge is missing %q:\n%s", expected, output)
		}
	}
}

func TestCompileTypeScriptNativeResultBridgeChecksStandardIdentityAndErrorAssignability(t *testing.T) {
	t.Run("assignable error", func(t *testing.T) {
		source := []byte(`import { Result } from trb/std/result
import { runQuery } from "@tanstack/react-query"

interface ErrorContract
	message(): String
end

class ConcreteError implements ErrorContract
	def message(): String
		return "load failed"
	end
end

def main()
	loader := fn(): Result<Integer, ConcreteError>
		return Result<Integer, ConcreteError>::Err(ConcreteError.new())
	end
	runQuery<Integer, ErrorContract>(loader)
	return
end
`)
		if _, err := CompileWithOptions("query.trb", source, Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(t)}); err != nil {
			t.Fatal(err)
		}
	})

	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "spoofed Result",
			source: `import { runQuery } from "@tanstack/react-query"

enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end

record LoadError
	message: String
end

def main()
	loader := fn(): Result<Integer, LoadError>
		return Result<Integer, LoadError>::Ok(7)
	end
	runQuery<Integer, LoadError>(loader)
	return
end
`,
			want: "requires a callback returning the standard Result<T, E>",
		},
		{
			name: "mismatched error",
			source: `import { Result } from trb/std/result
import { runQuery } from "@tanstack/react-query"

record ExpectedError
	message: String
end

record ActualError
	message: String
end

def main()
	loader := fn(): Result<Integer, ActualError>
		return Result<Integer, ActualError>::Err(ActualError.new(message: "failed"))
	end
	runQuery<Integer, ExpectedError>(loader)
	return
end
`,
			want: "callback error type ActualError is not assignable to ExpectedError",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := CompileWithOptions("query.trb", []byte(test.source), Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(t)})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("expected %q, got %v", test.want, err)
			}
		})
	}
}

func TestGeneratedTypeScriptNativeResultBridgeResolvesOkAndRejectsExactErr(t *testing.T) {
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("bun is not installed")
	}
	source := []byte(`import { Result } from trb/std/result
import { runQuery } from "@tanstack/react-query"

def main()
	ok := fn(): Result<Integer, String>
		return Result<Integer, String>::Ok(7)
	end
	err := fn(): Result<Integer, String>
		return Result<Integer, String>::Err("boom")
	end
	runQuery<Integer, String>(ok)
	runQuery<Integer, String>(err)
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "bun", NativePackages: nativeGenericQueryCatalog(t),
	})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, artifact := range artifacts {
		path := filepath.Join(root, filepath.FromSlash(artifact.IR.ModulePath+".ts"))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, artifact.Output, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	moduleRoot := filepath.Join(root, "node_modules", "@tanstack", "react-query")
	if err := os.MkdirAll(moduleRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	stub := `export function runQuery<T, E>(callback: () => Promise<T>): T {
  void callback().then(
    (value) => console.log("resolved:" + String(value)),
    (error) => console.log("rejected:" + String(error)),
  );
  return 0 as T;
}
`
	if err := os.WriteFile(filepath.Join(moduleRoot, "index.ts"), []byte(stub), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleRoot, "package.json"), []byte(`{"name":"@tanstack/react-query","type":"module","exports":"./index.ts"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bun", "run", "main.ts")
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("generated TypeScript failed: %v\n%s", err, output)
	}
	lines := strings.Fields(string(output))
	if strings.Join(lines, "\n") != "resolved:7\nrejected:boom" {
		t.Fatalf("unexpected native Result bridge output:\n%s", output)
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
