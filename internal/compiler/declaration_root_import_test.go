package compiler

import (
	"strings"
	"testing"
)

func TestDeclarationRootStandardImportLowersQualifiedMembers(t *testing.T) {
	wants := map[string]string{
		"go":         "math.Sqrt(9.0)",
		"ruby":       "Math.sqrt(value)",
		"typescript": "Math.sqrt(9.0)",
	}
	for mode, want := range wants {
		artifact, err := CompileWithOptions("math.trb", []byte("import trb/std/math\n\ndef value(): Float\n\treturn Math.sqrt(9.0)\nend\n"), Options{Mode: mode, ModulePath: "math"})
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if !strings.Contains(string(artifact.Output), want) {
			t.Fatalf("%s output does not contain %q:\n%s", mode, want, artifact.Output)
		}
	}
}

func TestDeclarationRootEnumImportRetainsTypeScriptRuntimeBinding(t *testing.T) {
	definitions := SourceUnit{
		Filename:   "/project/src/contracts/result.trb",
		ModulePath: "contracts/result",
		Package:    "contracts",
		Source: []byte(`enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import contracts/result

def main()
	result := Result<Integer, String>::Ok(7)
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error)
	end
	return
end
`),
	}
	requireEffectRuntime(t, "typescript")
	artifacts, err := CompileProject([]SourceUnit{definitions, consumer}, Options{
		Mode: "typescript", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil || !strings.Contains(string(main.Output), `import { Result } from "./contracts/result.ts";`) {
		t.Fatalf("declaration-root enum does not retain its runtime binding:\n%s", main.Output)
	}
	if output := strings.TrimSpace(runEffectProject(t, "typescript", artifacts, "")); output != "7" {
		t.Fatalf("declaration-root enum output = %q, want 7", output)
	}
}

func TestDeclarationRootStandardResultImportRetainsTypeScriptRuntimeBinding(t *testing.T) {
	source := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import trb/std/result

def main()
	result := Result<Integer, String>::Ok(7)
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error)
	end
	return
end
`),
	}
	requireEffectRuntime(t, "typescript")
	artifacts, err := CompileProject([]SourceUnit{source}, Options{
		Mode: "typescript", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun",
	})
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.TrimSpace(runEffectProject(t, "typescript", artifacts, "")); output != "7" {
		t.Fatalf("declaration-root standard Result output = %q, want 7", output)
	}
}

func TestNamedImportAliasKeepsOneCanonicalDeclarationIdentity(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{
		{Filename: "web/response.trb", ModulePath: "web/response", Source: []byte("class Response\nend\n")},
		{Filename: "browser/response.trb", ModulePath: "browser/response", Source: []byte("class Response\nend\n")},
		{Filename: "main.trb", ModulePath: "main", Source: []byte("import { Response as WebResponse } from web/response\nimport { Response as BrowserResponse } from browser/response\n\ndef values(): Array<Any>\n\treturn [WebResponse.new(), BrowserResponse.new()]\nend\n")},
	}, Options{Mode: "ruby", SourceRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	main := findArtifactByModule(artifacts, "main")
	if main == nil || !strings.Contains(string(main.Output), "Response.new") {
		t.Fatalf("aliased declarations were not lowered to their exported runtime names: %s", main.Output)
	}
}

func TestActivateEnablesRubyNativeSyntaxWithoutADeclarationBinding(t *testing.T) {
	_, err := CompileWithOptions("native.trb", []byte("activate trb/platform/ruby/native\n\ndef value(): Any\n\treturn native_call 1\nend\n"), Options{Mode: "ruby", ModulePath: "native"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestGeneratedImportDoesNotSatisfyAuthoredUnusedImport(t *testing.T) {
	_, err := CompileProject([]SourceUnit{
		{Filename: "models/response.trb", ModulePath: "models/response", Source: []byte("class Response\nend\n")},
		{
			Filename:   "main.trb",
			ModulePath: "main",
			Source:     []byte("import { Response } from models/response\n"),
			CompilerGeneratedSources: []CompilerGeneratedSource{{
				ID:     "test.generated",
				Source: []byte("import { Response } from models/response\n\ndef generated(): Response\n\treturn Response.new()\nend\n"),
			}},
		},
	}, Options{Mode: "ruby", SourceRoot: "/project"})
	if err == nil || !strings.Contains(err.Error(), "imported symbol Response is not used") {
		t.Fatalf("compile error = %v", err)
	}
}

func TestGeneratedAndAuthoredScopesCanImportTheSameNewtype(t *testing.T) {
	_, err := CompileProject([]SourceUnit{
		{Filename: "contracts/index.trb", ModulePath: "contracts/index", Source: []byte("newtype OrderId = Integer\n")},
		{
			Filename:   "main.trb",
			ModulePath: "main",
			Source:     []byte("import { OrderId } from contracts\n\ndef authored(value: OrderId): OrderId\n\treturn value\nend\n"),
			CompilerGeneratedSources: []CompilerGeneratedSource{
				{ID: "test.imports", Source: []byte("import { OrderId } from contracts/index\n")},
				{ID: "test.generated", Source: []byte("def generated(value: Integer): OrderId\n\treturn OrderId.new(value)\nend\n")},
			},
		},
	}, Options{Mode: "go", GoModule: "example.com/generated-newtype", SourceRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestDeclarationRootImportExposesOwnedNestedTypesAcrossBackends(t *testing.T) {
	definitions := SourceUnit{
		Filename:   "/project/src/contracts/payloads.trb",
		ModulePath: "contracts/payloads",
		Package:    "contracts",
		Source: []byte(`module Payloads
	record Error
		message: String
	end

	def self.failure(message: String): Payloads::Error
		return Payloads::Error.new(message: message)
	end
end
`),
	}
	consumer := SourceUnit{
		Filename:   "/project/src/main.trb",
		ModulePath: "main",
		Package:    "main",
		Source: []byte(`import contracts/payloads as API

def failure(): API::Error
	return API.failure("failed")
end

def main()
	puts(failure().message)
	return
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			requireEffectRuntime(t, mode)
			artifacts, err := CompileProject([]SourceUnit{definitions, consumer}, Options{
				Mode: mode, GoModule: "example.com/nested-root", RubyLoader: "require_relative",
				SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun",
			})
			if err != nil {
				t.Fatal(err)
			}
			main := findArtifactByModule(artifacts, "main")
			if main == nil {
				t.Fatal("main artifact is missing")
			}
			if output := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/nested-root")); output != "failed" {
				t.Fatalf("nested root output = %q, want failed", output)
			}
			if mode == "typescript" {
				checkTypeScriptArtifacts(t, artifacts, "owned_nested_root")
			}
		})
	}
}
