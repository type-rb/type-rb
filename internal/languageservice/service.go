// Package languageservice provides editor-independent analysis used by the
// REPL and future browser and language-server adapters.
package languageservice

import (
	"sync"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type CompletionKind string

const (
	CompletionKeyword    CompletionKind = "keyword"
	CompletionVariable   CompletionKind = "variable"
	CompletionParameter  CompletionKind = "parameter"
	CompletionConstant   CompletionKind = "constant"
	CompletionFunction   CompletionKind = "function"
	CompletionMethod     CompletionKind = "method"
	CompletionField      CompletionKind = "field"
	CompletionType       CompletionKind = "type"
	CompletionEnumMember CompletionKind = "enum_member"
	CompletionModule     CompletionKind = "module"
	CompletionCommand    CompletionKind = "command"
)

// OffsetRange is a half-open UTF-8 byte range. Protocol adapters may convert
// it to their own position representation without changing completion logic.
type OffsetRange struct {
	Start int
	End   int
}

// CompletionItem separates the displayed label from the exact source text
// that replaces Replacement.
type CompletionItem struct {
	Label       string
	InsertText  string
	Kind        CompletionKind
	Detail      string
	Replacement OffsetRange
}

// CallInfo records structured call syntax without making adapters parse the
// human-readable signature in Detail.
type CallInfo struct {
	ParameterCount        int
	ExplicitTypeArguments bool
}

// Symbol is the UI-independent semantic shape consumed by completion.
type Symbol struct {
	Name    string
	Kind    CompletionKind
	Detail  string
	Type    types.Type
	Call    *CallInfo
	Members []Symbol
}

// Context is the checked project information available at a cursor position.
// REPL contexts are rebuilt from typed IR after every accepted submission.
type Context struct {
	Symbols     []Symbol
	TypeMembers map[string][]Symbol
}

type CompletionRequest struct {
	Source  string
	Cursor  int
	Mode    string
	Context Context
}

type HighlightKind string

const (
	HighlightKeyword  HighlightKind = "keyword"
	HighlightType     HighlightKind = "type"
	HighlightConstant HighlightKind = "constant"
	HighlightString   HighlightKind = "string"
	HighlightNumber   HighlightKind = "number"
	HighlightBoolean  HighlightKind = "boolean"
	HighlightComment  HighlightKind = "comment"
	HighlightFunction HighlightKind = "function"
	HighlightMethod   HighlightKind = "method"
	HighlightInvalid  HighlightKind = "invalid"
)

type HighlightSpan struct {
	Range OffsetRange
	Kind  HighlightKind
}

type HighlightRequest struct {
	Source  string
	Mode    string
	Context Context
}

// Service owns a checked project snapshot while keeping terminal and browser
// presentation concerns outside the language analysis layer.
type Service struct {
	mu      sync.RWMutex
	mode    string
	context Context
}

func New(mode string) *Service {
	return &Service{mode: mode, context: emptyContext()}
}

func (s *Service) Update(programs []*ir.Program, modulePath string) {
	context := BuildContext(programs, modulePath)
	s.mu.Lock()
	s.context = context
	s.mu.Unlock()
}

func (s *Service) Complete(source string, cursor int) []CompletionItem {
	s.mu.RLock()
	request := CompletionRequest{Source: source, Cursor: cursor, Mode: s.mode, Context: s.context}
	s.mu.RUnlock()
	return Complete(request)
}

func (s *Service) Highlight(source string) []HighlightSpan {
	s.mu.RLock()
	request := HighlightRequest{Source: source, Mode: s.mode, Context: s.context}
	s.mu.RUnlock()
	return Highlight(request)
}

func emptyContext() Context {
	return Context{TypeMembers: map[string][]Symbol{}}
}
