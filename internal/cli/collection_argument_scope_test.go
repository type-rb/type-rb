package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestCollectionArgumentsRetainSourceScope(t *testing.T) {
	const source = `import trb/std/result

def receiver(mut events: Array<String>): String
	events.push("receiver")
	return "A😀BC"
end

def bounds(mut events: Array<String>): Range<Integer>
	events.push("bounds")
	return 1...3
end

def extend(mut items: Array<Integer>): Integer
	items.push(50)
	return items.size() - 1
end

def main()
	value := "A😀BC"
	values := [10, 20, 30, 40]
	puts(value.slice(1...value.size()))
	case value.try_slice(1...value.size())
	when Result::Ok(text)
		puts(text)
	when Result::Err(error)
		puts(error.message)
	end
	case value.try_fetch(value.size() - 1)
	when Result::Ok(text)
		puts(text)
	when Result::Err(error)
		puts(error.message)
	end
	puts(values.slice(1...values.size()).size())
	case values.try_slice(1...values.size())
	when Result::Ok(items)
		puts(items.size())
	when Result::Err(error)
		puts(error.message)
	end
	case values.try_fetch(values.size() - 1)
	when Result::Ok(number)
		puts(number)
	when Result::Err(error)
		puts(error.message)
	end
	# The separator must reference the authored value, not the receiver.
	puts("xAyAz".split(value[0]).join("|"))
	mut events: Array<String> := []
	puts(receiver(events).slice(bounds(events)))
	puts(events.join(","))
	mut growing := [10]
	case growing.try_fetch(extend(growing))
	when Result::Ok(number)
		puts(number)
	when Result::Err(error)
		puts(error.message)
	end
	case value.try_slice(0...value.size() + 1)
	when Result::Ok(text)
		puts(text)
	when Result::Err(error)
		puts(error.size)
	end
	case values.try_fetch(values.size())
	when Result::Ok(number)
		puts(number)
	when Result::Err(error)
		puts(error.index)
	end
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if _, err := exec.LookPath("ruby"); err != nil {
					t.Skip("Ruby is not installed")
				}
			}
			if mode == "typescript" {
				if _, err := exec.LookPath("node"); err != nil {
					t.Skip("Node is not installed")
				}
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/collection-scope"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if want := "😀BC\n😀BC\nC\n3\n3\n40\nx|y|z\n😀B\nreceiver,bounds\n50\n4\n4\n"; stdout.String() != want {
				t.Fatalf("want %q, got %q", want, stdout.String())
			}
		})
	}
}
