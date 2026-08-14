// Package lsp adapts the shared compiler and language services to the Language
// Server Protocol. Protocol transport remains separate from project discovery.
package lsp

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

type UnitResolver func(filename string, source []byte) (compiler.SourceUnit, error)

type Options struct {
	Mode            string
	Version         string
	Units           []compiler.SourceUnit
	CompilerOptions compiler.Options
	ResolveUnit     UnitResolver
	Input           io.Reader
	Output          io.Writer
}

type document struct {
	unit    compiler.SourceUnit
	source  []byte
	version int
}

type Server struct {
	mode        string
	version     string
	stream      *rpcStream
	compiler    *compilerservice.Service
	resolveUnit UnitResolver
	documents   map[string]document
	base        map[string]compiler.SourceUnit
	published   map[string]bool
	snapshot    compilerservice.Snapshot
}

const textDocumentSyncIncremental = 2

func New(options Options) *Server {
	base := make(map[string]compiler.SourceUnit, len(options.Units))
	for _, unit := range options.Units {
		unit.Filename = cleanPath(unit.Filename)
		base[unit.Filename] = unit
	}
	return &Server{
		mode: options.Mode, version: options.Version,
		stream:   newRPCStream(options.Input, options.Output),
		compiler: compilerservice.New(options.Units, options.CompilerOptions), resolveUnit: options.ResolveUnit,
		documents: map[string]document{}, base: base, published: map[string]bool{},
	}
}

func (s *Server) Run() error {
	for {
		request, err := s.stream.read()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		exit, err := s.handle(request)
		if err != nil {
			return err
		}
		if exit {
			return nil
		}
	}
}

func (s *Server) handle(request message) (bool, error) {
	switch request.Method {
	case "initialize":
		return false, s.stream.write(success(request.ID, initializeResult{
			Capabilities: serverCapabilities{
				TextDocumentSync:          textDocumentSyncIncremental,
				CompletionProvider:        completionOptions{TriggerCharacters: []string{".", ":"}},
				HoverProvider:             true,
				SignatureHelpProvider:     signatureOptions{TriggerCharacters: []string{"(", ","}},
				DefinitionProvider:        true,
				ReferencesProvider:        true,
				DocumentHighlightProvider: true,
				RenameProvider:            renameOptions{PrepareProvider: true},
				DocumentSymbolProvider:    true,
				FoldingRangeProvider:      true,
				WorkspaceSymbolProvider:   true,
				SemanticTokensProvider: semanticTokensOptions{
					Legend: semanticTokensLegend{TokenTypes: semanticTokenTypes, TokenModifiers: semanticTokenModifiers},
					Full:   true,
				},
				DocumentFormattingProvider: true, CodeActionProvider: true,
			},
			ServerInfo: serverInfo{Name: "TypeRB", Version: s.version},
		}))
	case "initialized":
		return false, s.publish()
	case "$/cancelRequest", "workspace/didChangeConfiguration":
		return false, nil
	case "shutdown":
		return false, s.stream.write(success(request.ID, nil))
	case "exit":
		return true, nil
	case "textDocument/didOpen":
		params, err := decodeParams[didOpenParams](request.Params)
		if err != nil {
			return false, err
		}
		return false, s.open(params.TextDocument)
	case "textDocument/didChange":
		params, err := decodeParams[didChangeParams](request.Params)
		if err != nil {
			return false, err
		}
		return false, s.change(params)
	case "textDocument/didClose":
		params, err := decodeParams[didCloseParams](request.Params)
		if err != nil {
			return false, err
		}
		return false, s.close(params.TextDocument.URI)
	case "textDocument/completion":
		return false, s.completion(request)
	case "textDocument/hover":
		return false, s.hover(request)
	case "textDocument/signatureHelp":
		return false, s.signatureHelp(request)
	case "textDocument/definition":
		return false, s.definition(request)
	case "textDocument/references":
		return false, s.references(request)
	case "textDocument/documentHighlight":
		return false, s.documentHighlights(request)
	case "textDocument/prepareRename":
		return false, s.prepareRename(request)
	case "textDocument/rename":
		return false, s.rename(request)
	case "textDocument/documentSymbol":
		return false, s.documentSymbols(request)
	case "textDocument/foldingRange":
		return false, s.foldingRanges(request)
	case "textDocument/semanticTokens/full":
		return false, s.semanticTokens(request)
	case "workspace/symbol":
		return false, s.workspaceSymbols(request)
	case "textDocument/formatting":
		return false, s.format(request)
	case "textDocument/codeAction":
		return false, s.codeActions(request)
	default:
		if len(request.ID) == 0 {
			return false, nil
		}
		return false, s.stream.write(failure(request.ID, -32601, fmt.Errorf("method %s is not supported", request.Method)))
	}
}

