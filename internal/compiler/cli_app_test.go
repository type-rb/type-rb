package compiler

import (
	"strings"
	"testing"

	cliapp "github.com/type-rb/type-rb/internal/cliapp"
)

const staticCLIAppSource = `import { run } from trb/cli

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
	source := []byte(`import { run } from trb/cli
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
		_, err := Compile("main.trb", []byte("import { run } from trb/cli\n"), mode)
		if err == nil || !strings.Contains(err.Error(), "does not support mode") {
			t.Fatalf("%s Compile() error=%v, want target diagnostic", mode, err)
		}
	}
}
