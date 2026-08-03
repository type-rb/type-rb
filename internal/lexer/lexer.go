// Package lexer provides the lossless lexical layer used by both the compiler
// and formatter. Comments and newlines are first-class tokens.
package lexer

import (
	"bytes"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/token"
)

type Lexer struct {
	source []byte
	offset int
	line   int
	column int
	tokens []token.Token
	diags  []diagnostic.Diagnostic
}

func Lex(source []byte) ([]token.Token, []diagnostic.Diagnostic) {
	l := &Lexer{source: source, line: 1, column: 1}
	l.run()
	return l.tokens, l.diags
}

func (l *Lexer) run() {
	for l.offset < len(l.source) {
		start := l.position()
		b := l.source[l.offset]
		switch {
		case b == ' ' || b == '\t' || b == '\r':
			l.advance()
		case b == '\n':
			l.advance()
			l.emit(token.Newline, "\n", start)
		case b == '#':
			l.scanComment(start)
		case b == '<' && l.offset+1 < len(l.source) && l.source[l.offset+1] == '<' && l.scanHeredoc(start):
			// scanHeredoc emitted the token.
		case b == '%' && l.scanPercentLiteral(start):
			// scanPercentLiteral emitted the token.
		case b == '/' && l.canStartRegex():
			l.scanRegex(start)
		case b == '\'' || b == '"' || b == '`':
			l.scanString(start, b)
		case isDigit(b):
			l.scanNumber(start)
		case isIdentifierStart(l.peekRune()):
			l.scanIdentifier(start)
		case isPunctuation(b):
			l.advance()
			l.emit(token.Punct, string(b), start)
		case isOperatorStart(b):
			l.scanOperator(start)
		default:
			l.advance()
			// Ruby has deliberately broad punctuation (globals, regexps, ternary
			// operators). Preserve unknown punctuation for a Native AST node; the
			// parser/checker decides whether it is valid for the selected mode.
			l.emit(token.Punct, string(b), start)
		}
	}
	pos := l.position()
	l.tokens = append(l.tokens, token.Token{Kind: token.EOF, Span: token.Span{Start: pos, End: pos}})
}

func (l *Lexer) scanComment(start token.Position) {
	begin := l.offset
	for l.offset < len(l.source) && l.source[l.offset] != '\n' {
		l.advance()
	}
	l.emit(token.Comment, string(l.source[begin:l.offset]), start)
}

func (l *Lexer) scanString(start token.Position, quote byte) {
	begin := l.offset
	kind := token.String
	if quote == '`' {
		kind = token.NativeLiteral
	}
	l.advance()
	interpolationDepth := 0
	innerQuote := byte(0)
	for l.offset < len(l.source) {
		if l.source[l.offset] == '\\' {
			l.advance()
			if l.offset < len(l.source) {
				l.advance()
			}
			continue
		}
		if quote == '"' && l.offset+1 < len(l.source) && l.source[l.offset] == '#' && l.source[l.offset+1] == '{' && innerQuote == 0 {
			interpolationDepth++
			l.advance()
			l.advance()
			continue
		}
		if interpolationDepth > 0 {
			b := l.source[l.offset]
			if innerQuote != 0 {
				if b == innerQuote {
					innerQuote = 0
				}
				l.advance()
				continue
			}
			if b == '\'' || b == '"' {
				innerQuote = b
				l.advance()
				continue
			}
			if b == '{' {
				interpolationDepth++
			} else if b == '}' {
				interpolationDepth--
			}
			l.advance()
			continue
		}
		if l.source[l.offset] == quote {
			l.advance()
			l.emit(kind, string(l.source[begin:l.offset]), start)
			return
		}
		l.advance()
	}
	l.emit(kind, string(l.source[begin:l.offset]), start)
	l.diags = append(l.diags, diagnostic.Diagnostic{
		Severity: diagnostic.Error,
		Message:  "unterminated string literal",
		Span:     token.Span{Start: start, End: l.position()},
	})
}

