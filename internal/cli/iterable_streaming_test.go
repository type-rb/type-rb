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

func TestRunIterableBoundariesStreamAcrossBackends(t *testing.T) {
	const helper = `def retain<T>(values: Iterable<T>): Iterable<T>
	return values
end
`
	const source = `import { retain } from sequence

record SequenceBox
	values: Iterable<Integer>
end

def first(values: Iterable<Integer>): Integer
	values.each.with_index do |value, index|
		return value + index
	end
	return -1
end

def total(values: Iterable<Integer>): Integer
	mut result := 0
	values.each.with_index do |value, index|
		next if index == 1
		while true
			break
		end
		result += value
	end
	return result
end

def nullable(values: Iterable<Integer>?): Integer
	return -2 if values == nil
	return first(values)
end

def bounds(mut events: Array<Integer>): Iterable<Integer>
	events.push(1)
	return 1..5
end

def batch_size(mut events: Array<Integer>): Integer
	events.push(2)
	return 2
end

def batch_first(values: Iterable<Integer>): Integer
	values.each_slice(2) do |part|
		return part[-1]
	end
	return -1
end

def batches(values: Iterable<Integer>, mut events: Array<Integer>)
	mut saved: Array<Array<Integer>> := []
	values.each_slice(batch_size(events)).with_index do |part, index|
		saved.push(part)
		next if index == 0
		puts(part[-1] + index * 10)
	end
	puts(saved[0][0])
	values.each_slice(1) do |_|
		break
	end
end

def main()
	values := retain<Integer>(0..9007199254740991)
	puts(first(values))
	puts(first(values))
	puts(batch_first(values))
	values.each do |outer|
		puts(first(values) + outer)
		break
	end
	mut array := [1, 2]
	retained := retain<Integer>(array)
	array.push(3)
	array[0] = 4
	puts(total(retained))
	array = [99]
	puts(first(retained))
	puts(total(retain<Integer>(1...5)))
	puts(total(retain<Integer>(3..2)))
	puts(total(retain<Integer>(2...2)))
	puts(total(retain<Integer>(2..2)))
	puts(first(retain<Integer>(9007199254740991..9007199254740991)))
	puts(first(retain<Integer>(-9007199254740991..-9007199254740990)))
	puts(nullable(nil))
	puts(nullable(3..4))
	maybe: Range<Integer>? := 5..6
	puts(nullable(maybe))
	empty: Range<Integer>? := nil
	puts(nullable(empty))
	mut events: Array<Integer> := []
	batches(bounds(events), events)
	puts(events[0] * 10 + events[1])
	batches(retain<Integer>([1, 2, 3, 4, 5]), events)
	puts((1..3).to_a().size())
	box := SequenceBox.new(values: 8..9007199254740991)
	puts(first(box.values))
	pick := fn(input: Iterable<Integer>): Integer
		return first(input)
	end
	puts(pick(9..9007199254740991))
	finite := retain<Integer>(1..3)
	mapped := finite.map { |value| value * 2 }
	puts(mapped.size())
	reduced := finite.reduce(0) { |sum, value| sum + value }
	puts(reduced)
end
`
	want := "0\n0\n1\n0\n7\n4\n8\n0\n0\n2\n9007199254740991\n-9007199254740991\n-2\n3\n5\n-2\n14\n25\n1\n12\n14\n25\n1\n3\n8\n9\n3\n6\n"
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
				config.Go.Module = "example.com/iterable-boundaries"
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
			if err := os.WriteFile(filepath.Join(root, "src", "sequence.trb"), []byte(helper), 0o644); err != nil {
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
