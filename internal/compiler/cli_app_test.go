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

const singleBinaryCLIAppSource = `import { run } from trb/cli

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

func TestCompileProjectGeneratesSingleBinaryCLI(t *testing.T) {
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(singleBinaryCLIAppSource)}}, Options{
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
		"func trb__cliRun0(__trbScope trbcontext.Context, name string, version *string, about *string) AppArgs",
		`Name: "serve", About: "Start the server"`,
		"trb__cliCommand = NewCommandServe(Trb__RecordNew__ServeArgs(",
		"return AppArgs{Command: trb__cliCommand}",
		"func trb__cliParse(args []string",
		"os.Args[1:]",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated Go is missing %q:\n%s", fragment, output)
		}
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, main.Filename, main.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, output)
	}
}

func TestCLIApplicationFailureUsesUnwindBoundary(t *testing.T) {
	source := []byte(`import { fail, run } from trb/cli

record Args
end

def main()
	_args := run<Args>(name: "failure")
	fail("stop")
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-failure", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	fragments := []string{
		"defer trb__cliApplicationFailureBoundary_",
		`panic(trb__cliApplicationFailure_`,
		`("stop"))`,
		"recovered := recover()",
		"fmt.Fprintln(os.Stderr, failure.TrbCLIApplicationFailure())",
		"os.Exit(1)",
	}
	for _, fragment := range fragments {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated CLI failure boundary is missing %q:\n%s", fragment, output)
		}
	}
	if strings.Index(output, fragments[1]) > strings.Index(output, fragments[5]) {
		t.Fatalf("generated CLI exits before the failure signal unwinds:\n%s", output)
	}
}

func TestCLIApplicationFailureSatisfiesNeverValuePositions(t *testing.T) {
	source := []byte(`import { fail, run } from trb/cli

record Args
end

record DeferredFailure
	value: String = fail("record default failure")
end

def default_failure(value: String = fail("parameter default failure")): String
	return value
end

def stopped(): String
	return fail("returned failure")
end

def selected(stop: Boolean): String
	value := if stop
		fail("conditional failure")
	else
		"ok"
	end
	return value
end

def consume(value: String): String
	return value
end

def compound_failure(): String
	return consume("before") + fail("compound failure")
end

def compound_first_failure(): String
	return fail("first compound failure") + consume("after")
end

def all_failed(stop: Boolean): String
	value: String := if stop
		fail("first failure")
	else
		fail("second failure")
	end
	return value
end

def inferred_binding()
	value := fail("binding failure")
	puts(value)
end

def condition_if()
	if fail("if condition")
		puts("unreachable")
	end
	puts("also unreachable")
end

def condition_while()
	while fail("while condition")
		puts("unreachable")
	end
	puts("also unreachable")
end

def unary_failure(): Integer
	return -fail("unary failure")
end

def member_receiver(): String
	return fail("member failure").to_s()
end

def index_receiver(): String
	return fail("receiver failure")[0]
end

def index_position(): String
	return "value"[fail("index failure")]
end

def call_callee(): String
	return fail("callee failure")()
end

def range_start(): Range<Integer>
	return fail("range start failure")..1
end

def range_end(): Range<Integer>
	return 1..fail("range end failure")
end

def inferred_array()
	values := [fail("array failure")]
	puts(values.size())
end

def iteration_source(): Array<String>
	return fail("source failure").map do |value|
		value
	end
end

def guarded(flag: Boolean): Boolean
	return flag && fail("and failure")
end

def fallback(flag: Boolean): Boolean
	return flag || fail("or failure")
end

def lazy_assignment_failure()
	mut left := false
	left &&= fail("skipped and assignment failure")
	mut right := true
	right ||= fail("skipped or assignment failure")
	left = true
	left &&= fail("taken assignment failure")
end

def main()
	_args := run<Args>(name: "failure-values")
	puts(selected(false))
	puts(stopped())
	puts(consume(fail("argument failure")))
	puts(compound_failure())
	puts(compound_first_failure())
	puts(all_failed(false))
	puts(guarded(false))
	puts(fallback(true))
	lazy_assignment_failure()
	inferred_binding()
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-failure-values", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, main.Filename, main.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, main.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, main.Output)
	}
}

func TestCLIApplicationFailureSupportsPackageInitializationBoundary(t *testing.T) {
	source := []byte(`import { fail, run } from trb/cli

direct := fail("direct global failure")

def stop(): String
	return fail("transitive global failure")
end

transitive: String := stop()

record Args
end

def unreachable_reference(): String
	return direct.to_s()
end

def later(): String
	return "later"
end

def main()
	_args := run<Args>(name: "global-failure")
	puts(later())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-global-failure", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	output := string(main.Output)
	if count := strings.Count(output, "defer trb__cliApplicationFailureBoundary_"); count < 3 {
		t.Fatalf("generated Go has %d CLI failure boundaries, want main and two initializers:\n%s", count, output)
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, main.Filename, main.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, output)
	}
}

