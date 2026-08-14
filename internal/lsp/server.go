// Package lsp adapts the shared compiler and language services to the Language
// Server Protocol. Protocol transport remains separate from project discovery.
package lsp

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"path/filepath"
	"runtime"
	"sort"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/languageservice"
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
				TextDocumentSync:           1,
				CompletionProvider:         completionOptions{TriggerCharacters: []string{".", ":"}},
				HoverProvider:              true,
				SignatureHelpProvider:      signatureOptions{TriggerCharacters: []string{"(", ","}},
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
	if len(params.ContentChanges) == 0 {
		return nil
	}
	if current, exists := s.documents[path]; exists && current.version > 0 && params.TextDocument.Version > 0 && params.TextDocument.Version <= current.version {
		return nil
	}
	source := []byte(params.ContentChanges[len(params.ContentChanges)-1].Text)
	unit, err := s.unit(path, source)
	if err != nil {
		return s.showError(err)
	}
	s.documents[path] = document{unit: unit, source: source, version: params.TextDocument.Version}
	s.compiler.SetDocument(unit)
	return s.publish()
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
