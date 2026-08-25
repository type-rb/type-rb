//go:build !js || !wasm

package repl

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/reeflective/readline"
	"github.com/reeflective/readline/inputrc"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/languageservice"
)

var (
	errInputInterrupted = errors.New("REPL input interrupted")
	errIncompleteInput  = errors.New("incomplete REPL input")
)

const interactiveIndentation = "  "

type submissionReader interface {
	Read() (string, error)
	Close() error
}

func newSubmissionReader(options Options) (submissionReader, error) {
	if !options.Interactive {
		scanner := bufio.NewScanner(options.Stdin)
		scanner.Buffer(make([]byte, 1024), 1024*1024)
		return &scannerSubmissionReader{scanner: scanner}, nil
	}
	if options.HistoryFile != "" {
		if err := os.MkdirAll(filepath.Dir(options.HistoryFile), 0o755); err != nil {
			return nil, err
		}
	}
	history, err := loadHistory(options.HistoryFile)
	if err != nil {
		return nil, err
	}
	terminal, err := newTerminalReader(options, slices.DeleteFunc(history, isQuitCommand))
	if err != nil {
		return nil, err
	}
	return &terminalSubmissionReader{terminal: terminal, options: options}, nil
}

func newTerminalReader(options Options, history []string) (*readline.Shell, error) {
	terminal := readline.NewShell(inputrc.WithApp("trb"))
	terminal.Prompt.Primary(func() string {
		return colorTitle + "trb:" + options.Mode + "> " + colorReset
	})
	terminal.Prompt.Secondary(func() string {
		return colorTitle + "trb:" + options.Mode + "*  " + colorReset
	})
	terminal.AcceptMultiline = func(line []rune) bool { return Complete(string(line)) }
	if err := configureInteractiveFormatting(terminal); err != nil {
		return nil, err
	}
	terminal.SyntaxHighlighter = func(line []rune) string {
		source := string(line)
		if options.language == nil {
			return colorInput + source + colorReset
		}
		return highlightInput(source, options.language.Highlight(source))
	}
	terminal.Completer = func(line []rune, cursor int) readline.Completions {
		return completeInput(options.language, line, cursor)
	}
	if err := terminal.Config.Set("enable-bracketed-paste", true); err != nil {
		return nil, err
	}
	if err := terminal.Config.Set("menu-complete-display-prefix", true); err != nil {
		return nil, err
	}
	for _, keymap := range []string{"emacs", "emacs-standard"} {
		for _, binding := range []struct{ sequence, action string }{
			{sequence: `\C-p`, action: "up-line-or-history"},
			{sequence: `\M-[A`, action: "up-line-or-history"},
			{sequence: `\C-n`, action: "down-line-or-history"},
			{sequence: `\M-[B`, action: "down-line-or-history"},
		} {
			if err := terminal.Config.Bind(keymap, inputrc.Unescape(binding.sequence), binding.action, false); err != nil {
				return nil, err
			}
		}
	}
	for _, entry := range history {
		if _, err := terminal.History.Current().Write(interactiveDisplayInput(entry)); err != nil {
			return nil, err
		}
	}
	return terminal, nil
}

func configureInteractiveFormatting(terminal *readline.Shell) error {
	acceptLine := terminal.Keymap.Commands()["accept-line"]
	if acceptLine == nil {
		return errors.New("readline accept-line command is unavailable")
	}
	terminal.Keymap.Register(map[string]func(){
		"trb-accept-line": func() {
			if Complete(string(*terminal.Line())) {
				if formatCompleteInput(terminal) {
					// The formatter can move text between terminal columns, such as
					// dedenting a closing end. Redraw before readline clears below
					// the accepted line so its display geometry matches the buffer.
					terminal.Display.Refresh()
				}
				acceptLine()
				return
			}
			acceptLine()
			reindentOpenInput(terminal)
		},
	})
	for _, keymap := range []string{"emacs", "emacs-standard", "vi-insert", "vi-command"} {
		for _, sequence := range []string{`\C-j`, `\C-m`} {
			if err := terminal.Config.Bind(keymap, inputrc.Unescape(sequence), "trb-accept-line", false); err != nil {
				return err
			}
		}
	}
	return nil
}

