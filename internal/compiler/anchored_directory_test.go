package compiler

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const anchoredDirectorySource = `import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/dir
import trb/std/file
import { DirEntry } from trb/std/dir
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read(file: File): Result<String, FileSystemError>
	return file.read_text(max_bytes: 100)
end

def list(root: Dir): Result<Array<DirEntry<RelativePath>>, FileSystemError>
	anchor := root
	return anchor.children(max_entries: 20)
end

def load(root: Dir, path: RelativePath): Result<String, FileSystemError>
	return root.open_file(path) do |file|
		try read(file)
	end
end

def run(path: Path, child: RelativePath): Result<String, FileSystemError>
	return Dir.open(path) do |root|
		try root.create_all(child)
		_entries := try list(root)
		try load(root, child)
	end
end
`

func TestAnchoredDirectoryCompiles(t *testing.T) {
	_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(anchoredDirectorySource)}}, Options{Mode: "go"})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAnchoredDirectoryExplicitTypeArguments(t *testing.T) {
	source := strings.ReplaceAll(anchoredDirectorySource, "root.open_file(path)", "root.open_file<String>(path)")
	source = strings.ReplaceAll(source, "Dir.open(path)", "Dir.open<String>(path)")
	if _, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "go"}); err != nil {
		t.Fatal(err)
	}
	invalid := strings.ReplaceAll(source, "root.open_file<String>", "root.open_file<Integer>")
	if _, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(invalid)}}, Options{Mode: "go"}); err == nil {
		t.Fatal("explicit result type was ignored")
	}
}

func TestAnchoredDirectoryUnsupportedBackends(t *testing.T) {
	for _, mode := range []string{"ruby", "typescript"} {
		_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(anchoredDirectorySource)}}, Options{Mode: mode})
		if err == nil || !strings.Contains(err.Error(), "anchored Dir operations require") {
			t.Fatalf("%s: %v", mode, err)
		}
	}
}

func TestAnchoredDirectoryRejectsEscapeAndSuspension(t *testing.T) {
	imports := "import trb/std/dir\n"
	cases := map[string]string{
		"return": `def leak(root: Dir): Any
	return root
end`,
		"storage": `def leak(root: Dir): Array<Any>
	return [root]
end`,
		"nullable": `def leak(root: Dir?)
	return
end`,
		"mutable alias": `def leak(root: Dir)
	mut handle := root
	return
end`,
		"forged origin": `def leak(value: Any)
	root: Dir := value
	return
end`,
		"native erasure": `def retain(value: Any)
	puts(value)
end
def leak(root: Dir)
	retain(root)
end`,
		"closure": `def leak(root: Dir): () -> Any
	return fn(): Any
		return root
	end
end`,
		"transitive suspension": `def suspend(): Array<Integer>
	return [1].concurrent_map(limit: 1) do |value|
		value
	end
end
def leak(_root: Dir): Array<Integer>
	return suspend()
end`,
	}
	for name, source := range cases {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(name+"/"+mode, func(t *testing.T) {
				_, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Source: []byte(imports + source)}}, Options{Mode: mode})
				if err == nil || !strings.Contains(err.Error(), "scoped resource") {
					t.Fatalf("expected resource rejection, got %v", err)
				}
			})
		}
	}
}

func TestAnchoredDirectoryExecutes(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "sample"), []byte("anchored"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := anchoredDirectorySource + `
def main()
	case RelativePath.parse("sample")
	when Result::Err(_error)
		puts("parse-failed")
	when Result::Ok(child)
		result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
			entries := try list(root)
			puts(entries.size())
			try load(root, child)
		end
		case result
		when Result::Ok(text)
			puts(text)
		when Result::Err(error)
			puts(error.message)
		end
	end
end
`
	source = strings.ReplaceAll(source, "root.open_file(path)", "root.open_file<String>(path)")
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(source)}}, Options{Mode: "go", GoModule: "example.com/anchor"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/anchor")); got != "1\nanchored" {
		t.Fatalf("got %q", got)
	}
}

func TestAnchoredDirectoryImportedBorrowAlias(t *testing.T) {
	helper := SourceUnit{Filename: "count.trb", ModulePath: "support/count", Package: "support", Source: []byte(`import { Dir as Anchor } from trb/std/dir
import { FileSystemError } from trb/std/errors
import trb/std/result

def count(root: Anchor): Result<Integer, FileSystemError>
	borrow: Anchor := root
	entries := try borrow.children(max_entries: 10)
	return Result<Integer, FileSystemError>::Ok(entries.size())
end
`)}
	main := SourceUnit{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(`import trb/std/path
import trb/std/dir
import trb/std/result
import { count } from support/count

def main()
	result := Dir.open(Path.new(` + strconv.Quote(t.TempDir()) + `)) do |root|
		try count(root)
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`)}
	artifacts, err := CompileProject([]SourceUnit{helper, main}, Options{Mode: "go", GoModule: "example.com/anchor"})
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/anchor")); got != "0" {
		t.Fatalf("got %q", got)
	}
}
