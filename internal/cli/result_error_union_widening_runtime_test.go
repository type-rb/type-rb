package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunResultTryWidensScalarFailureIntoOuterUnionAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript-node", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
		{name: "typescript-bun", required: "bun", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "bun", filename}
		}},
	}

	source := `import { Result } from trb/std/result

def integer_failure(): Result<Integer, Integer>
	return Result<Integer, Integer>::Err(7)
end

def widened_failure(): Result<String, Float | String>
	value := try integer_failure()
	return Result<String, Float | String>::Ok(value.to_s())
end

def widened_error_is_float?(): Boolean
	case widened_failure()
	when Result::Ok(_value)
		return false
	when Result::Err(error)
		case error
		when Float(value)
			return value == 7.0
		when String(_value)
			return false
		end
	end
end

def main()
	puts(widened_error_is_float?())
	return
end
`

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			root := t.TempDir()
			filename := filepath.Join(root, "result_error_union_widening.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if got, want := stdout.String(), "true\n"; got != want || stderr.Len() != 0 {
				t.Fatalf("scalar Result failure widening output=%q, want %q; stderr=%q", got, want, stderr.String())
			}
		})
	}
}
