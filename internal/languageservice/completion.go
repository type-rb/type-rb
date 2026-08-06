package languageservice

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

var keywordDetails = map[string]string{
	"and":        "Boolean conjunction",
	"break":      "exit the current loop",
	"case":       "dispatch on an enum",
	"class":      "declare a reference type",
	"def":        "declare a function or method",
	"do":         "start an iterator block",
	"else":       "add a fallback branch",
	"elsif":      "add a conditional branch",
	"end":        "close a block",
	"enum":       "declare a closed nominal type",
	"false":      "Boolean literal",
	"if":         "start a conditional",
	"implements": "declare implemented interfaces",
	"import":     "import a package",
	"interface":  "declare required methods",
	"module":     "group declarations",
	"mut":        "declare a mutable binding",
	"next":       "continue the current loop",
	"nil":        "empty value",
	"not":        "Boolean negation",
	"or":         "Boolean disjunction",
	"readonly":   "declare a readonly field",
	"record":     "declare a data value",
	"return":     "return from a function",
	"self":       "current class or instance",
	"then":       "separate a condition from its branch",
	"true":       "Boolean literal",
	"when":       "handle an enum member",
	"while":      "start a loop",
}

var builtInTypes = []string{
	"Any", "Array", "Boolean", "Bytes", "Float", "Hash", "Integer", "Iterable", "Range", "String", "StringBuilder",
}

func Complete(request CompletionRequest) []CompletionItem {
	if request.Cursor < 0 || request.Cursor > len(request.Source) {
		return nil
	}
	replacement := completionRange(request.Source, request.Cursor)
	prefix := request.Source[replacement.Start:request.Cursor]

	if marker, receiver := memberReceiver(request.Source, replacement.Start); marker != "" {
		members := completeMembers(receiver, marker, request, replacement)
		return filterCompletions(members, prefix)
	}

	byName := map[string]Symbol{}
	for _, symbol := range request.Context.Symbols {
		byName[symbol.Name] = symbol
	}
	for _, symbol := range lexicalSymbols(request.Source, request.Cursor, request.Context) {
		byName[symbol.Name] = symbol
	}
	for _, name := range builtInTypes {
		byName[name] = Symbol{Name: name, Kind: CompletionType, Detail: "built-in type", Type: types.FromName(name)}
	}
	byName["puts"] = Symbol{Name: "puts", Kind: CompletionFunction, Detail: "puts(value: Any)", Type: types.FromName("Void"), Call: &CallInfo{ParameterCount: 1}}
	for name, detail := range keywordDetails {
		if _, exists := byName[name]; !exists {
			byName[name] = Symbol{Name: name, Kind: CompletionKeyword, Detail: detail}
		}
	}

	items := make([]CompletionItem, 0, len(byName))
	for _, symbol := range byName {
		items = append(items, completionFromSymbol(symbol, replacement))
	}
	return filterCompletions(items, prefix)
}

func completeMembers(receiver, marker string, request CompletionRequest, replacement OffsetRange) []CompletionItem {
	symbols := lexicalSymbols(request.Source, request.Cursor, request.Context)
	lookup := func(name string) (Symbol, bool) {
		for index := len(symbols) - 1; index >= 0; index-- {
			if symbols[index].Name == name {
				return symbols[index], true
			}
		}
		for _, symbol := range request.Context.Symbols {
			if symbol.Name == name {
				return symbol, true
			}
		}
		for _, name := range builtInTypes {
			if name == receiver {
				return Symbol{Name: name, Kind: CompletionType, Type: types.FromName(name)}, true
			}
		}
		return Symbol{}, false
	}

	if marker == "::" {
		if symbol, ok := resolveNamespace(receiver, lookup); ok {
			return completionItems(symbol.Members, replacement)
		}
		return nil
	}

	if symbol, ok := lookup(receiver); ok {
		if symbol.Kind == CompletionModule || symbol.Kind == CompletionType && len(symbol.Members) > 0 {
			return completionItems(symbol.Members, replacement)
		}
		if symbol.Type.Kind != "" {
			return completionItems(receiverMembers(symbol.Type, request.Context), replacement)
		}
	}
	if typ, ok := literalReceiverType(receiver); ok {
		return completionItems(receiverMembers(typ, request.Context), replacement)
	}
	return nil
}

