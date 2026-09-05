package compiler

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

const anchoredSecuritySource = `import trb/std/path
import { RelativePath } from trb/std/path
import trb/std/dir
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemError, FileSystemTarget, FileSystemErrorKind } from trb/std/errors
import trb/std/result

def error_label(error: FileSystemError, name: String): String
	case error.target
	when FileSystemTarget::Relative(path)
		if path.to_s() != name
			return "wrong-relative-target"
		end
	when FileSystemTarget::Root
		if name != ""
			return "wrong-root-target"
		end
	else
		return "leaked-host-target"
	end
	if error.message.include?("/")
		return "leaked-native-path"
	end
	return "error"
end

def read(root: Dir, name: String, mode: FileMode): String
	case RelativePath.parse(name)
	when Result::Err(_error)
		return "invalid-test-path"
	when Result::Ok(path)
		result := root.open_file(path, mode: mode) do |file|
			try file.read_text(max_bytes: 100)
		end
		case result
		when Result::Ok(value)
			return value
		when Result::Err(error)
			return error_label(error, name)
		end
	end
end

def list(root: Dir, path: RelativePath?, maximum: Integer): String
	case root.children(path, max_entries: maximum)
	when Result::Ok(entries)
		return entries.map do |entry|
			entry.path.to_s()
		end.join(",")
	when Result::Err(error)
		if error.kind == FileSystemErrorKind::TooLarge
			return "too-large"
		end
		if error.kind == FileSystemErrorKind::UnsupportedName
			return "unsupported-name"
		end
		if error.kind == FileSystemErrorKind::InvalidLimit
			return "invalid-limit"
		end
		return "error"
	end
end
`

func runAnchoredSecuritySource(t *testing.T, source string, transform func(string) string) string {
	t.Helper()
	artifacts, err := CompileProject([]SourceUnit{{Filename: "main.trb", ModulePath: "main", Package: "main", Source: []byte(anchoredSecuritySource + source)}}, Options{Mode: "go", GoModule: "example.com/anchor", AllowUnusedImports: true})
	if err != nil {
		t.Fatal(err)
	}
	if transform != nil {
		main := artifactForModule(artifacts, "main")
		main.Output = []byte(transform(string(main.Output)))
	}
	return strings.TrimSpace(runEffectProject(t, "go", artifacts, "example.com/anchor"))
}

func TestAnchoredDirectoryContainmentAndErrorDomain(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "anchor")
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{filepath.Join(root, "sample"): "inside", filepath.Join(parent, "outside"): "outside"} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for name, target := range map[string]string{"internal": "sample", "escape": "../outside", "absolute": filepath.Join(root, "sample"), "dangling": "uncreated"} {
		if err := os.Symlink(target, filepath.Join(root, name)); err != nil {
			t.Fatal(err)
		}
	}
	source := `
def main()
	result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
		puts(read(root, "sample", FileMode::Read))
		puts(read(root, "internal", FileMode::Read))
		puts(read(root, "escape", FileMode::Read))
		puts(read(root, "absolute", FileMode::Read))
		puts(read(root, "dangling", FileMode::Read))
		puts(read(root, "dangling", FileMode::Write))
		puts(read(root, "sub", FileMode::Read))
		puts(read(root, "missing", FileMode::Read))
		puts(read(root, "sample", FileMode::CreateNew))
		puts(list(root, nil, 0))
		puts(list(root, nil, -1))
		"done"
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`
	want := "inside\ninside\nerror\nerror\nerror\nerror\nerror\nerror\nerror\ntoo-large\ninvalid-limit\ndone"
	if got := runAnchoredSecuritySource(t, source, nil); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if _, err := os.Lstat(filepath.Join(root, "uncreated")); !os.IsNotExist(err) {
		t.Fatalf("dangling write created its target: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(parent, "outside")); err != nil || string(data) != "outside" {
		t.Fatalf("outside changed: %q, %v", data, err)
	}
}

