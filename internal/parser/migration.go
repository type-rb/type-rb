package parser

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

const (
	failsRemovedMessage   = "fails was removed in TypeRB 0.3; return Result<T, E> instead (see docs/migrations/0.3-result-control.md)"
	attemptRemovedMessage = "attempt was removed in TypeRB 0.3; the operation already returns Result (see docs/migrations/0.3-result-control.md)"
)

func (p *Parser) parseExpression(tokens []token.Token) (ast.Expression, bool) {
	return parseExpressionTokensReporting(tokens, nil, p.migrationErrorAt)
}

func (p *Parser) parseExpressionWithEmbedded(tokens []token.Token, embedded map[int]ast.Expression) (ast.Expression, bool) {
	return parseExpressionTokensReporting(tokens, embedded, p.migrationErrorAt)
}

func (p *Parser) parseTypeRef(tokens []token.Token) ast.TypeRef {
	return parseTypeReporting(tokens, p.migrationErrorAt)
}

func (p *Parser) migrationErrorAt(span token.Span, message string) {
	for _, item := range p.diags {
		if item.Span == span && item.Message == message {
			return
		}
	}
	p.errorAt(span, message)
}
