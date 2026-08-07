package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/project"
)

func TestVersionCommandUsesBuildVersion(t *testing.T) {
	previous := Version
	Version = "9.8.7-test"
	t.Cleanup(func() { Version = previous })

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"version"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "trb 9.8.7-test\n" {
		t.Fatalf("unexpected version output %q", stdout.String())
	}
}

func TestPlaygroundModeSelection(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		selected, err := playgroundMode(mode)
		if err != nil || selected != mode {
			t.Fatalf("playgroundMode(%q) = %q, %v", mode, selected, err)
		}
	}
	if _, err := playgroundMode("python"); err == nil || !strings.Contains(err.Error(), "--mode must be") {
		t.Fatalf("unexpected invalid-mode result: %v", err)
	}
}

func TestTourCheckRejectsServerFlags(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"tour", "--check", "--mode", "go"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "tour --check cannot be combined") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestReplUsesProjectModeKeepsStateAndLoadsProjectImports(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "src", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelPath, []byte("record User\n  name: String\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	input := strings.Join([]string{
		"import trb/std/strings",
		"import { User } from models/user",
		`name := "Ada"`,
		"strings.uppercase(name)",
		"user := User.new(name: name)",
		"user.name",
		":type user",
		"name = 1",
		"name",
		":quit",
	}, "\n") + "\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := strings.Join([]string{
		`"Ada" : String`,
		`"ADA" : String`,
		`User(name: "Ada") : User`,
		`"Ada" : String`,
		`User`,
		`"Ada" : String`,
		"",
	}, "\n")
	if stdout.String() != want {
		t.Fatalf("unexpected REPL output\nwant:\n%s\ngot:\n%s", want, stdout.String())
	}
	if !strings.Contains(stderr.String(), "cannot assign Integer to String") {
		t.Fatalf("REPL did not report and recover from the type error:\n%s", stderr.String())
	}
}

