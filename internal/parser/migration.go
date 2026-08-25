package parser

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

const (
	failsRemovedMessage   = "fails is not valid TypeRB syntax; return Result<T, E> (see docs/language.md#result-control-flow)"
	attemptRemovedMessage = "attempt is not valid TypeRB syntax; handle the Result directly (see docs/language.md#result-control-flow)"
	typeAliasMovedMessage = "use alias Name = Target for a transparent alias or newtype Name = Target for a distinct nominal type (see docs/language.md#aliases-and-newtypes)"
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
