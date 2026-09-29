package compiler

import (
	"strings"
	"testing"
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

func TestGoAcceptsCrossDirectoryImportsOfRunnableEntrypointDuringAnalysis(t *testing.T) {
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
			if err := operation(); err != nil {
				t.Fatalf("valid entrypoint import rejected: %v", err)
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
