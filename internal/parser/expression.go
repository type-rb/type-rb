package parser

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

type exprParser struct {
	tokens   []token.Token
	pos      int
	embedded map[int]ast.Expression
}

func parseExpressionTokens(tokens []token.Token) (ast.Expression, bool) {
	return parseExpressionTokensWithEmbedded(tokens, nil)
}

func parseExpressionTokensWithEmbedded(tokens []token.Token, embedded map[int]ast.Expression) (ast.Expression, bool) {
	filtered := make([]token.Token, 0, len(tokens))
	for _, tok := range tokens {
		if tok.Kind != token.Newline && tok.Kind != token.Comment {
			filtered = append(filtered, tok)
		}
	}
	if len(filtered) == 0 {
		return nil, false
	}
	p := &exprParser{tokens: filtered, embedded: embedded}
	expr := p.parse(0)
	return expr, expr != nil && p.pos == len(p.tokens)
}

var precedences = map[string]int{
	"or": 1, "||": 1,
	"and": 2, "&&": 2,
	"..": 3, "...": 3,
	"==": 4, "!=": 4, "=~": 4, "!~": 4,
	"<": 5, "<=": 5, ">": 5, ">=": 5, "<=>": 5,
	"|": 6, "^": 7, "&": 8,
	"<<": 9, ">>": 9,
	"+": 10, "-": 10,
	"*": 11, "/": 11, "%": 11,
	"**": 12,
}

func (p *exprParser) parse(min int) ast.Expression {
	left := p.parsePrefix()
	if left == nil {
		return nil
	}
	for p.pos < len(p.tokens) {
		tok := p.tokens[p.pos]
		if tok.Lexeme == "<" {
			if applied := p.parseGenericApplication(left); applied != nil {
				left = applied
				continue
			}
		}
		if tok.Lexeme == "(" {
			left = p.parseCall(left)
			continue
		}
		if tok.Lexeme == "." || tok.Lexeme == "&." || tok.Lexeme == "::" {
			if p.pos+1 >= len(p.tokens) {
				return nil
			}
			name := p.tokens[p.pos+1]
			base := ast.Base{SourceSpan: token.Span{Start: left.Span().Start, End: name.Span.End}}
			left = &ast.MemberExpression{Base: base, Receiver: left, Name: name.Lexeme, Safe: tok.Lexeme == "&.", Namespace: tok.Lexeme == "::"}
			p.pos += 2
			continue
		}
		if tok.Lexeme == "[" {
			p.pos++
			index := p.parse(0)
			if index == nil || !p.take("]") {
				return nil
			}
			left = &ast.IndexExpression{Base: ast.Base{SourceSpan: token.Span{Start: left.Span().Start, End: p.tokens[p.pos-1].Span.End}}, Receiver: left, Index: index}
			continue
		}
		prec, ok := precedences[tok.Lexeme]
		if !ok || prec <= min {
			break
		}
		p.pos++
		rightMin := prec
		if tok.Lexeme == "**" {
			rightMin--
		}
		right := p.parse(rightMin)
		if right == nil {
			return nil
		}
		if tok.Lexeme == ".." || tok.Lexeme == "..." {
			left = &ast.RangeExpression{Base: ast.Base{SourceSpan: token.Span{Start: left.Span().Start, End: right.Span().End}}, Start: left, End: right, Exclusive: tok.Lexeme == "..."}
		} else {
			left = &ast.BinaryExpression{Base: ast.Base{SourceSpan: token.Span{Start: left.Span().Start, End: right.Span().End}}, Left: left, Operator: tok.Lexeme, Right: right}
		}
	}
	return left
}

