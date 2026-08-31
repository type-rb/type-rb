package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/project"
)

func TestCheckConfigFreeFileRootAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			entry := filepath.Join(root, "main.trb")
			helper := filepath.Join(root, "helper.trb")
			if err := os.WriteFile(entry, []byte("import { answer } from ./helper\n\nvalue := answer()\nputs(value)\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(helper, []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "unrelated.trb"), []byte("def broken(\n"), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"check", "--mode", mode, entry}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "checked 2 file(s) for mode "+mode+"\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}

func TestCheckRejectsFilesystemDeclarationOwnersAsValuesAcrossModes(t *testing.T) {
	tests := map[string]struct {
		imports string
		owner   string
	}{
		"File": {
			imports: "import trb/std/file",
			owner:   "File",
		},
		"Dir": {
			imports: "import trb/std/dir",
			owner:   "Dir",
		},
		"FileMode": {
			imports: "import { FileMode } from trb/std/file",
			owner:   "FileMode",
		},
		"DirEntry": {
			imports: "import { DirEntry } from trb/std/dir",
			owner:   "DirEntry",
		},
		"DirEntryKind": {
			imports: "import { DirEntryKind } from trb/std/dir",
			owner:   "DirEntryKind",
		},
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for name, test := range tests {
			t.Run(mode+"/"+name, func(t *testing.T) {
				root := t.TempDir()
				entry := filepath.Join(root, "main.trb")
				source := test.imports + `

def consume(_value: Any)
	return
end

def main()
	consume(` + test.owner + `)
	return
end
`
				if err := os.WriteFile(entry, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}

				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				if status := command.Run([]string{"check", "--mode", mode, entry}); status != 1 {
					t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
				}
				want := "declaration " + test.owner + " cannot be used as a value"
				if !strings.Contains(stderr.String(), want) {
					t.Fatalf("diagnostic %q does not contain %q", stderr.String(), want)
				}
			})
		}
	}
}

func TestCheckConfigFreeFileRootDefaultsToGo(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "library.trb")
	if err := os.WriteFile(entry, []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"check", entry}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "checked 1 file(s) for mode go\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestCheckConfigFreeFileRootEmitsJSONDiagnostics(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "main.trb")
	helper := filepath.Join(root, "helper.trb")
	if err := os.WriteFile(entry, []byte("import { answer } from ./helper\n\nputs(answer())\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(helper, []byte("def answer(): Integer\n\treturn \"not an integer\"\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"check", "--mode", "typescript", "--diagnostic-format", "json", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("JSON diagnostics wrote to stderr: %s", stderr.String())
	}
	var report diagnostic.JSONReport
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("invalid JSON diagnostics: %v\n%s", err, stdout.String())
	}
	if report.Summary.Errors != 1 || len(report.Diagnostics) != 1 || report.Diagnostics[0].Location == nil || report.Diagnostics[0].Location.Path != helper {
		t.Fatalf("unexpected report: %#v", report)
	}
}

func TestCheckStandaloneOptionsRequireAConfigFreeTRBFile(t *testing.T) {
	root := t.TempDir()
	entry := filepath.Join(root, "library.trb")
	if err := os.WriteFile(entry, []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "missing-file", args: []string{"check", "--mode", "ruby"}, want: "check requires FILE.trb when trbconfig.jsonc is unavailable"},
		{name: "invalid-mode", args: []string{"check", "--mode", "python", entry}, want: "standalone mode must be ruby, go, or typescript"},
		{name: "extension", args: []string{"check", filepath.Join(root, "library.rb")}, want: "standalone check source must be a .trb file"},
		{name: "multiple", args: []string{"check", entry, entry}, want: "check accepts at most one standalone .trb file"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("diagnostic %q does not contain %q", stderr.String(), test.want)
			}
		})
	}
}

func TestCheckSelectedProjectFileUsesDiscoveredConfiguration(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/check-discovery"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(root, "library.trb")
	if err := os.WriteFile(entry, []byte("def answer(): Integer\n\treturn 42\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "broken.trb"), []byte("def broken(\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"check", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), filepath.Join(root, "broken.trb")) {
		t.Fatalf("configured project diagnostic did not include sibling source: %s", stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if status := command.Run([]string{"check", "--mode", "ruby", entry}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "--mode and --runtime are available only when trbconfig.jsonc is unavailable") {
		t.Fatalf("unexpected configured mode diagnostic: %s", stderr.String())
	}
}
