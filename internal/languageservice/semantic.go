package languageservice

import (
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

// Hover resolves the checked symbol at a source position without exposing any
// editor-protocol representation to the compiler service.
func Hover(request SemanticRequest) (HoverInfo, bool) {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return HoverInfo{}, false
	}
	tokens, _ := lexer.Lex([]byte(request.Source))
	item, ok := semanticTokenAt(tokens, request.Cursor)
	if !ok {
		return HoverInfo{}, false
	}
	range_ := OffsetRange{Start: item.Span.Start.Offset, End: item.Span.End.Offset}
	switch item.Kind {
	case token.String:
		return HoverInfo{Range: range_, Detail: "String"}, true
	case token.Number:
		detail := "Integer"
		if strings.Contains(item.Lexeme, ".") {
			detail = "Float"
		}
		return HoverInfo{Range: range_, Detail: detail}, true
	case token.Identifier:
		if item.Lexeme == "true" || item.Lexeme == "false" {
			return HoverInfo{Range: range_, Detail: "Boolean"}, true
		}
		if item.Lexeme == "nil" {
			return HoverInfo{Range: range_, Detail: "nil"}, true
		}
	default:
		return HoverInfo{}, false
	}

	analysisCursor := semanticLineEnd(request.Source, item.Span.End.Offset)
	completionRequest := CompletionRequest{
		Source: request.Source, Cursor: analysisCursor, Mode: request.Mode, Context: request.Context,
	}
	if marker, receiver := memberReceiver(request.Source, item.Span.Start.Offset); marker != "" {
		members := completeMembers(receiver, marker, completionRequest, range_)
		for _, member := range members {
			if member.Label == item.Lexeme {
				detail := member.Detail
				if member.Kind == CompletionField && !strings.HasPrefix(detail, member.Label+":") {
					detail = member.Label + ": " + detail
				}
				return HoverInfo{Range: range_, Detail: detail}, true
			}
		}
		return HoverInfo{}, false
	}

	if symbol, found := semanticSymbol(item.Lexeme, request.Source, analysisCursor, request.Context); found {
		return HoverInfo{Range: range_, Detail: semanticDetail(symbol)}, true
	}
	return HoverInfo{}, false
}

// Signatures resolves the innermost call and its active argument using the
// same structured call metadata that powers completion.
func Signatures(request SemanticRequest) (SignatureHelp, bool) {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return SignatureHelp{}, false
	}
	tokens, _ := lexer.Lex([]byte(request.Source[:request.Cursor]))
	significant := completionTokens(tokens)
	open := innermostOpenCall(significant)
	if open < 1 {
		return SignatureHelp{}, false
	}
	call, ok := completionCallSymbol(significant, open, CompletionRequest{
		Source: request.Source, Cursor: request.Cursor, Mode: request.Mode, Context: request.Context,
	})
	if !ok || call.Call == nil {
		return SignatureHelp{}, false
	}

	parameters := make([]SignatureParameter, 0, len(call.Call.Parameters))
	for _, parameter := range call.Call.Parameters {
		label := parameter.Label
		if label == "" {
			label = parameter.Name
		}
		if label != "" {
			parameters = append(parameters, SignatureParameter{Label: label})
		}
	}
	arguments := splitCompletionArguments(significant[open+1:])
	active := activeCallParameter(call.Call, arguments)
	if len(parameters) > 0 && active >= len(parameters) {
		active = len(parameters) - 1
	}
	return SignatureHelp{
		Signatures:      []SignatureInfo{{Label: call.Detail, Parameters: parameters}},
		ActiveSignature: 0,
		ActiveParameter: active,
	}, true
}

