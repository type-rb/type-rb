package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTypeScriptIntrinsicsWithValueBindings(t *testing.T) {
	tests := []struct {
		name    string
		runtime string
	}{
		{name: "node", runtime: "node"},
		{name: "bun", runtime: "bun"},
	}
	source := `import { Result } from trb/std/result

enum Status
	Ready = "READY"
end

def shifted(): Integer
	mut value := [1, 2, 3]
	return value.shift()
end

def status_text(value: String): String
	parsed := Status.from_raw(value)
	case parsed
	when Result::Ok(status)
		return status.raw_value()
	when Result::Err(_error)
		return "invalid"
	end
end

def main()
	value := 2.5
	puts(value.to_s())
	puts(value.to_i())
	puts(shifted())
	puts(status_text("READY"))
	return
end
`
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.runtime); err != nil {
				t.Skipf("%s is unavailable: %v", test.runtime, err)
			}
			filename := filepath.Join(t.TempDir(), "value.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--mode", "typescript", "--runtime", test.runtime, filename}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "2.5\n2\n1\nREADY\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected output stdout=%q stderr=%q", stdout.String(), stderr.String())
			}
		})
	}
}