func TestReplSupportsPreludeAndNamespacedPutsForAnyValue(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-puts-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "puts(1 + 2)\nimport trb/std/io\nio.puts([1, 2])\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "3\n[1, 2]\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesPortableReceiverMethodsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-receiver-method-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "123.to_s()\n0.25.to_s()\n(-0.0).to_s()\n2.to_f()\n(-2.75).to_i()\n(-4).abs()\n0.zero?()\n1.positive?()\n(-1).negative?()\n2.even?()\n3.odd?()\n(-0.25).abs()\n0.25.finite?()\ntrue.to_s()\nfalse.to_s()\n0.25 * 100\n1 == 1.0\n\"123\".to_i()\n\"123\".try_to_i()\n\"12x\".try_to_i()\n\"9007199254740992\".try_to_i()\n\"a😀\".size()\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"123\" : String\n\"0.25\" : String\n\"0.0\" : String\n2 : Float\n-2 : Integer\n4 : Integer\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\n0.25 : Float\ntrue : Boolean\n\"true\" : String\n\"false\" : String\n25 : Float\ntrue : Boolean\n123 : Integer\nResult::Ok(value: 123) : Result<Integer, String>\nResult::Err(error: \"invalid Integer\") : Result<Integer, String>\nResult::Err(error: \"Integer is outside the portable range\") : Result<Integer, String>\n2 : Integer\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s receiver-method REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplRetainsPredicateAndBangFunctionNamesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-suffixed-name-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "def ready?(): Boolean; return true; end\n" +
			"def save!(): String; return \"saved\"; end\n" +
			"ready?()\n" +
			"save!()\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "true : Boolean\n\"saved\" : String\n"; stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s suffixed-name REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableStringTrimmingAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-string-trimming-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "\"\\t\\u00a0\\u3000 TypeRB \\u0085\\n\".strip()\n" +
			"\"\\t\\u3000TypeRB\".lstrip()\n" +
			"\"TypeRB\\u00a0\\u3000\".rstrip()\n" +
			"\" \\ufeffTypeRB\\ufeff \".strip() == \"\\ufeffTypeRB\\ufeff\"\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "\"TypeRB\" : String\n\"TypeRB\" : String\n\"TypeRB\" : String\ntrue : Boolean\n"; stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s String trimming REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableBytesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-bytes-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "value := \"A😀\".to_bytes()\nvalue\nvalue.size()\nvalue.at(1)\nvalue.to_s()\nvalue.valid_utf8()\nvalue.concat(\"!\".to_bytes()).to_s()\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Bytes[65, 240, 159, 152, 128] : Bytes\nBytes[65, 240, 159, 152, 128] : Bytes\n5 : Integer\n240 : Integer\n\"A😀\" : String\ntrue : Boolean\n\"A😀!\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Bytes REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableStringBuilderAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-string-builder-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/string_builder\nmut builder := string_builder.from_string(\"A\")\nbuilder.empty?()\nbuilder.append(\"😀\")\nbuilder.append_codepoint(33)\nbuilder\nbuilder.size()\nsnapshot := builder.to_s()\nbuilder.clear()\nbuilder.empty?()\nbuilder.to_s()\nsnapshot\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "StringBuilder(\"A\") : StringBuilder\nfalse : Boolean\nStringBuilder(\"A😀!\") : StringBuilder\n3 : Integer\n\"A😀!\" : String\ntrue : Boolean\n\"\" : String\n\"A😀!\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s StringBuilder REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArrayAndHashOperationsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-collections-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/arrays\nimport trb/std/hashes\nmut numbers := [1, 2]\nnumbers.first()\nnumbers.last()\nnumbers.fetch(1)\nnumbers.try_fetch(1)\nmissing := numbers.try_fetch(9)\nnumbers.empty?()\nnumbers.dup()\narrays.push(numbers, 3)\nnumbers\nnumbers.shift()\nnumbers.unshift(0)\narrays.reverse(numbers)\nnumbers\narrays.shift(numbers)\narrays.unshift(numbers, 1)\nnumbers.reverse()\nnumbers\nnumbers.include?(2)\nnumbers.count(2)\narrays.contains(numbers, 9)\narrays.count(numbers, 1)\nlabels: Hash<Integer, String> := {1 => \"one\", 2 => \"two\"}\nlabels.fetch(2)\nlabels.try_fetch(2)\nlabels.try_fetch(9)\nlabels.key?(3)\nlabels.keys()\nlabels.values()\nhashes.copy(labels)\nlabels.merge({2 => \"TWO\", 3 => \"three\"})\nlabels\nmut editable := labels.dup()\neditable.update({2 => \"TWO\", 3 => \"three\"})\nhashes.update(editable, {4 => \"four\"})\neditable.delete(1)\nhashes.delete(editable, 2)\neditable\n\"a/b/\".split(\"/\")\n\"TypeRB\".start_with?(\"Type\")\n\"TypeRB\".end_with?(\"RB\")\nmut words := [\"root\", \"leaf\"]\nwords.pop()\nwords.join(\"/\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[1, 2] : Array<Integer>\n1 : Integer\n2 : Integer\n2 : Integer\nResult::Ok(value: 2) : Result<Integer, String>\nResult::Err(error: \"Array index is out of bounds\") : Result<Integer, String>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[1, 2, 3] : Array<Integer>\n1 : Integer\n[3, 2, 0] : Array<Integer>\n[0, 2, 3] : Array<Integer>\n0 : Integer\n[3, 2, 1] : Array<Integer>\n[1, 2, 3] : Array<Integer>\ntrue : Boolean\n1 : Integer\nfalse : Boolean\n1 : Integer\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"two\" : String\nResult::Ok(value: \"two\") : Result<String, String>\nResult::Err(error: \"Hash key is missing\") : Result<String, String>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[\"one\", \"two\"] : Array<String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"TWO\", 3: \"three\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"one\" : String\n\"TWO\" : String\n{3: \"three\", 4: \"four\"} : Hash<Integer, String>\n[\"a\", \"b\", \"\"] : Array<String>\ntrue : Boolean\ntrue : Boolean\n[\"root\", \"leaf\"] : Array<String>\n\"leaf\" : String\n\"root\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Array/Hash REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableCollectionTransformationsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-collection-transformation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "[1, 2, 3].map { |value| value * 2 }\n[1, 2, 3].select.with_index { |value, index| value > 1 and index < 2 }\n[1, 2, 3].reduce(10) { |sum, value| sum + value }\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[2, 4, 6] : Array<Integer>\n[2] : Array<Integer>\n16 : Integer\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s collection-transformation REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortablePathAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-path-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/path\npath.separator()\npath.clean(\"a/./b/../c\")\npath.clean(\"/../../srv//app\")\npath.join(\"/srv/app\", \"../data\")\npath.absolute(\"/srv/app\")\npath.components(\"/srv/app/main.trb\")\npath.base(\"/srv/app/main.trb\")\npath.directory(\"/srv/app/main.trb\")\npath.join(\"\", \"\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"/\" : String\n\"a/c\" : String\n\"/srv/app\" : String\n\"/srv/data\" : String\ntrue : Boolean\n[\"srv\", \"app\", \"main.trb\"] : Array<String>\n\"main.trb\" : String\n\"/srv/app\" : String\n\".\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s path REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableFilesystemAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-filesystem-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		directory := filepath.Join(root, "data")
		textPath := filepath.Join(directory, "note.txt")
		bytesPath := filepath.Join(directory, "value.bin")
		missingPath := filepath.Join(directory, "missing.txt")
		input := strings.Join([]string{
			"import { FileError, create_directory, exists, list, read_bytes, read_text, write_bytes, write_text } from trb/std/filesystem",
			"import { Result } from trb/std/result",
			"def describe(value: Result<String, FileError>): String; case value; when Result::Ok(text); return text; when Result::Err(error); return error.operation; end; end",
			"create_directory(" + strconv.Quote(directory) + ")",
			"write_text(" + strconv.Quote(textPath) + ", \"A😀\")",
			"read_text(" + strconv.Quote(textPath) + ")",
			"exists(" + strconv.Quote(textPath) + ")",
			"exists(" + strconv.Quote(missingPath) + ")",
			"list(" + strconv.Quote(directory) + ")",
			"write_bytes(" + strconv.Quote(bytesPath) + ", \"B\".to_bytes())",
			"read_bytes(" + strconv.Quote(bytesPath) + ")",
			"describe(read_text(" + strconv.Quote(missingPath) + "))",
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: \"A😀\") : Result<String, FileError>\n" +
			"Result::Ok(value: true) : Result<Boolean, FileError>\n" +
			"Result::Ok(value: false) : Result<Boolean, FileError>\n" +
			"Result::Ok(value: [\"note.txt\"]) : Result<Array<String>, FileError>\n" +
			"Result::Ok(value: Unit()) : Result<Unit, FileError>\n" +
			"Result::Ok(value: Bytes[66]) : Result<Bytes, FileError>\n" +
			"\"read_text\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s filesystem REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableProcessAcrossModes(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-process-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("TRB_PROCESS_REPL_TEST", "available")
		input := "import trb/std/process\n" +
			"import { Result } from trb/std/result\n" +
			"def describe(value: Result<ProcessResult, ProcessError>): String; case value; when Result::Ok(result); return result.status.to_s() + \":\" + result.stdout + \":\" + result.stderr; when Result::Err(error); return error.operation; end; end\n" +
			"def operation(value: Result<ProcessResult, ProcessError>): String; case value; when Result::Ok(result); return result.stdout; when Result::Err(error); return error.operation; end; end\n" +
			"process.argv()\n" +
			"process.environment(\"TRB_PROCESS_REPL_TEST\")\n" +
			"describe(process.run(\"/bin/sh\", [\"-c\", \"printf out; printf err >&2; exit 3\"]))\n" +
			"empty_args: Array<String> := []\n" +
			"operation(process.run(\"/type-rb-command-that-does-not-exist\", empty_args))\n" +
			":quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[] : Array<String>\n\"available\" : String?\n\"3:out:err\" : String\n[] : Array<String>\n\"run\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s process REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableJSONAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { JsonError, JsonValue, as_string, parse, stringify } from trb/std/json",
			"import trb/std/jsonc",
			"import { Result } from trb/std/result",
			`parse("1")`,
			`parse("1.5")`,
			`parse("{\"name\":\"Ada\",\"enabled\":true}")`,
			`jsonc.parse("{\n  // comment\n  \"name\": \"Ada\"\n}")`,
			`stringify(JsonValue::Object({"name" => JsonValue::String("Ada")}))`,
			`as_string(JsonValue::Integer(1))`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: JsonValue::Integer(value: 1)) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Float(value: 1.5)) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Object(value: {\"enabled\": JsonValue::Boolean(value: true), \"name\": JsonValue::String(value: \"Ada\")})) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: JsonValue::Object(value: {\"name\": JsonValue::String(value: \"Ada\")})) : Result<JsonValue, JsonError>\n" +
			"Result::Ok(value: \"{\\\"name\\\":\\\"Ada\\\"}\") : Result<String, JsonError>\n" +
			"Result::Err(error: JsonError(kind: JsonErrorKind::Decode, message: \"JSON value is not String\", path: \"\", line: nil, column: nil)) : Result<String, JsonError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s JSON REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesTypedJSONRecordCodecsAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-json-codec-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := strings.Join([]string{
			"import { JsonError, decode, encode } from trb/std/json",
			"import { Result } from trb/std/result",
			`record User; id: Integer @json("user_id"); name: String; end`,
			`decode<User>("{\"user_id\":1,\"name\":\"Ada\"}")`,
			`encode(User.new(id: 2, name: "Lin"))`,
			`decode<User>("{\"user_id\":1}")`,
			":quit",
		}, "\n") + "\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Result::Ok(value: User(id: 1, name: \"Ada\")) : Result<User, JsonError>\n" +
			"Result::Ok(value: \"{\\\"name\\\":\\\"Lin\\\",\\\"user_id\\\":2}\") : Result<String, JsonError>\n" +
			"Result::Err(error: JsonError(kind: JsonErrorKind::Decode, message: \"missing field name\", path: \"/name\", line: nil, column: nil)) : Result<User, JsonError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s typed JSON codec REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesCompilerOwnedUnicodeAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-unicode-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/unicode\nunicode.version()\nunicode.letter(12354)\nunicode.digit(1632)\nunicode.identifier_start(64)\nunicode.valid_scalar(55296)\nunicode.from_codepoint(128512)\n\"A😀\".codepoints()\n\"\".empty?()\n\"TypeRB\".include?(\"RB\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"15.0.0\" : String\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\nfalse : Boolean\n\"😀\" : String\n[65, 128512] : Array<Integer>\ntrue : Boolean\ntrue : Boolean\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s Unicode REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableArithmeticSemantics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-operators-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "-5 / 2\n-5 % 2\n2 ** 3\n(1 + 2) * 3\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "-2 : Integer\n-1 : Integer\n8 : Integer\n9 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesBreakAndNext(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-loop-control-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `mut total := 0
[1, 2, 3, 4].each do |value|
  if value == 2
    next
  end
  if value == 4
    break
  end
  total += value
end
total
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "0 : Integer\n4 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesTypedHashAndReportsMissingKeys(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-hash-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `mut scores: Hash<String, Integer> := {"one" => 1}
scores["one"]
scores["two"] = 2
scores["two"]
scores["missing"]
scores["one"]
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "{\"one\": 1} : Hash<String, Integer>\n1 : Integer\n2 : Integer\n2 : Integer\n1 : Integer\n"
	if stdout.String() != want {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "Hash key is missing") {
		t.Fatalf("REPL did not report and recover from missing Hash key:\n%s", stderr.String())
	}
}

func TestReplInfersCommonNumericCollectionTypesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-common-collection-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "numbers := [1, 2.5]\nnumbers\nvalues := { integer: 1, float: 2.5 }\nvalues\nvalues[:integer]\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "[1, 2.5] : Array<Float>\n[1, 2.5] : Array<Float>\n{\"integer\": 1, \"float\": 2.5} : Hash<String, Float>\n{\"integer\": 1, \"float\": 2.5} : Hash<String, Float>\n1 : Float\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s common collection REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplInfersAndNarrowsUnionTypesAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-union-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := `def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end
describe(3)
describe("Ada")
values := [1, "two"]
values
fields := { count: 1, name: "Ada" }
fields
fields[:count]
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"3\" : String\n\"Ada\" : String\n[1, \"two\"] : Array<Integer | String>\n[1, \"two\"] : Array<Integer | String>\n{\"count\": 1, \"name\": \"Ada\"} : Hash<String, Integer | String>\n{\"count\": 1, \"name\": \"Ada\"} : Hash<String, Integer | String>\n1 : Integer | String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s union REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestRunUnionTypesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby union run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript union run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-union-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `def describe(value: Integer | String): String
	case value
	when Integer(number)
		return number.to_s()
	when String(text)
		return text
	end
end

def widen(value: Integer | String): Float | String
	return value
end

def describe_wide(value: Float | String): String
	case value
	when Float(number)
		return number.to_s()
	when String(text)
		return text
	end
end

def main()
	values := [1, "Ada"]
	values.each do |value|
		puts(describe(value))
	end
	fields := { count: 2, name: "Grace" }
	puts(describe(fields[:count]))
	puts(describe(fields[:name]))
	puts(describe_wide(widen(1)))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "1\nAda\n2\nGrace\n1.0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s union output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestReplRejectsPlatformPackageForConfiguredModeAndContinues(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-mode-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{
		Stdin:  strings.NewReader("import trb/platform/typescript/node\n1 + 1\n:quit\n"),
		Stdout: &stdout,
		Stderr: &stderr,
	}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if stdout.String() != "2 : Integer\n" {
		t.Fatalf("unexpected output %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "does not support mode go") {
		t.Fatalf("configured mode was not enforced:\n%s", stderr.String())
	}
}

func TestReplEvaluatesMultilineClassThroughTypedIR(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-class-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	input := "class Box\n" +
		"  @value: Integer\n\n" +
		"  def initialize(value: Integer)\n" +
		"    @value = value\n" +
		"    return\n" +
		"  end\n\n" +
		"  def value(): Integer\n" +
		"    return @value\n" +
		"  end\n" +
		"end\n" +
		"box := Box.new(4)\n" +
		"box.value() + 1\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "#<Box value: 4> : Box\n5 : Integer\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nstdout:\n%s\nstderr:\n%s", stdout.String(), stderr.String())
	}
}

func TestReplResolvesClassConstantsInsideInstanceAndClassMethods(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-class-constant-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	classSource := "class Config\n" +
		"  DEFAULT_NAME := \"TypeRB\"\n" +
		"  def name(): String\n" +
		"    return DEFAULT_NAME\n" +
		"  end\n" +
		"  def self.default_name(): String\n" +
		"    return DEFAULT_NAME\n" +
		"  end\n" +
		"end\n"
	if err := os.WriteFile(filepath.Join(root, "src", "config.trb"), []byte(classSource), 0o644); err != nil {
		t.Fatal(err)
	}
	input := "import { Config } from config\n" +
		"config := Config.new()\n" +
		"config.name()\n" +
		"Config.default_name()\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "#<Config > : Config\n\"TypeRB\" : String\n\"TypeRB\" : String\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesSemicolonSeparatedDeclarations(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-separator-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum State; Open; Closed; end\n" +
		"def label(state: State): String; case state; when State::Open; return \"open\"; when State::Closed; return \"closed\"; end; end\n" +
		"label(State::Closed)\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if want := "\"closed\" : String\n"; stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesPayloadEnumPatternBindings(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-payload-enum-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum Token; Text(value: String); EOF; end\n" +
		"def render(token: Token): String; case token; when Token::Text(value); return value; when Token::EOF; return \"eof\"; end; end\n" +
		"render(Token::Text(\"Ada\"))\n" +
		"Token::Text(\"Ada\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "\"Ada\" : String\nToken::Text(value: \"Ada\") : Token\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesExplicitUserGenerics(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-generics-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "enum Result<T, E>; Ok(value: T); Err(error: E); end\n" +
		"def identity<T>(value: T): T; return value; end\n" +
		"def unwrap(value: Result<Integer, String>): Integer; case value; when Result::Ok(number); return number; when Result::Err(_); return 0; end; end\n" +
		"unwrap(Result<Integer, String>::Ok(identity<Integer>(7)))\n" +
		"identity<String>(\"Ada\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "7 : Integer\n\"Ada\" : String\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected generic REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplEvaluatesStandardResult(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-result-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := "import { Result } from trb/std/result\n" +
		"def unwrap(value: Result<Integer, String>): Integer; case value; when Result::Ok(number); return number; when Result::Err(_); return 0; end; end\n" +
		"unwrap(Result<Integer, String>::Ok(7))\n" +
		"Result<Integer, String>::Err(\"missing\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	want := "7 : Integer\nResult::Err(error: \"missing\") : Result<Integer, String>\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected standard Result REPL output\nwant:\n%s\ngot:\n%s\nstderr:\n%s", want, stdout.String(), stderr.String())
	}
}

func TestReplDefaultsToGoWithoutProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "broken.trb"), []byte("if\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader("import trb/platform/go/context\n1 + 2\n:quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "3 : Integer\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected configless REPL output\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestReplModeFlagWorksWithoutProjectConfiguration(t *testing.T) {
	for _, test := range []struct {
		mode        string
		packagePath string
	}{
		{mode: "go", packagePath: "trb/platform/go/context"},
		{mode: "ruby", packagePath: "trb/platform/ruby/rails"},
		{mode: "typescript", packagePath: "trb/platform/typescript/node"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			root := t.TempDir()
			previous, err := os.Getwd()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Chdir(root); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = os.Chdir(previous) }()

			input := "import " + test.packagePath + "\n1\n:quit\n"
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run([]string{"repl", "--mode", test.mode}); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != "1 : Integer\n" || stderr.Len() != 0 {
				t.Fatalf("unexpected %s REPL output\nstdout=%s\nstderr=%s", test.mode, stdout.String(), stderr.String())
			}
		})
	}
}

