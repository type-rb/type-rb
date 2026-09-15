package repl

import (
	"bytes"
	"os"
	"runtime"
	"strings"
	"testing"
)

type interruptOnReady struct {
	bytes.Buffer
	sent bool
	err  error
}

func (w *interruptOnReady) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if !w.sent && strings.Contains(w.Buffer.String(), "ready\n") {
		w.sent = true
		process, findErr := os.FindProcess(os.Getpid())
		w.err = findErr
		if findErr == nil {
			w.err = process.Signal(os.Interrupt)
		}
	}
	return n, err
}

func TestRunInvalidatesFlowAfterInterruptedMutation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	input := "mut value: String? := nil\nvalue = \"kept\"\n" +
		"if true\nvalue = nil\nputs(\"ready\")\nwhile true\nend\nend\n" +
		":type value\nvalue == nil\n:quit\n"
	var stdout interruptOnReady
	var stderr bytes.Buffer
	err := Run(Options{Mode: "go", Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler("go")})
	want := "nil : String? [mut]\n\"kept\" : String? [mut]\nready\ninterrupted\nString?\ntrue : Boolean\n"
	if err != nil || stdout.err != nil || stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("err=%v signal=%v stdout=%q stderr=%q", err, stdout.err, stdout.String(), stderr.String())
	}
}

func TestRunInvalidatesFlowAfterPartialRuntimeFailure(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		for _, body := range []string{
			"if true\nvalue = nil\nputs(1 / 0)\nend",
			"value = if true\nvalue = nil\nputs(1 / 0)\n\"unused\"\nelse\n\"other\"\nend",
		} {
			t.Run(mode+"/"+body[:2], func(t *testing.T) {
				input := "mut value: String? := nil\nvalue = \"kept\"\nsaved := value\n" + body +
					"\n:type value\nvalue == nil\nvalue.size()\nsaved.size()\nvalue = \"again\"\nvalue.size()\n:quit\n"
				var stdout, stderr bytes.Buffer
				err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
				if err != nil {
					t.Fatal(err)
				}
				want := "nil : String? [mut]\n\"kept\" : String? [mut]\n\"kept\" : String\nString?\ntrue : Boolean\n4 : Integer\n\"again\" : String? [mut]\n5 : Integer\n"
				if stdout.String() != want || !strings.Contains(stderr.String(), "division by zero") ||
					!strings.Contains(stderr.String(), "type String? has no member size") ||
					strings.Contains(stderr.String(), "cannot use nil") || len(strings.Split(strings.TrimSpace(stderr.String()), "\n")) != 2 {
					t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
				}
			})
		}
	}
}

func TestRunPreservesEffectsAndResetsFlowAgainAfterLaterFailure(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			input := "mut events := [1]\nmut value: String? := nil\nvalue = \"kept\"\n" +
				"if true\nevents.push(2)\nvalue = nil\nputs(1 / 0)\nend\n" +
				"puts(events.size())\nvalue == nil\nvalue = \"again\"\nvalue = 2\n:type value\n" +
				"if true\nevents.push(3)\nvalue = nil\nputs(1 / 0)\nend\n" +
				":type value\nputs(events.size())\nvalue == nil\n:quit\n"
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			want := "[1] : Array<Integer> [mut]\nnil : String? [mut]\n\"kept\" : String? [mut]\n2\ntrue : Boolean\n\"again\" : String? [mut]\nString\nString?\n3\ntrue : Boolean\n"
			if err != nil || stdout.String() != want || strings.Count(stderr.String(), "division by zero") != 2 ||
				!strings.Contains(stderr.String(), "cannot assign Integer to String?") ||
				len(strings.Split(strings.TrimSpace(stderr.String()), "\n")) != 3 {
				t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunReloadPreservesCheckingBoundariesAndRebuildsRuntimeState(t *testing.T) {
	for _, mode := range []string{"go", "ruby", "typescript"} {
		t.Run(mode, func(t *testing.T) {
			input := "mut events := [1]\nevents.push(2)\nmut value: String? := nil\nvalue = \"kept\"\n" +
				"if true\nevents.push(3)\nvalue = nil\nputs(1 / 0)\nend\n:type value\n" +
				"value == nil\n:reload\n:type value\nif value != nil\nputs(value.size())\nend\nputs(events.size())\n:quit\n"
			var stdout, stderr bytes.Buffer
			err := Run(Options{Mode: mode, Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Compile: conditionalSessionCompiler(mode)})
			want := "[1] : Array<Integer> [mut]\nnil : String? [mut]\n\"kept\" : String? [mut]\nString?\ntrue : Boolean\nreloaded\nString?\n4\n2\n"
			if err != nil || stdout.String() != want || strings.Count(stderr.String(), "division by zero") != 1 ||
				len(strings.Split(strings.TrimSpace(stderr.String()), "\n")) != 1 {
				t.Fatalf("err=%v stdout=%q stderr=%q", err, stdout.String(), stderr.String())
			}
		})
	}
}
