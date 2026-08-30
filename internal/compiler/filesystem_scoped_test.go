package compiler

import (
	"strings"
	"testing"
)

const scopedFilesystemSource = `import trb/std/filesystem
import trb/std/result
import trb/std/unit

def read_bounded(path: String, maximum: Integer): Result<String, FileSystem::Error>
	return FileSystem.open(path, mode: FileSystem::OpenMode::Read) do |file|
		try file.read_text(max_bytes: maximum)
	end
end

def create(path: String, value: String): Result<Unit, FileSystem::Error>
	return FileSystem.open(path, mode: FileSystem::OpenMode::CreateNew) do |file|
		try file.write_text(value)
	end
end

def names(path: String): Result<Array<FileSystem::DirectoryEntry>, FileSystem::Error>
	return FileSystem.entries(path)
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
				"go":         {"defer func()", ".Close()"},
				"ruby":       {"ensure", ".close"},
				"typescript": {"finally", "fs.closeSync"},
			}[mode] {
				if !strings.Contains(string(artifact.Output), fragment) {
					t.Fatalf("generated %s resource block is missing %q:\n%s", mode, fragment, artifact.Output)
				}
			}
		})
	}
}

func TestScopedFilesystemHandleCannotEscape(t *testing.T) {
	source := `import trb/std/filesystem
import trb/std/result

def invalid(path: String): Result<String, FileSystem::Error>
	return FileSystem.open(path, mode: FileSystem::OpenMode::Read) do |file|
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

func TestScopedFilesystemHandleCannotBeConstructed(t *testing.T) {
	source := `import trb/std/filesystem

value := FileSystem::File.new()
`
	_, err := Compile("main.trb", []byte(source), "go")
	if err == nil || !strings.Contains(err.Error(), "FileSystem::File is an opaque scoped resource") {
		t.Fatalf("Compile() error = %v, want opaque resource diagnostic", err)
	}
}

func TestScopedFilesystemHandleCannotBeCaptured(t *testing.T) {
	source := `import trb/std/filesystem
import trb/std/result

def invalid(path: String): Result<String, FileSystem::Error>
	return FileSystem.open(path, mode: FileSystem::OpenMode::Read) do |file|
		callback := fn(): Result<String, FileSystem::Error>
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
