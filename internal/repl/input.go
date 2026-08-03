package repl

import (
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

// Complete reports whether source is a complete REPL submission. It uses the
// lossless lexer so keywords inside strings and comments do not affect nesting.
func Complete(source string) bool {
	tokens, diagnostics := lexer.Lex([]byte(source))
	if len(diagnostics) > 0 {
		return true
	}
	blocks := 0
	delimiters := []string{}
	lineStart := true
	lineOpenedBlock := false
	for _, item := range tokens {
		switch item.Kind {
		case token.Comment:
			continue
		case token.Newline:
			lineStart = true
			lineOpenedBlock = false
			continue
		case token.EOF:
			continue
		}
		if lineStart && item.Kind == token.Identifier {
			switch item.Lexeme {
			case "class", "record", "module", "interface", "def", "if", "while":
				blocks++
				lineOpenedBlock = true
			case "end":
				if blocks > 0 {
					blocks--
				}
			}
		}
		if item.Lexeme == "do" && len(delimiters) == 0 && !lineOpenedBlock {
			blocks++
			lineOpenedBlock = true
		}
		lineStart = false
		switch item.Lexeme {
		case "(", "[", "{":
			delimiters = append(delimiters, item.Lexeme)
		case ")", "]", "}":
			if len(delimiters) > 0 && matching(delimiters[len(delimiters)-1], item.Lexeme) {
				delimiters = delimiters[:len(delimiters)-1]
			}
		}
	}
	return blocks == 0 && len(delimiters) == 0 && !strings.HasSuffix(strings.TrimSpace(source), "\\")
}

func matching(open, close string) bool {
	return open == "(" && close == ")" || open == "[" && close == "]" || open == "{" && close == "}"
}
