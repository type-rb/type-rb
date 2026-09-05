package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestRunPathTypedBoundedFilesystemAcrossBackends(t *testing.T) {
	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			root := t.TempDir()
			for name, content := range map[string][]byte{
				"a": []byte("abc"), "z": {}, "invalid": {0xc0, 0xaf},
				"bom": {0xef, 0xbb, 0xbf, 'x'}, "\ufeffname": {},
			} {
				if err := os.WriteFile(filepath.Join(root, name), content, 0o600); err != nil {
					t.Fatal(err)
				}
			}
			source := `import { Path, RelativePath } from trb/std/path
import trb/std/file
import trb/std/dir
import { FileSystemError, FileSystemErrorKind, FileSystemTarget } from trb/std/errors
import trb/std/result

def error_label(error: FileSystemError, expected: Path): String
	correct := case error.target
	when FileSystemTarget::Host(path)
		path == expected
	else
		false
	end
	if !correct
		return "wrong-target"
	end
	case error.kind
	when FileSystemErrorKind::NotFound
		return "missing"
	when FileSystemErrorKind::InvalidLimit
		return "limit"
	when FileSystemErrorKind::InvalidPath
		return "path"
	when FileSystemErrorKind::TooLarge
		return "large"
	when FileSystemErrorKind::InvalidEncoding
		return "encoding"
	else
		return "other"
	end
end

def listing(path: Path, maximum: Integer): String
	case Dir.children(path, max_entries: maximum)
	when Result::Ok(entries)
		return entries.size().to_s()
	when Result::Err(error)
		return error_label(error, path)
	end
end

def reading(path: Path, maximum: Integer): String
	result := File.open(path) do |file|
		try file.read_text(max_bytes: maximum)
	end
	case result
	when Result::Ok(text)
		return "text:" + text
	when Result::Err(error)
		return error_label(error, path)
	end
end

def creating(path: Path): String
	case Dir.create_all(path)
	when Result::Ok(_unit)
		return "created"
	when Result::Err(error)
		return error.operation + ":" + error_label(error, path)
	end
end

def scenario(root: Path)
	child := RelativePath.parse("new/year/month") catch |_error|
		return
	end
	nested := root.join(child)
	puts(creating(nested))
	puts(creating(nested))
	puts(listing(nested, 0))
	puts(listing(root, -1))
	puts(listing(root, 5))
	puts(listing(root, 6))
	puts(listing(Path.new(""), 1))
	puts(creating(Path.new("")))
	puts(creating(Path.new(".")))
	puts(creating(Path.new("/")))
	entries := Dir.children(root, max_entries: 6) catch |_error|
		return
	end
	entries.each do |entry|
		puts(entry.name)
	end
	puts(reading(Path.new(root.to_s() + "/missing"), 10))
	puts(reading(Path.new(root.to_s() + "/a"), -1))
	puts(reading(Path.new(root.to_s() + "/a"), 0))
	puts(reading(Path.new(root.to_s() + "/z"), 0))
	puts(reading(Path.new(root.to_s() + "/a"), 3))
	puts(reading(Path.new(root.to_s() + "/a"), 2))
	puts(reading(Path.new(root.to_s() + "/invalid"), 1))
	puts(reading(Path.new(root.to_s() + "/invalid"), 2))
	puts(reading(Path.new(root.to_s() + "/bom"), 4))
	puts(reading(Path.new(""), 10))
	puts(listing(Path.new(root.to_s() + "/missing"), 1))
	puts(listing(Path.new("bad\u0000path"), 1))
	puts(creating(Path.new("bad\u0000path")))
	puts(reading(Path.new("bad\u0000path"), 1))
end

def main()
	scenario(Path.new(` + strconv.Quote(root) + `))
end
`
			want := "created\ncreated\n0\nlimit\nlarge\n6\npath\ncreate_all:path\ncreated\ncreated\na\nbom\ninvalid\nnew\nz\n\ufeffname\nmissing\nlimit\nlarge\ntext:\ntext:abc\nlarge\nlarge\nencoding\ntext:\ufeffx\npath\nmissing\npath\ncreate_all:path\npath\n"
			if got := runDirBoundaryProject(t, backend, source); got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}

func TestRunCreateAllPreservesHostResolutionAcrossBackends(t *testing.T) {
	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, "physical", "pivot"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "physical", "pivot"), filepath.Join(root, "route")); err != nil {
				t.Skip(err)
			}
			path := filepath.Join(root, "route") + "/../created/leaf"
			source := `import trb/std/path
import trb/std/dir
import trb/std/result
def main()
	case Dir.create_all(Path.new(` + strconv.Quote(path) + `))
	when Result::Ok(_unit)
		puts("ok")
	when Result::Err(error)
		puts(error.message)
	end
end
`
			if got := runDirBoundaryProject(t, backend, source); got != "ok\n" {
				t.Fatal(got)
			}
			if _, err := os.Stat(filepath.Join(root, "physical", "created", "leaf")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(root, "created")); !os.IsNotExist(err) {
				t.Fatalf("path was cleaned: %v", err)
			}
			info, err := os.Stat(filepath.Join(root, "physical", "pivot"))
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o700 {
				t.Fatalf("existing permissions changed: %v", info.Mode())
			}
		})
	}
}
