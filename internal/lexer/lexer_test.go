package lexer

import (
	"testing"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/token"
)

func TestLexPreservesCommentsAndSpans(t *testing.T) {
	tokens, diags := Lex([]byte("class User # target\nname := \"A\"\nend\n"))
	if len(diags) != 0 {
		t.Fatalf("unexpected diagnostics: %v", diags)
	}
	found := false
	for _, tok := range tokens {
		if tok.Kind == token.Comment {
			found = true
			if tok.Lexeme != "# target" || tok.Span.Start.Line != 1 {
				t.Fatalf("bad comment token: %#v", tok)
			}
		}
	}
	if !found {
		t.Fatal("comment token not emitted")
	}
}

func TestLexUnicodeIdentifier(t *testing.T) {
	tokens, diags := Lex([]byte("名前 := \"太郎\"\n"))
	if len(diags) != 0 || tokens[0].Lexeme != "名前" {
		t.Fatalf("tokens=%#v diagnostics=%v", tokens, diags)
	}
}

func TestLexRejectsInvalidUTF8AtTheFirstInvalidByte(t *testing.T) {
	source := append([]byte("名前 := 1\nvalue := "), 0xff, 0xfe)
	tokens, diagnostics := Lex(source)
	if len(tokens) != 1 || tokens[0].Kind != token.EOF {
		t.Fatalf("invalid source produced tokens: %#v", tokens)
	}
	if len(diagnostics) != 1 {
		t.Fatalf("diagnostics=%#v", diagnostics)
	}
	item := diagnostics[0]
	wantOffset := len([]byte("名前 := 1\nvalue := "))
	if item.Code != diagnostic.SyntaxError || item.Message != "source is not valid UTF-8" {
		t.Fatalf("unexpected diagnostic: %#v", item)
	}
	if item.Span.Start.Offset != wantOffset || item.Span.Start.Line != 2 || item.Span.Start.Column != 10 || item.Span.End.Offset != wantOffset+1 || item.Span.End.Column != 11 {
		t.Fatalf("unexpected invalid byte span: %#v", item.Span)
	}
}

func TestLexRangeOperatorsLongestFirst(t *testing.T) {
	tokens, diagnostics := Lex([]byte("0..10\n0...10\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	operators := []string{}
	for _, item := range tokens {
		if item.Kind == token.Operator {
			operators = append(operators, item.Lexeme)
		}
	}
	if len(operators) != 2 || operators[0] != ".." || operators[1] != "..." {
		t.Fatalf("range operators were not tokenized atomically: %#v", operators)
	}
}

func TestLexRegexMayStartAfterStatementSeparator(t *testing.T) {
	tokens, diagnostics := Lex([]byte("value := 1; /a;b/ # comment; text\n"))
	if len(diagnostics) != 0 {
		t.Fatal(diagnostics)
	}
	for _, item := range tokens {
		if item.Kind == token.NativeLiteral && item.Lexeme == "/a;b/" {
			return
		}
	}
	t.Fatalf("regex after semicolon was not retained as one literal: %#v", tokens)
}