func (s *Server) workspaceSymbols(request message) error {
	params, err := decodeParams[workspaceSymbolParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	query := strings.ToLower(params.Query)
	result := []symbolInformation{}
	for _, document := range s.semanticDocuments() {
		source := []byte(document.Source)
		uri := uriFromPath(document.Path)
		for _, item := range languageservice.DocumentSymbols(document.Source) {
			appendWorkspaceSymbols(&result, source, uri, query, "", item)
		}
	}
	sort.SliceStable(result, func(left, right int) bool {
		leftName := strings.ToLower(result[left].Name)
		rightName := strings.ToLower(result[right].Name)
		if leftName != rightName {
			return leftName < rightName
		}
		if result[left].Location.URI != result[right].Location.URI {
			return result[left].Location.URI < result[right].Location.URI
		}
		leftStart := result[left].Location.Range.Start
		rightStart := result[right].Location.Range.Start
		return leftStart.Line < rightStart.Line || leftStart.Line == rightStart.Line && leftStart.Character < rightStart.Character
	})
	return s.stream.write(success(request.ID, result))
}

func appendWorkspaceSymbols(result *[]symbolInformation, source []byte, uri, query, container string, item languageservice.DocumentSymbol) {
	if query == "" || strings.Contains(strings.ToLower(item.Name), query) {
		*result = append(*result, symbolInformation{
			Name: item.Name, Kind: documentSymbolKind(item.Kind), ContainerName: container,
			Location: location{URI: uri, Range: offsetRange(source, item.SelectionRange.Start, item.SelectionRange.End)},
		})
	}
	childContainer := item.Name
	if container != "" {
		childContainer = container + "." + item.Name
	}
	for _, child := range item.Children {
		appendWorkspaceSymbols(result, source, uri, query, childContainer, child)
	}
}

var semanticTokenTypes = []string{"type", "variable", "string", "number", "comment", "function", "method", "keyword"}
var semanticTokenModifiers = []string{"readonly"}

func (s *Server) semanticTokens(request message) error {
	params, err := decodeParams[documentParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, semanticTokens{Data: []int{}}))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	spans := languageservice.Highlight(languageservice.HighlightRequest{
		Source: string(document.source), Mode: s.mode, Context: context,
	})
	return s.stream.write(success(request.ID, semanticTokens{Data: semanticTokenData(document.source, spans)}))
}

func semanticTokenData(source []byte, spans []languageservice.HighlightSpan) []int {
	data := []int{}
	previousLine := 0
	previousStart := 0
	for _, span := range spans {
		typeIndex, modifiers, ok := semanticTokenClassification(span.Kind)
		if !ok {
			continue
		}
		for _, segment := range semanticTokenSegments(source, span.Range) {
			length := segment.End.Character - segment.Start.Character
			if length <= 0 {
				continue
			}
			deltaLine := segment.Start.Line - previousLine
			deltaStart := segment.Start.Character
			if deltaLine == 0 {
				deltaStart -= previousStart
			}
			data = append(data, deltaLine, deltaStart, length, typeIndex, modifiers)
			previousLine = segment.Start.Line
			previousStart = segment.Start.Character
		}
	}
	return data
}

