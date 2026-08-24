package token

import "fmt"

// Position is a one-based source location with a zero-based byte offset.
type Position struct {
	Offset int
	Line   int
	Column int
}

func (p Position) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Column) }

// Span is a half-open range in the original source.
type Span struct {
	Start Position
	End   Position
}

type Kind string

const (
	EOF           Kind = "eof"
	Identifier    Kind = "identifier"
	Number        Kind = "number"
	String        Kind = "string"
	JSXLiteral    Kind = "jsx_literal"
	NativeLiteral Kind = "native_literal"
	NativeIsland  Kind = "native_island"
	Comment       Kind = "comment"
	Newline       Kind = "newline"
	Operator      Kind = "operator"
	Punct         Kind = "punctuation"
)

// Token retains its exact spelling so formatters and source-to-source backends
// never need to reconstruct comments or literals.
type Token struct {
	Kind   Kind
	Lexeme string
	Span   Span
}