func formatCompleteInput(terminal *readline.Shell) bool {
	source := string(*terminal.Line())
	canonical, ok := canonicalInput(source)
	if !ok {
		return false
	}
	line := []rune(interactiveDisplayInput(canonical))
	terminal.Line().Set(line...)
	terminal.Cursor().Set(len(line))
	return string(line) != source
}

func canonicalInput(source string) (string, bool) {
	formatted, diagnostics := formatter.Format([]byte(source))
	if len(diagnostics) > 0 {
		return source, false
	}
	return strings.TrimSuffix(string(formatted), "\n"), true
}

func interactiveDisplayInput(source string) string {
	return string(formatter.ReindentPartialWithIndentation([]byte(source), interactiveIndentation))
}

func reindentOpenInput(terminal *readline.Shell) {
	source := []rune(string(*terminal.Line()))
	cursor := terminal.Cursor().Pos()
	if cursor < 0 || cursor > len(source) {
		return
	}
	cursorLine := strings.Count(string(source[:cursor]), "\n")
	indent := formatter.NextLineIndentWithIndentation([]byte(string(source[:cursor])), interactiveIndentation)
	formattedLines := strings.Split(string(formatter.ReindentPartialWithIndentation([]byte(string(source)), interactiveIndentation)), "\n")
	if cursorLine >= len(formattedLines) {
		return
	}
	if strings.TrimSpace(formattedLines[cursorLine]) == "" {
		formattedLines[cursorLine] = indent
	}
	formatted := strings.Join(formattedLines, "\n")
	cursor = 0
	for lineIndex := 0; lineIndex < cursorLine; lineIndex++ {
		cursor += utf8.RuneCountInString(formattedLines[lineIndex]) + 1
	}
	cursor += utf8.RuneCountInString(formattedLines[cursorLine]) - utf8.RuneCountInString(strings.TrimLeft(formattedLines[cursorLine], " \t"))
	terminal.Line().Set([]rune(formatted)...)
	terminal.Cursor().Set(cursor)
}

func isQuitCommand(input string) bool {
	switch strings.TrimSpace(input) {
	case ":quit", ":exit", ":q":
		return true
	default:
		return false
	}
}

type scannerSubmissionReader struct {
	scanner *bufio.Scanner
}

func (r *scannerSubmissionReader) Read() (string, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return "", err
		}
		return "", io.EOF
	}
	snippet := r.scanner.Text()
	for !Complete(snippet) {
		if !r.scanner.Scan() {
			if err := r.scanner.Err(); err != nil {
				return "", err
			}
			return "", errIncompleteInput
		}
		snippet += "\n" + r.scanner.Text()
	}
	return snippet, nil
}

func (*scannerSubmissionReader) Close() error { return nil }

type terminalSubmissionReader struct {
	terminal *readline.Shell
	options  Options
}

func (r *terminalSubmissionReader) Read() (string, error) {
	input, err := r.terminal.Readline()
	switch {
	case errors.Is(err, io.EOF):
		return "", io.EOF
	case errors.Is(err, readline.ErrInterrupt):
		return "", errInputInterrupted
	default:
		if err == nil {
			if canonical, ok := canonicalInput(input); ok {
				input = canonical
			}
		}
		return input, err
	}
}

func (r *terminalSubmissionReader) Close() error {
	history := make([]string, 0, r.terminal.History.Current().Len())
	for index := range r.terminal.History.Current().Len() {
		entry, err := r.terminal.History.Current().GetLine(index)
		if err != nil {
			return err
		}
		if canonical, ok := canonicalInput(entry); ok {
			entry = canonical
		}
		history = append(history, entry)
	}
	history = slices.DeleteFunc(history, isQuitCommand)
	if len(history) > 1000 {
		history = history[len(history)-1000:]
	}
	return saveHistory(r.options.HistoryFile, history)
}