func semanticTokenClassification(kind languageservice.HighlightKind) (typeIndex, modifiers int, ok bool) {
	switch kind {
	case languageservice.HighlightType:
		return 0, 0, true
	case languageservice.HighlightConstant:
		return 1, 1, true
	case languageservice.HighlightString:
		return 2, 0, true
	case languageservice.HighlightNumber:
		return 3, 0, true
	case languageservice.HighlightComment:
		return 4, 0, true
	case languageservice.HighlightFunction:
		return 5, 0, true
	case languageservice.HighlightMethod:
		return 6, 0, true
	case languageservice.HighlightKeyword, languageservice.HighlightBoolean:
		return 7, 0, true
	default:
		return 0, 0, false
	}
}

func semanticTokenSegments(source []byte, range_ languageservice.OffsetRange) []rangeValue {
	start := max(range_.Start, 0)
	end := min(range_.End, len(source))
	segments := []rangeValue{}
	for start < end {
		lineEnd := end
		if relative := bytes.IndexByte(source[start:end], '\n'); relative >= 0 {
			lineEnd = start + relative
		}
		contentEnd := lineEnd
		if contentEnd > start && source[contentEnd-1] == '\r' {
			contentEnd--
		}
		if contentEnd > start {
			segments = append(segments, offsetRange(source, start, contentEnd))
		}
		if lineEnd == end {
			break
		}
		start = lineEnd + 1
	}
	return segments
}

func (s *Server) documentSymbols(request message) error {
	params, err := decodeParams[documentParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []documentSymbol{}))
	}
	items := languageservice.DocumentSymbols(string(document.source))
	result := make([]documentSymbol, 0, len(items))
	for _, item := range items {
		result = append(result, protocolDocumentSymbol(document.source, item))
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) foldingRanges(request message) error {
	params, err := decodeParams[documentParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []foldingRange{}))
	}
	items := languageservice.FoldingRanges(string(document.source))
	result := make([]foldingRange, 0, len(items))
	for _, item := range items {
		start := positionAt(document.source, item.Range.Start)
		end := positionAt(document.source, item.Range.End)
		if end.Line <= start.Line {
			continue
		}
		result = append(result, foldingRange{
			StartLine: start.Line, StartCharacter: start.Character,
			EndLine: end.Line, EndCharacter: end.Character,
		})
	}
	return s.stream.write(success(request.ID, result))
}

func protocolDocumentSymbol(source []byte, item languageservice.DocumentSymbol) documentSymbol {
	children := make([]documentSymbol, 0, len(item.Children))
	for _, child := range item.Children {
		children = append(children, protocolDocumentSymbol(source, child))
	}
	return documentSymbol{
		Name: item.Name, Detail: item.Detail, Kind: documentSymbolKind(item.Kind),
		Range:          offsetRange(source, item.Range.Start, item.Range.End),
		SelectionRange: offsetRange(source, item.SelectionRange.Start, item.SelectionRange.End),
		Children:       children,
	}
}

func documentSymbolKind(kind languageservice.DocumentSymbolKind) int {
	// LSP SymbolKind values are stable protocol constants.
	switch kind {
	case languageservice.DocumentSymbolModule:
		return 2
	case languageservice.DocumentSymbolClass:
		return 5
	case languageservice.DocumentSymbolMethod:
		return 6
	case languageservice.DocumentSymbolField:
		return 8
	case languageservice.DocumentSymbolEnum:
		return 10
	case languageservice.DocumentSymbolInterface:
		return 11
	case languageservice.DocumentSymbolFunction:
		return 12
	case languageservice.DocumentSymbolVariable:
		return 13
	case languageservice.DocumentSymbolConstant:
		return 14
	case languageservice.DocumentSymbolEnumMember:
		return 22
	case languageservice.DocumentSymbolRecord:
		return 23
	case languageservice.DocumentSymbolType:
		return 26
	default:
		return 13
	}
}

