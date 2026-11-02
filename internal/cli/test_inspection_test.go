package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestAssertionValuesUseAuthoredNamesAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runtime := map[string]string{"go": "go", "ruby": "ruby", "typescript": "bun"}[mode]
			if _, err := exec.LookPath(runtime); err != nil {
				t.Skipf("%s unavailable: %v", runtime, err)
			}
			config := project.New(t.TempDir(), mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/assertion-values"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0755); err != nil {
				t.Fatal(err)
			}
			source, err := os.ReadFile("../compiler/testdata/assertion_values.trb")
			if err != nil {
				t.Fatal(err)
			}
			for _, module := range []string{"left", "right"} {
				directory := filepath.Join(config.SourcePath(), module)
				if err := os.MkdirAll(directory, 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(directory, "model.trb"), []byte("record Labelled\n value: Integer\nend\n"), 0644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "values_test.trb"), source, 0644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			cli := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if code := cli.Run([]string{"test", "--config", config.Path, "--reporter", "json"}); code != 1 {
				t.Fatalf("code=%d\n%s\n%s", code, stdout.String(), stderr.String())
			}
			nodeDisplay := "Node(value: 1, next: <cycle>)"
			fullCycleRow := "[" + strings.Repeat(nodeDisplay+", ", 15) + nodeDisplay + "]"
			partialCycleRow := "[" + strings.Repeat(nodeDisplay+", ", 3) + strings.Repeat("..., ", 12) + "...]"
			expected := map[string]string{
				"cycle budget":          "expected [" + strings.Repeat(fullCycleRow+", ", 5) + partialCycleRow + ", " + strings.Repeat("..., ", 9) + "...] to be nil",
				"imported names":        "expected left/model::Labelled(value: 1) to be nil",
				"other imported name":   "expected right/model::Labelled(value: 2) to be nil",
				"node budget":           "expected [" + strings.Repeat("[0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15], ", 15) + "...] to be nil",
				"range":                 "expected 2..5 to be nil",
				"exclusive range":       "expected 2...5 to be nil",
				"small float":           "expected 0.0000001 to be nil",
				"large float":           "expected 100000000000000000000 to be nil",
				"negative zero":         "expected -0 to be nil",
				"function":              "expected <function> to be nil",
				"plain enum":            `expected "North" to be nil`,
				"raw enum":              `expected "READY" to be nil`,
				"integer raw enum":      "expected -1 to be nil",
				"inherited class":       `expected Child(child: "child") to be nil`,
				"generic helper":        "expected Point(x: 8, y: 9) to be nil",
				"shared value":          "expected [Point(x: 1, y: 2), Point(x: 1, y: 2)] to be nil",
				"unicode hash order":    `expected {"" => 1, "😀" => 2} to be nil`,
				"long string":           `expected "` + strings.Repeat("値", 128) + `"... to be nil`,
				"depth limit":           "expected Node(value: 0, next: Node(value: 1, next: Node(value: 2, next: Node(value: 3, next: Node(value: 4, next: Node(value: 5, next: Node(value: 6, next: Node(value: ..., next: ...)))))))) to be nil",
				"integer hash":          `expected {2 => "two", 10 => "ten"} to be nil`,
				"decoded record":        "expected Point(x: 4, y: 5) to be nil",
				"decoded result":        "expected Result::Ok(Point(x: 4, y: 5)) to be nil",
				"nested generic record": "expected Box(value: Point(x: 1, y: 2)) to be nil",
				"long array":            "expected [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, ...] to be nil",
				"record":                "expected Point(x: 1, y: 2) to equal Point(x: 1, y: 3)",
				"generic record":        `expected Box(value: "text") to be nil`,
				"nested array":          "expected [Point(x: 1, y: 2)] to be nil",
				"hash":                  `expected {"a" => 1, "b" => 2} to be nil`,
				"optional":              `expected "present" to be nil`,
				"nil":                   "expected nil to be true",
				"enum":                  "expected Shape::Circle(2) to equal Shape::Circle(3)",
				"empty variant":         "expected Shape::Empty to be nil",
				"result payload":        "expected Ok, got Err(Point(x: 1, y: 2))",
				"cycle":                 "expected Node(value: 7, next: <cycle>) to be nil",
				"bytes":                 "expected Bytes[65, 66] to be nil",
				"quote":                 `expected "quote\"\\\n値😀" to be nil`,
			}
			failures, passes, summaries := 0, 0, 0
			for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
				var event struct {
					Type, Name, Message         string
					Line, Column, Total, Failed int
				}
				if err := json.Unmarshal([]byte(line), &event); err != nil {
					t.Fatalf("invalid event %q: %v\nstderr=%s", line, err, stderr.String())
				}
				switch event.Type {
				case "test_failed":
					name := strings.TrimPrefix(event.Name, "Values / ")
					want, ok := expected[name]
					if !ok || event.Message != want {
						t.Errorf("%s: got %q, want %q", name, event.Message, want)
					}
					if event.Line < 1 || event.Column < 1 {
						t.Errorf("lost assertion origin: %+v", event)
					}
					failures++
				case "test_passed":
					if event.Name != "Values / continued" {
						t.Errorf("unexpected pass: %+v", event)
					}
					passes++
				case "test_summary":
					if event.Total != len(expected)+1 || event.Failed != len(expected) {
						t.Errorf("summary: %+v", event)
					}
					summaries++
				}
			}
			if failures != len(expected) || passes != 1 || summaries != 1 {
				t.Fatalf("events=%d failures,%d passes,%d summaries\n%s\n%s", failures, passes, summaries, stdout.String(), stderr.String())
			}
		})
	}
}