func loadHistory(filename string) ([]string, error) {
	if filename == "" {
		return nil, nil
	}
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var history []string
	if json.Unmarshal(data, &history) == nil {
		return history, nil
	}
	// v0.1 initially stored one entry per line. Retain those entries once, then
	// rewrite the file as JSON so future multiline submissions round-trip.
	for _, line := range strings.Split(string(data), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			history = append(history, line)
		}
	}
	return history, nil
}

func saveHistory(filename string, history []string) error {
	if filename == "" {
		return nil
	}
	data, err := json.Marshal(history)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filename, append(data, '\n'), 0o600); err != nil {
		return err
	}
	return os.Chmod(filename, 0o600)
}

var commandCompletions = []languageservice.CompletionItem{
	{Label: ":help", Kind: languageservice.CompletionCommand, Detail: "show REPL commands"},
	{Label: ":type", Kind: languageservice.CompletionCommand, Detail: "show an expression's checked type"},
	{Label: ":load", Kind: languageservice.CompletionCommand, Detail: "load a .trb file"},
	{Label: ":reload", Kind: languageservice.CompletionCommand, Detail: "reload the project"},
	{Label: ":quit", Kind: languageservice.CompletionCommand, Detail: "leave the REPL"},
}

func completionSuggestions(service *languageservice.Service, input string, cursor int) []languageservice.CompletionItem {
	if cursor < 0 || cursor > len(input) {
		return nil
	}
	if strings.HasPrefix(strings.TrimLeft(input, " \t"), ":") {
		prefix := input[:cursor]
		start := strings.LastIndexAny(prefix, " \t\r\n") + 1
		replacement := languageservice.OffsetRange{Start: start, End: cursor}
		var suggestions []languageservice.CompletionItem
		for _, item := range commandCompletions {
			if strings.HasPrefix(item.Label, prefix[start:]) {
				item.Replacement = replacement
				suggestions = append(suggestions, item)
			}
		}
		return suggestions
	}
	if service == nil {
		return nil
	}
	return service.Complete(input, cursor)
}

func completeInput(service *languageservice.Service, line []rune, cursor int) readline.Completions {
	if cursor < 0 || cursor > len(line) {
		return readline.Completions{}
	}
	source := string(line)
	byteCursor := len(string(line[:cursor]))
	suggestions := completionSuggestions(service, source, byteCursor)
	values := make([]readline.Completion, 0, len(suggestions))
	for _, item := range suggestions {
		value := item.InsertText
		if value == "" {
			value = item.Label
		}
		accepted := item
		accepted.InsertText = value
		_, requireConfirmation := bareCompletionImport(source, accepted)
		completion := readline.Completion{
			Value:               value,
			Display:             item.Label,
			Description:         item.Detail,
			Tag:                 string(item.Kind),
			RequireConfirmation: requireConfirmation,
			OnAccept: func(line []rune, cursor int) ([]rune, int) {
				updated, byteCursor, ok := acceptedImportConfirmationSource(source, accepted)
				if !ok {
					return line, cursor
				}
				return []rune(updated), utf8.RuneCountInString(updated[:byteCursor])
			},
		}
		if characters := completionCommitCharacters(accepted, requireConfirmation); characters != "" {
			completion.CommitCharacters = characters
			completion.OnCommit = func(line []rune, cursor int, _ rune) ([]rune, int) {
				updated, byteCursor, ok := acceptedCompletionSource(source, accepted)
				if !ok {
					return line, cursor
				}
				return []rune(updated), utf8.RuneCountInString(updated[:byteCursor])
			}
		}
		values = append(values, completion)
	}
	if len(values) == 0 {
		return readline.Completions{}
	}
	replacement := suggestions[0].Replacement
	if replacement.Start < 0 || replacement.Start > byteCursor || replacement.End < byteCursor || replacement.End > len(source) || !utf8.ValidString(source[replacement.Start:replacement.End]) {
		return readline.Completions{}
	}
	result := readline.CompleteRaw(values).JustifyDescriptions()
	result.PREFIX = source[replacement.Start:byteCursor]
	result.SUFFIX = source[byteCursor:replacement.End]
	return result
}

