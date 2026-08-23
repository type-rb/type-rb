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
	"syscall"
	"testing"
	"time"

	"github.com/creack/pty"
	"github.com/type-rb/type-rb/internal/languageservice"
)

const replCompletionPTYChild = "TRB_REPL_COMPLETION_PTY_CHILD"

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

func runCompletionPTY(t *testing.T, input string) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCompletionPTYChild$")
	command.Env = append(os.Environ(), replCompletionPTYChild+"=1")
	terminal, err := pty.Start(command)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	if _, err := terminal.Write([]byte(input)); err != nil {
		t.Fatal(err)
	}
	output, err := io.ReadAll(terminal)
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

func TestCompletionPTYChild(t *testing.T) {
	if os.Getenv(replCompletionPTYChild) != "1" {
		t.Skip("PTY helper process")
	}

	terminal, err := newTerminalReader(Options{Mode: "go", language: languageservice.New("go")}, nil)
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

	line, err := terminal.Readline()
	if err != nil {
		t.Fatal(err)
	}
	fmt.Fprintf(os.Stdout, "\r\n[LINE:%s]\r\n", line)
}
