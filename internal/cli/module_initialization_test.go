package cli

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/project"
)

func TestModuleInitializationFollowsImportsAcrossDirectories(t *testing.T) {
	runModuleProjectFiles(t, map[string]string{
		"shared.trb": "def mark(value: String): Integer\nputs(value)\nreturn 1\nend\nBASE := mark(\"shared\")\n",
		"a/z.trb":    "import { mark, BASE } from shared\nZ := mark(\"z\") + BASE\n",
		"b/y.trb":    "import { mark } from shared\nimport { Z } from a/z\nY := mark(\"y\") + Z\n",
		"a/x.trb":    "import { mark } from shared\nimport { Y } from b/y\nX := mark(\"x\") + Y\n",
		"unused.trb": "def mark(): Integer\nputs(\"unused\")\nreturn 99\nend\nUNUSED := mark()\n",
		"main.trb":   "import { X } from a/x\nimport { Z } from a/z\ndef main()\nputs(X)\nputs(Z)\nend\n",
	}, "shared\nz\ny\nx\n4\n2\n", "")
}

func TestModuleEmissionPreservesSameNamedDeclarations(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a/value.trb": "record Item\ncount: Integer\nend\ndef item(): Item\nreturn Item.new(count: 7)\nend\n",
		"b/value.trb": "record Item\nlabel: String\nend\ndef item(): Item\nreturn Item.new(label: \"right\")\nend\n",
		"main.trb":    "import { item as left } from a/value\nimport { item as right } from b/value\ndef main()\nputs(left().count)\nputs(right().label)\nend\n",
	}, "7\nright\n", "")
}

func runModuleProjectFiles(t *testing.T, files map[string]string, want, failure string, modes ...string) {
	t.Helper()
	if len(modes) == 0 {
		modes = []string{"go", "ruby", "typescript"}
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			tool := map[string]string{"go": "go", "ruby": "ruby", "typescript": "bun"}[mode]
			if _, err := exec.LookPath(tool); err != nil {
				t.Skip(err)
			}
			config := moduleTestProject(t, mode, files)
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				if err := config.Save(); err != nil {
					t.Fatal(err)
				}
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			status := command.Run([]string{"run", "--config", config.Path})
			if status != 0 || stdout.String() != want || stderr.String() != failure {
				t.Fatalf("status=%d stdout=%q stderr=%s", status, stdout.String(), stderr.String())
			}
		})
	}
}

func moduleTestProject(t *testing.T, mode string, files map[string]string) *project.Config {
	t.Helper()
	config := project.New(t.TempDir(), mode)
	config.SourceDir = "src"
	if config.Go != nil {
		config.Go.Module = "example.com/module-initialization"
	}
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	for name, source := range files {
		filename := filepath.Join(config.SourcePath(), filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(filename), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(source), 0644); err != nil {
			t.Fatal(err)
		}
	}
	return config
}

func TestSelectedTestsInitializeOnlyTheirImportClosures(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			config := moduleTestProject(t, mode, map[string]string{
				"main.trb":         "def mark(): Integer\nputs(\"application\")\nreturn 9\nend\nAPP := mark()\ndef main()\nputs(APP)\nend\n",
				"value.trb":        "def shared(): Integer\nputs(\"shared\")\nreturn 7\nend\nVALUE := shared()\n",
				"a/first_test.trb": moduleTestSource("first"),
				"b/last_test.trb":  moduleTestSource("last"),
			})
			if config.TypeScript != nil {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip(err)
				}
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				if err := config.Save(); err != nil {
					t.Fatal(err)
				}
			}

			for _, selected := range [][]string{{"src/b/last_test.trb"}, {"src/b/last_test.trb", "src/a/first_test.trb"}} {
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				args := []string{"test", "--config", config.Path}
				for _, name := range selected {
					args = append(args, filepath.Join(config.Root, name))
				}
				if status := command.Run(args); status != 0 {
					t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
				}
				output := stdout.String()
				if strings.Contains(output, "application") || strings.Count(output, "shared\n") != 1 {
					t.Fatalf("unexpected roots: %q", output)
				}
				first, last := strings.Index(output, "initialize-first\n"), strings.Index(output, "initialize-last\n")
				if len(selected) == 1 && (first >= 0 || last < 0) || len(selected) == 2 && (first < 0 || last <= first) {
					t.Fatalf("unexpected test root order: %q", output)
				}
				if strings.Index(output, "execute-") < last {
					t.Fatalf("tests executed before initialization: %q", output)
				}
			}
		})
	}
}