func TestReplModeFlagOverridesProjectMode(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.Go.Module = "example.com/type-rb/repl-mode-override"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}

	input := "import trb/platform/ruby/rails\n1\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path, "--mode", "ruby"}); status != 0 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if stdout.String() != "1 : Integer\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected overridden REPL output\nstdout=%s\nstderr=%s", stdout.String(), stderr.String())
	}
}

func TestReplRejectsInvalidMode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(":quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "python"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "repl --mode must be ruby, go, or typescript") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestReplDoesNotIgnoreMissingExplicitConfig(t *testing.T) {
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(":quit\n"), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", "missing.jsonc", "--mode", "ruby"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "missing.jsonc") {
		t.Fatalf("unexpected error: %s", stderr.String())
	}
}

func TestBuildCopiesRailsProjectAndTranspilesTRBTree(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(root, "ruby")
	config.Dependencies["rails"] = "~> 8.0"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	routes := "import trb/platform/ruby/rails\n\nRails.application.routes.draw do\n  resources :posts\nend\n"
	if err := os.WriteFile(filepath.Join(root, "config", "routes.trb"), []byte(routes), 0o644); err != nil {
		t.Fatal(err)
	}
	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(root, "build", "Gemfile")); err != nil {
		t.Fatalf("Gemfile was not copied: %v", err)
	}
	gemfile, err := os.ReadFile(filepath.Join(root, "Gemfile"))
	if err != nil || !strings.Contains(string(gemfile), `gem "rails", "~> 8.0"`) {
		t.Fatalf("Gemfile was not managed from config: err=%v\n%s", err, gemfile)
	}
	generated, err := os.ReadFile(filepath.Join(root, "build", "config", "routes.rb"))
	if err != nil {
		t.Fatalf("routes were not generated: %v", err)
	}
	if strings.Contains(string(generated), "mode: ruby") || !strings.Contains(string(generated), "resources :posts") {
		t.Fatalf("unexpected generated routes:\n%s", generated)
	}
}

