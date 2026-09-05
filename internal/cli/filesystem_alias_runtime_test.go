package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

type filesystemRuntimeBackend struct {
	name       string
	mode       string
	executable string
	bun        bool
}

var filesystemRuntimeBackends = []filesystemRuntimeBackend{
	{name: "go", mode: "go", executable: "go"},
	{name: "ruby", mode: "ruby", executable: "ruby"},
	{name: "typescript-node", mode: "typescript", executable: "node"},
	{name: "typescript-bun", mode: "typescript", executable: "bun", bun: true},
}

func TestRunScopedFilesystemAliasesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runtimeName := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(runtimeName); err != nil {
				t.Skipf("%s is unavailable: %v", runtimeName, err)
			}

			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-filesystem-alias-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			directory := filepath.Join(root, "entries")
			if err := os.MkdirAll(filepath.Join(directory, "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "seed.txt"), []byte("seed"), 0o644); err != nil {
				t.Fatal(err)
			}
			createdPath := filepath.Join(root, "created.txt")
			source := `import trb/std/path
import trb/std/file as HostFile
import { FileMode as OpenMode } from trb/std/file
import trb/std/dir as HostDir
import { DirEntry as Entry, DirEntryKind as EntryKind } from trb/std/dir
import { FileSystemError as IOError, FileSystemErrorKind as IOErrorKind } from trb/std/errors
import { Result as Outcome } from trb/std/result

def create(path: String): Outcome<String, IOError>
	return HostFile.open(Path.new(path), mode: OpenMode::CreateNew) do |file|
		try file.write_text("created")
		"created"
	end
end

def create_label(path: String): String
	return create(path) catch |error|
		if error.kind == IOErrorKind::AlreadyExists
			"exists"
		else
			error.operation
		end
	end
end

def entry_label(entry: Entry<Path>): String
	case entry.kind
	when EntryKind::File
		return entry.name + ":file"
	when EntryKind::Directory
		return entry.name + ":directory"
	when EntryKind::Other
		return entry.name + ":other"
	end
end

def entry_labels(path: String): String
	entries := HostDir.children(Path.new(path), max_entries: 1000) catch |error|
		return error.operation
	end
	mut labels: Array<String> := []
	entries.each do |entry|
		labels.push(entry_label(entry))
	end
	return labels.join(",")
end

def main()
	puts(create_label(` + strconv.Quote(createdPath) + `))
	puts(create_label(` + strconv.Quote(createdPath) + `))
	puts(entry_labels(` + strconv.Quote(directory) + `))
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "created\nexists\nnested:directory,seed.txt:file\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}

func TestRunScopedFilesystemDefaultModeAcrossAvailableBackends(t *testing.T) {
	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}

			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-filesystem-default-mode-test"
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
			seedPath := filepath.Join(root, "seed.txt")
			if err := os.WriteFile(seedPath, []byte("seed"), 0o644); err != nil {
				t.Fatal(err)
			}
			source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read_default(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: 16)
	end
end

def main()
	value := read_default(` + strconv.Quote(seedPath) + `) catch |_error|
		return
	end
	puts(value)
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "seed\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}

func TestRunFilesystemMaximumReadBoundAcrossAvailableBackends(t *testing.T) {
	const maximumReadBound int64 = 9_007_199_254_740_991

	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}

			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-filesystem-maximum-read-bound-test"
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

			inputPath := filepath.Join(root, "tiny.txt")
			if err := os.WriteFile(inputPath, []byte("tiny"), 0o644); err != nil {
				t.Fatal(err)
			}
			maximum := strconv.FormatInt(maximumReadBound, 10)
			source := `import trb/std/path
import trb/std/file
import { Result } from trb/std/result

def read_text_at_maximum(path: String): String
	result := File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: ` + maximum + `)
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def read_bytes_at_maximum(path: String): String
	result := File.open(Path.new(path)) do |file|
		bytes := try file.read(max_bytes: ` + maximum + `)
		bytes.to_s()
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def main()
	puts(read_text_at_maximum(` + strconv.Quote(inputPath) + `))
	puts(read_bytes_at_maximum(` + strconv.Quote(inputPath) + `))
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "tiny\ntiny\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}

func TestRunFilesystemStrictUTF8AndExplicitReplacementAcrossAvailableBackends(t *testing.T) {
	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}

			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-filesystem-utf8-test"
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

			inputPath := filepath.Join(root, "invalid.bin")
			input := []byte{
				0x80, 0x80, '|',
				0xe2, 0x82, '|',
				0xe2, 0x28, 0xa1, '|',
				0xed, 0xa0, 0x80, '|',
				0xf0, 0x9f, 0x92, '|',
				0xef, 0xbf, 0xbd,
			}
			if err := os.WriteFile(inputPath, input, 0o644); err != nil {
				t.Fatal(err)
			}
			source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def decoded_text(path: String): String
	result := File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: 21)
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def decoded_bytes(path: String): String
	result := File.open(Path.new(path)) do |file|
		bytes := try file.read(max_bytes: 21)
		bytes.to_s()
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def main()
	puts(decoded_text(` + strconv.Quote(inputPath) + `))
	puts(decoded_bytes(` + strconv.Quote(inputPath) + `))
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "read_text\n��|�|�(�|���|�|�\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}

func TestRunSuspendingScopedFileAcrossTypeScriptRuntimes(t *testing.T) {
	for _, backend := range filesystemRuntimeBackends {
		if backend.mode != "typescript" {
			continue
		}
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}

			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
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
			outputPath := filepath.Join(root, "output.txt")
			source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def create(path: String): Result<Unit, FileSystemError>
	return File.open(Path.new(path), mode: FileMode::Write) do |file|
		values := [1, 2].concurrent_map(limit: 2) do |value|
			value.to_s()
		end
		try file.write_text(values.join(","))
	end
end

def read(path: String): String
	result := File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: 16)
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		return error.operation
	end
end

def main()
	create(` + strconv.Quote(outputPath) + `) catch |error|
		puts(error.operation)
		return
	end
	puts(read(` + strconv.Quote(outputPath) + `))
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "1,2\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}

func TestRunScopedFilesystemWithNaturalSourceNamesAcrossAvailableBackends(t *testing.T) {
	for _, backend := range filesystemRuntimeBackends {
		t.Run(backend.name, func(t *testing.T) {
			if _, err := exec.LookPath(backend.executable); err != nil {
				t.Skipf("%s is unavailable: %v", backend.executable, err)
			}

			root := t.TempDir()
			config := project.New(root, backend.mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/run-filesystem-source-name-test"
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

			outputPath := filepath.Join(root, "output.txt")
			source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def write(fs: String, flags: String, file: String, data: String): Result<String, FileSystemError>
	return File.open(Path.new(fs), mode: FileMode::Write) do |handle|
		error := "error-ok"
		try handle.write_text(data)
		flags + ":" + file + ":" + error
	end
end

def read(fs: String, file: Integer): Result<String, FileSystemError>
	return File.open(Path.new(fs)) do |handle|
		try handle.read_text(max_bytes: file)
	end
end

def main()
	written := write(` + strconv.Quote(outputPath) + `, "flags-ok", "file-ok", "payload")
	case written
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.operation)
	end
	loaded := read(` + strconv.Quote(outputPath) + `, 16)
	case loaded
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.operation)
	end
	return
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "flags-ok:file-ok:error-ok\npayload\n"; stdout.String() != want {
				t.Fatalf("unexpected output: want %q, got %q", want, stdout.String())
			}
		})
	}
}
