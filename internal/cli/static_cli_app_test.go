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
}
