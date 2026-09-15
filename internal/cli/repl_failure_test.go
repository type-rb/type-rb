package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestReplFailureFlowTracksChangingImportPrelude(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/repl-failure"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "src", "helpers.trb"), []byte("def label(): String\nreturn \"imported\"\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			input := "label()\nmut value: String? := nil\nvalue = \"kept\"\n" +
				"if true\nvalue = nil\nputs(1 / 0)\nend\n:type value\n" +
				"import { label } from helpers\n:type value\nvalue == nil\n" +
				"value = \"restored\"\nvalue.size()\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			status := command.Run([]string{"repl", "--config", config.Path})
			want := "\"imported\" : String\nnil : String? [mut]\n\"kept\" : String? [mut]\nString?\nString?\ntrue : Boolean\n\"restored\" : String? [mut]\n8 : Integer\n"
			if status != 0 || stdout.String() != want || !strings.Contains(stderr.String(), "(trb):6:6: error: division by zero") ||
				len(strings.Split(strings.TrimSpace(stderr.String()), "\n")) != 1 {
				t.Fatalf("status=%d stdout=%q stderr=%q", status, stdout.String(), stderr.String())
			}
		})
	}
}
