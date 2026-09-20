package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLiteralTypeParametersAndAssignmentAcrossTargetsAndREPL(t *testing.T) {
	const declarations = `def label(status: 200 | 404): String
return case status
when 200
"ok"
when 404
"missing"
end
end

def word(kind: "created" | "missing"): String
return kind
end

def widened(value: 200 | Integer): Integer
return value
end

def update(): String
mut status: 200 | 404 := 200
status = 404
mut values: Array<200 | 404> := [200]
values[0] = 404
mut names: Hash<String, "created" | "missing"> := {"a" => "created"}
names["a"] = "missing"
mut exact: 201 := 201
exact = 201
puts(exact)
puts(label(values[0]))
return label(status) + ":" + word(names["a"])
end
`
	const body = "puts(label(200))\nputs(word(\"created\"))\nputs(widened(202))\nputs(update())\n"
	const want = "ok\ncreated\n202\n201\nmissing\nmissing:missing\n"
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

func TestLiteralTypeAssignmentRejectsScalarNarrowing(t *testing.T) {
	for name, assignment := range map[string]string{
		"wrong literal":    "status = 500",
		"computed literal": "status = 400 + 4",
		"scalar binding":   "value := 404\nstatus = value",
		"compound":         "status += 0",
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				source := "def main()\nmut status: 200 | 404 := 200\n" + assignment + "\nputs(status)\nend\n"
				filename := filepath.Join(t.TempDir(), "main.trb")
				if err := os.WriteFile(filename, []byte(source), 0600); err != nil {
					t.Fatal(err)
				}
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				if code := command.Run([]string{"check", "--mode", mode, filename}); code != 1 || !strings.Contains(stderr.String(), "cannot assign") {
					t.Fatalf("code=%d stdout=%s stderr=%s", code, &stdout, &stderr)
				}
			})
		}
	}
}