func TestAnchoredDirectoryFollowsOpenedAnchorAfterRename(t *testing.T) {
	parent := t.TempDir()
	root, moved := filepath.Join(parent, "root"), filepath.Join(parent, "moved")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample"), []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	source := `
def main()
	result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
		puts("rename-checkpoint")
		puts(list(root, nil, 10))
		read(root, "sample", FileMode::Read)
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`
	got := runAnchoredSecuritySource(t, source, func(output string) string {
		marker := `fmt.Println("rename-checkpoint")`
		if strings.Count(output, marker) != 1 {
			t.Fatal("missing acquisition checkpoint")
		}
		return strings.Replace(output, marker, `if err := os.Rename(`+strconv.Quote(root)+`, `+strconv.Quote(moved)+`); err != nil { panic(err) }; if err := os.Mkdir(`+strconv.Quote(root)+`, 0700); err != nil { panic(err) }; if err := os.WriteFile(`+strconv.Quote(filepath.Join(root, "sample"))+`, []byte("replacement"), 0600); err != nil { panic(err) }`, 1)
	})
	if got != "sample\noriginal" {
		t.Fatalf("got %q", got)
	}
}

func TestAnchoredDirectoryListingAndCreation(t *testing.T) {
	root := t.TempDir()
	source := `
def main()
	case RelativePath.parse("a/b")
	when Result::Err(_error)
		puts("parse-failed")
	when Result::Ok(child)
		result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
			try root.create_all(child)
			try root.create_all(child)
			puts(list(root, child, 10))
			case RelativePath.parse("a")
			when Result::Err(_error)
				"parse-failed"
			when Result::Ok(parent)
				entries := try root.children(parent, max_entries: 10)
				entries.map do |entry|
					entry.path.to_s()
				end.join(",")
			end
		end
		case result
		when Result::Ok(value)
			puts(value)
		when Result::Err(error)
			puts(error.message)
		end
	end
end
`
	if got := runAnchoredSecuritySource(t, source, nil); got != "a/b" {
		t.Fatalf("got %q", got)
	}
	info, err := os.Stat(filepath.Join(root, "a", "b"))
	if err != nil || !info.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
}

func TestAnchoredDirectoryRejectsNonportableNames(t *testing.T) {
	for _, name := range []string{"CON", "file:stream", "trailing.", "trailing ", "a\\b", string([]byte{0xff})} {
		t.Run(strconv.Quote(name), func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, name), nil, 0o600); err != nil {
				if !utf8.ValidString(name) {
					t.Skipf("host filesystem rejects non-UTF-8 names: %v", err)
				}
				t.Fatal(err)
			}
			source := `
def main()
	result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
		list(root, nil, 10)
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`
			if got := runAnchoredSecuritySource(t, source, nil); got != "unsupported-name" {
				t.Fatalf("got %q", got)
			}
		})
	}
}

func TestAnchoredDirectoryRejectsConcurrentEscape(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "root")
	pivot := filepath.Join(root, "pivot")
	stash := filepath.Join(root, "stash")
	outside := filepath.Join(parent, "outside")
	for _, dir := range []string{pivot, outside} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for dir, content := range map[string]string{pivot: "inside", outside: "outside"} {
		if err := os.WriteFile(filepath.Join(dir, "sample"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	source := `
def probe(root: Dir): String
	mut index := 0
	while index < 1000
		value := read(root, "pivot/sample", FileMode::Read)
		if value != "inside" && value != "error"
			return "escaped:" + value
		end
		index = index + 1
	end
	return "contained"
end

def main()
	result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
		puts("race-checkpoint")
		probe(root)
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.message)
	end
end
`
	got := runAnchoredSecuritySource(t, source, func(output string) string {
		marker := `fmt.Println("race-checkpoint")`
		if strings.Count(output, marker) != 1 {
			t.Fatal("missing race checkpoint")
		}
		injection := `stop := make(chan struct{}); done := make(chan struct{}); go func() { defer close(done); for { select { case <-stop: return; default: }; if os.Rename(` + strconv.Quote(pivot) + `, ` + strconv.Quote(stash) + `) == nil { _ = os.Symlink("../outside", ` + strconv.Quote(pivot) + `); _ = os.Remove(` + strconv.Quote(pivot) + `); _ = os.Rename(` + strconv.Quote(stash) + `, ` + strconv.Quote(pivot) + `) } } }(); defer func() { close(stop); <-done }()`
		return strings.Replace(output, marker, injection, 1)
	})
	if got != "contained" {
		t.Fatalf("got %q", got)
	}
}

func TestAnchoredDirectoryReportedCloseErrorPrecedence(t *testing.T) {
	root := t.TempDir()
	for _, bodyError := range []bool{false, true} {
		name, body, want := "successful-body", "_entries := try root.children(max_entries: 10)\n\t\t\"body-success\"", "close"
		if bodyError {
			name, body, want = "failed-body", "try root.children(max_entries: -1)\n\t\t\"unexpected\"", "children"
		}
		t.Run(name, func(t *testing.T) {
			source := `
def main()
	result := Dir.open(Path.new(` + strconv.Quote(root) + `)) do |root|
		` + body + `
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.operation)
	end
end
`
			got := runAnchoredSecuritySource(t, source, func(output string) string {
				// os.Root.Close is idempotent; double-close is not fault injection.
				// Inject an adapter-reported failure after actually closing it.
				pattern := regexp.MustCompile(`(__trbFileHandle[0-9]+)\.Close\(\)`)
				if !pattern.MatchString(output) {
					t.Fatal("missing close boundary")
				}
				return pattern.ReplaceAllString(output, `func() error { _ = $1.Close(); return os.ErrPermission }()`)
			})
			if got != want {
				t.Fatalf("got %q, want %q", got, want)
			}
		})
	}
}
