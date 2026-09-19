package parser

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

// Collection blocks are postfix expressions, including within call arguments,
// literals and operator operands. Parse their original statement tokens so
// nested control and source positions do not depend on the enclosing wrapper.
func (p *exprParser) parseIterationValue(receiver ast.Expression, iteration *ast.IterationExpression) ast.Expression {
	base := ast.Base{SourceSpan: receiver.Span()}
	if p.tokens[p.pos].Lexeme == "{" {
		value, close := p.owner.parseBraceIteration(p.tokens, p.pos, base, iteration)
		if close < 0 {
			return nil
		}
		p.pos = close + 1
		return value
	}
	start := p.pos
	if start+1 >= len(p.tokens) || p.tokens[start+1].Lexeme != "|" {
		p.reportAt(p.tokens[start].Span, "iteration block parameters must be written as |name, ...|")
		return nil
	}
	pipe := start + 2
	for pipe < len(p.tokens) && p.tokens[pipe].Lexeme != "|" {
		pipe++
	}
	if pipe >= len(p.tokens) {
		p.reportAt(p.tokens[start].Span, "iteration block parameters must end with |")
		return nil
	}
	parameters, valid := p.owner.blockParameters(p.tokens[start+1 : pipe+1])
	if !valid {
		p.reportAt(p.tokens[start].Span, "iteration block parameters must be identifiers")
		return nil
	}
	first := sort.Search(len(p.owner.tokens), func(index int) bool {
		return p.owner.tokens[index].Span.Start.Offset >= p.tokens[pipe].Span.End.Offset
	})
	finish := p.tokens[len(p.tokens)-1].Span.End
	last := sort.Search(len(p.owner.tokens), func(index int) bool {
		return p.owner.tokens[index].Span.Start.Offset >= finish.Offset
	})
	if first >= last || (p.owner.tokens[first].Kind != token.Newline && p.owner.tokens[first].Kind != token.Comment && p.owner.tokens[first].Lexeme != ";") {
		p.reportAt(p.tokens[pipe].Span, "expected a statement separator after iteration block parameters")
		return nil
	}
	bodyTokens := append([]token.Token(nil), p.owner.tokens[first:last]...)
	bodyTokens = append(bodyTokens, token.Token{Kind: token.EOF, Span: token.Span{Start: finish, End: finish}})
	child := &Parser{source: p.owner.source, tokens: bodyTokens, statementDepth: p.owner.statementDepth}
	body := child.parseStatements(map[string]bool{"end": true})
	p.owner.diags = append(p.owner.diags, child.diags...)
	p.owner.nativeIslands = append(p.owner.nativeIslands, child.nativeIslands...)
	if child.current().Lexeme != "end" {
		p.reportAt(p.tokens[start].Span, "unterminated iteration block; expected end")
		return nil
	}
	close := child.current()
	p.pos = sort.Search(len(p.tokens), func(index int) bool {
		return p.tokens[index].Span.Start.Offset >= close.Span.End.Offset
	})
	iteration.Base = base
	iteration.SourceSpan.End = close.Span.End
	iteration.Block = &ast.BlockExpression{
		Base:       ast.Base{SourceSpan: token.Span{Start: p.tokens[start].Span.Start, End: close.Span.End}},
		Parameters: parameters,
		Body:       body,
	}
	return iteration
}
