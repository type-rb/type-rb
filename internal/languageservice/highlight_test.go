package languageservice_test

import (
	"testing"

	"github.com/type-rb/type-rb/internal/languageservice"
)

func TestHighlightClassifiesTypeRBTokens(t *testing.T) {
	source := "def greet(name: String): String\n\tMAX := 2\n\tputs(\"Hi\") # note\n\treturn true\nend"
	spans := languageservice.Highlight(languageservice.HighlightRequest{Source: source, Mode: "go"})

	for _, test := range []struct {
		text string
		kind languageservice.HighlightKind
	}{
		{text: "def", kind: languageservice.HighlightKeyword},
		{text: "greet", kind: languageservice.HighlightFunction},
		{text: "String", kind: languageservice.HighlightType},
		{text: "MAX", kind: languageservice.HighlightConstant},
		{text: "2", kind: languageservice.HighlightNumber},
		{text: "puts", kind: languageservice.HighlightFunction},
		{text: `"Hi"`, kind: languageservice.HighlightString},
		{text: "# note", kind: languageservice.HighlightComment},
		{text: "true", kind: languageservice.HighlightBoolean},
	} {
		if !hasHighlight(source, spans, test.text, test.kind) {
			t.Errorf("missing %q highlight for %q in %#v", test.kind, test.text, spans)
		}
	}
}

func TestHighlightMarksLexicallyInvalidInputWithoutPanicking(t *testing.T) {
	source := `value := "unterminated`
	spans := languageservice.Highlight(languageservice.HighlightRequest{Source: source, Mode: "go"})
	if !hasHighlight(source, spans, `"unterminated`, languageservice.HighlightInvalid) {
		t.Fatalf("spans=%#v", spans)
	}
}

func TestHighlightDoesNotClassifyLegacyEffectWordsAsKeywords(t *testing.T) {
	source := "attempt\nfails"
	spans := languageservice.Highlight(languageservice.HighlightRequest{Source: source, Mode: "go"})
	for _, word := range []string{"attempt", "fails"} {
		if hasHighlight(source, spans, word, languageservice.HighlightKeyword) {
			t.Errorf("legacy word %q retained keyword highlight in %#v", word, spans)
		}
	}
}

func TestHighlightClassifiesResultControlFlowKeywords(t *testing.T) {
	source := "value := try load()\nreturn load() catch |error|\n\terror.message\nend"
	spans := languageservice.Highlight(languageservice.HighlightRequest{Source: source, Mode: "go"})
	for _, keyword := range []string{"try", "catch"} {
		if !hasHighlight(source, spans, keyword, languageservice.HighlightKeyword) {
			t.Errorf("missing keyword highlight for %q in %#v", keyword, spans)
		}
	}
}

func TestHighlightUsesByteOffsetsForUnicodeSource(t *testing.T) {
	source := `puts("こんにちは")`
	spans := languageservice.Highlight(languageservice.HighlightRequest{Source: source, Mode: "go"})
	if !hasHighlight(source, spans, `"こんにちは"`, languageservice.HighlightString) {
		t.Fatalf("spans=%#v", spans)
	}
}

func hasHighlight(source string, spans []languageservice.HighlightSpan, text string, kind languageservice.HighlightKind) bool {
	for _, span := range spans {
		if span.Kind == kind && source[span.Range.Start:span.Range.End] == text {
			return true
		}
	}
	return false
}
