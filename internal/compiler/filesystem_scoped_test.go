package compiler

import (
	"strings"
	"testing"
)

const scopedFilesystemSource = `import trb/std/file
import { FileMode } from trb/std/file
import trb/std/dir
import { DirEntry } from trb/std/dir
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def read_bounded(path: String, maximum: Integer): Result<String, FileSystemError>
	return File.open(path) do |file|
		try file.read_text(max_bytes: maximum)
	end
end

def create(path: String, value: String): Result<Unit, FileSystemError>
	return File.open(path, mode: FileMode::CreateNew) do |file|
		try file.write_text(value)
	end
end

def children(path: String): Result<Array<DirEntry>, FileSystemError>
	return Dir.children(path)
end
`

func TestScopedFilesystemCompilesAcrossPortableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("main.trb", []byte(scopedFilesystemSource), mode)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			for _, fragment := range map[string][]string{
				"go":         {"defer func()", ".Close()", "os.O_EXCL", "utf8.ValidString(sourceEntry.Name())", "filepath.VolumeName(path)", "volume == path && len(volume) == 2 && volume[1] == ':'", "os.IsPathSeparator", "childPath += name", "sourceEntry.Info()", "directory entry name is not valid UTF-8"},
				"ruby":       {"ensure", ".close", `"wbx"`, "encoding: Encoding::BINARY", "name.valid_encoding?", `!::File::ALT_SEPARATOR.nil? && path.match?(/\A[A-Za-z]:\z/)`, "path.end_with?(::File::SEPARATOR)", "path + separator + name", "active_path = child_path", "::File.lstat(child_path)", "path: active_path", "directory entry name is not valid UTF-8"},
				"typescript": {"finally", "fs.closeSync", `"wx"`, `encoding: "buffer"`, `new TextDecoder("utf-8", { fatal: true })`, `pathModule.sep === "\\" && /^[A-Za-z]:$/.test(__trbPath)`, `__trbPath.endsWith(pathModule.sep)`, "__trbPath + separator + name", "fs.lstatSync(childPath)", "info.isFile()", "info.isDirectory()", "path: activePath", "directory entry name is not valid UTF-8", "while (count < buffer.length)", "buffer.length - count"},
			}[mode] {
				if !strings.Contains(string(artifact.Output), fragment) {
					t.Fatalf("generated %s resource block is missing %q:\n%s", mode, fragment, artifact.Output)
				}
			}
			for _, forbidden := range map[string][]string{
				"go":         {"filepath.Join(path, sourceEntry.Name())"},
				"ruby":       {"::File.join(path, name)"},
				"typescript": {"pathModule.join(__trbPath, entry.name)"},
			}[mode] {
				if strings.Contains(string(artifact.Output), forbidden) {
					t.Fatalf("generated %s resource block still contains cleaning join %q:\n%s", mode, forbidden, artifact.Output)
				}
			}
		})
	}
}

func TestScopedFilesystemAliasesCompileAcrossPortableBackends(t *testing.T) {
	source := `import trb/std/file as HostFile
import { FileMode as OpenMode } from trb/std/file
import trb/std/dir as HostDir
import { DirEntry as Entry, DirEntryKind as EntryKind } from trb/std/dir
import { FileSystemError as IOError, FileSystemErrorKind as IOErrorKind } from trb/std/errors
import { Result as Outcome } from trb/std/result
import { Unit } from trb/std/unit

def create(path: String, value: String): Outcome<Unit, IOError>
	return HostFile.open(path, mode: OpenMode::CreateNew) do |file|
		try file.write_text(value)
	end
end

def children(path: String): Outcome<Array<Entry>, IOError>
	return HostDir.children(path)
end

def entry_label(entry: Entry): String
	case entry.kind
	when EntryKind::File
		return "file"
	when EntryKind::Directory
		return "directory"
	when EntryKind::Other
		return "other"
	end
end

def already_exists(error: IOError): Boolean
	return error.kind == IOErrorKind::AlreadyExists
end
`
	expected := map[string][]string{
		"go":         {"file.FileModeCreatenew", "dir.DirEntryKindFile", "errors.FileSystemErrorKindAlreadyexists", "__trb_result.Result[", "errors.FileSystemError"},
		"ruby":       {"FileMode::CreateNew", "DirEntryKind::File", "FileSystemErrorKind::AlreadyExists", "Result::"},
		"typescript": {"FileMode as OpenMode", "DirEntry as Entry", "DirEntryKind as EntryKind", "FileSystemError as IOError", "FileSystemErrorKind as IOErrorKind", "Result as Outcome", "OpenMode.CreateNew", "EntryKind.File", "IOErrorKind.AlreadyExists", "Outcome<", "Outcome.Ok", "Outcome.Err"},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			artifact, err := Compile("main.trb", []byte(source), mode)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}
			for _, fragment := range expected[mode] {
				if !strings.Contains(string(artifact.Output), fragment) {
					t.Fatalf("generated %s alias output is missing %q:\n%s", mode, fragment, artifact.Output)
				}
			}
		})
	}
}

func TestScopedFilesystemDefaultModeImportsItsRuntimeDeclaration(t *testing.T) {
	source := `import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read_default(path: String): Result<String, FileSystemError>
	return File.open(path) do |file|
		try file.read_text(max_bytes: 16)
	end
end
`
	artifact, err := Compile("main.trb", []byte(source), "typescript")
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	output := string(artifact.Output)
	for _, fragment := range []string{"FileMode", ".Read", `from "./trb/std/file/index.ts"`} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated TypeScript default File.open is missing %q:\n%s", fragment, output)
		}
	}
}