func resolveNamespace(receiver string, lookup func(string) (Symbol, bool)) (Symbol, bool) {
	parts := strings.Split(receiver, "::")
	if len(parts) == 0 {
		return Symbol{}, false
	}
	current, ok := lookup(parts[0])
	if !ok {
		return Symbol{}, false
	}
	for _, name := range parts[1:] {
		found := false
		for _, member := range current.Members {
			if member.Name == name {
				current = member
				found = true
				break
			}
		}
		if !found {
			return Symbol{}, false
		}
	}
	return current, true
}

func receiverMembers(receiver types.Type, context Context) []Symbol {
	result := append([]Symbol(nil), context.TypeMembers[receiver.Name]...)
	if len(result) == 0 && strings.Contains(receiver.Name, "::") {
		parts := strings.Split(receiver.Name, "::")
		result = append(result, context.TypeMembers[parts[len(parts)-1]]...)
	}
	for _, method := range stdlib.ReceiverMethods(receiver) {
		result = append(result, Symbol{Name: method.Name, Kind: CompletionMethod, Detail: librarySignature(method), Type: method.Return, Call: &CallInfo{ParameterCount: len(method.Parameters)}})
	}
	byName := map[string]Symbol{}
	for _, symbol := range result {
		byName[symbol.Name] = symbol
	}
	result = result[:0]
	for _, symbol := range byName {
		result = append(result, symbol)
	}
	sortSymbols(result)
	return result
}

func completionItems(symbols []Symbol, replacement OffsetRange) []CompletionItem {
	result := make([]CompletionItem, 0, len(symbols))
	for _, symbol := range symbols {
		result = append(result, completionFromSymbol(symbol, replacement))
	}
	return result
}

func completionFromSymbol(symbol Symbol, replacement OffsetRange) CompletionItem {
	insertText := symbol.Name
	if symbol.Call != nil && symbol.Call.ParameterCount == 0 && !symbol.Call.ExplicitTypeArguments {
		insertText += "()"
	}
	return CompletionItem{Label: symbol.Name, InsertText: insertText, Kind: symbol.Kind, Detail: symbol.Detail, Replacement: replacement}
}

func filterCompletions(items []CompletionItem, prefix string) []CompletionItem {
	filtered := items[:0]
	seen := map[string]bool{}
	for _, item := range items {
		if !strings.HasPrefix(item.Label, prefix) || seen[item.Label] {
			continue
		}
		seen[item.Label] = true
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(left, right int) bool {
		if filtered[left].Kind != filtered[right].Kind {
			return completionPriority(filtered[left].Kind) < completionPriority(filtered[right].Kind)
		}
		return filtered[left].Label < filtered[right].Label
	})
	return filtered
}

func completionRange(source string, cursor int) OffsetRange {
	start := cursor
	for start > 0 {
		r, width := utf8.DecodeLastRuneInString(source[:start])
		if !completionRune(r) {
			break
		}
		start -= width
	}
	end := cursor
	for end < len(source) {
		r, width := utf8.DecodeRuneInString(source[end:])
		if !completionRune(r) {
			break
		}
		end += width
	}
	return OffsetRange{Start: start, End: end}
}

