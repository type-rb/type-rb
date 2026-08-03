// Package formatter implements a deterministic, comment-preserving TypeRB
// printer. Parsing happens first; the printer then uses the lossless token
// stream so comments, strings, percent literals, and heredocs are never lost.
package formatter

import (
	"strings"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

func Format(source []byte) ([]byte, []diagnostic.Diagnostic) {
	program, diagnostics := parser.Parse(source)
	if hasErrors(diagnostics) {
		return nil, diagnostics
	}
	lines := tokensByLine(program.Tokens)
	var out strings.Builder
	indent := 0
	continuation := 0
	blank := false
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		line := lines[lineIndex]
		code := withoutNewline(line)
		if len(code) == 0 {
			if !blank && out.Len() > 0 {
				out.WriteByte('\n')
			}
			blank = true
			continue
		}
		blank = false
		first := firstCode(code)
		dedent := isDedent(first)
		if dedent && indent > 0 {
			indent--
		}
		lineContinuation := continuation
		if (first == "}" || first == ")" || first == "]") && lineContinuation > 0 {
			lineContinuation--
		}
		lineIndent := indent + lineContinuation
		if lineIndent < 0 {
			lineIndent = 0
		}
		out.WriteString(strings.Repeat("  ", lineIndent))
		out.WriteString(formatTokens(code))
		out.WriteByte('\n')

		if dedent && isMidBlock(first) {
			indent++
		} else if opensEndBlock(code) {
			indent++
		}
		continuation += delimiterDelta(code)
		if continuation < 0 {
			continuation = 0
		}
		for _, item := range code {
			if covered := item.Span.End.Line - 1; covered > lineIndex {
				lineIndex = covered
			}
		}
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), nil
}

func tokensByLine(tokens []token.Token) [][]token.Token {
	maxLine := 1
	for _, item := range tokens {
		if item.Kind != token.EOF && item.Span.Start.Line > maxLine {
			maxLine = item.Span.Start.Line
		}
	}
	lines := make([][]token.Token, maxLine)
	for _, item := range tokens {
		if item.Kind == token.EOF {
			continue
		}
		line := item.Span.Start.Line - 1
		lines[line] = append(lines[line], item)
	}
	return lines
}

func withoutNewline(tokens []token.Token) []token.Token {
	result := tokens[:0]
	for _, item := range tokens {
		if item.Kind != token.Newline {
			result = append(result, item)
		}
	}
	return result
}

func firstCode(tokens []token.Token) string {
	for _, item := range tokens {
		if item.Kind != token.Comment {
			return item.Lexeme
		}
	}
	return ""
}

func formatTokens(tokens []token.Token) string {
	var out strings.Builder
	var previous *token.Token
	var beforePrevious *token.Token
	inBlockParameters := false
	lineKind := firstCode(tokens)
	importLine := lineKind == "import"
	for i := range tokens {
		current := tokens[i]
		if current.Kind == token.Comment {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(current.Lexeme)
			beforePrevious = previous
			previous = &current
			continue
		}
		var next *token.Token
		for j := i + 1; j < len(tokens); j++ {
			if tokens[j].Kind != token.Comment {
				t := tokens[j]
				next = &t
				break
			}
		}
		if previous != nil {
			openingPipe := current.Lexeme == "|" && (previous.Lexeme == "do" || previous.Lexeme == "{")
			closingPipe := current.Lexeme == "|" && inBlockParameters && !openingPipe
			space := needsSpace(beforePrevious, *previous, current, next)
			if lineKind == "class" && current.Lexeme == "<" {
				space = true
			}
			if lineKind == "class" && previous.Lexeme == "<" {
				space = true
			}
			if importLine && (previous.Lexeme == "/" || current.Lexeme == "/") {
				space = false
			}
			if inBlockParameters && previous.Lexeme == "|" {
				space = false
			}
			if closingPipe {
				space = false
			}
			if space {
				out.WriteByte(' ')
			}
			if openingPipe {
				inBlockParameters = true
			} else if closingPipe {
				inBlockParameters = false
			}
		}
		out.WriteString(current.Lexeme)
		beforePrevious = previous
		previous = &current
	}
	return strings.TrimSpace(out.String())
}

