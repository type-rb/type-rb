package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunGeneratedStaticCLIApplication(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { run } from trb/cli

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
	case args.command
	when Command::Serve(serve)
		puts(serve.directory)
		puts(serve.port)
		puts(serve.verbose)
	when Command::Version
		puts("version command")
	end
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path, "--", "serve", "public", "-p", "9000", "-v"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "public\n9000\ntrue\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path, "--", "serve", "public", "-p", "9007199254740992"}); status == 0 {
		t.Fatalf("out-of-range Integer unexpectedly succeeded stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `invalid value "9007199254740992" for port`) {
		t.Fatalf("unexpected out-of-range Integer error stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path, "--", "serve", "public"}); status != 0 {
		t.Fatalf("default status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "public\n8080\nfalse\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected default output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path, "--", "--help"}); status != 0 {
		t.Fatalf("help status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	for _, fragment := range []string{"Usage: demo <COMMAND>", "A generated CLI", "serve", "Start the server", "--version"} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Fatalf("help is missing %q: %s", fragment, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path, "--", "--", "serve", "public"}); status == 0 {
		t.Fatalf("argument after -- unexpectedly selected a command stdout=%s stderr=%s", stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `unexpected argument "serve"`) {
		t.Fatalf("unexpected -- error stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunGeneratedStaticCLIPreservesMetadataArgumentOrder(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-order"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { run } from trb/cli

record Args
end

def metadata(value: String): String
	puts(value)
	return value
end

def main()
	_args := run<Args>(about: metadata("about"), name: metadata("name"), version: metadata("version"))
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "about\nname\nversion\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunGeneratedStaticCLIWithImportedAliasAndUnicodeOption(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-import"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	models := filepath.Join(config.SourcePath(), "models")
	if err := os.MkdirAll(models, 0o755); err != nil {
		t.Fatal(err)
	}
	arguments := `record Args
	name: String @cli(:option, long: "名前", short: "名")
end

alias Arguments = Args
`
	if err := os.WriteFile(filepath.Join(models, "arguments.trb"), []byte(arguments), 0o644); err != nil {
		t.Fatal(err)
	}
	main := `import { run } from trb/cli
import { Arguments } from models/arguments

def main()
	args := run<Arguments>(name: "unicode")
	puts(args.name)
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	command := &CLI{Stdin: strings.NewReader("")}
	for _, test := range []struct {
		name string
		args []string
		want string
	}{
		{name: "long", args: []string{"--名前", "太郎"}, want: "太郎\n"},
		{name: "short", args: []string{"-名", "花子"}, want: "花子\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command.Stdout = &stdout
			command.Stderr = &stderr
			arguments := append([]string{"run", "--config", config.Path, "--"}, test.args...)
			if status := command.Run(arguments); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != test.want || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunGeneratedStaticCLIPositionalHelpDoesNotClaimOptions(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-help"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { run } from trb/cli

record Args
	path: String @cli(about: "Input path", value_name: "FILE")
end

def main()
	_args := run<Args>(name: "positional")
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path, "--", "--help"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String(), "Usage: positional <FILE>") || !strings.Contains(stdout.String(), "Arguments:") || !strings.Contains(stdout.String(), "<FILE>") || !strings.Contains(stdout.String(), "Input path") || strings.Contains(stdout.String(), "Usage: positional [OPTIONS]") || stderr.Len() != 0 {
		t.Fatalf("unexpected help stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunGeneratedStaticCLISeparatesRootAndCommandValues(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-keys"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { run } from trb/cli

record CommandArgs
	value: String @cli(:option, long: "command-value")
end

enum Command
	Root(args: CommandArgs) @cli(name: "root")
end

record Args
	value: String @cli(:option, long: "root-value")
	command: Command @cli(:subcommand)
end

def main()
	args := run<Args>(name: "keys")
	puts(args.value)
	case args.command
	when Command::Root(command)
		puts(command.value)
	end
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	arguments := []string{"run", "--config", config.Path, "--", "--root-value", "outer", "root", "--command-value", "inner"}
	if status := command.Run(arguments); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "outer\ninner\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
