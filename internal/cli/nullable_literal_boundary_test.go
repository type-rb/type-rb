package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNullableLiteralUnionsAcrossTargetsAndREPL(t *testing.T) {
	const declarations = `alias Status = 201 | 404

def widen(value: Status): Float
return value
end

def maybe_widen(value: Status?): Float?
return value
end

def show(value: Status?): String
if value == nil
return "absent"
end
return (value + 1).to_s()
end

def exercise()
present: Status? := 201
absent: Status? := nil
puts(present != nil)
puts(absent == nil)
puts(show(present))
puts(show(absent))
puts(widen(201).floor())
number := maybe_widen(present)
if number != nil
puts(number.floor())
end
puts(maybe_widen(absent) == nil)
values: Array<Status?> := [201, nil]
puts(values.size())
end
`
	const body = "exercise()\n"
	const want = "true\ntrue\n202\nabsent\n201\n201\ntrue\n2\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s unavailable", tool)
			}
			filename := filepath.Join(t.TempDir(), "main.trb")
			if err := os.WriteFile(filename, []byte(declarations+"def main()\n"+body+"end\n"), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if code := command.Run([]string{"run", "--mode", mode, filename}); code != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("code=%d stdout=%q stderr=%s", code, stdout.String(), &stderr)
			}
			stdout.Reset()
			stderr.Reset()
			command.Stdin = strings.NewReader(declarations + body + ":quit\n")
			if code := command.Run([]string{"repl", "--mode", mode}); code != 0 || stderr.Len() != 0 || stdout.String() != want {
				t.Fatalf("REPL code=%d stdout=%q stderr=%s", code, stdout.String(), &stderr)
			}
		})
	}
}

func TestNullableLiteralUnionRequiresNarrowing(t *testing.T) {
	const source = "alias Status = 201 | 404\ndef unsafe(value: Status?): Integer\nreturn value + 1\nend\ndef main()\nputs(unsafe(nil))\nend\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			filename := filepath.Join(t.TempDir(), "main.trb")
			if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if code := command.Run([]string{"check", "--mode", mode, filename}); code != 1 || !strings.Contains(stderr.String(), "error[TRB3000]") {
				t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
			}
		})
	}
}
