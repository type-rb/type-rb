package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestImplicitMethodReceiverSurvivesCaptureAndReplay(t *testing.T) {
	const input = `class Secret
@_value: Integer
def initialize(value: Integer)
@_value = value
end
def _read(): Integer
return @_value
end
def reader(): () -> Integer
return fn(): Integer; return _read(); end
end
end
saved := Secret.new(9).reader()
class Later
end
puts(saved())
:reload
puts(saved())
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			want := "#<fn> : () -> Integer\n9\nreloaded\n9\n9\n"
			if stdout.String() != want {
				t.Fatalf("retained implicit receiver: got %q, want %q", stdout.String(), want)
			}
		})
	}
}

func TestImplicitMethodDispatchDoesNotExposePrivateMembers(t *testing.T) {
	const input = `class Secret
def _read(): Integer
return 9
end
def value(): Integer
return _read()
end
end
puts(Secret.new()._read())
puts(Secret.new().value())
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stdout.String() != "9\n" || !strings.Contains(stderr.String(), "private member _read cannot be accessed externally") {
				t.Fatalf("private dispatch: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}
