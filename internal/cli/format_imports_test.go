package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestFmtCanonicalizesEquivalentProjectIndexImports(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "src"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	writeFormatTestFile(t, filepath.Join(root, "src", "shared", "ui", "DataTable", "index.trb"), "def DataTable()\n\treturn\nend\n")
	entry := filepath.Join(root, "src", "main.trb")
	writeFormatTestFile(t, entry, "import { DataTable } from shared / ui / DataTable / index # component\n\ndef main()\nDataTable()\nreturn\nend\n")

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"fmt", entry}); status != 0 {
		t.Fatalf("fmt status=%d stderr=%s", status, stderr.String())
	}
	formatted, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	want := "import { DataTable } from shared/ui/DataTable # component\n\ndef main()\n\tDataTable()\n\treturn\nend\n"
	if string(formatted) != want {
		t.Fatalf("formatted source:\n%s\nwant:\n%s", formatted, want)
	}
}

func TestFmtKeepsProjectIndexImportWhenShortPathSelectsDirectFile(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "src"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	writeFormatTestFile(t, filepath.Join(root, "src", "models", "user.trb"), "class DirectUser\nend\n")
	writeFormatTestFile(t, filepath.Join(root, "src", "models", "user", "index.trb"), "class IndexedUser\nend\n")
	entry := filepath.Join(root, "src", "main.trb")
	writeFormatTestFile(t, entry, "import { IndexedUser } from models / user / index\n")

	var stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &stderr}
	if status := command.Run([]string{"fmt", entry}); status != 0 {
		t.Fatalf("fmt status=%d stderr=%s", status, stderr.String())
	}
	formatted, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if string(formatted) != "import { IndexedUser } from models/user/index\n" {
		t.Fatalf("formatted source:\n%s", formatted)
	}
}

func TestFmtCanonicalizesEquivalentConfigFreeIndexImport(t *testing.T) {
	root := t.TempDir()
	writeFormatTestFile(t, filepath.Join(root, "helpers", "index.trb"), "def helper()\n\treturn\nend\n")
	entry := filepath.Join(root, "main.trb")
	writeFormatTestFile(t, entry, "import { helper } from helpers / index\n")

	var stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &stderr}
	if status := command.Run([]string{"fmt", entry}); status != 0 {
		t.Fatalf("fmt status=%d stderr=%s", status, stderr.String())
	}
	formatted, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if string(formatted) != "import { helper } from helpers\n" {
		t.Fatalf("formatted source:\n%s", formatted)
	}
}

func TestFmtCheckReportsCanonicalIndexImport(t *testing.T) {
	root := t.TempDir()
	writeFormatTestFile(t, filepath.Join(root, "helpers", "index.trb"), "def helper()\n\treturn\nend\n")
	entry := filepath.Join(root, "main.trb")
	original := "import { helper } from helpers/index\n"
	writeFormatTestFile(t, entry, original)

	var stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &bytes.Buffer{}, Stderr: &stderr}
	if status := command.Run([]string{"fmt", "--check", entry}); status != 1 {
		t.Fatalf("fmt --check status=%d stderr=%s", status, stderr.String())
	}
	unchanged, err := os.ReadFile(entry)
	if err != nil {
		t.Fatal(err)
	}
	if string(unchanged) != original {
		t.Fatalf("fmt --check changed source:\n%s", unchanged)
	}
}

func writeFormatTestFile(t *testing.T, path, source string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
}
