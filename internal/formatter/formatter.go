// Package formatter implements a deterministic, comment-preserving TypeRB
// printer. Parsing happens first; the printer then uses the lossless token
// stream so comments, strings, percent literals, and heredocs are never lost.
package formatter

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

const indentation = "\t"

type Options struct {
	// CanonicalImportPath returns the authored spelling that should be printed
	// for one parsed import. Project-aware callers may use their shared module
	// resolver; source-only callers leave it nil and preserve import paths.
	CanonicalImportPath func(string) string
	// ResolveImport supplies project-aware declaration-root information. The
	// formatter uses it only for transformations whose declaration identity and
	// root stability are proven by the current project snapshot.
	ResolveImport func(*ast.ImportStatement) ImportMetadata
}

type ImportMetadata struct {
	CanonicalPath string
	Root          string
	RootStable    bool
	Resolved      bool
}

func Format(source []byte) ([]byte, []diagnostic.Diagnostic) {
	return FormatWithOptions(source, Options{})
}

func FormatWithOptions(source []byte, options Options) ([]byte, []diagnostic.Diagnostic) {
	program, diagnostics := parser.Parse(source)
	if hasErrors(diagnostics) {
		return nil, diagnostics
	}
	if options.ResolveImport != nil {
		canonical := canonicalImportSource(source, program, options.ResolveImport)
		if string(canonical) != string(source) {
			source = canonical
			program, diagnostics = parser.Parse(source)
			if hasErrors(diagnostics) {
				return nil, diagnostics
			}
		}
	}
	tokens := opaqueNativeTokens(source, canonicalImportTokens(program, options.CanonicalImportPath), program.NativeIslands)
	return formatTokens(tokens), nil
}

type canonicalImport struct {
	node     *ast.ImportStatement
	metadata ImportMetadata
	path     string
}

func canonicalImportSource(source []byte, program *ast.Program, resolve func(*ast.ImportStatement) ImportMetadata) []byte {
	var imports []canonicalImport
	for _, statement := range program.Statements {
		node, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		metadata := resolve(node)
		if metadata.CanonicalPath == "" {
			metadata.CanonicalPath = node.Path
		}
		imports = append(imports, canonicalImport{
			node: node, metadata: metadata,
			path: canonicalImportPath(source, node, metadata.CanonicalPath),
		})
	}
	if len(imports) == 0 {
		return source
	}
	type replacement struct {
		start int
		end   int
		text  string
	}
	var replacements []replacement
	for start := 0; start < len(imports); {
		end := start + 1
		for end < len(imports) && mergeableImportPair(source, imports[end-1], imports[end]) {
			end++
		}
		group := imports[start:end]
		if text, ok := canonicalImportGroup(group); ok {
			first := group[0].node.Span()
			last := group[len(group)-1].node.Span()
			replacements = append(replacements, replacement{start: first.Start.Offset, end: last.End.Offset, text: text})
		}
		start = end
	}
	if len(replacements) == 0 {
		return source
	}
	var result strings.Builder
	position := 0
	for _, item := range replacements {
		if item.start < position || item.start < 0 || item.end > len(source) {
			continue
		}
		result.Write(source[position:item.start])
		result.WriteString(item.text)
		position = item.end
	}
	result.Write(source[position:])
	return []byte(result.String())
}

func mergeableImportPair(source []byte, left, right canonicalImport) bool {
	if left.node.TrailingComment != "" || right.node.TrailingComment != "" {
		return false
	}
	if left.metadata.CanonicalPath != right.metadata.CanonicalPath {
		return false
	}
	between := source[left.node.Span().End.Offset:right.node.Span().Start.Offset]
	return strings.Count(string(between), "\n") == 1 && !strings.Contains(string(between), "#")
}

type canonicalSpecifier struct {
	name  string
	alias string
}

