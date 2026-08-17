package languageservice

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

func completeCallArgumentReferences(request CompletionRequest) ([]CompletionItem, bool) {
	tokens, _ := lexer.Lex([]byte(request.Source[:request.Cursor]))
	significant := completionTokens(tokens)
	open := innermostOpenCall(significant)
	if open < 1 {
		return nil, false
	}
	call, ok := completionCallSymbol(significant, open, request)
	if !ok || call.Call == nil {
		return nil, false
	}
	arguments := splitCompletionArguments(significant[open+1:])
	position, current, ok := positionalCompletionArgument(arguments)
	if !ok {
		return nil, false
	}
	parameter := positionalCallParameter(call.Call, position)
	if parameter == nil || len(parameter.ReferenceScopes) == 0 {
		return nil, false
	}
	scope, active := referenceScopeAt(parameter.ReferenceScopes, request.Source, request.Cursor, significant[open-1].Span.Start.Offset)
	if !active {
		return nil, false
	}
	if len(current) > 1 || len(current) == 1 && current[0].Kind != token.Identifier {
		return nil, true
	}
	replacement := completionRange(request.Source, request.Cursor)
	prefix := request.Source[replacement.Start:request.Cursor]
	items := make([]CompletionItem, 0, len(scope.Symbols))
	for _, symbol := range scope.Symbols {
		items = append(items, completionFromSymbol(symbol, replacement, request.Source))
	}
	return filterCompletions(items, prefix), true
}

func callArgumentReferenceSymbol(request SemanticRequest, item token.Token) (Symbol, bool) {
	tokens, _ := lexer.Lex([]byte(request.Source[:item.Span.End.Offset]))
	significant := completionTokens(tokens)
	if len(significant) == 0 || significant[len(significant)-1].Span != item.Span {
		return Symbol{}, false
	}
	open := innermostOpenCall(significant)
	if open < 1 {
		return Symbol{}, false
	}
	call, ok := completionCallSymbol(significant, open, CompletionRequest{
		Source: request.Source, Cursor: item.Span.End.Offset, Mode: request.Mode, Context: request.Context,
	})
	if !ok || call.Call == nil {
		return Symbol{}, false
	}
	arguments := splitCompletionArguments(significant[open+1:])
	position, current, ok := positionalCompletionArgument(arguments)
	if !ok || len(current) != 1 || current[0].Kind != token.Identifier || current[0].Span != item.Span {
		return Symbol{}, false
	}
	parameter := positionalCallParameter(call.Call, position)
	if parameter == nil {
		return Symbol{}, false
	}
	scope, active := referenceScopeAt(parameter.ReferenceScopes, request.Source, item.Span.Start.Offset, significant[open-1].Span.Start.Offset)
	if !active {
		return Symbol{}, false
	}
	for _, symbol := range scope.Symbols {
		if symbol.Name == item.Lexeme {
			return symbol, true
		}
	}
	return Symbol{}, false
}

func positionalCompletionArgument(arguments [][]token.Token) (int, []token.Token, bool) {
	if len(arguments) == 0 {
		return 0, nil, false
	}
	position := 0
	for _, argument := range arguments[:len(arguments)-1] {
		keyword, _ := completionKeywordArgument(argument)
		if keyword == "" {
			position++
		}
	}
	current := arguments[len(arguments)-1]
	if keyword, _ := completionKeywordArgument(current); keyword != "" {
		return 0, nil, false
	}
	return position, current, true
}

func referenceScopeAt(scopes []ReferenceScope, source string, cursor, callStart int) (ReferenceScope, bool) {
	program, diagnostics := parser.Parse([]byte(source))
	currentOwners := map[string]bool{}
	for _, scope := range scopes {
		contains, found := classDirectCallContainsCursor(program.Statements, scope.Owner, cursor, callStart)
		if found {
			currentOwners[scope.Owner] = true
		}
		if contains {
			return scope, true
		}
	}
	if len(diagnostics) == 0 {
		return ReferenceScope{}, false
	}
	for _, scope := range scopes {
		if currentOwners[scope.Owner] {
			continue
		}
		if scope.Range.Start <= cursor && cursor <= scope.Range.End {
			return scope, true
		}
	}
	return ReferenceScope{}, false
}

func classDirectCallContainsCursor(statements []ast.Statement, name string, cursor, callStart int) (bool, bool) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			if node.Name == name {
				for _, member := range node.Body {
					expression, ok := member.(*ast.ExpressionStatement)
					if !ok {
						continue
					}
					call, ok := expression.Expression.(*ast.CallExpression)
					if !ok || call.Callee.Span().Start.Offset != callStart {
						continue
					}
					span := call.Span()
					return span.Start.Offset <= cursor && cursor <= span.End.Offset, true
				}
				return false, true
			}
			if contains, found := classDirectCallContainsCursor(node.Body, name, cursor, callStart); found {
				return contains, true
			}
		case *ast.ModuleStatement:
			if contains, found := classDirectCallContainsCursor(node.Body, name, cursor, callStart); found {
				return contains, true
			}
		}
	}
	return false, false
}