func TestScopedFilesystemHandleCannotEscape(t *testing.T) {
	source := `import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(path) do |file|
		copy := file
		try copy.read_text(max_bytes: 10)
	end
end
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "scoped resource file may only be used as a direct method receiver") {
		t.Fatalf("Compile() error = %v, want scoped resource diagnostic", err)
	}
}

func TestScopedFilesystemHandleRejectsPassingReturningAndCollectionStorage(t *testing.T) {
	tests := map[string]string{
		"pass": `def consume(_value: Any): String
	return ""
end

def invalid(path: String): Result<String, FileSystemError>
	return File.open(path) do |file|
		consume(file)
	end
end`,
		"return": `def invalid(path: String): Result<Any, FileSystemError>
	return File.open(path) do |file|
		file
	end
end`,
		"collection": `def invalid(path: String): Result<Array<Any>, FileSystemError>
	return File.open(path) do |file|
		[file]
	end
end`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

` + body + "\n"
			_, err := Compile("main.trb", []byte(source), "go")
			if err == nil || !strings.Contains(err.Error(), "scoped resource file may only be used as a direct method receiver") {
				t.Fatalf("Compile() error = %v, want scoped escape diagnostic", err)
			}
		})
	}
}

func TestScopedFileCannotAppearInAuthoredValueTypes(t *testing.T) {
	tests := map[string]string{
		"parameter": `def invalid(_value: File)
	return
end`,
		"return": `def invalid(): File
	return File.new()
end`,
		"class field": `class Holder
	@file: File
end`,
		"record field": `record Holder
	file: File
end`,
		"collection": `def invalid(_values: Array<File>)
	return
end`,
		"function": `def invalid(_callback: (File) -> String)
	return
end`,
		"transparent alias": `alias SavedFile = File`,
	}
	want := "scoped File may only be introduced as the File.open() block parameter; it cannot appear in an authored value type"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, body := range tests {
			t.Run(mode+"/"+name, func(t *testing.T) {
				source := "import trb/std/file\n\n" + body + "\n"
				_, err := Compile("main.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("Compile() error = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestScopedFileImportAliasCannotAppearInAuthoredValueTypes(t *testing.T) {
	source := `import { File as HostFile } from trb/std/file

def invalid(_value: HostFile)
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("main.trb", []byte(source), mode)
			if err == nil || !strings.Contains(err.Error(), "scoped File may only be introduced as the File.open() block parameter") {
				t.Fatalf("Compile() error = %v, want scoped File type diagnostic", err)
			}
		})
	}
}

func TestCompilerGeneratedDeclarationCannotMintScopedFile(t *testing.T) {
	unit := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`def main()
	return
end
`),
		CompilerGeneratedSources: []CompilerGeneratedSource{{
			ID: "generated-resource", Source: []byte(`import { File } from trb/std/file

def generated_file(value: File): File
	return value
end
`),
		}},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := CompileProject([]SourceUnit{unit}, Options{Mode: mode, GoModule: "example.com/scoped-file-origin", SourceRoot: "/project", ProjectRoot: "/project"})
			if err == nil || !strings.Contains(err.Error(), "scoped File may only be introduced as the File.open() block parameter") {
				t.Fatalf("CompileProject() error = %v, want generated File origin diagnostic", err)
			}
		})
	}
}

func TestScopedFilesystemHandleCannotBeConstructed(t *testing.T) {
	source := `import trb/std/file

value := File.new()
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "File cannot be constructed directly; use File.open() with a block") {
		t.Fatalf("Compile() error = %v, want opaque resource diagnostic", err)
	}
}

func TestDirectoryOwnerCannotBeConstructedAsAnEmptyValue(t *testing.T) {
	source := `import trb/std/dir

value := Dir.new()
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "Dir cannot be constructed directly; use Dir.children()") {
		t.Fatalf("Compile() error = %v, want opaque resource diagnostic", err)
	}
}

func TestScopedFilesystemHandleCannotBeCaptured(t *testing.T) {
	source := `import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(path) do |file|
		callback := fn(): Result<String, FileSystemError>
			return file.read_text(max_bytes: 10)
		end
		try callback()
	end
end
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "scoped resource file may only be used as a direct method receiver") {
		t.Fatalf("Compile() error = %v, want scoped capture diagnostic", err)
	}
}

func TestScopedFileIdentityDoesNotAffectAnUnrelatedFileDeclaration(t *testing.T) {
	source := `import trb/std/file as HostFile
import { File as BrowserFile } from trb/platform/typescript/browser
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def browser_file(): BrowserFile
	return BrowserFile.new(name: "note.txt", size: 5, type: "text/plain", lastModified: 0)
end

def load(path: String): Result<String, FileSystemError>
	return HostFile.open(path) do |file|
		try file.read_text(max_bytes: 5)
	end
end
`
	if _, err := Compile("main.trb", []byte(source), "typescript"); err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
}

func TestUnrelatedFileDoesNotReceiveScopedFileMethods(t *testing.T) {
	source := `import { File as BrowserFile } from trb/platform/typescript/browser

value := BrowserFile.new(name: "note.txt", size: 5, type: "text/plain", lastModified: 0)
value.write_text("hello")
`
	_, err := Compile("main.trb", []byte(source), "typescript")
	if err == nil || !strings.Contains(err.Error(), "has no member write_text") {
		t.Fatalf("Compile() error = %v, want unrelated File receiver diagnostic", err)
	}
}
