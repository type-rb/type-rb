package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/nativesnapshot"
	"github.com/type-rb/type-rb/internal/project"
	"github.com/type-rb/type-rb/internal/toolingprotocol"
)

func TestCompilerNativeSnapshotEmitsTheExperimentalGate1Boundary(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\tputs(\"ok\")\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"compiler", "native-snapshot", "--mode", "go", entry}); status != 0 {
		t.Fatalf("status=%d; stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("native snapshot wrote to stderr: %s", stderr.String())
	}
	var snapshot nativesnapshot.Snapshot
	if err := json.Unmarshal(stdout.Bytes(), &snapshot); err != nil {
		t.Fatalf("invalid native snapshot JSON: %v\n%s", err, stdout.String())
	}
	if snapshot.Format != nativesnapshot.Format || snapshot.Version != nativesnapshot.Version || snapshot.EntryFunction != "main#main" {
		t.Fatalf("unexpected native snapshot: %#v", snapshot)
	}
	if len(snapshot.Functions) != 1 || len(snapshot.Functions[0].Blocks) != 1 {
		t.Fatalf("unexpected native snapshot functions: %#v", snapshot.Functions)
	}
}

func TestCompilerInspectEmitsTheSameTypedSnapshotAcrossModes(t *testing.T) {
	var baseline []toolingprotocol.Declaration
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/tooling"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			helperSource := "def answer(): Integer\n\treturn 42\nend\n"
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "helper.trb"), []byte(helperSource), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte("import { answer } from ./helper\n\ndef main()\n\tputs(answer())\n\treturn\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			report, stdout := runCompilerInspection(t, []string{"compiler", "inspect", "--config", config.Path}, 0)
			if report.ProtocolVersion != toolingprotocol.ProtocolVersion || report.CompilerVersion != Version || report.Mode != mode {
				t.Fatalf("unexpected report metadata: %#v", report)
			}
			if len(report.Sources) != 2 || len(report.Modules) != 2 || len(report.Declarations) != 2 || report.Summary.Errors != 0 || len(report.Diagnostics) != 0 {
				t.Fatalf("unexpected compiler inspection: %#v", report)
			}
			if report.Sources[0].Encoding != "utf-8" || report.Sources[0].Content != helperSource || report.Modules[0].ModulePath != "helper" {
				t.Fatalf("snapshot inputs are not deterministic: %#v %#v", report.Sources, report.Modules)
			}
			mainModule := report.Modules[1]
			if mainModule.ModulePath != "main" || len(mainModule.Imports) != 1 || mainModule.Imports[0].Path != "./helper" || mainModule.Imports[0].ModulePath != "helper" {
				t.Fatalf("unexpected module graph: %#v", report.Modules)
			}
			answer, ok := compilerInspectionDeclaration(report, toolingprotocol.DeclarationFunction, "answer")
			if !ok || answer.ReturnType == nil || answer.ReturnType.Kind != "int" || answer.ReturnType.Name != "Integer" {
				t.Fatalf("unexpected answer declaration: %#v", answer)
			}

			second, secondStdout := runCompilerInspection(t, []string{"compiler", "inspect", "--config", config.Path}, 0)
			if stdout != secondStdout || !reflect.DeepEqual(report, second) {
				t.Fatal("compiler inspection output changed without an input change")
			}
			if baseline == nil {
				baseline = report.Declarations
			} else if !equivalentCompilerInspectionDeclarations(baseline, report.Declarations) {
				t.Fatalf("%s declarations differ from the portable baseline:\n%#v\n%#v", mode, baseline, report.Declarations)
			}
		})
	}
}

func TestCompilerInspectReturnsAJSONSnapshotWithDiagnostics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/tooling-invalid"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := "def answer(): Integer\n\treturn \"wrong\"\nend\n"
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	report, _ := runCompilerInspection(t, []string{"compiler", "inspect", "--config", config.Path}, 1)
	if report.Mode != "go" || len(report.Sources) != 1 || report.Sources[0].Encoding != "utf-8" || report.Sources[0].Content != source || len(report.Modules) != 1 {
		t.Fatalf("diagnostic snapshot lost its inputs: %#v", report)
	}
	if len(report.Declarations) != 0 || report.Summary.Errors != 1 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Location == nil {
		t.Fatalf("unexpected diagnostic snapshot: %#v", report)
	}
}

func TestCompilerInspectSupportsAConfigFreeFileRoot(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	if err := os.WriteFile(entry, []byte("import { answer } from ./helper\n\ndef main()\n\tputs(answer())\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "helper.trb"), []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "unrelated.trb"), []byte("def broken(\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	report, _ := runCompilerInspection(t, []string{"compiler", "inspect", "--mode", "ruby", entry}, 0)
	if report.Mode != "ruby" || len(report.Sources) != 2 || len(report.Modules) != 2 {
		t.Fatalf("unexpected standalone snapshot: %#v", report)
	}
	for _, source := range report.Sources {
		if filepath.Base(source.Path) == "unrelated.trb" {
			t.Fatal("standalone inspection included an unrelated sibling")
		}
	}
}

func TestCompilerInspectOmitsAnInvalidModeFromAProjectDiagnostic(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	if err := os.WriteFile(entry, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, _ := runCompilerInspection(t, []string{"compiler", "inspect", "--mode", "python", entry}, 1)
	if report.Mode != "" || report.Summary.Errors != 1 || len(report.Diagnostics) != 1 {
		t.Fatalf("unexpected invalid-mode report: %#v", report)
	}
}

func runCompilerInspection(t *testing.T, args []string, wantStatus int) (toolingprotocol.Report, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run(args); status != wantStatus {
		t.Fatalf("status=%d, want %d; stdout=%s stderr=%s", status, wantStatus, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("compiler inspection wrote to stderr: %s", stderr.String())
	}
	var report toolingprotocol.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid compiler inspection JSON: %v\n%s", err, stdout.String())
	}
	return report, stdout.String()
}

func compilerInspectionDeclaration(report toolingprotocol.Report, kind toolingprotocol.DeclarationKind, name string) (toolingprotocol.Declaration, bool) {
	for _, declaration := range report.Declarations {
		if declaration.Kind == kind && declaration.Name == name {
			return declaration, true
		}
	}
	return toolingprotocol.Declaration{}, false
}

func equivalentCompilerInspectionDeclarations(left, right []toolingprotocol.Declaration) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		leftDeclaration := left[index]
		rightDeclaration := right[index]
		leftDeclaration.Location.Path = ""
		rightDeclaration.Location.Path = ""
		if !reflect.DeepEqual(leftDeclaration, rightDeclaration) {
			return false
		}
	}
	return true
}
