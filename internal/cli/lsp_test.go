package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestLSPCommandServesConfiguredProjectOverStandardIO(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/lsp-command"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := lspFrames(t,
		map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
		map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown", "params": nil},
		map[string]any{"jsonrpc": "2.0", "method": "exit"},
	)
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: bytes.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"lsp", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"name":"TypeRB"`) || !strings.Contains(stdout.String(), `"textDocumentSync":2`) {
		t.Fatalf("unexpected LSP output stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestLSPCommandServesStandaloneFileAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			filename := filepath.Join(root, "hello.trb")
			if err := os.WriteFile(filename, []byte("def main()\n\tputs(\"hello\")\n\treturn\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "broken.trb"), []byte("not valid TypeRB"), 0o644); err != nil {
				t.Fatal(err)
			}
			input := lspFrames(t,
				map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}},
				map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}},
				map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown", "params": nil},
				map[string]any{"jsonrpc": "2.0", "method": "exit"},
			)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: bytes.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			args := []string{"lsp", filename}
			if mode != "go" {
				args = []string{"lsp", "--mode", mode, filename}
			}
			if status := command.Run(args); status != 0 {
				t.Fatalf("status=%d stderr=%s", status, stderr.String())
			}
			if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"name":"TypeRB"`) {
				t.Fatalf("unexpected LSP output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String(), "broken.trb") {
				t.Fatalf("standalone LSP compiled a sibling file: %s", stdout.String())
			}
		})
	}
}

func TestLSPCommandRequiresProjectOrStandaloneFile(t *testing.T) {
	root := t.TempDir()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(previous) })

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"lsp"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "lsp requires FILE.trb when trbconfig.jsonc is unavailable") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestLSPCommandRejectsStandaloneModeForConfiguredProject(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/lsp-project-precedence"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(root, "main.trb")
	if err := os.WriteFile(filename, []byte("def main()\n\treturn\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"lsp", "--mode", "ruby", filename}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "available only when trbconfig.jsonc is unavailable") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func lspFrames(t *testing.T, messages ...map[string]any) []byte {
	t.Helper()
	var result bytes.Buffer
	for _, message := range messages {
		payload, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&result, "Content-Length: %d\r\n\r\n", len(payload))
		result.Write(payload)
	}
	return result.Bytes()
}