func (p *exprParser) parseGenericApplication(receiver ast.Expression) ast.Expression {
	// Keep comparison parsing unambiguous: explicit type arguments are only
	// recognized immediately before a call or namespace access.
	original := p.tokens
	prefix := append([]token.Token(nil), p.tokens[:p.pos]...)
	expanded := expandGenericClosers(p.tokens[p.pos:])
	p.tokens = append(prefix, expanded...)
	depth := 0
	close := -1
	for index := p.pos; index < len(p.tokens); index++ {
		switch p.tokens[index].Lexeme {
		case "<":
			depth++
		case ">":
			depth--
			if depth == 0 {
				close = index
			}
		}
		if close >= 0 {
			break
		}
	}
	if close < 0 || close+1 >= len(p.tokens) || p.tokens[close+1].Lexeme != "(" && p.tokens[close+1].Lexeme != "::" {
		p.tokens = original
		return nil
	}
	parts := splitTopLevel(p.tokens[p.pos+1:close], ",")
	arguments := make([]ast.TypeRef, 0, len(parts))
	for _, part := range parts {
		argument := parseType(part)
		if argument.Empty() {
			p.tokens = original
			return nil
		}
		arguments = append(arguments, argument)
	}
	if len(arguments) == 0 {
		p.tokens = original
		return nil
	}
	p.pos = close + 1
	return &ast.GenericExpression{
		Base:      ast.Base{SourceSpan: token.Span{Start: receiver.Span().Start, End: p.tokens[close].Span.End}},
		Receiver:  receiver,
		Arguments: arguments,
	}
}

func (p *exprParser) parsePrefix() ast.Expression {
	if p.pos >= len(p.tokens) {
		return nil
	}
	tok := p.tokens[p.pos]
	p.pos++
	if expression := p.embedded[tok.Span.Start.Offset]; expression != nil {
		return expression
	}
	switch tok.Lexeme {
	case "!", "not", "-", "+", "~":
		operand := p.parse(11)
		if operand == nil {
			return nil
		}
		return &ast.UnaryExpression{Base: ast.Base{SourceSpan: token.Span{Start: tok.Span.Start, End: operand.Span().End}}, Operator: tok.Lexeme, Operand: operand}
	case "attempt":
		operand := p.parse(0)
		if operand == nil {
			return nil
		}
		return &ast.AttemptExpression{Base: ast.Base{SourceSpan: token.Span{Start: tok.Span.Start, End: operand.Span().End}}, Value: operand}
	case "(":
		expr := p.parse(0)
		if expr == nil || !p.take(")") {
			return nil
		}
		return expr
	case "[":
		return p.parseArray(tok)
	case "{":
		return p.parseHash(tok)
	case ":":
		if p.pos >= len(p.tokens) {
			return nil
		}
		name := p.tokens[p.pos]
		p.pos++
		raw := ""
		value := name.Lexeme
		if name.Kind == token.String {
			raw = name.Lexeme
			value = unquote(name.Lexeme)
		}
		return &ast.SymbolLiteral{Base: ast.Base{SourceSpan: token.Span{Start: tok.Span.Start, End: name.Span.End}}, Name: value, Raw: raw}
	}
	if tok.Kind == token.String {
		if interpolated, ok := parseInterpolatedString(tok); ok {
			return interpolated
		}
		return &ast.Literal{Base: ast.Base{SourceSpan: tok.Span}, Kind: ast.StringLiteral, Raw: tok.Lexeme}
	}
	if tok.Kind == token.JSXLiteral {
		element, ok := parseJSXExpression(tok)
		if !ok {
			return nil
		}
		return element
	}
	if tok.Kind == token.NativeLiteral {
		return &ast.NativeExpression{Base: ast.Base{SourceSpan: tok.Span}, Text: tok.Lexeme}
	}
	if tok.Kind == token.Number {
		kind := ast.IntegerLiteral
		if strings.Contains(tok.Lexeme, ".") {
			kind = ast.FloatLiteral
		}
		return &ast.Literal{Base: ast.Base{SourceSpan: tok.Span}, Kind: kind, Raw: tok.Lexeme}
	}
	switch tok.Lexeme {
	case "true", "false":
		return &ast.Literal{Base: ast.Base{SourceSpan: tok.Span}, Kind: ast.BooleanLiteral, Raw: tok.Lexeme}
	case "nil":
		return &ast.Literal{Base: ast.Base{SourceSpan: tok.Span}, Kind: ast.NilLiteral, Raw: tok.Lexeme}
	}
	if tok.Kind == token.Identifier {
		return &ast.Identifier{Base: ast.Base{SourceSpan: tok.Span}, Name: tok.Lexeme}
	}
	return nil
}