func canonicalImportGroup(group []canonicalImport) (string, bool) {
	if len(group) == 0 {
		return "", false
	}
	path := group[0].path
	root := group[0].metadata.Root
	rootStable := group[0].metadata.Resolved && group[0].metadata.RootStable && root != ""
	allNamed := true
	var specifiers []canonicalSpecifier
	for _, imported := range group {
		if len(imported.node.Symbols) == 0 {
			allNamed = false
			if !rootStable {
				if len(group) == 1 {
					return canonicalBareImport(imported.node, path), true
				}
				return "", false
			}
			specifiers = append(specifiers, canonicalSpecifier{name: root, alias: imported.node.Alias})
			continue
		}
		for _, name := range imported.node.Symbols {
			specifiers = append(specifiers, canonicalSpecifier{name: name, alias: imported.node.SymbolAliases[name]})
		}
	}
	if len(group) == 1 && allNamed && len(specifiers) == 1 && rootStable && specifiers[0].name == root {
		return canonicalBareImportWithAlias(path, specifiers[0].alias), true
	}
	if !allNamed && len(group) == 1 {
		return canonicalBareImport(group[0].node, path), true
	}
	sort.SliceStable(specifiers, func(left, right int) bool {
		if specifiers[left].name != specifiers[right].name {
			return specifiers[left].name < specifiers[right].name
		}
		return specifiers[left].alias < specifiers[right].alias
	})
	unique := specifiers[:0]
	for _, item := range specifiers {
		if len(unique) > 0 && unique[len(unique)-1] == item {
			continue
		}
		unique = append(unique, item)
	}
	parts := make([]string, len(unique))
	for index, item := range unique {
		parts[index] = item.name
		if item.alias != "" && item.alias != item.name {
			parts[index] += " as " + item.alias
		}
	}
	return "import { " + strings.Join(parts, ", ") + " } from " + path, true
}

func canonicalBareImport(node *ast.ImportStatement, path string) string {
	return canonicalBareImportWithAlias(path, node.Alias)
}

func canonicalBareImportWithAlias(path, alias string) string {
	result := "import " + path
	if alias != "" {
		result += " as " + alias
	}
	return result
}

func canonicalImportPath(source []byte, node *ast.ImportStatement, canonical string) string {
	start := node.PathSpan.Start.Offset
	end := node.PathSpan.End.Offset
	if start < 0 || end < start || end > len(source) {
		return canonical
	}
	original := string(source[start:end])
	if len(original) < 2 || original[0] != original[len(original)-1] || original[0] != '\'' && original[0] != '"' {
		return canonical
	}
	if canonical == node.Path {
		return original
	}
	return strconv.Quote(canonical)
}

func opaqueNativeTokens(source []byte, tokens []token.Token, islands []ast.NativeIsland) []token.Token {
	if len(islands) == 0 {
		return tokens
	}
	sorted := append([]ast.NativeIsland(nil), islands...)
	sort.SliceStable(sorted, func(left, right int) bool {
		if sorted[left].Span.Start.Offset != sorted[right].Span.Start.Offset {
			return sorted[left].Span.Start.Offset < sorted[right].Span.Start.Offset
		}
		return sorted[left].Span.End.Offset > sorted[right].Span.End.Offset
	})
	outer := sorted[:0]
	for _, island := range sorted {
		if len(outer) > 0 && island.Span.Start.Offset < outer[len(outer)-1].Span.End.Offset {
			continue
		}
		outer = append(outer, island)
	}

	result := make([]token.Token, 0, len(tokens))
	tokenIndex := 0
	for _, island := range outer {
		start := island.Span.Start.Offset
		end := island.Span.End.Offset
		endPosition := island.Span.End
		if island.WholeStatement {
			for _, item := range tokens {
				if item.Kind == token.Comment && item.Span.Start.Line == endPosition.Line && item.Span.Start.Offset >= end {
					end = item.Span.End.Offset
					endPosition = item.Span.End
					break
				}
			}
		}
		if start < 0 || end < start || end > len(source) {
			continue
		}
		for tokenIndex < len(tokens) && tokens[tokenIndex].Span.End.Offset <= start {
			result = append(result, tokens[tokenIndex])
			tokenIndex++
		}
		result = append(result, token.Token{
			Kind:   token.NativeIsland,
			Lexeme: string(source[start:end]),
			Span:   token.Span{Start: island.Span.Start, End: endPosition},
		})
		for tokenIndex < len(tokens) && tokens[tokenIndex].Span.Start.Offset < end {
			tokenIndex++
		}
	}
	result = append(result, tokens[tokenIndex:]...)
	return result
}

