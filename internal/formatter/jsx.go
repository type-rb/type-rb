package formatter

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

func formatJSXToken(root token.Token, baseIndent int, flat bool) string {
	element, ok := parser.ParseJSXToken(root)
	if !ok {
		return root.Lexeme
	}
	var out strings.Builder
	writeJSXElement(&out, root, element, baseIndent, flat)
	return out.String()
}

func writeJSXElement(out *strings.Builder, root token.Token, element *ast.JSXElement, baseIndent int, flat bool) {
	name := element.Name
	out.WriteByte('<')
	out.WriteString(name)
	for _, attribute := range element.Attributes {
		out.WriteByte(' ')
		out.WriteString(attribute.Name)
		if attribute.Boolean {
			continue
		}
		out.WriteByte('=')
		if literal, ok := attribute.Value.(*ast.Literal); ok && literal.Kind == ast.StringLiteral {
			out.WriteString(literal.Raw)
			continue
		}
		out.WriteByte('{')
		out.WriteString(formatJSXEmbeddedExpression(root, attribute.Value))
		out.WriteByte('}')
	}

	if len(element.Children) == 0 && !element.Fragment {
		out.WriteString(" />")
		return
	}
	out.WriteByte('>')
	if len(element.Children) == 0 {
		out.WriteString("</>")
		return
	}

	multiline := !flat && jsxNeedsMultiline(element)
	if !multiline {
		for _, child := range element.Children {
			writeFlatJSXChild(out, root, child, baseIndent)
		}
		writeJSXClosingTag(out, name)
		return
	}

	out.WriteByte('\n')
	for _, child := range element.Children {
		out.WriteString(strings.Repeat(indentation, baseIndent+1))
		switch child := child.(type) {
		case *ast.JSXElement:
			writeJSXElement(out, root, child, baseIndent+1, false)
		case *ast.JSXExpression:
			out.WriteByte('{')
			out.WriteString(formatJSXEmbeddedExpression(root, child.Value))
			out.WriteByte('}')
		case *ast.JSXText:
			out.WriteString(child.Text)
		}
		out.WriteByte('\n')
	}
	out.WriteString(strings.Repeat(indentation, baseIndent))
	writeJSXClosingTag(out, name)
}

func writeFlatJSXChild(out *strings.Builder, root token.Token, child ast.JSXChild, baseIndent int) {
	switch child := child.(type) {
	case *ast.JSXElement:
		writeJSXElement(out, root, child, baseIndent, true)
	case *ast.JSXExpression:
		out.WriteByte('{')
		out.WriteString(formatJSXEmbeddedExpression(root, child.Value))
		out.WriteByte('}')
	case *ast.JSXText:
		out.WriteString(child.Text)
	}
}

func writeJSXClosingTag(out *strings.Builder, name string) {
	out.WriteString("</")
	out.WriteString(name)
	out.WriteByte('>')
}

// jsxNeedsMultiline expands structure-only child lists. Text-bearing elements
// stay flat so formatting cannot change provider-visible whitespace.
func jsxNeedsMultiline(element *ast.JSXElement) bool {
	for _, child := range element.Children {
		if _, ok := child.(*ast.JSXText); ok {
			return false
		}
	}
	if len(element.Children) > 1 {
		return true
	}
	if len(element.Children) == 1 {
		if child, ok := element.Children[0].(*ast.JSXElement); ok {
			return jsxNeedsMultiline(child)
		}
	}
	return false
}

func formatJSXEmbeddedExpression(root token.Token, expression ast.Expression) string {
	if expression == nil {
		return ""
	}
	source, ok := jsxSourceForSpan(root, expression.Span())
	if !ok {
		return ""
	}
	source = strings.TrimSpace(source)
	if strings.ContainsAny(source, "\r\n") {
		return source
	}
	tokens, diagnostics := lexer.Lex([]byte(source))
	if hasErrors(diagnostics) {
		return source
	}
	code := make([]token.Token, 0, len(tokens))
	for _, item := range tokens {
		if item.Kind != token.EOF && item.Kind != token.Newline {
			code = append(code, item)
		}
	}
	return formatTokensAt(code, 0, true)
}

func jsxSourceForSpan(root token.Token, span token.Span) (string, bool) {
	start := span.Start.Offset - root.Span.Start.Offset
	end := span.End.Offset - root.Span.Start.Offset
	if start < 0 || end < start || end > len(root.Lexeme) {
		return "", false
	}
	return root.Lexeme[start:end], true
}
