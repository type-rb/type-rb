package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
)

func TestRunKeepsSubmissionOffsetsWhenGeneratedImportsChange(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			compile := conditionalSessionCompiler(mode)
			var stdout, stderr bytes.Buffer
			err := Run(Options{
				Mode: mode, Stdout: &stdout, Stderr: &stderr,
				Stdin: strings.NewReader("mut n := 1\nn += 1\nn += 1\nn\n:quit\n"),
				Compile: func(source string, resets []int) (*Compilation, error) {
					compilation, err := compile(source, resets)
					if err != nil {
						return nil, err
					}
					// A compiler-generated type dependency may appear or merge
					// into an authored import between successful submissions.
					if strings.Count(source, "n += 1") == 1 {
						compilation.Session.IR.Statements = append([]ir.Statement{
							&ir.Import{Path: "generated/types", Implicit: true},
						}, compilation.Session.IR.Statements...)
					}
					return compilation, nil
				},
			})
			want := "1 : Integer [mut]\n2 : Integer [mut]\n3 : Integer [mut]\n3 : Integer [mut]\n"
			if err != nil || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}