func completionRune(value rune) bool {
	return value == '_' || value == '@' || value == '?' || value == '!' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

func memberReceiver(source string, wordStart int) (string, string) {
	marker := ""
	receiverEnd := wordStart
	switch {
	case wordStart >= 2 && source[wordStart-2:wordStart] == "::":
		marker = "::"
		receiverEnd -= 2
	case wordStart >= 2 && source[wordStart-2:wordStart] == "&.":
		marker = "."
		receiverEnd -= 2
	case wordStart >= 1 && source[wordStart-1:wordStart] == ".":
		marker = "."
		receiverEnd--
	default:
		return "", ""
	}
	start := receiverStart(source, receiverEnd, marker)
	return marker, strings.TrimSpace(source[start:receiverEnd])
}

func receiverStart(source string, end int, marker string) int {
	start := end
	depth := 0
	for start > 0 {
		r, width := utf8.DecodeLastRuneInString(source[:start])
		if r == ')' || r == ']' || r == '}' {
			depth++
			start -= width
			continue
		}
		if depth > 0 {
			if r == '(' || r == '[' || r == '{' {
				depth--
			}
			start -= width
			continue
		}
		if completionRune(r) || r == ':' && marker == "::" || r == '.' && marker == "." || r == '"' || r == '\'' {
			start -= width
			continue
		}
		break
	}
	return start
}

func literalReceiverType(receiver string) (types.Type, bool) {
	trimmed := strings.TrimSpace(receiver)
	if trimmed == "true" || trimmed == "false" {
		return types.FromName("Boolean"), true
	}
	if strings.HasPrefix(trimmed, "\"") || strings.HasPrefix(trimmed, "'") {
		return types.FromName("String"), true
	}
	if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
		tokens, _ := lexer.Lex([]byte(trimmed))
		return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{inferArrayElement(tokens, 0)}}, true
	}
	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
		return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Any"), types.FromName("Any")}}, true
	}
	if trimmed != "" {
		isNumber := true
		float := false
		for index, value := range trimmed {
			if value == '.' {
				float = true
				continue
			}
			if value == '-' && index == 0 || value == '_' || unicode.IsDigit(value) {
				continue
			}
			isNumber = false
			break
		}
		if isNumber {
			if float {
				return types.FromName("Float"), true
			}
			return types.FromName("Integer"), true
		}
	}
	return types.Type{}, false
}

func lexicalSymbols(source string, cursor int, context Context) []Symbol {
	tokens, _ := lexer.Lex([]byte(source))
	significant := make([]token.Token, 0, len(tokens))
	for _, item := range tokens {
		if item.Span.Start.Offset >= cursor || item.Kind == token.Comment || item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		significant = append(significant, item)
	}
	known := map[string]Symbol{}
	for _, symbol := range context.Symbols {
		known[symbol.Name] = symbol
	}
	for index, item := range significant {
		if item.Kind != token.Identifier {
			continue
		}
		switch item.Lexeme {
		case "class", "record", "enum", "interface":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionType, Detail: item.Lexeme, Type: types.FromName(name.Lexeme)}
			}
		case "module":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionModule, Detail: "module"}
			}
		case "def":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionFunction, Detail: "function"}
				collectParameters(significant, index+2, known)
			}
		case "import":
			collectImportedNames(significant, index+1, known)
		default:
			collectVariable(significant, index, known)
		}
	}
	result := make([]Symbol, 0, len(known))
	for _, symbol := range known {
		result = append(result, symbol)
	}
	sortSymbols(result)
	return result
}

func collectParameters(tokens []token.Token, start int, symbols map[string]Symbol) {
	for index := start; index+2 < len(tokens); index++ {
		if tokens[index].Lexeme == ")" {
			return
		}
		if tokens[index].Kind != token.Identifier || tokens[index+1].Lexeme != ":" || tokens[index+2].Kind != token.Identifier {
			continue
		}
		name := tokens[index].Lexeme
		typ := types.FromName(tokens[index+2].Lexeme)
		symbols[name] = Symbol{Name: name, Kind: CompletionParameter, Detail: typ.String(), Type: typ}
	}
}

func collectImportedNames(tokens []token.Token, start int, symbols map[string]Symbol) {
	if start >= len(tokens) || tokens[start].Lexeme != "{" {
		return
	}
	for index := start + 1; index < len(tokens) && tokens[index].Lexeme != "}"; index++ {
		if tokens[index].Kind != token.Identifier {
			continue
		}
		name := tokens[index].Lexeme
		kind := CompletionFunction
		if isTypeName(name) {
			kind = CompletionType
		}
		symbols[name] = Symbol{Name: name, Kind: kind, Detail: "imported name", Type: inferredNamedType(name, kind)}
	}
}