func formatTokens(tokens []token.Token) []byte {
	lines := tokensByLine(tokens)
	var out strings.Builder
	indent := 0
	continuation := 0
	blank := false
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		line := lines[lineIndex]
		code := withoutNewline(line)
		coveredLine := lineIndex
		coveredOffset := -1
		for _, item := range code {
			if endLine := item.Span.End.Line - 1; endLine > coveredLine || endLine == coveredLine && item.Span.End.Offset > coveredOffset {
				coveredLine = endLine
				coveredOffset = item.Span.End.Offset
			}
		}
		// A token beginning after one multiline token may itself continue onto a
		// later line, so follow the chain until its final physical line.
		for coveredLine > lineIndex && coveredLine < len(lines) {
			endingLine := coveredLine
			endingOffset := coveredOffset
			for _, item := range withoutNewline(lines[endingLine]) {
				if item.Span.Start.Offset < endingOffset {
					continue
				}
				code = append(code, item)
				if endLine := item.Span.End.Line - 1; endLine > coveredLine || endLine == coveredLine && item.Span.End.Offset > coveredOffset {
					coveredLine = endLine
					coveredOffset = item.Span.End.Offset
				}
			}
			if coveredLine == endingLine {
				break
			}
		}
		statements := splitStatements(code, continuation)
		if len(statements) == 0 {
			if !blank && out.Len() > 0 {
				out.WriteByte('\n')
			}
			blank = true
			continue
		}
		blank = false
		for _, statement := range statements {
			writeStatement(&out, statement, &indent, &continuation)
		}
		if coveredLine > lineIndex {
			lineIndex = coveredLine
		}
	}
	return []byte(strings.TrimRight(out.String(), "\n") + "\n")
}

// ReindentPartial applies canonical leading indentation to a tokenizable source
// fragment without requiring the fragment to parse as a complete program. It
// preserves all non-leading text and the contents of multiline tokens so an
// interactive editor can safely call it while a submission is still open.
func ReindentPartial(source []byte) []byte {
	return ReindentPartialWithIndentation(source, indentation)
}

// ReindentPartialWithIndentation applies leading indentation using the given
// display unit. It is intended for interactive editors that keep canonical
// formatting at submission boundaries but need a different on-screen width.
func ReindentPartialWithIndentation(source []byte, indentation string) []byte {
	tokens, diagnostics := lexer.Lex(source)
	if hasErrors(diagnostics) {
		return append([]byte(nil), source...)
	}
	levels, _ := partialIndentation(tokens)
	lines := strings.Split(string(source), "\n")
	for lineIndex, level := range levels {
		if lineIndex >= len(lines) || level < 0 || strings.TrimSpace(lines[lineIndex]) == "" {
			continue
		}
		lines[lineIndex] = strings.Repeat(indentation, level) + strings.TrimLeft(lines[lineIndex], " \t")
	}
	return []byte(strings.Join(lines, "\n"))
}

// NextLineIndent returns the indentation for a new line following a
// tokenizable, possibly incomplete source fragment.
func NextLineIndent(source []byte) string {
	return NextLineIndentWithIndentation(source, indentation)
}

// NextLineIndentWithIndentation returns the next-line indentation using the
// given display unit.
func NextLineIndentWithIndentation(source []byte, indentation string) string {
	tokens, diagnostics := lexer.Lex(source)
	if hasErrors(diagnostics) {
		return ""
	}
	_, level := partialIndentation(tokens)
	return strings.Repeat(indentation, level)
}

