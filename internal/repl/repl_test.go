package repl

import (
	"bytes"
	"path/filepath"
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

func TestDisplayReplPath(t *testing.T) {
	projectRoot := t.TempDir()
	sessionPath := filepath.Join(projectRoot, "src", ".trb-repl.trb")
	projectPath := filepath.Join(projectRoot, "src", "models", "user.trb")
	externalPath := filepath.Join(t.TempDir(), "dependency.trb")

	for _, test := range []struct {
		name string
		path string
		want string
	}{
		{name: "interactive source", path: sessionPath, want: "(trb)"},
		{name: "project source", path: projectPath, want: filepath.Join("src", "models", "user.trb")},
		{name: "external source", path: externalPath, want: externalPath},
		{name: "relative source", path: filepath.Join("src", "main.trb"), want: filepath.Join("src", "main.trb")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := displayReplPath(test.path, projectRoot, sessionPath); got != test.want {
				t.Fatalf("displayReplPath(%q)=%q, want %q", test.path, got, test.want)
			}
		})
	}
}