func (s *Server) open(item textDocumentItem) error {
	path, err := pathFromURI(item.URI)
	if err != nil {
		return err
	}
	unit, err := s.unit(path, []byte(item.Text))
	if err != nil {
		return s.showError(err)
	}
	s.documents[path] = document{unit: unit, source: []byte(item.Text), version: item.Version}
	s.compiler.SetDocument(unit)
	return s.publish()
}

func (s *Server) change(params didChangeParams) error {
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return err
	}
	current, exists := s.documents[path]
	if !exists {
		return s.showError(fmt.Errorf("cannot change unopened document %s", path))
	}
	if len(params.ContentChanges) == 0 {
		current.version = params.TextDocument.Version
		s.documents[path] = current
		return nil
	}
	if current.version > 0 && params.TextDocument.Version > 0 && params.TextDocument.Version <= current.version {
		return nil
	}
	source, err := applyContentChanges(current.source, params.ContentChanges)
	if err != nil {
		return s.showError(fmt.Errorf("cannot apply changes to %s: %w", path, err))
	}
	unit, err := s.unit(path, source)
	if err != nil {
		return s.showError(err)
	}
	s.documents[path] = document{unit: unit, source: source, version: params.TextDocument.Version}
	s.compiler.SetDocument(unit)
	return s.publish()
}

func applyContentChanges(source []byte, changes []contentChange) ([]byte, error) {
	result := append([]byte(nil), source...)
	for _, change := range changes {
		if change.Range == nil {
			result = []byte(change.Text)
			continue
		}
		start, ok := exactOffsetAt(result, change.Range.Start)
		if !ok {
			return nil, fmt.Errorf("change start %d:%d is outside the document", change.Range.Start.Line, change.Range.Start.Character)
		}
		end, ok := exactOffsetAt(result, change.Range.End)
		if !ok {
			return nil, fmt.Errorf("change end %d:%d is outside the document", change.Range.End.Line, change.Range.End.Character)
		}
		if end < start {
			return nil, fmt.Errorf("change range ends before it starts")
		}
		next := make([]byte, 0, len(result)-(end-start)+len(change.Text))
		next = append(next, result[:start]...)
		next = append(next, change.Text...)
		next = append(next, result[end:]...)
		result = next
	}
	return result, nil
}

func (s *Server) close(uri string) error {
	path, err := pathFromURI(uri)
	if err != nil {
		return err
	}
	delete(s.documents, path)
	s.compiler.CloseDocument(path)
	return s.publish()
}

