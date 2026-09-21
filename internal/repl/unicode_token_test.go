package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunRejectsUnsupportedUnicodeWithoutEvaluatingIt(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, source := range []string{"😀 := 1", "1😀", "١value := 1"} {
			t.Run(mode+"/"+source, func(t *testing.T) {
				var stdout, stderr bytes.Buffer
				err := Run(Options{
					Mode: mode, Stdin: strings.NewReader(source + "\n7\n:quit\n"),
					Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode),
				})
				if err != nil || stdout.String() != "7 : Integer\n" || !strings.Contains(stderr.String(), "unsupported source character ") {
					t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
				}
			})
		}
	}
}