func acceptedImportConfirmationSource(source string, item languageservice.CompletionItem) (string, int, bool) {
	if imported, ok := bareCompletionImport(source, item); ok {
		return imported, len(imported), true
	}
	return acceptedCompletionSource(source, item)
}

func acceptedCompletionSource(source string, item languageservice.CompletionItem) (string, int, bool) {
	type completionEdit struct {
		range_  languageservice.OffsetRange
		text    string
		primary bool
	}
	edits := make([]completionEdit, 0, len(item.AdditionalEdits)+1)
	for _, edit := range item.AdditionalEdits {
		edits = append(edits, completionEdit{range_: edit.Range, text: edit.NewText})
	}
	edits = append(edits, completionEdit{range_: item.Replacement, text: item.InsertText, primary: true})
	for _, edit := range edits {
		if edit.range_.Start < 0 || edit.range_.Start > edit.range_.End || edit.range_.End > len(source) ||
			!utf8.ValidString(source[:edit.range_.Start]) || !utf8.ValidString(source[:edit.range_.End]) {
			return source, 0, false
		}
	}
	ascending := append([]completionEdit(nil), edits...)
	sort.SliceStable(ascending, func(left, right int) bool {
		if ascending[left].range_.Start != ascending[right].range_.Start {
			return ascending[left].range_.Start < ascending[right].range_.Start
		}
		return ascending[left].range_.End < ascending[right].range_.End
	})
	for index := 1; index < len(ascending); index++ {
		if ascending[index-1].range_.End > ascending[index].range_.Start {
			return source, 0, false
		}
	}

	cursor := item.Replacement.Start + len(item.InsertText)
	for _, edit := range item.AdditionalEdits {
		if edit.Range.End <= item.Replacement.Start {
			cursor += len(edit.NewText) - (edit.Range.End - edit.Range.Start)
		}
	}
	sort.SliceStable(edits, func(left, right int) bool {
		if edits[left].range_.Start != edits[right].range_.Start {
			return edits[left].range_.Start > edits[right].range_.Start
		}
		if edits[left].primary != edits[right].primary {
			return edits[left].primary
		}
		return edits[left].range_.End > edits[right].range_.End
	})
	updated := source
	for _, edit := range edits {
		updated = updated[:edit.range_.Start] + edit.text + updated[edit.range_.End:]
	}
	if cursor < 0 || cursor > len(updated) || !utf8.ValidString(updated[:cursor]) {
		return source, 0, false
	}
	return updated, cursor, true
}

func completionCommitCharacters(item languageservice.CompletionItem, requireConfirmation bool) string {
	if !requireConfirmation {
		return ""
	}
	switch item.Kind {
	case languageservice.CompletionModule:
		return "."
	case languageservice.CompletionFunction:
		if !strings.HasSuffix(item.InsertText, "()") {
			return "("
		}
	}
	return ""
}

func bareCompletionImport(source string, item languageservice.CompletionItem) (string, bool) {
	if item.Replacement.Start != 0 || item.Replacement.End != len(source) || len(item.AdditionalEdits) != 1 {
		return "", false
	}
	edit := item.AdditionalEdits[0]
	if edit.Range != (languageservice.OffsetRange{}) || !strings.HasPrefix(edit.NewText, "import ") {
		return "", false
	}
	imported := strings.TrimSuffix(edit.NewText, "\n")
	if imported == edit.NewText || strings.Contains(imported, "\n") {
		return "", false
	}
	return imported, true
}
