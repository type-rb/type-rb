//go:build darwin || linux

package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestPortableProcessExitOutcomes(t *testing.T) {
	const source = `import trb/std/process
import trb/std/result

def report(command: String, args: Array<String>)
	case Process.run(command, args)
	when Result::Ok(value)
		puts(value.status)
		puts(value.stdout)
		puts(value.stderr)
		puts(value.success)
		puts(value.success == (value.status == 0))
	when Result::Err(error)
		puts(error.operation)
		puts(error.command)
		puts(error.message.empty?())
	end
end

def main()
	report("/bin/sh", ["-c", "printf out; printf err >&2"])
	report("/bin/sh", ["-c", "printf before; printf failure >&2; exit 7"])
	report("/bin/sh", ["-c", "printf signal; printf stopped >&2; kill -TERM $$"])
	report("trb-process-command-does-not-exist", [])
end
`
	const want = "0\nout\nerr\ntrue\ntrue\n" +
		"7\nbefore\nfailure\nfalse\ntrue\n" +
		"-1\nsignal\nstopped\nfalse\ntrue\n" +
		"run\ntrb-process-command-does-not-exist\nfalse\n"
	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}
			for _, path := range []string{"run", "repl"} {
				t.Run(path, func(t *testing.T) {
					root := t.TempDir()
					config := project.New(root, backend.mode)
					config.SourceDir = "src"
					if config.Go != nil {
						config.Go.Module = "example.com/process-outcomes"
					}
					if backend.bun {
						config.TypeScript.Runtime = project.TypeScriptRuntimeBun
						config.TypeScript.PackageManager = "bun"
					}
					if err := config.Save(); err != nil {
						t.Fatal(err)
					}
					if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
						t.Fatal(err)
					}
					if path == "run" {
						if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					var stdout, stderr bytes.Buffer
					input := ""
					if path == "repl" {
						input = source + "\nmain()\n:quit\n"
					}
					command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
					if status := command.Run([]string{path, "--config", config.Path}); status != 0 {
						t.Fatalf("status=%d stderr=%s", status, stderr.String())
					}
					if stdout.String() != want || stderr.Len() != 0 {
						t.Fatalf("unexpected process outcomes\nwant: %q\ngot: %q\nstderr: %s", want, stdout.String(), stderr.String())
					}
				})
			}
		})
	}
}
