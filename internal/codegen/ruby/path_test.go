package ruby

import (
	"encoding/json"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/pathvalue/pathfixture"
)

func TestHostPathCompositionBothGrammars(t *testing.T) {
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("Ruby is unavailable")
	}
	var source, expected strings.Builder
	for _, windows := range []bool{false, true} {
		for _, fixture := range pathfixture.Cases {
			parent, _ := json.Marshal(fixture.Parent)
			child, _ := json.Marshal(fixture.Child)
			source.WriteString("puts(" + pathJoinExpression(string(parent), string(child), strconv.FormatBool(windows)) + ")\n")
			expected.WriteString(fixture.Expected(windows) + "\n")
		}
	}
	output, err := exec.Command("ruby", "-e", source.String()).CombinedOutput()
	if err != nil || string(output) != expected.String() {
		t.Fatalf("host composition: %v\n%s\nwant:\n%s", err, output, expected.String())
	}
}
