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
	if stderr.Len() != 0 || !strings.Contains(stdout.String(), `"name":"TypeRB"`) || !strings.Contains(stdout.String(), `"textDocumentSync":1`) {
		t.Fatalf("unexpected LSP output stdout=%q stderr=%q", stdout.String(), stderr.String())
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
