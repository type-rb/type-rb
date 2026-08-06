package languageservice

import (
	"sort"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

// Highlight classifies source without adding terminal or browser formatting.
// Lexical diagnostics override ordinary token classifications.
func Highlight(request HighlightRequest) []HighlightSpan {
	tokens, diagnostics := lexer.Lex([]byte(request.Source))
	typeNames := map[string]bool{}
	semanticKinds := map[string]CompletionKind{}
	for _, name := range builtInTypes {
		typeNames[name] = true
	}
	for _, symbol := range request.Context.Symbols {
		semanticKinds[symbol.Name] = symbol.Kind
		if symbol.Kind == CompletionType {
			typeNames[symbol.Name] = true
		}
	}
	for _, symbol := range lexicalSymbols(request.Source, len(request.Source), request.Context) {
		semanticKinds[symbol.Name] = symbol.Kind
		if symbol.Kind == CompletionType {
			typeNames[symbol.Name] = true
		}
	}

	invalid := make([]OffsetRange, 0, len(diagnostics))
	for _, item := range diagnostics {
		start := clampOffset(item.Span.Start.Offset, len(request.Source))
		end := clampOffset(item.Span.End.Offset, len(request.Source))
		if end <= start && start < len(request.Source) {
			end = start + 1
		}
		if end > start {
			invalid = append(invalid, OffsetRange{Start: start, End: end})
		}
	}

	significant := make([]int, 0, len(tokens))
	for index, item := range tokens {
		if item.Kind != token.Comment && item.Kind != token.Newline && item.Kind != token.EOF {
			significant = append(significant, index)
		}
	}
	position := map[int]int{}
	for order, index := range significant {
		position[index] = order
	}

	spans := make([]HighlightSpan, 0, len(tokens))
	for index, item := range tokens {
		if item.Kind == token.EOF || item.Kind == token.Newline {
			continue
		}
		range_ := OffsetRange{Start: item.Span.Start.Offset, End: item.Span.End.Offset}
		if overlapsAny(range_, invalid) {
			spans = append(spans, HighlightSpan{Range: range_, Kind: HighlightInvalid})
			continue
		}
		kind, ok := highlightToken(item, neighboringTokens(tokens, significant, position[index]), typeNames, semanticKinds)
		if ok {
			spans = append(spans, HighlightSpan{Range: range_, Kind: kind})
		}
	}
	for _, item := range invalid {
		if !overlapsHighlight(item, spans) {
			spans = append(spans, HighlightSpan{Range: item, Kind: HighlightInvalid})
		}
	}
	sort.SliceStable(spans, func(left, right int) bool {
		if spans[left].Range.Start != spans[right].Range.Start {
			return spans[left].Range.Start < spans[right].Range.Start
		}
		return spans[left].Range.End < spans[right].Range.End
	})
	return spans
}

type tokenNeighbors struct {
	previous token.Token
	next     token.Token
	hasPrev  bool
	hasNext  bool
}

func neighboringTokens(tokens []token.Token, significant []int, order int) tokenNeighbors {
	result := tokenNeighbors{}
	if order > 0 && order <= len(significant)-1 {
		result.previous = tokens[significant[order-1]]
		result.hasPrev = true
	}
	if order >= 0 && order+1 < len(significant) {
		result.next = tokens[significant[order+1]]
		result.hasNext = true
	}
	return result
}

func highlightToken(item token.Token, neighbors tokenNeighbors, typeNames map[string]bool, semanticKinds map[string]CompletionKind) (HighlightKind, bool) {
	switch item.Kind {
	case token.Comment:
		return HighlightComment, true
	case token.String, token.NativeLiteral:
		return HighlightString, true
	case token.Number:
		return HighlightNumber, true
	case token.Identifier:
		if item.Lexeme == "true" || item.Lexeme == "false" {
			return HighlightBoolean, true
		}
		if _, keyword := keywordDetails[item.Lexeme]; keyword {
			return HighlightKeyword, true
		}
		if neighbors.hasPrev && neighbors.previous.Lexeme == "def" {
			return HighlightFunction, true
		}
		if neighbors.hasPrev && (neighbors.previous.Lexeme == "." || neighbors.previous.Lexeme == "&.") {
			return HighlightMethod, true
		}
		switch semanticKinds[item.Lexeme] {
		case CompletionFunction:
			return HighlightFunction, true
		case CompletionMethod:
			return HighlightMethod, true
		case CompletionConstant:
			return HighlightConstant, true
		case CompletionType:
			return HighlightType, true
		}
		if typeNames[item.Lexeme] || isTypeName(item.Lexeme) {
			return HighlightType, true
		}
		if isConstantName(item.Lexeme) || neighbors.hasPrev && neighbors.previous.Lexeme == "::" {
			return HighlightConstant, true
		}
		if neighbors.hasNext && neighbors.next.Lexeme == "(" {
			return HighlightFunction, true
		}
	}
	return "", false
}

func clampOffset(offset, length int) int {
	if offset < 0 {
		return 0
	}
	if offset > length {
		return length
	}
	return offset
}

func overlapsAny(target OffsetRange, ranges []OffsetRange) bool {
	for _, candidate := range ranges {
		if target.Start < candidate.End && candidate.Start < target.End {
			return true
		}
	}
	return false
}

func overlapsHighlight(target OffsetRange, spans []HighlightSpan) bool {
	for _, span := range spans {
		if target.Start < span.Range.End && span.Range.Start < target.End {
			return true
		}
	}
	return false
}
