package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
)

const recordDefaultsSource = `record Window
	start: Integer
	finish: Integer = start + 10
	tags: Array<String> = []
end

def sample(): Window
	return Window.new(start: 2)
end
`

func TestRecordFieldDefaultsAreFirstClassAcrossPortableBackends(t *testing.T) {
	expected := map[string][]string{
		"go": {
			"func TrbRecordNewWindow(start int, finish int, trbFinishProvided bool, tags []string, trbTagsProvided bool) Window",
			"finish = trbIntegerAdd_",
			"tags = []string{}",
			"return TrbRecordNewWindow(trbRecordArg1, *new(int), false, *new([]string), false)",
		},
		"ruby": {
			"Window = Data.define(:start, :finish, :tags) do",
			"finish = __trb_integer_add(start, 10) unless trb_finish_provided",
			"tags = [] unless trb_tags_provided",
			"Window.__trb_record_new(__trb_record_arg_1, nil, false, nil, false)",
		},
		"typescript": {
			"export function __trbRecordNewWindow(args: { start: number; finish?: number; tags?: Array<string> }): Window",
			"const finish = Object.prototype.hasOwnProperty.call(args, \"finish\") ? args.finish as number : __trbIntegerAdd(start, 10);",
			"const tags = Object.prototype.hasOwnProperty.call(args, \"tags\") ? args.tags as Array<string> : [];",
			"return __trbRecordNewWindow({ start: 2 });",
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("window.trb", []byte(recordDefaultsSource), mode)
			if err != nil {
				t.Fatal(err)
			}
			record, ok := artifact.AST.Statements[0].(*ast.RecordStatement)
			if !ok {
				t.Fatalf("expected record AST, got %#v", artifact.AST.Statements[0])
			}
			if field := record.Body[2].(*ast.RecordFieldStatement); field.Default == nil {
				t.Fatal("record field default was not preserved in the AST")
			}
			lowered, ok := artifact.IR.Statements[0].(*ir.Record)
			if !ok {
				t.Fatalf("expected record IR, got %#v", artifact.IR.Statements[0])
			}
			if field := lowered.Body[2].(*ir.RecordField); field.Default == nil {
				t.Fatal("record field default was not preserved in the IR")
			}
			output := string(artifact.Output)
			for _, fragment := range expected[mode] {
				if !strings.Contains(output, fragment) {
					t.Fatalf("generated %s is missing %q:\n%s", mode, fragment, output)
				}
			}
		})
	}
}

func TestRecordFieldDefaultDiagnostics(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "wrong type",
			source: "record Config\n\tport: Integer = \"8080\"\nend\n",
			want:   "record field default has type String, expected Integer",
		},
		{
			name:   "required after default",
			source: "record Config\n\tport: Integer = 8080\n\thost: String\nend\n",
			want:   "required record field cannot follow a default field",
		},
		{
			name:   "later field reference",
			source: "record Range\n\tfinish: Integer = start + 1\n\tstart: Integer = 0\nend\n",
			want:   "operator + does not support Any and Integer",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("config.trb", []byte(test.source), "go")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestImportedRecordDefaultsStayOwnedByTheDefiningModule(t *testing.T) {
	model := SourceUnit{
		Filename: "config.trb", ModulePath: "models/config",
		Source: []byte(`def default_port(): Integer
	return 8080
end

record Config
	host: String
	port: Integer = default_port()
end
`),
	}
	main := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import { Config } from models/config

def sample(): Config
	return Config.new(host: "localhost")
end
`),
	}
	expected := map[string][]string{
		"go":         {"models.TrbRecordNewConfig("},
		"ruby":       {"Config.__trb_record_new("},
		"typescript": {`import { __trbRecordNewConfig } from "./models/config.ts";`, `return __trbRecordNewConfig({ host: "localhost" });`},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
				Mode: mode, GoModule: "example.com/defaults", RubyLoader: "require_relative", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			output := string(artifactForModule(artifacts, "main").Output)
			for _, fragment := range expected[mode] {
				if !strings.Contains(output, fragment) {
					t.Fatalf("generated %s is missing %q:\n%s", mode, fragment, output)
				}
			}
		})
	}
}

func TestEnumMembersPreservePostfixAttributes(t *testing.T) {
	source := []byte(`enum Command
	Serve(port: Integer) @cli(about: "Start the server")
	Version @cli(about: "Print the version")
end
`)
	artifact, err := Compile("command.trb", source, "go")
	if err != nil {
		t.Fatal(err)
	}
	parsed := artifact.AST.Statements[0].(*ast.EnumStatement)
	serve := parsed.Body[0].(*ast.EnumMemberStatement)
	if len(serve.Attributes) != 1 || serve.Attributes[0].Name != "cli" || len(serve.Attributes[0].Arguments) != 1 {
		t.Fatalf("unexpected enum member attributes in AST: %#v", serve.Attributes)
	}
	lowered := artifact.IR.Statements[0].(*ir.Enum)
	version := lowered.Body[1].(*ir.EnumMember)
	if len(version.Attributes) != 1 || version.Attributes[0].Name != "cli" {
		t.Fatalf("unexpected enum member attributes in IR: %#v", version.Attributes)
	}
}

func TestRecordDefaultsPreserveAttributesAfterLessThanExpressions(t *testing.T) {
	artifact, err := Compile("filter.trb", []byte(`record Filter
	limit: Integer
	below_limit: Boolean = limit < 10 @cli(:option)
end
`), "go")
	if err != nil {
		t.Fatal(err)
	}
	record := artifact.AST.Statements[0].(*ast.RecordStatement)
	field := record.Body[1].(*ast.RecordFieldStatement)
	if field.Default == nil || len(field.Attributes) != 1 || field.Attributes[0].Name != "cli" {
		t.Fatalf("unexpected default or attributes: default=%#v attributes=%#v", field.Default, field.Attributes)
	}
}

func TestSuspendingTypeScriptRecordDefaultsPropagateThroughConstructors(t *testing.T) {
	model := SourceUnit{Filename: "request_config.trb", ModulePath: "models/request_config", Source: []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import { Result } from trb/std/result

record RequestConfig
	client: HttpClient
	response: Result<Response<Body>, RequestError> = client.request("/health")
end
`)}
	main := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { HttpClient } from trb/platform/typescript/browser
