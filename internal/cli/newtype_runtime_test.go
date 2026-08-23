package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestRunImportedNewtypeJSONRoundTripAcrossBackends(t *testing.T) {
	files := map[string]string{
		"contracts/index.trb": `newtype UserId = Integer
newtype UserIds = Array<UserId>

record Payload
	id: UserId
	ids: UserIds
	parent_id: UserId?
end
`,
		"main.trb": `import { Payload, UserId, UserIds } from contracts
import { decode, encode } from trb/std/json

def main()
	id := UserId.new(7)
	payload := Payload.new(id: id, ids: UserIds.new([id]), parent_id: nil)
	encoded := encode(payload) catch |_error|
		return
	end
	decoded := decode<Payload>(encoded) catch |_error|
		return
	end
	puts(decoded.id.value())
	puts(decoded.ids.value()[0].value())
	puts(decoded.id == id)
	return
end
`,
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is unavailable: %v", required, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/newtype-runtime-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			for name, source := range files {
				filename := filepath.Join(config.SourcePath(), filepath.FromSlash(name))
				if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stdout=%s stderr=%s", mode, status, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "7\n7\ntrue\n"; got != want || stderr.Len() != 0 {
				t.Fatalf("%s output=%q, want %q; stderr=%q", mode, got, want, stderr.String())
			}
		})
	}
}
