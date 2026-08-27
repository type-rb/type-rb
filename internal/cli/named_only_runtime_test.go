package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNamedOnlyEvaluationAndDefaultsAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
	}
	source := `def mark(value: String): String
	puts(value)
	return value
end

def mark_integer(value: Integer): Integer
	puts(value.to_s())
	return value
end

def describe(count: Integer = mark_integer(7), *, first: String = mark("first-default"), second: String = mark("second-default")): String
	return count.to_s() + ":" + first + ":" + second
end

class BaseClient
	def timeout(*, value: Integer = 10): Integer
		return value
	end
end

class Client < BaseClient
	def timeout(*, value: Integer = 30): Integer
		return value
	end
end

interface Renderer
	render(*, prefix: String, value: String): String
end

class TextRenderer implements Renderer
	def render(*, value: String, prefix: String): String
		return prefix + ":" + value
	end
end

def render_with(renderer: Renderer): String
	return renderer.render(value: "value", prefix: "prefix")
end

def main()
	puts(describe(second: mark("second-argument"), first: mark("first-argument")))
	puts(describe())
	client := Client.new()
	puts(client.timeout())
	puts(render_with(TextRenderer.new()))
	return
end
`
	want := "second-argument\nfirst-argument\n7\n7:first-argument:second-argument\n7\nfirst-default\nsecond-default\n7:first-default:second-default\n30\nprefix:value\n"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			filename := filepath.Join(t.TempDir(), "named_only.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected output\nwant: %q\ngot:  %q\nstderr: %s", want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestNamedOnlyEnumMethodsAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
	}
	source := `def mark_enum(value: String): String
	puts(value)
	return value
end

enum State
	Ready

	def describe(prefix: String = mark_enum("prefix-default"), *, label: String, suffix: String = mark_enum("suffix-default")): String
		return prefix + ":" + label + ":" + suffix
	end
end

def main()
	state := State::Ready
	puts(state.describe(label: mark_enum("label-argument")))
	puts(state.describe("prefix-explicit", suffix: mark_enum("suffix-argument"), label: mark_enum("label-argument-2")))
	return
end
`
	want := "label-argument\nprefix-default\nsuffix-default\nprefix-default:label-argument:suffix-default\nsuffix-argument\nlabel-argument-2\nprefix-explicit:label-argument-2:suffix-argument\n"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			filename := filepath.Join(t.TempDir(), "named_only_enum.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected output\nwant: %q\ngot:  %q\nstderr: %s", want, stdout.String(), stderr.String())
			}
		})
	}
}

func TestNamedOnlyEnumPayloadsAcrossBackends(t *testing.T) {
	tests := []struct {
		name     string
		required string
		args     func(string) []string
	}{
		{name: "go", required: "go", args: func(filename string) []string { return []string{filename} }},
		{name: "ruby", required: "ruby", args: func(filename string) []string { return []string{"run", "--mode", "ruby", filename} }},
		{name: "typescript", required: "node", args: func(filename string) []string {
			return []string{"run", "--mode", "typescript", "--runtime", "node", filename}
		}},
	}
	source := `def mark_payload(value: String): String
	puts(value)
	return value
end

enum Change
	Renamed(id: Integer, *, before: String, after: String)
end

alias ChangeEvent = Change

def describe(change: ChangeEvent): String
	case change
	when ChangeEvent::Renamed(id, after: current, before: previous)
		return id.to_s() + ":" + previous + ":" + current
	end
end

def main()
	change := ChangeEvent::Renamed(7, after: mark_payload("after-argument"), before: mark_payload("before-argument"))
	puts(describe(change))
	return
end
`
	want := "after-argument\nbefore-argument\n7:before-argument:after-argument\n"
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := exec.LookPath(test.required); err != nil {
				t.Skipf("%s is unavailable: %v", test.required, err)
			}
			filename := filepath.Join(t.TempDir(), "named_only_enum_payload.trb")
			if err := os.WriteFile(filename, []byte(source), 0o644); err != nil {
				t.Fatal(err)
			}
			var stdout, stderr bytes.Buffer
			command := &CLI{Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr}
			if status := command.Run(test.args(filename)); status != 0 {
				t.Fatalf("status=%d stdout=%s stderr=%s", status, stdout.String(), stderr.String())
			}
			if stdout.String() != want || stderr.Len() != 0 {
				t.Fatalf("unexpected output\nwant: %q\ngot:  %q\nstderr: %s", want, stdout.String(), stderr.String())
			}
		})
	}
}