func (s *Server) completion(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []completionItem{}))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	cursor := offsetAt(document.source, params.Position)
	items := languageservice.Complete(languageservice.CompletionRequest{
		Source: string(document.source), Cursor: cursor, Mode: s.mode, Context: context,
	})
	result := make([]completionItem, 0, len(items))
	for _, item := range items {
		result = append(result, completionItem{
			Label: item.Label, Kind: completionKind(item.Kind), Detail: item.Detail,
			TextEdit: textEdit{Range: offsetRange(document.source, item.Replacement.Start, item.Replacement.End), NewText: item.InsertText},
		})
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) hover(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	info, ok := languageservice.Hover(languageservice.SemanticRequest{
		Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	})
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	result := hoverResult{
		Contents: markupContent{Kind: "markdown", Value: "```trb\n" + info.Detail + "\n```"},
		Range:    offsetRange(document.source, info.Range.Start, info.Range.End),
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) signatureHelp(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	info, ok := languageservice.Signatures(languageservice.SemanticRequest{
		Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	})
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	result := signatureHelpResult{
		Signatures:      make([]signatureInformation, 0, len(info.Signatures)),
		ActiveSignature: info.ActiveSignature, ActiveParameter: info.ActiveParameter,
	}
	for _, signature := range info.Signatures {
		converted := signatureInformation{Label: signature.Label, Parameters: make([]parameterInformation, 0, len(signature.Parameters))}
		for _, parameter := range signature.Parameters {
			converted.Parameters = append(converted.Parameters, parameterInformation{Label: parameter.Label})
		}
		result.Signatures = append(result.Signatures, converted)
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) definition(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	info, ok := languageservice.Definition(languageservice.SemanticRequest{
		Path: path, Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	})
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	targetPath := cleanPath(info.Path)
	targetSource, ok := s.source(targetPath)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	range_ := refineDefinitionRange(targetSource, info)
	return s.stream.write(success(request.ID, location{URI: uriFromPath(targetPath), Range: range_}))
}

func (s *Server) references(request message) error {
	params, err := decodeParams[referenceParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []location{}))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	items, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	}, s.semanticDocuments(), params.Context.IncludeDeclaration)
	if !ok {
		return s.stream.write(success(request.ID, []location{}))
	}
	result := make([]location, 0, len(items))
	for _, item := range items {
		source, exists := s.source(item.Path)
		if !exists {
			continue
		}
		result = append(result, location{URI: uriFromPath(item.Path), Range: offsetRange(source, item.Range.Start, item.Range.End)})
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) documentHighlights(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []documentHighlight{}))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	items, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	}, s.semanticDocuments(), true)
	if !ok {
		return s.stream.write(success(request.ID, []documentHighlight{}))
	}
	result := []documentHighlight{}
	for _, item := range items {
		if cleanPath(item.Path) != path {
			continue
		}
		result = append(result, documentHighlight{
			Range: offsetRange(document.source, item.Range.Start, item.Range.End),
			Kind:  1,
		})
	}
	return s.stream.write(success(request.ID, result))
}

func (s *Server) prepareRename(request message) error {
	params, err := decodeParams[documentPositionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	cursor := offsetAt(document.source, params.Position)
	items, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: string(document.source), Cursor: cursor, Mode: s.mode, Context: context,
	}, s.semanticDocuments(), true)
	if !ok || len(items) == 0 {
		return s.stream.write(success(request.ID, nil))
	}
	tokens, _ := lexer.Lex(document.source)
	item, ok := semanticTokenAtOffset(tokens, cursor)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	range_ := identifierProtocolRange(document.source, item)
	return s.stream.write(success(request.ID, prepareRenameResult{Range: range_, Placeholder: strings.TrimPrefix(item.Lexeme, "@")}))
}

func (s *Server) rename(request message) error {
	params, err := decodeParams[renameParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	if !validRenameIdentifier(params.NewName) {
		return s.stream.write(failure(request.ID, -32602, fmt.Errorf("%q is not a valid TypeRB identifier", params.NewName)))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, nil))
	}
	context, _ := s.snapshot.Context(document.unit.ModulePath)
	items, ok := languageservice.References(languageservice.SemanticRequest{
		Path: path, Source: string(document.source), Cursor: offsetAt(document.source, params.Position), Mode: s.mode, Context: context,
	}, s.semanticDocuments(), true)
	if !ok || len(items) == 0 {
		return s.stream.write(success(request.ID, nil))
	}
	changes := map[string][]textEdit{}
	for _, item := range items {
		source, exists := s.source(item.Path)
		if !exists {
			continue
		}
		uri := uriFromPath(item.Path)
		changes[uri] = append(changes[uri], textEdit{Range: offsetRange(source, item.Range.Start, item.Range.End), NewText: params.NewName})
	}
	return s.stream.write(success(request.ID, workspaceEdit{Changes: changes}))
}

func (s *Server) format(request message) error {
	params, err := decodeParams[formattingParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []textEdit{}))
	}
	formatted, diagnostics := formatter.Format(document.source)
	if hasErrors(diagnostics) || string(formatted) == string(document.source) {
		return s.stream.write(success(request.ID, []textEdit{}))
	}
	edit := textEdit{Range: offsetRange(document.source, 0, len(document.source)), NewText: string(formatted)}
	return s.stream.write(success(request.ID, []textEdit{edit}))
}

