package compiler

import (
	"errors"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
)

func TestRunnableMainRequiresExactSignatureAcrossBackends(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "parameter",
			source: `def main(value: Integer)
	puts(value)
	return
end
`,
		},
		{
			name: "return type",
			source: `def main(): Integer
	return 1
end
`,
		},
		{
			name: "type parameter",
			source: `def main<T>()
	return
end
`,
		},
		{
			name: "class function",
			source: `def self.main()
	return
end
`,
		},
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, test := range tests {
			t.Run(mode+"/"+test.name, func(t *testing.T) {
				_, err := Compile("main.trb", []byte(test.source), mode)
				if err == nil || !strings.Contains(err.Error(), "runnable main must have signature def main()") {
					t.Fatalf("expected exact runnable main diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestRubyNativeNestedMainIsNotTheRunnableEntrypoint(t *testing.T) {
	source := []byte(`activate trb/platform/ruby/native

begin
	def main(value: Integer)
		puts(value)
		return
	end
end
`)
	if _, err := Compile("library.trb", source, "ruby"); err != nil {
		t.Fatalf("rejected non-entrypoint native main: %v", err)
	}
}

func TestNonEntrypointMainMethodKeepsOrdinarySignatureRulesAcrossBackends(t *testing.T) {
	source := []byte(`class Runner
	def main(value: Integer): Integer
		return value
	end
end
`)
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if _, err := Compile("runner.trb", source, mode); err != nil {
				t.Fatalf("%s rejected a non-entrypoint main method: %v", mode, err)
			}
		})
	}
}

func TestGoRejectsCrossPackageImportsOfRunnableEntrypointDuringAnalysis(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`OIDC_ISSUER := "https://identity.example.com/"

def main()
	return
end
`),
		},
		{
			Filename: "/project/src/routes/admin.trb", ModulePath: "routes/admin", Package: "routes",
			Source: []byte(`import { OIDC_ISSUER } from main

def issuer(): String
	return OIDC_ISSUER
end
`),
		},
	}
	options := Options{Mode: "go", GoModule: "example.com/root-import", SourceRoot: "/project/src", ProjectRoot: "/project"}
	operations := map[string]func() error{
		"analyze": func() error {
			_, err := AnalyzeProject(sources, options)
			return err
		},
		"compile": func() error {
			_, err := CompileProject(sources, options)
			return err
		},
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			err := operation()
			var compilation *CompileError
			if !errors.As(err, &compilation) || len(compilation.Diagnostics) != 1 {
				t.Fatalf("expected one compile diagnostic, got %v", err)
			}
			item := compilation.Diagnostics[0]
			if item.Code != diagnostic.BackendError || item.Path != sources[1].Filename || item.Span.Start.Offset != 0 {
				t.Fatalf("unexpected runnable import diagnostic: %#v", item)
			}
			if !strings.Contains(item.Message, "cannot import runnable entrypoint module main in Go mode") || !strings.Contains(item.Message, "move shared declarations into a separate module") {
				t.Fatalf("runnable import diagnostic does not explain the correction: %q", item.Message)
			}
			if len(item.Related) != 1 || item.Related[0].Location.Path != sources[0].Filename || item.Related[0].Message != "runnable entrypoint declared here" {
				t.Fatalf("runnable import diagnostic lost its entrypoint location: %#v", item.Related)
			}
		})
	}

	for _, mode := range []string{"ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			portableOptions := options
			portableOptions.Mode = mode
			portableOptions.TypeScriptRuntime = "bun"
			if _, err := AnalyzeProject(sources, portableOptions); err != nil {
				t.Fatalf("%s rejected a backend-safe runnable import: %v", mode, err)
			}
		})
	}
}

func TestGoAllowsSamePackageImportOfRunnableEntrypoint(t *testing.T) {
	sources := []SourceUnit{
		{
			Filename: "/project/src/main.trb", ModulePath: "main", Package: "main",
			Source: []byte(`OIDC_ISSUER := "https://identity.example.com/"

def main()
	return
end
`),
		},
		{
			Filename: "/project/src/config.trb", ModulePath: "config", Package: "main",
			Source: []byte(`import { OIDC_ISSUER } from main

def issuer(): String
	return OIDC_ISSUER
end
`),
		},
	}
	if _, err := AnalyzeProject(sources, Options{Mode: "go", GoModule: "example.com/root-import", SourceRoot: "/project/src", ProjectRoot: "/project"}); err != nil {
		t.Fatalf("same-package import unexpectedly failed: %v", err)
	}
}
