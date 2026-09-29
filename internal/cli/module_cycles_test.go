package cli

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestCyclicModulesExecuteMutuallyRecursiveFunctions(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { odd } from b\ndef even(value: Integer): Boolean\nif value == 0\nreturn true\nend\nreturn odd(value - 1)\nend\n",
		"b.trb":    "import { even } from a\ndef odd(value: Integer): Boolean\nif value == 0\nreturn false\nend\nreturn even(value - 1)\nend\n",
		"main.trb": "import { even } from a\nimport { odd } from b\ndef main()\nputs(even(10))\nputs(odd(11))\nend\n",
	}, "true\ntrue\n", "")
}

func TestCyclicModuleValuesFollowTransitiveReadsAndDeclarationOrder(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { BASE } from b\ndef value(): Integer\nreturn BASE + 1\nend\ndef mark(label: String): Integer\nputs(label)\nreturn 1\nend\nFIRST := value() + mark(\"a-first\")\nLAST := mark(\"a-last\")\n",
		"b.trb":    "import { mark, value } from a\nBASE := mark(\"b-base\")\ndef later(): Integer\nreturn value()\nend\n",
		"main.trb": "import { FIRST, LAST } from a\nimport { later } from b\ndef main()\nputs(FIRST + LAST + later())\nend\n",
	}, "b-base\na-first\na-last\n6\n", "")
}

func TestCyclicModulesPrepareSuperclassesBeforeValues(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { Base } from b\nclass Child < Base\nVALUE := Base::VALUE + 1\nend\ndef make(): Child\nreturn Child.new()\nend\n",
		"b.trb":    "import { make } from a\nclass Base\nVALUE := 40\ndef number(): Integer\nreturn VALUE\nend\nend\ndef value(): Integer\nreturn make().number()\nend\n",
		"main.trb": "import { Child } from a\ndef main()\nputs(Child::VALUE)\nend\n",
	}, "41\n", "")
}

