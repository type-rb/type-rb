//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package repl

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/hinshun/vt10x"
	"github.com/type-rb/type-rb/internal/languageservice"
)

const replCompletionPTYChild = "TRB_REPL_COMPLETION_PTY_CHILD"
const replScreenPTYChild = "TRB_REPL_SCREEN_PTY_CHILD"

func TestCompletionRemainsCommittedBeforeTrailingInput(t *testing.T) {
	output := runCompletionPTY(t, "pu\t(1)\r")
	if !bytes.Contains(output, []byte("[LINE:puts(1)]")) {
		t.Fatalf("completed line was not preserved: %q", output)
	}
}

func TestZeroArgumentCompletionInsertsParentheses(t *testing.T) {
	output := runCompletionPTY(t, "\"hello\".si\t\r")
	if !bytes.Contains(output, []byte("[LINE:\"hello\".size()]")) {
		t.Fatalf("zero-argument call was not completed: %q", output)
	}
}

func TestRangeLiteralCompletionInsertsToArray(t *testing.T) {
	output := runCompletionPTY(t, "(1..10).to\t\r")
	if !bytes.Contains(output, []byte("[LINE:(1..10).to_a()]")) {
		t.Fatalf("Range literal call was not completed: %q", output)
	}
}

func TestCompletionAppliesVisibleImportEdit(t *testing.T) {
	output := bytes.ReplaceAll(runCompletionPTY(t, "math.sq\t(9)\r"), []byte("\r"), nil)
	if !bytes.Contains(output, []byte("[LINE:import trb/std/math\nmath.sqrt(9)]")) {
		t.Fatalf("completion import was not preserved in the submitted line: %q", output)
	}
}

func TestBareUniqueCompletionSubmitsOnlyItsImport(t *testing.T) {
	output := bytes.ReplaceAll(runCompletionPTY(t, "ma\t\r"), []byte("\r"), nil)
	if !bytes.Contains(output, []byte("[LINE:import trb/std/math]")) {
		t.Fatalf("bare package completion did not submit its import: %q", output)
	}
}

func TestBareSelectedCompletionSubmitsOnlySelectedImport(t *testing.T) {
	output := bytes.ReplaceAll(runCompletionPTY(t, "sha\t\t\r"), []byte("\r"), nil)
	if !bytes.Contains(output, []byte("[LINE:import { sha256 } from trb/std/hmac]")) {
		t.Fatalf("selected function completion did not submit its import: %q", output)
	}
}

func TestMultilineInputIsAutomaticallyFormatted(t *testing.T) {
	input := "class User\r" +
		"    def value(): Integer\r" +
		"      return 1\r" +
		"       end\r" +
		"      end\r"
	output := bytes.ReplaceAll(runCompletionPTY(t, input), []byte("\r"), nil)
	want := []byte("[LINE:class User\n\tdef value(): Integer\n\t\treturn 1\n\tend\nend]")
	if !bytes.Contains(output, want) {
		t.Fatalf("multiline input was not formatted: %q", output)
	}
}

func TestCompletedMultilineInputRemainsVisibleAfterFormatting(t *testing.T) {
	output := runPTY(t, "class A\rend\r", true)
	terminal := vt10x.New(vt10x.WithSize(80, 24))
	if _, err := terminal.Write(output); err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(terminal.String(), "\n") {
		if strings.TrimSpace(line) == "end" {
			return
		}
	}
	t.Fatalf("accepted end was erased from the terminal:\n%s\nraw=%q", terminal.String(), output)
}

func TestCompletedNestedMultilineInputRetainsInteractiveIndentation(t *testing.T) {
	output := runPTY(t, "class A\rdef abc()\rend\rend\r", true)
	terminal := vt10x.New(vt10x.WithSize(80, 24))
	if _, err := terminal.Write(output); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(terminal.String(), "\n")
	want := []string{
		"trb:go> class A",
		"          def abc()",
		"          end",
		"        end",
	}
	for index, expected := range want {
		if actual := strings.TrimRight(lines[index], " "); actual != expected {
			t.Fatalf("rendered line %d=%q, want %q:\n%s\nraw=%q", index+1, actual, expected, terminal.String(), output)
		}
	}
}

func runCompletionPTY(t *testing.T, input string) []byte {
	return runPTY(t, input, false)
}

func runPTY(t *testing.T, input string, screenOnly bool) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCompletionPTYChild$")
	command.Env = append(os.Environ(), replCompletionPTYChild+"=1")
	if screenOnly {
		command.Env = append(command.Env, replScreenPTYChild+"=1")
	}
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	var output []byte
	if screenOnly {
		output = readPTYUntil(t, terminal, []byte("trb:go> "))
	}
	if _, err := terminal.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	rest, err := io.ReadAll(terminal)
	output = append(output, rest...)
	if err != nil && ctx.Err() == nil && !errors.Is(err, syscall.EIO) {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatalf("timed out waiting for the REPL: %q", output)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("REPL helper failed: %v: %q", err, output)
	}
	return output
}

func readPTYUntil(t *testing.T, terminal io.Reader, marker []byte) []byte {
	t.Helper()
	var output []byte
	buffer := make([]byte, 256)
	for !bytes.Contains(output, marker) {
		count, err := terminal.Read(buffer)
		output = append(output, buffer[:count]...)
		if err != nil {
			t.Fatalf("PTY closed before marker %q: %v: %q", marker, err, output)
		}
	}
	return output
}

func TestCompletionPTYChild(t *testing.T) {
	if os.Getenv(replCompletionPTYChild) != "1" {
		t.Skip("PTY helper process")
	}

	language := languageservice.New("go")
	packageImport := &languageservice.Import{Path: "trb/std/math", ModulePath: "trb/std/math"}
	language.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{
		{
			Name: "math", Kind: languageservice.CompletionModule, Detail: "trb/std/math", Import: packageImport,
			Members: []languageservice.Symbol{{
				Name: "sqrt", Kind: languageservice.CompletionFunction, Detail: "sqrt(value: Float): Float", Import: packageImport,
				Call: &languageservice.CallInfo{ParameterCount: 1},
			}},
		},
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "sha256(value: Bytes): Bytes — trb/std/hash",
			Import: &languageservice.Import{Path: "trb/std/hash", ModulePath: "trb/std/hash/index", Symbol: "sha256"},
			Call:   &languageservice.CallInfo{ParameterCount: 1},
		},
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "sha256(key: Bytes, value: Bytes): Bytes — trb/std/hmac",
			Import: &languageservice.Import{Path: "trb/std/hmac", ModulePath: "trb/std/hmac/index", Symbol: "sha256"},
			Call:   &languageservice.CallInfo{ParameterCount: 2},
		},
	}})
	terminal, err := newTerminalReader(Options{Mode: "go", language: language}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]bool{
		"autopairs":             false,
		"cursor-position-probe": false,
	} {
		if err := terminal.Config.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	reader := &terminalSubmissionReader{terminal: terminal}
	line, err := reader.Read()
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv(replScreenPTYChild) == "1" {
		return
	}
	fmt.Fprintf(os.Stdout, "\r\n[LINE:%s]\r\n", line)
}
