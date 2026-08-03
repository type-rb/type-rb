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

	"github.com/reeflective/readline"
	"github.com/reeflective/readline/inputrc"
)

var (
	errInputInterrupted = errors.New("REPL input interrupted")
	errIncompleteInput  = errors.New("incomplete REPL input")
)

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
	terminal.SyntaxHighlighter = func(line []rune) string {
		return colorInput + string(line) + colorReset
	}
	terminal.Completer = completeInput
	if err := terminal.Config.Set("enable-bracketed-paste", true); err != nil {
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

type completionItem struct {
	Text        string
	Description string
}

var completionItems = []completionItem{
	{Text: ":help", Description: "show REPL commands"},
	{Text: ":type", Description: "show an expression's checked type"},
	{Text: ":load", Description: "load a .trb file"},
	{Text: ":reload", Description: "reload the project"},
	{Text: ":quit", Description: "leave the REPL"},
	{Text: "class", Description: "declare a reference type"},
	{Text: "record", Description: "declare a data value"},
	{Text: "enum", Description: "declare a closed nominal type"},
	{Text: "module", Description: "group declarations"},
	{Text: "interface", Description: "declare required methods"},
	{Text: "def", Description: "declare a function or method"},
	{Text: "if", Description: "start a conditional"},
	{Text: "case", Description: "dispatch on an enum"},
	{Text: "when", Description: "handle an enum member"},
	{Text: "elsif", Description: "add a conditional branch"},
	{Text: "else", Description: "add a fallback branch"},
	{Text: "while", Description: "start a loop"},
	{Text: "return", Description: "return from a function"},
	{Text: "end", Description: "close a block"},
	{Text: "import", Description: "import a package"},
	{Text: "true", Description: "Boolean literal"},
	{Text: "false", Description: "Boolean literal"},
	{Text: "nil", Description: "empty value"},
	{Text: "puts", Description: "write a value to standard output"},
}

func completionSuggestions(input string) []completionItem {
	word := input
	if separator := strings.LastIndexAny(word, " \t\r\n([{,"); separator >= 0 {
		word = word[separator+1:]
	}
	if word == "" {
		return nil
	}
	var suggestions []completionItem
	for _, item := range completionItems {
		if strings.HasPrefix(item.Text, word) {
			suggestions = append(suggestions, item)
		}
	}
	return suggestions
}

func completeInput(line []rune, cursor int) readline.Completions {
	if cursor < 0 || cursor > len(line) {
		return readline.Completions{}
	}
	suggestions := completionSuggestions(string(line[:cursor]))
	values := make([]string, 0, len(suggestions)*2)
	for _, item := range suggestions {
		values = append(values, item.Text, item.Description)
	}
	return readline.CompleteValuesDescribed(values...).NoSpace()
}
