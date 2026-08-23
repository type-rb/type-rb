package compiler

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestWebBindCallSpecializationRechecksVirtualTypeRBSourceAcrossModes(t *testing.T) {
	mainSource := []byte(`import { Input } from contracts/input
import { Context } from trb/web

def bind(context: Context)
	input := context.bind<Input>() catch |_error|
		return
	end
	puts(input.params.id.to_s())
	return
end
`)
	sources := []SourceUnit{
		{
			Filename: "/project/src/contracts/input.trb", ModulePath: "contracts/input",
			Source: []byte(`record Params
	id: Integer
end

record Payload
	title: String
end

record Input
	params: Params
	body: Payload
end
`),
		},
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: mainSource,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject(sources, Options{
				Mode: mode, SourceRoot: "/project/src", ProjectRoot: "/project",
				GoModule: "example.com/package-extension", RubyLoader: "require_relative",
			})
			if err != nil {
				t.Fatal(err)
			}
			main := artifactForModule(artifacts, "main")
			if main == nil || main.CompilerGeneratedStart <= 0 {
				t.Fatalf("%s compilation did not retain the virtual source boundary", mode)
			}
			generatedMethod := false
			for _, statement := range main.AST.Statements {
				method, ok := statement.(*ast.MethodStatement)
				if ok && strings.HasPrefix(method.Name, "__trb_specialize_bind_") {
					generatedMethod = true
					break
				}
			}
			generatedOutput := strings.ReplaceAll(strings.ToLower(string(main.Output)), "_", "")
			if !generatedMethod || !strings.Contains(generatedOutput, "trbspecializebind") {
				t.Fatalf("%s output is missing the rechecked TypeRB helper:\n%s", mode, main.Output)
			}
			generatedImport := false
			for _, statement := range main.IR.Statements {
				imported, ok := statement.(*ir.Import)
				if ok && imported.Implicit && imported.DeclaredPath == "contracts/input" {
					generatedImport = true
				}
			}
			if !generatedImport {
				t.Fatalf("%s compilation did not retain a hidden generated type import", mode)
			}
			for _, mapping := range main.SourceMap.Mappings {
				if mapping.Source.Path == "/project/src/main.trb" && mapping.Source.Span.Start.Offset >= main.CompilerGeneratedStart {
					t.Fatalf("%s source map exposes virtual source: %#v", mode, mapping.Source)
				}
			}
		})
	}
}

func TestWebBindCallSpecializationReusesImportsThroughPackageAliases(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/packages/contracts/src/input.trb", ModulePath: "github.com/acme/contracts/input",
			Source: []byte(`record Payload
	title: String
end
`),
		},
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			PackageAliases: map[string]string{"contracts": "github.com/acme/contracts"},
			Source: []byte(`import { Payload } from contracts/input
import { Context } from trb/web

record Input
	body: Payload
end

def bind(context: Context)
	_input := context.bind<Input>() catch |_error|
		return
	end
	return
end
`),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject(sources, Options{
				Mode: mode, SourceRoot: "/project/src", ProjectRoot: "/project",
				GoModule: "example.com/package-extension", RubyLoader: "require_relative",
			})
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPackageCallSpecializationImportsDoNotBecomeAuthoredSourceVisible(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/contracts/input.trb", ModulePath: "contracts/input",
			Source: []byte(`record HiddenPayload
	page: Integer?
end

alias HiddenAlias = String

record Input
	query: HiddenPayload
	body: HiddenAlias
end
`),
		},
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`import { Input } from contracts/input
import { Context } from trb/web

def bind(context: Context)
	_input := context.bind<Input>() catch |_error|
		return
	end
	return
end

def invalid(value: HiddenAlias)
	return
end
`),
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject(sources, Options{Mode: mode, SourceRoot: "/project/src", ProjectRoot: "/project", GoModule: "example.com/package-extension", RubyLoader: "require_relative"})
			if err == nil || !strings.Contains(err.Error(), "type HiddenAlias is not declared or imported") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}

func TestStandaloneCompilerSupportsPackageCallSpecialization(t *testing.T) {
	source := []byte(`import { Context } from trb/web

record Params
	id: Integer
end

record Input
	params: Params
end

def bind(context: Context)
	input := context.bind<Input>() catch |_error|
		return
	end
	puts(input.params.id.to_s())
	return
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := CompileWithOptions("main.trb", source, Options{Mode: mode, ModulePath: "main", Package: "main", GoModule: "example.com/standalone-specialization", RubyLoader: "require_relative"})
			if err != nil {
				t.Fatal(err)
			}
			generatedOutput := strings.ReplaceAll(strings.ToLower(string(artifact.Output)), "_", "")
			if artifact.CompilerGeneratedStart <= 0 || !strings.Contains(generatedOutput, "trbspecializebind") {
				t.Fatalf("%s standalone output is missing specialization:\n%s", mode, artifact.Output)
			}
		})
	}
}
