package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestFunctionReferencesKeepCheckedIdentityAfterLaterBindings(t *testing.T) {
	const input = `def read(): Integer
  return 1
end
def before(): Integer
  return read()
end
def generic<T>(_value: T): Integer
  return read()
end
def defaulted(value: Integer = read()): Integer
  return value
end
saved := read
closure := fn(): Integer; return read(); end
read := fn(): Integer; return 2; end
def after(): Integer
  return read()
end
puts(saved())
puts(before())
puts(generic<Integer>(3))
puts(defaulted())
puts(closure())
puts(read())
puts(after())
:reload
puts(before())
puts(after())
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			want := "#<callable> : () -> Integer\n#<fn> : () -> Integer\n#<fn> : () -> Integer\n1\n1\n1\n1\n1\n2\n2\nreloaded\n1\n1\n1\n1\n1\n2\n2\n1\n2\n"
			if stdout.String() != want {
				t.Fatalf("checked function references: got %q, want %q", stdout.String(), want)
			}
		})
	}
}
