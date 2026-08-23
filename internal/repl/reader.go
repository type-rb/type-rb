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
		if _, err := terminal.History.Current().Write(entry); err != nil {
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
				formatCompleteInput(terminal)
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

func formatCompleteInput(terminal *readline.Shell) {
	formatted, diagnostics := formatter.Format([]byte(string(*terminal.Line())))
	if len(diagnostics) > 0 {
		return
	}
	line := []rune(strings.TrimSuffix(string(formatted), "\n"))
	terminal.Line().Set(line...)
	terminal.Cursor().Set(len(line))
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
		values = append(values, readline.Completion{
			Value:       value,
			Display:     item.Label,
			Description: item.Detail,
			Tag:         string(item.Kind),
		})
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
