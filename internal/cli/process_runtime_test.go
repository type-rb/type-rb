//go:build darwin || linux

package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
	testPortableProcessProgram(t, source, want)
}

func TestPortableProcessLiteralCommands(t *testing.T) {
	command := filepath.Join(t.TempDir(), "literal command")
	const script = "#!/bin/sh\nprintf 'started'\nfor argument do printf '<%s>' \"$argument\"; done\nprintf 'captured' >&2\n"
	if err := os.WriteFile(command, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	source := `import trb/std/process
import trb/std/result

def report(command: String, args: Array<String>)
	case Process.run(command, args)
	when Result::Ok(value)
		puts("ok")
		puts(value.status)
		puts(value.stdout)
		puts(value.stderr)
		puts(value.success)
	when Result::Err(error)
		puts("error")
		puts(error.operation)
		puts(error.command)
		puts(error.message.empty?())
	end
end

def main()
	report("printf process-shell-probe", [])
	report("printf 'quoted-probe'", [])
	report("printf first; printf second", [])
	report(` + strconv.Quote(command) + `, [])
	report(` + strconv.Quote(command) + `, ["two words", "$(printf expanded)", "; literal", "*"])
end
`
	const want = "error\nrun\nprintf process-shell-probe\nfalse\n" +
		"error\nrun\nprintf 'quoted-probe'\nfalse\n" +
		"error\nrun\nprintf first; printf second\nfalse\n" +
		"ok\n0\nstarted\ncaptured\ntrue\n" +
		"ok\n0\nstarted<two words><$(printf expanded)><; literal><*>\ncaptured\ntrue\n"
	testPortableProcessProgram(t, source, want)
}

func TestPortableProcessOutputDecoding(t *testing.T) {
	const source = `import trb/std/process
import trb/std/result

def report(format: String)
	case Process.run("/usr/bin/printf", [format])
	when Result::Ok(value)
		puts(value.stdout.size())
		puts(value.stdout)
	when Result::Err(error)
		puts(error.operation)
	end
end

def main()
	report("\\377\\377")
	report("\\342\\202")
	report("\\355\\240\\200")
	report("\\360\\237\\230\\200")
	report("\\357\\273\\277X")
	report("a\\000b")
	case Process.run("/bin/sh", ["-c", "printf '\\342'; printf '\\377' >&2; sleep 0.02; printf '\\202\\254'; printf '\\377' >&2"])
	when Result::Ok(value)
		puts(value.stdout)
		puts(value.stderr)
		puts(value.success)
	when Result::Err(error)
		puts(error.operation)
	end
end
`
	const want = "2\n��\n1\n�\n3\n���\n1\n😀\n2\n\ufeffX\n3\na\x00b\n€\n��\ntrue\n"
	testPortableProcessProgram(t, source, want)
}

func TestPortableProcessExplicitInterpreters(t *testing.T) {
	directory := t.TempDir()
	text := filepath.Join(directory, "plain executable")
	empty := filepath.Join(directory, "empty executable")
	missingInterpreter := filepath.Join(directory, "missing interpreter")
	for name, content := range map[string]string{
		text:               "printf explicit\nprintf captured >&2\n",
		empty:              "",
		missingInterpreter: "#!\nprintf unexpected\n",
	} {
		if err := os.WriteFile(name, []byte(content), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	source := `import trb/std/process
import trb/std/result

def report(command: String, args: Array<String>)
	case Process.run(command, args)
	when Result::Ok(value)
		puts("ok")
		puts(value.status)
		puts(value.stdout)
		puts(value.stderr)
		puts(value.success)
	when Result::Err(error)
		puts("error")
		puts(error.operation)
		puts(error.command == command)
		puts(error.message.empty?())
	end
end

def main()
	report(` + strconv.Quote(text) + `, [])
	report(` + strconv.Quote(empty) + `, [])
	report(` + strconv.Quote(missingInterpreter) + `, [])
	report("/bin/sh", [` + strconv.Quote(text) + `])
	report("/bin/sh", ["-c", "printf child; exit 127"])
end
`
	const failure = "error\nrun\ntrue\nfalse\n"
	const want = failure + failure + failure + "ok\n0\nexplicit\ncaptured\ntrue\n" + "ok\n127\nchild\n\nfalse\n"
	testPortableProcessProgram(t, source, want)
}

func testPortableProcessProgram(t *testing.T, source, want string) {
	t.Helper()
	dependencies := t.TempDir()
	installed := false
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
						if config.Mode == "typescript" {
							if !installed {
								installProcessAdapter(t, dependencies)
								installed = true
							}
							if err := os.Symlink(filepath.Join(dependencies, "node_modules"), filepath.Join(root, "node_modules")); err != nil {
								t.Fatal(err)
							}
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

func installProcessAdapter(t *testing.T, root string) {
	t.Helper()
	install := exec.Command("npm", "install", "--prefix", root, "--ignore-scripts", "--no-audit", "--no-fund", "koffi@3.3.7")
	if output, err := install.CombinedOutput(); err != nil {
		t.Fatalf("install process adapter: %v\n%s", err, output)
	}
}

func TestPortableProcessCaptureAndInputBoundaries(t *testing.T) {
	const source = `import trb/std/process
import trb/std/result

def report(command: String, args: Array<String>)
	case Process.run(command, args)
	when Result::Ok(value)
		puts(value.status)
		puts(value.stdout.size())
		puts(value.stderr.size())
	when Result::Err(error)
		puts(error.operation)
		puts(error.command == command)
	end
end

def main()
	report("sh", ["-c", "i=0; while [ $i -lt 5000 ]; do printf 0123456789abcdef; printf 0123456789abcdef >&2; i=$((i+1)); done"])
	report("/bin/sh", ["-c", "if (printf leaked >&3) 2>/dev/null; then printf leaked; else printf closed; fi"])
	report("/bin/cat", [])
	report("", [])
	report("/bin/sh\u0000ignored", [])
	report("/bin/sh", ["-c", "printf ignored\u0000"])
end
`
	const want = "0\n80000\n80000\n0\n6\n0\n0\n0\n0\nrun\ntrue\nrun\ntrue\nrun\ntrue\n"
	testPortableProcessProgram(t, source, want)
}

func TestProcessManagedAndStandaloneDependencies(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node unavailable")
	}
	root := t.TempDir()
	config := project.New(root, "typescript")
	config.DevDependencies = map[string]string{}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
		t.Fatal(err)
	}
	const source = `import trb/std/process
import trb/std/result
def main()
	case Process.run("/bin/echo", ["installed"])
	when Result::Ok(value)
		puts(value.stdout)
	when Result::Err(error)
		puts(error.message)
	end
end
`
	entry := filepath.Join(config.SourcePath(), "main.trb")
	if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	cli := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := cli.Run([]string{"install", "--config", config.Path}); status != 0 {
		t.Fatalf("install: status %d, stderr %s", status, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "package.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Dependencies map[string]string }
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Dependencies["koffi"] != "3.3.7" {
		t.Fatalf("managed process dependency: %s, %v", data, err)
	}
	for _, standalone := range []bool{false, true} {
		args := []string{"run", "--config", config.Path}
		if standalone {
			if err := os.Remove(config.Path); err != nil {
				t.Fatal(err)
			}
			args = []string{"run", "--mode", "typescript", entry}
		}
		stdout.Reset()
		stderr.Reset()
		if status := cli.Run(args); status != 0 || stdout.String() != "installed\n\n" || stderr.Len() != 0 {
			t.Fatalf("standalone=%v status=%d stdout=%q stderr=%s", standalone, status, stdout.String(), stderr.String())
		}
	}
}

func TestProcessImportsSelectHostDependencies(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		config := project.New(t.TempDir(), mode)
		for _, imported := range []string{"trb/std/process", "trb/std/path"} {
			dependencies, err := sourcePackageDependencies(config, [][]byte{[]byte("import " + imported + "\n")}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if imported != "trb/std/process" || mode == "go" {
				if len(dependencies) != 0 {
					t.Fatalf("%s %s acquired unrelated dependencies: %v", mode, imported, dependencies)
				}
			} else if (mode == "ruby" && dependencies["fiddle"] != "1.1.8") || (mode == "typescript" && dependencies["koffi"] != "3.3.7") {
				t.Fatalf("%s process dependencies: %v", mode, dependencies)
			}
		}
	}
}
