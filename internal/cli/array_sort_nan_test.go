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

func TestPortableArraySortKeepsNaNsLastAcrossExecutionPaths(t *testing.T) {
	declarations := `record Ranked
	key: Float
	position: Integer
end

def exercise()
	nan := 0.0 / 0.0
	infinity := 1.0 / 0.0
	values := [nan, 0.0, -0.0, infinity, -infinity, 2.5, nan, -4.0]
	ascending := values.sort()
	descending := values.sort_descending()
	ascending.each { |value| puts(value.to_s()) }
	descending.each { |value| puts(value.to_s()) }
	puts(1.0 / ascending[2] == infinity)
	puts(1.0 / ascending[3] == -infinity)
	puts(1.0 / descending[2] == infinity)
	puts(1.0 / descending[3] == -infinity)
	puts(values[0].nan?())
	rows := [Ranked.new(key: nan, position: 0), Ranked.new(key: 2.0, position: 1), Ranked.new(key: nan, position: 2), Ranked.new(key: 0.0, position: 3), Ranked.new(key: -0.0, position: 4), Ranked.new(key: -1.0, position: 5)]
	ordered := rows.sort_by { |row| row.key }
	reversed := rows.sort_by_descending { |row| row.key }
	ordered.each { |row| puts(row.position) }
	reversed.each { |row| puts(row.position) }
	puts(rows[0].position)
end
`
	want := "-Infinity\n-4.0\n0.0\n0.0\n2.5\nInfinity\nNaN\nNaN\n" +
		"Infinity\n2.5\n0.0\n0.0\n-4.0\n-Infinity\nNaN\nNaN\n" +
		strings.Repeat("true\n", 5) + "5\n3\n4\n1\n0\n2\n1\n3\n4\n5\n0\n2\n0\n"
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "node"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skipf("%s unavailable: %v", tool, err)
			}
			root := t.TempDir()
			config := project.New(root, mode)
			config.SourceDir = "src"
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/array-sort-test"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "main.trb"), []byte(declarations+"\ndef main()\nexercise()\nend\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			for _, action := range []string{"run", "repl"} {
				t.Run(action, func(t *testing.T) {
					var stdout, stderr bytes.Buffer
					command := &CLI{Stdin: strings.NewReader(declarations + "\nexercise()\n:quit\n"), Stdout: &stdout, Stderr: &stderr}
					status := command.Run([]string{action, "--config", config.Path})
					if status != 0 || stdout.String() != want || stderr.Len() != 0 {
						t.Fatalf("status=%d\nstdout=%q\nwant=%q\nstderr=%q", status, stdout.String(), want, stderr.String())
					}
				})
			}
		})
	}
}
