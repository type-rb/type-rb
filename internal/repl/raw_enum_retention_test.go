package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestRawEnumResultInitializerIsNotReplayedOnInspection(t *testing.T) {
	const input = `enum Status
  Ready = "ready"
end
def input(): String
  puts("input")
  return "ready"
end
parsed := Status.from_raw(input())
case parsed
when Result::Ok(value)
  puts(value.raw_value())
when Result::Err(error)
  puts(error.message)
end
marker := 3
puts(marker)
missing()
case parsed
when Result::Ok(value)
  puts(value.raw_value())
when Result::Err(error)
  puts(error.message)
end
:reload
case parsed
when Result::Ok(value)
  puts(value.raw_value())
when Result::Err(error)
  puts(error.message)
end
:quit
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || !strings.Contains(stderr.String(), "missing") {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			beforeReload, afterReload, found := strings.Cut(stdout.String(), "reloaded\n")
			if !found || strings.Count(beforeReload, "input\n") != 1 || strings.Count(afterReload, "input\n") != 1 {
				t.Fatalf("initializer should run once initially and once on reload: %q", stdout.String())
			}
			if strings.Count(beforeReload, "ready\n") != 2 || strings.Count(afterReload, "ready\n") != 3 {
				t.Fatalf("inspection should use the retained Result: %q", stdout.String())
			}
		})
	}
}
