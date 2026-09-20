package repl

import (
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

// Complete reports whether source is a complete REPL submission. It uses the
// lossless lexer so keywords inside strings and comments do not affect nesting.
func Complete(source string) bool {
	tokens, diagnostics := lexer.Lex([]byte(source))
	if len(diagnostics) > 0 {
		return true
	}
	var symbolNames map[int]bool
	for index, item := range tokens {
		if item.Kind == token.Identifier && (index > 0 && tokens[index-1].Lexeme == ":" || index+1 < len(tokens) && tokens[index+1].Lexeme == ":") {
			switch item.Lexeme {
			case "class", "record", "enum", "module", "interface", "def", "while", "if", "case", "fn", "catch", "do", "return", "break", "next", "end":
				symbolNames = parser.SymbolNameOffsets([]byte(source))
			}
		}
		if symbolNames != nil {
			break
		}
	}
	blockDepths := []int{}
	delimiters := []string{}
	transferConditions := make(map[int]bool)
	blockBodies := make(map[int]bool)
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
		if symbolNames[item.Span.Start.Offset] {
			lineStart = false
			continue
		}
		if at := conditionalTransferIf(tokens, index, symbolNames); at >= 0 {
			transferConditions[at] = true
		}
		if item.Lexeme == "{" || item.Lexeme == "do" {
			if at := braceParameterEnd(tokens, index+1); at >= 0 {
				blockBodies[at] = true
			}
		}
		statementContext := len(delimiters) == 0 || delimiters[len(delimiters)-1] == "{" ||
			len(blockDepths) > 0 && blockDepths[len(blockDepths)-1] == len(delimiters)
		if item.Lexeme == ";" && statementContext || blockBodies[index] {
			lineStart = true
			lineOpenedBlock = false
			continue
		}
		if lineStart && item.Kind == token.Identifier {
			switch item.Lexeme {
			case "class", "record", "enum", "module", "interface", "def", "while":
				blockDepths = append(blockDepths, len(delimiters))
				lineOpenedBlock = true
			case "end":
				if len(blockDepths) > 0 {
					blockDepths = blockDepths[:len(blockDepths)-1]
				}
			}
		}
		if item.Kind == token.Identifier && (item.Lexeme == "case" || item.Lexeme == "if" && !transferConditions[index]) {
			blockDepths = append(blockDepths, len(delimiters))
			lineOpenedBlock = true
		}
		if item.Kind == token.Identifier && (item.Lexeme == "fn" || item.Lexeme == "catch") && statementContext {
			blockDepths = append(blockDepths, len(delimiters))
			lineOpenedBlock = true
		}
		if item.Lexeme == "do" && (statementContext && !lineOpenedBlock || braceParameterEnd(tokens, index+1) >= 0) {
			blockDepths = append(blockDepths, len(delimiters))
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
	return len(blockDepths) == 0 && len(delimiters) == 0 && !strings.HasSuffix(strings.TrimSpace(source), "\\")
}

func braceParameterEnd(tokens []token.Token, start int) int {
	for start < len(tokens) && (tokens[start].Kind == token.Newline || tokens[start].Kind == token.Comment) {
		start++
	}
	if start >= len(tokens) || tokens[start].Lexeme != "|" {
		return -1
	}
	for at := start + 1; at < len(tokens); at++ {
		item := tokens[at]
		if item.Lexeme == "|" {
			return at
		}
		if item.Kind != token.Identifier && item.Kind != token.Newline && item.Kind != token.Comment && item.Lexeme != "," {
			return -1
		}
	}
	return -1
}

// A transfer's trailing condition does not open an end-delimited block. Only
// the first ungrouped if on that logical statement is the modifier; grouped
// value-producing if expressions still contribute their own block depth.
func conditionalTransferIf(tokens []token.Token, start int, symbolNames map[int]bool) int {
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
			if symbolNames[next.Span.Start.Offset] {
				continue
			}
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