func partialIndentation(tokens []token.Token) ([]int, int) {
	lines := tokensByLine(tokens)
	levels := make([]int, len(lines))
	for index := range levels {
		levels[index] = -1
	}
	indent := 0
	continuation := 0
	for lineIndex := 0; lineIndex < len(lines); lineIndex++ {
		code := withoutNewline(lines[lineIndex])
		coveredLine := lineIndex
		coveredOffset := -1
		for _, item := range code {
			if endLine := item.Span.End.Line - 1; endLine > coveredLine || endLine == coveredLine && item.Span.End.Offset > coveredOffset {
				coveredLine = endLine
				coveredOffset = item.Span.End.Offset
			}
		}
		for coveredLine > lineIndex && coveredLine < len(lines) {
			endingLine := coveredLine
			endingOffset := coveredOffset
			for _, item := range withoutNewline(lines[endingLine]) {
				if item.Span.Start.Offset < endingOffset {
					continue
				}
				code = append(code, item)
				if endLine := item.Span.End.Line - 1; endLine > coveredLine || endLine == coveredLine && item.Span.End.Offset > coveredOffset {
					coveredLine = endLine
					coveredOffset = item.Span.End.Offset
				}
			}
			if coveredLine == endingLine {
				break
			}
		}
		statements := splitStatements(code, continuation)
		for statementIndex, statement := range statements {
			level := advanceIndentation(statement, &indent, &continuation)
			if statementIndex == 0 {
				levels[lineIndex] = level
			}
		}
		if coveredLine > lineIndex {
			lineIndex = coveredLine
		}
	}
	level := indent + continuation
	if level < 0 {
		level = 0
	}
	return levels, level
}

type importTokenReplacement struct {
	end    int
	lexeme string
}

func canonicalImportTokens(program *ast.Program, canonicalize func(string) string) []token.Token {
	if canonicalize == nil {
		return program.Tokens
	}
	replacements := map[int]importTokenReplacement{}
	for _, statement := range program.Statements {
		var path string
		var span token.Span
		switch node := statement.(type) {
		case *ast.ImportStatement:
			path = node.Path
			span = node.PathSpan
		case *ast.ActivateStatement:
			path = node.Path
			span = node.PathSpan
		default:
			continue
		}
		if path == "" {
			continue
		}
		canonical := canonicalize(path)
		if canonical == "" || canonical == path {
			continue
		}
		replacements[span.Start.Offset] = importTokenReplacement{
			end: span.End.Offset, lexeme: canonical,
		}
	}
	if len(replacements) == 0 {
		return program.Tokens
	}
	result := make([]token.Token, 0, len(program.Tokens))
	skipUntil := -1
	for _, item := range program.Tokens {
		if replacement, ok := replacements[item.Span.Start.Offset]; ok {
			item.Lexeme = canonicalImportLexeme(item, replacement.lexeme)
			result = append(result, item)
			skipUntil = replacement.end
			continue
		}
		if item.Span.Start.Offset < skipUntil {
			continue
		}
		result = append(result, item)
	}
	return result
}

func canonicalImportLexeme(original token.Token, canonical string) string {
	if original.Kind != token.String || len(original.Lexeme) < 2 {
		return canonical
	}
	quote := original.Lexeme[0]
	if (quote == '\'' || quote == '"') && original.Lexeme[len(original.Lexeme)-1] == quote {
		return string(quote) + canonical + string(quote)
	}
	return original.Lexeme
}

// splitStatements treats a top-level semicolon as a physical newline for
// canonical formatting. Semicolons inside (), [], or {} remain part of the
// expression; this preserves the compact brace form of iterator blocks.
func splitStatements(tokens []token.Token, initialDepth int) [][]token.Token {
	depth := initialDepth
	start := 0
	statements := [][]token.Token{}
	appendStatement := func(end int) {
		if end > start {
			statements = append(statements, tokens[start:end])
		}
	}
	for index, item := range tokens {
		if item.Lexeme == ";" && depth == 0 {
			appendStatement(index)
			start = index + 1
			continue
		}
		if strings.Contains(item.Lexeme, "\n") {
			continue
		}
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		}
	}
	appendStatement(len(tokens))
	return statements
}

