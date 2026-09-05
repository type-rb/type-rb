package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/type-rb/type-rb/internal/testsupport/regularfile"
)

func TestRunRegularFileAcquisitionAcrossBackends(t *testing.T) {
	for _, backend := range dirBoundaryBackends {
		t.Run(backend.name, func(t *testing.T) {
			requireDirBoundaryRuntime(t, backend)
			declarations, expression, expected := regularfile.Scenario(t)
			source := declarations + "\ndef main()\n\tputs(" + expression + ")\nend\n"
			if got := runDirBoundaryProject(t, backend, source); got != expected+"\n" {
				t.Fatalf("got %q, want %q", got, expected)
			}
		})
	}
}

func TestRunRegularFileAcquisitionFailureCleanup(t *testing.T) {
	for _, backend := range dirBoundaryBackends {
		if backend.name != "ruby" && backend.name != "typescript-node" {
			continue
		}
		for _, failure := range []string{"metadata", "truncate", "borrow-body", "borrow-close"} {
			t.Run(backend.name+"/"+failure, func(t *testing.T) {
				requireDirBoundaryRuntime(t, backend)
				root := t.TempDir()
				path, trace := filepath.Join(root, "input"), filepath.Join(root, "closed")
				if err := os.WriteFile(path, []byte("unchanged"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("TRB_FILE_TEST_TARGET", path)
				t.Setenv("TRB_FILE_TEST_TRACE", trace)
				t.Setenv("TRB_FILE_TEST_FAILURE", failure)
				hook := `const fs = require("node:fs");
const opened = new Set();
const open = fs.openSync, stat = fs.fstatSync, truncate = fs.ftruncateSync, close = fs.closeSync;
fs.openSync = function(path, ...args) {
  const fd = open.call(this, path, ...args);
  if (path === process.env.TRB_FILE_TEST_TARGET) opened.add(fd);
  return fd;
};
fs.fstatSync = function(fd, ...args) {
  if (opened.has(fd) && process.env.TRB_FILE_TEST_FAILURE === "metadata") throw new Error("metadata-fault");
  return stat.call(this, fd, ...args);
};
fs.ftruncateSync = function(fd, ...args) {
  if (opened.has(fd) && process.env.TRB_FILE_TEST_FAILURE === "truncate") throw new Error("truncate-fault");
  return truncate.call(this, fd, ...args);
};
fs.closeSync = function(fd) {
  const tracked = opened.delete(fd);
  const result = close.call(this, fd);
  if (tracked) {
    fs.writeFileSync(process.env.TRB_FILE_TEST_TRACE, "closed");
    throw new Error("close-fault");
  }
  return result;
};
`
				extension, option := ".cjs", "NODE_OPTIONS"
				if backend.mode == "ruby" {
					extension, option = ".rb", "RUBYOPT"
					hook = `File.singleton_class.prepend(Module.new do
  def open(path, *args, **options, &block)
    file = super
    if path == ENV["TRB_FILE_TEST_TARGET"]
      file.define_singleton_method(:stat) { raise IOError, "metadata-fault" } if ENV["TRB_FILE_TEST_FAILURE"] == "metadata"
      file.define_singleton_method(:truncate) { |_size| raise IOError, "truncate-fault" } if ENV["TRB_FILE_TEST_FAILURE"] == "truncate"
      file.define_singleton_method(:close) do
        super()
        File.write(ENV["TRB_FILE_TEST_TRACE"], "closed")
        raise IOError, "close-fault"
      end
    end
    file
  end
end)
`
				}
				hookPath := filepath.Join(root, "hook"+extension)
				if err := os.WriteFile(hookPath, []byte(hook), 0o600); err != nil {
					t.Fatal(err)
				}
				if option == "NODE_OPTIONS" {
					t.Setenv(option, "--require="+strconv.Quote(hookPath))
				} else {
					t.Setenv(option, "-r"+hookPath)
				}
				source := `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import trb/std/result
def main()
	result := File.open(Path.new(` + strconv.Quote(path) + `), mode: FileMode::Write) do |_file|
		"unexpected-body"
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.operation + ":" + error.message)
	end
end
`
				want := "open:" + failure + "-fault\n"
				if failure == "borrow-body" || failure == "borrow-close" {
					limit := "100"
					want = "close\n"
					if failure == "borrow-body" {
						limit = "-1"
						want = "read_text\n"
					}
					source = `import trb/std/path
import trb/std/file
import { Result } from trb/std/result
import { FileSystemError } from trb/std/errors
def read(file: File): Result<String, FileSystemError>
	handle := file
	return handle.read_text(max_bytes: ` + limit + `)
end
def main()
	result := File.open(Path.new(` + strconv.Quote(path) + `)) do |file|
		try read(file)
	end
	case result
	when Result::Ok(value)
		puts(value)
	when Result::Err(error)
		puts(error.operation)
	end
end
`
				}
				if got := runDirBoundaryProject(t, backend, source); got != want {
					t.Fatalf("acquisition failure must win over close failure: %q", got)
				}
				if closed, err := os.ReadFile(trace); err != nil || string(closed) != "closed" {
					t.Fatalf("failed acquisition did not close its handle: %q, %v", closed, err)
				}
				if data, err := os.ReadFile(path); err != nil || string(data) != "unchanged" {
					t.Fatalf("file changed before successful validation/truncation: %q, %v", data, err)
				}
			})
		}
	}
}
