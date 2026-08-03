package repl

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/reeflective/readline/inputrc"
)

func TestCompleteTracksBlocksAndDelimiters(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"1 + 2", true},
		{"class User", false},
		{"class User\nend", true},
		{"if true\n  1", false},
		{"if true\n  1\nend", true},
		{"enum State\n\tOpen", false},
		{"enum State\n\tOpen\nend", true},
		{"case State::Open\nwhen State::Open\n\t1", false},
		{"case State::Open\nwhen State::Open\n\t1\nend", true},
		{"call(\n  1,", false},
		{"call(\n  1,\n)", true},
		{"[1, 2, 3].each do |value|", false},
		{"[1, 2, 3].each do |value|\n  puts(value)\nend", true},
		{"[1, 2, 3].each { |value|", false},
		{"[1, 2, 3].each { |value| puts(value) }", true},
		{"# class does not open a block", true},
		{"\"class\"", true},
	}
	for _, test := range tests {
		if actual := Complete(test.source); actual != test.want {
			t.Errorf("Complete(%q)=%v, want %v", test.source, actual, test.want)
		}
	}
}

func TestHistoryRoundTripsMultilineSubmissions(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "repl_history")
	want := []string{"1 + 2", "def double(value: Integer): Integer\n  return value * 2\nend"}
	if err := saveHistory(filename, want); err != nil {
		t.Fatal(err)
	}
	got, err := loadHistory(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Fatalf("history=%q, want %q", got, want)
	}
}

func TestCompleteInputSuggestsCommandsAndLanguageKeywords(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: ":he", want: ":help"},
		{input: "rec", want: "record"},
		{input: "put", want: "puts"},
	}
	for _, test := range tests {
		suggestions := completionSuggestions(test.input)
		if len(suggestions) == 0 || suggestions[0].Text != test.want {
			t.Errorf("completionSuggestions(%q)=%v, want first suggestion %q", test.input, suggestions, test.want)
		}
	}
}

func TestTerminalReaderUsesMultilineAwareHistoryNavigation(t *testing.T) {
	terminal, err := newTerminalReader(Options{Mode: "go"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.AcceptMultiline([]rune("def greet()")) {
		t.Fatal("incomplete definition should continue on a secondary line")
	}
	if !terminal.AcceptMultiline([]rune("def greet()\nend")) {
		t.Fatal("closed definition should be accepted")
	}
	for _, test := range []struct{ sequence, action string }{
		{sequence: `\C-p`, action: "up-line-or-history"},
		{sequence: `\M-[A`, action: "up-line-or-history"},
		{sequence: `\C-n`, action: "down-line-or-history"},
		{sequence: `\M-[B`, action: "down-line-or-history"},
	} {
		binding := terminal.Config.Binds["emacs"][inputrc.Unescape(test.sequence)]
		if binding.Action != test.action {
			t.Errorf("binding %q=%q, want %q", test.sequence, binding.Action, test.action)
		}
	}
}