// scanHeredoc recognizes the common Ruby forms <<SQL, <<~SQL, <<-'SQL' and
// returns false when the operator is not followed by a delimiter.
func (l *Lexer) scanHeredoc(start token.Position) bool {
	if l.offset+2 >= len(l.source) || l.source[l.offset+2] == ' ' || l.source[l.offset+2] == '\t' {
		return false
	}
	lineEndRel := bytes.IndexByte(l.source[l.offset:], '\n')
	if lineEndRel < 0 {
		return false
	}
	headerEnd := l.offset + lineEndRel
	header := string(l.source[l.offset:headerEnd])
	rest := strings.TrimSpace(strings.TrimPrefix(header, "<<"))
	rest = strings.TrimPrefix(rest, "~")
	rest = strings.TrimPrefix(rest, "-")
	if rest == "" {
		return false
	}
	marker := rest
	if (marker[0] == '\'' || marker[0] == '"' || marker[0] == '`') && len(marker) >= 2 {
		q := marker[0]
		if i := strings.IndexByte(marker[1:], q); i >= 0 {
			marker = marker[1 : i+1]
		}
	} else {
		for i, r := range marker {
			if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_') {
				marker = marker[:i]
				break
			}
		}
	}
	if marker == "" {
		return false
	}
	begin := l.offset
	for l.offset <= headerEnd {
		l.advance()
	}
	for l.offset < len(l.source) {
		endRel := bytes.IndexByte(l.source[l.offset:], '\n')
		lineEnd := len(l.source)
		if endRel >= 0 {
			lineEnd = l.offset + endRel
		}
		line := strings.TrimSpace(string(l.source[l.offset:lineEnd]))
		for l.offset < lineEnd {
			l.advance()
		}
		if line == marker {
			l.emit(token.NativeLiteral, string(l.source[begin:l.offset]), start)
			return true
		}
		if l.offset < len(l.source) {
			l.advance()
		}
	}
	l.emit(token.NativeLiteral, string(l.source[begin:l.offset]), start)
	l.diags = append(l.diags, diagnostic.Diagnostic{Severity: diagnostic.Error, Message: "unterminated heredoc", Span: token.Span{Start: start, End: l.position()}})
	return true
}

func (l *Lexer) scanPercentLiteral(start token.Position) bool {
	if l.offset+1 >= len(l.source) {
		return false
	}
	openerOffset := 2
	kind := l.source[l.offset+1]
	if !strings.ContainsRune("qQwWiIrxs", rune(kind)) {
		if !strings.ContainsRune("([{<", rune(kind)) {
			return false
		}
		openerOffset = 1
	}
	if l.offset+openerOffset >= len(l.source) {
		return false
	}
	opener := l.source[l.offset+openerOffset]
	closer := opener
	switch opener {
	case '(':
		closer = ')'
	case '[':
		closer = ']'
	case '{':
		closer = '}'
	case '<':
		closer = '>'
	}
	begin := l.offset
	for range openerOffset + 1 {
		l.advance()
	}
	depth := 1
	for l.offset < len(l.source) {
		b := l.source[l.offset]
		if b == '\\' {
			l.advance()
			if l.offset < len(l.source) {
				l.advance()
			}
			continue
		}
		if opener != closer && b == opener {
			depth++
		} else if b == closer {
			depth--
			l.advance()
			if depth == 0 {
				l.emit(token.NativeLiteral, string(l.source[begin:l.offset]), start)
				return true
			}
			continue
		}
		l.advance()
	}
	return false
}

func (l *Lexer) canStartRegex() bool {
	for i := len(l.tokens) - 1; i >= 0; i-- {
		previous := l.tokens[i]
		if previous.Kind == token.Newline {
			return true
		}
		if previous.Kind == token.Comment {
			continue
		}
		switch previous.Lexeme {
		case "(", "[", "{", ",", ":", "=", ":=", "=>", "return", "when", "if", "unless", "and", "or", "&&", "||", "!", "~":
			return true
		}
		return false
	}
	return true
}