func (s *Server) codeActions(request message) error {
	params, err := decodeParams[codeActionParams](request.Params)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	path, err := pathFromURI(params.TextDocument.URI)
	if err != nil {
		return s.stream.write(failure(request.ID, -32602, err))
	}
	document, ok := s.document(path)
	if !ok {
		return s.stream.write(success(request.ID, []codeAction{}))
	}
	requestStart := offsetAt(document.source, params.Range.Start)
	requestEnd := offsetAt(document.source, params.Range.End)
	actions := []codeAction{}
	for _, item := range s.snapshot.Diagnostics {
		if cleanPath(item.Path) != path || !overlaps(item.Span.Start.Offset, item.Span.End.Offset, requestStart, requestEnd) {
			continue
		}
		for _, fix := range item.Fixes {
			changes := map[string][]textEdit{}
			for _, edit := range fix.Edits {
				editPath := cleanPath(edit.Location.Path)
				source, exists := s.source(editPath)
				if !exists {
					continue
				}
				uri := uriFromPath(editPath)
				changes[uri] = append(changes[uri], textEdit{
					Range: sourceSpan(source, edit.Location.Span), NewText: edit.Replacement,
				})
			}
			if len(changes) > 0 {
				actions = append(actions, codeAction{Title: fix.Message, Kind: "quickfix", Edit: workspaceEdit{Changes: changes}})
			}
		}
	}
	return s.stream.write(success(request.ID, actions))
}

func (s *Server) publish() error {
	s.snapshot = s.compiler.Analyze()
	grouped := map[string][]diagnostic.Diagnostic{}
	for _, item := range s.snapshot.Diagnostics {
		if item.Path == "" {
			if err := s.showError(errors.New(item.Message)); err != nil {
				return err
			}
			continue
		}
		path := cleanPath(item.Path)
		grouped[path] = append(grouped[path], item)
	}
	targets := map[string]bool{}
	for path := range s.published {
		targets[path] = true
	}
	for path := range grouped {
		targets[path] = true
	}
	for path := range s.documents {
		targets[path] = true
	}
	paths := make([]string, 0, len(targets))
	for path := range targets {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		source, _ := s.source(path)
		items := make([]protocolDiagnostic, 0, len(grouped[path]))
		for _, item := range grouped[path] {
			items = append(items, s.protocolDiagnostic(source, item))
		}
		if err := s.stream.write(notification{JSONRPC: "2.0", Method: "textDocument/publishDiagnostics", Params: publishDiagnosticsParams{
			URI: uriFromPath(path), Diagnostics: items,
		}}); err != nil {
			return err
		}
		if len(items) == 0 {
			delete(s.published, path)
		} else {
			s.published[path] = true
		}
	}
	return nil
}

func (s *Server) protocolDiagnostic(source []byte, item diagnostic.Diagnostic) protocolDiagnostic {
	severity := 1
	if item.Severity == diagnostic.Warning {
		severity = 2
	}
	result := protocolDiagnostic{
		Range: sourceSpan(source, item.Span), Severity: severity,
		Code: string(item.Code), Source: "trb", Message: item.Message,
	}
	for _, related := range item.Related {
		relatedPath := cleanPath(related.Location.Path)
		relatedSource, _ := s.source(relatedPath)
		result.RelatedInformation = append(result.RelatedInformation, relatedInformation{
			Location: location{URI: uriFromPath(relatedPath), Range: sourceSpan(relatedSource, related.Location.Span)},
			Message:  related.Message,
		})
	}
	if len(item.Fixes) > 0 {
		result.Data = map[string]interface{}{"fixes": len(item.Fixes)}
	}
	return result
}

func (s *Server) unit(path string, source []byte) (compiler.SourceUnit, error) {
	path = cleanPath(path)
	if document, exists := s.documents[path]; exists {
		unit := document.unit
		unit.Source = append([]byte(nil), source...)
		return unit, nil
	}
	if unit, exists := s.base[path]; exists {
		unit.Source = append([]byte(nil), source...)
		return unit, nil
	}
	if s.resolveUnit == nil {
		return compiler.SourceUnit{}, fmt.Errorf("cannot derive a TypeRB module for %s", path)
	}
	return s.resolveUnit(path, source)
}

