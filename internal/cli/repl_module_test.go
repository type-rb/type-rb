package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestReplLoadsWebTemplateAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "web-app")
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"init", "--mode", mode, "--template", "web", root}); status != 0 {
				t.Fatalf("init status=%d stderr=%s", status, stderr.String())
			}
			stdout.Reset()
			stderr.Reset()
			command.Stdin = strings.NewReader("MIDDLEWARES.size()\nimport trb/web/middleware/request_id\nRequestID.default_options().header_name\n:reload\nMIDDLEWARES.size()\n:quit\n")
			if status := command.Run([]string{"repl", "--config", filepath.Join(root, project.ConfigName)}); status != 0 {
				t.Fatalf("repl status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "2 : Integer\n\"x-request-id\" : String\nreloaded\n2 : Integer\n"; got != want || stderr.Len() != 0 {
				t.Fatalf("stdout=%q, want %q; stderr=%s", got, want, stderr.String())
			}
		})
	}
}

func TestReplKeepsModuleMethodOwnersAcrossModes(t *testing.T) {
	const source = `module Factory
	def self.label(): String
		return "factory"
	end

	def self.describe(prefix: String = Factory.label()): String
		return prefix + "!"
	end
end

module Other
	def self.label(): String
		return "other"
	end
end

def label(): String
	return "top-level"
end

VALUE := Factory.describe()
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			if config.Go != nil {
				config.Go.Module = "example.com/module-methods"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, "factory.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			consumer := "import factory as Maker\n\ndef factory_label(): String\n\treturn Maker.describe()\nend\n"
			if err := os.WriteFile(filepath.Join(root, "consumer.trb"), []byte(consumer), 0o644); err != nil {
				t.Fatal(err)
			}
			input := strings.Join([]string{
				"import { Factory, Other, VALUE, label } from factory",
				"Factory.label()",
				"Other.label()",
				"label()",
				"VALUE",
				"Factory.describe(Other.label())",
				"factory_label()",
				":reload",
				"Factory.describe()",
				":quit",
			}, "\n") + "\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("repl status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			want := "\"factory\" : String\n\"other\" : String\n\"top-level\" : String\n\"factory!\" : String\n\"other!\" : String\n\"factory!\" : String\nreloaded\n\"factory!\" : String\n"
			if got := stdout.String(); got != want || stderr.Len() != 0 {
				t.Fatalf("stdout=%q, want %q; stderr=%s", got, want, stderr.String())
			}
		})
	}
}