func writeStatement(out *strings.Builder, code []token.Token, indent, continuation *int) {
	lineIndent := advanceIndentation(code, indent, continuation)
	out.WriteString(strings.Repeat(indentation, lineIndent))
	out.WriteString(formatTokensAt(code, lineIndent, false))
	out.WriteByte('\n')
}

func advanceIndentation(code []token.Token, indent, continuation *int) int {
	first := firstCode(code)
	dedent := isDedent(first)
	if dedent && *indent > 0 {
		*indent--
	}
	lineContinuation := *continuation
	if (first == "}" || first == ")" || first == "]") && lineContinuation > 0 {
		lineContinuation--
	}
	lineIndent := *indent + lineContinuation
	if lineIndent < 0 {
		lineIndent = 0
	}
	if dedent && isMidBlock(first) {
		*indent++
	} else if opensEndBlock(code, *continuation) {
		*indent++
	}
	*continuation += delimiterDelta(code)
	if *continuation < 0 {
		*continuation = 0
	}
	return lineIndent
}

func tokensByLine(tokens []token.Token) [][]token.Token {
	maxLine := 1
	for _, item := range tokens {
		if item.Kind != token.EOF && item.Span.Start.Line > maxLine {
			maxLine = item.Span.Start.Line
		}
	}
	lines := make([][]token.Token, maxLine)
	for _, item := range tokens {
		if item.Kind == token.EOF {
			continue
		}
		line := item.Span.Start.Line - 1
		lines[line] = append(lines[line], item)
	}
	return lines
}

func withoutNewline(tokens []token.Token) []token.Token {
	result := tokens[:0]
	for _, item := range tokens {
		if item.Kind != token.Newline {
			result = append(result, item)
		}
	}
	return result
}

func firstCode(tokens []token.Token) string {
	for _, item := range tokens {
		if item.Kind != token.Comment {
			return item.Lexeme
		}
	}
	return ""
}

func formatTokensAt(tokens []token.Token, baseIndent int, flatJSX bool) string {
	var out strings.Builder
	var previous *token.Token
	var beforePrevious *token.Token
	inBlockParameters := false
	lineKind := firstCode(tokens)
	importLine := lineKind == "import" || importFromLine(tokens)
	genericDepth := 0
	classInheritance := false
	ternaryQuestions, ternaryColons := ternaryTokenIndices(tokens)
	for i := range tokens {
		current := tokens[i]
		if current.Kind == token.Comment {
			if out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteString(current.Lexeme)
			beforePrevious = previous
			previous = &current
			continue
		}
		var next *token.Token
		for j := i + 1; j < len(tokens); j++ {
			if tokens[j].Kind != token.Comment {
				t := tokens[j]
				next = &t
				break
			}
		}
		if previous != nil {
			genericOpen := current.Lexeme == "<" && (startsUpper(previous.Lexeme) || genericApplicationOpen(tokens, i))
			if genericOpen && lineKind == "class" && genericDepth == 0 && !classInheritance {
				if !classTypeParameterOpen(tokens, i) {
					genericOpen = false
					classInheritance = true
				}
			}
			genericClosers := 0
			if current.Lexeme == ">" && genericDepth > 0 {
				genericClosers = 1
			} else if current.Lexeme == ">>" && genericDepth >= 2 {
				genericClosers = 2
			}
			openingPipe := current.Lexeme == "|" && (previous.Lexeme == "do" || previous.Lexeme == "{" || previous.Lexeme == "catch")
			closingPipe := current.Lexeme == "|" && inBlockParameters && !openingPipe
			space := needsSpace(beforePrevious, *previous, current, next)
			if current.Lexeme == "?" {
				space = ternaryQuestions[i]
			}
			if current.Lexeme == ":" && ternaryColons[i] || previous.Lexeme == ":" && ternaryColons[i-1] {
				space = true
			}
			if (current.Lexeme == "|" || previous.Lexeme == "|") && !openingPipe && !closingPipe && !inBlockParameters {
				space = true
			}
			if lineKind == "class" && current.Lexeme == "<" && !genericOpen {
				space = true
			}
			if lineKind == "class" && previous.Lexeme == "<" && genericDepth == 0 {
				space = true
			}
			if importLine && (previous.Lexeme == "/" || current.Lexeme == "/") {
				space = false
			}
			if inBlockParameters && previous.Lexeme == "|" {
				space = false
			}
			if closingPipe {
				space = false
			}
			if genericOpen || genericClosers > 0 || (genericDepth > 0 && previous.Lexeme == "<") {
				space = false
			}
			if !space && !importLine && tokenBoundaryChanges(*previous, current) {
				space = true
			}
			if space {
				out.WriteByte(' ')
			}
			if openingPipe {
				inBlockParameters = true
			} else if closingPipe {
				inBlockParameters = false
			}
			if genericOpen {
				genericDepth++
			}
			genericDepth -= genericClosers
		}
		lexeme := current.Lexeme
		if current.Kind == token.JSXLiteral {
			lexeme = formatJSXToken(current, baseIndent, flatJSX)
		} else if current.Kind == token.NativeIsland {
			lexeme = formatNativeToken(current, baseIndent)
		}
		out.WriteString(lexeme)
		beforePrevious = previous
		previous = &current
	}
	return strings.TrimSpace(out.String())
}

