package languageservice

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

var keywordDetails = map[string]string{
	"alias":      "declare a transparent type alias",
	"break":      "exit the current loop",
	"case":       "dispatch on an enum",
	"catch":      "recover from a Result error",
	"class":      "declare a reference type",
	"def":        "declare a function or method",
	"do":         "start an iterator block",
	"else":       "add a fallback branch",
	"elsif":      "add a conditional branch",
	"end":        "close a block",
	"enum":       "declare a closed nominal type",
	"false":      "Boolean literal",
	"fn":         "create a typed function value",
	"if":         "start a conditional",
	"implements": "declare implemented interfaces",
	"import":     "import a package",
	"interface":  "declare required methods",
	"module":     "group declarations",
	"mut":        "declare a mutable binding",
	"newtype":    "declare a nominal representation type",
	"next":       "continue the current loop",
	"nil":        "empty value",
	"readonly":   "declare a readonly field",
	"record":     "declare a data value",
	"return":     "return from a function",
	"self":       "current class or instance",
	"then":       "separate a condition from its branch",
	"true":       "Boolean literal",
	"try":        "propagate a Result error",
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
	callRequest := withCallArgumentCandidate(request)
	if items, ok := completeCallArgumentReferences(callRequest); ok {
		return items
	}
	if items, ok := completeCallArgumentLiterals(callRequest); ok {
		return items
	}
	replacement := completionRange(request.Source, request.Cursor)
	prefix := request.Source[replacement.Start:request.Cursor]
	typePosition := typeCompletionPosition(request.Source, replacement.Start)

	if marker, receiver := memberReceiver(request.Source, replacement.Start); marker != "" {
		request.Context = memberCandidateContext(request.Context, request.Candidates, request.Source, receiver, request.RepairImports)
		members := completeMembers(receiver, marker, request, replacement)
		return filterCompletions(members, prefix)
	}

	byName := map[string]Symbol{}
	candidateSymbols := []Symbol{}
	lexical, localNames := lexicalSymbolsWithLocals(request.Source, request.Cursor, request.Context)
	for _, symbol := range lexical {
		byName[symbol.Name] = symbol
	}
	if len(request.Candidates.Symbols) > 0 {
		visible := sourceVisibleNames(request.Source)
		for _, symbol := range request.Candidates.Symbols {
			if !strings.HasPrefix(symbol.Name, prefix) || visible[symbol.Name] || localNames[symbol.Name] {
				continue
			}
			if _, checked := byName[symbol.Name]; checked && !request.RepairImports {
				continue
			}
			// A current-source import or declaration keeps its checked symbol.
			// Otherwise the candidate replaces a stale checked symbol so accepting
			// completion restores the missing import.
			if request.RepairImports {
				delete(byName, symbol.Name)
			}
			candidateSymbols = append(candidateSymbols, symbol)
		}
	}
	for _, name := range builtInTypes {
		byName[name] = Symbol{Name: name, Kind: CompletionType, Detail: "built-in type", Type: types.FromName(name)}
	}
	byName["puts"] = Symbol{Name: "puts", Kind: CompletionFunction, Detail: "puts(value: Any)", Type: types.FromName("Void"), Call: &CallInfo{
		ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: Any"}},
	}}
	for name, detail := range keywordDetails {
		if _, exists := byName[name]; !exists {
			byName[name] = Symbol{Name: name, Kind: CompletionKeyword, Detail: detail}
		}
	}

	capacity := len(byName)
	if prefix != "" {
		capacity = 16
	}
	items := make([]CompletionItem, 0, capacity)
	for _, symbol := range byName {
		if !strings.HasPrefix(symbol.Name, prefix) || typePosition && symbol.Kind != CompletionType {
			continue
		}
		items = append(items, completionFromSymbol(symbol, replacement, request.Source))
	}
	for _, symbol := range candidateSymbols {
		if _, shadowed := byName[symbol.Name]; shadowed || !strings.HasPrefix(symbol.Name, prefix) || typePosition && symbol.Kind != CompletionType {
			continue
		}
		items = append(items, completionFromSymbol(symbol, replacement, request.Source))
	}
	return filterCompletions(items, prefix)
}

// withCallArgumentCandidate makes only the direct callee candidate available
// to the early literal/reference completion paths. The full candidate set must
// not become part of checked expression resolution.
func withCallArgumentCandidate(request CompletionRequest) CompletionRequest {
	if len(request.Candidates.Symbols) == 0 {
		return request
	}
	tokens, _ := lexer.Lex([]byte(request.Source[:request.Cursor]))
	significant := completionTokens(tokens)
	open := innermostOpenCall(significant)
	if open < 1 || significant[open-1].Kind != token.Identifier {
		return request
	}
	if open >= 3 && (significant[open-2].Lexeme == "." || significant[open-2].Lexeme == "&.") {
		return request
	}
	name := significant[open-1].Lexeme
	var candidate Symbol
	found := false
	for _, symbol := range request.Candidates.Symbols {
		if symbol.Name == name && symbol.Call != nil {
			candidate = symbol
			found = true
			break
		}
	}
	if !found || sourceVisibleNames(request.Source)[name] {
		return request
	}
	_, localNames := lexicalSymbolsWithLocals(request.Source, request.Cursor, request.Context)
	if localNames[name] {
		return request
	}

	result := request.Context
	result.Symbols = append([]Symbol(nil), request.Context.Symbols...)
	for index, current := range result.Symbols {
		if current.Name != name {
			continue
		}
		if !request.RepairImports {
			return request
		}
		result.Symbols[index] = candidate
		request.Context = result
		return request
	}
	result.Symbols = append(result.Symbols, candidate)
	request.Context = result
	return request
}

