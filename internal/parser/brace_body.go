package parser

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

// Parse the original token interval so newlines, comments, nested statements
// and diagnostic positions follow the same rules as a do/end body. The outer
// logical line has already removed newlines and is only useful for its bounds.
func (p *Parser) braceBody(start, finish token.Position) []ast.Statement {
	first := sort.Search(len(p.tokens), func(index int) bool {
		return p.tokens[index].Span.Start.Offset >= start.Offset
	})
	last := sort.Search(len(p.tokens), func(index int) bool {
		return p.tokens[index].Span.Start.Offset >= finish.Offset
	})
	tokens := append([]token.Token(nil), p.tokens[first:last]...)
	tokens = append(tokens, token.Token{Kind: token.EOF, Span: token.Span{Start: finish, End: finish}})
	child := &Parser{source: p.source, tokens: tokens, statementDepth: p.statementDepth, symbolNames: p.symbolNames}
	body := child.parseStatements(nil)
	p.diags = append(p.diags, child.diags...)
	p.nativeIslands = append(p.nativeIslands, child.nativeIslands...)
	return body
}
