package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

type dirBoundaryBackend struct {
	name       string
	mode       string
	executable string
	bun        bool
}

var dirBoundaryBackends = []dirBoundaryBackend{
	{name: "go", mode: "go", executable: "go"},
	{name: "ruby", mode: "ruby", executable: "ruby"},
	{name: "typescript-node", mode: "typescript", executable: "node"},
	{name: "typescript-bun", mode: "typescript", executable: "bun", bun: true},
}

func TestRunDirChildrenPreservesSymlinkParentResolutionAcrossBackends(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the symlink/.. resolution reproduction uses POSIX path traversal")
	}

	dataRoot := t.TempDir()
	physical := filepath.Join(dataRoot, "physical")
	if err := os.MkdirAll(filepath.Join(physical, "pivot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(physical, "listed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "listed", "value.txt"), []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dataRoot, "listed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataRoot, "listed", "value.txt"), []byte("wrong"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(physical, "pivot"), filepath.Join(dataRoot, "route")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	separator := string(os.PathSeparator)
	directory := filepath.Join(dataRoot, "route") + separator + ".." + separator + "listed"
	expectedChild := directory + separator + "value.txt"

	source := `import trb/std/file
import { FileMode } from trb/std/file
import trb/std/dir
import { DirEntryKind } from trb/std/dir
import { Result } from trb/std/result

def read(path: String): String
	result := File.open(path, mode: FileMode::Read) do |file|
		try file.read_text(max_bytes: 5)
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def listed(directory: String, expected: String): String
	case Dir.children(directory)
	when Result::Err(error)
		return error.operation
	when Result::Ok(entries)
		mut labels: Array<String> := []
		entries.each do |entry|
			if entry.kind == DirEntryKind::File
				labels.push(entry.name + ":" + read(entry.path) + ":" + (entry.path == expected).to_s())
			end
		end
		return labels.join(",")
	end
end

def main()
	puts(listed(` + strconv.Quote(directory) + `, ` + strconv.Quote(expectedChild) + `))
	return
end
`

	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			stdout := runDirBoundaryProject(t, backend, source)
			if want := "value.txt:right:true\n"; stdout != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout)
			}
		})
	}
}

func TestRunDirChildrenRejectsNonUTF8NamesAcrossBackends(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows directory APIs do not expose POSIX byte names")
	}

	directory := filepath.Join(t.TempDir(), "entries")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	invalidName := string([]byte{0xff})
	if err := os.WriteFile(directory+string(os.PathSeparator)+invalidName, nil, 0o644); err != nil {
		t.Skipf("the host filesystem does not support a non-UTF-8 name: %v", err)
	}

	source := `import trb/std/dir
import { FileSystemErrorKind } from trb/std/errors
import { Result } from trb/std/result

def listed(path: String): String
	case Dir.children(path)
	when Result::Ok(_entries)
		return "ok"
	when Result::Err(error)
		kind := case error.kind
		when FileSystemErrorKind::Other
			"other"
		else
			"unexpected"
		end
		return error.operation + ":" + kind + ":" + (error.path == path).to_s() + ":" + error.message
	end
end

def main()
	puts(listed(` + strconv.Quote(directory) + `))
	return
end
`

	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			stdout := runDirBoundaryProject(t, backend, source)
			want := "children:other:true:directory entry name is not valid UTF-8\n"
			if stdout != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout)
			}
		})
	}
}

func requireDirBoundaryRuntime(t *testing.T, backend dirBoundaryBackend) {
	t.Helper()
	if _, err := exec.LookPath(backend.executable); err != nil {
		t.Skipf("%s is unavailable: %v", backend.executable, err)
	}
}

func runDirBoundaryProject(t *testing.T, backend dirBoundaryBackend, source string) string {
	t.Helper()
	root := t.TempDir()
	config := project.New(root, backend.mode)
	config.SourceDir = "src"
	if config.Go != nil {
		config.Go.Module = "example.com/type-rb/dir-boundary-test"
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
	if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	return stdout.String()
}