func memberCandidateContext(current, candidates Context, source, receiver string, repairImports bool) Context {
	tokens, _ := lexer.Lex([]byte(receiver))
	name := ""
	for _, item := range tokens {
		if item.Kind == token.Identifier {
			name = item.Lexeme
			break
		}
	}
	if name == "" {
		return current
	}
	for _, candidate := range candidates.Symbols {
		if candidate.Name == name {
			if !repairImports {
				return MergeContexts(current, Context{Symbols: []Symbol{candidate}})
			}
			return MergeImportCandidates(current, Context{Symbols: []Symbol{candidate}}, source)
		}
	}
	return current
}

func typeCompletionPosition(source string, wordStart int) bool {
	tokens, _ := lexer.Lex([]byte(source[:wordStart]))
	significant := completionTokens(tokens)
	if len(significant) == 0 {
		return false
	}
	last := significant[len(significant)-1].Lexeme
	switch last {
	case "implements", "|", "->":
		return true
	case "=":
		return currentLineStartsWith(significant, "alias") || currentLineStartsWith(significant, "newtype")
	case ":":
		if unclosedDelimiter(significant, "{", "}") >= 0 {
			return false
		}
		open := unclosedDelimiter(significant, "(", ")")
		if open < 0 {
			return true
		}
		return declarationParameterList(significant, open)
	case "<", ",":
		return unclosedTypeArguments(significant)
	default:
		return false
	}
}

func currentLineStartsWith(tokens []token.Token, lexeme string) bool {
	for index := len(tokens) - 1; index >= 0; index-- {
		if index > 0 && tokens[index-1].Span.End.Line < tokens[index].Span.Start.Line {
			return tokens[index].Lexeme == lexeme
		}
	}
	return len(tokens) > 0 && tokens[0].Lexeme == lexeme
}

func unclosedDelimiter(tokens []token.Token, opening, closing string) int {
	depth := 0
	for index := len(tokens) - 1; index >= 0; index-- {
		switch tokens[index].Lexeme {
		case closing:
			depth++
		case opening:
			if depth == 0 {
				return index
			}
			depth--
		}
	}
	return -1
}

func declarationParameterList(tokens []token.Token, open int) bool {
	if open > 0 && tokens[open-1].Lexeme == "fn" {
		return true
	}
	if open >= 2 && tokens[open-1].Kind == token.Identifier && tokens[open-2].Lexeme == "def" {
		return true
	}
	if open > 3 && tokens[open-1].Lexeme == ">" {
		for index := open - 2; index >= 1; index-- {
			if tokens[index].Lexeme == "<" {
				return tokens[index-1].Kind == token.Identifier && index >= 2 && tokens[index-2].Lexeme == "def"
			}
		}
	}
	return false
}

func unclosedTypeArguments(tokens []token.Token) bool {
	depth := 0
	for index := len(tokens) - 1; index >= 0; index-- {
		switch tokens[index].Lexeme {
		case ">":
			depth++
		case "<":
			if depth > 0 {
				depth--
				continue
			}
			if index == 0 || tokens[index-1].Kind != token.Identifier {
				return false
			}
			name := tokens[index-1].Lexeme
			first, _ := utf8.DecodeRuneInString(name)
			return unicode.IsUpper(first) || slices.Contains(builtInTypes, name)
		}
	}
	return false
}

func completeCallArgumentLiterals(request CompletionRequest) ([]CompletionItem, bool) {
	tokens, _ := lexer.Lex([]byte(request.Source[:request.Cursor]))
	significant := completionTokens(tokens)
	open := innermostOpenCall(significant)
	if open < 1 {
		return nil, false
	}
	call, ok := completionCallSymbol(significant, open, request)
	if !ok || call.Call == nil {
		return nil, false
	}
	arguments := splitCompletionArguments(significant[open+1:])
	position := len(arguments) - 1
	current := arguments[position]
	keyword, valueTokens := completionKeywordArgument(current)
	values := callLiteralValues(call.Call, position, keyword, arguments[:position])
	literalArrays, literalArrayElements := callLiteralArrayValues(call.Call, position, keyword)
	if len(values) == 0 && (len(literalArrays) > 0 || len(literalArrayElements) > 0) {
		items, active := completeLiteralArrayArgument(request, call.Name, valueTokens, literalArrays, literalArrayElements)
		if active {
			return items, true
		}
	}
	if len(values) == 0 {
		return nil, false
	}
	prefix, replacement, before, after, active := literalCompletionContext(request.Source, request.Cursor, valueTokens, keyword != "")
	if !active {
		return nil, false
	}
	items := make([]CompletionItem, 0, len(values))
	for _, value := range values {
		items = append(items, CompletionItem{
			Label: value, InsertText: before + value + after, Kind: CompletionValue,
			Detail: call.Name + "() literal", Replacement: replacement,
		})
	}
	return filterCompletions(items, prefix), true
}