func tokenBoundaryChanges(previous, current token.Token) bool {
	if previous.Span.End.Offset == current.Span.Start.Offset {
		return false
	}
	if previous.Kind == token.NativeIsland || previous.Kind == token.JSXLiteral || current.Kind == token.NativeIsland || current.Kind == token.JSXLiteral {
		return false
	}
	combined, diagnostics := lexer.Lex([]byte("value " + previous.Lexeme + current.Lexeme + "\n"))
	if hasErrors(diagnostics) {
		return true
	}
	code := make([]token.Token, 0, 2)
	for _, item := range combined {
		if item.Kind != token.EOF && item.Kind != token.Newline && item.Kind != token.Comment {
			code = append(code, item)
		}
	}
	return len(code) != 3 || code[1].Kind != previous.Kind || code[1].Lexeme != previous.Lexeme || code[2].Kind != current.Kind || code[2].Lexeme != current.Lexeme
}

func formatNativeToken(item token.Token, baseIndent int) string {
	if !strings.Contains(item.Lexeme, "\n") {
		return item.Lexeme
	}
	lines := strings.Split(item.Lexeme, "\n")
	originalIndent := item.Span.Start.Column - 1
	prefix := strings.Repeat(indentation, baseIndent)
	for index := 1; index < len(lines); index++ {
		line := lines[index]
		removed := 0
		for removed < len(line) && removed < originalIndent && (line[removed] == ' ' || line[removed] == '\t') {
			removed++
		}
		lines[index] = prefix + line[removed:]
	}
	return strings.Join(lines, "\n")
}

func importFromLine(tokens []token.Token) bool {
	from := -1
	for index, item := range tokens {
		if item.Kind != token.Comment && item.Lexeme == "from" {
			from = index
			break
		}
	}
	if from < 0 {
		return false
	}
	for _, item := range tokens[from+1:] {
		if item.Kind != token.Comment && item.Lexeme == "/" {
			return true
		}
	}
	return false
}

func classTypeParameterOpen(tokens []token.Token, open int) bool {
	if open != 2 {
		return false
	}
	close := matchingTokenIndex(tokens, open, "<", ">")
	if close < 0 {
		return false
	}
	for index := open + 1; index < close; index++ {
		if tokens[index].Kind == token.Comment {
			continue
		}
		if tokens[index].Kind != token.Identifier && tokens[index].Lexeme != "," {
			return false
		}
	}
	return true
}

