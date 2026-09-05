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

func TestNilAssertionsDistinguishNullableAndPresentValuesAcrossBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			runtime := map[string]string{"go": "go", "ruby": "ruby", "typescript": "bun"}[mode]
			if _, err := exec.LookPath(runtime); err != nil {
				t.Skipf("%s is unavailable: %v", runtime, err)
			}
			config := project.New(t.TempDir(), mode)
			config.SourceDir = "src"
			if mode == "go" {
				config.Go.Module = "example.com/nil-assertions"
			}
			if mode == "typescript" {
				config.TypeScript.Runtime = project.TypeScriptRuntimeBun
				config.TypeScript.PackageManager = "bun"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(config.SourcePath(), 0o755); err != nil {
				t.Fatal(err)
			}
			source := `import { describe, expect, test } from trb/std/test

record Point
	x: Integer
end

describe("Nil") do
	test("literal nil") do
		expect(nil).to_be_nil()
	end

	test("nullable scalar") do
		value: String? := nil
		expect(value).to_be_nil()
	end

	test("nullable record") do
		value: Point? := nil
		expect(value).to_be_nil()
	end

	test("nullable array") do
		value: Array<Integer>? := nil
		expect(value).to_be_nil()
	end

	test("present optional") do
		value: String? := "present"
		expect(value).to_be_nil()
	end

	test("false value") do
		expect(false).to_be_nil()
	end

	test("empty bytes") do
		expect("".to_bytes()).to_be_nil()
	end

	test("empty array") do
		value: Array<Integer> := []
		expect(value).to_be_nil()
	end
end
`
			if err := os.WriteFile(filepath.Join(config.SourcePath(), "nil_test.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"test", "--config", config.Path}); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			for _, want := range []string{
				"PASS Nil / literal nil", "PASS Nil / nullable scalar", "PASS Nil / nullable record", "PASS Nil / nullable array",
				"FAIL Nil / present optional", "FAIL Nil / false value", "FAIL Nil / empty bytes", "FAIL Nil / empty array",
				"8 test(s), 4 failure(s)",
			} {
				if !strings.Contains(stdout.String(), want) {
					t.Fatalf("missing %q in stdout=%s stderr=%s", want, stdout.String(), stderr.String())
				}
			}
		})
	}
}