// Definition resolves project declarations, imported declarations, receiver
// members, and common lexical bindings to their TypeRB source location.
func Definition(request SemanticRequest) (DefinitionInfo, bool) {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return DefinitionInfo{}, false
	}
	tokens, _ := lexer.Lex([]byte(request.Source))
	item, ok := semanticTokenAt(tokens, request.Cursor)
	if !ok || item.Kind != token.Identifier {
		return DefinitionInfo{}, false
	}
	analysisCursor := semanticLineEnd(request.Source, item.Span.End.Offset)
	completionRequest := CompletionRequest{
		Source: request.Source, Cursor: analysisCursor, Mode: request.Mode, Context: request.Context,
	}
	if marker, receiver := memberReceiver(request.Source, item.Span.Start.Offset); marker != "" {
		for _, member := range memberSymbols(receiver, marker, completionRequest) {
			if member.Name == item.Lexeme && member.Definition != nil {
				return definitionInfo(member.Definition), true
			}
		}
		return DefinitionInfo{}, false
	}
	if definition, found := callArgumentDefinition(request, item); found {
		return definitionInfo(definition), true
	}
	if symbol, found := semanticSymbol(item.Lexeme, request.Source, analysisCursor, request.Context); found && symbol.Definition != nil {
		return definitionInfo(symbol.Definition), true
	}
	if definition, found := declarationAt(request.Context, request.Path, tokens, item); found {
		return definitionInfo(definition), true
	}
	if definition, found := lexicalDefinition(request.Path, tokens, item); found {
		return definitionInfo(definition), true
	}
	if definition, found := enclosingTypeMemberDefinition(request.Context, request.Path, item); found {
		return definitionInfo(definition), true
	}
	return DefinitionInfo{}, false
}

func enclosingTypeMemberDefinition(context Context, path string, item token.Token) (*DefinitionLocation, bool) {
	var owner *Symbol
	for index := range context.Symbols {
		symbol := &context.Symbols[index]
		definition := symbol.Definition
		if symbol.Kind != CompletionType || definition == nil || definition.Path != path ||
			item.Span.Start.Offset < definition.Range.Start || definition.Range.End < item.Span.End.Offset {
			continue
		}
		if owner == nil || definition.Range.End-definition.Range.Start < owner.Definition.Range.End-owner.Definition.Range.Start {
			owner = symbol
		}
	}
	if owner == nil {
		return nil, false
	}
	members := append([]Symbol(nil), owner.Members...)
	members = append(members, context.TypeMembers[owner.Name]...)
	if owner.Type.Kind != "" {
		members = append(members, context.TypeMembers[owner.Type.String()]...)
	}
	for _, member := range members {
		if member.Name == item.Lexeme && member.Definition != nil {
			return member.Definition, true
		}
	}
	return nil, false
}

func callArgumentDefinition(request SemanticRequest, item token.Token) (*DefinitionLocation, bool) {
	tokens, _ := lexer.Lex([]byte(request.Source[:item.Span.End.Offset]))
	significant := completionTokens(tokens)
	if len(significant) == 0 || significant[len(significant)-1].Lexeme != item.Lexeme {
		return nil, false
	}
	open := innermostOpenCall(significant)
	if open < 1 {
		return nil, false
	}
	call, ok := completionCallSymbol(significant, open, CompletionRequest{
		Source: request.Source, Cursor: item.Span.End.Offset, Mode: request.Mode, Context: request.Context,
	})
	if !ok || call.Call == nil {
		return nil, false
	}
	for _, parameter := range call.Call.Parameters {
		if parameter.Keyword && parameter.Name == item.Lexeme && parameter.Definition != nil {
			return parameter.Definition, true
		}
	}
	return nil, false
}

