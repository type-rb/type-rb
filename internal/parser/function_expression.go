package parser

import (
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

type parsedFunctionExpression struct {
	value *ast.LambdaExpression
	next  int
}

func startsFunctionExpression(tokens []token.Token, at int) bool {
	return at >= 0 && at+1 < len(tokens) && tokens[at].Lexeme == "fn" && tokens[at+1].Lexeme == "(" &&
		(at == 0 || tokens[at-1].Lexeme != "." && tokens[at-1].Lexeme != "&." && tokens[at-1].Lexeme != "::")
}

// Function bodies retain their original statement separators even when an
// enclosing expression has removed newlines and comments from its token view.
// Control-expression scanning and the expression parser share the parsed body.
func (p *Parser) functionExpressionAt(offset int) (*ast.LambdaExpression, int) {
	if parsed, ok := p.functionExpressions[offset]; ok {
		return parsed.value, parsed.next
	}
	if p.functionExpressions == nil {
		p.functionExpressions = map[int]parsedFunctionExpression{}
	}
	position := sort.Search(len(p.tokens), func(index int) bool {
		return p.tokens[index].Span.Start.Offset >= offset
	})
	child := &Parser{
		source: p.source, tokens: p.tokens, pos: position,
		statementDepth: p.statementDepth, symbolNames: p.symbolNames,
		functionExpressions: p.functionExpressions,
	}
	start, end, next, _ := child.logicalLine(position)
	line := child.codeTokens(start, end)
	close := matchingIndex(line, 1, "(", ")")
	node := &ast.LambdaExpression{Base: ast.Base{SourceSpan: spanOf(line)}}
	if close < 0 {
		child.errorAt(node.Span(), "unterminated fn parameters; expected )")
		child.pos = next
	} else {
		node.Parameters = child.parseParameters(line[2:close])
		tail := line[close+1:]
		failsAt := topLevelIndex(tail, "fails")
		returnTail := tail
		if failsAt >= 0 {
			child.migrationErrorAt(tail[failsAt].Span, failsRemovedMessage)
			returnTail = tail[:failsAt]
			if failsAt+1 < len(tail) {
				node.Fails = child.parseTypeRef(tail[failsAt+1:])
			}
		}
		if len(returnTail) > 0 {
			if returnTail[0].Lexeme != ":" || len(returnTail) == 1 {
				child.errorAt(spanOf(returnTail), "fn return type must be written as : Type")
			} else {
				node.ReturnType = child.parseReturnType(returnTail[1:])
			}
		}
		child.pos = next
		node.Body = child.parseStatements(map[string]bool{"end": true})
		if child.current().Lexeme != "end" {
			child.errorAt(child.current().Span, "expected \"end\"")
		} else {
			node.SourceSpan.End = child.current().Span.End
			child.pos++
		}
	}
	p.diags = append(p.diags, child.diags...)
	p.nativeIslands = append(p.nativeIslands, child.nativeIslands...)
	p.functionExpressions[offset] = parsedFunctionExpression{value: node, next: child.pos}
	return node, child.pos
}

func (p *exprParser) parseFunctionValue(start token.Token) ast.Expression {
	value, _ := p.owner.functionExpressionAt(start.Span.Start.Offset)
	p.pos = sort.Search(len(p.tokens), func(index int) bool {
		return p.tokens[index].Span.Start.Offset >= value.Span().End.Offset
	})
	return value
}
