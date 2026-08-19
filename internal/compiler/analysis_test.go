package compiler

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/schemalock"
	"github.com/type-rb/type-rb/internal/sourcemap"
)

func TestAnalyzeProjectProducesSemanticArtifactsWithoutBackendOutput(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/models/user.trb", ModulePath: "models/user", Package: "models", ExternalPackage: true,
			Source: []byte("record User\n\tname: String\nend\n"),
		},
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte("import { User } from models/user\n\ndef name(user: User): String\n\treturn user.name\nend\n"),
		},
	}
	options := Options{Mode: "go", GoModule: "example.com/analysis", SourceRoot: "/project/src", ProjectRoot: "/project"}

	analyzed, err := AnalyzeProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	compiled, err := CompileProject(sources, options)
	if err != nil {
		t.Fatal(err)
	}
	if len(analyzed) != len(compiled) || len(analyzed) != 2 {
		t.Fatalf("artifact counts analyze=%d compile=%d, want 2", len(analyzed), len(compiled))
	}
	for _, semantic := range analyzed {
		if semantic.AST == nil || semantic.IR == nil {
			t.Fatalf("analysis artifact is missing semantic state: %#v", semantic)
		}
		if len(semantic.Output) != 0 || semantic.SourceMap.Version != 0 || len(semantic.SourceMap.Mappings) != 0 {
			t.Fatalf("analysis generated backend output: %#v", semantic)
		}
		generated := artifactForModule(compiled, semantic.IR.ModulePath)
		if generated == nil {
			t.Fatalf("compiled artifacts omit module %s", semantic.IR.ModulePath)
		}
		if generated.Filename != semantic.Filename || generated.Mode != semantic.Mode || generated.CompilerOwned != semantic.CompilerOwned || generated.Official != semantic.Official || generated.ExternalPackage != semantic.ExternalPackage {
			t.Fatalf("artifact metadata changed during generation:\nanalyze=%#v\ncompile=%#v", semantic, generated)
		}
		if len(generated.Output) == 0 || generated.SourceMap.Version != sourcemap.Version {
			t.Fatalf("compiled artifact has no generated output or source map: %#v", generated)
		}
	}
}

func TestAnalyzeProjectMatchesCompileProjectDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		sources []SourceUnit
		code    diagnostic.Code
	}{
		{
			name: "syntax",
			sources: []SourceUnit{{
				Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
				Source: []byte("def view(): Any\n\treturn <div>missing\nend\n"),
			}},
			code: diagnostic.SyntaxError,
		},
		{
			name: "resolution",
			sources: []SourceUnit{{
				Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
				Source: []byte("import { Missing } from models/missing\n"),
			}},
			code: diagnostic.ResolutionError,
		},
		{
			name: "type",
			sources: []SourceUnit{{
				Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
				Source: []byte("def value(): String\n\treturn 1\nend\n"),
			}},
			code: diagnostic.TypeError,
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			options := Options{Mode: mode, GoModule: "example.com/analysis", RubyLoader: "require_relative", TypeScriptRuntime: "bun", SourceRoot: "/project/src", ProjectRoot: "/project"}
			for _, test := range tests {
				t.Run(test.name, func(t *testing.T) {
					analyzeDiagnostics := compileErrorDiagnostics(t, func() error {
						_, err := AnalyzeProject(test.sources, options)
						return err
					})
					compileDiagnostics := compileErrorDiagnostics(t, func() error {
						_, err := CompileProject(test.sources, options)
						return err
					})
					if len(analyzeDiagnostics) == 0 || analyzeDiagnostics[0].Code != test.code {
						t.Fatalf("analysis diagnostics=%#v, want first code %s", analyzeDiagnostics, test.code)
					}
					if !reflect.DeepEqual(analyzeDiagnostics, compileDiagnostics) {
						t.Fatalf("analysis and compilation diagnostics differ:\nanalyze=%#v\ncompile=%#v", analyzeDiagnostics, compileDiagnostics)
					}
				})
			}
		})
	}
}

