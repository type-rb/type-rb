package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestNamespacedRecordValuesSurviveDeclarationsAndReload(t *testing.T) {
	const input = `module First
record Entry
value: Integer
copy: Integer = value
end
end
first := First::Entry.new(value: 3)
module Second
record Entry
value: String
copy: String = value
end
end
second := Second::Entry.new(value: "kept")
puts(first.copy)
puts(second.copy)
:reload
puts(first.copy)
puts(second.copy)
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "First::Entry(value: 3, copy: 3) : First::Entry\nSecond::Entry(value: \"kept\", copy: \"kept\") : Second::Entry\n3\nkept\nreloaded\n3\nkept\n3\nkept\n"; got != want {
				t.Fatalf("retained records: got %q, want %q", got, want)
			}
		})
	}
}