func parseInterpolatedString(tok token.Token) (ast.Expression, bool) {
	raw := tok.Lexeme
	if len(raw) < 2 || raw[0] != '"' || !strings.Contains(raw, "#{") {
		return nil, false
	}
	content := raw[1 : len(raw)-1]
	result := &ast.InterpolatedString{Base: ast.Base{SourceSpan: tok.Span}, Raw: raw}
	textStart := 0
	for i := 0; i+1 < len(content); i++ {
		if content[i] == '\\' {
			i++
			continue
		}
		if content[i] != '#' || content[i+1] != '{' {
			continue
		}
		result.Parts = append(result.Parts, ast.StringPart{Text: content[textStart:i]})
		expressionStart := i + 2
		depth := 1
		quote := byte(0)
		j := expressionStart
		for ; j < len(content); j++ {
			b := content[j]
			if b == '\\' {
				j++
				continue
			}
			if quote != 0 {
				if b == quote {
					quote = 0
				}
				continue
			}
			if b == '\'' || b == '"' {
				quote = b
				continue
			}
			if b == '{' {
				depth++
			} else if b == '}' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if depth != 0 {
			return nil, false
		}
		innerTokens, diagnostics := lexer.Lex([]byte(content[expressionStart:j]))
		if len(diagnostics) > 0 {
			return nil, false
		}
		if len(innerTokens) > 0 && innerTokens[len(innerTokens)-1].Kind == token.EOF {
			innerTokens = innerTokens[:len(innerTokens)-1]
		}
		expression, ok := parseExpressionTokens(innerTokens)
		if !ok {
			return nil, false
		}
		result.Parts = append(result.Parts, ast.StringPart{Expression: expression})
		i = j
		textStart = j + 1
	}
	if textStart == 0 {
		return nil, false
	}
	result.Parts = append(result.Parts, ast.StringPart{Text: content[textStart:]})
	return result, true
}

func (p *exprParser) parseArray(open token.Token) ast.Expression {
	var elements []ast.Expression
	if p.take("]") {
		return &ast.ArrayLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}}
	}
	for {
		element := p.parse(0)
		if element == nil {
			return nil
		}
		elements = append(elements, element)
		if p.take("]") {
			return &ast.ArrayLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}, Elements: elements}
		}
		if !p.take(",") {
			return nil
		}
		if p.take("]") {
			return &ast.ArrayLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}, Elements: elements}
		}
	}
}

func (p *exprParser) parseHash(open token.Token) ast.Expression {
	var entries []ast.HashEntry
	if p.take("}") {
		return &ast.HashLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}}
	}
	for {
		key := p.parse(0)
		if key == nil {
			return nil
		}
		colon := p.take(":")
		if !colon && !p.take("=>") {
			return nil
		}
		if colon {
			if identifier, ok := key.(*ast.Identifier); ok {
				key = &ast.SymbolLiteral{Base: identifier.Base, Name: identifier.Name}
			}
		}
		value := p.parse(0)
		if value == nil {
			return nil
		}
		entries = append(entries, ast.HashEntry{Key: key, Value: value})
		if p.take("}") {
			return &ast.HashLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}, Entries: entries}
		}
		if !p.take(",") {
			return nil
		}
		if p.take("}") {
			return &ast.HashLiteral{Base: ast.Base{SourceSpan: token.Span{Start: open.Span.Start, End: p.tokens[p.pos-1].Span.End}}, Entries: entries}
		}
	}
}

func (p *exprParser) parseCall(callee ast.Expression) ast.Expression {
	p.pos++ // (
	var args []ast.CallArgument
	if p.take(")") {
		return &ast.CallExpression{Base: ast.Base{SourceSpan: token.Span{Start: callee.Span().Start, End: p.tokens[p.pos-1].Span.End}}, Callee: callee}
	}
	for {
		arg := ast.CallArgument{}
		if p.pos < len(p.tokens) && (p.tokens[p.pos].Lexeme == "*" || p.tokens[p.pos].Lexeme == "**") {
			arg.Splat = p.tokens[p.pos].Lexeme
			p.pos++
		}
		if p.pos+1 < len(p.tokens) && p.tokens[p.pos].Kind == token.Identifier && p.tokens[p.pos+1].Lexeme == ":" {
			arg.Name = p.tokens[p.pos].Lexeme
			p.pos += 2
		}
		arg.Value = p.parse(0)
		if arg.Value == nil {
			return nil
		}
		args = append(args, arg)
		if p.take(")") {
			return &ast.CallExpression{Base: ast.Base{SourceSpan: token.Span{Start: callee.Span().Start, End: p.tokens[p.pos-1].Span.End}}, Callee: callee, Arguments: args}
		}
		if !p.take(",") {
			return nil
		}
		if p.take(")") {
			return &ast.CallExpression{Base: ast.Base{SourceSpan: token.Span{Start: callee.Span().Start, End: p.tokens[p.pos-1].Span.End}}, Callee: callee, Arguments: args}
		}
	}
}