func callLiteralArrayValues(call *CallInfo, position int, keyword string) ([][]string, []string) {
	if keyword != "" {
		for _, parameter := range call.Parameters {
			if parameter.Keyword && parameter.Name == keyword {
				return parameter.LiteralArrays, parameter.LiteralArrayElements
			}
		}
		return nil, nil
	}
	positional := 0
	for _, parameter := range call.Parameters {
		if parameter.Keyword {
			continue
		}
		if positional == position {
			return parameter.LiteralArrays, parameter.LiteralArrayElements
		}
		positional++
	}
	return nil, nil
}

func completeLiteralArrayArgument(request CompletionRequest, callName string, tokens []token.Token, exact [][]string, elements []string) ([]CompletionItem, bool) {
	if len(tokens) == 0 {
		arrays := exact
		if len(arrays) == 0 {
			arrays = make([][]string, len(elements))
			for index, element := range elements {
				arrays[index] = []string{element}
			}
		}
		items := make([]CompletionItem, 0, len(arrays))
		for _, array := range arrays {
			labels := make([]string, len(array))
			for index, value := range array {
				labels[index] = ":" + value
			}
			value := "[" + strings.Join(labels, ", ") + "]"
			items = append(items, CompletionItem{
				Label: value, InsertText: value, Kind: CompletionValue, Detail: callName + "() literal array",
				Replacement: OffsetRange{Start: request.Cursor, End: request.Cursor},
			})
		}
		return items, true
	}
	if tokens[0].Lexeme != "[" {
		return nil, false
	}
	parts := splitCompletionArguments(tokens[1:])
	current := parts[len(parts)-1]
	completed := make([]string, 0, len(parts)-1)
	for _, part := range parts[:len(parts)-1] {
		value, ok := completionArgumentLiteral(part)
		if !ok {
			return nil, false
		}
		completed = append(completed, value)
	}
	allowed := map[string]bool{}
	if len(exact) > 0 {
		for _, candidate := range exact {
			if len(candidate) <= len(completed) || !slices.Equal(candidate[:len(completed)], completed) {
				continue
			}
			allowed[candidate[len(completed)]] = true
		}
	} else {
		for _, candidate := range elements {
			if !slices.Contains(completed, candidate) {
				allowed[candidate] = true
			}
		}
	}
	if len(allowed) == 0 {
		return nil, false
	}
	prefix, replacement, before, after, active := literalCompletionContext(request.Source, request.Cursor, current, true)
	if !active {
		return nil, false
	}
	items := make([]CompletionItem, 0, len(allowed))
	for value := range allowed {
		items = append(items, CompletionItem{
			Label: value, InsertText: before + value + after, Kind: CompletionValue,
			Detail: callName + "() literal array element", Replacement: replacement,
		})
	}
	return filterCompletions(items, prefix), true
}

func completionTokens(tokens []token.Token) []token.Token {
	result := make([]token.Token, 0, len(tokens))
	for _, item := range tokens {
		if item.Kind == token.Comment || item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		result = append(result, item)
	}
	return result
}

func innermostOpenCall(tokens []token.Token) int {
	type opening struct {
		index  int
		lexeme string
	}
	stack := []opening{}
	for index, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			stack = append(stack, opening{index: index, lexeme: item.Lexeme})
		case ")", "]", "}":
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
		}
	}
	for index := len(stack) - 1; index >= 0; index-- {
		if stack[index].lexeme == "(" {
			return stack[index].index
		}
	}
	return -1
}

func completionCallSymbol(tokens []token.Token, open int, request CompletionRequest) (Symbol, bool) {
	callee := tokens[open-1]
	if callee.Kind != token.Identifier {
		return Symbol{}, false
	}
	lookup := checkedSymbolLookup(request.Source, request.Cursor, request.Context)
	if open < 3 || tokens[open-2].Lexeme != "." && tokens[open-2].Lexeme != "&." {
		return lookup(callee.Lexeme)
	}
	receiverEnd := tokens[open-2].Span.Start.Offset
	receiverStart := receiverStart(request.Source, receiverEnd, ".")
	receiver, ok := resolveCompletionExpression(request.Source[receiverStart:receiverEnd], lookup, request.Context)
	if !ok {
		return Symbol{}, false
	}
	return completionMember(receiver, callee.Lexeme, request.Context)
}

func checkedSymbolLookup(source string, cursor int, context Context) func(string) (Symbol, bool) {
	lexical := lexicalSymbols(source, cursor, context)
	return func(name string) (Symbol, bool) {
		for _, symbol := range lexical {
			if symbol.Name == name && (symbol.Kind == CompletionVariable || symbol.Kind == CompletionParameter || symbol.Kind == CompletionConstant) {
				return symbol, true
			}
		}
		for _, symbol := range context.Symbols {
			if symbol.Name == name {
				return symbol, true
			}
		}
		for _, symbol := range lexical {
			if symbol.Name == name {
				return symbol, true
			}
		}
		for _, builtIn := range builtInTypes {
			if builtIn == name {
				return Symbol{Name: name, Kind: CompletionType, Detail: "built-in type", Type: types.FromName(name)}, true
			}
		}
		if name == "puts" {
			return Symbol{
				Name: "puts", Kind: CompletionFunction, Detail: "puts(value: Any)", Type: types.FromName("Void"),
				Call: &CallInfo{ParameterCount: 1, Parameters: []CallParameter{{Name: "value", Label: "value: Any"}}},
			}, true
		}
		return Symbol{}, false
	}
}

func splitCompletionArguments(tokens []token.Token) [][]token.Token {
	result := [][]token.Token{{}}
	depth := 0
	for _, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case ",":
			if depth == 0 {
				result = append(result, []token.Token{})
				continue
			}
		}
		result[len(result)-1] = append(result[len(result)-1], item)
	}
	return result
}

