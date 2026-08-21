package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
)

func TestRunReusesProvidedInitialCompilation(t *testing.T) {
	program := &ir.Program{Mode: "go", ModulePath: "__trb_repl__"}
	artifact := &compiler.Artifact{IR: program}
	initial := &Compilation{
		Session:   artifact,
		Artifacts: []*compiler.Artifact{artifact},
		Programs:  []*ir.Program{program},
	}
	compileCalls := 0
	var stdout, stderr bytes.Buffer
	err := Run(Options{
		Mode:    "go",
		Stdin:   strings.NewReader(":quit\n"),
		Stdout:  &stdout,
		Stderr:  &stderr,
		Initial: initial,
		Compile: func(string) (*Compilation, error) {
			compileCalls++
			return initial, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if compileCalls != 0 {
		t.Fatalf("compile calls=%d, want 0", compileCalls)
	}
}
