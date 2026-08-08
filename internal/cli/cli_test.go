package cli

import (
	"bytes"
	"encoding/json"
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

func TestReplRejectsValueReturningFunctionThatFallsThrough(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "go")
	config.SourceDir = "src"
	config.Go.Module = "example.com/type-rb/repl-return-test"
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	input := `def hello2(): String
	puts("hello!")
end
def hello2(): String
	return "hello!"
end
hello2()
:quit
`
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	if want := "\"hello!\" : String\n"; stdout.String() != want {
		t.Fatalf("unexpected REPL output\nwant:\n%s\ngot:\n%s", want, stdout.String())
	}
	if !strings.Contains(stderr.String(), "hello2() must return String on every path") {
		t.Fatalf("REPL did not reject the incomplete function:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "nil : String") {
		t.Fatalf("REPL evaluated an incomplete function as a String:\n%s", stdout.String())
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

		input := `import trb/std/math
123.to_s()
0.25.to_s()
(-0.0).to_s()
2.to_f()
(-2.75).to_i()
(-4).abs()
0.zero?()
1.positive?()
(-1).negative?()
2.even?()
3.odd?()
(-0.25).abs()
0.25.finite?()
true.to_s()
false.to_s()
0.25 * 100
1 == 1.0
"123".to_i()
"123".try_to_i()
"12x".try_to_i()
"9007199254740992".try_to_i()
5.min(3)
5.max(7)
12.clamp(0, 10)
(-2.75).floor()
(-2.75).ceil()
2.5.round()
(-2.5).round()
2.75.truncate()
math.sqrt(9)
math.exp(0)
math.log(1)
math.log2(8)
math.log10(100)
math.sqrt(-1).nan?()
math.log(0).infinite?()
"a😀".size()
:quit
`
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := `"123" : String
"0.25" : String
"0.0" : String
2 : Float
-2 : Integer
4 : Integer
true : Boolean
true : Boolean
true : Boolean
true : Boolean
true : Boolean
0.25 : Float
true : Boolean
"true" : String
"false" : String
25 : Float
true : Boolean
123 : Integer
Result::Ok(value: 123) : Result<Integer, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::InvalidFormat, input: "12x", message: "invalid Integer")) : Result<Integer, NumberParseError>
Result::Err(error: NumberParseError(kind: NumberParseErrorKind::OutOfRange, input: "9007199254740992", message: "Integer is outside the portable range")) : Result<Integer, NumberParseError>
3 : Integer
7 : Integer
10 : Integer
-3 : Integer
-2 : Integer
3 : Integer
-3 : Integer
2 : Integer
3 : Float
1 : Float
0 : Float
3 : Float
2 : Float
true : Boolean
true : Boolean
2 : Integer
`
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

func TestReplEvaluatesPortableHexAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hex-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/encoding/hex\nhex.encode(\"A😀\".to_bytes())\nhex.decode(\"41F09F9880\")\nhex.decode(\"0g\")\nhex.decode(\"abc\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"41f09f9880\" : String\n" +
			"Result::Ok(value: Bytes[65, 240, 159, 152, 128]) : Result<Bytes, HexDecodeError>\n" +
			"Result::Err(error: HexDecodeError(kind: HexDecodeErrorKind::InvalidCharacter, input: \"0g\", index: 1, message: \"invalid hexadecimal character\")) : Result<Bytes, HexDecodeError>\n" +
			"Result::Err(error: HexDecodeError(kind: HexDecodeErrorKind::OddLength, input: \"abc\", index: 3, message: \"hex input has odd length\")) : Result<Bytes, HexDecodeError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hex REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableBase64AcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-base64-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/encoding/base64\nbase64.encode(\"A😀\".to_bytes())\nbase64.url_encode(\"???\".to_bytes())\nbase64.decode(\"QfCfmIA=\")\nbase64.url_decode(\"Pz8_\")\nbase64.decode(\"AAA\")\nbase64.decode(\"AA=A\")\nbase64.decode(\"AA$=\")\nbase64.decode(\"AB==\")\nbase64.url_decode(\"A\")\nbase64.url_decode(\"AA==\")\nbase64.url_decode(\"AA$\")\nbase64.url_decode(\"AB\")\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"QfCfmIA=\" : String\n" +
			"\"Pz8_\" : String\n" +
			"Result::Ok(value: Bytes[65, 240, 159, 152, 128]) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Ok(value: Bytes[63, 63, 63]) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidLength, input: \"AAA\", index: 3, message: \"base64 input length must be a multiple of 4\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidPadding, input: \"AA=A\", index: 3, message: \"invalid base64 padding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidCharacter, input: \"AA$=\", index: 2, message: \"invalid base64 character\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::NonCanonical, input: \"AB==\", index: 1, message: \"non-canonical base64 encoding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidLength, input: \"A\", index: 1, message: \"base64url input has invalid length\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidPadding, input: \"AA==\", index: 2, message: \"base64url input must not contain padding\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::InvalidCharacter, input: \"AA$\", index: 2, message: \"invalid base64url character\")) : Result<Bytes, Base64DecodeError>\n" +
			"Result::Err(error: Base64DecodeError(kind: Base64DecodeErrorKind::NonCanonical, input: \"AB\", index: 1, message: \"non-canonical base64url encoding\")) : Result<Bytes, Base64DecodeError>\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s base64 REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableHashAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hash-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/hash\nimport trb/std/encoding/hex\nhex.encode(hash.sha256(\"\".to_bytes()))\nhex.encode(hash.sha256(\"abc\".to_bytes()))\nhex.encode(hash.sha512(\"\".to_bytes()))\nhex.encode(hash.sha512(\"abc\".to_bytes()))\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "\"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\" : String\n" +
			"\"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\" : String\n" +
			"\"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e\" : String\n" +
			"\"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\" : String\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hash REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableHMACAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-hmac-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/hmac\nimport trb/std/encoding/hex\nkey := \"Jefe\".to_bytes()\nmessage := \"what do ya want for nothing?\".to_bytes()\ntag := hmac.sha256(key, message)\nhex.encode(tag)\nhex.encode(hmac.sha512(key, message))\nhmac.equal(tag, tag)\nhmac.equal(tag, hmac.sha256(key, \"other\".to_bytes()))\nhmac.equal(tag, \"short\".to_bytes())\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Bytes[74, 101, 102, 101] : Bytes\n" +
			"Bytes[119, 104, 97, 116, 32, 100, 111, 32, 121, 97, 32, 119, 97, 110, 116, 32, 102, 111, 114, 32, 110, 111, 116, 104, 105, 110, 103, 63] : Bytes\n" +
			"Bytes[91, 220, 193, 70, 191, 96, 117, 78, 106, 4, 36, 38, 8, 149, 117, 199, 90, 0, 63, 8, 157, 39, 57, 131, 157, 236, 88, 185, 100, 236, 56, 67] : Bytes\n" +
			"\"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843\" : String\n" +
			"\"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737\" : String\n" +
			"true : Boolean\nfalse : Boolean\nfalse : Boolean\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s hmac REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplEvaluatesPortableRandomAcrossModes(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/repl-random-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		input := "import trb/std/random\nimport trb/std/secure_random\nrandom.float() >= 0.0\nrandom.float() < 1.0\nrandom.integer(10) >= 0\nrandom.integer(10) < 10\nsecure_random.bytes(0).size()\nsecure_random.bytes(32).size()\n:quit\n"
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true : Boolean\ntrue : Boolean\ntrue : Boolean\ntrue : Boolean\n0 : Integer\n32 : Integer\n"
		if stdout.String() != want || stderr.Len() != 0 {
			t.Fatalf("unexpected %s random REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, want, stdout.String(), stderr.String())
		}
	}
}

func TestReplRejectsInvalidPortableRandomBounds(t *testing.T) {
	input := "import trb/std/random\nimport trb/std/secure_random\nrandom.integer(0)\nsecure_random.bytes(65537)\n:quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "go"}); status != 0 {
		t.Fatalf("status=%d stderr=%s", status, stderr.String())
	}
	for _, want := range []string{
		"random.integer upper bound must be greater than zero",
		"secure_random.bytes length must be between 0 and 65536",
	} {
		if !strings.Contains(stderr.String(), want) {
			t.Fatalf("random REPL error is missing %q:\n%s", want, stderr.String())
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
		want := "[1, 2] : Array<Integer>\n1 : Integer\n2 : Integer\n2 : Integer\nResult::Ok(value: 2) : Result<Integer, IndexLookupError>\nResult::Err(error: IndexLookupError(index: 9, size: 2, message: \"Array index is out of bounds\")) : Result<Integer, IndexLookupError>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[1, 2, 3] : Array<Integer>\n1 : Integer\n[3, 2, 0] : Array<Integer>\n[0, 2, 3] : Array<Integer>\n0 : Integer\n[3, 2, 1] : Array<Integer>\n[1, 2, 3] : Array<Integer>\ntrue : Boolean\n1 : Integer\nfalse : Boolean\n1 : Integer\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"two\" : String\nResult::Ok(value: \"two\") : Result<String, KeyLookupError>\nResult::Err(error: KeyLookupError(key: 9, message: \"Hash key is missing\")) : Result<String, KeyLookupError>\nfalse : Boolean\n[1, 2] : Array<Integer>\n[\"one\", \"two\"] : Array<String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"TWO\", 3: \"three\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n{1: \"one\", 2: \"two\"} : Hash<Integer, String>\n\"one\" : String\n\"TWO\" : String\n{3: \"three\", 4: \"four\"} : Hash<Integer, String>\n[\"a\", \"b\", \"\"] : Array<String>\ntrue : Boolean\ntrue : Boolean\n[\"root\", \"leaf\"] : Array<String>\n\"leaf\" : String\n\"root\" : String\n"
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

func TestDivergingControlFlowExpressionsAcrossAvailableBackendsAndREPL(t *testing.T) {
	definitions := `enum Outcome
	Found(value: String)
	Missing
end

def describe(outcome: Outcome): String
	value := case outcome
	when Outcome::Found(found)
		found
	when Outcome::Missing
		return "missing"
	end
	return "found: " + value
end

def stop_before_two(): String
	mut result := ""
	[1, 2, 3].each do |number|
		value := if number == 2
			break
		else
			number.to_s()
		end
		result += value
	end
	return result
end

def skip_two(): String
	mut result := ""
	[1, 2, 3].each do |number|
		value := if number == 2
			next
		else
			number.to_s()
		end
		result += value
	end
	return result
end

def nested_choice(primary: Boolean, secondary: Boolean): String
	value := if primary
		if secondary
			return "first"
		else
			return "second"
		end
	else
		"fallback"
	end
	return value
end

def stop_on_string(values: Array<Integer | String>): String
	mut result := ""
	values.each do |value|
		text := case value
		when Integer(number)
			number.to_s()
		when String(_text)
			break
		end
		result += text
	end
	return result
end
`

	for _, mode := range []string{"go", "ruby", "typescript"} {
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/diverging-control-flow-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}

		replInput := definitions + `describe(Outcome::Found("Ada"))
describe(Outcome::Missing)
stop_before_two()
skip_two()
nested_choice(true, false)
nested_choice(false, false)
stop_on_string([1, "stop", 2])
:quit
`
		var replStdout, replStderr bytes.Buffer
		replCommand := &CLI{Stdin: strings.NewReader(replInput), Stdout: &replStdout, Stderr: &replStderr}
		if status := replCommand.Run([]string{"repl", "--config", config.Path}); status != 0 {
			t.Fatalf("%s REPL status=%d stderr=%s", mode, status, replStderr.String())
		}
		replWant := "\"found: Ada\" : String\n\"missing\" : String\n\"1\" : String\n\"13\" : String\n\"second\" : String\n\"fallback\" : String\n\"1\" : String\n"
		if replStdout.String() != replWant || replStderr.Len() != 0 {
			t.Fatalf("unexpected %s diverging REPL result\nwant:\n%s\ngot:\n%s\nstderr:\n%s", mode, replWant, replStdout.String(), replStderr.String())
		}

		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby diverging run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript diverging run")
				continue
			}
		}
		source := definitions + `
def main()
	puts(describe(Outcome::Found("Ada")))
	puts(describe(Outcome::Missing))
	puts(stop_before_two())
	puts(skip_two())
	puts(nested_choice(true, false))
	puts(nested_choice(false, false))
	puts(stop_on_string([1, "stop", 2]))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "found: Ada\nmissing\n1\n13\nsecond\nfallback\n1\n"; stdout.String() != want {
			t.Fatalf("unexpected %s diverging output: want %q, got %q", mode, want, stdout.String())
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

func TestRunInterfaceValuesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby interface-value run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript interface-value run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/interface-value-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `interface Named
	name(): String
end

class Person implements Named
	@value: String

	def initialize(name: String)
		@value = name
		return
	end

	def name(): String
		return @value
	end
end

def display(value: Named): String
	return value.name()
end

def main()
	values: Array<Named> := [Person.new("Ada"), Person.new("Grace")]
	values.each do |value|
		puts(display(value))
	end
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if stdout.String() != "Ada\nGrace\n" {
			t.Fatalf("unexpected %s interface-value output %q", mode, stdout.String())
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

func TestRunPortableMathAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby math run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript math run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-math-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/math

def main()
	puts(5.min(3))
	puts(5.max(7))
	puts(12.clamp(0, 10))
	puts((-2.75).floor())
	puts((-2.75).ceil())
	puts(2.5.round())
	puts((-2.5).round())
	puts(2.75.truncate())
	puts(math.sqrt(9) == 3.0)
	puts(math.exp(0) == 1.0)
	puts(math.log(1) == 0.0)
	puts(math.log2(8) == 3.0)
	puts(math.log10(100) == 2.0)
	puts(math.sqrt(-1).nan?())
	puts(math.log(0).infinite?())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "3\n7\n10\n-3\n-2\n3\n-3\n2\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable math output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHexAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hex run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hex run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hex-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { decode, encode } from trb/std/encoding/hex

def decoded_text(input: String): String
	case decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def main()
	puts(encode("A😀".to_bytes()))
	puts(decoded_text("41F09F9880"))
	puts(decoded_text("0g"))
	puts(decoded_text("abc"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "41f09f9880\nA😀\ninvalid hexadecimal character:1\nhex input has odd length:3\n"; stdout.String() != want {
			t.Fatalf("unexpected %s portable hex output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunCompilerOwnedNamespaceImportsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping namespace import run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping namespace import run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/namespace-import-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/encoding/hex
import trb/std/process
import { Result } from trb/std/result

def decoded_text(input: String): String
	case hex.decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message
	end
end

def main()
	puts(hex.encode("A".to_bytes()))
	puts(decoded_text("41"))
	puts(process.argv().size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "41\nA\n0\n"; stdout.String() != want {
			t.Fatalf("unexpected %s namespace import output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableBase64AcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby base64 run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript base64 run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-base64-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { decode, encode, url_decode, url_encode } from trb/std/encoding/base64

def decoded_text(input: String): String
	case decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def url_decoded_text(input: String): String
	case url_decode(input)
	when Result::Ok(value)
		return value.to_s()
	when Result::Err(error)
		return error.message + ":" + error.index.to_s()
	end
end

def main()
	puts(encode("A😀".to_bytes()))
	puts(url_encode("???".to_bytes()))
	puts(decoded_text("QfCfmIA="))
	puts(url_decoded_text("Pz8_"))
	puts(decoded_text("AAA"))
	puts(decoded_text("AA=A"))
	puts(decoded_text("AA$="))
	puts(decoded_text("AB=="))
	puts(url_decoded_text("A"))
	puts(url_decoded_text("AA=="))
	puts(url_decoded_text("AA$"))
	puts(url_decoded_text("AB"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "QfCfmIA=\nPz8_\nA😀\n???\n" +
			"base64 input length must be a multiple of 4:3\n" +
			"invalid base64 padding:3\n" +
			"invalid base64 character:2\n" +
			"non-canonical base64 encoding:1\n" +
			"base64url input has invalid length:1\n" +
			"base64url input must not contain padding:2\n" +
			"invalid base64url character:2\n" +
			"non-canonical base64url encoding:1\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable base64 output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHashAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hash run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hash run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hash-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { sha256, sha512 } from trb/std/hash
import { decode, encode } from trb/std/encoding/hex

def main()
	a8 := "aaaaaaaa"
	a56 := a8 + a8 + a8 + a8 + a8 + a8 + a8
	a112 := a56 + a56
	_decoded := decode("00")
	puts(encode(sha256("".to_bytes())))
	puts(encode(sha256("abc".to_bytes())))
	puts(encode(sha256(a56.to_bytes())))
	puts(encode(sha512("".to_bytes())))
	puts(encode(sha512("abc".to_bytes())))
	puts(encode(sha512(a112.to_bytes())))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855\n" +
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n" +
			"b35439a4ac6f0948b6d6f9e3c6af0f5f590ce20f1bde7090ef7970686ec6738a\n" +
			"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e\n" +
			"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f\n" +
			"c01d080efd492776a1c43bd23dd99d0a2e626d481e16782e75d54c2503b5dc32bd05f0f1ba33e568b88fd2d970929b719ecbb152f58f130a407c8830604b70ca\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable hash output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableHMACAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby hmac run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript hmac run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-hmac-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { equal, sha256, sha512 } from trb/std/hmac
import { decode, encode } from trb/std/encoding/hex

def main()
	a8 := "aaaaaaaa"
	a64 := a8 + a8 + a8 + a8 + a8 + a8 + a8 + a8
	key80 := a64 + a8 + a8
	key136 := a64 + a64 + a8
	key := "Jefe".to_bytes()
	message := "what do ya want for nothing?".to_bytes()
	tag := sha256(key, message)
	_decoded := decode("00")
	puts(encode(tag))
	puts(encode(sha512(key, message)))
	puts(encode(sha256(key80.to_bytes(), "message".to_bytes())))
	puts(encode(sha512(key136.to_bytes(), "message".to_bytes())))
	puts(equal(tag, tag))
	puts(equal(tag, sha256(key, "other".to_bytes())))
	puts(equal(tag, "short".to_bytes()))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843\n" +
			"164b7a7bfcf819e2e395fbe73b56e0a387bd64222e831fd610270cd7ea2505549758bf75c05a994a6d034f65f8f0e6fdcaeab1a34d4a6b4b636e070a38bce737\n" +
			"d0c62b445e5d504c9809dcaa12bfedd969deb591591984b81c68b352cec257ee\n" +
			"435bf6bbcffb2d5301b470b17314c3571666de1cd1f96776dfd9e59ce07f32338bfca69d7be3f6d33c3eee5def6ebec48e8181d86ea9ebeeb639fa3ce6da44d7\n" +
			"true\nfalse\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable hmac output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableRandomAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby random run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript random run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-random-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import trb/std/random
import trb/std/secure_random

def main()
	fraction := random.float()
	index := random.integer(10)
	puts(fraction >= 0.0 && fraction < 1.0)
	puts(index >= 0 && index < 10)
	puts(secure_random.bytes(0).size())
	puts(secure_random.bytes(32).size())
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "true\ntrue\n0\n32\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable random output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableURLComponentsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby URL component run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript URL component run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-url-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { PercentDecodeError, PercentDecodeErrorKind, decode_component, encode_component } from trb/std/url

def decode(value: String): Result<String, PercentDecodeError>
	return decode_component(value)
end

def decoded(value: String): String
	case decode(value)
	when Result::Ok(text)
		return text
	when Result::Err(error)
		case error.kind
		when PercentDecodeErrorKind::InvalidEscape
			return "invalid escape"
		when PercentDecodeErrorKind::InvalidUtf8
			return "invalid utf8"
		end
	end
end

def main()
	puts(encode_component("a b/😀+~"))
	puts(decoded("a%20b%2F%F0%9F%98%80%2B~"))
	puts(decoded("a+b"))
	puts(decoded("%"))
	puts(decoded("%FF"))
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "a%20b%2F%F0%9F%98%80%2B~\na b/😀+~\na+b\ninvalid escape\ninvalid utf8\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable URL component output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunPortableURLQueryAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby URL query run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript URL query run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/portable-url-query-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		source := `import { Result } from trb/std/result
import { QueryParameter, build_query, parse_query } from trb/std/url

def print_query(source: String)
	case parse_query(source)
	when Result::Ok(parameters)
		parameters.each do |parameter|
			puts(parameter.name + ":" + parameter.value)
		end
	when Result::Err(error)
		puts(error.input + ":" + error.message)
	end
	return
end

def main()
	query := build_query([
		QueryParameter.new(name: "tag", value: "type rb"),
		QueryParameter.new(name: "tag", value: "go"),
		QueryParameter.new(name: "symbol", value: "+&="),
		QueryParameter.new(name: "tilde", value: "~"),
		QueryParameter.new(name: "star", value: "*"),
	])
	puts(query)
	print_query("tag=go&&tag=type+rb&empty&symbol=%2B&text=%E6%97%A5%E6%9C%AC%E8%AA%9E&")
	print_query("name=%")
	print_query("%FF=value")
	return
end
`
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "tag=type+rb&tag=go&symbol=%2B%26%3D&tilde=%7E&star=*\n" +
			"tag:go\ntag:type rb\nempty:\nsymbol:+\ntext:日本語\n" +
			"%:invalid percent escape in URL query component\n" +
			"%FF:decoded URL query component is not valid UTF-8\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s portable URL query output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestREPLPortableURLQueryUsesCompilerOwnedSource(t *testing.T) {
	input := "import { QueryParameter, build_query, parse_query } from trb/std/url\n" +
		"build_query([QueryParameter.new(name: \"tag\", value: \"type rb\"), QueryParameter.new(name: \"tag\", value: \"go\")])\n" +
		"parse_query(\"tag=type+rb&tag=go\")\n" +
		":quit\n"
	var stdout, stderr bytes.Buffer
	command := &CLI{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr}
	if status := command.Run([]string{"repl", "--mode", "go"}); status != 0 {
		t.Fatalf("REPL status=%d stderr=%s", status, stderr.String())
	}
	want := "\"tag=type+rb&tag=go\" : String\n" +
		"Result::Ok(value: [QueryParameter(name: \"tag\", value: \"type rb\"), QueryParameter(name: \"tag\", value: \"go\")]) : Result<Array<QueryParameter>, PercentDecodeError>\n"
	if stdout.String() != want {
		t.Fatalf("unexpected URL query REPL output: want %q, got %q; stderr=%s", want, stdout.String(), stderr.String())
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
			"import { IndexLookupError, KeyLookupError, NumberParseError } from trb/std/errors\n" +
			"import trb/std/string_builder\n" +
			"import trb/std/numbers\n" +
			"import trb/std/booleans\n\n" +
			"def parse_result(value: Result<Integer, NumberParseError>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def index_result(value: Result<Integer, IndexLookupError>): String; case value; when Result::Ok(number); return \"ok:\" + number.to_s(); when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def key_result(value: Result<String, KeyLookupError>): String; case value; when Result::Ok(text); return \"ok:\" + text; when Result::Err(error); return \"err:\" + error.message; end; end\n" +
			"def scalar_check(value: Float): Boolean; return (-4).abs() == numbers.absolute(-4) && 0.zero?() && 1.positive?() && (-1).negative?() && 2.even?() && 3.odd?() && (-0.25).abs() == 0.25 && (value.finite?() || value.infinite?() || value.nan?()) && true.to_s() == booleans.to_string(true); end\n\n" +
			"def main()\n" +
			"\tputs(scalar_check(0.25))\n" +
			"\tputs(parse_result(\"12\".try_to_i()))\n" +
			"\tputs(parse_result(\"12x\".try_to_i()))\n" +
			"\tputs(parse_result(\"9007199254740992\".try_to_i()))\n" +
			"\tvalues := [7]\n" +
			"\tputs(index_result(values.try_fetch(0)))\n" +
			"\tputs(index_result(values.try_fetch(1)))\n" +
			"\tlabels: Hash<String, String> := {\"name\" => \"Ada\"}\n" +
			"\tputs(key_result(labels.try_fetch(\"name\")))\n" +
			"\tputs(key_result(labels.try_fetch(\"missing\")))\n" +
			"\tbuilder := string_builder.new()\n" +
			"\tputs(builder.empty?())\n" +
			"\treturn\n" +
			"end\n"
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(source), 0o644); err != nil {
			t.Fatal(err)
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

func TestRunPortableDefaultArgumentsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby default argument run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript default argument run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-default-argument-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `class Greeter
	@prefix: String

	def initialize(prefix: String = "Hello")
		@prefix = prefix
		return
	end

	def greet(name: String, suffix: String = "!"): String
		return @prefix + ", " + name + suffix
	end
end

def count_label(count: Integer = 2): String
	return count.to_s()
end

def fallback(value: String, replacement: String = value): String
	return replacement
end

def missing?(value: String? = nil): Boolean
	return value == nil
end

def main()
	puts(Greeter.new().greet("Ada"))
	puts(Greeter.new("Hi").greet("Lin", "."))
	puts(count_label())
	puts(count_label(3))
	puts(fallback("same"))
	puts(missing?())
	puts(missing?("value"))
	puts(missing?(nil))
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "Hello, Ada!\nHi, Lin.\n2\n3\nsame\ntrue\nfalse\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s default argument output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRequestHeadersAndCookiesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web cookie run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web cookie run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-cookie-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import {
	CookieValueError,
	HeaderValueError,
	Request,
	cookie,
	cookie_value,
	cookie_values,
	cookies,
	header_value,
	header_values,
} from trb/web
import { Result } from trb/std/result

def render_header_value(result: Result<String, HeaderValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when HeaderValueError::Missing(name)
			return "missing:" + name
		when HeaderValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def render_cookie_value(result: Result<String, CookieValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when CookieValueError::Missing(name)
			return "missing:" + name
		when CookieValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def main()
	request := Request.new(
		method: "GET",
		path: "/",
		query_string: "",
		headers: {
			"Cookie" => [
				"session=abc; theme=dark",
				"tag=first; broken; =empty; tag=second; token=a=b",
			],
			"X-Request-ID" => ["req-1"],
		},
		body: "".to_bytes(),
	)
	puts(header_values(request, "COOKIE").size())
	puts(render_header_value(header_value(request, "x-request-id")))
	puts(render_header_value(header_value(request, "cookie")))
	puts(render_header_value(header_value(request, "missing")))
	parsed := cookies(request)
	puts(parsed.size())
	parsed.each do |value|
		puts(value.name + "=" + value.value)
	end
	puts(cookie_values(request, "tag").size())
	puts(render_cookie_value(cookie_value(request, "session")))
	puts(render_cookie_value(cookie_value(request, "tag")))
	puts(render_cookie_value(cookie_value(request, "missing")))
	puts(cookie(request, "tag") == nil)
	puts(cookie(request, "missing") == nil)
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "2\nok:req-1\nduplicate:cookie\nmissing:missing\n5\nsession=abc\ntheme=dark\ntag=first\ntag=second\ntoken=a=b\n2\nok:abc\nduplicate:tag\nmissing:missing\nfalse\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web cookie output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebQueryHelpersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web query helper run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web query helper run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-query-helper-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { QueryValueError, Request, query_value, query_values } from trb/web
import { PercentDecodeError } from trb/std/url
import { Result } from trb/std/result

def request(query_string: String): Request
	return Request.new(method: "GET", path: "/", query_string: query_string, headers: {}, body: "".to_bytes())
end

def render_value(result: Result<String, QueryValueError>): String
	case result
	when Result::Ok(value)
		return "ok:" + value
	when Result::Err(error)
		case error
		when QueryValueError::Malformed(decode_error)
			return "malformed:" + decode_error.input
		when QueryValueError::Missing(name)
			return "missing:" + name
		when QueryValueError::Duplicate(name)
			return "duplicate:" + name
		end
	end
end

def print_values(result: Result<Array<String>, PercentDecodeError>)
	case result
	when Result::Ok(values)
		puts(values.size())
		values.each do |value|
			puts(value)
		end
	when Result::Err(error)
		puts(error.message)
	end
	return
end

def main()
	parsed := request("tag=go&tag=web&page=2&empty=")
	print_values(query_values(parsed, "tag"))
	print_values(query_values(parsed, "missing"))
	puts(render_value(query_value(parsed, "page")))
	puts(render_value(query_value(parsed, "missing")))
	puts(render_value(query_value(parsed, "tag")))
	puts(render_value(query_value(request("value=%ZZ"), "value")))
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "2\ngo\nweb\n0\nok:2\nmissing:missing\nduplicate:tag\nmalformed:%ZZ\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web query helper output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseHeadersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response header run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response header run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-header-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import {
	Response,
	add_header,
	redirect,
	response_header_values,
	vary,
	with_header,
	with_status,
	without_header,
} from trb/web

def main()
	base := Response.new(
		status: 204,
		headers: {"X-Trace" => ["one"], "x-keep" => ["yes"], "Vary" => ["Accept, Accept-Encoding"]},
		body: "body".to_bytes(),
	)
	replaced := with_header(base, "x-TRACE", "two")
	added := add_header(replaced, "X-Trace", "three")
	removed := without_header(added, "X-Keep")
	created := with_status(removed, 201)
	varied := vary(vary(vary(created, "accept"), "Origin"), "origin")
	found := response_header_values(varied, "VARY")
	default_redirect := redirect("/login")
	temporary_redirect := redirect("/next", 307)
	puts(base.headers["X-Trace"].size())
	puts(base.headers["X-Trace"][0])
	puts(added.headers["x-trace"].size())
	puts(added.headers["x-trace"][0])
	puts(added.headers["x-trace"][1])
	puts(added.headers["x-keep"][0])
	puts(removed.headers.key?("x-keep"))
	puts(varied.status)
	puts(varied.body.to_s())
	puts(found.size())
	puts(found.join("|"))
	puts(default_redirect.status)
	puts(default_redirect.headers["location"][0])
	puts(default_redirect.body.size())
	puts(temporary_redirect.status)
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "1\none\n2\ntwo\nthree\nyes\nfalse\n201\nbody\n2\nAccept, Accept-Encoding|Origin\n302\n/login\n0\n307\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response headers: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseBuildersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response builder run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response builder run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-builder-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { bytes, empty, text } from trb/web

def main()
	plain := text("hello")
	not_found := text("missing", 404)
	binary := bytes("raw".to_bytes())
	no_content := empty()
	reset := empty(205)
	puts(plain.status)
	puts(plain.headers["content-type"][0])
	puts(plain.body.to_s())
	puts(not_found.status)
	puts(binary.status)
	puts(binary.headers["content-type"][0])
	puts(binary.body.to_s())
	puts(no_content.status)
	puts(no_content.headers.size())
	puts(no_content.body.size())
	puts(reset.status)
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "200\ntext/plain; charset=utf-8\nhello\n404\n200\napplication/octet-stream\nraw\n204\n0\n0\n205\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response builder output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseCookiesAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response cookie run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response cookie run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-cookie-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import {
	CookieSameSite,
	Response,
	ResponseCookie,
	ResponseCookieAttribute,
	new_response_cookie,
	set_cookie,
} from trb/web

def main()
	base := Response.new(status: 204, headers: {}, body: "".to_bytes())
	simple := set_cookie(base, new_response_cookie("theme", "dark"))
	session_cookie := ResponseCookie.new(
		name: "session",
		value: "abc",
		attributes: [
			ResponseCookieAttribute::Domain("example.com"),
			ResponseCookieAttribute::Path("/"),
			ResponseCookieAttribute::MaxAge(3600),
			ResponseCookieAttribute::Secure,
			ResponseCookieAttribute::HttpOnly,
			ResponseCookieAttribute::SameSite(CookieSameSite::Lax),
		],
	)
	complete := set_cookie(simple, session_cookie)
	puts(base.headers.size())
	puts(complete.headers["set-cookie"].size())
	puts(complete.headers["set-cookie"][0])
	puts(complete.headers["set-cookie"][1])
	return
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "0\n2\ntheme=dark\nsession=abc; Domain=example.com; Path=/; Max-Age=3600; Secure; HttpOnly; SameSite=Lax\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response cookie output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebJSONAPIsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web JSON run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web JSON run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-json-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	request := Request.new(
		method: "POST",
		path: "/todos/7",
		query_string: "tag=type+rb&tag=go",
		headers: { "content-type" => ["application/json"] },
		body: "{\"title\":\"ship\"}".to_bytes(),
	)
	response := dispatch(request)
	puts(response.status)
	puts(response.body.to_s())
	method_not_allowed := dispatch(Request.new(method: "GET", path: "/todos/7", query_string: "", headers: {}, body: "".to_bytes()))
	puts(method_not_allowed.status)
	puts(method_not_allowed.headers["allow"][0])
	puts(method_not_allowed.body.to_s())
	mut oversized_body := "a".to_bytes()
	(0...21).each do |_index|
		oversized_body = oversized_body.concat(oversized_body)
	end
	payload_too_large := dispatch(Request.new(method: "POST", path: "/todos/7", query_string: "", headers: {}, body: oversized_body))
	puts(payload_too_large.status)
	puts(payload_too_large.body.to_s())
	return
end
`
		routeSource := `import { Context, Response, json, path_param, query_parameters, request_json } from trb/web
import { Result } from trb/std/result

record TodoRequest
	title: String
end

record TodoResponse
	id: String
	title: String
end

def post(context: Context): Response
	id := path_param(context, "id")
	case request_json<TodoRequest>(context.request)
	when Result::Ok(payload)
		case query_parameters(context.request)
		when Result::Ok(parameters)
			return json(TodoResponse.new(id: id, title: payload.title + ":" + parameters[0].value + ":" + parameters[1].value), 201)
		when Result::Err(_error)
			return json(TodoResponse.new(id: id, title: "invalid query"), 400)
		end
	when Result::Err(_error)
		return json(TodoResponse.new(id: id, title: "invalid"), 400)
	end
end
`
		rootMiddlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("root:before")
	response := next_handler.call(context)
	puts("root:after")
	return response
end
`
		nestedMiddlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("todos:before")
	response := next_handler.call(context)
	puts("todos:after")
	return response
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		routePath := filepath.Join(root, "src", "routes", "todos", "[id].trb")
		if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routePath, []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(rootMiddlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "todos", "_middleware.trb"), []byte(nestedMiddlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "root:before\ntodos:before\ntodos:after\nroot:after\n201\n{\"id\":\"7\",\"title\":\"ship:type rb:go\"}\n405\nOPTIONS, POST\n{\"error\":\"method_not_allowed\"}\n413\n{\"error\":\"payload_too_large\"}\n"; stdout.String() != want {
			t.Fatalf("unexpected %s trb/web JSON output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRequestErrorsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web request error run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web request error run")
				continue
			}
		}

		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-request-error-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}

		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing
import { decode } from trb/std/encoding/hex
import { Result } from trb/std/result

record RequestInput
	headers: Hash<String, Array<String>>
	body: Bytes
end

def print_response(input: RequestInput)
	response := dispatch(Request.new(
		method: "POST",
		path: "/payload",
		query_string: "",
		headers: input.headers,
		body: input.body,
	))
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	valid_body := "{\"title\":\"ship\"}".to_bytes()
	print_response(RequestInput.new(headers: {}, body: valid_body))
	print_response(RequestInput.new(headers: { "Content-Type" => ["application/json"], "content-type" => ["application/json"] }, body: valid_body))
	print_response(RequestInput.new(headers: { "content-type" => ["text/plain"] }, body: valid_body))
	print_response(RequestInput.new(headers: { "content-type" => ["Application/JSON; Charset=UTF-8"] }, body: valid_body))
	print_response(RequestInput.new(headers: { "content-type" => ["application/vnd.example+json"] }, body: valid_body))
	case decode("FF")
	when Result::Ok(invalid_utf8)
		print_response(RequestInput.new(headers: { "content-type" => ["application/json"] }, body: invalid_utf8))
	when Result::Err(_error)
		return
	end
	print_response(RequestInput.new(headers: { "content-type" => ["application/json"] }, body: "{".to_bytes()))
	return
end
`
		routeSource := `import { Context, RequestError, Response, request_json, text } from trb/web
import { Result } from trb/std/result

record Payload
	title: String
end

def post(context: Context): Response
	case request_json<Payload>(context.request)
	when Result::Ok(payload)
		return text("ok:" + payload.title)
	when Result::Err(error)
		case error
		when RequestError::MissingContentType
			return text("missing_content_type", 400)
		when RequestError::DuplicateContentType
			return text("duplicate_content_type", 400)
		when RequestError::UnsupportedContentType(value)
			return text("unsupported_content_type:" + value, 400)
		when RequestError::InvalidUtf8
			return text("invalid_utf8", 400)
		when RequestError::InvalidJson(_json_error)
			return text("invalid_json", 400)
		end
	end
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "payload.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "400\nmissing_content_type\n" +
			"400\nduplicate_content_type\n" +
			"400\nunsupported_content_type:text/plain\n" +
			"200\nok:ship\n" +
			"200\nok:ship\n" +
			"400\ninvalid_utf8\n" +
			"400\ninvalid_json\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web request error output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebMethodSemanticsAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web method semantics run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web method semantics run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-head-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def request(method: String, path: String): Request
	return Request.new(
		method: method,
		path: path,
		query_string: "",
		headers: {"X-Trace" => ["value"]},
		body: "".to_bytes(),
	)
end

def main()
	fallback := dispatch(request("head", "/fallback"))
	puts(fallback.status)
	puts(fallback.body.size())
	puts(fallback.headers["x-handler"][0])
	puts(fallback.headers["x-method"][0])
	puts(fallback.headers["x-trace"][0])
	explicit := dispatch(request("HEAD", "/explicit"))
	puts(explicit.status)
	puts(explicit.body.size())
	puts(explicit.headers["x-handler"][0])
	automatic_options := dispatch(request("OPTIONS", "/fallback"))
	puts(automatic_options.status)
	puts(automatic_options.headers["allow"][0])
	puts(automatic_options.body.size())
	explicit_options := dispatch(request("OPTIONS", "/explicit"))
	puts(explicit_options.status)
	puts(explicit_options.headers["x-handler"][0])
	puts(explicit_options.body.to_s())
	unsupported := dispatch(request("POST", "/fallback"))
	puts(unsupported.status)
	puts(unsupported.headers["allow"][0])
	puts(unsupported.body.to_s())
	missing := dispatch(request("HEAD", "/missing"))
	puts(missing.status)
	puts(missing.body.size())
	return
end
`
		fallbackRouteSource := `import { Context, Response } from trb/web

def get(context: Context): Response
	puts("fallback:get")
	return Response.new(
		status: 200,
		headers: {
			"x-handler" => ["get"],
			"x-method" => [context.request.method],
			"x-trace" => [context.request.headers["x-trace"][0]],
		},
		body: "fallback".to_bytes(),
	)
end
`
		explicitRouteSource := `import { Context, Response } from trb/web

def get(_context: Context): Response
	puts("explicit:get")
	return Response.new(status: 200, headers: {"x-handler" => ["get"]}, body: "get".to_bytes())
end

def head(_context: Context): Response
	puts("explicit:head")
	return Response.new(status: 202, headers: {"x-handler" => ["head"]}, body: "head".to_bytes())
end

def options(_context: Context): Response
	puts("explicit:options")
	return Response.new(status: 203, headers: {"x-handler" => ["options"]}, body: "explicit-options".to_bytes())
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web

def call(context: Context, next_handler: Next): Response
	puts("middleware:before")
	response := next_handler.call(context)
	puts("middleware:after")
	return response
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "fallback.trb"), []byte(fallbackRouteSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "explicit.trb"), []byte(explicitRouteSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "middleware:before\nfallback:get\nmiddleware:after\n200\n0\nget\nHEAD\nvalue\nmiddleware:before\nexplicit:head\nmiddleware:after\n202\n0\nhead\nmiddleware:before\nmiddleware:after\n204\nGET, HEAD, OPTIONS\n0\nmiddleware:before\nexplicit:options\nmiddleware:after\n203\noptions\nexplicit-options\n405\nGET, HEAD, OPTIONS\n{\"error\":\"method_not_allowed\"}\n404\n0\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web method semantics output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRecoveryAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web recovery run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web recovery run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-recovery-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	request := Request.new(method: "GET", path: "/failure", query_string: "", headers: {}, body: "".to_bytes())
	response := dispatch(request)
	puts(response.status)
	puts(response.body.to_s())
	return
end
`
		routeSource := `import { Context, Response, json } from trb/web

record FailureResponse
	value: Integer
end

def get(_context: Context): Response
	value := "not-an-integer".to_i()
	return json(FailureResponse.new(value: value))
end
`
		if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		routePath := filepath.Join(root, "src", "routes", "failure.trb")
		if err := os.MkdirAll(filepath.Dir(routePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(routePath, []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		if want := "500\n{\"error\":\"internal_server_error\"}\n"; stdout.String() != want {
			t.Fatalf("unexpected %s trb/web recovery output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebResponseValidationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web response validation run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web response validation run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-response-validation-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request, Response } from trb/web
import { dispatch } from trb/web/testing

def print_response(response: Response)
	puts(response.status)
	puts(response.body.to_s())
	puts(response.headers.key?("x-injected"))
	return
end

def main()
	["invalid-name", "invalid-value", "invalid-status", "valid"].each do |path|
		print_response(dispatch(Request.new(
			method: "GET",
			path: "/" + path,
			query_string: "",
			headers: {},
			body: "".to_bytes(),
		)))
	end
	return
end
`
		routes := map[string]string{
			"invalid-name.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 200, headers: {"bad name" => ["value"]}, body: "unsafe".to_bytes())
end
`,
			"invalid-value.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 200, headers: {"x-safe" => ["safe\r\nx-injected: yes"]}, body: "unsafe".to_bytes())
end
`,
			"invalid-status.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 42, headers: {}, body: "unsafe".to_bytes())
end
`,
			"valid.trb": `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 218, headers: {"x-valid_token" => ["value"]}, body: "valid".to_bytes())
end
`,
		}
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		for filename, source := range routes {
			if err := os.WriteFile(filepath.Join(root, "src", "routes", filename), []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "500\n{\"error\":\"internal_server_error\"}\nfalse\n500\n{\"error\":\"internal_server_error\"}\nfalse\n500\n{\"error\":\"internal_server_error\"}\nfalse\n218\nvalid\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web response validation output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebJSONLLoggerAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web logger run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web logger run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-logger-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	_logged_response := dispatch(Request.new(method: "GET", path: "/logged", query_string: "", headers: {}, body: "".to_bytes()))
	_excluded_response := dispatch(Request.new(method: "GET", path: "/health", query_string: "", headers: {}, body: "".to_bytes()))
	return
end
`
		routeSource := `import { Context, Response } from trb/web

def get(_context: Context): Response
	return Response.new(status: 204, headers: {}, body: "".to_bytes())
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/logger
import { LoggerOptions } from trb/web/middleware/logger

LOGGER_OPTIONS := LoggerOptions.new(stderr: false, exclude_paths: ["/health"])

def call(context: Context, next_handler: Next): Response
	return logger.call(context, next_handler, LOGGER_OPTIONS)
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "logged.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "health.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) != 1 {
			t.Fatalf("%s logger emitted %d lines, want 1: %q", mode, len(lines), stdout.String())
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(lines[0]), &entry); err != nil {
			t.Fatalf("%s logger did not emit JSONL: %v: %q", mode, err, lines[0])
		}
		if entry["event"] != "http_request" || entry["level"] != "info" || entry["method"] != "GET" || entry["path"] != "/logged" || entry["status"] != float64(204) {
			t.Fatalf("unexpected %s logger entry: %#v", mode, entry)
		}
		if timestamp, ok := entry["timestamp"].(string); !ok || timestamp == "" {
			t.Fatalf("%s logger timestamp is missing: %#v", mode, entry)
		}
		if duration, ok := entry["duration_ms"].(float64); !ok || duration < 0 {
			t.Fatalf("%s logger duration is invalid: %#v", mode, entry)
		}
	}
}

func TestRunOfficialWebMiddlewareCompositionAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby middleware composition run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript middleware composition run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-middleware-composition-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	ordered := dispatch(Request.new(method: "GET", path: "/ordered", query_string: "", headers: {}, body: "".to_bytes()))
	puts(ordered.status)
	puts(ordered.headers["x-content-type-options"][0])
	rejected := dispatch(Request.new(method: "GET", path: "/twice", query_string: "", headers: {}, body: "".to_bytes()))
	puts(rejected.status)
	puts(rejected.body.to_s())
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import { Middleware, compose } from trb/web/middleware
import trb/web/middleware/secure_headers

class TraceMiddleware implements Middleware
	@label: String

	def initialize(label: String)
		@label = label
		return
	end

	def call(context: Context, next_handler: Next): Response
		puts(@label + ":before")
		response := next_handler.call(context)
		puts(@label + ":after")
		return response
	end
end

class DoubleCallMiddleware implements Middleware
	def call(context: Context, next_handler: Next): Response
		_first := next_handler.call(context)
		return next_handler.call(context)
	end
end

ORDERED: Array<Middleware> := [
	TraceMiddleware.new("first"),
	secure_headers.middleware(),
	TraceMiddleware.new("second"),
]
DOUBLE_CALL: Array<Middleware> := [DoubleCallMiddleware.new()]

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/twice"
		return compose(context, next_handler, DOUBLE_CALL)
	end
	return compose(context, next_handler, ORDERED)
end
`
		routeSource := `import { Context, Response, text } from trb/web

def get(context: Context): Response
	puts("route:" + context.request.path)
	return text("ok")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "ordered.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "twice.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "first:before\nsecond:before\nroute:/ordered\nsecond:after\nfirst:after\n200\nnosniff\nroute:/twice\n500\n{\"error\":\"internal_server_error\"}\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s middleware composition output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebSecureHeadersAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web secure headers run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web secure headers run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-secure-headers-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	default_response := dispatch(Request.new(method: "GET", path: "/default", query_string: "", headers: {}, body: "".to_bytes()))
	puts(default_response.headers["x-content-type-options"][0])
	puts(default_response.headers["x-frame-options"][0])
	puts(default_response.headers["referrer-policy"][0])
	puts(default_response.headers["x-xss-protection"][0])
	custom_response := dispatch(Request.new(method: "GET", path: "/custom", query_string: "", headers: {}, body: "".to_bytes()))
	puts(custom_response.headers["x-custom-security"][0])
	puts(custom_response.headers.key?("x-content-type-options"))
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/secure_headers
import { SecureHeadersOptions } from trb/web/middleware/secure_headers

CUSTOM_OPTIONS := SecureHeadersOptions.new(headers: {"x-custom-security" => "enabled"})

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/custom"
		return secure_headers.call(context, next_handler, CUSTOM_OPTIONS)
	end
	return secure_headers.call(context, next_handler)
end
`
		routeSource := `import { Context, Response, text, with_header } from trb/web

def get(_context: Context): Response
	return with_header(text("ok"), "X-Frame-Options", "DENY")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "default.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "custom.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "nosniff\nSAMEORIGIN\nno-referrer\n0\nenabled\nfalse\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web secure headers output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebRequestIDAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web request ID run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web request ID run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-request-id-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request, Response } from trb/web
import { dispatch } from trb/web/testing

def send(headers: Hash<String, Array<String>>): Response
	return dispatch(Request.new(
		method: "GET",
		path: "/id",
		query_string: "",
		headers: headers,
		body: "".to_bytes(),
	))
end

def print_generated(response: Response)
	value := response.body.to_s()
	puts(value.size())
	puts(value == response.headers["x-request-id"][0])
	return
end

def main()
	preserved := send({"x-request-id" => ["upstream-123"]})
	puts(preserved.body.to_s())
	puts(preserved.body.to_s() == preserved.headers["x-request-id"][0])

	invalid := send({"x-request-id" => ["bad id"]})
	print_generated(invalid)
	puts(invalid.body.to_s() != "bad id")

	duplicate := send({"x-request-id" => ["first", "second"]})
	print_generated(duplicate)

	missing := send({})
	print_generated(missing)
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/request_id

def call(context: Context, next_handler: Next): Response
	return request_id.call(context, next_handler)
end
`
		routeSource := `import { Context, Response, header_value, text } from trb/web
import { Result } from trb/std/result

def get(context: Context): Response
	case header_value(context.request, "x-request-id")
	when Result::Ok(value)
		return text(value)
	when Result::Err(_error)
		return text("missing", 500)
	end
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "id.trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "upstream-123\ntrue\n32\ntrue\ntrue\n32\ntrue\n32\ntrue\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web request ID output: want %q, got %q", mode, want, stdout.String())
		}
	}
}

func TestRunOfficialWebCORSAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web CORS run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web CORS run")
				continue
			}
		}
		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-cors-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}
		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def main()
	allowed := dispatch(Request.new(
		method: "GET",
		path: "/allowed",
		query_string: "",
		headers: {"origin" => ["https://app.example"]},
		body: "".to_bytes(),
	))
	puts(allowed.headers["access-control-allow-origin"][0])
	puts(allowed.headers["access-control-allow-credentials"][0])
	puts(allowed.headers["access-control-expose-headers"][0])
	puts(allowed.headers["vary"].join("|"))

	disallowed := dispatch(Request.new(
		method: "GET",
		path: "/disallowed",
		query_string: "",
		headers: {"origin" => ["https://other.example"]},
		body: "".to_bytes(),
	))
	puts(disallowed.headers.key?("access-control-allow-origin"))
	puts(disallowed.headers["vary"].join("|"))

	preflight := dispatch(Request.new(
		method: "OPTIONS",
		path: "/allowed",
		query_string: "",
		headers: {
			"origin" => ["https://app.example"],
			"access-control-request-method" => ["POST"],
			"access-control-request-headers" => ["content-type, x-trace"],
		},
		body: "".to_bytes(),
	))
	puts(preflight.status)
	puts(preflight.headers["access-control-allow-methods"][0])
	puts(preflight.headers["access-control-allow-headers"][0])
	puts(preflight.headers["access-control-max-age"][0])
	puts(preflight.headers["vary"].join("|"))
	puts(preflight.headers.key?("x-handler"))

	wildcard := dispatch(Request.new(
		method: "GET",
		path: "/wildcard",
		query_string: "",
		headers: {"origin" => ["https://any.example"]},
		body: "".to_bytes(),
	))
	puts(wildcard.headers["access-control-allow-origin"][0])
	puts(wildcard.headers["vary"].join("|"))
	return
end
`
		middlewareSource := `import { Context, Next, Response } from trb/web
import trb/web/middleware/cors
import { CORSOptions, PreflightMaxAge } from trb/web/middleware/cors

CORS_OPTIONS := CORSOptions.new(
	allow_origins: ["https://app.example"],
	allow_methods: ["GET", "POST", "OPTIONS"],
	allow_headers: [],
	expose_headers: ["x-trace-id"],
	credentials: true,
	max_age: PreflightMaxAge::Seconds(600),
)

def call(context: Context, next_handler: Next): Response
	if context.request.path == "/wildcard"
		return cors.call(context, next_handler)
	end
	return cors.call(context, next_handler, CORS_OPTIONS)
end
`
		routeSource := `import { Context, Response, text, with_header } from trb/web

def get(_context: Context): Response
	return with_header(with_header(text("ok"), "Vary", "Accept"), "X-Handler", "route")
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "_middleware.trb"), []byte(middlewareSource), 0o644); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"allowed.trb", "disallowed.trb", "wildcard.trb"} {
			if err := os.WriteFile(filepath.Join(root, "src", "routes", name), []byte(routeSource), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}
		want := "https://app.example\ntrue\nx-trace-id\nAccept|Origin\nfalse\nAccept|Origin\n204\nGET, POST, OPTIONS\ncontent-type, x-trace\n600\nOrigin|Access-Control-Request-Headers\nfalse\n*\nAccept\n"
		if stdout.String() != want {
			t.Fatalf("unexpected %s trb/web CORS output: want %q, got %q", mode, want, stdout.String())
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

func TestBuildEmitsOfficialPackageOutsideProjectSourceTree(t *testing.T) {
	root := t.TempDir()
	config := project.New(root, "ruby")
	config.SourceDir = "."
	config.OutDir = "build"
	copyFiles := false
	config.CopyFiles = &copyFiles
	if err := config.Save(); err != nil {
		t.Fatal(err)
	}
	source := `import { Response } from trb/web

def response(): Response
	return Response.new(status: 204, headers: {}, body: "".to_bytes())
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
	packageOutput, err := os.ReadFile(filepath.Join(root, "build", "trb", "web", "index.rb"))
	if err != nil || !strings.Contains(string(packageOutput), "Response = Data.define") {
		t.Fatalf("official package was not emitted: err=%v\n%s", err, packageOutput)
	}
	consumer, err := os.ReadFile(filepath.Join(root, "build", "main.rb"))
	if err != nil || !strings.Contains(string(consumer), `require_relative "./trb/web/index"`) {
		t.Fatalf("official package consumer did not require its runtime: err=%v\n%s", err, consumer)
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

func TestRunOfficialWebPathNormalizationAcrossAvailableBackends(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		if mode == "ruby" {
			if _, err := exec.LookPath("ruby"); err != nil {
				t.Log("ruby is not installed; skipping Ruby trb/web path normalization run")
				continue
			}
		}
		if mode == "typescript" {
			if _, err := exec.LookPath("node"); err != nil {
				t.Log("node is not installed; skipping TypeScript trb/web path normalization run")
				continue
			}
		}

		root := t.TempDir()
		config := project.New(root, mode)
		config.SourceDir = "src"
		if config.Go != nil {
			config.Go.Module = "example.com/type-rb/run-web-path-normalization-test"
		}
		if err := config.Save(); err != nil {
			t.Fatal(err)
		}

		mainSource := `import { Request } from trb/web
import { dispatch } from trb/web/testing

def print_response(path: String)
	response := dispatch(Request.new(
		method: "GET",
		path: path,
		query_string: "",
		headers: {},
		body: "".to_bytes(),
	))
	puts(response.status)
	puts(response.body.to_s())
	return
end

def main()
	print_response("/files/hello%20world")
	print_response("/files/%E3%81%82")
	print_response("/files/a+b")
	[
		"/files/%",
		"/files/%FF",
		"/files/%2F",
		"/files/%5c",
		"/files/\\",
		"/files/.",
		"/files/%2e",
		"/files/..",
		"/files/%2E%2e",
		"files/value",
	].each do |path|
		print_response(path)
	end
	print_response("/files//value")
	print_response("/files/value/")
	return
end
`
		routeSource := `import { Context, Response, path_param } from trb/web

def get(context: Context): Response
	value := context.request.path + "|" + path_param(context, "name")
	return Response.new(status: 200, headers: {}, body: value.to_bytes())
end
`
		if err := os.MkdirAll(filepath.Join(root, "src", "routes", "files"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "main.trb"), []byte(mainSource), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "src", "routes", "files", "[name].trb"), []byte(routeSource), 0o644); err != nil {
			t.Fatal(err)
		}

		var stdout, stderr bytes.Buffer
		command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
		if status := command.Run([]string{"run", "--config", config.Path}); status != 0 {
			t.Fatalf("%s status=%d stderr=%s", mode, status, stderr.String())
		}

		var want strings.Builder
		want.WriteString("200\n/files/hello world|hello world\n")
		want.WriteString("200\n/files/あ|あ\n")
		want.WriteString("200\n/files/a+b|a+b\n")
		for range 10 {
			want.WriteString("400\n{\"error\":\"bad_request\"}\n")
		}
		for range 2 {
			want.WriteString("404\n{\"error\":\"not_found\"}\n")
		}
		if stdout.String() != want.String() {
			t.Fatalf("unexpected %s trb/web path output: want %q, got %q", mode, want.String(), stdout.String())
		}
	}
}