func TestBuildCompileCreatesRunnableGoExecutable(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go is not installed")
	}
	for _, test := range []struct {
		name     string
		outfile  string
		relative string
	}{
		{name: "default", relative: filepath.Join("bin", "hello-default")},
		{name: "outfile", outfile: filepath.Join("dist", "hello"), relative: filepath.Join("dist", "hello")},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, "go")
			config.Name = "hello-" + test.name
			config.SourceDir = "src"
			config.Go.Module = "example.com/type-rb/" + config.Name
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
				t.Fatal(err)
			}
			source := "def main()\n  puts(\"Hello compiled\")\n  return\nend\n"
			if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
			t.Setenv("CGO_ENABLED", "0")

			args := []string{"build", "--config", config.Path, "--compile"}
			if test.outfile != "" {
				args = append(args, "--outfile", test.outfile)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			output := filepath.Join(root, test.relative)
			if runtime.GOOS == "windows" {
				output += ".exe"
			}
			info, err := os.Stat(output)
			if err != nil || info.IsDir() {
				t.Fatalf("executable was not created at %s: info=%v err=%v", output, info, err)
			}
			if want := "executable -> " + output + "\n"; stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected build output\nwant: %q\nstdout: %q\nstderr: %q", want, stdout.String(), stderr.String())
			}
			result, err := exec.Command(output).CombinedOutput()
			if err != nil || string(result) != "Hello compiled\n" {
				t.Fatalf("compiled executable failed: err=%v output=%q", err, result)
			}
			if _, err := os.Stat(filepath.Join(root, "build", "main.go")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("--compile retained generated source: %v", err)
			}
		})
	}
}

