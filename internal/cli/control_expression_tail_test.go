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

func TestGroupedControlExpressionsAcrossBackendsAndREPL(t *testing.T) {
	definitions := `def mark(value: Integer): Integer
	puts(value)
	return value
end
def add(left: Integer, right: Integer): Integer
	puts("add")
	return left + right
end
def first(flag: Boolean): Integer
	value := add(if flag
		return 7
	else
		3
	end, mark(2))
	return value
end
def second(flag: Boolean): Integer
	value := add(mark(1), if flag
		return 8
	else
		3
	end)
	return value
end
def grouped(flag: Integer): Integer
	value := 10 + ((case flag
	when 1
		return 9
	else
		3
	end))
	return value
end
def collection(flag: Boolean): Integer
	values := [if flag
		3
	else
		4
	end, 5]
	return values[0] + values[1]
end
`
	calls := []string{"first(true)", "first(false)", "second(true)", "second(false)", "grouped(1)", "grouped(2)", "collection(false)"}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/control-expression-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			var replOut, replErr bytes.Buffer
			repl := &CLI{Stdin: strings.NewReader(definitions + strings.Join(calls, "\n") + "\n:quit\n"), Stdout: &replOut, Stderr: &replErr}
			if status := repl.Run([]string{"repl", "--config", config.Path}); status != 0 {
				t.Fatalf("REPL status=%d: %s", status, &replErr)
			}
			wantREPL := "7 : Integer\n2\nadd\n5 : Integer\n1\n8 : Integer\n1\nadd\n4 : Integer\n9 : Integer\n13 : Integer\n9 : Integer\n"
			if replOut.String() != wantREPL || replErr.Len() != 0 {
				t.Fatalf("REPL output=%q stderr=%s, want %q", &replOut, &replErr, wantREPL)
			}
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s unavailable; REPL verified", tool)
			}
			source := definitions + "def main()\n"
			for _, call := range calls {
				source += "puts(" + call + ")\n"
			}
			source += "end\n"
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("run status=%d: %s", status, &stderr)
			}
			want := "7\n2\nadd\n5\n1\n8\n1\nadd\n4\n9\n13\n9\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("output=%q stderr=%s, want %q", &stdout, &stderr, want)
			}
		})
	}
}