func moduleTestSource(name string) string {
	return fmt.Sprintf(`import { VALUE } from value
import { describe, expect, test } from trb/std/test
def mark(): Integer
puts("initialize-%s")
return VALUE
end
READY := mark()
describe("root") do
 test("case") do
  puts("execute-%s")
  expect(READY).to_equal(7)
 end
end
`, name, name)
}

func generatedEntrypoint(t *testing.T, root, mode string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "trb", "entry", "*", "main"+codegen.Extension(mode)))
	if err != nil || len(files) != 1 {
		t.Fatalf("generated entrypoints=%v err=%v", files, err)
	}
	return files[0]
}

func generatedModulePath(root, mode, module, unit string) string {
	if mode != "go" {
		return filepath.Join(root, filepath.FromSlash(module+codegen.Extension(mode)))
	}
	sum := sha256.Sum256([]byte(module))
	return filepath.Join(root, filepath.FromSlash(unit), fmt.Sprintf("module_%x.go", sum[:16]))
}

func TestDiscoveredRoutesInitializeAfterEntryDependencies(t *testing.T) {
	runModuleProjectFiles(t, map[string]string{
		"main.trb": `import { Body, Headers, HttpMethod } from trb/http
import { Request } from trb/web
import { dispatch } from trb/web/testing

def mark(value: String): String
 puts(value)
 return value
end
VALUE := mark("entry")
class Config
end

def main()
 response := dispatch(Request.new(method: HttpMethod.get(), path: "/", query_string: "", headers: Headers.new(), body: Body.empty()))
 puts(response.body.to_s())
end
`,
		"routes/index.trb": `import { Config, VALUE, mark } from main
import { Context, Response } from trb/web
READY := mark(VALUE + ":route")
class RouteConfig < Config
 @label: String := VALUE
end
def get(_context: Context): Response
 return Response.text(RouteConfig.new().label + ":" + READY)
end
`,
	}, "entry\nentry:route\nentry:entry:route\n", "")
}

func TestInitializationPreservesAuthoredOrderAfterMovingTypedModules(t *testing.T) {
	for _, paths := range [][2]string{{"z/value", "a/value"}, {"domain/first", "lib/last"}} {
		runModuleProjectFiles(t, map[string]string{
			paths[0] + ".trb": "def mark(): Integer\nputs(\"first\")\nreturn 1\nend\nVALUE := mark()\nrecord First\nend\n",
			paths[1] + ".trb": "def mark(): Integer\nputs(\"second\")\nreturn 2\nend\nVALUE := mark()\nrecord Second\nend\n",
			"main.trb":        fmt.Sprintf("import { First } from %s\nimport { Second } from %s\nrecord Holder\nfirst: First\nsecond: Second\nend\ndef main()\nputs(\"main\")\nend\n", paths[0], paths[1]),
		}, "first\nsecond\nmain\n", "")
	}
}

func TestDiscoveredJobsInitializeAfterEntryValues(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "ruby" {
				if err := exec.Command("ruby", "-rsequel", "-e", "").Run(); err != nil {
					t.Skip("Ruby Sequel is unavailable")
				}
			}
			config := moduleTestProject(t, mode, map[string]string{
				"main.trb":        "def mark(): Integer\nputs(\"entry\")\nreturn 1\nend\nREADY := mark()\ndef main()\nputs(READY)\nend\n",
				"jobs/sample.trb": "import { Job } from trb/jobs\ndef mark(): Integer\nputs(\"job\")\nreturn 2\nend\nREADY := mark()\nclass SampleJob < Job\ndef perform()\nreturn\nend\nend\n",
			})
			configureSQLJobs(t, config, "sqlite", "jobs.sqlite3")
			if config.TypeScript != nil {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip(err)
				}
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			status := command.Run([]string{"run", "--config", config.Path})
			if status != 0 || stdout.String() != "entry\njob\n1\n" {
				t.Fatalf("status=%d stdout=%q stderr=%s", status, stdout.String(), stderr.String())
			}
		})
	}
}

func TestGoModuleEmissionPreservesSameNamedClassesAndEnums(t *testing.T) {
	files := map[string]string{}
	for _, name := range []string{"left", "right"} {
		files[name+"/item.trb"] = fmt.Sprintf(`enum State
Ready
end
class Item
VALUE := "%s"
@label: String := VALUE
def state(): State
return State::Ready
end
end
`, name)
	}
	files["main.trb"] = `import { Item as Left, State as LeftState } from left/item
import { Item as Right, State as RightState } from right/item
def main()
puts(Left.new().label)
puts(Right.new().label)
puts(Left.new().state() == LeftState::Ready)
puts(Right.new().state() == RightState::Ready)
end
`
	runModuleProjectFiles(t, files, "left\nright\ntrue\ntrue\n", "", "go")
}
