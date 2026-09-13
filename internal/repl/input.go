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
	transferConditions := make(map[int]bool)
	lineStart := true
	lineOpenedBlock := false
	for index, item := range tokens {
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
		if at := conditionalTransferIf(tokens, index); at >= 0 {
			transferConditions[at] = true
		}
		if item.Lexeme == ";" && len(delimiters) == 0 {
			lineStart = true
			lineOpenedBlock = false
			continue
		}
		if lineStart && item.Kind == token.Identifier {
			switch item.Lexeme {
			case "class", "record", "enum", "module", "interface", "def", "while":
				blocks++
				lineOpenedBlock = true
			case "end":
				if blocks > 0 {
					blocks--
				}
			}
		}
		if item.Kind == token.Identifier && (item.Lexeme == "case" || item.Lexeme == "if" && !transferConditions[index]) {
			blocks++
			lineOpenedBlock = true
		}
		if item.Kind == token.Identifier && (item.Lexeme == "fn" || item.Lexeme == "catch") && len(delimiters) == 0 {
			blocks++
			lineOpenedBlock = true
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

// A transfer's trailing condition does not open an end-delimited block. Only
// the first ungrouped if on that logical statement is the modifier; grouped
// value-producing if expressions still contribute their own block depth.
func conditionalTransferIf(tokens []token.Token, start int) int {
	item := tokens[start]
	if item.Kind != token.Identifier || item.Lexeme != "return" && item.Lexeme != "break" && item.Lexeme != "next" {
		return -1
	}
	if start > 0 && (tokens[start-1].Lexeme == "." || tokens[start-1].Lexeme == "::") {
		return -1
	}
	depth := 0
	for at := start + 1; at < len(tokens); at++ {
		next := tokens[at]
		if next.Kind == token.EOF || depth == 0 && (next.Kind == token.Newline || next.Kind == token.Comment || next.Lexeme == ";") {
			break
		}
		if depth == 0 && next.Kind == token.Identifier && next.Lexeme == "if" {
			return at
		}
		// break and next have no value form. Do not mistake a contextual
		// name in an initializer or call for a conditional transfer.
		if item.Lexeme != "return" {
			break
		}
		switch next.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth == 0 {
				return -1
			}
			depth--
		}
	}
	return -1
}

func matching(open, close string) bool {
	return open == "(" && close == ")" || open == "[" && close == "]" || open == "{" && close == "}"
}