func (l *Lexer) scanRegex(start token.Position) {
	begin := l.offset
	l.advance() // leading slash
	inClass := false
	for l.offset < len(l.source) {
		b := l.source[l.offset]
		if b == '\\' {
			l.advance()
			if l.offset < len(l.source) {
				l.advance()
			}
			continue
		}
		if b == '[' {
			inClass = true
		} else if b == ']' {
			inClass = false
		} else if b == '/' && !inClass {
			l.advance()
			for l.offset < len(l.source) && strings.ContainsRune("imxonesu", rune(l.source[l.offset])) {
				l.advance()
			}
			l.emit(token.NativeLiteral, string(l.source[begin:l.offset]), start)
			return
		}
		if b == '\n' {
			break
		}
		l.advance()
	}
	l.emit(token.NativeLiteral, string(l.source[begin:l.offset]), start)
	l.diags = append(l.diags, diagnostic.Diagnostic{Severity: diagnostic.Error, Message: "unterminated regular expression", Span: token.Span{Start: start, End: l.position()}})
}

func (l *Lexer) scanNumber(start token.Position) {
	begin := l.offset
	for l.offset < len(l.source) && (isDigit(l.source[l.offset]) || l.source[l.offset] == '_') {
		l.advance()
	}
	if l.offset < len(l.source)-1 && l.source[l.offset] == '.' && isDigit(l.source[l.offset+1]) {
		l.advance()
		for l.offset < len(l.source) && (isDigit(l.source[l.offset]) || l.source[l.offset] == '_') {
			l.advance()
		}
	}
	l.emit(token.Number, string(l.source[begin:l.offset]), start)
}

func (l *Lexer) scanIdentifier(start token.Position) {
	begin := l.offset
	first := l.peekRune()
	for l.offset < len(l.source) {
		r := l.peekRune()
		if !isIdentifierPart(r) {
			break
		}
		l.advanceRune(r)
	}
	// Ruby predicate/bang method names are kept in one token.
	if l.offset < len(l.source) && (l.source[l.offset] == '?' || l.source[l.offset] == '!') && !unicode.IsUpper(first) {
		l.advance()
	}
	l.emit(token.Identifier, string(l.source[begin:l.offset]), start)
}

func (l *Lexer) scanOperator(start token.Position) {
	begin := l.offset
	operators := []string{"<=>", "...", "**=", "&&=", "||=", "&.", "::", ":=", "->", "==", "!=", "<=", ">=", "=>", "=~", "!~", "&&", "||", "**", "<<", ">>", "+=", "-=", "*=", "/=", "%="}
	for _, op := range operators {
		if bytes.HasPrefix(l.source[l.offset:], []byte(op)) {
			for range len(op) {
				l.advance()
			}
			l.emit(token.Operator, string(l.source[begin:l.offset]), start)
			return
		}
	}
	l.advance()
	l.emit(token.Operator, string(l.source[begin:l.offset]), start)
}

func (l *Lexer) emit(kind token.Kind, lexeme string, start token.Position) {
	l.tokens = append(l.tokens, token.Token{Kind: kind, Lexeme: lexeme, Span: token.Span{Start: start, End: l.position()}})
}

func (l *Lexer) position() token.Position {
	return token.Position{Offset: l.offset, Line: l.line, Column: l.column}
}

func (l *Lexer) advance() {
	if l.offset >= len(l.source) {
		return
	}
	if l.source[l.offset] == '\n' {
		l.line++
		l.column = 1
	} else {
		l.column++
	}
	l.offset++
}

func (l *Lexer) advanceRune(r rune) {
	width := utf8.RuneLen(r)
	if width < 1 {
		width = 1
	}
	l.offset += width
	l.column++
}

func (l *Lexer) peekRune() rune {
	r, _ := utf8.DecodeRune(l.source[l.offset:])
	return r
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }
func isIdentifierStart(r rune) bool {
	return r == '_' || r == '@' || unicode.IsLetter(r)
}
func isIdentifierPart(r rune) bool {
	return r == '_' || r == '@' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
func isPunctuation(b byte) bool {
	return b == '(' || b == ')' || b == '[' || b == ']' || b == '{' || b == '}' || b == ',' || b == ';'
}
func isOperatorStart(b byte) bool {
	return b == ':' || b == '=' || b == '+' || b == '-' || b == '*' || b == '/' || b == '%' || b == '<' || b == '>' || b == '!' || b == '&' || b == '|' || b == '.' || b == '^' || b == '~'
}
