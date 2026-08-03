package selfhost_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
)

func TestStageOneModelMatchesBootstrapSnapshot(t *testing.T) {
	source, err := os.ReadFile("src/model.trb")
	if err != nil {
		t.Fatal(err)
	}

	artifact, err := compiler.CompileWithOptions("src/model.trb", source, compiler.Options{
		Mode:       "go",
		Package:    "selfhost",
		ModulePath: "model",
		GoModule:   "github.com/type-rb/type-rb",
	})
	if err != nil {
		t.Fatal(err)
	}

	want, err := os.ReadFile("generated/model.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(artifact.Output, want) {
		t.Fatalf("self-host snapshot is stale; run ./scripts/check-self-host.sh")
	}
}
