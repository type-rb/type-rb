package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateDirChildrenPreservesSymlinkParentResolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the symlink/.. resolution reproduction uses POSIX path traversal")
	}

	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	if err := os.MkdirAll(filepath.Join(physical, "pivot"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(physical, "listed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(physical, "listed", "value.txt"), []byte("right"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "listed"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "listed", "value.txt"), []byte("wrong"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(physical, "pivot"), filepath.Join(root, "route")); err != nil {
		t.Skipf("symlinks are unavailable: %v", err)
	}
	separator := string(os.PathSeparator)
	directory := filepath.Join(root, "route") + separator + ".." + separator + "listed"
	expectedChild := directory + separator + "value.txt"
	source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import trb/std/dir
import { DirEntryKind } from trb/std/dir
import { Result } from trb/std/result

def read(path: String): String
	result := File.open(Path.new(path), mode: FileMode::Read) do |file|
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
	case Dir.children(Path.new(directory), max_entries: 1000)
	when Result::Err(error)
		return error.operation
	when Result::Ok(entries)
		mut labels: Array<String> := []
		entries.each do |entry|
			if entry.kind == DirEntryKind::File
				labels.push(entry.name + ":" + read(entry.path.to_s()) + ":" + (entry.path.to_s() == expected).to_s())
			end
		end
		return labels.join(",")
	end
end

listed(` + strconv.Quote(directory) + `, ` + strconv.Quote(expectedChild) + `)
`

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if got := evaluateDirBoundarySource(t, mode, source); got != `"value.txt:right:true"` {
				t.Fatalf("result=%s, want preserved child path", got)
			}
		})
	}
}

func TestEvaluateDirChildrenRejectsNonUTF8Name(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "entries")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	source := `import trb/std/path
import trb/std/dir
import { FileSystemErrorKind, FileSystemTarget } from trb/std/errors
import { Result } from trb/std/result

def listed(path: String): String
	case Dir.children(Path.new(path), max_entries: 1000)
	when Result::Ok(_entries)
		return "ok"
	when Result::Err(error)
		kind := case error.kind
		when FileSystemErrorKind::UnsupportedName
			"unsupported_name"
		else
			"unexpected"
		end
		correct_target := case error.target
		when FileSystemTarget::Host(target_path)
			target_path.to_s() == path
		else
			false
		end
		return error.operation + ":" + kind + ":" + correct_target.to_s() + ":" + error.message
	end
end

listed(` + strconv.Quote(directory) + `)
`

	// Keep source checking covered before the host-specific byte-name fixture.
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run("empty/"+mode, func(t *testing.T) {
			if got := evaluateDirBoundarySource(t, mode, source); got != `"ok"` {
				t.Fatalf("empty listing = %s", got)
			}
		})
	}

	if runtime.GOOS == "windows" {
		t.Skip("Windows directory APIs do not expose POSIX byte names")
	}
	if err := os.WriteFile(directory+string(os.PathSeparator)+string([]byte{0xff}), nil, 0o644); err != nil {
		t.Skipf("the host filesystem does not support a non-UTF-8 name: %v", err)
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			want := `"children:unsupported_name:true:directory entry name is not valid UTF-8"`
			if got := evaluateDirBoundarySource(t, mode, source); got != want {
				t.Fatalf("result=%s, want %s", got, want)
			}
		})
	}
}

func TestEvaluateBoundedDirectoryCreationAndListing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "new", "nested")
	source := `import trb/std/path
import trb/std/dir

def inspect(path: Path): String
	Dir.create_all(path) catch |error|
		return error.operation
	end
	entries := Dir.children(path, max_entries: 0) catch |error|
		return error.operation
	end
	return entries.size().to_s()
end

inspect(Path.new(` + strconv.Quote(path) + `))
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if got := evaluateDirBoundarySource(t, mode, source); got != `"0"` {
			t.Fatalf("%s: %s", mode, got)
		}
	}
}

func evaluateDirBoundarySource(t *testing.T, mode, sourceText string) string {
	t.Helper()
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main", Source: []byte(sourceText),
	}
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
		Mode: mode, GoModule: "example.com/dir-boundary-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
	})
	if err != nil {
		t.Fatal(err)
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	var session *ir.Program
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.IR.ModulePath == source.ModulePath {
			session = artifact.IR
		}
	}
	if session == nil {
		t.Fatal("interactive compilation did not produce a session")
	}
	evaluator := NewEvaluator(&bytes.Buffer{}, mode)
	t.Cleanup(func() { _ = evaluator.Close() })
	if err := evaluator.LoadProject(programs, source.ModulePath); err != nil {
		t.Fatal(err)
	}
	evaluator.LoadDefinitions(session)
	result, err := evaluator.Evaluate(session.Statements, source.ModulePath)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Display {
		t.Fatal("interactive result was not displayed")
	}
	return Inspect(result.Value)
}