func TestCLIApplicationFailureRuntimePropagatesFromNestedLambda(t *testing.T) {
	source := []byte(`import { fail, run } from trb/cli

record Args
end

def main()
	_args := run<Args>(name: "nested-failure")
	callback := fn(): String
		return fail("nested failure")
	end
	puts(callback())
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-nested-failure", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if !strings.Contains(string(main.Output), "type trb__cliApplicationFailure_") {
		t.Fatalf("nested CLI failure did not request its runtime marker:\n%s", main.Output)
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, main.Filename, main.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, main.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, main.Output)
	}
}

func TestCLIApplicationFailureRuntimePropagatesFromTransformBody(t *testing.T) {
	source := []byte(`import { fail, run } from trb/cli

record Args
end

def main()
	_args := run<Args>(name: "transform-failure")
	values := ["ok", "stop"].map do |value|
		selected := if value == "stop"
			fail("transform failure")
		else
			value
		end
		selected
	end
	puts(values.join(","))
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-transform-failure", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	main := artifactForModule(artifacts, "main")
	if !strings.Contains(string(main.Output), "type trb__cliApplicationFailure_") {
		t.Fatalf("transform CLI failure did not request its runtime marker:\n%s", main.Output)
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, main.Filename, main.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, main.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, main.Output)
	}
}

