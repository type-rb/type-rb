package lexer

import (
	"testing"

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
