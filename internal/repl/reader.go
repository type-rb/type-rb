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

	"github.com/nao1215/prompt"
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
	terminal, err := newTerminalPrompt(options)
	if err != nil {
		return nil, err
	}
	history, err := loadHistory(options.HistoryFile)
	if err != nil {
		_ = terminal.Close()
		return nil, err
	}
	terminal.SetHistory(slices.DeleteFunc(history, isQuitCommand))
	return &terminalSubmissionReader{prompt: terminal, options: options}, nil
}

func newTerminalPrompt(options Options) (*prompt.Prompt, error) {
	return prompt.New(
		"trb:"+options.Mode+"> ",
		prompt.WithMultiline(true),
		prompt.WithIsComplete(Complete),
		prompt.WithContinuationPrefix("trb:"+options.Mode+"*  "),
		prompt.WithMemoryHistory(1000),
		prompt.WithCompleter(completeInput),
		prompt.WithColorScheme(prompt.ThemeNightOwl),
		prompt.WithKeyMap(typeRBKeyMap()),
	)
}

func typeRBKeyMap() *prompt.KeyMap {
	keyMap := prompt.NewDefaultKeyMap()
	keyMap.Bind('\x02', prompt.ActionMoveLeft)           // Ctrl-B
	keyMap.Bind('\x06', prompt.ActionMoveRight)          // Ctrl-F
	keyMap.Bind('\x10', prompt.ActionMoveUp)             // Ctrl-P
	keyMap.Bind('\x0e', prompt.ActionMoveDown)           // Ctrl-N
	keyMap.BindSequence("b", prompt.ActionMoveWordLeft)  // Alt-B
	keyMap.BindSequence("f", prompt.ActionMoveWordRight) // Alt-F
	return keyMap
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
	prompt  *prompt.Prompt
	options Options
}

func (r *terminalSubmissionReader) Read() (string, error) {
	input, err := r.prompt.Run()
	if err == nil && isQuitCommand(input) {
		r.prompt.SetHistory(slices.DeleteFunc(r.prompt.GetHistory(), isQuitCommand))
	}
	// prompt keeps its previous rendered-line count between Run calls. After a
	// multiline submit, its next render therefore clears the continuation lines
	// that should remain in terminal scrollback. Recreate only the terminal
	// renderer while retaining history; the compiled REPL session is unaffected.
	if err == nil && strings.Contains(input, "\n") {
		if err := r.resetPrompt(); err != nil {
			return "", err
		}
	}
	switch {
	case errors.Is(err, prompt.ErrEOF), errors.Is(err, io.EOF):
		return "", io.EOF
	case errors.Is(err, prompt.ErrInterrupted):
		if resetErr := r.resetPrompt(); resetErr != nil {
			return "", resetErr
		}
		return "", errInputInterrupted
	default:
		return input, err
	}
}

func (r *terminalSubmissionReader) resetPrompt() error {
	history := slices.DeleteFunc(r.prompt.GetHistory(), isQuitCommand)
	if err := r.prompt.Close(); err != nil {
		return err
	}
	terminal, err := newTerminalPrompt(r.options)
	if err != nil {
		return err
	}
	terminal.SetHistory(history)
	r.prompt = terminal
	return nil
}

func (r *terminalSubmissionReader) Close() error {
	history := slices.DeleteFunc(r.prompt.GetHistory(), isQuitCommand)
	return errors.Join(saveHistory(r.options.HistoryFile, history), r.prompt.Close())
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

var completionItems = []prompt.Suggestion{
	{Text: ":help", Description: "show REPL commands"},
	{Text: ":type", Description: "show an expression's checked type"},
	{Text: ":load", Description: "load a .trb file"},
	{Text: ":reload", Description: "reload the project"},
	{Text: ":quit", Description: "leave the REPL"},
	{Text: "class", Description: "declare a reference type"},
	{Text: "record", Description: "declare a data value"},
	{Text: "module", Description: "group declarations"},
	{Text: "interface", Description: "declare required methods"},
	{Text: "def", Description: "declare a function or method"},
	{Text: "if", Description: "start a conditional"},
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

func completeInput(document prompt.Document) []prompt.Suggestion {
	word := document.GetWordBeforeCursor()
	if word == "" {
		return nil
	}
	var suggestions []prompt.Suggestion
	for _, item := range completionItems {
		if strings.HasPrefix(item.Text, word) {
			suggestions = append(suggestions, item)
		}
	}
	return suggestions
}
