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
		{"double := fn(value: Integer): Integer\n\treturn value * 2\nend", true},
		{"double := fn(value: Integer): Integer", false},
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

func TestTerminalReaderDisplaysCanonicalHistoryWithInteractiveIndentation(t *testing.T) {
	canonical := "class A\n\tdef abc()\n\tend\nend"
	terminal, err := newTerminalReader(Options{Mode: "go", language: languageservice.New("go")}, []string{canonical})
	if err != nil {
		t.Fatal(err)
	}
	got, err := terminal.History.Current().GetLine(0)
	if err != nil {
		t.Fatal(err)
	}
	want := "class A\n  def abc()\n  end\nend"
	if got != want {
		t.Fatalf("display history=%q, want %q", got, want)
	}
}

func TestTerminalReaderStoresCanonicalHistory(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "repl_history")
	terminal, err := newTerminalReader(Options{Mode: "go", language: languageservice.New("go")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := terminal.History.Current().Write("class A\n  def abc()\n  end\nend"); err != nil {
		t.Fatal(err)
	}
	reader := &terminalSubmissionReader{terminal: terminal, options: Options{HistoryFile: filename}}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := loadHistory(filename)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"class A\n\tdef abc()\n\tend\nend"}
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

func TestAcceptedCompletionSourceAppliesVisibleImportAndPrimaryReplacement(t *testing.T) {
	source := "note := \"😀\"\nma"
	start := len("note := \"😀\"\n")
	item := languageservice.CompletionItem{
		InsertText:  "math",
		Replacement: languageservice.OffsetRange{Start: start, End: len(source)},
		AdditionalEdits: []languageservice.TextEdit{{
			Range: languageservice.OffsetRange{}, NewText: "import trb/std/math\n",
		}},
	}
	got, cursor, ok := acceptedCompletionSource(source, item)
	want := "import trb/std/math\nnote := \"😀\"\nmath"
	if !ok || got != want {
		t.Fatalf("accepted completion=(%q, %v), want (%q, true)", got, ok, want)
	}
	if cursor != len(want) {
		t.Fatalf("cursor=%d, want %d", cursor, len(want))
	}
}

func TestAcceptedImportConfirmationSourceTurnsBareCandidateIntoImportSubmission(t *testing.T) {
	source := "sha"
	item := languageservice.CompletionItem{
		InsertText:  "sha256",
		Replacement: languageservice.OffsetRange{Start: 0, End: len(source)},
		AdditionalEdits: []languageservice.TextEdit{{
			Range: languageservice.OffsetRange{}, NewText: "import { sha256 } from trb/std/digest\n",
		}},
	}
	got, cursor, ok := acceptedImportConfirmationSource(source, item)
	want := "import { sha256 } from trb/std/digest"
	if !ok || got != want || cursor != len(want) {
		t.Fatalf("bare completion=(%q, %d, %v), want (%q, %d, true)", got, cursor, ok, want, len(want))
	}
}

func TestAcceptedCompletionSourceKeepsBareCandidateAfterImport(t *testing.T) {
	source := "math"
	item := languageservice.CompletionItem{
		Kind:        languageservice.CompletionModule,
		InsertText:  "math",
		Replacement: languageservice.OffsetRange{Start: 0, End: len(source)},
		AdditionalEdits: []languageservice.TextEdit{{
			Range: languageservice.OffsetRange{}, NewText: "import trb/std/math\n",
		}},
	}
	got, cursor, ok := acceptedCompletionSource(source, item)
	want := "import trb/std/math\nmath"
	if !ok || got != want || cursor != len(want) {
		t.Fatalf("expression completion=(%q, %d, %v), want (%q, %d, true)", got, cursor, ok, want, len(want))
	}
}

func TestCompletionCommitCharactersAreLimitedToExpressionContinuations(t *testing.T) {
	tests := []struct {
		name                 string
		item                 languageservice.CompletionItem
		requireConfirmation  bool
		wantCommitCharacters string
	}{
		{
			name:                 "package namespace",
			item:                 languageservice.CompletionItem{Kind: languageservice.CompletionModule, InsertText: "math"},
			requireConfirmation:  true,
			wantCommitCharacters: ".",
		},
		{
			name:                 "parameterized function",
			item:                 languageservice.CompletionItem{Kind: languageservice.CompletionFunction, InsertText: "sha256"},
			requireConfirmation:  true,
			wantCommitCharacters: "(",
		},
		{
			name:                "zero-argument function",
			item:                languageservice.CompletionItem{Kind: languageservice.CompletionFunction, InsertText: "now()"},
			requireConfirmation: true,
		},
		{
			name: "ordinary completion",
			item: languageservice.CompletionItem{Kind: languageservice.CompletionModule, InsertText: "math"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := completionCommitCharacters(test.item, test.requireConfirmation); got != test.wantCommitCharacters {
				t.Fatalf("commit characters=%q, want %q", got, test.wantCommitCharacters)
			}
		})
	}
}

func TestAcceptedCompletionSourceRejectsOverlappingEdits(t *testing.T) {
	source := "math"
	item := languageservice.CompletionItem{
		InsertText:  "math",
		Replacement: languageservice.OffsetRange{Start: 0, End: len(source)},
		AdditionalEdits: []languageservice.TextEdit{{
			Range: languageservice.OffsetRange{Start: 1, End: 2}, NewText: "x",
		}},
	}
	if got, _, ok := acceptedCompletionSource(source, item); ok || got != source {
		t.Fatalf("overlapping edits=(%q, %v), want (%q, false)", got, ok, source)
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
	if !terminal.Config.GetBool("menu-complete-display-prefix") {
		t.Fatal("completion menu is not configured to display multiple candidates before cycling")
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
	for _, keymap := range []string{"emacs", "emacs-standard", "vi-insert", "vi-command"} {
		for _, sequence := range []string{`\C-j`, `\C-m`} {
			binding := terminal.Config.Binds[keymap][inputrc.Unescape(sequence)]
			if binding.Action != "trb-accept-line" {
				t.Errorf("%s binding %q=%q, want trb-accept-line", keymap, sequence, binding.Action)
			}
		}
	}
}

func TestTerminalAcceptLineCommitsSelectedImportCompletionWithoutSubmitting(t *testing.T) {
	language := languageservice.New("go")
	language.SetCandidates(languageservice.Context{Symbols: []languageservice.Symbol{
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "hmac",
			Import: &languageservice.Import{Path: "trb/std/hmac", Symbol: "sha256"},
			Call:   &languageservice.CallInfo{ParameterCount: 2},
		},
		{
			Name: "sha256", Kind: languageservice.CompletionFunction, Detail: "digest",
			Import: &languageservice.Import{Path: "trb/std/digest", Symbol: "sha256"},
			Call:   &languageservice.CallInfo{ParameterCount: 1},
		},
	}})
	terminal, err := newTerminalReader(Options{Mode: "go", language: language}, nil)
	if err != nil {
		t.Fatal(err)
	}
	terminal.Line().Set([]rune("sha256")...)
	terminal.Cursor().Set(6)
	complete := terminal.Keymap.Commands()["complete"]
	complete()
	terminal.Keymap.Commands()["accept-line"]()

	if got := string(*terminal.Line()); got != "import { sha256 } from trb/std/digest" && got != "import { sha256 } from trb/std/hmac" {
		t.Fatalf("accepted line=%q, want selected import", got)
	}
	if accepted, _, _ := terminal.History.LineAccepted(); accepted {
		t.Fatal("completion confirmation submitted the import")
	}
}

func TestReindentOpenInputCorrectsExistingLinesAndIndentsTheCursorLine(t *testing.T) {
	terminal, err := newTerminalReader(Options{Mode: "go", language: languageservice.New("go")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	input := []rune("class User\n      def name(): String\n return \"Ada\"\n")
	terminal.Line().Set(input...)
	terminal.Cursor().Set(len(input))
	reindentOpenInput(terminal)
	want := "class User\n  def name(): String\n    return \"Ada\"\n    "
	if got := string(*terminal.Line()); got != want {
		t.Fatalf("open input\nwant:\n%q\ngot:\n%q", want, got)
	}
	if got := terminal.Cursor().Pos(); got != len([]rune(want)) {
		t.Fatalf("cursor=%d, want %d", got, len([]rune(want)))
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

func TestCompleteTracksResultCatch(t *testing.T) {
	for _, source := range []string{
		"value := source() catch |error|\n\terror.message",
		"value := Database.transaction() do |tx|\n\twork(tx)\nend catch |error|\n\tfallback(error)",
	} {
		if Complete(source) {
			t.Fatalf("open catch handler should be incomplete: %q", source)
		}
		if !Complete(source + "\nend") {
			t.Fatalf("closed catch handler should be complete: %q", source)
		}
	}
}
