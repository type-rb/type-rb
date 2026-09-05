package compiler

import (
	goast "go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	gotypes "go/types"
	"strings"
	"testing"
)

const scopedFilesystemSource = `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import trb/std/dir
import { DirEntry, DirEntryKind } from trb/std/dir
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def read_bounded(path: String, maximum: Integer): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
		try file.read_text(max_bytes: maximum)
	end
end

def create(path: String, value: String): Result<Unit, FileSystemError>
	return File.open(Path.new(path), mode: FileMode::CreateNew) do |file|
		try file.write_text(value)
	end
end

def children(path: String): Result<Array<DirEntry<Path>>, FileSystemError>
	return Dir.children(Path.new(path), max_entries: 1000)
end

def synthetic_entry(name: String, path: String): DirEntry<Path>
	return DirEntry<Path>.new(name: name, path: Path.new(path), kind: DirEntryKind::Other)
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
				"go":         {"defer func()", ".Close()", "os.O_EXCL", "utf8.ValidString(name)", "filepath.VolumeName(path)", "volume == path && len(volume) == 2 && volume[1] == ':'", "os.IsPathSeparator", "childPath += name", "os.Lstat(childPath)", "directory entry name is not valid UTF-8"},
				"ruby":       {"ensure", ".close", "::File::EXCL", "encoding: Encoding::BINARY", "name.valid_encoding?", "native_separator = ::File::ALT_SEPARATOR.nil? ? ::File::SEPARATOR : ::File::ALT_SEPARATOR", `!::File::ALT_SEPARATOR.nil? && path.match?(/\A[A-Za-z]:\z/)`, "path.end_with?(::File::SEPARATOR)", `: native_separator`, "path + separator + name", "active_path = child_path", "::File.lstat(child_path)", "target: FileSystemTarget::Host.new(active_path)", "directory entry name is not valid UTF-8", "while data.bytesize <= max_bytes", "request_bytes = remaining == 0 ? 1 : [65536, remaining].min", "file.read(request_bytes)"},
				"typescript": {"finally", ".closeSync(", ".constants.O_EXCL", `encoding: "buffer"`, `new TextDecoder("utf-8", {fatal: true, ignoreBOM: true})`, `driveRelativeRoot`, `.sep === "\\" && /^[A-Za-z]:$/.test(path)`, `.endsWith(pathModule`, ` + separator`, `.lstatSync(childPath`, `.isFile()`, `.isDirectory()`, `.Host(activePath)`, "directory entry name is not valid UTF-8", "new Uint8Array(65536)", "while (__trbFileReadCount", "=== 0 ? 1 : Math.min(__trbFileReadBuffer", ".readSync(__trbFileReadHandle"},
			}[mode] {
				if !strings.Contains(string(artifact.Output), fragment) {
					t.Fatalf("generated %s resource block is missing %q:\n%s", mode, fragment, artifact.Output)
				}
			}
			for _, forbidden := range map[string][]string{
				"go":         {"filepath.Join(path, sourceEntry.Name())"},
				"ruby":       {"::File.join(path, name)", "file.read(max_bytes + 1)"},
				"typescript": {".join(__trbDirPath", "new Uint8Array(__trbFileReadLimit", "< __trbFileReadBuffer", ".length - __trbFileReadCount"},
			}[mode] {
				if strings.Contains(string(artifact.Output), forbidden) {
					t.Fatalf("generated %s resource block still contains cleaning join %q:\n%s", mode, forbidden, artifact.Output)
				}
			}
		})
	}
}

func TestFilesystemDeclarationOwnersCannotBeUsedAsValuesAcrossPortableBackends(t *testing.T) {
	tests := map[string]struct {
		imports string
		owner   string
	}{
		"File": {
			imports: "import trb/std/file",
			owner:   "File",
		},
		"Dir": {
			imports: "import trb/std/dir",
			owner:   "Dir",
		},
		"FileMode": {
			imports: "import { FileMode } from trb/std/file",
			owner:   "FileMode",
		},
		"DirEntry": {
			imports: "import { DirEntry } from trb/std/dir",
			owner:   "DirEntry",
		},
		"DirEntryKind": {
			imports: "import { DirEntryKind } from trb/std/dir",
			owner:   "DirEntryKind",
		},
		"File alias": {
			imports: "import trb/std/file as HostFile",
			owner:   "HostFile",
		},
		"Dir alias": {
			imports: "import trb/std/dir as HostDir",
			owner:   "HostDir",
		},
		"FileMode alias": {
			imports: "import { FileMode as OpenMode } from trb/std/file",
			owner:   "OpenMode",
		},
		"DirEntry alias": {
			imports: "import { DirEntry as Entry } from trb/std/dir",
			owner:   "Entry",
		},
		"DirEntryKind alias": {
			imports: "import { DirEntryKind as EntryKind } from trb/std/dir",
			owner:   "EntryKind",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, test := range tests {
			t.Run(mode+"/"+name, func(t *testing.T) {
				source := test.imports + `

def consume(_value: Any)
	return
end

def main()
	consume(` + test.owner + `)
	return
end
`
				_, err := Compile("main.trb", []byte(source), mode)
				want := "declaration " + test.owner + " cannot be used as a value"
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("Compile() error = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestFileOpenRejectsFileModeDeclarationAsModeAcrossPortableBackends(t *testing.T) {
	source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file

def open_invalid(path: String)
	File.open(Path.new(path), mode: FileMode) do |_file|
		return
	end
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			_, err := Compile("main.trb", []byte(source), mode)
			const want = "declaration FileMode cannot be used as a value"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("Compile() error = %v, want %q", err, want)
			}
		})
	}
}

func TestScopedFilesystemAliasesCompileAcrossPortableBackends(t *testing.T) {
	source := `import trb/std/path
import trb/std/file as HostFile
import { FileMode as OpenMode } from trb/std/file
import trb/std/dir as HostDir
import { DirEntry as Entry, DirEntryKind as EntryKind } from trb/std/dir
import { FileSystemError as IOError, FileSystemErrorKind as IOErrorKind } from trb/std/errors
import { Result as Outcome } from trb/std/result
import { Unit } from trb/std/unit

def create(path: String, value: String): Outcome<Unit, IOError>
	return HostFile.open(Path.new(path), mode: OpenMode::CreateNew) do |file|
		try file.write_text(value)
	end
end

def children(path: String): Outcome<Array<Entry<Path>>, IOError>
	return HostDir.children(Path.new(path), max_entries: 1000)
end

def entry_label(entry: Entry<Path>): String
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
	source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def read_default(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
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
	source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
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
	return File.open(Path.new(path)) do |file|
		consume(file)
	end
end`,
		"return": `def invalid(path: String): Result<Any, FileSystemError>
	return File.open(Path.new(path)) do |file|
		file
	end
end`,
		"collection": `def invalid(path: String): Result<Array<Any>, FileSystemError>
	return File.open(Path.new(path)) do |file|
		[file]
	end
end`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			source := `import trb/std/path
import trb/std/file
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
				source := "import trb/std/path\nimport trb/std/file\n\n" + body + "\n"
				_, err := Compile("main.trb", []byte(source), mode)
				if err == nil || !strings.Contains(err.Error(), want) {
					t.Fatalf("Compile() error = %v, want %q", err, want)
				}
			})
		}
	}
}