// References resolves only occurrences that bind to the same checked source
// declaration. Textually equal names owned by another type or lexical binding
// are intentionally excluded.
func References(request SemanticRequest, documents []SemanticDocument, includeDeclaration bool) ([]ReferenceInfo, bool) {
	tokens, _ := lexer.Lex([]byte(request.Source))
	origin, ok := semanticTokenAt(tokens, request.Cursor)
	if !ok || origin.Kind != token.Identifier {
		return nil, false
	}
	target, ok := Definition(request)
	if !ok || !sameDefinitionName(origin.Lexeme, target.Name) {
		return nil, false
	}

	declarationRange := OffsetRange{Start: target.Range.Start, End: target.Range.End}
	for _, document := range documents {
		if document.Path == target.Path {
			declarationRange = exactDefinitionRange(document.Source, target)
			break
		}
	}

	result := []ReferenceInfo{}
	seen := map[string]bool{}
	for _, document := range documents {
		documentTokens, _ := lexer.Lex([]byte(document.Source))
		for _, item := range documentTokens {
			if item.Kind != token.Identifier || !sameDefinitionName(item.Lexeme, target.Name) {
				continue
			}
			resolved, found := Definition(SemanticRequest{
				Path: document.Path, Source: document.Source, Cursor: item.Span.Start.Offset,
				Mode: document.Mode, Context: document.Context,
			})
			if !found || resolved.ID != target.ID {
				continue
			}
			range_ := identifierRange(item, target.Name)
			declaration := document.Path == target.Path && range_ == declarationRange
			if declaration && !includeDeclaration {
				continue
			}
			key := document.Path + "#" + offsetKey(range_)
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, ReferenceInfo{ID: target.ID, Path: document.Path, Range: range_, Declaration: declaration})
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].Path != result[right].Path {
			return result[left].Path < result[right].Path
		}
		return result[left].Range.Start < result[right].Range.Start
	})
	return result, true
}

func semanticTokenAt(tokens []token.Token, cursor int) (token.Token, bool) {
	for _, item := range tokens {
		if item.Kind == token.Comment || item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		if item.Span.Start.Offset <= cursor && cursor < item.Span.End.Offset {
			return item, true
		}
	}
	for _, item := range tokens {
		if item.Kind != token.Comment && item.Kind != token.Newline && item.Kind != token.EOF && item.Span.End.Offset == cursor {
			return item, true
		}
	}
	return token.Token{}, false
}

func definitionInfo(location *DefinitionLocation) DefinitionInfo {
	return DefinitionInfo{ID: location.ID, Name: location.Name, Path: location.Path, Range: location.Range}
}

func declarationAt(context Context, path string, tokens []token.Token, item token.Token) (*DefinitionLocation, bool) {
	var best *DefinitionLocation
	var consider func([]Symbol)
	consider = func(symbols []Symbol) {
		for _, symbol := range symbols {
			definition := symbol.Definition
			if definition != nil && definition.Path == path &&
				exactDefinitionRangeFromTokens(tokens, definitionInfo(definition)) == identifierRange(item, definition.Name) {
				if best == nil || definition.Range.End-definition.Range.Start < best.Range.End-best.Range.Start {
					best = definition
				}
			}
			consider(symbol.Members)
		}
	}
	consider(context.Symbols)
	for _, members := range context.TypeMembers {
		consider(members)
	}
	return best, best != nil
}

func exactDefinitionRange(source string, definition DefinitionInfo) OffsetRange {
	tokens, _ := lexer.Lex([]byte(source))
	return exactDefinitionRangeFromTokens(tokens, definition)
}

func exactDefinitionRangeFromTokens(tokens []token.Token, definition DefinitionInfo) OffsetRange {
	start := max(definition.Range.Start, 0)
	end := definition.Range.End
	for _, item := range tokens {
		if item.Kind == token.Identifier && start <= item.Span.Start.Offset && item.Span.End.Offset <= end && sameDefinitionName(item.Lexeme, definition.Name) {
			return identifierRange(item, definition.Name)
		}
	}
	return OffsetRange{Start: start, End: end}
}

func identifierRange(item token.Token, definitionName string) OffsetRange {
	start := item.Span.Start.Offset
	if strings.HasPrefix(item.Lexeme, "@") && !strings.HasPrefix(definitionName, "@") {
		start++
	}
	return OffsetRange{Start: start, End: item.Span.End.Offset}
}

func sameDefinitionName(sourceName, definitionName string) bool {
	return strings.TrimPrefix(sourceName, "@") == strings.TrimPrefix(definitionName, "@")
}

func offsetKey(range_ OffsetRange) string {
	return fmt.Sprintf("%d:%d", range_.Start, range_.End)
}

