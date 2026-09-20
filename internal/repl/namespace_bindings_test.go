package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestNamespaceBindingsSurviveCallsAndReplay(t *testing.T) {
	const input = `module Counter
  mut value := 1
  def self.bump(): Integer
    value += 1
    return value
  end
end
puts(Counter.bump())
module Other
  value := 30
  def self.read(): Integer
    return value
  end
end
puts(Other.read())
puts(Counter.bump())
:reload
puts(Counter.bump())
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "2\n30\n3\nreloaded\n2\n30\n3\n4\n"; got != want {
				t.Fatalf("namespace storage: got %q, want %q", got, want)
			}
		})
	}
}
