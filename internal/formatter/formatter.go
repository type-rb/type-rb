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

const indentation = "\t"

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
		coveredLine := lineIndex
		coveredOffset := -1
		for _, item := range code {
			if endLine := item.Span.End.Line - 1; endLine > coveredLine || endLine == coveredLine && item.Span.End.Offset > coveredOffset {
				coveredLine = endLine
				coveredOffset = item.Span.End.Offset
			}
		}
		if coveredLine > lineIndex && coveredLine < len(lines) {
			for _, item := range withoutNewline(lines[coveredLine]) {
				if item.Span.Start.Offset >= coveredOffset {
					code = append(code, item)
				}
			}
		}
		statements := splitStatements(code, continuation)
		if len(statements) == 0 {
			if !blank && out.Len() > 0 {
				out.WriteByte('\n')
			}
			blank = true
			continue
		}
		blank = false
		for _, statement := range statements {
			writeStatement(&out, statement, &indent, &continuation)
		}
		if coveredLine > lineIndex {
			lineIndex = coveredLine
		}
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n"), nil
}

// splitStatements treats a top-level semicolon as a physical newline for
// canonical formatting. Semicolons inside (), [], or {} remain part of the
// expression; this preserves the compact brace form of iterator blocks.
func splitStatements(tokens []token.Token, initialDepth int) [][]token.Token {
	depth := initialDepth
	start := 0
	statements := [][]token.Token{}
	appendStatement := func(end int) {
		if end > start {
			statements = append(statements, tokens[start:end])
		}
	}
	for index, item := range tokens {
		if item.Lexeme == ";" && depth == 0 {
			appendStatement(index)
			start = index + 1
			continue
		}
		if strings.Contains(item.Lexeme, "\n") {
			continue
		}
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		}
	}
	appendStatement(len(tokens))
	return statements
}

func writeStatement(out *strings.Builder, code []token.Token, indent, continuation *int) {
	first := firstCode(code)
	dedent := isDedent(first)
	if dedent && *indent > 0 {
		*indent--
	}
	lineContinuation := *continuation
	if (first == "}" || first == ")" || first == "]") && lineContinuation > 0 {
		lineContinuation--
	}
	lineIndent := *indent + lineContinuation
	if lineIndent < 0 {
		lineIndent = 0
	}
	out.WriteString(strings.Repeat(indentation, lineIndent))
	out.WriteString(formatTokens(code))
	out.WriteByte('\n')

	if dedent && isMidBlock(first) {
		*indent++
	} else if opensEndBlock(code, *continuation) {
		*indent++
	}
	*continuation += delimiterDelta(code)
	if *continuation < 0 {
		*continuation = 0
	}
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
	importLine := lineKind == "import" || importFromLine(tokens)
	genericDepth := 0
	classInheritance := false
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
			genericOpen := current.Lexeme == "<" && (startsUpper(previous.Lexeme) || genericApplicationOpen(tokens, i))
			if genericOpen && lineKind == "class" && genericDepth == 0 && !classInheritance {
				if !classTypeParameterOpen(tokens, i) {
					genericOpen = false
					classInheritance = true
				}
			}
			genericClosers := 0
			if current.Lexeme == ">" && genericDepth > 0 {
				genericClosers = 1
			} else if current.Lexeme == ">>" && genericDepth >= 2 {
				genericClosers = 2
			}
			openingPipe := current.Lexeme == "|" && (previous.Lexeme == "do" || previous.Lexeme == "{" || previous.Lexeme == "catch")
			closingPipe := current.Lexeme == "|" && inBlockParameters && !openingPipe
			space := needsSpace(beforePrevious, *previous, current, next)
			if (current.Lexeme == "|" || previous.Lexeme == "|") && !openingPipe && !closingPipe && !inBlockParameters {
				space = true
			}
			if lineKind == "class" && current.Lexeme == "<" && !genericOpen {
				space = true
			}
			if lineKind == "class" && previous.Lexeme == "<" && genericDepth == 0 {
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
			if genericOpen || genericClosers > 0 || (genericDepth > 0 && previous.Lexeme == "<") {
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
			if genericOpen {
				genericDepth++
			}
			genericDepth -= genericClosers
		}
		out.WriteString(current.Lexeme)
		beforePrevious = previous
		previous = &current
	}
	return strings.TrimSpace(out.String())
}

func importFromLine(tokens []token.Token) bool {
	from := -1
	for index, item := range tokens {
		if item.Kind != token.Comment && item.Lexeme == "from" {
			from = index
			break
		}
	}
	if from < 0 {
		return false
	}
	for _, item := range tokens[from+1:] {
		if item.Kind != token.Comment && item.Lexeme == "/" {
			return true
		}
	}
	return false
}

func classTypeParameterOpen(tokens []token.Token, open int) bool {
	if open != 2 {
		return false
	}
	close := matchingTokenIndex(tokens, open, "<", ">")
	if close < 0 {
		return false
	}
	for index := open + 1; index < close; index++ {
		if tokens[index].Kind == token.Comment {
			continue
		}
		if tokens[index].Kind != token.Identifier && tokens[index].Lexeme != "," {
			return false
		}
	}
	return true
}

func matchingTokenIndex(tokens []token.Token, open int, opening, closing string) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Lexeme {
		case opening:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func genericApplicationOpen(tokens []token.Token, open int) bool {
	depth := 0
	for index := open; index < len(tokens); index++ {
		if tokens[index].Kind == token.Comment {
			continue
		}
		switch tokens[index].Lexeme {
		case "<":
			depth++
		case ">":
			depth--
		case ">>":
			depth -= 2
		}
		if depth != 0 {
			continue
		}
		for next := index + 1; next < len(tokens); next++ {
			if tokens[next].Kind == token.Comment {
				continue
			}
			return tokens[next].Lexeme == "(" || tokens[next].Lexeme == "." || tokens[next].Lexeme == "::"
		}
		return false
	}
	return false
}

func needsSpace(beforePrevious *token.Token, previous, current token.Token, next *token.Token) bool {
	if previous.Kind == token.Comment {
		return false
	}
	if previous.Lexeme == "!" || previous.Lexeme == "~" {
		return false
	}
	if current.Lexeme == "?" {
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
		if (previous.Lexeme == ">" || previous.Lexeme == ">>") && beforePrevious != nil && startsUpper(beforePrevious.Lexeme) {
			return false
		}
		return previous.Lexeme == "if" || previous.Lexeme == "while" || previous.Lexeme == "unless" || previous.Lexeme == "until" || previous.Lexeme == "return" || previous.Lexeme == "," || isOperator(previous.Lexeme)
	}
	if current.Lexeme == "{" {
		return previous.Lexeme != "(" && previous.Lexeme != "["
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

func opensEndBlock(tokens []token.Token, initialDepth int) bool {
	first := firstCode(tokens)
	switch first {
	case "class", "record", "enum", "module", "interface", "def", "if", "unless", "case", "begin", "while", "until", "for":
		return true
	}
	for _, item := range tokens {
		if item.Lexeme == "case" || item.Lexeme == "if" {
			return true
		}
	}
	depth := initialDepth
	for index, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case "do":
			if depth == 0 {
				return true
			}
		case "fn":
			if depth == 0 {
				return true
			}
		case "catch":
			if depth == 0 {
				for next := index + 1; next < len(tokens); next++ {
					if tokens[next].Kind == token.Comment {
						continue
					}
					if tokens[next].Lexeme == "|" {
						return true
					}
					break
				}
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