func lexicalDefinition(path string, tokens []token.Token, reference token.Token) (*DefinitionLocation, bool) {
	if path == "" {
		return nil, false
	}
	scopes := lexicalScopeRanges(tokens)
	var found *DefinitionLocation
	foundScopeSize := 0
	for index, item := range tokens {
		if item.Span.Start.Offset > reference.Span.Start.Offset {
			break
		}
		if item.Kind != token.Identifier || item.Lexeme != reference.Lexeme {
			continue
		}
		if !lexicalAssignmentDeclaration(tokens, index) && !lexicalParameterDeclaration(tokens, index) && !lexicalBlockDeclaration(tokens, index) {
			continue
		}
		scope := innermostLexicalScope(scopes, item.Span.Start.Offset)
		if reference.Span.Start.Offset < scope.Start || scope.End < reference.Span.End.Offset {
			continue
		}
		scopeSize := scope.End - scope.Start
		if found == nil || scopeSize < foundScopeSize || scopeSize == foundScopeSize && found.Range.Start < item.Span.Start.Offset {
			found = sourceDefinition(path, item.Lexeme, item.Span)
			foundScopeSize = scopeSize
		}
	}
	return found, found != nil
}

type lexicalScopeRange struct {
	Start int
	End   int
}

type lexicalScopeFrame struct {
	Close string
	Scope int
}

func lexicalScopeRanges(tokens []token.Token) []lexicalScopeRange {
	end := 0
	if len(tokens) > 0 {
		end = tokens[len(tokens)-1].Span.End.Offset
	}
	scopes := []lexicalScopeRange{{Start: 0, End: end}}
	frames := []lexicalScopeFrame{}
	for index, item := range tokens {
		if item.Kind == token.Identifier && lexicalEndBlockOpener(item.Lexeme) {
			scopes = append(scopes, lexicalScopeRange{Start: item.Span.Start.Offset, End: end})
			frames = append(frames, lexicalScopeFrame{Close: "end", Scope: len(scopes) - 1})
			continue
		}
		if item.Lexeme == "{" {
			scopeIndex := -1
			if lexicalBraceBlock(tokens, index) {
				scopes = append(scopes, lexicalScopeRange{Start: item.Span.Start.Offset, End: end})
				scopeIndex = len(scopes) - 1
			}
			frames = append(frames, lexicalScopeFrame{Close: "}", Scope: scopeIndex})
			continue
		}
		if len(frames) == 0 || item.Lexeme != frames[len(frames)-1].Close {
			continue
		}
		frame := frames[len(frames)-1]
		frames = frames[:len(frames)-1]
		if frame.Scope >= 0 {
			scopes[frame.Scope].End = item.Span.End.Offset
		}
	}
	return scopes
}

func lexicalEndBlockOpener(name string) bool {
	switch name {
	case "case", "class", "def", "do", "enum", "fn", "if", "interface", "module", "record", "while":
		return true
	default:
		return false
	}
}

func lexicalBraceBlock(tokens []token.Token, index int) bool {
	next := nextSemanticToken(tokens, index+1)
	return next >= 0 && tokens[next].Lexeme == "|"
}

func innermostLexicalScope(scopes []lexicalScopeRange, offset int) lexicalScopeRange {
	result := scopes[0]
	for _, scope := range scopes[1:] {
		if scope.Start <= offset && offset <= scope.End && scope.End-scope.Start <= result.End-result.Start {
			result = scope
		}
	}
	return result
}

func lexicalAssignmentDeclaration(tokens []token.Token, index int) bool {
	for next := index + 1; next < len(tokens); next++ {
		if tokens[next].Kind == token.Comment {
			continue
		}
		if tokens[next].Kind == token.Newline || tokens[next].Kind == token.EOF {
			return false
		}
		if tokens[next].Lexeme == ":=" {
			return true
		}
		if tokens[next].Lexeme != ":" && tokens[next].Kind != token.Identifier && tokens[next].Lexeme != "<" && tokens[next].Lexeme != ">" && tokens[next].Lexeme != "?" && tokens[next].Lexeme != "," {
			return false
		}
	}
	return false
}

