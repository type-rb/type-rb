package languageservice

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

// SelectionRange describes one source selection and the next enclosing
// selection. It is editor-independent and uses UTF-8 byte offsets.
type SelectionRange struct {
	Range  OffsetRange
	Parent *SelectionRange
}

// SelectionRanges builds token, line, and structural selection chains for the
// requested source offsets. It uses syntax ranges so it remains available for
// incomplete or type-invalid documents.
func SelectionRanges(source string, cursors []int) []SelectionRange {
	tokens, _ := lexer.Lex([]byte(source))
	folds := FoldingRanges(source)
	result := make([]SelectionRange, 0, len(cursors))
	for _, requested := range cursors {
		cursor := max(0, min(requested, len(source)))
		candidates := selectionCandidates(source, cursor, tokens, folds)
		current := OffsetRange{Start: cursor, End: cursor}
		chain := []OffsetRange{}
		for _, candidate := range candidates {
			if candidate == current || candidate.Start > current.Start || candidate.End < current.End {
				continue
			}
			chain = append(chain, candidate)
			current = candidate
		}
		result = append(result, selectionChain(chain, current))
	}
	return result
}

func selectionCandidates(source string, cursor int, tokens []token.Token, folds []FoldingRange) []OffsetRange {
	candidates := []OffsetRange{}
	if item, ok := selectionTokenAt(tokens, cursor); ok {
		candidates = append(candidates, OffsetRange{Start: item.Span.Start.Offset, End: item.Span.End.Offset})
	}
	candidates = append(candidates, sourceLineRange(source, cursor))
	for _, fold := range folds {
		if fold.Range.Start <= cursor && cursor <= fold.Range.End {
			candidates = append(candidates, fold.Range)
		}
	}
	candidates = append(candidates, OffsetRange{Start: 0, End: len(source)})

	seen := map[OffsetRange]bool{}
	unique := candidates[:0]
	for _, candidate := range candidates {
		if seen[candidate] {
			continue
		}
		seen[candidate] = true
		unique = append(unique, candidate)
	}
	sort.SliceStable(unique, func(left, right int) bool {
		leftSize := unique[left].End - unique[left].Start
		rightSize := unique[right].End - unique[right].Start
		if leftSize != rightSize {
			return leftSize < rightSize
		}
		return unique[left].Start > unique[right].Start
	})
	return unique
}

func selectionTokenAt(tokens []token.Token, cursor int) (token.Token, bool) {
	for _, item := range tokens {
		if item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		if item.Span.Start.Offset <= cursor && cursor < item.Span.End.Offset {
			return item, true
		}
	}
	for _, item := range tokens {
		if item.Kind != token.Newline && item.Kind != token.EOF && item.Span.End.Offset == cursor {
			return item, true
		}
	}
	return token.Token{}, false
}

func sourceLineRange(source string, cursor int) OffsetRange {
	start := strings.LastIndexByte(source[:cursor], '\n') + 1
	end := len(source)
	if relative := strings.IndexByte(source[cursor:], '\n'); relative >= 0 {
		end = cursor + relative
	}
	return OffsetRange{Start: start, End: end}
}

func selectionChain(ranges []OffsetRange, fallback OffsetRange) SelectionRange {
	if len(ranges) == 0 {
		return SelectionRange{Range: fallback}
	}
	var parent *SelectionRange
	for index := len(ranges) - 1; index >= 0; index-- {
		parent = &SelectionRange{Range: ranges[index], Parent: parent}
	}
	return *parent
}