func completionKeywordArgument(tokens []token.Token) (string, []token.Token) {
	if len(tokens) >= 2 && tokens[0].Kind == token.Identifier && tokens[1].Lexeme == ":" {
		return tokens[0].Lexeme, tokens[2:]
	}
	return "", tokens
}

func callLiteralValues(call *CallInfo, position int, keyword string, previous [][]token.Token) []string {
	allowed := map[string]bool{}
	if keyword != "" {
		for _, parameter := range call.Parameters {
			if parameter.Keyword && parameter.Name == keyword {
				for _, value := range parameter.LiteralValues {
					allowed[value] = true
				}
			}
		}
	} else if len(call.Alternatives) > 0 {
		for _, signature := range call.Alternatives {
			if position >= len(signature.Parameters) || !completionSignatureMatches(signature, previous) {
				continue
			}
			for _, value := range signature.Parameters[position].LiteralValues {
				allowed[value] = true
			}
		}
	} else {
		positional := 0
		for _, parameter := range call.Parameters {
			if parameter.Keyword {
				continue
			}
			if positional == position {
				for _, value := range parameter.LiteralValues {
					allowed[value] = true
				}
				break
			}
			positional++
		}
	}
	values := make([]string, 0, len(allowed))
	for value := range allowed {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func completionSignatureMatches(signature CallSignature, previous [][]token.Token) bool {
	for index, argument := range previous {
		if index >= len(signature.Parameters) {
			return false
		}
		constraints := signature.Parameters[index].LiteralValues
		if len(constraints) == 0 {
			continue
		}
		value, ok := completionArgumentLiteral(argument)
		if !ok {
			continue
		}
		matched := false
		for _, candidate := range constraints {
			matched = matched || candidate == value
		}
		if !matched {
			return false
		}
	}
	return true
}

func completionArgumentLiteral(tokens []token.Token) (string, bool) {
	_, tokens = completionKeywordArgument(tokens)
	if len(tokens) == 1 && tokens[0].Kind == token.String {
		value, err := strconv.Unquote(tokens[0].Lexeme)
		return value, err == nil
	}
	if len(tokens) == 2 && tokens[0].Lexeme == ":" && tokens[1].Kind == token.Identifier {
		return tokens[1].Lexeme, true
	}
	return "", false
}

func literalCompletionContext(source string, cursor int, tokens []token.Token, keyword bool) (string, OffsetRange, string, string, bool) {
	if len(tokens) == 0 {
		if keyword {
			return "", OffsetRange{Start: cursor, End: cursor}, ":", "", true
		}
		return "", OffsetRange{Start: cursor, End: cursor}, "\"", "\"", true
	}
	first := tokens[0]
	if first.Kind == token.String && len(first.Lexeme) > 0 {
		quote := first.Lexeme[0]
		if len(first.Lexeme) >= 2 && first.Lexeme[len(first.Lexeme)-1] == quote {
			return "", OffsetRange{}, "", "", false
		}
		start := first.Span.Start.Offset + 1
		end := cursor
		for end < len(source) && source[end] != quote {
			end++
		}
		after := string(quote)
		if end < len(source) && source[end] == quote {
			after = ""
		}
		return source[start:cursor], OffsetRange{Start: start, End: end}, "", after, true
	}
	if first.Lexeme == ":" {
		start := cursor
		if len(tokens) > 1 && tokens[1].Kind == token.Identifier {
			start = tokens[1].Span.Start.Offset
		}
		end := completionRange(source, cursor).End
		return source[start:cursor], OffsetRange{Start: start, End: end}, "", "", true
	}
	start := first.Span.Start.Offset
	end := completionRange(source, cursor).End
	before, after := "\"", "\""
	if keyword {
		before, after = ":", ""
	}
	return source[start:cursor], OffsetRange{Start: start, End: end}, before, after, true
}

func completeMembers(receiver, marker string, request CompletionRequest, replacement OffsetRange) []CompletionItem {
	return completionItems(memberSymbols(receiver, marker, request), replacement, request.Source)
}

func memberSymbols(receiver, marker string, request CompletionRequest) []Symbol {
	lookup := checkedSymbolLookup(request.Source, request.Cursor, request.Context)

	if marker == "::" {
		if symbol, ok := resolveNamespace(receiver, lookup); ok {
			return append([]Symbol(nil), symbol.Members...)
		}
		return nil
	}

	if symbol, ok := resolveCompletionExpression(receiver, lookup, request.Context); ok {
		if symbol.Kind == CompletionModule || symbol.Kind == CompletionType && len(symbol.Members) > 0 {
			return append([]Symbol(nil), symbol.Members...)
		}
		members := append([]Symbol(nil), symbol.Members...)
		if symbol.Type.Kind != "" {
			members = append(members, receiverMembers(symbol.Type, request.Context)...)
		}
		return members
	}
	if typ, ok := literalReceiverType(receiver); ok {
		return receiverMembers(typ, request.Context)
	}
	return nil
}

func resolveCompletionExpression(source string, lookup func(string) (Symbol, bool), context Context) (Symbol, bool) {
	tokens, _ := lexer.Lex([]byte(source))
	return resolveCompletionTokens(completionTokens(tokens), lookup, context)
}

func resolveCompletionTokens(tokens []token.Token, lookup func(string) (Symbol, bool), context Context) (Symbol, bool) {
	if len(tokens) == 0 {
		return Symbol{}, false
	}
	var current Symbol
	var ok bool
	first := tokens[0]
	if first.Kind == token.Identifier {
		current, ok = lookup(first.Lexeme)
	} else if typ, literal := literalReceiverType(first.Lexeme); literal {
		current, ok = Symbol{Name: first.Lexeme, Type: typ}, true
	}
	if !ok {
		return Symbol{}, false
	}
	for index := 1; index < len(tokens); {
		if tokens[index].Lexeme == "<" && current.Call != nil && len(current.Call.TypeParameters) > 0 {
			next := completionMatchingTypeClose(tokens, index)
			if next < 0 {
				return current, true
			}
			arguments, parsed := completionTypeArguments(tokens[index+1 : next])
			if !parsed {
				return Symbol{}, false
			}
			current = instantiateSymbol(current, current.Call.TypeParameters, arguments)
			index = next + 1
			continue
		}
		if tokens[index].Lexeme == "(" {
			next := completionMatchingClose(tokens, index)
			if next < 0 {
				return current, true
			}
			index = next + 1
			continue
		}
		if tokens[index].Lexeme != "." && tokens[index].Lexeme != "&." && tokens[index].Lexeme != "::" || index+1 >= len(tokens) || tokens[index+1].Kind != token.Identifier {
			break
		}
		current, ok = completionMember(current, tokens[index+1].Lexeme, context)
		if !ok {
			return Symbol{}, false
		}
		index += 2
	}
	return current, true
}

func completionMatchingClose(tokens []token.Token, open int) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Lexeme {
		case "(":
			depth++
		case ")":
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func completionMatchingTypeClose(tokens []token.Token, open int) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Lexeme {
		case "<":
			depth++
		case ">":
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func completionTypeArguments(tokens []token.Token) ([]types.Type, bool) {
	parts := splitCompletionTypeArguments(tokens)
	result := make([]types.Type, 0, len(parts))
	for _, part := range parts {
		argument, ok := completionType(part)
		if !ok {
			return nil, false
		}
		result = append(result, argument)
	}
	return result, len(result) > 0
}

func splitCompletionTypeArguments(tokens []token.Token) [][]token.Token {
	result := [][]token.Token{{}}
	depth := 0
	for _, item := range tokens {
		switch item.Lexeme {
		case "<", "(", "[", "{":
			depth++
		case ">", ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case ",":
			if depth == 0 {
				result = append(result, []token.Token{})
				continue
			}
		}
		result[len(result)-1] = append(result[len(result)-1], item)
	}
	return result
}

func completionType(tokens []token.Token) (types.Type, bool) {
	if len(tokens) == 0 {
		return types.Type{}, false
	}
	nullable := tokens[len(tokens)-1].Lexeme == "?"
	if nullable {
		tokens = tokens[:len(tokens)-1]
	}
	depth := 0
	unionStart := 0
	var alternatives []types.Type
	for index, item := range tokens {
		switch item.Lexeme {
		case "<", "(", "[", "{":
			depth++
		case ">", ")", "]", "}":
			depth--
		case "|":
			if depth == 0 {
				alternative, ok := completionType(tokens[unionStart:index])
				if !ok {
					return types.Type{}, false
				}
				alternatives = append(alternatives, alternative)
				unionStart = index + 1
			}
		}
	}
	if len(alternatives) > 0 {
		last, ok := completionType(tokens[unionStart:])
		if !ok {
			return types.Type{}, false
		}
		result := types.UnionOf(append(alternatives, last)...)
		result.Nullable = nullable
		return result, true
	}
	if len(tokens) == 0 || tokens[0].Kind != token.Identifier && tokens[0].Kind != token.String && tokens[0].Kind != token.Number {
		return types.Type{}, false
	}
	name := tokens[0].Lexeme
	index := 1
	for index+1 < len(tokens) && tokens[index].Lexeme == "::" && tokens[index+1].Kind == token.Identifier {
		name += "::" + tokens[index+1].Lexeme
		index += 2
	}
	result := types.FromName(name)
	if index < len(tokens) {
		if tokens[index].Lexeme != "<" {
			return types.Type{}, false
		}
		close := completionMatchingTypeClose(tokens, index)
		if close != len(tokens)-1 {
			return types.Type{}, false
		}
		arguments, ok := completionTypeArguments(tokens[index+1 : close])
		if !ok {
			return types.Type{}, false
		}
		result.Args = arguments
	}
	result.Nullable = nullable
	return result, true
}

func completionMember(receiver Symbol, name string, context Context) (Symbol, bool) {
	if len(receiver.Members) > 0 {
		for _, member := range receiver.Members {
			if member.Name == name {
				return member, true
			}
		}
	}
	if receiver.Type.Kind != "" {
		if member, ok := resolveReceiverMember(receiver.Type, name, context, map[string]bool{}); ok {
			return member, true
		}
	}
	return Symbol{}, false
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
	return receiverMembersSeen(receiver, context, map[string]bool{})
}

func receiverMembersSeen(receiver types.Type, context Context, seen map[string]bool) []Symbol {
	key := receiver.String()
	if seen[key] {
		return nil
	}
	seen[key] = true
	defer delete(seen, key)
	if target, ok := aliasTarget(receiver, context); ok {
		return receiverMembersSeen(target, context, seen)
	}
	if receiver.Kind == types.Union {
		return commonReceiverMembers(receiver.Args, context, seen)
	}
	result := directReceiverMembers(receiver, context)
	sortSymbols(result)
	return result
}

func directReceiverMembers(receiver types.Type, context Context) []Symbol {
	result := append([]Symbol(nil), context.TypeMembers[receiver.Name]...)
	if len(result) == 0 && strings.Contains(receiver.Name, "::") {
		parts := strings.Split(receiver.Name, "::")
		result = append(result, context.TypeMembers[parts[len(parts)-1]]...)
	}
	if info, ok := context.Types[receiver.Name]; ok && len(info.TypeParameters) > 0 {
		result = instantiateSymbols(result, info.TypeParameters, receiver.Args)
	}
	for _, method := range stdlib.ReceiverMethods(receiver) {
		result = append(result, Symbol{Name: method.Name, Kind: CompletionMethod, Detail: librarySignature(method), Type: method.Return, Call: &CallInfo{ParameterCount: len(method.Parameters)}})
	}
	if receiver.Kind == types.Array || receiver.Kind == types.Range {
		for _, name := range []string{"all?", "any?", "find", "find_index", "none?", "sort_by", "sort_by_descending"} {
			returnType := types.FromName("Boolean")
			if name == "find" && len(receiver.Args) > 0 {
				returnType = receiver.Args[0]
				returnType.Nullable = true
			} else if name == "find_index" {
				returnType = types.FromName("Integer")
				returnType.Nullable = true
			} else if name == "sort_by" || name == "sort_by_descending" {
				returnType = receiver
			}
			detailType := "Boolean"
			if name == "sort_by" || name == "sort_by_descending" {
				detailType = "ordered key"
			}
			result = append(result, Symbol{Name: name, Kind: CompletionMethod, Detail: name + " { |value| " + detailType + " }: " + displayType(returnType), Type: returnType})
		}
	}
	if receiver.Kind == types.Array {
		resultType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Any")}}
		result = append(result, Symbol{
			Name: "concurrent_map", Kind: CompletionMethod,
			Detail: "concurrent_map(limit: Integer = 8) { |value| U }: Array<U>", Type: resultType,
		})
	}
	byName := map[string]Symbol{}
	for _, symbol := range result {
		byName[symbol.Name] = symbol
	}
	result = result[:0]
	for _, symbol := range byName {
		result = append(result, symbol)
	}
	return result
}

func commonReceiverMembers(alternatives []types.Type, context Context, seen map[string]bool) []Symbol {
	if len(alternatives) == 0 {
		return nil
	}
	common := map[string]Symbol{}
	for _, member := range receiverMembersSeen(alternatives[0], context, seen) {
		common[member.Name] = member
	}
	for _, alternative := range alternatives[1:] {
		current := map[string]Symbol{}
		for _, member := range receiverMembersSeen(alternative, context, seen) {
			current[member.Name] = member
		}
		for name, member := range common {
			candidate, ok := current[name]
			if !ok {
				delete(common, name)
				continue
			}
			common[name] = mergeMemberSymbols(member, candidate)
		}
	}
	result := make([]Symbol, 0, len(common))
	for _, member := range common {
		result = append(result, member)
	}
	sortSymbols(result)
	return result
}

func resolveReceiverMember(receiver types.Type, name string, context Context, seen map[string]bool) (Symbol, bool) {
	key := receiver.String() + "." + name
	if seen[key] {
		return Symbol{}, false
	}
	seen[key] = true
	defer delete(seen, key)
	if target, ok := aliasTarget(receiver, context); ok {
		return resolveReceiverMember(target, name, context, seen)
	}
	if receiver.Kind == types.Union {
		var result Symbol
		found := false
		for _, alternative := range receiver.Args {
			member, ok := resolveReceiverMember(alternative, name, context, seen)
			if !ok {
				continue
			}
			if found {
				result = mergeMemberSymbols(result, member)
			} else {
				result = member
				found = true
			}
		}
		return result, found
	}
	for _, member := range directReceiverMembers(receiver, context) {
		if member.Name == name {
			return member, true
		}
	}
	return Symbol{}, false
}

func aliasTarget(receiver types.Type, context Context) (types.Type, bool) {
	info, ok := context.Types[receiver.Name]
	if !ok && strings.Contains(receiver.Name, "::") {
		parts := strings.Split(receiver.Name, "::")
		info, ok = context.Types[parts[len(parts)-1]]
	}
	if !ok || info.AliasTarget == nil {
		return types.Type{}, false
	}
	target := substituteCompletionType(*info.AliasTarget, completionTypeSubstitutions(info.TypeParameters, receiver.Args))
	target.Nullable = target.Nullable || receiver.Nullable
	target.Readonly = target.Readonly || receiver.Readonly
	return target, true
}

func instantiateSymbols(symbols []Symbol, parameters []string, arguments []types.Type) []Symbol {
	result := make([]Symbol, len(symbols))
	for index, symbol := range symbols {
		result[index] = instantiateSymbol(symbol, parameters, arguments)
	}
	return result
}

func instantiateSymbol(symbol Symbol, parameters []string, arguments []types.Type) Symbol {
	result := symbol
	substitutions := completionTypeSubstitutions(parameters, arguments)
	result.Type = substituteCompletionType(symbol.Type, substitutions)
	result.Members = instantiateSymbols(symbol.Members, parameters, arguments)
	return result
}

func completionTypeSubstitutions(parameters []string, arguments []types.Type) map[string]types.Type {
	result := map[string]types.Type{}
	for index, parameter := range parameters {
		if index < len(arguments) {
			result[parameter] = arguments[index]
		}
	}
	return result
}

func substituteCompletionType(input types.Type, substitutions map[string]types.Type) types.Type {
	if replacement, ok := substitutions[input.Name]; ok && input.Kind == types.Named && len(input.Args) == 0 {
		replacement.Nullable = replacement.Nullable || input.Nullable
		replacement.Readonly = replacement.Readonly || input.Readonly
		return replacement
	}
	result := input
	result.Args = make([]types.Type, len(input.Args))
	for index, argument := range input.Args {
		result.Args[index] = substituteCompletionType(argument, substitutions)
	}
	return result
}

func mergeMemberSymbols(left, right Symbol) Symbol {
	result := left
	if left.Type.Kind == "" {
		result.Type = right.Type
	} else if right.Type.Kind != "" && !types.Equivalent(left.Type, right.Type) {
		result.Type = types.UnionOf(left.Type, right.Type)
	}
	if result.Kind == CompletionField && result.Type.Kind != "" {
		result.Detail = displayType(result.Type)
	}
	return result
}

func completionItems(symbols []Symbol, replacement OffsetRange, source string) []CompletionItem {
	result := make([]CompletionItem, 0, len(symbols))
	for _, symbol := range symbols {
		result = append(result, completionFromSymbol(symbol, replacement, source))
	}
	return result
}

func completionFromSymbol(symbol Symbol, replacement OffsetRange, source string) CompletionItem {
	insertText := symbol.Name
	if symbol.Call != nil && symbol.Call.ParameterCount == 0 && !symbol.Call.ExplicitTypeArguments {
		insertText += "()"
	}
	item := CompletionItem{Label: symbol.Name, InsertText: insertText, Kind: symbol.Kind, Detail: symbol.Detail, Replacement: replacement}
	if symbol.Import != nil {
		if edit, ok := autoImportEdit(source, *symbol.Import); ok {
			item.AdditionalEdits = []TextEdit{edit}
		}
	}
	return item
}

func autoImportEdit(source string, required Import) (TextEdit, bool) {
	program, _ := parser.Parse([]byte(source))
	lastImportEnd := -1
	for _, statement := range program.Statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		if required.Symbol == "" && len(imported.Symbols) == 0 && imported.Path == required.Path && imported.Alias == "" {
			return TextEdit{}, false
		}
		for _, name := range imported.Symbols {
			if imported.Path == required.Path && name == required.Symbol {
				return TextEdit{}, false
			}
		}
		span := imported.Span()
		lastImportEnd = lineEnd(source, span.End.Offset)
		if required.Symbol != "" && imported.Path == required.Path && len(imported.Symbols) > 0 {
			line := source[span.Start.Offset:span.End.Offset]
			if close := strings.LastIndex(line, "}"); close >= 0 {
				offset := span.Start.Offset + close
				return TextEdit{Range: OffsetRange{Start: offset, End: offset}, NewText: ", " + required.Symbol}, true
			}
		}
	}
	text := "import " + required.Path + "\n"
	if required.Symbol != "" {
		text = "import { " + required.Symbol + " } from " + required.Path + "\n"
	}
	if lastImportEnd >= 0 {
		return TextEdit{Range: OffsetRange{Start: lastImportEnd, End: lastImportEnd}, NewText: text}, true
	}
	insertion := 0
	for _, statement := range program.Statements {
		switch statement.(type) {
		case *ast.CommentStatement, *ast.BlankStatement:
			continue
		default:
			insertion = statement.Span().Start.Offset
		}
		break
	}
	return TextEdit{Range: OffsetRange{Start: insertion, End: insertion}, NewText: text}, true
}

func lineEnd(source string, offset int) int {
	if offset < 0 {
		return 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	if next := strings.IndexByte(source[offset:], '\n'); next >= 0 {
		return offset + next + 1
	}
	return len(source)
}

func filterCompletions(items []CompletionItem, prefix string) []CompletionItem {
	filtered := items[:0]
	seen := map[string]bool{}
	for _, item := range items {
		key := completionItemKey(item)
		if !strings.HasPrefix(item.Label, prefix) || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, item)
	}
	sort.SliceStable(filtered, func(left, right int) bool {
		if filtered[left].Kind != filtered[right].Kind {
			return completionPriority(filtered[left].Kind) < completionPriority(filtered[right].Kind)
		}
		if filtered[left].Label != filtered[right].Label {
			return filtered[left].Label < filtered[right].Label
		}
		if filtered[left].Detail != filtered[right].Detail {
			return filtered[left].Detail < filtered[right].Detail
		}
		return filtered[left].InsertText < filtered[right].InsertText
	})
	return filtered
}

func completionItemKey(item CompletionItem) string {
	var result strings.Builder
	result.WriteString(string(item.Kind))
	result.WriteByte(0)
	result.WriteString(item.Label)
	result.WriteByte(0)
	result.WriteString(item.InsertText)
	result.WriteByte(0)
	result.WriteString(item.Detail)
	for _, edit := range item.AdditionalEdits {
		result.WriteByte(0)
		result.WriteString(strconv.Itoa(edit.Range.Start))
		result.WriteByte(':')
		result.WriteString(strconv.Itoa(edit.Range.End))
		result.WriteByte(':')
		result.WriteString(edit.NewText)
	}
	return result.String()
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
	if strings.Contains(trimmed, "..") {
		if typ, ok := integerRangeLiteralType(trimmed); ok {
			return typ, true
		}
	}
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

func integerRangeLiteralType(source string) (types.Type, bool) {
	program, diagnostics := parser.Parse([]byte(source))
	if len(diagnostics) > 0 || len(program.Statements) != 1 {
		return types.Type{}, false
	}
	statement, ok := program.Statements[0].(*ast.ExpressionStatement)
	if !ok {
		return types.Type{}, false
	}
	rangeExpression, ok := statement.Expression.(*ast.RangeExpression)
	if !ok || !integerLiteralExpression(rangeExpression.Start) || !integerLiteralExpression(rangeExpression.End) {
		return types.Type{}, false
	}
	return types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}, true
}

func integerLiteralExpression(expression ast.Expression) bool {
	switch node := expression.(type) {
	case *ast.Literal:
		return node.Kind == ast.IntegerLiteral
	case *ast.UnaryExpression:
		return (node.Operator == "+" || node.Operator == "-") && integerLiteralExpression(node.Operand)
	default:
		return false
	}
}

func lexicalSymbols(source string, cursor int, context Context) []Symbol {
	result, _ := lexicalSymbolsWithLocals(source, cursor, context)
	return result
}

func lexicalSymbolsWithLocals(source string, cursor int, context Context) ([]Symbol, map[string]bool) {
	tokens, _ := lexer.Lex([]byte(source))
	significant := make([]token.Token, 0, len(tokens))
	for _, item := range tokens {
		if item.Span.Start.Offset >= cursor || item.Kind == token.Comment || item.Kind == token.Newline || item.Kind == token.EOF {
			continue
		}
		significant = append(significant, item)
	}
	known := map[string]Symbol{}
	locals := map[string]bool{}
	for _, symbol := range context.Symbols {
		known[symbol.Name] = symbol
	}
	for index, item := range significant {
		if item.Kind != token.Identifier {
			continue
		}
		switch item.Lexeme {
		case "class", "record", "enum", "interface", "alias", "newtype":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionType, Detail: item.Lexeme, Type: types.FromName(name.Lexeme)}
				collectTypeParameters(significant, index+2, known)
			}
		case "module":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionModule, Detail: "module"}
			}
		case "def":
			if name, ok := tokenAt(significant, index+1); ok && name.Kind == token.Identifier {
				known[name.Lexeme] = Symbol{Name: name.Lexeme, Kind: CompletionFunction, Detail: "function"}
				collectTypeParameters(significant, index+2, known)
				collectParameters(significant, index+2, known, locals)
			}
		case "fn":
			collectParameters(significant, index+1, known, locals)
		case "import":
			collectImportedNames(significant, index+1, known)
		default:
			collectVariable(significant, index, known, context, locals)
		}
	}
	result := make([]Symbol, 0, len(known))
	for _, symbol := range known {
		result = append(result, symbol)
	}
	sortSymbols(result)
	return result, locals
}

