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

func TestGenericCallableFactoriesAcrossBackendsAndREPL(t *testing.T) {
	definitions := `def keep<T>(value: T, *, label: String = "saved"): () -> T
 puts(label)
 return fn(): T
  return value
 end
end
class Factory
 def keep<T>(value: T): () -> T
  return fn(): T
   return value
  end
 end
end
def probe()
 number := keep<Integer>(7)
 text := keep<String>("held", label: "explicit")
 factory := Factory.new()
 method := factory.keep<Integer>(9)
 puts(number())
 puts(text())
 puts(method())
end
`
	want := "saved\nexplicit\n7\nheld\n9\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/callable-factory"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			var replOut, replErr bytes.Buffer
			repl := &CLI{Stdin: strings.NewReader(definitions + "probe()\n:quit\n"), Stdout: &replOut, Stderr: &replErr}
			if status := repl.Run([]string{"repl", "--config", config.Path}); status != 0 || replErr.Len() != 0 || replOut.String() != want {
				t.Fatalf("REPL status=%d output=%q stderr=%s, want %q", status, &replOut, &replErr, want)
			}
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s unavailable; REPL verified", tool)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(definitions+"def main()\nprobe()\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("run status=%d output=%q stderr=%s, want %q", status, &stdout, &stderr, want)
			}
		})
	}
}