import { RequestConfig } from models/request_config

def build(client: HttpClient): RequestConfig
	return RequestConfig.new(client: client)
end
`)}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "models/request_config").Output) + string(artifactForModule(artifacts, "main").Output)
	for _, fragment := range []string{
		"export async function __trbRecordNewRequestConfig(__trbScope: AbortSignal | undefined, args:",
		"): Promise<RequestConfig>",
		"export async function build(__trbScope: AbortSignal | undefined, client:",
		"return (await __trbRecordNewRequestConfig(__trbScope, { client: client }));",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", fragment, output)
		}
	}
}

func TestTopLevelSuspendingTypeScriptRecordDefaultsUseRootExecutionScope(t *testing.T) {
	artifact, err := Compile("main.trb", []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import { Result } from trb/std/result

record RequestConfig
	client: HttpClient
	response: Result<Response<Body>, RequestError> = client.request("/health")
end

CLIENT := HttpClient.new("https://example.test")
CONFIG := RequestConfig.new(client: CLIENT)

def main()
	return
end
`), "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if !strings.Contains(output, "__trbRecordNewRequestConfig(undefined, { client: CLIENT })") {
		t.Fatalf("top-level record construction is missing its root execution scope:\n%s", output)
	}
}

func TestSuspendingTypeScriptParameterDefaultsLowerIntoTheFunctionBody(t *testing.T) {
	artifact, err := Compile("main.trb", []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import { Result } from trb/std/result

record RequestConfig
	client: HttpClient
	response: Result<Response<Body>, RequestError> = client.request("/health")
end

CLIENT := HttpClient.new("https://example.test")

def load(config: RequestConfig = RequestConfig.new(client: CLIENT)): RequestConfig
	return config
end

CONFIG := load()

def main()
	return
end
`), "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	for _, fragment := range []string{
		"export async function load(__trbScope: AbortSignal | undefined, __trbOptional: unknown[]): Promise<RequestConfig>",
		"config = (await __trbRecordNewRequestConfig(__trbScope, { client: CLIENT }));",
		"export const CONFIG: RequestConfig = (await load(undefined, []));",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated TypeScript is missing %q:\n%s", fragment, output)
		}
	}
}

func TestImportedSuspendingTypeScriptParameterDefaultsUseTheLoweredCallABI(t *testing.T) {
	model := SourceUnit{Filename: "request_config.trb", ModulePath: "models/request_config", Source: []byte(`import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import { Result } from trb/std/result

record RequestConfig
	client: HttpClient
	response: Result<Response<Body>, RequestError> = client.request("/health")
end

CLIENT := HttpClient.new("https://example.test")

def load(config: RequestConfig = RequestConfig.new(client: CLIENT)): RequestConfig
	return config
end
`)}
	main := SourceUnit{Filename: "main.trb", ModulePath: "main", Source: []byte(`import { load } from models/request_config

CONFIG := load()
`)}
	artifacts, err := CompileProject([]SourceUnit{model, main}, Options{
		Mode: "typescript", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	modelOutput := string(artifactForModule(artifacts, "models/request_config").Output)
	mainOutput := string(artifactForModule(artifacts, "main").Output)
	if !strings.Contains(modelOutput, "export async function load(__trbScope: AbortSignal | undefined, __trbOptional: unknown[])") {
		t.Fatalf("defining module did not lower the suspending default:\n%s", modelOutput)
	}
	if !strings.Contains(mainOutput, "export const CONFIG: RequestConfig = (await load(undefined, []));") {
		t.Fatalf("importing module did not use the lowered call ABI:\n%s", mainOutput)
	}
}

func TestTypeScriptRejectsSuspendingDeclarationInitializersWithoutAsyncBoundaries(t *testing.T) {
	prelude := `import { HttpClient, RequestError, Response } from trb/platform/typescript/browser
import { Body } from trb/http
import { Result } from trb/std/result

record RequestConfig
	client: HttpClient
	response: Result<Response<Body>, RequestError> = client.request("/health")
end

CLIENT := HttpClient.new("https://example.test")
`
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "module constant",
			source: `module Settings
	CONFIG := RequestConfig.new(client: CLIENT)
end
`,
			want: "TypeScript module constant Settings::CONFIG cannot use an operation that may suspend",
		},
		{
			name: "nested module constant",
			source: `module Outer
	module Settings
		CONFIG := RequestConfig.new(client: CLIENT)
	end
end
`,
			want: "TypeScript module constant Outer::Settings::CONFIG cannot use an operation that may suspend",
		},
		{
			name: "class field",
			source: `class Holder
	@config: RequestConfig := RequestConfig.new(client: CLIENT)

	def initialize()
		return
	end
end
`,
			want: "TypeScript class field Holder#config cannot use an operation that may suspend",
		},
		{
			name: "class initializer parameter",
			source: `class Holder
	def initialize(config: RequestConfig = RequestConfig.new(client: CLIENT))
		return
	end
end
`,
			want: "TypeScript class initializer Holder#initialize cannot use an operation that may suspend",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := Compile("main.trb", []byte(prelude+test.source), "typescript")
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Compile() error=%v, want %q", err, test.want)
			}
		})
	}
}
