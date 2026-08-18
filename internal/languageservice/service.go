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
	CompletionValue      CompletionKind = "value"
	CompletionCommand    CompletionKind = "command"
)

// OffsetRange is a half-open UTF-8 byte range. Protocol adapters may convert
// it to their own position representation without changing completion logic.
type OffsetRange struct {
	Start int
	End   int
}

// TextEdit is an editor-independent source change applied together with a
// completion. Protocol adapters translate byte offsets to their wire format.
type TextEdit struct {
	Range   OffsetRange
	NewText string
}

// Import identifies the explicit source import required by a completion.
type Import struct {
	Path   string
	Symbol string
}

// CompletionItem separates the displayed label from the exact source text
// that replaces Replacement.
type CompletionItem struct {
	Label           string
	InsertText      string
	Kind            CompletionKind
	Detail          string
	Replacement     OffsetRange
	AdditionalEdits []TextEdit
}

// SymbolID is a project-stable identity derived from a source declaration.
// Protocol adapters use it indirectly through semantic queries; generated and
// built-in symbols intentionally have no identity or source location.
type SymbolID string

type DefinitionLocation struct {
	ID    SymbolID
	Name  string
	Path  string
	Range OffsetRange
}

// CallInfo records structured call syntax without making adapters parse the
// human-readable signature in Detail.
type CallInfo struct {
	ParameterCount        int
	ExplicitTypeArguments bool
	TypeParameters        []string
	Parameters            []CallParameter
	Alternatives          []CallSignature
}

type CallParameter struct {
	Name                 string
	Label                string
	Keyword              bool
	Definition           *DefinitionLocation
	ReferenceScopes      []ReferenceScope
	LiteralValues        []string
	LiteralArrays        [][]string
	LiteralArrayElements []string
}

// ReferenceScope makes provider-owned project declarations visible only in a
// declarative call argument owned by one source declaration.
type ReferenceScope struct {
	Owner   string
	Range   OffsetRange
	Symbols []Symbol
}

type CallSignature struct {
	Parameters []CallParameter
}

// Symbol is the UI-independent semantic shape consumed by completion.
type Symbol struct {
	Name       string
	Kind       CompletionKind
	Detail     string
	Type       types.Type
	Call       *CallInfo
	Members    []Symbol
	Definition *DefinitionLocation
	Import     *Import
}

// TypeInfo retains generic parameters and transparent alias targets needed to
// instantiate checked member types for editor queries.
type TypeInfo struct {
	TypeParameters []string
	AliasTarget    *types.Type
}

// Context is the checked project information available at a cursor position.
// REPL contexts are rebuilt from typed IR after every accepted submission.
type Context struct {
	Symbols         []Symbol
	TypeMembers     map[string][]Symbol
	Types           map[string]TypeInfo
	Implementations map[SymbolID][]DefinitionLocation
}

type CompletionRequest struct {
	Source     string
	Cursor     int
	Mode       string
	Context    Context
	Candidates Context
	// RepairImports lets candidates replace checked symbols that are no longer
	// declared or imported by the current editor source. LSP adapters use this
	// while retaining a last-good checked context across invalid edits.
	RepairImports bool
}

// SemanticRequest identifies a source position using the same checked context
// as completion while keeping editor protocol details outside this package.
type SemanticRequest struct {
	Path    string
	Source  string
	Cursor  int
	Mode    string
	Context Context
}

type HoverInfo struct {
	Range  OffsetRange
	Detail string
}

type DefinitionInfo struct {
	ID    SymbolID
	Name  string
	Path  string
	Range OffsetRange
}

// SemanticDocument supplies one checked project file to project-wide semantic
// queries without exposing compiler snapshots or editor protocol types.
type SemanticDocument struct {
	Path    string
	Source  string
	Mode    string
	Context Context
}

type ReferenceInfo struct {
	ID          SymbolID
	Path        string
	Range       OffsetRange
	Declaration bool
}

type SignatureParameter struct {
	Label string
}

type SignatureInfo struct {
	Label      string
	Parameters []SignatureParameter
}

type SignatureHelp struct {
	Signatures      []SignatureInfo
	ActiveSignature int
	ActiveParameter int
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
	mu         sync.RWMutex
	mode       string
	context    Context
	candidates Context
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

// SetCandidates adds declarations that tooling may offer before source has
// imported them. They participate in completion and highlighting only; the
// compiler remains responsible for activating the corresponding import.
func (s *Service) SetCandidates(context Context) {
	s.mu.Lock()
	s.candidates = context
	s.mu.Unlock()
}

func (s *Service) Complete(source string, cursor int) []CompletionItem {
	s.mu.RLock()
	request := CompletionRequest{Source: source, Cursor: cursor, Mode: s.mode, Context: s.context, Candidates: s.candidates}
	s.mu.RUnlock()
	return Complete(request)
}

func (s *Service) Hover(source string, cursor int) (HoverInfo, bool) {
	s.mu.RLock()
	request := SemanticRequest{Source: source, Cursor: cursor, Mode: s.mode, Context: mergeCandidateContext(s.context, s.candidates)}
	s.mu.RUnlock()
	return Hover(request)
}

func (s *Service) Signatures(source string, cursor int) (SignatureHelp, bool) {
	s.mu.RLock()
	request := SemanticRequest{Source: source, Cursor: cursor, Mode: s.mode, Context: mergeCandidateContext(s.context, s.candidates)}
	s.mu.RUnlock()
	return Signatures(request)
}

func (s *Service) Definition(path, source string, cursor int) (DefinitionInfo, bool) {
	s.mu.RLock()
	request := SemanticRequest{Path: path, Source: source, Cursor: cursor, Mode: s.mode, Context: mergeCandidateContext(s.context, s.candidates)}
	s.mu.RUnlock()
	return Definition(request)
}

func (s *Service) Implementations(path, source string, cursor int) ([]DefinitionInfo, bool) {
	s.mu.RLock()
	request := SemanticRequest{Path: path, Source: source, Cursor: cursor, Mode: s.mode, Context: mergeCandidateContext(s.context, s.candidates)}
	s.mu.RUnlock()
	return Implementations(request)
}

func (s *Service) Highlight(source string) []HighlightSpan {
	s.mu.RLock()
	request := HighlightRequest{Source: source, Mode: s.mode, Context: mergeCandidateContext(s.context, s.candidates)}
	s.mu.RUnlock()
	return Highlight(request)
}

// MergeContexts adds completion-only declarations without overriding symbols
// already visible in checked source.
func MergeContexts(current, candidates Context) Context {
	result := current
	result.Symbols = append([]Symbol(nil), current.Symbols...)
	visible := make(map[string]bool, len(current.Symbols))
	for _, symbol := range current.Symbols {
		visible[symbol.Name] = true
	}
	for _, symbol := range candidates.Symbols {
		if !visible[symbol.Name] {
			result.Symbols = append(result.Symbols, symbol)
		}
	}
	return result
}

func mergeCandidateContext(current, candidates Context) Context {
	return MergeContexts(current, candidates)
}

func emptyContext() Context {
	return Context{
		TypeMembers:     map[string][]Symbol{},
		Types:           map[string]TypeInfo{},
		Implementations: map[SymbolID][]DefinitionLocation{},
	}
}