func matchingTokenIndex(tokens []token.Token, open int, opening, closing string) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Lexeme {
		case opening:
			depth++
		case closing:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func genericApplicationOpen(tokens []token.Token, open int) bool {
	depth := 0
	for index := open; index < len(tokens); index++ {
		if tokens[index].Kind == token.Comment {
			continue
		}
		switch tokens[index].Lexeme {
		case "<":
			depth++
		case ">":
			depth--
		case ">>":
			depth -= 2
		}
		if depth != 0 {
			continue
		}
		for next := index + 1; next < len(tokens); next++ {
			if tokens[next].Kind == token.Comment {
				continue
			}
			return tokens[next].Lexeme == "(" || tokens[next].Lexeme == "." || tokens[next].Lexeme == "::"
		}
		return false
	}
	return false
}

func needsSpace(beforePrevious *token.Token, previous, current token.Token, next *token.Token) bool {
	if previous.Kind == token.Comment {
		return false
	}
	if previous.Lexeme == "!" || previous.Lexeme == "~" {
		return false
	}
	if current.Lexeme == "?" {
		return true
	}
	if current.Lexeme == ":" {
		return current.Span.Start.Offset > previous.Span.End.Offset
	}
	if current.Lexeme == ".." || current.Lexeme == "..." || previous.Lexeme == ".." || previous.Lexeme == "..." {
		return false
	}
	if previous.Lexeme == "-" && current.Lexeme == ">" {
		return false
	}
	if current.Lexeme == "[" {
		switch previous.Lexeme {
		case ":", ":=", "=", ",", "return":
			return true
		default:
			return false
		}
	}
	if current.Lexeme == "<" && startsUpper(previous.Lexeme) {
		return false
	}
	if previous.Lexeme == "<" && beforePrevious != nil && startsUpper(beforePrevious.Lexeme) {
		return false
	}
	if current.Lexeme == ">" && beforePrevious != nil && beforePrevious.Lexeme == "<" {
		return false
	}
	if current.Lexeme == "}" {
		return previous.Lexeme != "{"
	}
	if current.Lexeme == ")" || current.Lexeme == "]" || current.Lexeme == "," || current.Lexeme == ";" || current.Lexeme == "." || current.Lexeme == "&." || current.Lexeme == "::" {
		return false
	}
	if previous.Lexeme == "::" {
		return beforePrevious != nil && startsLower(beforePrevious.Lexeme)
	}
	if previous.Lexeme == "(" || previous.Lexeme == "[" || previous.Lexeme == "." || previous.Lexeme == "&." {
		return false
	}
	if previous.Lexeme == "," || previous.Lexeme == ";" {
		return true
	}
	if previous.Lexeme == ":" {
		// :symbol has no space; type annotations, mode declarations, keyword
		// arguments, and hash labels do.
		return !isSymbolColon(beforePrevious, previous, current.Lexeme)
	}
	if current.Lexeme == "(" {
		if (previous.Lexeme == ">" || previous.Lexeme == ">>") && beforePrevious != nil && startsUpper(beforePrevious.Lexeme) {
			return false
		}
		return previous.Lexeme == "if" || previous.Lexeme == "while" || previous.Lexeme == "unless" || previous.Lexeme == "until" || previous.Lexeme == "return" || previous.Lexeme == "," || isOperator(previous.Lexeme)
	}
	if current.Lexeme == "{" {
		return previous.Lexeme != "(" && previous.Lexeme != "["
	}
	if previous.Lexeme == "{" {
		return true
	}
	if previous.Lexeme == "|" || current.Lexeme == "|" {
		if previous.Span.End.Offset == current.Span.Start.Offset {
			return false
		}
		return true
	}
	if isUnary(previous.Lexeme, current.Lexeme) {
		return false
	}
	if isOperator(previous.Lexeme) || isOperator(current.Lexeme) {
		return true
	}
	if next != nil && current.Lexeme == "*" && (next.Kind == token.Identifier) {
		return false
	}
	return true
}

func startsUpper(s string) bool {
	return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z'
}

func startsLower(s string) bool {
	return len(s) > 0 && ((s[0] >= 'a' && s[0] <= 'z') || s[0] == '_')
}

func isSymbolColon(before *token.Token, colon token.Token, next string) bool {
	if next == "" {
		return false
	}
	if before == nil {
		return true
	}
	if isOperator(before.Lexeme) {
		return true
	}
	if colon.Span.Start.Offset > before.Span.End.Offset {
		return true
	}
	switch before.Lexeme {
	case "(", "[", "{", ",", "=", ":=", "=>", "return":
		return true
	}
	return false
}

func isOperator(s string) bool {
	switch s {
	case ":=", "=", "+", "-", "*", "/", "%", "**", "==", "!=", "<", ">", "<=", ">=", "<=>", "=~", "!~", "&&", "||", "=>", "+=", "-=", "*=", "/=", "||=", "&&=", "|", "&", "^", "..", "...":
		return true
	}
	return false
}

func ternaryTokenIndices(tokens []token.Token) (map[int]bool, map[int]bool) {
	questionIndices := map[int]bool{}
	colonIndices := map[int]bool{}
	type question struct {
		index int
		depth int
	}
	questions := []question{}
	depth := 0
	for index, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		case "?":
			if nullableQuestionToken(tokens, index) {
				continue
			}
			questions = append(questions, question{index: index, depth: depth})
		case ":":
			if ternaryPrefixColon(tokens, index) {
				continue
			}
			for candidate := len(questions) - 1; candidate >= 0; candidate-- {
				if questions[candidate].depth != depth {
					continue
				}
				questionIndices[questions[candidate].index] = true
				colonIndices[index] = true
				questions = append(questions[:candidate], questions[candidate+1:]...)
				break
			}
		}
	}
	return questionIndices, colonIndices
}

