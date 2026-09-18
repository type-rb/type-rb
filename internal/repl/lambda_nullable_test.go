package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestClosureNullableFactsAcrossREPLSubmissions(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			input := "mut value: Integer? := 1\n" +
				"callback := fn(): Integer\nif value != nil\nreturn value\nend\nreturn 0\nend\n" +
				"puts(callback())\nvalue = nil\nputs(callback())\nvalue = 3\nputs(callback())\n:quit\n"
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 {
				t.Fatalf("REPL: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if !strings.Contains(stdout.String(), "1\n") || !strings.Contains(stdout.String(), "0\n") || !strings.Contains(stdout.String(), "3\n") {
				t.Fatalf("missing successive callback results: %q", stdout.String())
			}
			stdout.Reset()
			stderr.Reset()
			input = "mut value: Integer? := 1\nif value != nil\ncallback := fn(): Integer; return value; end\nputs(callback())\nend\n:quit\n"
			err = Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || !strings.Contains(stderr.String(), "return type is Integer?, expected Integer") {
				t.Fatalf("stale capture: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}