func TestBuildCompileValidatesModeAndFlags(t *testing.T) {
	for _, test := range []struct {
		name string
		mode string
		args []string
		want string
	}{
		{name: "ruby", mode: "ruby", args: []string{"--compile"}, want: "--compile is supported only for mode go"},
		{name: "typescript", mode: "typescript", args: []string{"--compile"}, want: "--compile is supported only for mode go"},
		{name: "outfile", mode: "go", args: []string{"--outfile", "bin/app"}, want: "--outfile requires --compile"},
		{name: "check", mode: "go", args: []string{"--compile", "--check"}, want: "--compile cannot be combined"},
		{name: "path", mode: "go", args: []string{"--compile", "."}, want: "--compile builds the configured project"},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			config := project.New(root, test.mode)
			if config.Go != nil {
				config.Go.Module = "example.com/type-rb/compile-flags"
			}
			if err := config.Save(); err != nil {
				t.Fatal(err)
			}
			args := append([]string{"build", "--config", config.Path}, test.args...)
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(args); status != 1 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if !strings.Contains(stderr.String(), test.want) {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
		})
	}
}

func TestBuildCompileRequiresMain(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/library"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "library.trb"), []byte("def value(): Integer\n  return 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path, "--compile"}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "project has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestRunCompilesProjectImportClosure(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/acme/import-run"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	modelPath := filepath.Join(root, "src", "models", "user.trb")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatal(err)
	}
	model := "class User\n  @name: String\n\n  def initialize(name: String)\n    @name = name\n    return\n  end\n\n  def name(): String\n    return @name\n  end\nend\n"
	if err := os.WriteFile(modelPath, []byte(model), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(root, "src", "main.trb")
	main := "import trb/std/io\nimport models/user\n\ndef main()\n  user := User.new(\"Imported\")\n  io.puts(user.name())\n  return\nend\n"
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	previous, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(previous) }()
	t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
	t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))

	for _, args := range [][]string{{"run"}, {"run", mainPath}} {
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run(args); status != 0 {
			t.Fatalf("args=%v status=%d stderr=%s", args, status, stderr.String())
		}
		if stdout.String() != "Imported\n" {
			t.Fatalf("args=%v unexpected program output %q", args, stdout.String())
		}
	}
	matches, err := filepath.Glob(filepath.Join(root, "trb-run-*"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("run directory leaked: matches=%v err=%v", matches, err)
	}
}

