// Package regularfile supplies the same adversarial file-acquisition scenario
// to compiled runtime and typed-IR evaluator tests.
package regularfile

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func Scenario(t *testing.T) (declarations, expression, expected string) {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("regular-file acquisition currently supports Linux and macOS")
	}
	root := t.TempDir()
	regular := filepath.Join(root, "regular")
	if err := os.WriteFile(regular, []byte("preserve until Write"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	dangling := filepath.Join(root, "dangling")
	fifo := filepath.Join(root, "fifo")
	fifoLink := filepath.Join(root, "fifo-link")
	for from, to := range map[string]string{link: regular, dangling: filepath.Join(root, "missing"), fifoLink: fifo} {
		if err := os.Symlink(to, from); err != nil {
			t.Fatal(err)
		}
	}
	if output, err := exec.Command("mkfifo", fifo).CombinedOutput(); err != nil {
		t.Fatalf("mkfifo: %v: %s", err, output)
	}
	// Release an accidentally blocking FIFO open so a regression fails instead
	// of leaving a target process hung. Correct acquisitions need no peer.
	stop, finished := make(chan struct{}), make(chan struct{})
	watchdog := time.AfterFunc(90*time.Second, func() {
		defer close(finished)
		t.Error("file acquisition did not finish without a FIFO peer")
		peer, err := os.OpenFile(fifo, os.O_RDWR|syscall.O_NONBLOCK, 0)
		if err == nil {
			defer peer.Close()
		}
		<-stop
	})
	t.Cleanup(func() {
		close(stop)
		if !watchdog.Stop() {
			<-finished
		}
	})
	declarations = `import trb/std/path
import trb/std/file
import { FileMode } from trb/std/file
import { FileSystemErrorKind, FileSystemTarget } from trb/std/errors
import trb/std/result

def opened(path: Path, mode: FileMode): String
	result := File.open(path, mode: mode) do |_file|
		"body"
	end
	case result
	when Result::Ok(value)
		return value
	when Result::Err(error)
		correct := case error.target
		when FileSystemTarget::Host(target)
			target == path
		else
			false
		end
		if !correct || error.operation != "open"
			return "wrong-error-target"
		end
		case error.kind
		when FileSystemErrorKind::AlreadyExists
			return "exists"
		when FileSystemErrorKind::NotFound
			return "missing"
		when FileSystemErrorKind::Other
			return "other"
		else
			return "wrong-kind"
		end
	end
end
`
	var calls, labels []string
	add := func(path, mode, label string) {
		calls = append(calls, "opened(Path.new("+strconv.Quote(path)+"), FileMode::"+mode+")")
		labels = append(labels, label)
	}
	add(regular, "Read", "body")
	add(link, "Read", "body")
	add(regular, "CreateNew", "exists")
	add(link, "CreateNew", "exists")
	add(dangling, "CreateNew", "exists")
	add(dangling, "Read", "missing")
	for _, path := range []string{root, fifo, fifoLink, os.DevNull} {
		add(path, "Read", "other")
		add(path, "Write", "other")
		add(path, "CreateNew", "exists")
	}
	add(link, "Write", "body")
	created := filepath.Join(root, "created")
	add(created, "CreateNew", "body")
	add(created, "CreateNew", "exists")
	expression = "[" + strings.Join(calls, ", ") + "].join(\",\")"
	expected = strings.Join(labels, ",")
	t.Cleanup(func() {
		for _, path := range []string{regular, created} {
			data, err := os.ReadFile(path)
			if err != nil || len(data) != 0 {
				t.Errorf("expected empty regular file after successful acquisition: %q, %v", data, err)
			}
		}
		info, err := os.Lstat(link)
		if err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Error("Write must follow, not replace, an ambient symlink")
		}
	})
	return
}
