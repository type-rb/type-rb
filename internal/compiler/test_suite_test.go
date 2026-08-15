package compiler

import (
	"strings"
	"testing"
)

func TestCompilePortableTestSuite(t *testing.T) {
	const testSource = `import { describe, expect, test } from trb/std/test

describe("Calculator") do
	test("adds numbers") do
		expect(1 + 2).to_equal(3)
	end
end
`
	const runnerSource = `import { finish } from trb/std/test
import { trb_test_register_sample } from calculator_test

def main()
	trb_test_register_sample()
	finish()
	return
end
`
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			rootPackage := ""
			if mode == "go" {
				rootPackage = "main"
			}
			artifacts, err := CompileProject([]SourceUnit{
				{Filename: "/project/src/calculator_test.trb", Source: []byte(testSource), ModulePath: "calculator_test", Package: rootPackage, TestRegistration: "trb_test_register_sample"},
				{Filename: "/project/src/__trb_test_main.trb", Source: []byte(runnerSource), ModulePath: "__trb_test_main", Package: rootPackage, CompilerOwned: true},
			}, Options{Mode: mode, GoModule: "example.com/test", SourceRoot: "/project/src", ProjectRoot: "/project", TypeScriptRuntime: "bun"})
			if err != nil {
				t.Fatal(err)
			}
			joined := ""
			for _, artifact := range artifacts {
				joined += string(artifact.Output)
			}
			for _, expected := range []string{"Calculator", "adds numbers"} {
				if !strings.Contains(joined, expected) {
					t.Fatalf("generated %s test suite is missing %q:\n%s", mode, expected, joined)
				}
			}
		})
	}
}

func TestTestRunnerProtocolIsNotAUserImport(t *testing.T) {
	_, err := Compile("example_test.trb", []byte("import { finish } from trb/std/test\n"), "go")
	if err == nil || !strings.Contains(err.Error(), "finish is internal to the TypeRB compiler") {
		t.Fatalf("unexpected finish import result: %v", err)
	}
}