func (p *exprParser) take(lexeme string) bool {
	if p.pos < len(p.tokens) && p.tokens[p.pos].Lexeme == lexeme {
		p.pos++
		return true
	}
	return false
}

func parseType(tokens []token.Token) ast.TypeRef {
	if len(tokens) == 0 {
		return ast.TypeRef{}
	}
	tokens = expandGenericClosers(tokens)
	if arrow := topLevelIndex(tokens, "->"); arrow >= 0 {
		result := ast.TypeRef{Base: ast.Base{SourceSpan: spanOf(tokens)}}
		if arrow < 2 || tokens[0].Lexeme != "(" || tokens[arrow-1].Lexeme != ")" || matchingIndex(tokens, 0, "(", ")") != arrow-1 || arrow+1 >= len(tokens) {
			result.Name = joinLexemes(tokens)
			return result
		}
		for _, part := range splitTopLevel(tokens[1:arrow-1], ",") {
			if len(part) > 0 {
				result.FunctionParameters = append(result.FunctionParameters, parseType(part))
			}
		}
		returned := parseType(tokens[arrow+1:])
		result.FunctionReturn = &returned
		return result
	}
	if alternatives := splitTopLevel(tokens, "|"); len(alternatives) > 1 {
		result := ast.TypeRef{Base: ast.Base{SourceSpan: spanOf(tokens)}}
		for _, alternative := range alternatives {
			result.Union = append(result.Union, parseType(alternative))
		}
		return result
	}
	t := ast.TypeRef{Base: ast.Base{SourceSpan: spanOf(tokens)}}
	end := len(tokens)
	if tokens[end-1].Lexeme == "?" {
		t.Nullable = true
		end--
	}
	if end >= 2 && tokens[end-2].Lexeme == "[" && tokens[end-1].Lexeme == "]" {
		t.Array = true
		end -= 2
	}
	generic := -1
	for i := 0; i < end; i++ {
		if tokens[i].Lexeme == "<" {
			generic = i
			break
		}
	}
	if generic >= 0 && end > generic+1 && tokens[end-1].Lexeme == ">" {
		t.Name = joinLexemes(tokens[:generic])
		for _, part := range splitTopLevel(tokens[generic+1:end-1], ",") {
			t.Arguments = append(t.Arguments, parseType(part))
		}
	} else {
		t.Name = joinLexemes(tokens[:end])
	}
	return t
}

// The lexer must retain >> as an expression operator. In a type reference,
// however, the same bytes can close two nested generic argument lists.
func expandGenericClosers(tokens []token.Token) []token.Token {
	var expanded []token.Token
	for _, item := range tokens {
		if item.Lexeme != ">>" {
			expanded = append(expanded, item)
			continue
		}
		middle := item.Span.Start
		middle.Offset++
		middle.Column++
		first := item
		first.Lexeme = ">"
		first.Span.End = middle
		second := item
		second.Lexeme = ">"
		second.Span.Start = middle
		expanded = append(expanded, first, second)
	}
	return expanded
}

func splitTopLevel(tokens []token.Token, separator string) [][]token.Token {
	var out [][]token.Token
	start, depth := 0, 0
	for i, tok := range tokens {
		switch tok.Lexeme {
		case "(", "[", "{", "<":
			depth++
		case ")", "]", "}", ">":
			if depth > 0 {
				depth--
			}
		}
		if tok.Lexeme == separator && depth == 0 {
			out = append(out, tokens[start:i])
			start = i + 1
		}
	}
	out = append(out, tokens[start:])
	return out
}

func topLevelIndex(tokens []token.Token, lexeme string) int {
	depth := 0
	for i, tok := range tokens {
		if tok.Lexeme == lexeme && depth == 0 {
			return i
		}
		switch tok.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		}
	}
	return -1
}

func matchingIndex(tokens []token.Token, start int, open, close string) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Lexeme == open {
			depth++
		} else if tokens[i].Lexeme == close {
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func joinLexemes(tokens []token.Token) string {
	var b strings.Builder
	for _, tok := range tokens {
		b.WriteString(tok.Lexeme)
	}
	return b.String()
}
