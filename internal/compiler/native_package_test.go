package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativepackage"
)

func nativeComponentCatalog() *nativepackage.Catalog {
	propsName := "Native_react_spinners_ClipLoaderProps"
	tablePropsName := "Native_ui_TableProps"
	childrenPropsName := "Native_ui_ChildrenProps"
	return &nativepackage.Catalog{
		FormatVersion: 1,
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
				},
			},
		},
	}
}

func nativeGenericQueryCatalog() *nativepackage.Catalog {
	typeParameter := func(name string) nativepackage.Type {
		return nativepackage.Type{Kind: "named", Name: name}
	}
	named := func(name string, arguments ...nativepackage.Type) nativepackage.Type {
		return nativepackage.Type{Kind: "named", Name: name, Args: arguments}
	}
	tData := typeParameter("TData")
	tError := typeParameter("TError")
	parameters := []string{"TData", "TError"}
	queryCallback := nativepackage.Type{Kind: "function", Name: "Function", Args: []nativepackage.Type{tData}, Fails: &tError, EffectBridge: "promise_rejection"}
	queryResultTarget := nativepackage.Type{Kind: "union", Name: "Union", Args: []nativepackage.Type{
		named("QueryObserverPendingResult", tData, tError),
		named("QueryObserverLoadingErrorResult", tData, tError),
		named("QueryObserverRefetchErrorResult", tData, tError),
		named("QueryObserverSuccessResult", tData, tError),
		named("QueryObserverPlaceholderResult", tData, tError),
	}}
	resultRecord := func(name, status string, fields ...nativepackage.Field) nativepackage.Export {
		fields = append([]nativepackage.Field{{Name: "status", Type: nativepackage.Type{Kind: "string_literal", Name: `"` + status + `"`}}}, fields...)
		return nativepackage.Export{Kind: "record", Type: named(name), TypeParameters: parameters, Fields: fields}
	}
	queryOptions := nativepackage.Export{
		Kind: "record", Type: named("UseQueryOptions"), TypeParameters: parameters, Fields: []nativepackage.Field{
			{Name: "queryKey", Type: nativepackage.Type{Kind: "array", Name: "Array", Args: []nativepackage.Type{{Kind: "string", Name: "String"}}}},
			{Name: "queryFn", Type: queryCallback},
			{Name: "enabled", Type: nativepackage.Type{Kind: "bool", Name: "Boolean"}, Optional: true},
		},
	}
	return &nativepackage.Catalog{
		FormatVersion: nativepackage.FormatVersion,
		Dependencies:  map[string]string{"@tanstack/react-query": "5.101.2"},
		Modules: map[string]nativepackage.Module{
			"@tanstack/react-query": {
				Exports: map[string]nativepackage.Export{
					"QueryClient": {Kind: "class", Type: named("QueryClient")},
					"QueryClientProvider": {
						Kind: "component", Type: named("ReactNode"), Parameters: []nativepackage.Type{named("QueryClientProviderProps")}, Required: 1,
					},
					"UseQueryResult": {
						Kind: "type_alias", Type: named("UseQueryResult", tData, tError), TypeParameters: parameters, AliasTarget: &queryResultTarget,
					},
					"UseQueryOptions":                 queryOptions,
					"QueryObserverPendingResult":      resultRecord("QueryObserverPendingResult", "pending"),
					"QueryObserverLoadingErrorResult": resultRecord("QueryObserverLoadingErrorResult", "error", nativepackage.Field{Name: "error", Type: tError}),
					"QueryObserverRefetchErrorResult": resultRecord("QueryObserverRefetchErrorResult", "error", nativepackage.Field{Name: "error", Type: tError}),
					"QueryObserverSuccessResult":      resultRecord("QueryObserverSuccessResult", "success", nativepackage.Field{Name: "data", Type: tData}),
					"QueryObserverPlaceholderResult":  resultRecord("QueryObserverPlaceholderResult", "success", nativepackage.Field{Name: "data", Type: tData}),
					"queryOptions": {
						Kind: "function", Type: named("UseQueryOptions", tData, tError), Parameters: []nativepackage.Type{named("UseQueryOptions", tData, tError)}, Required: 1, TypeParameters: parameters,
					},
					"useQuery": {
						Kind: "function", Type: named("UseQueryResult", tData, tError), Parameters: []nativepackage.Type{named("UseQueryOptions", tData, tError)}, Required: 1, TypeParameters: parameters,
					},
					"runQuery": {
						Kind: "function", Type: tData, Parameters: []nativepackage.Type{queryCallback}, Required: 1, TypeParameters: parameters,
					},
				},
				Records: map[string]nativepackage.Export{
					"QueryClientProviderProps": {
						Kind: "record", Type: named("QueryClientProviderProps"), Fields: []nativepackage.Field{
							{Name: "client", Type: named("QueryClient")},
							{Name: "children", Type: named("ReactNode"), Optional: true},
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

func TestCompileTypeScriptUsesGenericNativeQueryContracts(t *testing.T) {
	source := []byte(`import { ReactNode } from trb/platform/typescript/react
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
	query_fn := fn(): Todo
		return Todo.new(id: 1, title: "Ship TypeRB")
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
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(),
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

func TestCompileTypeScriptBridgesSuspendingHTTPResultToNativePromiseRejection(t *testing.T) {
	source := []byte(`import { HttpClient, RequestError } from trb/platform/typescript/browser
import { UseQueryOptions } from "@tanstack/react-query"

record Todo
	id: Integer
	title: String
end

def options(client: HttpClient): UseQueryOptions<Todo, RequestError>
	query_fn := fn(): Todo fails RequestError
		return client.request("/todos/1").json<Todo>().body
	end
	return UseQueryOptions<Todo, RequestError>.new(
		queryKey: ["todos"],
		queryFn: query_fn,
	)
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "query.trb", ModulePath: "app/query", Source: source}}, Options{
		Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog(),
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
	source := []byte(`import { runQuery } from "@tanstack/react-query"

record LoadError
	message: String
end

def load(): Integer fails LoadError
	return 7
end

def main()
	loader := fn(): Integer fails LoadError
		return load()
	end
	puts(runQuery<Integer, LoadError>(loader))
	return
end
`)
	artifact, err := CompileWithOptions("query.trb", source, Options{Mode: "typescript", TypeScriptRuntime: "browser", NativePackages: nativeGenericQueryCatalog()})
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
