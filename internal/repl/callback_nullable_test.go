package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestCallbackNullableInvalidationAcrossREPLSubmissions(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			input := "mut value: Integer? := 1\n" +
				"clear := fn(); value = nil; end\n" +
				"callbacks := [clear]\nvalue = 2\ncallbacks[0]()\n" +
				"value + 1\n" +
				"restore := fn(); value = 7; end\nrestore()\n" +
				"if value != nil\nputs(value + 1)\nend\n:quit\n"
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || !strings.Contains(stderr.String(), "operator + does not support Integer? and Integer") {
				t.Fatalf("stale callback proof: err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
			if strings.Count(stderr.String(), "error[") != 1 || !strings.Contains(stdout.String(), "8\n") {
				t.Fatalf("fresh guard after rejection: stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