func lexicalParameterDeclaration(tokens []token.Token, index int) bool {
	if next := nextSemanticToken(tokens, index+1); next < 0 || tokens[next].Lexeme != ":" {
		return false
	}
	depth := 0
	for previous := index - 1; previous >= 0; previous-- {
		switch tokens[previous].Lexeme {
		case ")":
			depth++
		case "(":
			if depth > 0 {
				depth--
				continue
			}
			callee := previousSemanticToken(tokens, previous-1)
			if callee < 0 {
				return false
			}
			if tokens[callee].Lexeme == "fn" {
				return true
			}
			declaration := previousSemanticToken(tokens, callee-1)
			return declaration >= 0 && tokens[declaration].Lexeme == "def"
		}
	}
	return false
}

func lexicalBlockDeclaration(tokens []token.Token, index int) bool {
	left := previousSemanticToken(tokens, index-1)
	for left >= 0 && tokens[left].Kind != token.Newline && tokens[left].Lexeme != "|" {
		left = previousSemanticToken(tokens, left-1)
	}
	if left < 0 || tokens[left].Lexeme != "|" {
		return false
	}
	right := nextSemanticToken(tokens, index+1)
	for right >= 0 && right < len(tokens) && tokens[right].Kind != token.Newline && tokens[right].Lexeme != "|" {
		right = nextSemanticToken(tokens, right+1)
	}
	return right >= 0 && right < len(tokens) && tokens[right].Lexeme == "|"
}

func previousSemanticToken(tokens []token.Token, index int) int {
	for index >= 0 {
		if tokens[index].Kind != token.Comment {
			return index
		}
		index--
	}
	return -1
}

func nextSemanticToken(tokens []token.Token, index int) int {
	for index < len(tokens) {
		if tokens[index].Kind != token.Comment {
			return index
		}
		index++
	}
	return -1
}

func semanticLineEnd(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		return len(source)
	}
	if newline := strings.IndexByte(source[offset:], '\n'); newline >= 0 {
		return offset + newline
	}
	return len(source)
}

func semanticSymbol(name, source string, cursor int, context Context) (Symbol, bool) {
	if symbol, ok := checkedSymbolLookup(source, cursor, context)(name); ok {
		return symbol, true
	}
	if detail, keyword := keywordDetails[name]; keyword {
		return Symbol{Name: name, Kind: CompletionKeyword, Detail: detail}, true
	}
	return Symbol{}, false
}

func semanticDetail(symbol Symbol) string {
	switch symbol.Kind {
	case CompletionVariable, CompletionParameter, CompletionConstant, CompletionField:
		if symbol.Type.Kind != "" {
			return symbol.Name + ": " + symbol.Type.String()
		}
	case CompletionType:
		if symbol.Detail == "built-in type" {
			return "type " + symbol.Name
		}
	}
	if symbol.Detail != "" {
		return symbol.Detail
	}
	if symbol.Type.Kind != "" {
		return symbol.Type.String()
	}
	return symbol.Name
}

func activeCallParameter(call *CallInfo, arguments [][]token.Token) int {
	if call == nil || len(call.Parameters) == 0 || len(arguments) == 0 {
		return 0
	}
	currentKeyword, _ := completionKeywordArgument(arguments[len(arguments)-1])
	if currentKeyword != "" {
		for index, parameter := range call.Parameters {
			if parameter.Keyword && parameter.Name == currentKeyword {
				return index
			}
		}
	}

	position := 0
	for _, argument := range arguments[:len(arguments)-1] {
		if keyword, _ := completionKeywordArgument(argument); keyword == "" {
			position++
		}
	}
	positional := 0
	for index, parameter := range call.Parameters {
		if parameter.Keyword {
			continue
		}
		if positional == position {
			return index
		}
		positional++
	}
	return min(position, len(call.Parameters)-1)
}