func nullableQuestionToken(tokens []token.Token, index int) bool {
	if index+1 >= len(tokens) {
		return true
	}
	switch tokens[index+1].Lexeme {
	case ",", ")", "]", "}", "=", ":=", "|", ">", ">>":
		return true
	}
	return false
}

func ternaryPrefixColon(tokens []token.Token, index int) bool {
	if index == 0 {
		return true
	}
	switch tokens[index-1].Lexeme {
	case "?", "(", "[", "{", ",", "=", ":=", "=>", "return":
		return true
	}
	return isOperator(tokens[index-1].Lexeme)
}

func isUnary(previous, current string) bool {
	if current != "!" && current != "~" && current != "+" && current != "-" {
		return false
	}
	return previous == "(" || previous == "[" || previous == "{" || previous == "," || previous == "=" || previous == ":="
}

func isDedent(first string) bool {
	switch first {
	case "end", "else", "elsif", "when", "rescue", "ensure":
		return true
	}
	return false
}

func isMidBlock(first string) bool {
	switch first {
	case "else", "elsif", "when", "rescue", "ensure":
		return true
	}
	return false
}

func opensEndBlock(tokens []token.Token, initialDepth int) bool {
	first := firstCode(tokens)
	switch first {
	case "class", "record", "enum", "module", "interface", "def", "if", "unless", "case", "begin", "while", "until", "for":
		return true
	}
	conditionalTransferIf := -1
	if first == "return" || first == "break" || first == "next" {
		depth := 0
		for index, item := range tokens {
			switch item.Lexeme {
			case "(", "[", "{":
				depth++
			case ")", "]", "}":
				if depth > 0 {
					depth--
				}
			case "if":
				if depth == 0 {
					conditionalTransferIf = index
				}
			}
		}
	}
	for index, item := range tokens {
		if item.Lexeme == "case" || item.Lexeme == "if" && index != conditionalTransferIf {
			return true
		}
	}
	depth := initialDepth
	for index, item := range tokens {
		switch item.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case "do":
			if depth == 0 {
				return true
			}
		case "fn":
			if depth == 0 {
				return true
			}
		case "catch":
			if depth == 0 {
				for next := index + 1; next < len(tokens); next++ {
					if tokens[next].Kind == token.Comment {
						continue
					}
					if tokens[next].Lexeme == "|" {
						return true
					}
					break
				}
			}
		}
	}
	return false
}

func delimiterDelta(tokens []token.Token) int {
	delta := 0
	for _, item := range tokens {
		if strings.Contains(item.Lexeme, "\n") {
			continue
		}
		switch item.Lexeme {
		case "(", "[", "{":
			delta++
		case ")", "]", "}":
			delta--
		}
	}
	return delta
}

func hasErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}