func TestScopedFileImportAliasCannotAppearInAuthoredValueTypes(t *testing.T) {
	source := `import trb/std/path
import { File as HostFile } from trb/std/file

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
	source := `import trb/std/path
import trb/std/file

value := File.new()
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "File cannot be constructed directly; use File.open() with a block") {
		t.Fatalf("Compile() error = %v, want opaque resource diagnostic", err)
	}
}

func TestDirectoryOwnerCannotBeConstructedAsAnEmptyValue(t *testing.T) {
	source := `import trb/std/path
import trb/std/dir

value := Dir.new()
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "Dir cannot be constructed directly; use Dir.children()") {
		t.Fatalf("Compile() error = %v, want opaque resource diagnostic", err)
	}
}

func TestScopedFilesystemHandleCannotBeCaptured(t *testing.T) {
	source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
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

func TestScopedFilesystemRejectsOpaqueRubyNativeSyntaxInResourceScope(t *testing.T) {
	tests := map[string]string{
		"native block": `retain do
			puts("retained")
		end`,
		"native statement":  `puts :opaque`,
		"native expression": `opaque := ` + "`echo opaque`",
	}
	for name, nested := range tests {
		t.Run(name, func(t *testing.T) {
			source := `activate trb/platform/ruby/native
import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
		` + nested + `
		"done"
	end
end
`
			_, err := Compile("main.trb", []byte(source), "ruby")
			if err == nil || !strings.Contains(err.Error(), "Ruby-native syntax cannot be used while a scoped resource is in scope") {
				t.Fatalf("Compile() error = %v, want scoped native-syntax diagnostic", err)
			}
		})
	}
}

func TestScopedFilesystemHandleCannotBeCapturedByIterationBlock(t *testing.T) {
	tests := map[string]string{
		"each": `[1].each do |_value|
			file.read_text(max_bytes: 10) catch |_error|
				"failed"
			end
		end`,
		"concurrent_map": `[1].concurrent_map(limit: 1) do |_value|
			file.read_text(max_bytes: 10) catch |_error|
				"failed"
			end
		end`,
	}
	for name, nested := range tests {
		t.Run(name, func(t *testing.T) {
			source := `import trb/std/path
import trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def invalid(path: String): Result<String, FileSystemError>
	return File.open(Path.new(path)) do |file|
		` + nested + `
		"done"
	end
end
`
			_, err := Compile("main.trb", []byte(source), "go")
			if err == nil || !strings.Contains(err.Error(), "scoped resource file may only be used as a direct method receiver") {
				t.Fatalf("Compile() error = %v, want scoped iteration-block capture diagnostic", err)
			}
		})
	}
}

func TestSuspendingTypeScriptFileBlockUsesAwaitedAsyncCleanupBoundary(t *testing.T) {
	source := SourceUnit{
		Filename: "main.trb", ModulePath: "main", Package: "main",
		Source: []byte(`import trb/std/path
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
`),
	}
	artifacts, err := CompileProject([]SourceUnit{source}, Options{Mode: "typescript"})
	if err != nil {
		t.Fatal(err)
	}
	consumer := findArtifactByModule(artifacts, "main")
	if consumer == nil {
		t.Fatal("missing TypeScript consumer artifact")
	}
	output := string(consumer.Output)
	for _, fragment := range []string{
		`= await (async (): Promise<Result<Unit, FileSystemError>> => {`,
		`await Promise.all`,
		`} finally {`,
		`.closeSync(`,
	} {
		if !strings.Contains(output, fragment) {
			t.Fatalf("generated suspending File.open is missing %q:\n%s", fragment, output)
		}
	}
	checkTypeScriptArtifacts(t, artifacts, "suspending_scoped_file")
}

func TestTypeScriptFileArgumentsAreEvaluatedBeforeHostChecksAndErrorCatch(t *testing.T) {
	source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result
import { Unit } from trb/std/unit

def selected_mode(): FileMode
	puts("mode")
	return FileMode::Write
end

def payload(): String
	puts("payload")
	return "value"
end

def write(path: String): Result<Unit, FileSystemError>
	return File.open(Path.new(path), mode: selected_mode()) do |file|
		try file.write_text(payload())
	end
end
`
	artifact, err := Compile("main.trb", []byte(source), "typescript")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	mode := strings.Index(output, "const __trbFileMode")
	host := strings.Index(output, `const __trbFileSystem`)
	data := strings.Index(output, "const __trbFileWriteData")
	writeTry := strings.Index(output, ".writeFileSync(")
	if mode < 0 || host < 0 || mode > host {
		t.Fatalf("generated File.open does not evaluate mode before the host check:\n%s", output)
	}
	if data < 0 || !strings.Contains(output[data:], "= payload();") || writeTry < 0 || data > writeTry {
		t.Fatalf("generated file.write_text does not evaluate its argument before the filesystem error catcher:\n%s", output)
	}
}

func TestGoUTF8ReplacementRuntimePropagatesFromValueProducingBranch(t *testing.T) {
	source := `def decoded(value: Bytes, enabled: Boolean): String
	result := if enabled
		value.to_s()
	else
		""
	end
	return result
end
`
	artifact, err := Compile("main.trb", []byte(source), "go")
	if err != nil {
		t.Fatal(err)
	}
	output := string(artifact.Output)
	if strings.Count(output, "func trbDecodeUTF8_") != 1 {
		t.Fatalf("generated Go did not emit exactly one UTF-8 replacement helper:\n%s", output)
	}
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, "main.go", artifact.Output, parser.AllErrors)
	if err != nil {
		t.Fatalf("generated invalid Go: %v\n%s", err, output)
	}
	if _, err := (&gotypes.Config{Importer: importer.Default()}).Check("main", fileSet, []*goast.File{parsed}, nil); err != nil {
		t.Fatalf("generated Go failed to type-check: %v\n%s", err, output)
	}
}

func TestScopedFileIdentityDoesNotAffectAnUnrelatedFileDeclaration(t *testing.T) {
	source := `import trb/std/path
import trb/std/file as HostFile
import { File as BrowserFile } from trb/platform/typescript/browser
import { FileSystemError } from trb/std/errors
import { Result } from trb/std/result

def browser_file(): BrowserFile
	return BrowserFile.new(name: "note.txt", size: 5, type: "text/plain", lastModified: 0)
end

def load(path: String): Result<String, FileSystemError>
	return HostFile.open(Path.new(path)) do |file|
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
