package repl

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunNullableConditionalClearsStaleFlow(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, branch := range []string{
			"if true\nvalue = nil\nend",
			"case 1\nwhen 1\nvalue = nil\nelse\nvalue = \"other\"\nend",
		} {
			t.Run(mode+"/"+branch[:2], func(t *testing.T) {
				input := "mut value: String? := nil\nvalue = \"kept\"\n" + branch +
					"\n:type value\nvalue == nil\nvalue.size()\nvalue = \"again\"\nvalue.size()\n:quit\n"
				var stdout, stderr bytes.Buffer
				err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
				want := "nil : String? [mut]\n\"kept\" : String? [mut]\nnil : Nil [mut]\nString?\ntrue : Boolean\n\"again\" : String? [mut]\n5 : Integer\n"
				if err != nil || stdout.String() != want || !strings.Contains(stderr.String(), "type String? has no member size") || len(strings.Split(strings.TrimSpace(stderr.String()), "\n")) != 1 {
					t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
				}
			})
		}
	}
}