func needsSpace(beforePrevious *token.Token, previous, current token.Token, next *token.Token) bool {
	if previous.Kind == token.Comment {
		return false
	}
	if previous.Lexeme == "!" || previous.Lexeme == "~" {
		return false
	}
	if current.Lexeme == ":" {
		return current.Span.Start.Offset > previous.Span.End.Offset
	}
	if current.Lexeme == ".." || current.Lexeme == "..." || previous.Lexeme == ".." || previous.Lexeme == "..." {
		return false
	}
	if previous.Lexeme == "-" && current.Lexeme == ">" {
		return false
	}
	if current.Lexeme == "[" {
		switch previous.Lexeme {
		case ":", ":=", "=", ",", "return":
			return true
		default:
			return false
		}
	}
	if current.Lexeme == "<" && startsUpper(previous.Lexeme) {
		return false
	}
	if previous.Lexeme == "<" && beforePrevious != nil && startsUpper(beforePrevious.Lexeme) {
		return false
	}
	if current.Lexeme == ">" && beforePrevious != nil && beforePrevious.Lexeme == "<" {
		return false
	}
	if current.Lexeme == "}" {
		return previous.Lexeme != "{"
	}
	if current.Lexeme == ")" || current.Lexeme == "]" || current.Lexeme == "," || current.Lexeme == ";" || current.Lexeme == "." || current.Lexeme == "&." || current.Lexeme == "::" {
		return false
	}
	if previous.Lexeme == "::" {
		return beforePrevious != nil && startsLower(beforePrevious.Lexeme)
	}
	if previous.Lexeme == "(" || previous.Lexeme == "[" || previous.Lexeme == "." || previous.Lexeme == "&." {
		return false
	}
	if previous.Lexeme == "," || previous.Lexeme == ";" {
		return true
	}
	if previous.Lexeme == ":" {
		// :symbol has no space; type annotations, mode declarations, keyword
		// arguments, and hash labels do.
		return !isSymbolColon(beforePrevious, previous, current.Lexeme)
	}
	if current.Lexeme == "(" {
		return previous.Lexeme == "if" || previous.Lexeme == "while" || previous.Lexeme == "unless" || previous.Lexeme == "until"
	}
	if current.Lexeme == "{" {
		return previous.Lexeme != "=" && previous.Lexeme != ":=" && previous.Lexeme != "(" && previous.Lexeme != "["
	}
	if previous.Lexeme == "{" {
		return true
	}
	if previous.Lexeme == "|" || current.Lexeme == "|" {
		if previous.Span.End.Offset == current.Span.Start.Offset {
			return false
		}
		return true
	}
	if isUnary(previous.Lexeme, current.Lexeme) {
		return false
	}
	if isOperator(previous.Lexeme) || isOperator(current.Lexeme) {
		return true
	}
	if next != nil && current.Lexeme == "*" && (next.Kind == token.Identifier) {
		return false
	}
	return true
}

func startsUpper(s string) bool {
	return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z'
}

func startsLower(s string) bool {
	return len(s) > 0 && ((s[0] >= 'a' && s[0] <= 'z') || s[0] == '_')
}

func isSymbolColon(before *token.Token, colon token.Token, next string) bool {
	if next == "" {
		return false
	}
	if before == nil {
		return true
	}
	if colon.Span.Start.Offset > before.Span.End.Offset {
		return true
	}
	switch before.Lexeme {
	case "(", "[", "{", ",", "=", ":=", "=>", "return":
		return true
	}
	return false
}

func isOperator(s string) bool {
	switch s {
	case ":=", "=", "+", "-", "*", "/", "%", "**", "==", "!=", "<", ">", "<=", ">=", "<=>", "=~", "!~", "&&", "||", "and", "or", "=>", "+=", "-=", "*=", "/=", "||=", "&&=", "|", "&", "^", "..", "...":
		return true
	}
	return false
}

func isUnary(previous, current string) bool {
	if current != "!" && current != "~" && current != "+" && current != "-" {
		return false
	}
	return previous == "(" || previous == "[" || previous == "{" || previous == "," || previous == "=" || previous == ":="
}

func isDedent(first string) bool {
	switch first {
	case "end", "else", "elsif", "when", "rescue", "ensure":
		return true
	}
	return false
}

func isMidBlock(first string) bool {
	switch first {
	case "else", "elsif", "when", "rescue", "ensure":
		return true
	}
	return false
}

func opensEndBlock(tokens []token.Token) bool {
	first := firstCode(tokens)
	switch first {
	case "class", "record", "module", "interface", "def", "if", "unless", "case", "begin", "while", "until", "for":
		return true
	}
	depth := 0
	for _, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case "do":
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func delimiterDelta(tokens []token.Token) int {
	delta := 0
	for _, item := range tokens {
		if strings.Contains(item.Lexeme, "\n") {
			continue
		}
		switch item.Lexeme {
		case "(", "[", "{":
			delta++
		case ")", "]", "}":
			delta--
		}
	}
	return delta
}

func hasErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}
