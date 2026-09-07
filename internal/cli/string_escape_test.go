package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/formatter"
)

func TestStringEscapesAcrossTargetsAndREPL(t *testing.T) {
	const declarations = `import trb/std/result
enum Marker
 Literal = "\#{missing}"
end
def marker_value(): String
case Marker.from_raw("\#{missing}")
when Result::Ok(value)
 return value.raw_value()
when Result::Err(error)
 return error.message
end
end
`
	const body = `s := "abc"
puts("aaa\#{missing}")
puts("aaa\\#{s}")
puts("aaa\\\#{missing}")
puts("aaa\#{s} #{s}")
puts("#{"\#{missing}"}")
puts("\u0023{missing}")
puts("${missing} #@missing #$missing")
puts("\\n #{s}\nend")
puts("\a #{s}")
puts(marker_value())
`
	const want = "aaa#{missing}\naaa\\abc\naaa\\#{missing}\naaa#{s} abc\n#{missing}\n#{missing}\n${missing} #@missing #$missing\n\\n abc\nend\n\a abc\n#{missing}\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s is not installed", tool)
			}
			path := filepath.Join(t.TempDir(), "main.trb")
			source := []byte(declarations + "def main()\n" + body + "end\n")
			formatted, diagnostics := formatter.Format(source)
			if len(diagnostics) != 0 {
				t.Fatal(diagnostics)
			}
			// Formatting must preserve authored escapes, not turn them into code.
			if !bytes.Contains(formatted, []byte(`"aaa\#{missing}"`)) || !bytes.Contains(formatted, []byte(`"aaa\\#{s}"`)) {
				t.Fatalf("formatter changed escapes:\n%s", formatted)
			}
			if err := os.WriteFile(path, formatted, 0o600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--mode", mode, path}); status != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("run status=%d stdout=%q stderr=%s", status, stdout.String(), stderr.String())
			}
			stdout.Reset()
			stderr.Reset()
			command.Stdin = strings.NewReader(declarations + body + ":quit\n")
			if status := command.Run([]string{"repl", "--mode", mode}); status != 0 || stderr.Len() != 0 || stdout.String() != "\"abc\" : String\n"+want {
				t.Fatalf("repl status=%d stdout=%q stderr=%s", status, stdout.String(), stderr.String())
			}
		})
	}
}
