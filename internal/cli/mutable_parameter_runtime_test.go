package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMutableParameterReassignmentStaysLocalAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
	}
	source := `def advance(mut value: Integer, *, mut amount: Integer = 1): Integer
	value += amount
	amount = 0
	return value + amount
end

def main()
	value := 3
	puts(advance(value).to_s())
	puts(advance(value, amount: 2).to_s())
	puts(value.to_s())
	advance_fn := fn(mut input: Integer): Integer
		input += 2
		return input
	end
	puts(advance_fn(value).to_s())
	puts(value.to_s())
	return
end
`
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			filename := filepath.Join(t.TempDir(), "mutable_parameter.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "4\n5\n3\n5\n3\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
