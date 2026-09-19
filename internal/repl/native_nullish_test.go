package repl

import (
	"bytes"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/nativepackage"
)

func TestNativeNullishCallsRemainUnavailableInTypedIRREPL(t *testing.T) {
	catalog := &nativepackage.Catalog{
		Dependencies: map[string]string{"native-values": "1"},
		Modules: map[string]nativepackage.Module{"native-values": {Exports: map[string]nativepackage.Export{
			"describe": {
				Kind: "function", Type: nativepackage.Type{Kind: "string", Name: "String"}, Required: 1,
				Parameters: []nativepackage.Type{{Kind: "string", Name: "String", Nullable: true, NativeNil: "undefined"}},
			},
		}}},
	}
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
		Filename: "session.trb", ModulePath: "session",
		Source: []byte("import { describe } from \"native-values\"\n\ndescribe(nil)\n"),
	}}, compiler.Options{Mode: "typescript", TypeScriptRuntime: "bun", InteractiveModule: "session", NativePackages: catalog})
	if err != nil {
		t.Fatal(err)
	}
	var programs []*ir.Program
	var session *ir.Program
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.IR.ModulePath == "session" {
			session = artifact.IR
		}
	}
	var output bytes.Buffer
	evaluator := NewEvaluator(&output, "typescript")
	t.Cleanup(func() { _ = evaluator.Close() })
	if err := evaluator.LoadProject(programs, "session"); err != nil {
		t.Fatal(err)
	}
	evaluator.LoadDefinitions(session)
	_, err = evaluator.Evaluate(session.Statements, "session")
	if err == nil || !strings.Contains(err.Error(), "describe is not available in the REPL environment") {
		t.Fatalf("expected native execution limitation, got %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("unavailable native call produced output: %s", output.String())
	}
}
