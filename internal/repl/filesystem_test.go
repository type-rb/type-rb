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
		Source: []byte(`import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def bounded(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
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

func TestEvaluateFilesystemStrictUTF8AndExplicitReplacementAcrossModes(t *testing.T) {
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
		Source: []byte(`import trb/std/path
import trb/std/file
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
			if got := Inspect(result.Value); !result.Display || got != `"read_text|��|�|�(�|���|�|�"` {
				t.Fatalf("result=%s display=%t, want strict text failure and explicit byte replacement", got, result.Display)
			}
		})
	}
}

func TestEvaluateFilesystemPropagationAndResultValuedSuccess(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte{0xff}, 0o600); err != nil {
		t.Fatal(err)
	}
	source := `import trb/std/path
import trb/std/file
import trb/std/result

def inspect_result(path: Path): String
	result := File.open(path) do |file|
		text := try file.read_text(max_bytes: 10)
		text
	end
	case result
	when Result::Err(error)
		return error.operation
	when Result::Ok(_text)
		return "wrong"
	end
end

def nested_result(path: Path): String
	result := File.open(path) do |_file|
		Result<String, String>::Err("domain-value")
	end
	case result
	when Result::Err(_error)
		return "wrong"
	when Result::Ok(inner)
		case inner
		when Result::Ok(text)
			return text
		when Result::Err(error)
			return error
		end
	end
end

inspect_result(Path.new(` + strconv.Quote(path) + `)) + ":" + nested_result(Path.new(` + strconv.Quote(path) + `))
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if got := evaluateDirBoundarySource(t, mode, source); got != `"read_text:domain-value"` {
			t.Fatalf("%s: %s", mode, got)
		}
	}
}
