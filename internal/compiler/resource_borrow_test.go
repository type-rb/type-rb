package compiler

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const resourceBorrowImports = `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
`

const resourceBorrowHelpers = `
def read(file: File): Result<String, FileSystemError>
	handle := file
	return handle.read_text(max_bytes: 100)
end

def forward(file: File): Result<String, FileSystemError>
	return read(file)
end

def load(path: Path): Result<String, FileSystemError>
	return File.open(path) do |file|
		try forward(file)
	end
end
`

func TestResourceBorrowSourceHelpers(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			source := resourceBorrowImports + resourceBorrowHelpers
			artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: mode})
			if err != nil {
				t.Fatal(err)
			}
			if mode == "typescript" {
				checkTypeScriptArtifacts(t, artifacts, "resource_borrow")
			}
		})
	}
}

func TestResourceBorrowRejectsEscapeAndTransitiveSuspension(t *testing.T) {
	cases := map[string]string{
		"forged annotated alias": `def read(value: Any)
	handle: File := value
	return
end`,
		"return Any": `def read(file: File): Any
	return file
end`,
		"alias Any": `def read(file: File)
	value: Any := file
	return
end`,
		"mutable alias": `def read(file: File)
	mut value := file
	return
end`,
		"array": `def read(file: File): Array<Any>
	return [file]
end`,
		"closure": `def read(file: File): () -> Result<String, FileSystemError>
	return fn(): Result<String, FileSystemError>
		return file.read_text(max_bytes: 1)
	end
end`,
		"transitive": `def work(): Array<Integer>
	return [1].concurrent_map(limit: 1) do |value|
		value
	end
end
def middle(): Array<Integer>
	return work()
end
def read(_file: File): Array<Integer>
	return middle()
end`,
		"default": `def work(): Integer
	values := [1].concurrent_map(limit: 1) do |value|
		value
	end
	return values[0]
end
def middle(value: Integer = work()): Integer
	return value
end
def read(_file: File): Integer
	return middle()
end`,
		"callback invocation": `def read(_file: File, callback: () -> String): String
	return callback()
end`,
		"generic erasure": `def retain<T>(value: T): Any
	return value
end
def read(file: File): Any
	return retain<File>(file)
end`,
		"field storage": `class Holder
	@value: Any
	def initialize(file: File)
		@value = file
	end
end`,
		"record default": `def work(): Integer
	values := [1].concurrent_map(limit: 1) do |value|
		value
	end
	return values[0]
end
record Config
	value: Integer = work()
end
def read(_file: File): Config
	return Config.new()
end`,
		"constructor": `class Config
	def initialize()
		[1].concurrent_map(limit: 1) do |value|
			value
		end
	end
end
def read(_file: File): Config
	return Config.new()
end`,
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, source := range cases {
			t.Run(mode+"/"+name, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(resourceBorrowImports + source)}}, Options{Mode: mode, AllowUnusedImports: true})
				if err == nil || !strings.Contains(err.Error(), "scoped resource") && !strings.Contains(err.Error(), "scoped File") {
					t.Fatalf("expected resource diagnostic, got %v", err)
				}
			})
		}
	}
}

func TestResourceBorrowImportedIdentityAndIncrementalRecheck(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			helper := SourceUnit{Filename: "/project/src/io.trb", ModulePath: "io", Source: []byte(`import { File as Handle } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
def read(handle: Handle): Result<String, FileSystemError>
	return handle.read_text(max_bytes: 100)
end
`)}
			consumer := SourceUnit{Filename: "/project/src/main.trb", ModulePath: "main", Source: []byte(resourceBorrowImports + `import { read as read_handle } from "./io"
def load(path: Path): Result<String, FileSystemError>
	return File.open(path) do |handle|
		try read_handle(handle)
	end
end
`)}
			options := Options{Mode: mode, ProjectRoot: "/project", SourceRoot: "/project/src"}
			analyzer := NewAnalyzer()
			if _, err := analyzer.AnalyzeProject([]SourceUnit{helper, consumer}, options); err != nil {
				t.Fatal(err)
			}
			helper.Source = []byte(strings.Replace(string(helper.Source), "\treturn handle.read_text", "\t[1].concurrent_map(limit: 1) do |value|\n\t\tvalue\n\tend\n\treturn handle.read_text", 1))
			_, err := analyzer.AnalyzeProject([]SourceUnit{helper, consumer}, options)
			if err == nil || !strings.Contains(err.Error(), "scoped resource") {
				t.Fatalf("incremental resource check: %v", err)
			}
			if !strings.Contains(err.Error(), "io.trb") {
				t.Fatalf("missing helper diagnostic path: %v", err)
			}
		})
	}
}

func TestResourceBorrowRecursiveAndNamedHelpers(t *testing.T) {
	source := resourceBorrowImports + `
def first(file: File, count: Integer): Result<String, FileSystemError>
	if count == 0
		return file.read_text(max_bytes: 100)
	end
	return second(count - 1, file: file)
end
def second(count: Integer, *, file: File): Result<String, FileSystemError>
	return first(file, count)
end
def load(path: Path): Result<String, FileSystemError>
	return File.open(path) do |file|
		try first(file, 2)
	end
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if _, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(source)}}, Options{Mode: mode}); err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestResourceBorrowImportedHelperExecutesAcrossBackends(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sample")
	if err := os.WriteFile(path, []byte("imported"), 0o600); err != nil {
		t.Fatal(err)
	}
	helper := SourceUnit{Filename: "reader.trb", ModulePath: "support/reader", Package: "support", Source: []byte(`import { File as Handle } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
def read(file: Handle): Result<String, FileSystemError>
	handle := file
	return handle.read_text(max_bytes: 100)
end
`)}
	main := SourceUnit{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(resourceBorrowImports + `import { read as read_handle } from support/reader
def main()
	result := File.open(Path.new(` + strconv.Quote(path) + `)) do |file|
		try read_handle(file)
	end
	case result
	when Result::Ok(text)
		puts(text)
	when Result::Err(error)
		puts(error.operation)
	end
end
`)}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifacts, err := CompileProject([]SourceUnit{helper, main}, Options{Mode: mode, GoModule: "example.com/borrow", RubyLoader: "require_relative", AllowUnusedImports: true})
			if err != nil {
				t.Fatal(err)
			}
			requireEffectRuntime(t, mode)
			if got := strings.TrimSpace(runEffectProject(t, mode, artifacts, "example.com/borrow")); got != "imported" {
				t.Fatalf("got %q", got)
			}
		})
	}
}