func TestCyclicInitializersIncludeConstructorDefaultsButNotLambdaBodies(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb": `import { BASE } from b
class Item
 @number: Integer := BASE
end
LAZY := fn(): Integer
 return BASE
end
VALUE := Item.new().number
`,
		"b.trb": `import { VALUE } from a
def mark(): Integer
 puts("base")
 return 7
end
BASE := mark()
def later(): Integer
 return VALUE
end
`,
		"main.trb": "import { LAZY, VALUE } from a\ndef main()\nputs(LAZY())\nputs(VALUE)\nend\n",
	}, "base\n7\n7\n", "")
}

func TestCyclicUnusedModulesAreCheckedWithoutRunning(t *testing.T) {
	runModuleProjectFiles(t, map[string]string{
		"a.trb":    "import { B } from b\ndef mark(): Integer\nputs(\"unreachable\")\nreturn B\nend\nA := mark()\n",
		"b.trb":    "import { A } from a\nB := 1\ndef later(): Integer\nreturn A\nend\n",
		"main.trb": "def main()\nputs(\"main\")\nend\n",
	}, "main\n", "")
}

func TestCyclicModuleNamespaceValues(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { BASE } from b\nmodule Settings\nVALUE := BASE + 1\ndef self.read(): Integer\nreturn VALUE\nend\nend\n",
		"b.trb":    "import { Settings } from a\nBASE := 7\ndef later(): Integer\nreturn Settings.read()\nend\n",
		"main.trb": "import { Settings } from a\ndef main()\nputs(Settings::VALUE)\nputs(Settings.read())\nend\n",
	}, "8\n8\n", "")
}

func TestSelectedTestRootsShareOneCyclicInitialization(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			if mode == "typescript" {
				if _, err := exec.LookPath("bun"); err != nil {
					t.Skip("Bun is required for generated TypeScript test execution")
				}
			}
			config := moduleTestProject(t, mode, map[string]string{
				"main.trb":         "def mark(): Integer\nputs(\"application\")\nreturn 1\nend\nAPP := mark()\ndef main()\nputs(APP)\nend\n",
				"value.trb":        "import { BASE } from helper\nVALUE := BASE + 1\ndef later(): Integer\nreturn VALUE\nend\n",
				"helper.trb":       "import { later } from value\ndef mark(): Integer\nputs(\"shared\")\nreturn 6\nend\nBASE := mark()\ndef read(): Integer\nreturn later()\nend\n",
				"a/first_test.trb": moduleTestSource("first"), "b/last_test.trb": moduleTestSource("last"),
			})
			if config.TypeScript != nil {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				if err := config.Save(); err != nil {
					t.Fatal(err)
				}
			}
			for _, selected := range [][]string{{"b/last_test.trb"}, {"b/last_test.trb", "a/first_test.trb"}} {
				var stdout, stderr bytes.Buffer
				command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
				args := []string{"test", "--config", config.Path}
				for _, name := range selected {
					args = append(args, filepath.Join(config.SourcePath(), name))
				}
				if status := command.Run(args); status != 0 {
					t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
				}
				output := stdout.String()
				if strings.Contains(output, "application") || strings.Count(output, "shared\n") != 1 {
					t.Fatalf("unexpected runtime roots: %q", output)
				}
				first, last := strings.Index(output, "initialize-first\n"), strings.Index(output, "initialize-last\n")
				if len(selected) == 1 && (first >= 0 || last < 0) || len(selected) == 2 && (first < 0 || last <= first) {
					t.Fatalf("unexpected root order: %q", output)
				}
				if strings.Index(output, "execute-") < last {
					t.Fatalf("test executed before initialization: %q", output)
				}
			}
		})
	}
}

func TestCyclicGenericClassesKeepRuntimeAndTypeIdentities(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb": `import { VALUE } from b
class Box<T>
 @value: T
 def initialize(value: T)
  @value = value
 end
 def read(): T
  return @value
 end
end
def result(): Integer
 return Box<Integer>.new(VALUE).read()
end
`,
		"b.trb":    "import { result } from a\nVALUE := 7\ndef later(): Integer\nreturn result()\nend\n",
		"main.trb": "import { result } from a\ndef main()\nputs(result())\nend\n",
	}, "7\n", "")
}

func TestCyclicInheritedClassConstantUsesDeclaringOwner(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { Base } from b\nclass Child < Base\nend\nVALUE := Child::NUMBER + 1\n",
		"b.trb":    "import { VALUE } from a\nclass Base\nNUMBER := 7\nend\ndef later(): Integer\nreturn VALUE\nend\n",
		"main.trb": "import { Child, VALUE } from a\ndef main()\nputs(VALUE)\nputs(Child::NUMBER)\nend\n",
	}, "8\n7\n", "")
}

func TestCyclicEnumAliasesWaitForDeclarations(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { VALUE } from b\nenum State\nReady\nend\nalias Status = State\ndef result(): Status\nreturn VALUE\nend\n",
		"b.trb":    "import { Status } from a\nVALUE := Status::Ready\n",
		"main.trb": "import { State, result } from a\ndef main()\nputs(result() == State::Ready)\nend\n",
	}, "true\n", "")
}

func TestCyclicInitializersReadOnlyOmittedParameterDefaults(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { B } from b\ndef choose(value: Integer = B): Integer\nreturn value\nend\nA := choose(7)\n",
		"b.trb":    "import { A, choose } from a\nB := A + 1\nLATER := choose()\n",
		"main.trb": "import { A } from a\nimport { B, LATER } from b\ndef main()\nputs(A)\nputs(B)\nputs(LATER)\nend\n",
	}, "7\n8\n8\n", "")
}

func TestCyclicMethodInitializersReadOnlyOmittedParameterDefaults(t *testing.T) {
	runPortableExecutionFiles(t, map[string]string{
		"a.trb":    "import { B } from b\nclass Item\ndef read(value: Integer = B): Integer\nreturn value\nend\nend\nA := Item.new().read(7)\n",
		"b.trb":    "import { A, Item } from a\nB := A + 1\nLATER := Item.new().read()\n",
		"main.trb": "import { A } from a\nimport { B, LATER } from b\ndef main()\nputs(A)\nputs(B)\nputs(LATER)\nend\n",
	}, "7\n8\n8\n", "")
}
