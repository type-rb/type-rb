package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	if status := command.Run([]string{"run", "--config", config.Path, "--", "serve", "public", "-p", "8000", "-p", "9000", "-v"}); status != 0 {
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
	if status := command.Run([]string{"run", "--config", config.Path, "--", "serve", "public", "--verbose=false"}); status != 0 {
		t.Fatalf("explicit false status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "public\n8080\nfalse\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected explicit false output stdout=%q stderr=%q", stdout.String(), stderr.String())
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

func TestRunGeneratedStaticCLIReportsApplicationFailureAfterCleanup(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Name = "application-failure"
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-application-failure"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { fail, run } from trb/cli
import trb/std/path
import { File, FileMode } from trb/std/file
import { Result } from trb/std/result

record CleanupArgs
	path: String
end

enum Command
	Direct
	Evaluation
	SafeNavigation
	Cleanup(args: CleanupArgs)
end

record AppArgs
	command: Command @cli(:subcommand)
end

class Consumer
	def take(value: String): String
		return value
	end
end

def no_consumer(): Consumer?
	return nil
end

def before_failure(): String
	puts("before")
	return "value"
end

def consume(left: String, right: String): String
	return left + right
end

def main()
	args := run<AppArgs>(name: "application-failure")
	case args.command
	when Command::Direct
		fail("application failed")
	when Command::Evaluation
		puts(consume(before_failure(), fail("argument failed")))
	when Command::SafeNavigation
		value := no_consumer()&.take(fail("skipped failure"))
		puts(value == nil)
	when Command::Cleanup(cleanup)
		_result := File.open(Path.new(cleanup.path), mode: FileMode::Write) do |file|
			try file.write_text("written before failure")
			fail("cleanup failed")
		end
		case _result
		when Result::Ok(_value)
			return
		when Result::Err(error)
			fail(error.message)
		end
	end
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path, "--compile"}); status != 0 {
		t.Fatalf("compile status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	executable := filepath.Join(root, "bin", config.Name)
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	run := func(arguments ...string) (int, string, string) {
		t.Helper()
		direct := exec.Command(executable, arguments...)
		var directStdout, directStderr bytes.Buffer
		direct.Stdout = &directStdout
		direct.Stderr = &directStderr
		err := direct.Run()
		if err == nil {
			return 0, directStdout.String(), directStderr.String()
		}
		exitError, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("run %v: %v", arguments, err)
		}
		return exitError.ExitCode(), directStdout.String(), directStderr.String()
	}

	status, directStdout, directStderr := run("direct")
	if status != 1 || directStdout != "" || directStderr != "application failed\n" {
		t.Fatalf("direct failure status=%d stdout=%q stderr=%q", status, directStdout, directStderr)
	}
	status, directStdout, directStderr = run("evaluation")
	if status != 1 || directStdout != "before\n" || directStderr != "argument failed\n" {
		t.Fatalf("evaluation failure status=%d stdout=%q stderr=%q", status, directStdout, directStderr)
	}
	status, directStdout, directStderr = run("safe-navigation")
	if status != 0 || directStdout != "true\n" || directStderr != "" {
		t.Fatalf("safe-navigation status=%d stdout=%q stderr=%q", status, directStdout, directStderr)
	}
	outputPath := filepath.Join(root, "cleanup-output.txt")
	status, directStdout, directStderr = run("cleanup", outputPath)
	if status != 1 || directStdout != "" || directStderr != "cleanup failed\n" {
		t.Fatalf("cleanup failure status=%d stdout=%q stderr=%q", status, directStdout, directStderr)
	}
	content, err := os.ReadFile(outputPath)
	if err != nil || string(content) != "written before failure" {
		t.Fatalf("cleanup output content=%q error=%v", content, err)
	}
	renamed := outputPath + ".renamed"
	if err := os.Rename(outputPath, renamed); err != nil {
		t.Fatalf("file handle remained active after failure cleanup: %v", err)
	}
}

func TestRunGeneratedStaticCLICollectsRepeatedOptions(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Name = "repeated"
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/static-cli-repeated"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import { run } from trb/cli

record CollectArgs
	counts: Array<Integer> @cli(:option, name: "count", short: "c", value_name: "COUNT", about: "Add a count")
	tags: Array<String> = [] @cli(:option, name: "tag", short: "t", value_name: "TAG", about: "Add a tag")
	ratios: Array<Float> = [] @cli(:option, name: "ratio", value_name: "RATIO", about: "Add a ratio")
	flags: Array<Boolean> = [] @cli(:option, name: "flag", short: "f", about: "Add a flag")
	fallbacks: Array<String> = ["fallback"] @cli(:option, name: "fallback", value_name: "VALUE", about: "Override fallback values")
end

enum Command
	Collect(args: CollectArgs)
end

record AppArgs
	command: Command @cli(:subcommand)
end

def main()
	args := run<AppArgs>(name: "repeated")
	case args.command
	when Command::Collect(collect)
		puts(collect.tags.join(","))
		puts(collect.counts[0].to_s() + "," + collect.counts[1].to_s() + "," + collect.counts[2].to_s())
		puts(collect.ratios[0].to_s() + "," + collect.ratios[1].to_s())
		puts(collect.flags[0].to_s() + "," + collect.flags[1].to_s() + "," + collect.flags[2].to_s())
		puts(collect.fallbacks.join(","))
	end
	return
end
`
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	arguments := []string{
		"run", "--config", config.Path, "--", "collect",
		"--tag", "one", "--tag=two", "-t", "three",
		"--count", "1", "--count=-2", "-c", "3",
		"--ratio", "1.5", "--ratio=-2.25",
		"--flag", "--flag=false", "-f",
	}
	if status := command.Run(arguments); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "one,two,three\n1,-2,3\n1.5,-2.25\ntrue,false,true\nfallback\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected repeated output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	overridden := append(append([]string(nil), arguments...), "--fallback", "first", "--fallback=second")
	if status := command.Run(overridden); status != 0 {
		t.Fatalf("override status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "one,two,three\n1,-2,3\n1.5,-2.25\ntrue,false,true\nfirst,second\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected repeated override stdout=%q stderr=%q", stdout.String(), stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"run", "--config", config.Path, "--", "collect", "--help"}); status != 0 {
		t.Fatalf("help status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	for _, fragment := range []string{"-t, --tag <TAG>...", "-c, --count <COUNT>...", "--ratio <RATIO>...", "-f, --flag..."} {
		if !strings.Contains(stdout.String(), fragment) {
			t.Fatalf("repeated option help is missing %q: %s", fragment, stdout.String())
		}
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"build", "--config", config.Path, "--compile"}); status != 0 {
		t.Fatalf("compile status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	executable := filepath.Join(root, "bin", config.Name)
	if runtime.GOOS == "windows" {
		executable += ".exe"
	}
	direct := exec.Command(executable, "collect", "--count", "not-an-integer")
	var directStdout, directStderr bytes.Buffer
	direct.Stdout = &directStdout
	direct.Stderr = &directStderr
	err := direct.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 2 {
		t.Fatalf("invalid repeated Integer error=%v, want exit status 2; stdout=%s stderr=%s", err, directStdout.String(), directStderr.String())
	}
	if !strings.Contains(directStderr.String(), `invalid value "not-an-integer" for counts`) {
		t.Fatalf("unexpected invalid repeated Integer error stdout=%q stderr=%q", directStdout.String(), directStderr.String())
	}

	direct = exec.Command(executable, "collect")
	directStdout.Reset()
	directStderr.Reset()
	direct.Stdout = &directStdout
	direct.Stderr = &directStderr
	err = direct.Run()
	exitError, ok = err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 2 {
		t.Fatalf("missing repeated option error=%v, want exit status 2; stdout=%s stderr=%s", err, directStdout.String(), directStderr.String())
	}
	if !strings.Contains(directStderr.String(), "missing option --count") {
		t.Fatalf("unexpected missing repeated option error stdout=%q stderr=%q", directStdout.String(), directStderr.String())
	}

	direct = exec.Command(executable)
	directStdout.Reset()
	directStderr.Reset()
	direct.Stdout = &directStdout
	direct.Stderr = &directStderr
	err = direct.Run()
	exitError, ok = err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 2 {
		t.Fatalf("missing subcommand error=%v, want exit status 2; stdout=%s stderr=%s", err, directStdout.String(), directStderr.String())
	}
	if !strings.Contains(directStderr.String(), "a command is required") {
		t.Fatalf("unexpected missing subcommand error stdout=%q stderr=%q", directStdout.String(), directStderr.String())
	}
}

func TestRunGeneratedStaticCLIReportsPackageInitializationFailure(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		message     string
	}{
		{
			name:        "direct",
			declaration: `value := fail("direct initialization failed")`,
			message:     "direct initialization failed\n",
		},
		{
			name: "transitive",
			declaration: `def initialize_value(): String
	return fail("transitive initialization failed")
end

value: String := initialize_value()`,
			message: "transitive initialization failed\n",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, "go")
			config.Name = "initializer-" + test.name
			config.SourceDir = "src"
			config.Go.Module = "example.com/type-rb/static-cli-initializer-" + test.name
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `import { fail, run } from trb/cli

` + test.declaration + `

enum Command
	Run
end

record AppArgs
	command: Command @cli(:subcommand)
end

def main()
	_args := run<AppArgs>(name: "initializer")
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"build", "--config", config.Path, "--compile"}); status != 0 {
				t.Fatalf("compile status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			executable := filepath.Join(root, "bin", config.Name)
			if runtime.GOOS == "windows" {
				executable += ".exe"
			}
			direct := exec.Command(executable, "run")
			var directStdout, directStderr bytes.Buffer
			direct.Stdout = &directStdout
			direct.Stderr = &directStderr
			err := direct.Run()
			exitError, ok := err.(*exec.ExitError)
			if !ok || exitError.ExitCode() != 1 {
				t.Fatalf("initializer failure error=%v, want exit status 1; stdout=%s stderr=%s", err, directStdout.String(), directStderr.String())
			}
			if directStdout.Len() != 0 || directStderr.String() != test.message {
				t.Fatalf("unexpected initializer failure stdout=%q stderr=%q", directStdout.String(), directStderr.String())
			}
		})
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