func TestRunPredicateAndBangNamesAcrossAvailableBackends(t *testing.T) {
	files := map[string]string{
		"contracts/capability.trb": "interface Capability\n\tready?(): Boolean\n\tsave!(): String\nend\n",
		"helpers/functions.trb": "def imported_ready?(): Boolean\n\treturn true\nend\n\n" +
			"def imported_save!(): String\n\treturn \"imported\"\nend\n\n" +
			"def imported_label?(): String\n\treturn \"question\"\nend\n",
		"models/base.trb": "import { Capability } from contracts/capability\n\n" +
			"class Base implements Capability\n" +
			"\tdef ready?(): Boolean\n\t\treturn true\n\tend\n\n" +
			"\tdef save!(): String\n\t\treturn \"base\"\n\tend\n\n" +
			"\tdef self.available?(): Boolean\n\t\treturn true\n\tend\n" +
			"end\n\n" +
			"def base_available?(): Boolean\n\treturn Base.available?()\nend\n",
		"models/child.trb": "import { Base, base_available? } from models/base\n\n" +
			"class Child < Base\n" +
			"\tdef ready?(): Boolean\n\t\treturn true\n\tend\n\n" +
			"\tdef child_ready?(): Boolean\n\t\treturn self.ready?()\n\tend\n" +
			"\n\tdef inherited_available?(): Boolean\n\t\treturn base_available?()\n\tend\n" +
			"end\n",
		"main.trb": "import { imported_ready?, imported_save!, imported_label? } from helpers/functions\n" +
			"import { base_available? } from models/base\n" +
			"import { Child } from models/child\n\n" +
			"def local_ready?(): Boolean\n\treturn true\nend\n\n" +
			"def local_save!(): String\n\treturn \"local\"\nend\n\n" +
			"def main()\n" +
			"\tchild := Child.new()\n" +
			"\tputs(local_ready?())\n" +
			"\tputs(local_save!())\n" +
			"\tputs(imported_ready?())\n" +
			"\tputs(imported_save!())\n" +
			"\tputs(imported_label?())\n" +
			"\tputs(base_available?())\n" +
			"\tputs(child.ready?())\n" +
			"\tputs(child.save!())\n" +
			"\tputs(child.child_ready?())\n" +
			"\tputs(child.inherited_available?())\n" +
			"\treturn\n" +
			"end\n",
	}

	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby suffixed-name run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript suffixed-name run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/suffixed-name-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		for name, source := range files {
			filename := filepath.Join(root, "src", filepath.FromSlash(name))
			if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\nlocal\ntrue\nimported\nquestion\ntrue\ntrue\nbase\ntrue\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s suffixed-name output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableStringTrimmingAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby String trimming run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript String trimming run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/string-trimming-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "def main()\n" +
			"\tputs(\"\\t\\u00a0\\u3000 TypeRB \\u0085\\n\".strip())\n" +
			"\tputs(\"\\t\\u3000TypeRB\".lstrip())\n" +
			"\tputs(\"TypeRB\\u00a0\\u3000\".rstrip())\n" +
			"\tputs(\" \\ufeffTypeRB\\ufeff \".strip() == \"\\ufeffTypeRB\\ufeff\")\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "TypeRB\nTypeRB\nTypeRB\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s String trimming output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedUnicodeAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby Unicode run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript Unicode run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-unicode-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import trb/std/unicode\n\ndef main()\n\tputs(unicode.version())\n\tputs(unicode.letter(12354))\n\tputs(unicode.from_codepoint(128512))\n\treturn\nend\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "15.0.0\ntrue\n😀\n"; stdout.String() != want {
			t.Fatalf("unexpected %s Unicode program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunSafePortableConversionAndLookupAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby safe-operation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript safe-operation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-safe-operation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import { Result } from trb/std/result\n" +
			"import trb/std/string_builder\n" +
			"import trb/std/numbers\n" +
			"import trb/std/booleans\n\n" +
			"def integer_result(value: Result<Integer, String>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error; end; end\n" +
			"def string_result(value: Result<String, String>): String; case value; when Result::Ok(text); return \"ok:\" + text; when Result::Err(error); return \"err:\" + error; end; end\n" +
			"def scalar_check(value: Float): Boolean; return (-4).abs() == numbers.absolute(-4) && 0.zero?() && 1.positive?() && (-1).negative?() && 2.even?() && 3.odd?() && (-0.25).abs() == 0.25 && (value.finite?() || value.infinite?() || value.nan?()) && true.to_s() == booleans.to_string(true); end\n\n" +
			"def main()\n" +
			"\tputs(scalar_check(0.25))\n" +
			"\tputs(integer_result(\"12\".try_to_i()))\n" +
			"\tputs(integer_result(\"12x\".try_to_i()))\n" +
			"\tputs(integer_result(\"9007199254740992\".try_to_i()))\n" +
			"\tvalues := [7]\n" +
			"\tputs(integer_result(values.try_fetch(0)))\n" +
			"\tputs(integer_result(values.try_fetch(1)))\n" +
			"\tlabels: Hash<String, String> := {\"name\" => \"Ada\"}\n" +
			"\tputs(string_result(labels.try_fetch(\"name\")))\n" +
			"\tputs(string_result(labels.try_fetch(\"missing\")))\n" +
			"\tbuilder := string_builder.new()\n" +
			"\tputs(builder.empty?())\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\nok:12\nerr:invalid Integer\nerr:Integer is outside the portable range\nok:7\nerr:Array index is out of bounds\nok:Ada\nerr:Hash key is missing\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s safe-operation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableCollectionTransformationsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby collection-transformation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript collection-transformation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-collection-transformation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `def main()
	mapped := [1, 2, 3].map do |value|
		value * 2
	end
	selected := mapped.select.with_index do |value, index|
		value > 2 and index < 2
	end
	total := selected.reduce(10) do |sum, value|
		sum + value
	end
	puts(mapped.fetch(2))
	puts(selected.size())
	puts(total)
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "6\n1\n14\n"; stdout.String() != want {
			t.Fatalf("unexpected %s collection-transformation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedFilesystemAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby filesystem run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript filesystem run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-filesystem-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		directory := filepath.Join(root, "data")
		textPath := filepath.Join(directory, "note.txt")
		missingPath := filepath.Join(directory, "missing.txt")
		bmpPath := filepath.Join(directory, "\uE000")
		astralPath := filepath.Join(directory, "\U00010000")
		source := "import { FileError, create_directory, exists, list, read_text, write_text } from trb/std/filesystem\n" +
			"import { Result } from trb/std/result\n\n" +
			"def text_or_operation(value: Result<String, FileError>): String; case value; when Result::Ok(text); return text; when Result::Err(error); return error.operation; end; end\n" +
			"def names_or_error(value: Result<Array<String>, FileError>): Array<String>; case value; when Result::Ok(names); return names; when Result::Err(error); return [error.operation]; end; end\n" +
			"def boolean_or_false(value: Result<Boolean, FileError>): Boolean; case value; when Result::Ok(found); return found; when Result::Err(error); return error.operation.empty?(); end; end\n\n" +
			"def main()\n" +
			"\tcreate_directory(" + strconv.Quote(directory) + ")\n" +
			"\twrite_text(" + strconv.Quote(textPath) + ", \"A😀\")\n" +
			"\twrite_text(" + strconv.Quote(astralPath) + ", \"\")\n" +
			"\twrite_text(" + strconv.Quote(bmpPath) + ", \"\")\n" +
			"\tputs(text_or_operation(read_text(" + strconv.Quote(textPath) + ")))\n" +
			"\tputs(text_or_operation(read_text(" + strconv.Quote(missingPath) + ")))\n" +
			"\tputs(names_or_error(list(" + strconv.Quote(directory) + ")).join(\",\"))\n" +
			"\tputs(boolean_or_false(exists(" + strconv.Quote(textPath) + ")))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "A😀\nread_text\nnote.txt,\uE000,\U00010000\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s filesystem program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedProcessAcrossAvailableBackends(t *testing.T) {
	if _, err := os.Stat("/bin/sh"); err != nil {
		t.Skip("/bin/sh is unavailable")
	}
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby process run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript process run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-process-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { ProcessError, ProcessResult, argv, run, working_directory } from trb/std/process
import { Result } from trb/std/result

def describe(value: Result<ProcessResult, ProcessError>): String
	case value
	when Result::Ok(result)
		return result.status.to_s() + ":" + result.stdout + ":" + result.stderr
	when Result::Err(error)
		return "error:" + error.operation
	end
end

def succeeded(value: Result<ProcessResult, ProcessError>): Boolean
	case value
	when Result::Ok(result)
		return result.success
	when Result::Err(error)
		return error.message.empty?()
	end
end

def operation(value: Result<ProcessResult, ProcessError>): String
	case value
	when Result::Ok(result)
		return result.stdout
	when Result::Err(error)
		return error.operation
	end
end

def directory_available(value: Result<String, ProcessError>): Boolean
	case value
	when Result::Ok(directory)
		return !directory.empty?()
	when Result::Err(error)
		return error.message.empty?()
	end
end

def main()
	result := run("/bin/sh", ["-c", "printf out; printf err >&2; exit 7"])
	puts(describe(result))
	puts(succeeded(result))
	empty_arguments: Array<String> := []
	puts(operation(run("/type-rb-command-that-does-not-exist", empty_arguments)))
	puts(directory_available(working_directory()))
	puts(argv().size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "7:out:err\nfalse\nrun\ntrue\n0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s process output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedJSONAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby JSON run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript JSON run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := "import { JsonError, JsonValue, parse, stringify } from trb/std/json\n" +
			"import trb/std/jsonc\n" +
			"import { Result } from trb/std/result\n\n" +
			"def render(value: Result<JsonValue, JsonError>): String; case value; when Result::Ok(item); case stringify(item); when Result::Ok(source); return source; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; end\n" +
			"def error_path(value: Result<JsonValue, JsonError>): String; case value; when Result::Ok(item); return render(Result<JsonValue, JsonError>::Ok(item)); when Result::Err(error); return error.path; end; end\n\n" +
			"def valid(value: Result<JsonValue, JsonError>): Boolean; case value; when Result::Ok(item); return render(Result<JsonValue, JsonError>::Ok(item)).empty?(); when Result::Err(error); return error.message.empty?() or !error.message.empty?(); end; end\n\n" +
			"def main()\n" +
			"\tputs(render(jsonc.parse(\"{\\n  // comment\\n  \\\"items\\\": [1, 1.5, true, null]\\n}\")))\n" +
			"\tputs(error_path(parse(\"{\\\"items\\\":[9007199254740992]}\")))\n" +
			"\tputs(valid(jsonc.parse(\"{\\\"value\\\":1,}\")))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "{\"items\":[1,1.5,true,null]}\n/items/0\ntrue\n"; stdout.String() != want {
			t.Fatalf("unexpected %s JSON program output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunTypedJSONRecordCodecsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby typed JSON codec run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript typed JSON codec run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-json-codec-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		contracts := filepath.Join(root, "src", "contracts", "user.trb")
		if err := os.MkdirAll(filepath.Dir(contracts), 0o755); err != nil {
			t.Fatal(err)
		}
		contractSource := "record Address\n\tcity: String\nend\n\n" +
			"record User\n\tid: Integer @json(\"user_id\")\n\tname: String\n\tnickname: String?\n\tscores: Array<Float>\n\tmetadata: Hash<String, Integer>\n\taddress: Address\nend\n"
		if err := os.WriteFile(contracts, []byte(contractSource), 0o644); err != nil {
			t.Fatal(err)
		}
		mainSource := "import { Address, User } from contracts/user\n" +
			"import { JsonError, decode, encode } from trb/std/json\n" +
			"import { Result } from trb/std/result\n\n" +
			"def round_trip(source: String): String; case decode<User>(source); when Result::Ok(user); case encode(user); when Result::Ok(encoded); case decode<User>(encoded); when Result::Ok(copy); return copy.name + \":\" + copy.address.city; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; when Result::Err(error); return error.path; end; end\n" +
			"def main()\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"name\\\":\\\"Ada\\\",\\\"scores\\\":[1,1.5],\\\"metadata\\\":{\\\"active\\\":1},\\\"address\\\":{\\\"city\\\":\\\"Tokyo\\\"}}\"))\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"scores\\\":[],\\\"metadata\\\":{},\\\"address\\\":{\\\"city\\\":\\\"Tokyo\\\"}}\"))\n" +
			"\tputs(round_trip(\"{\\\"user_id\\\":7,\\\"name\\\":\\\"Ada\\\",\\\"scores\\\":[],\\\"metadata\\\":{},\\\"address\\\":{\\\"city\\\":1}}\"))\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if mode == "go" {
			t.Setenv("GOCACHE", filepath.Join(t.TempDir(), "go-build"))
			t.Setenv("GOMODCACHE", filepath.Join(t.TempDir(), "go-mod"))
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "Ada:Tokyo\n/name\n/address/city\n"; stdout.String() != want {
			t.Fatalf("unexpected %s typed JSON codec output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunWithoutSourceRequiresMain(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/acme/no-main"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "library.trb"), []byte("def value(): Integer\n  return 1\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"run", "--config", config.Path}); status != 1 {
		t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "project has no top-level main()") {
		t.Fatalf("unexpected diagnostic: %s", stderr.String())
	}
}

func TestBuildCanEmbedInExistingRailsProjectWithoutManagingGemfile(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "type_rb"
	config.OutDir = "type_rb_build"
	config.PackageManagement = project.ExternalPackages
	copyFiles := false
	config.CopyFiles = &copyFiles
	config.Ruby.Loader = "zeitwerk"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	gemfile := filepath.Join(root, "Gemfile")
	const originalGemfile = "source 'https://example.invalid'\n"
	if err := os.WriteFile(gemfile, []byte(originalGemfile), 0o644); err != nil {
		t.Fatal(err)
	}
	controller := filepath.Join(root, "type_rb", "app", "controllers", "api", "v1", "internal", "insurers_controller.trb")
	if err := os.MkdirAll(filepath.Dir(controller), 0o755); err != nil {
		t.Fatal(err)
	}
	source := "import trb/platform/ruby/rails\n\nmodule Api\n  module V1\n    module Internal\n      class InsurersController < Api::ApplicationController\n        include PaginationHelper\n\n        def index()\n          page := paginate_with_headers(Insurer.all())\n          insurers := page[0]\n          render(json: insurers)\n          return\n        end\n\n        def show()\n          insurer := Insurer.find_by!(code: params[:code])\n          render(json: insurer.as_json())\n          return\n        end\n      end\n    end\n  end\nend\n"
	if err := os.WriteFile(controller, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	actualGemfile, err := os.ReadFile(gemfile)
	if err != nil || string(actualGemfile) != originalGemfile {
		t.Fatalf("host Gemfile was modified: err=%v\n%s", err, actualGemfile)
	}
	generated := filepath.Join(root, "type_rb_build", "app", "controllers", "api", "v1", "internal", "insurers_controller.rb")
	output, err := os.ReadFile(generated)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(output), "Insurer.find_by!(code: params[:code])") {
		t.Fatalf("unexpected generated controller:\n%s", output)
	}
	if !strings.Contains(string(output), "page = paginate_with_headers(Insurer.all())") {
		t.Fatalf("generated controller omitted index pagination:\n%s", output)
	}
	if strings.Contains(stdout.String(), "packages ->") {
		t.Fatalf("external build reported a managed manifest:\n%s", stdout.String())
	}
}

func TestBuildEmitsCompilerOwnedResultRuntimeWhenSourceDirIsProjectRoot(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "."
	config.OutDir = "build"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := `import { Result } from trb/std/result

def successful(): Result<Integer, String>
	return Result<Integer, String>::Ok(1)
end
`
	if err := os.WriteFile(filepath.Join(root, "main.trb"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	runtime, err := os.ReadFile(filepath.Join(root, "build", "trb", "std", "result", "index.rb"))
	if err != nil || !strings.Contains(string(runtime), "module Result") {
		t.Fatalf("compiler-owned Result runtime was not emitted: err=%v\n%s", err, runtime)
	}
	consumer, err := os.ReadFile(filepath.Join(root, "build", "main.rb"))
	if err != nil || !strings.Contains(string(consumer), `require_relative "./trb/std/result/index"`) {
		t.Fatalf("Result consumer did not require its runtime: err=%v\n%s", err, consumer)
	}
}

func TestBuildCompilesLocalRecordPackageIntoGoTargetTree(t *testing.T) {
	workspace := t.TempDir()
	appRoot := filepath.Join(workspace, "api")
	contractRoot := filepath.Join(workspace, "contracts")
	if err := os.MkdirAll(filepath.Join(appRoot, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(contractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	config := project.New(appRoot, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/local-package"
	config.LocalPackages["acme/contracts"] = "../contracts"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(contractRoot, "index.trb"), []byte("record Message\n  text: String\nend\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainPath := filepath.Join(appRoot, "src", "main.trb")
	main := "import { Message } from acme/contracts\n\ndef main()\n  message := Message.new(text: \"shared\")\n  puts(message.text)\n  return\nend\n"
	if err := os.WriteFile(mainPath, []byte(main), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"build", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	contractOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "acme", "contracts", "index.go"))
	if err != nil || !strings.Contains(string(contractOutput), "type Message struct") {
		t.Fatalf("local contract was not generated: err=%v\n%s", err, contractOutput)
	}
	mainOutput, err := os.ReadFile(filepath.Join(appRoot, "build", "main.go"))
	if err != nil || !strings.Contains(string(mainOutput), `contracts.Message{Text: "shared"}`) {
		t.Fatalf("application did not consume local record: err=%v\n%s", err, mainOutput)
	}
}
