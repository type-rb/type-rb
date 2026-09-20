package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandedUnionAliasesAcrossTargetsAndREPL(t *testing.T) {
	const declarations = `record Created
kind: "created"
body: String
end
record Missing
kind: "missing"
code: Integer
end
record Queued
kind: "created"
body: String
end
alias Response = Created | Missing
alias Extended = Response | Queued | Created
alias Whole = 201 | Integer
alias Quantity = Whole | Float
alias Pair<T> = T | Missing
alias GenericResponse = Pair<Created> | Queued

def show(response: Extended): String
return case response.kind
when "created"
response.body
else
response.code.to_s()
end
end

def generic(response: GenericResponse): String
return case response.kind
when "created"
response.body
else
response.code.to_s()
end
end

def numeric(value: Quantity): Float
return value
end

def collection(): Integer
index: 0 := 0
text: "held" := "held"
values: Array<1 | 2> := [2, 1]
puts(values[index])
puts(text[index])
puts(values.include?(1))
puts(values.count(2))
return values.count(1)
end
`
	const body = `puts(show(Created.new(kind: "created", body: "held")))
puts(show(Queued.new(kind: "created", body: "queued")))
puts(show(Missing.new(kind: "missing", code: 404)))
puts(generic(Queued.new(kind: "created", body: "generic")))
puts(numeric(2).floor())
puts(collection())
`
	const want = "held\nqueued\n404\ngeneric\n2\n2\nh\ntrue\n1\n1\n"
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

func TestLiteralCollectionBoundariesRejectInvalidValues(t *testing.T) {
	for name, source := range map[string]string{
		"String index": `index: "a" := "a"
puts([1][index])`,
		"nullable index": `index: Integer? := nil
puts([1][index])`,
		"unlisted member": `values: Array<1 | 2> := [1]
puts(values.include?(3))`,
		"scalar member": `values: Array<1 | 2> := [1]
value := 1
puts(values.include?(value))`,
		"computed member": `values: Array<1 | 2> := [1]
puts(values.include?(1 + 0))`,
	} {
		for _, mode := range []string{"go", "ruby", "typescript"} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				filename := filepath.Join(t.TempDir(), "main.trb")
				if err := os.WriteFile(filename, []byte("def main()\n"+source+"\nend\n"), 0600); err != nil {
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
}
