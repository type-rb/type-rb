package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewtypeAliasAndRepresentationAcrossTargetsAndREPL(t *testing.T) {
	const declarations = `newtype Id = Integer
alias Name = Id
newtype Value = Integer | String
newtype Price = Float

def show(value: Value): String
case value.value()
when Integer(number)
return number.to_s()
when String(text)
return text
end
end
`
	const body = `puts(Name.new(7).value())
puts(show(Value.new("held")))
puts(show(Value.new(9)))
puts(Price.new(2).value().floor())
`
	const want = "7\nheld\n9\n2\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s is unavailable", tool)
			}
			filename := filepath.Join(t.TempDir(), "main.trb")
			if err := os.WriteFile(filename, []byte(declarations+"def main()\n"+body+"end\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if code := command.Run([]string{"run", "--mode", mode, filename}); code != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("run code=%d stdout=%q stderr=%s", code, stdout.String(), stderr.String())
			}
			stdout.Reset()
			stderr.Reset()
			command.Stdin = strings.NewReader(declarations + body + ":quit\n")
			if code := command.Run([]string{"repl", "--mode", mode}); code != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("REPL code=%d stdout=%q stderr=%s", code, stdout.String(), stderr.String())
			}
		})
	}
}

func TestNewtypeDeclarationConflictIsDiagnostic(t *testing.T) {
	for _, declaration := range []string{"record Id\nvalue: Integer\nend\n", "alias Id = Integer\n", "enum Id\nValue\nend\n", "newtype Id = String\n"} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(mode+"/"+strings.Fields(declaration)[0], func(t *testing.T) {
				filename := filepath.Join(t.TempDir(), "main.trb")
				source := declaration + "newtype Id = Integer\ndef main()\nreturn\nend\n"
				if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				if code := command.Run([]string{"check", "--mode", mode, filename}); code != 1 || !strings.Contains(stderr.String(), "already declared") {
					t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
			})
		}
	}
}
