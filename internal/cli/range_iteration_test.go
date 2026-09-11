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

func TestRunRangeIterationStreamsAcrossBackends(t *testing.T) {
	const source = `def bounds(mut events: Array<Integer>): Range<Integer>
	events.push(1)
	return 1..5
end

def batch_size(mut events: Array<Integer>): Integer
	events.push(2)
	return 2
end

def first(span: Range<Integer>): Integer
	span.each.with_index do |value, index|
		return value + index
	end
	return -1
end

def main()
	mut span := -2..2
	span.each.with_index do |value, index|
		span = 100..101
		next if value == -1
		while true
			break
		end
		puts(value * 10 + index)
	end
	mut events: Array<Integer> := []
	mut saved: Array<Array<Integer>> := []
	bounds(events).each_slice(batch_size(events)).with_index do |part, index|
		saved.push(part)
		next if index == 0
		puts(part[-1] + index * 10)
	end
	puts(saved[0][0])
	puts(events[0] * 10 + events[1])
	(0..9007199254740991).each do |value|
		puts(value)
		break
	end
	(0..9007199254740991).each_slice(2) do |part|
		puts(part[-1] + part.size())
		break
	end
	puts(first(9007199254740990..9007199254740991))
	(9007199254740990..9007199254740991).each do |value|
		puts(value)
	end
	(9007199254740990...9007199254740991).each do |value|
		puts(value)
	end
	(9007199254740990..9007199254740991).each_slice(3) do |part|
		puts(part.size())
	end
	(-9007199254740991..-9007199254740990).each do |value|
		puts(value)
	end
	(3..2).each { |_| puts("bad") }
	(2...2).each_slice(1) { |_| puts("bad") }
	(0..9007199254740991).each.with_index do |_, index|
		puts(index)
		break
	end
	(0..9007199254740991).each_slice(1) do |_|
		break
	end
	mut count := 0
	(0..1_000_000).each { |_| count += 1 }
	puts(count)
end
`
	want := "-20\n2\n13\n24\n14\n25\n1\n12\n0\n3\n9007199254740990\n9007199254740990\n9007199254740991\n9007199254740990\n2\n-9007199254740991\n-9007199254740990\n0\n1000001\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" || mode == "typescript" {
				tool := "ruby"
				if mode == "typescript" {
					tool = "node"
				}
				if _, err := exec.LookPath(tool); err != nil {
					t.Skipf("%s is not installed", tool)
				}
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/range-iteration"
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
				t.Fatalf("status=%d stderr=%s", status, &stderr)
			}
			if stdout.String() != want {
				t.Fatalf("want %q, got %q", want, stdout.String())
			}
		})
	}
}
