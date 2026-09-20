package parser

import "github.com/type-rb/type-rb/internal/token"

// SymbolNameOffsets exposes only token ownership to the REPL's completeness
// scanner. Parsing an incomplete submission can still identify literal names;
// no type checking or evaluation is performed here.
func SymbolNameOffsets(source []byte) map[int]bool {
	_, _, names := ParseWithSymbolNames(source)
	return names
}

// Use the expression grammar to distinguish a Symbol prefix from a Hash,
// argument or conditional colon before lifting an end-delimited construct.
// A partial expression is enough: parsePrefix records a Symbol name before an
// enclosing call or collection reports its missing closing delimiter.
func expressionSymbolNames(tokens []token.Token, owner *Parser) map[int]bool {
	names := map[int]bool{}
	start, depth := 0, 0
	for index, item := range tokens {
		if depth == 0 {
			switch item.Lexeme {
			case ":=", "=", "+=", "-=", "*=", "/=", "||=", "&&=":
				if index == 0 || tokens[index-1].Lexeme != ":" {
					start = index + 1
				}
			}
		}
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		}
	}
	if start == 0 && len(tokens) > 0 {
		switch tokens[0].Lexeme {
		case "return", "if", "elsif", "while", "case", "when":
			start++
		}
	}
	var probeOwner *Parser
	if owner != nil {
		probeOwner = &Parser{source: owner.source, tokens: owner.tokens, pos: owner.pos, statementDepth: owner.statementDepth, symbolNames: names}
	}
	probe := &exprParser{tokens: tokens[start:], owner: probeOwner, symbolNames: names}
	probe.parse(0)
	return names
}