func TestCLIRejectsUnsupportedRepeatedOptionShapes(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{name: "positional", field: "values: Array<String>", want: "must be an option, not a positional argument"},
		{name: "nullable array", field: "values: Array<String>? = nil @cli(:option)", want: "must use a non-nullable Array"},
		{name: "nullable element", field: "values: Array<String?> = [] @cli(:option)", want: "must use a non-nullable scalar element type"},
		{name: "non-scalar element", field: "values: Array<Array<String>> = [] @cli(:option)", want: "must use String, Integer, Float, Boolean, or an option Array of one of those scalar types"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(`import { run } from trb/cli
record Args
	` + test.field + `
end
def main()
	_args := run<Args>(name: "bad")
	return
end
`)
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileProject() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCLIRejectsUnsupportedRootTypeShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name: "nullable root",
			source: `record Args
end

def parse(): Args?
	return run<Args?>(name: "nullable")
end`,
			want: "run root Args? must be non-nullable",
		},
		{
			name: "generic root",
			source: `record Args<T>
	name: String
end

def parse(): Args<Integer>
	return run<Args<Integer>>(name: "generic")
end`,
			want: "run root Args<Integer> must be non-generic",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("import { run } from trb/cli\n" + test.source + "\n")
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileProject() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCLIPackageRequiresCurrentNativeExecutableTarget(t *testing.T) {
	for _, mode := range []string{"ruby", "typescript"} {
		_, err := Compile("main.trb", []byte("import { run } from trb/cli\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "does not support mode") {
			t.Fatalf("%s Compile() error=%v, want target diagnostic", mode, err)
		}
	}
}

func TestLegacyGoPlatformCLIImportUsesCanonicalSchema(t *testing.T) {
	source := []byte(`import { run } from trb/platform/go/cli

record Args
	name: String
end

def parse(): Args
	return run<Args>(name: "legacy")
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-compat", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	manifest := cliapp.ManifestFrom(artifactForModule(artifacts, "main").IR.Extensions)
	if manifest == nil || len(manifest.Invocations) != 1 || manifest.Invocations[0].Schema.Root.Name != "Args" {
		t.Fatalf("legacy import did not use the canonical CLI schema: %#v", manifest)
	}
}

func TestCLIRuntimeIsGeneratedOncePerGoPackage(t *testing.T) {
	first := SourceUnit{Filename: "first.trb", ModulePath: "app/first", Package: "app", Source: []byte(`import { fail, run } from trb/cli
record FirstArgs
	value: String
end
def parse_first(): FirstArgs
	return run<FirstArgs>(name: "first")
end
def fail_first()
	fail("first")
end
`)}
	second := SourceUnit{Filename: "second.trb", ModulePath: "app/second", Package: "app", Source: []byte(`import { fail, run } from trb/cli
record SecondArgs
	value: String
end
def parse_second(): SecondArgs
	return run<SecondArgs>(name: "second")
end
def fail_second()
	fail("second")
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
	if count := strings.Count(output.String(), "type trb__cliField struct"); count != 1 {
		t.Fatalf("CLI runtime generated %d times in one Go package:\n%s", count, output.String())
	}
	if count := strings.Count(output.String(), "type trb__cliApplicationFailure_"); count != 2 {
		t.Fatalf("CLI failure marker generated %d times for two source modules:\n%s", count, output.String())
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("app", fileSet, files, nil); typeErr != nil {
		t.Fatalf("generated Go package does not type-check: %v\n%s", typeErr, output.String())
	}
}

func TestCLIRuntimeNamesDoNotCollideWithPrivateFunctions(t *testing.T) {
	source := []byte(`import { run } from trb/cli

def _trb_cli_parse()
	return
end

def _trb_cli_fail()
	return
end

def _trb_cli_run0()
	return
end

record Args
end

def main()
	_args := run<Args>(name: "collision")
	_trb_cli_parse()
	_trb_cli_fail()
	_trb_cli_run0()
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := artifactForModule(artifacts, "main")
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, artifact.Output)
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
			source := []byte("import { run } from trb/cli\nrecord Args\n\t" + test.field + "\nend\ndef parse(): Args\n\treturn run<Args>(name: \"test\")\nend\n")
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.collision+" conflicts with a generated option") {
				t.Fatalf("CompileProject() error=%v, want generated option collision for %s", err, test.collision)
			}
		})
	}
}

func TestCLIRejectsDuplicateMetadataAliases(t *testing.T) {
	source := []byte(`import { run } from trb/cli
record Args
	value: String = "" @cli(:option, name: "first", long: "second")
end
def parse(): Args
	return run<Args>(name: "test")
end
`)
	_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err == nil || !strings.Contains(err.Error(), "duplicate @cli metadata name") {
		t.Fatalf("CompileProject() error=%v, want duplicate metadata diagnostic", err)
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
			sources: []SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { run } from trb/cli

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
				{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import { run } from trb/cli
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

func TestCLIRejectsReservedNames(t *testing.T) {
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
		{
			name: "NUL subcommand",
			source: `enum Command
	Serve @cli(name: "\u0000")
end
record Args
	command: Command @cli(:subcommand)
end`,
			want: `subcommand name "\x00" must not contain U+0000`,
		},
		{
			name: "NUL long option",
			source: `record Args
	value: String = "" @cli(:option, long: "\u0000")
end`,
			want: `long option name "\x00" must not contain U+0000`,
		},
		{
			name: "NUL short option",
			source: `record Args
	value: String = "" @cli(:option, short: "\u0000")
end`,
			want: `short option name "\x00" must not contain U+0000`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte("import { run } from trb/cli\n" + test.source + "\ndef parse(): Args\n\treturn run<Args>(name: \"test\")\nend\n")
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileProject() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCLIRejectsOptionalRootSubcommandSelectors(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  string
	}{
		{name: "default", field: "command: Command = Command::Serve @cli(:subcommand)", want: "cannot declare a default"},
		{name: "nullable", field: "command: Command? @cli(:subcommand)", want: "must be non-nullable"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source := []byte(`import { run } from trb/cli
enum Command
	Serve
end
record Args
	` + test.field + `
end
def parse(): Args
	return run<Args>(name: "test")
end
`)
			_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
				Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
			})
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("CompileProject() error=%v, want %q", err, test.want)
			}
		})
	}
}

func TestCLIAllowsUnicodeNamesOtherThanNUL(t *testing.T) {
	source := []byte(`import { run } from trb/cli
enum Command
	Serve @cli(name: "配信")
end
record Args
	command: Command @cli(:subcommand)
	name: String = "" @cli(:option, long: "名前", short: "名")
end
args := run<Args>(name: "unicode")
`)
	_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCLIQualifiedNestedSchemaUsesBackendIdentifiers(t *testing.T) {
	source := []byte(`import { run } from trb/cli

module Services
	enum Command
		Serve
	end

	record AppArgs
		command: Command @cli(:subcommand)
	end
end

def main()
	_args := run<AppArgs>(name: "nested")
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := artifactForModule(artifacts, "main")
	manifest := cliapp.ManifestFrom(artifact.IR.Extensions)
	if manifest == nil || len(manifest.Invocations) != 1 {
		t.Fatalf("unexpected CLI manifest: %#v", manifest)
	}
	schema := manifest.Invocations[0].Schema
	if schema.Root.Name != "Services::AppArgs" || schema.SubcommandEnum.Name != "Services::Command" {
		t.Fatalf("nested declaration identity was not retained: %#v", schema)
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, artifact.Output)
	}
}

func TestCLIRunForwardsExecutionScopeToRecordDefaults(t *testing.T) {
	source := []byte(`import { run } from trb/cli

def default_port(): Integer
	values := [7000].concurrent_map do |value|
		value
	end
	return values[0]
end

record Args
	port: Integer = default_port() @cli(:option)
end

def parse(): Args
	return run<Args>(name: "scope")
end

def main()
	_args := parse()
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifactForModule(artifacts, "main").Output)
	for _, fragment := range []string{
		"func Parse(__trbScope trbcontext.Context) Args",
		"trb__cliRun0(__trbScope",
		"Trb__RecordNew__Args(__trbScope",
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated Go is missing execution-scope forwarding %q:\n%s", fragment, output)
		}
	}
}

func TestCLINamedOnlyCommandPayloadUsesEnumABI(t *testing.T) {
	source := []byte(`import { run } from trb/cli

record ServeArgs
	path: String
end

enum Command
	Serve(*, args: ServeArgs)
end

record Args
	command: Command @cli(:subcommand)
end

def main()
	_args := run<Args>(name: "named-payload")
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := artifactForModule(artifacts, "main")
	output := string(artifact.Output)
	if !strings.Contains(output, `NewCommandServe(map[string]any{"args": ServeArgs{Path:`) {
		t.Fatalf("generated Go did not use the named-only enum ABI:\n%s", output)
	}
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, output)
	}
}

func TestCLIMetadataAcceptsExplicitNil(t *testing.T) {
	source := []byte(`import { run } from trb/cli

record Args
end

def main()
	_args := run<Args>(name: "nil-metadata", version: nil, about: nil)
	return
end
`)
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: source}}, Options{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/cli-app", SourceRoot: "/project", ProjectRoot: "/project",
	})
	if err != nil {
		t.Fatal(err)
	}
	artifact := artifactForModule(artifacts, "main")
	fileSet := token.NewFileSet()
	parsed, parseErr := parser.ParseFile(fileSet, artifact.Filename, artifact.Output, parser.AllErrors)
	if parseErr != nil {
		t.Fatalf("generated Go does not parse: %v\n%s", parseErr, artifact.Output)
	}
	if _, typeErr := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); typeErr != nil {
		t.Fatalf("generated Go does not type-check: %v\n%s", typeErr, artifact.Output)
	}
}
