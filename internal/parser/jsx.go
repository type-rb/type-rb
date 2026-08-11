package parser

import (
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

type jsxParser struct {
	raw  string
	root token.Token
	pos  int
}

func parseJSXExpression(root token.Token) (*ast.JSXElement, bool) {
	parser := &jsxParser{raw: root.Lexeme, root: root}
	element, ok := parser.element()
	parser.skipSpace()
	return element, ok && parser.pos == len(parser.raw)
}

func (p *jsxParser) element() (*ast.JSXElement, bool) {
	start := p.pos
	if !p.take("<") || p.starts("/") {
		return nil, false
	}
	name := p.name()
	fragment := name == ""
	result := &ast.JSXElement{Base: ast.Base{SourceSpan: p.span(start, start+1)}, Name: name, Fragment: fragment}
	if !fragment && startsComponentName(name) {
		component, ok := p.expression(name, start+1)
		if !ok {
			return nil, false
		}
		result.Component = component
	}
	for {
		p.skipSpace()
		if p.take("/>") {
			result.SourceSpan = p.span(start, p.pos)
			return result, true
		}
		if p.take(">") {
			break
		}
		attributeStart := p.pos
		attributeName := p.attributeName()
		if attributeName == "" {
			return nil, false
		}
		attribute := ast.JSXAttribute{Base: ast.Base{SourceSpan: p.span(attributeStart, p.pos)}, Name: attributeName, Boolean: true}
		p.skipSpace()
		if p.take("=") {
			attribute.Boolean = false
			p.skipSpace()
			switch {
			case p.pos < len(p.raw) && (p.raw[p.pos] == '\'' || p.raw[p.pos] == '"'):
				valueStart := p.pos
				raw, ok := p.quoted()
				if !ok {
					return nil, false
				}
				attribute.Value = &ast.Literal{Base: ast.Base{SourceSpan: p.span(valueStart, p.pos)}, Kind: ast.StringLiteral, Raw: raw}
			case p.starts("{"):
				inner, innerStart, ok := p.braced()
				if !ok {
					return nil, false
				}
				value, valid := p.expression(inner, innerStart)
				if !valid {
					return nil, false
				}
				attribute.Value = value
			default:
				return nil, false
			}
		}
		attribute.SourceSpan.End = p.position(p.pos)
		result.Attributes = append(result.Attributes, attribute)
	}

	for {
		if p.pos >= len(p.raw) {
			return nil, false
		}
		if p.starts("</") {
			p.pos += 2
			closingName := p.name()
			p.skipSpace()
			if !p.take(">") || closingName != name {
				return nil, false
			}
			result.SourceSpan = p.span(start, p.pos)
			return result, true
		}
		if p.starts("<") {
			child, ok := p.element()
			if !ok {
				return nil, false
			}
			result.Children = append(result.Children, child)
			continue
		}
		if p.starts("{") {
			childStart := p.pos
			inner, innerStart, ok := p.braced()
			if !ok {
				return nil, false
			}
			if strings.TrimSpace(inner) == "" {
				continue
			}
			value, valid := p.expression(inner, innerStart)
			if !valid {
				return nil, false
			}
			result.Children = append(result.Children, &ast.JSXExpression{Base: ast.Base{SourceSpan: p.span(childStart, p.pos)}, Value: value})
			continue
		}
		textStart := p.pos
		for p.pos < len(p.raw) && p.raw[p.pos] != '<' && p.raw[p.pos] != '{' {
			p.pos++
		}
		if p.pos > textStart {
			text := p.raw[textStart:p.pos]
			if strings.TrimSpace(text) != "" || !strings.Contains(text, "\n") {
				result.Children = append(result.Children, &ast.JSXText{Base: ast.Base{SourceSpan: p.span(textStart, p.pos)}, Text: text})
			}
		}
	}
}

func (p *jsxParser) expression(source string, relativeStart int) (ast.Expression, bool) {
	tokens, diagnostics := lexer.Lex([]byte(source))
	if len(diagnostics) > 0 {
		return nil, false
	}
	if len(tokens) > 0 && tokens[len(tokens)-1].Kind == token.EOF {
		tokens = tokens[:len(tokens)-1]
	}
	base := p.position(relativeStart)
	for index := range tokens {
		tokens[index].Span.Start = shiftJSXPosition(tokens[index].Span.Start, base)
		tokens[index].Span.End = shiftJSXPosition(tokens[index].Span.End, base)
	}
	return parseExpressionTokens(tokens)
}

func shiftJSXPosition(position, base token.Position) token.Position {
	result := position
	result.Offset += base.Offset
	result.Line += base.Line - 1
	if position.Line == 1 {
		result.Column += base.Column - 1
	}
	return result
}

func (p *jsxParser) braced() (string, int, bool) {
	if !p.take("{") {
		return "", 0, false
	}
	start := p.pos
	depth := 1
	quote := byte(0)
	for p.pos < len(p.raw) {
		current := p.raw[p.pos]
		if quote != 0 {
			if current == '\\' {
				p.pos += 2
				continue
			}
			p.pos++
			if current == quote {
				quote = 0
			}
			continue
		}
		if current == '\'' || current == '"' || current == '`' {
			quote = current
			p.pos++
			continue
		}
		if current == '{' {
			depth++
		} else if current == '}' {
			depth--
			if depth == 0 {
				value := p.raw[start:p.pos]
				p.pos++
				return value, start, true
			}
		}
		p.pos++
	}
	return "", 0, false
}

func (p *jsxParser) quoted() (string, bool) {
	if p.pos >= len(p.raw) || p.raw[p.pos] != '\'' && p.raw[p.pos] != '"' {
		return "", false
	}
	start := p.pos
	quote := p.raw[p.pos]
	p.pos++
	for p.pos < len(p.raw) {
		if p.raw[p.pos] == '\\' {
			p.pos += 2
			continue
		}
		current := p.raw[p.pos]
		p.pos++
		if current == quote {
			return p.raw[start:p.pos], true
		}
	}
	return "", false
}

func (p *jsxParser) name() string {
	start := p.pos
	for p.pos < len(p.raw) && isJSXNameRune(rune(p.raw[p.pos])) {
		p.pos++
	}
	return p.raw[start:p.pos]
}

func (p *jsxParser) attributeName() string {
	start := p.pos
	for p.pos < len(p.raw) {
		current := rune(p.raw[p.pos])
		if !isJSXNameRune(current) {
			break
		}
		p.pos++
	}
	return p.raw[start:p.pos]
}

func isJSXNameRune(value rune) bool {
	return value == '_' || value == '-' || value == '.' || value == ':' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func startsComponentName(name string) bool {
	if name == "" {
		return false
	}
	first, _ := utf8DecodeRune(name)
	return unicode.IsUpper(first)
}

func utf8DecodeRune(value string) (rune, int) {
	for _, character := range value {
		return character, len(string(character))
	}
	return 0, 0
}

func (p *jsxParser) skipSpace() {
	for p.pos < len(p.raw) {
		switch p.raw[p.pos] {
		case ' ', '\t', '\r', '\n':
			p.pos++
		default:
			return
		}
	}
}

func (p *jsxParser) starts(value string) bool {
	return strings.HasPrefix(p.raw[p.pos:], value)
}

func (p *jsxParser) take(value string) bool {
	if !p.starts(value) {
		return false
	}
	p.pos += len(value)
	return true
}

func (p *jsxParser) span(start, end int) token.Span {
	return token.Span{Start: p.position(start), End: p.position(end)}
}

func (p *jsxParser) position(relative int) token.Position {
	position := p.root.Span.Start
	position.Offset += relative
	for index := 0; index < relative && index < len(p.raw); index++ {
		if p.raw[index] == '\n' {
			position.Line++
			position.Column = 1
		} else {
			position.Column++
		}
	}
	return position
}
