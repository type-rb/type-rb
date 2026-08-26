package compiler

import (
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"strings"
	"testing"

	cliapp "github.com/type-rb/type-rb/internal/cliapp"
)

const staticCLIAppSource = `import { run } from trb/platform/go/cli

record ServeArgs
	directory: String
	port: Integer = 8080 @cli(:option, short: "p", about: "Port to listen on")
	verbose: Boolean = false @cli(:option, short: "v", about: "Enable verbose output")
end

enum Command
	Serve(args: ServeArgs) @cli(about: "Start the server")
	Version @cli(about: "Print version details")
end

record AppArgs
	command: Command @cli(:subcommand)
end

def main()
	args := run<AppArgs>(name: "demo", version: "1.0.0", about: "A generated CLI")
	puts(args.command)
	return
end
`

func TestCompileProjectGeneratesStaticGoCLI(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(staticCLIAppSource)}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if main == nil {
		t.Fatal("main artifact was not generated")
	}
	manifest := cliapp.ManifestFrom(main.IR.Extensions)
	if manifest == nil || len(manifest.Invocations) != 1 || len(manifest.Invocations[0].Schema.Commands) != 2 {
		t.Fatalf("unexpected CLI manifest: %#v", manifest)
	}
	output := string(main.Output)
	for _, fragment := range []string{
		"func trbCliRun0(name string, version *string, about *string) AppArgs",
		`Name: "serve", About: "Start the server"`,
		"trbCliCommand = NewCommandServe(TrbRecordNewServeArgs(",
		"return AppArgs{Command: trbCliCommand}",
		"func trbCliParse(args []string",
		"os.Args[1:]",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated Go is missing %q:\n%s", fragment, output)
		}
	}
}

func TestCLIRejectsUnsupportedSchemaShapes(t *testing.T) {
	source := []byte(`import { run } from trb/platform/go/cli
record Args
	values: Array<String>
end
def main()
	args := run<Args>(name: "bad")
	puts(args.values)
	return
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err == nil || !strings.Contains(err.Error(), "must use String, Integer, Float, or Boolean") {
		t.Fatalf("CompileProject() error=%v, want unsupported CLI field diagnostic", err)
	}
}

func TestCLIPackageIsGoOnly(t *testing.T) {
	for _, mode := range []string{"ruby", "typescript"} {
		_, err := Compile("main.trb", []byte("import { run } from trb/platform/go/cli\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "does not support mode") {
			t.Fatalf("%s Compile() error=%v, want target diagnostic", mode, err)
		}
	}
}

func TestCLIRuntimeIsGeneratedOncePerGoPackage(t *testing.T) {
	first := SourceUnit{Filename: "first.trb", ModulePath: "app/first", Package: "app", Source: []byte(`import { run } from trb/platform/go/cli
record FirstArgs
	value: String
end
def parse_first(): FirstArgs
	return run<FirstArgs>(name: "first")
end
`)}
	second := SourceUnit{Filename: "second.trb", ModulePath: "app/second", Package: "app", Source: []byte(`import { run } from trb/platform/go/cli
record SecondArgs
	value: String
end
def parse_second(): SecondArgs
	return run<SecondArgs>(name: "second")
end
`)}
	artifacts, err := CompileProject([]SourceUnit{first, second}, Options{Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	fileSet := token.NewFileSet()
	files := []*goast.File{}
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.IR.ModulePath, "app/") {
			output.Write(artifact.Output)
			parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
			if parseErr != nil {
				t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
			}
			files = append(files, parsed)
		}
	}
	if count := strings.Count(output.String(), "type trbCliField struct"); count != 1 {
		t.Fatalf("CLI runtime generated %d times in one Go package:\n%s", count, output.String())
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("app", fileSet, files, nil); typeErr != nil {
		t.Fatalf("generated Go package does not type-check: %v\n%s", typeErr, output.String())
	}
}

func TestCLIRejectsGeneratedOptionCollisions(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		collision string
	}{
		{name: "long help", field: `help: Boolean = false @cli(:option)`, collision: "--help"},
		{name: "short help", field: `verbose: Boolean = false @cli(:option, short: "h")`, collision: "-h"},
		{name: "long version", field: `version: String = "" @cli(:option)`, collision: "--version"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("import { run } from trb/platform/go/cli\nrecord Args\n\t" + test.field + "\nend\ndef parse(): Args\n\treturn run<Args>(name: \"test\")\nend\n")
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.collision+" conflicts with a generated option") {
				t.Fatalf("CompileProject() error=%v, want generated option collision for %s", err, test.collision)
			}
		})
	}
}

func TestCLIRootResolvesTransparentRecordAlias(t *testing.T) {
	tests := []struct {
		name    string
		sources []SourceUnit
		module  string
	}{
		{
			name: "local alias",
			sources: []SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { run } from trb/platform/go/cli

record AppArgs
	name: String
end

alias Arguments = AppArgs

args := run<Arguments>(name: "cli-alias")
			`)}},
			module: "main",
		},
		{
			name: "imported alias chain",
			sources: []SourceUnit{
				{Filename: "arguments.trb", ModulePath: "models/arguments", Source: []byte(`record AppArgs
	name: String
end

alias BaseArguments = AppArgs
alias Arguments = BaseArguments
				`)},
				{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { run } from trb/platform/go/cli
import { Arguments } from models/arguments

args := run<Arguments>(name: "cli-alias")
				`)},
			},
			module: "models/arguments",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			artifacts, err := CompileProject(test.sources, Options{
				Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err != nil {
				t.Fatal(err)
			}
			manifest := cliapp.ManifestFrom(artifactForModule(artifacts, "main").IR.Extensions)
			if manifest == nil || len(manifest.Invocations) != 1 || manifest.Invocations[0].Schema.Root.Name != "AppArgs" || manifest.Invocations[0].Schema.Root.ModulePath != test.module {
				t.Fatalf("transparent CLI root alias did not resolve to %s::AppArgs: %#v", test.module, manifest)
			}
		})
	}
}

func TestCLIRejectsNamesReservedByTheParserGrammar(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "subcommand starting with hyphen",
			source: `enum Command
	Serve @cli(name: "-serve")
end
record Args
	command: Command @cli(:subcommand)
end`,
			want: `subcommand "-serve" must not start with '-'`,
		},
		{
			name: "long option containing equals",
			source: `record Args
	value: String = "" @cli(:option, long: "foo=bar")
end`,
			want: `long option "foo=bar" for field value must not contain '='`,
		},
		{
			name: "reserved short option",
			source: `record Args
	value: String = "" @cli(:option, short: "-")
end`,
			want: "short option for value must not be '-'",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("import { run } from trb/platform/go/cli\n" + test.source + "\ndef parse(): Args\n\treturn run<Args>(name: \"test\")\nend\n")
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileProject() error=%v, want %q", err, test.want)
			}
		})
	}
}