func collectVariable(tokens []token.Token, index int, symbols map[string]Symbol) {
	name := tokens[index].Lexeme
	if _, keyword := keywordDetails[name]; keyword || index > 0 && tokens[index-1].Lexeme == "def" {
		return
	}
	declaration := index + 1
	var typ types.Type
	if declaration+1 < len(tokens) && tokens[declaration].Lexeme == ":" && tokens[declaration+1].Kind == token.Identifier {
		typ = types.FromName(tokens[declaration+1].Lexeme)
		declaration += 2
	}
	if declaration >= len(tokens) || tokens[declaration].Lexeme != ":=" {
		return
	}
	if typ.Kind == "" {
		typ = inferLexicalValue(tokens, declaration+1, symbols)
	}
	kind := CompletionVariable
	if isConstantName(name) {
		kind = CompletionConstant
	}
	symbols[name] = Symbol{Name: name, Kind: kind, Detail: typ.String(), Type: typ}
}

func inferLexicalValue(tokens []token.Token, start int, symbols map[string]Symbol) types.Type {
	if start >= len(tokens) {
		return types.FromName("Any")
	}
	item := tokens[start]
	switch item.Kind {
	case token.String:
		return types.FromName("String")
	case token.Number:
		if strings.Contains(item.Lexeme, ".") {
			return types.FromName("Float")
		}
		return types.FromName("Integer")
	case token.Identifier:
		if item.Lexeme == "true" || item.Lexeme == "false" {
			return types.FromName("Boolean")
		}
		if start+2 < len(tokens) && tokens[start+1].Lexeme == "." && tokens[start+2].Lexeme == "new" {
			return types.FromName(item.Lexeme)
		}
		if symbol, exists := symbols[item.Lexeme]; exists {
			return symbol.Type
		}
	case token.Punct:
		if item.Lexeme == "[" {
			return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{inferArrayElement(tokens, start)}}
		}
		if item.Lexeme == "{" {
			return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Any"), types.FromName("Any")}}
		}
	}
	return types.FromName("Any")
}

func inferArrayElement(tokens []token.Token, start int) types.Type {
	depth := 0
	expecting := false
	var inferred types.Type
	for index := start; index < len(tokens); index++ {
		item := tokens[index]
		switch item.Lexeme {
		case "[":
			depth++
			if depth == 1 {
				expecting = true
			}
			continue
		case "]":
			if depth == 1 {
				if inferred.Kind == "" {
					return types.FromName("Any")
				}
				return inferred
			}
			depth--
			continue
		case ",":
			if depth == 1 {
				expecting = true
			}
			continue
		}
		if depth != 1 || !expecting || item.Kind == token.Comment || item.Kind == token.Newline {
			continue
		}
		current := simpleTokenType(item)
		if current.Kind == "" {
			return types.FromName("Any")
		}
		if inferred.Kind == "" {
			inferred = current
		} else if !types.Equivalent(inferred, current) {
			return types.FromName("Any")
		}
		expecting = false
	}
	return types.FromName("Any")
}

func simpleTokenType(item token.Token) types.Type {
	switch item.Kind {
	case token.String:
		return types.FromName("String")
	case token.Number:
		if strings.Contains(item.Lexeme, ".") {
			return types.FromName("Float")
		}
		return types.FromName("Integer")
	case token.Identifier:
		if item.Lexeme == "true" || item.Lexeme == "false" {
			return types.FromName("Boolean")
		}
	}
	return types.Type{}
}

func tokenAt(tokens []token.Token, index int) (token.Token, bool) {
	if index < 0 || index >= len(tokens) {
		return token.Token{}, false
	}
	return tokens[index], true
}

func isTypeName(name string) bool {
	first, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(first) && !isConstantName(name)
}

func isConstantName(name string) bool {
	if name == "" {
		return false
	}
	hasLetter := false
	for _, value := range name {
		if unicode.IsLetter(value) {
			hasLetter = true
			if unicode.IsLower(value) {
				return false
			}
			continue
		}
		if !unicode.IsDigit(value) && value != '_' {
			return false
		}
	}
	return hasLetter
}
