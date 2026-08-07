package repl

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/reeflective/readline/inputrc"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/types"
)

func TestCompleteTracksBlocksAndDelimiters(t *testing.T) {
	tests := []struct {
		source string
		want   bool
	}{
		{"1 + 2", true},
		{"class User", false},
		{"class User\nend", true},
		{"class User;", false},
		{"class User; end", true},
		{"class User; def value(); return; end; end", true},
		{"if true\n  1", false},
		{"if true\n  1\nend", true},
		{"enum State\n\tOpen", false},
		{"enum State\n\tOpen\nend", true},
		{"enum State; Open; Closed; end", true},
		{"enum Token\n\tText(value: String)", false},
		{"enum Token\n\tText(value: String)\n\tEOF\nend", true},
		{"case State::Open\nwhen State::Open\n\t1", false},
		{"case State::Open\nwhen State::Open\n\t1\nend", true},
		{"case State::Open; when State::Open; 1; end", true},
		{"call(\n  1,", false},
		{"call(\n  1,\n)", true},
		{"[1, 2, 3].each do |value|", false},
		{"[1, 2, 3].each do |value|\n  puts(value)\nend", true},
		{"[1, 2, 3].each { |value|", false},
		{"[1, 2, 3].each { |value| puts(value) }", true},
		{"[1, 2, 3].each { |value| puts(value); puts(value) }", true},
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
	service := languageservice.New("go")
	tests := []struct {
		input string
		want  string
	}{
		{input: ":he", want: ":help"},
		{input: "rec", want: "record"},
		{input: "put", want: "puts"},
	}
	for _, test := range tests {
		suggestions := completionSuggestions(service, test.input, len(test.input))
		if len(suggestions) == 0 || suggestions[0].Label != test.want {
			t.Errorf("completionSuggestions(%q)=%v, want first suggestion %q", test.input, suggestions, test.want)
		}
	}
}

func TestCompleteInputUsesCheckedReplContextForMembers(t *testing.T) {
	service := languageservice.New("go")
	service.Update([]*ir.Program{{
		Mode:       "go",
		ModulePath: "repl",
		Statements: []ir.Statement{
			&ir.Class{Name: "User", Body: []ir.Statement{
				&ir.Method{Name: "name", ReturnType: types.FromName("String")},
			}},
			&ir.Variable{Name: "user", Type: types.FromName("User")},
		},
	}}, "repl")

	input := "user.na"
	suggestions := completionSuggestions(service, input, len(input))
	if len(suggestions) != 1 || suggestions[0].Label != "name" {
		t.Fatalf("suggestions=%#v", suggestions)
	}
	if suggestions[0].Replacement != (languageservice.OffsetRange{Start: 5, End: 7}) {
		t.Fatalf("replacement=%#v", suggestions[0].Replacement)
	}
}

func TestHighlightInputRendersANSIWithoutChangingSource(t *testing.T) {
	service := languageservice.New("go")
	service.Update([]*ir.Program{{Mode: "go", ModulePath: "repl"}}, "repl")
	source := "puts(\"hello\") # note"
	rendered := highlightInput(source, service.Highlight(source))
	plain := regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(rendered, "")
	if plain != source {
		t.Fatalf("plain=%q, want %q", plain, source)
	}
	for _, color := range []string{colorFunction, colorString, colorComment} {
		if !strings.Contains(rendered, color) {
			t.Errorf("rendered input is missing color %q: %q", color, rendered)
		}
	}
}

func TestTerminalReaderUsesMultilineAwareHistoryNavigation(t *testing.T) {
	terminal, err := newTerminalReader(Options{Mode: "go", language: languageservice.New("go")}, nil)
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

func TestCompleteTracksValueProducingControlFlow(t *testing.T) {
	for _, source := range []string{
		"value := if enabled\n\t1",
		"def value(result: Result<Integer, String>): Integer\n\treturn case result\n\twhen Result::Ok(value)\n\t\tvalue\n\tend",
	} {
		if Complete(source) {
			t.Fatalf("value-producing control flow should be incomplete: %q", source)
		}
		if !Complete(source + "\nend") {
			t.Fatalf("closed value-producing control flow should be complete: %q", source)
		}
	}
}