func TestAnalyzeProjectMatchesCompileProjectBackendValidation(t *testing.T) {
	ormRoot := t.TempDir()
	if err := schemalock.New("sqlite").Write(filepath.Join(ormRoot, "schema.lock.json")); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		sources []SourceUnit
		options Options
		want    string
	}{
		{
			name: "typescript ORM requires bun",
			sources: []SourceUnit{{
				Filename: filepath.Join(ormRoot, "src", "main.trb"), ModulePath: "main", Package: "main",
				Source: []byte("import { Database } from trb/orm\n\ndef main()\n\treturn\nend\n"),
			}},
			options: Options{
				Mode: "typescript", TypeScriptRuntime: "node", SourceRoot: filepath.Join(ormRoot, "src"), ProjectRoot: ormRoot,
				PackageOptions: map[string][]byte{"trb/orm": []byte(`{"adapter":"sqlite","database":"application.sqlite3","schemaLock":"schema.lock.json"}`)},
			},
			want: `trb/orm in mode: typescript currently requires typescript.runtime: "bun"`,
		},
		{
			name: "typescript jobs require bun",
			sources: []SourceUnit{
				{
					Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
					Source: []byte(`import { Job } from trb/jobs

class ExampleJob < Job
	def perform()
		return
	end
end

def main()
	return
end
`),
				},
				{Filename: "/project/src/config/jobs.trb", ModulePath: "config/jobs", Package: "config", Source: []byte(jobsSQLConfigurationSource)},
			},
			options: Options{Mode: "typescript", TypeScriptRuntime: "node", SourceRoot: "/project/src", ProjectRoot: "/project", JobsConfiguration: "config/jobs"},
			want:    `trb/jobs in mode: typescript currently requires typescript.runtime: "bun"`,
		},
		{
			name: "typescript suspending pure function value",
			sources: []SourceUnit{{
				Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
				Source: []byte(`import { HttpClient } from trb/platform/typescript/browser

def build_loader(client: HttpClient)
	loader := fn(): String
		response := client.request("/todos")
		puts(response)
		return "loaded"
	end
	puts(loader)
	return
end
`),
			}},
			options: Options{Mode: "typescript", TypeScriptRuntime: "browser", SourceRoot: "/project/src", ProjectRoot: "/project"},
			want:    "TypeScript function values that may suspend must omit their return type",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, analyzeErr := AnalyzeProject(test.sources, test.options)
			_, compileErr := CompileProject(test.sources, test.options)
			if analyzeErr == nil || compileErr == nil {
				t.Fatalf("backend validation analyze=%v compile=%v, want both errors", analyzeErr, compileErr)
			}
			if analyzeErr.Error() != compileErr.Error() || analyzeErr.Error() != test.want {
				t.Fatalf("backend validation differs:\nanalyze=%v\ncompile=%v\nwant=%s", analyzeErr, compileErr, test.want)
			}
		})
	}
}

func TestAnalyzeProjectIncludesProjectIntegrationDiagnostics(t *testing.T) {
	sources := []SourceUnit{
		{Filename: "/project/src/main.trb", ModulePath: "main", Package: "main", Source: []byte("def main()\n\treturn\nend\n")},
		{
			Filename: "/project/src/routes/todos.trb", ModulePath: "routes/todos", Package: "routes",
			Source: []byte(`import { Context } from trb/web

def post(context: Context)
	puts(context.request.path)
	return
end
`),
		},
	}
	diagnostics := compileErrorDiagnostics(t, func() error {
		_, err := AnalyzeProject(sources, Options{Mode: "go", GoModule: "example.com/analysis", SourceRoot: "/project/src", ProjectRoot: "/project"})
		return err
	})
	if len(diagnostics) != 1 || diagnostics[0].Code != diagnostic.ProjectIntegration {
		t.Fatalf("project analysis diagnostics=%#v, want one integration diagnostic", diagnostics)
	}
}

func BenchmarkProjectAnalysisWithoutBackendGeneration(b *testing.B) {
	const modules = 64
	sources := make([]SourceUnit, 0, modules)
	for index := 0; index < modules; index++ {
		sources = append(sources, SourceUnit{
			Filename:   fmt.Sprintf("/project/src/module_%03d.trb", index),
			ModulePath: fmt.Sprintf("module_%03d", index),
			Package:    "main",
			Source:     []byte(fmt.Sprintf("def value_%03d(): Integer\n\treturn %d\nend\n", index, index)),
		})
	}
	options := Options{Mode: "go", GoModule: "example.com/analysis", SourceRoot: "/project/src", ProjectRoot: "/project"}
	b.Run("analyze", func(b *testing.B) {
		for b.Loop() {
			if _, err := AnalyzeProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("compile", func(b *testing.B) {
		for b.Loop() {
			if _, err := CompileProject(sources, options); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func compileErrorDiagnostics(t *testing.T, operation func() error) []diagnostic.Diagnostic {
	t.Helper()
	err := operation()
	if err == nil {
		t.Fatal("operation succeeded, want diagnostics")
	}
	var compilation *CompileError
	if !errors.As(err, &compilation) {
		t.Fatalf("error type=%T, want *CompileError: %v", err, err)
	}
	return compilation.Diagnostics
}
