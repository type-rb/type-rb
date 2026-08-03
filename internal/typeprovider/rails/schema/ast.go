// Package schema defines the normalized AST for the subset of Rails schema DSL
// that contributes model types. Non-type-bearing statements such as indexes
// are intentionally ignored by the parser.
package schema

import "github.com/type-rb/type-rb/internal/token"

type Schema struct {
	Tables []Table
}

type Table struct {
	Name    string
	ID      bool
	Columns []Column
	Span    token.Span
}

type Column struct {
	Name         string
	DatabaseType string
	Nullable     bool
	Span         token.Span
}
