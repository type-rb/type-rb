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

func TestRunPortableIntegerAndIEEEFloatArithmeticAcrossBackends(t *testing.T) {
	valid := `def main()
	mut value := 9007199254740990
	value += 1
	puts(value)
	value -= 1
	value *= 1
	value /= 1
	puts(value)
	puts(2 ** 52)
	puts(-5 / 2)
	puts(-5 % 2)
	puts((1.0 / 0.0).to_s())
	puts((1.0 / ((-1.0) * 0.0)).to_s())
	puts((0.0 / 0.0).to_s())
	puts((10.0 ** 400.0).to_s())
	puts(((-1.0) ** 0.5).to_s())
	return
end
`
	overflow := `def main()
	puts(9007199254740991 + 1)
	return
end
`

	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			required := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(required); err != nil {
				t.Skipf("%s is unavailable: %v", required, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/numeric-runtime-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			filename := filepath.Join(config.SourcePath(), "main.trb")
			if err := os.WriteFile(filename, []byte(valid), 0o644); err != nil {
				t.Fatal(err)
			}

			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
				t.Fatalf("%s status=%d stdout=%s stderr=%s", mode, status, stdout.String(), stderr.String())
			}
			want := "9007199254740991\n9007199254740990\n4503599627370496\n-2\n-1\nInfinity\n-Infinity\nNaN\nInfinity\nNaN\n"
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("%s output=%q, want %q; stderr=%q", mode, stdout.String(), want, stderr.String())
			}

			if err := os.WriteFile(filename, []byte(overflow), 0o644); err != nil {
				t.Fatal(err)
			}
			stdout.Reset()
			stderr.Reset()
			if status := command.Run([]string{"run", "--config", config.Path}); status == 0 {
				t.Fatalf("%s accepted Integer overflow; stdout=%s", mode, stdout.String())
			}
			if !strings.Contains(stderr.String(), "Integer is outside the portable range") {
				t.Fatalf("%s overflow error=%s", mode, stderr.String())
			}
		})
	}
}
