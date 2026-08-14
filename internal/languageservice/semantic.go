package languageservice

import (
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

// Hover resolves the checked symbol at a source position without exposing any
// editor-protocol representation to the compiler service.
func Hover(request SemanticRequest) (HoverInfo, bool) {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return HoverInfo{}, false
	}
	tokens, _ := lexer.Lex([]byte(request.Source))
	item, ok := semanticTokenAt(tokens, request.Cursor)
	if !ok {
		return HoverInfo{}, false
	}
	range_ := OffsetRange{Start: item.Span.Start.Offset, End: item.Span.End.Offset}
	switch item.Kind {
	case token.String:
		return HoverInfo{Range: range_, Detail: "String"}, true
	case token.Number:
		detail := "Integer"
		if strings.Contains(item.Lexeme, ".") {
			detail = "Float"
		}
		return HoverInfo{Range: range_, Detail: detail}, true
	case token.Identifier:
		if item.Lexeme == "true" || item.Lexeme == "false" {
			return HoverInfo{Range: range_, Detail: "Boolean"}, true
		}
		if item.Lexeme == "nil" {
			return HoverInfo{Range: range_, Detail: "nil"}, true
		}
	default:
		return HoverInfo{}, false
	}

	analysisCursor := semanticLineEnd(request.Source, item.Span.End.Offset)
	completionRequest := CompletionRequest{
		Source: request.Source, Cursor: analysisCursor, Mode: request.Mode, Context: request.Context,
	}
	if marker, receiver := memberReceiver(request.Source, item.Span.Start.Offset); marker != "" {
		members := completeMembers(receiver, marker, completionRequest, range_)
		for _, member := range members {
			if member.Label == item.Lexeme {
				detail := member.Detail
				if member.Kind == CompletionField && !strings.HasPrefix(detail, member.Label+":") {
					detail = member.Label + ": " + detail
				}
				return HoverInfo{Range: range_, Detail: detail}, true
			}
		}
		return HoverInfo{}, false
	}

	if symbol, found := semanticSymbol(item.Lexeme, request.Source, analysisCursor, request.Context); found {
		return HoverInfo{Range: range_, Detail: semanticDetail(symbol)}, true
	}
	return HoverInfo{}, false
}

// Signatures resolves the innermost call and its active argument using the
// same structured call metadata that powers completion.
func Signatures(request SemanticRequest) (SignatureHelp, bool) {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return SignatureHelp{}, false
	}
	tokens, _ := lexer.Lex([]byte(request.Source[:request.Cursor]))
	significant := completionTokens(tokens)
	open := innermostOpenCall(significant)
	if open < 1 {
		return SignatureHelp{}, false
	}
	call, ok := completionCallSymbol(significant, open, CompletionRequest{
		Source: request.Source, Cursor: request.Cursor, Mode: request.Mode, Context: request.Context,
	})
	if !ok || call.Call == nil {
		return SignatureHelp{}, false
	}

	parameters := make([]SignatureParameter, 0, len(call.Call.Parameters))
	for _, parameter := range call.Call.Parameters {
		label := parameter.Label
		if label == "" {
			label = parameter.Name
		}
		if label != "" {
			parameters = append(parameters, SignatureParameter{Label: label})
		}
	}
	arguments := splitCompletionArguments(significant[open+1:])
	active := activeCallParameter(call.Call, arguments)
	if len(parameters) > 0 && active >= len(parameters) {
		active = len(parameters) - 1
	}
	return SignatureHelp{
		Signatures:      []SignatureInfo{{Label: call.Detail, Parameters: parameters}},
		ActiveSignature: 0,
		ActiveParameter: active,
	}, true
}

func semanticTokenAt(tokens []token.Token, cursor int) (token.Token, bool) {
	for _, item := range tokens {
		if item.Kind == token.Comment || item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		if item.Span.Start.Offset <= cursor && cursor < item.Span.End.Offset {
			return item, true
		}
	}
	for _, item := range tokens {
		if item.Kind != token.Comment && item.Kind != token.Newline && item.Kind != token.EOF && item.Span.End.Offset == cursor {
			return item, true
		}
	}
	return token.Token{}, false
}

func semanticLineEnd(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		return len(source)
	}
	if newline := strings.IndexByte(source[offset:], '\n'); newline >= 0 {
		return offset + newline
	}
	return len(source)
}

func semanticSymbol(name, source string, cursor int, context Context) (Symbol, bool) {
	if symbol, ok := checkedSymbolLookup(source, cursor, context)(name); ok {
		return symbol, true
	}
	if detail, keyword := keywordDetails[name]; keyword {
		return Symbol{Name: name, Kind: CompletionKeyword, Detail: detail}, true
	}
	return Symbol{}, false
}

func semanticDetail(symbol Symbol) string {
	switch symbol.Kind {
	case CompletionVariable, CompletionParameter, CompletionConstant, CompletionField:
		if symbol.Type.Kind != "" {
			return symbol.Name + ": " + symbol.Type.String()
		}
	case CompletionType:
		if symbol.Detail == "built-in type" {
			return "type " + symbol.Name
		}
	}
	if symbol.Detail != "" {
		return symbol.Detail
	}
	if symbol.Type.Kind != "" {
		return symbol.Type.String()
	}
	return symbol.Name
}

func activeCallParameter(call *CallInfo, arguments [][]token.Token) int {
	if call == nil || len(call.Parameters) == 0 || len(arguments) == 0 {
		return 0
	}
	currentKeyword, _ := completionKeywordArgument(arguments[len(arguments)-1])
	if currentKeyword != "" {
		for index, parameter := range call.Parameters {
			if parameter.Keyword && parameter.Name == currentKeyword {
				return index
			}
		}
	}

	position := 0
	for _, argument := range arguments[:len(arguments)-1] {
		if keyword, _ := completionKeywordArgument(argument); keyword == "" {
			position++
		}
	}
	positional := 0
	for index, parameter := range call.Parameters {
		if parameter.Keyword {
			continue
		}
		if positional == position {
			return index
		}
		positional++
	}
	return min(position, len(call.Parameters)-1)
}