func (s *Server) document(path string) (document, bool) {
	path = cleanPath(path)
	if current, exists := s.documents[path]; exists {
		return current, true
	}
	unit, exists := s.base[path]
	if !exists {
		return document{}, false
	}
	return document{unit: unit, source: unit.Source}, true
}

func (s *Server) source(path string) ([]byte, bool) {
	document, exists := s.document(path)
	return document.source, exists
}

func (s *Server) semanticDocuments() []languageservice.SemanticDocument {
	paths := make(map[string]bool, len(s.base)+len(s.documents))
	for path := range s.base {
		paths[path] = true
	}
	for path := range s.documents {
		paths[path] = true
	}
	ordered := make([]string, 0, len(paths))
	for path := range paths {
		ordered = append(ordered, path)
	}
	sort.Strings(ordered)
	result := make([]languageservice.SemanticDocument, 0, len(ordered))
	for _, path := range ordered {
		document, exists := s.document(path)
		if !exists {
			continue
		}
		context, _ := s.snapshot.Context(document.unit.ModulePath)
		result = append(result, languageservice.SemanticDocument{
			Path: path, Source: string(document.source), Mode: s.mode, Context: context,
		})
	}
	return result
}

func (s *Server) showError(err error) error {
	return s.stream.write(notification{JSONRPC: "2.0", Method: "window/showMessage", Params: map[string]interface{}{"type": 1, "message": err.Error()}})
}

func pathFromURI(value string) (string, error) {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" {
		return "", fmt.Errorf("TypeRB LSP requires a file URI; got %q", value)
	}
	path, err := url.PathUnescape(parsed.Path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "windows" && len(path) >= 3 && path[0] == '/' && path[2] == ':' {
		path = path[1:]
	}
	return cleanPath(filepath.FromSlash(path)), nil
}

func uriFromPath(path string) string {
	path = cleanPath(path)
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func cleanPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		return filepath.Clean(absolute)
	}
	return filepath.Clean(path)
}

func offsetAt(source []byte, target position) int {
	line, character, offset := 0, 0, 0
	for offset < len(source) {
		if line == target.Line && character >= target.Character {
			return offset
		}
		r, size := utf8.DecodeRune(source[offset:])
		if r == '\n' {
			if line == target.Line {
				return offset
			}
			line++
			character = 0
			offset += size
			continue
		}
		if line == target.Line {
			character += utf16.RuneLen(r)
		}
		offset += size
	}
	return len(source)
}

func exactOffsetAt(source []byte, target position) (int, bool) {
	if target.Line < 0 || target.Character < 0 {
		return 0, false
	}
	line, character := 0, 0
	for offset := 0; offset < len(source); {
		if line == target.Line && character == target.Character {
			return offset, true
		}
		r, size := utf8.DecodeRune(source[offset:])
		if r == '\n' {
			if line == target.Line {
				return 0, false
			}
			line++
			character = 0
			offset += size
			continue
		}
		if line == target.Line {
			character += utf16.RuneLen(r)
			if character > target.Character {
				return 0, false
			}
		}
		offset += size
	}
	if line == target.Line && character == target.Character {
		return len(source), true
	}
	return 0, false
}

func positionAt(source []byte, target int) position {
	if target < 0 {
		target = 0
	}
	if target > len(source) {
		target = len(source)
	}
	result := position{}
	for offset := 0; offset < target; {
		r, size := utf8.DecodeRune(source[offset:])
		if offset+size > target {
			break
		}
		if r == '\n' {
			result.Line++
			result.Character = 0
		} else {
			result.Character += utf16.RuneLen(r)
		}
		offset += size
	}
	return result
}

