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

func TestEvaluateScopedFileAcrossModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def bounded(path: String): Result<String, FileSystemError>
	return File.open(path) do |file|
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

func TestEvaluateFilesystemUTF8ReplacementAcrossModes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.bin")
	input := []byte{
		0x80, 0x80, '|',
		0xe2, 0x82, '|',
		0xe2, 0x28, 0xa1, '|',
		0xed, 0xa0, 0x80, '|',
		0xf0, 0x9f, 0x92, '|',
		0xef, 0xbf, 0xbd,
	}
	if err := os.WriteFile(path, input, 0o644); err != nil {
		t.Fatal(err)
	}
	source := compiler.SourceUnit{
		Filename: "/project/.trb-repl.trb", ModulePath: "__trb_repl__", Package: "main",
		Source: []byte(`import trb/std/file
import { Result } from trb/std/result

def decoded_text(path: String): String
	result := File.open(path) do |file|
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
	result := File.open(path) do |file|
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

decoded_text(` + strconv.Quote(path) + `) + "|" + decoded_bytes(` + strconv.Quote(path) + `)
`),
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := compiler.CompileProject([]compiler.SourceUnit{source}, compiler.Options{
				Mode: mode, GoModule: "example.com/filesystem-utf8-repl", RubyLoader: "require_relative", InteractiveModule: source.ModulePath,
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
			if got := Inspect(result.Value); !result.Display || got != `"��|�|�(�|���|�|�|��|�|�(�|���|�|�"` {
				t.Fatalf("result=%s display=%t, want maximal-subpart replacements", got, result.Display)
			}
		})
	}
}
