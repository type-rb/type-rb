package repl

import (
	"path/filepath"
	"slices"
	"testing"

	"github.com/nao1215/prompt"
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
		{"call(\n  1,", false},
		{"call(\n  1,\n)", true},
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
		document := prompt.Document{Text: test.input, CursorPosition: len(test.input)}
		suggestions := completeInput(document)
		if len(suggestions) == 0 || suggestions[0].Text != test.want {
			t.Errorf("completeInput(%q)=%v, want first suggestion %q", test.input, suggestions, test.want)
		}
	}
}

func TestTypeRBKeyMapAddsReadlineNavigation(t *testing.T) {
	keyMap := typeRBKeyMap()
	bindings := []struct {
		key    rune
		action prompt.KeyAction
	}{
		{key: '\x02', action: prompt.ActionMoveLeft},
		{key: '\x06', action: prompt.ActionMoveRight},
		{key: '\x10', action: prompt.ActionMoveUp},
		{key: '\x0e', action: prompt.ActionMoveDown},
	}
	for _, binding := range bindings {
		if got := keyMap.GetAction(binding.key); got != binding.action {
			t.Errorf("key %U action=%v, want %v", binding.key, got, binding.action)
		}
	}
	if got := keyMap.GetSequenceAction("b"); got != prompt.ActionMoveWordLeft {
		t.Errorf("Alt-B action=%v, want %v", got, prompt.ActionMoveWordLeft)
	}
	if got := keyMap.GetSequenceAction("f"); got != prompt.ActionMoveWordRight {
		t.Errorf("Alt-F action=%v, want %v", got, prompt.ActionMoveWordRight)
	}
}