func sourceSpan(source []byte, span token.Span) rangeValue {
	if len(source) == 0 {
		return rangeValue{
			Start: position{Line: max(span.Start.Line-1, 0), Character: max(span.Start.Column-1, 0)},
			End:   position{Line: max(span.End.Line-1, 0), Character: max(span.End.Column-1, 0)},
		}
	}
	return offsetRange(source, span.Start.Offset, span.End.Offset)
}

func refineDefinitionRange(source []byte, info languageservice.DefinitionInfo) rangeValue {
	start := max(info.Range.Start, 0)
	end := min(info.Range.End, len(source))
	tokens, _ := lexer.Lex(source)
	for _, item := range tokens {
		if item.Kind != token.Identifier || item.Span.Start.Offset < start || item.Span.End.Offset > end {
			continue
		}
		if sameIdentifierName(item.Lexeme, info.Name) {
			return identifierProtocolRange(source, item)
		}
	}
	return offsetRange(source, start, end)
}

func semanticTokenAtOffset(tokens []token.Token, cursor int) (token.Token, bool) {
	for _, item := range tokens {
		if item.Kind == token.Identifier && item.Span.Start.Offset <= cursor && cursor < item.Span.End.Offset {
			return item, true
		}
	}
	for _, item := range tokens {
		if item.Kind == token.Identifier && item.Span.End.Offset == cursor {
			return item, true
		}
	}
	return token.Token{}, false
}

func identifierProtocolRange(source []byte, item token.Token) rangeValue {
	start := item.Span.Start.Offset
	if strings.HasPrefix(item.Lexeme, "@") {
		start++
	}
	return offsetRange(source, start, item.Span.End.Offset)
}

func sameIdentifierName(left, right string) bool {
	return strings.TrimPrefix(left, "@") == strings.TrimPrefix(right, "@")
}

func validRenameIdentifier(name string) bool {
	if name == "" || strings.HasPrefix(name, "@") || renameReservedWords[name] {
		return false
	}
	tokens, diagnostics := lexer.Lex([]byte(name))
	return len(diagnostics) == 0 && len(tokens) == 2 && tokens[0].Kind == token.Identifier &&
		tokens[0].Lexeme == name && tokens[0].Span.Start.Offset == 0 && tokens[0].Span.End.Offset == len(name)
}

var renameReservedWords = map[string]bool{
	"and": true, "attempt": true, "begin": true, "break": true, "case": true,
	"class": true, "def": true, "defer": true, "do": true, "else": true,
	"elsif": true, "end": true, "enum": true, "false": true, "fails": true,
	"fn": true, "for": true, "if": true, "implements": true, "import": true,
	"interface": true, "module": true, "mut": true, "next": true, "nil": true,
	"not": true, "or": true, "readonly": true, "record": true, "return": true,
	"self": true, "then": true, "true": true, "try": true, "type": true,
	"unless": true, "until": true, "when": true, "while": true,
}

func offsetRange(source []byte, start, end int) rangeValue {
	return rangeValue{Start: positionAt(source, start), End: positionAt(source, end)}
}

func completionKind(kind languageservice.CompletionKind) int {
	switch kind {
	case languageservice.CompletionMethod:
		return 2
	case languageservice.CompletionFunction:
		return 3
	case languageservice.CompletionField:
		return 5
	case languageservice.CompletionVariable:
		return 6
	case languageservice.CompletionType:
		return 7
	case languageservice.CompletionModule:
		return 9
	case languageservice.CompletionConstant, languageservice.CompletionEnumMember, languageservice.CompletionValue:
		return 21
	case languageservice.CompletionKeyword:
		return 14
	default:
		return 1
	}
}

func overlaps(leftStart, leftEnd, rightStart, rightEnd int) bool {
	if leftEnd == leftStart {
		leftEnd++
	}
	if rightEnd == rightStart {
		rightEnd++
	}
	return leftStart < rightEnd && rightStart < leftEnd
}

func hasErrors(items []diagnostic.Diagnostic) bool {
	for _, item := range items {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}