func collectTypeParameters(tokens []token.Token, start int, symbols map[string]Symbol) {
	if start >= len(tokens) || tokens[start].Lexeme != "<" {
		return
	}
	for index := start + 1; index < len(tokens) && tokens[index].Lexeme != ">"; index++ {
		if tokens[index].Kind != token.Identifier {
			continue
		}
		name := tokens[index].Lexeme
		symbols[name] = Symbol{Name: name, Kind: CompletionType, Detail: "type parameter", Type: types.FromName(name)}
	}
}

func collectParameters(tokens []token.Token, start int, symbols map[string]Symbol, locals map[string]bool) {
	for index := start; index+2 < len(tokens); index++ {
		if tokens[index].Lexeme == ")" {
			return
		}
		if tokens[index].Kind != token.Identifier || tokens[index+1].Lexeme != ":" || tokens[index+2].Kind != token.Identifier {
			continue
		}
		name := tokens[index].Lexeme
		typ := types.FromName(tokens[index+2].Lexeme)
		symbols[name] = Symbol{Name: name, Kind: CompletionParameter, Detail: displayType(typ), Type: typ}
		locals[name] = true
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
		if _, exists := symbols[name]; exists {
			continue
		}
		kind := CompletionFunction
		if isTypeName(name) {
			kind = CompletionType
		}
		symbols[name] = Symbol{Name: name, Kind: kind, Detail: "imported name", Type: inferredNamedType(name, kind)}
	}
}

func collectVariable(tokens []token.Token, index int, symbols map[string]Symbol, context Context, locals map[string]bool) {
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
		typ = inferLexicalValue(tokens, declaration+1, symbols, context)
	}
	kind := CompletionVariable
	if isConstantName(name) {
		kind = CompletionConstant
	}
	symbols[name] = Symbol{Name: name, Kind: kind, Detail: displayType(typ), Type: typ}
	locals[name] = true
}

func inferLexicalValue(tokens []token.Token, start int, symbols map[string]Symbol, context Context) types.Type {
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
		lookup := func(name string) (Symbol, bool) {
			value, ok := symbols[name]
			return value, ok
		}
		if resolved, ok := resolveCompletionTokens(tokens[start:], lookup, context); ok && resolved.Type.Kind != "" {
			return resolved.Type
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
		} else {
			joined, ok := types.CommonType(inferred, current)
			if !ok {
				return types.FromName("Any")
			}
			inferred = joined
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
