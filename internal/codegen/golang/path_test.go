package golang

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/pathvalue/pathfixture"
)

func TestHostPathCompositionBothGrammars(t *testing.T) {
	var source, expected strings.Builder
	source.WriteString("package main\nimport (\"fmt\"; \"strings\")\nfunc main() {\n")
	for _, windows := range []bool{false, true} {
		for _, fixture := range pathfixture.Cases {
			source.WriteString("fmt.Println(" + pathJoinExpression(strconv.Quote(fixture.Parent), strconv.Quote(fixture.Child), strconv.FormatBool(windows)) + ")\n")
			expected.WriteString(fixture.Expected(windows) + "\n")
		}
	}
	source.WriteString("}\n")
	filename := filepath.Join(t.TempDir(), "main.go")
	if err := os.WriteFile(filename, []byte(source.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command("go", "run", filename).CombinedOutput()
	if err != nil || string(output) != expected.String() {
		t.Fatalf("host composition: %v\n%s\nwant:\n%s", err, output, expected.String())
	}
}
