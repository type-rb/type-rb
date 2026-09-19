package repl

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestReplIterationBraceBlocks(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/iteration_brace_blocks.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			e, session := compileRangeSession(t, mode, string(source)+"\nmain()\n")
			var output bytes.Buffer
			e.stdout = &output
			if _, err := e.Evaluate(session.Statements, session.ModulePath); err != nil {
				t.Fatal(err)
			}
			if want := "24\nfirst\nsecond\ntwo\nthree\n"; output.String() != want {
				t.Fatalf("output=%q, want %q", output.String(), want)
			}
		})
	}
}

func TestReadIterationBraceBlocksAcrossModes(t *testing.T) {
	source, err := os.ReadFile("../compiler/testdata/iteration_brace_blocks.trb")
	if err != nil {
		t.Fatal(err)
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(string(source) + "\nmain()\n:quit\n"),
				Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			if err != nil || stderr.Len() != 0 || stdout.String() != "24\nfirst\nsecond\ntwo\nthree\n" {
				t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}

func TestCompleteIterationBraceBlocks(t *testing.T) {
	for _, source := range []string{
		"[1].each { |item| while true; puts(item); break; end }",
		"[1].each { |item| [item].each do |inner|; puts(inner); end }",
		"[1].each { |item| callback := fn(): Integer; return item; end; puts(callback()) }",
	} {
		if !Complete(source) || Complete(strings.TrimSuffix(source, "}")) {
			t.Fatalf("incorrect completion for brace body %q", source)
		}
	}
}
