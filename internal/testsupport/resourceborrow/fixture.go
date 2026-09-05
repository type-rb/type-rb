// Package resourceborrow supplies one scoped-borrow scenario to emitted
// runtimes and the typed-IR evaluator.
package resourceborrow

import (
	"path/filepath"
	"strconv"
	"testing"
)

func Scenario(t *testing.T) (declarations, expression, expected string) {
	t.Helper()
	root := t.TempDir()
	declarations = `import trb/std/path
import { File as HostFile, FileMode } from trb/std/file
import { Result } from trb/std/result
import { FileSystemError } from trb/std/errors
import { Unit } from trb/std/unit

def append(file: HostFile, text: String): Result<Unit, FileSystemError>
	handle: HostFile := file
	return handle.write_text(text)
end

def forward(file: HostFile, text: String): Result<Unit, FileSystemError>
	return append(file, text)
end

def contents(file: HostFile): Result<String, FileSystemError>
	return file.read_text(max_bytes: 100)
end

def read(path: Path): Result<String, FileSystemError>
	return HostFile.open(path) do |file|
		try contents(file)
	end
end

def execute(outer: Path, inner: Path): Result<String, FileSystemError>
	_created := try HostFile.open(outer, mode: FileMode::CreateNew) do |file|
		try forward(file, "a")
		_nested := try HostFile.open(inner, mode: FileMode::CreateNew) do |nested|
			try forward(file, "b")
			try append(nested, "inner")
		end
		["c", "d"].each do |part|
			try append(file, part)
		end
		try forward(file, "e")
	end
	first := try read(outer)
	second := try read(inner)
	return Result<String, FileSystemError>::Ok(first + ":" + second)
end

def scenario(): String
	return execute(Path.new(` + strconv.Quote(filepath.Join(root, "outer")) + `), Path.new(` + strconv.Quote(filepath.Join(root, "inner")) + `)) catch |error|
		error.operation + ":" + error.message
	end
end
`
	return declarations, "scenario()", "abcde:inner"
}
