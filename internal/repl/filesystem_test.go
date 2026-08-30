package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestEvaluateScopedFilesystemAcrossModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`import trb/std/filesystem
import { Result } from trb/std/result

def bounded(path: String): Result<String, FileSystem::Error>
	return FileSystem.open(path, mode: FileSystem::OpenMode::Read) do |file|
		try file.read_text(max_bytes: 5)
	end
end

case bounded(` + strconv.Quote(path) + `)
when Result::Ok(text)
	text
when Result::Err(error)
	error.operation
end
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/scoped-filesystem-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
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
			if got := Inspect(result.Value); !result.Display || got != `"hello"` {
				t.Fatalf("result=%s display=%t, want hello", got, result.Display)
			}
		})
	}
}
