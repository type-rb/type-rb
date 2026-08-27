// Package parser implements TypeRB's handwritten recursive-descent and Pratt
// parsers. The syntax tree is independent from parser internals and retains
// source spans for diagnostics, formatting, and Ruby interoperability.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/lexer"
	"github.com/type-rb/type-rb/internal/token"
)

type Parser struct {
	source        []byte
	tokens        []token.Token
	pos           int
	diags         []diagnostic.Diagnostic
	nativeIslands []ast.NativeIsland
}

func Parse(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
	tokens, lexDiags := lexer.Lex(source)
	p := &Parser{source: source, tokens: tokens, diags: append([]diagnostic.Diagnostic(nil), lexDiags...)}
	program := &ast.Program{Tokens: tokens}
	if len(tokens) > 0 {
		program.SourceSpan.Start = tokens[0].Span.Start
	}
	program.Statements = p.parseStatements(nil)
	program.NativeIslands = p.nativeIslands
	if len(tokens) > 0 {
		program.SourceSpan.End = tokens[len(tokens)-1].Span.End
	}
	for _, stmt := range program.Statements {
		if n, ok := stmt.(*ast.NativeStatement); ok {
			trimmed := strings.TrimSpace(n.Text)
			if strings.HasPrefix(trimmed, "mode:") {
				p.errorAt(n.Span(), "mode belongs in trbconfig.jsonc, not in source files")
			}
			if strings.HasPrefix(trimmed, "package ") {
				p.errorAt(n.Span(), "package is derived from trbconfig.jsonc and the source path, not declared in source files")
			}
		}
	}
	return program, diagnostic.Normalize(p.diags, "", diagnostic.SyntaxError)
}

func (p *Parser) parseStatements(stop map[string]bool) []ast.Statement {
	var statements []ast.Statement
	for !p.atEOF() {
		p.skipSeparators()
		if p.atEOF() {
			break
		}
		if stop != nil && stop[p.current().Lexeme] && p.atStatementStart() {
			break
		}
		stmt := p.parseStatement()
		if stmt != nil {
			statements = append(statements, stmt)
		} else if !p.atEOF() {
			p.pos++
		}
	}
	return statements
}

func (p *Parser) parseStatement() ast.Statement {
	if p.current().Kind == token.Comment {
		t := p.current()
		p.pos++
		return &ast.CommentStatement{Base: ast.Base{SourceSpan: t.Span}, Text: t.Lexeme}
	}
	word := p.current().Lexeme
	switch word {
	case "class":
		start, end, _, _ := p.logicalLine(p.pos)
		line := p.codeTokens(start, end)
		if len(line) > 1 && line[1].Lexeme == "<<" {
			return p.parseNativeBlock()
		}
		return p.parseClass()
	case "record":
		return p.parseRecord()
	case "enum":
		return p.parseEnum()
	case "alias":
		return p.parseTypeAlias()
	case "newtype":
		return p.parseNewtype()
	case "type":
		statement := p.parseTypeAlias()
		p.migrationErrorAt(statement.Span(), typeAliasMovedMessage)
		return statement
	case "module":
		return p.parseModule()
	case "interface":
		return p.parseInterface()
	case "def":
		start, end, _, _ := p.logicalLine(p.pos)
		line := p.codeTokens(start, end)
		if isEndlessDefinition(line) {
			return p.parseNativeLine()
		}
		return p.parseMethod()
	case "if":
		return p.parseIf()
	case "case":
		return p.parseCase()
	case "while":
		return p.parseWhile()
	case "break", "next":
		return p.parseLoopControl(word)
	case "import":
		return p.parseImport()
	}

	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	if len(line) == 0 {
		p.pos = next
		return nil
	}
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	if word == "return" {
		if conditional := p.tryConditionalReturn(line, next, base); conditional != nil {
			return conditional
		}
	}
	if unsupported := p.tryUnsupportedTrailingCondition(line, next, base); unsupported != nil {
		return unsupported
	}
	if catch := p.tryCatchBlockStatement(line, next, base); catch != nil {
		return catch
	}
	if lambda := p.tryLambdaExpressionStatement(line, next, base); lambda != nil {
		return lambda
	}
	if attempt := p.tryAttemptBlockStatement(line, next, base); attempt != nil {
		return attempt
	}
	if expression := p.tryControlFlowExpressionStatement(line, next, base); expression != nil {
		return expression
	}
	if blockOperation := p.tryIterationStatement(line, next, base); blockOperation != nil {
		return blockOperation
	}
	if callBlock := p.tryCallBlockStatement(line, next, base); callBlock != nil {
		return callBlock
	}
	if word == "return" {
		return p.parseReturn()
	}

	if field := p.tryField(line, base); field != nil {
		p.pos = next
		return field
	}
	if variable := p.tryVariable(line, base); variable != nil {
		p.pos = next
		return variable
	}
	if assignment := p.tryAssignment(line, base); assignment != nil {
		p.pos = next
		return assignment
	}
	if p.opensNativeBlock(line) {
		return p.parseNativeBlock()
	}
	if expression, ok := p.parseExpression(line); ok {
		p.pos = next
		return &ast.ExpressionStatement{Base: base, Expression: expression}
	}

	p.pos = next
	return p.nativeStatement(base)
}

func (p *Parser) tryCatchBlockStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	catchAt := topLevelIndex(line, "catch")
	if catchAt <= 0 {
		return nil
	}
	prefix := line[:catchAt]
	wrapper, valueTokens := expressionWrapper(prefix)
	value, ok := p.parseExpression(valueTokens)
	if !ok {
		return nil
	}

	header := p.parseCatchHeader(line[catchAt], line[catchAt+1:], base.TrailingComment)
	p.pos = next
	value = p.parseCatchBody(value, header)
	base.SourceSpan.End = value.Span().End
	return p.wrapExpression(prefix, wrapper, base, value)
}

func (p *Parser) tryLambdaExpressionStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	fnAt := topLevelIndex(line, "fn")
	if fnAt < 0 || fnAt+1 >= len(line) || line[fnAt+1].Lexeme != "(" {
		return nil
	}
	close := matchingIndex(line, fnAt+1, "(", ")")
	if close < 0 {
		p.errorAt(spanOf(line[fnAt:]), "unterminated fn parameters; expected )")
		p.pos = next
		return &ast.ExpressionStatement{Base: base, Expression: &ast.LambdaExpression{Base: ast.Base{SourceSpan: spanOf(line[fnAt:])}}}
	}
	node := &ast.LambdaExpression{
		Base:       ast.Base{SourceSpan: token.Span{Start: line[fnAt].Span.Start, End: line[close].Span.End}},
		Parameters: p.parseParameters(line[fnAt+2 : close]),
	}
	tail := line[close+1:]
	if len(tail) > 0 {
		failsAt := topLevelIndex(tail, "fails")
		returnTail := tail
		if failsAt >= 0 {
			p.migrationErrorAt(tail[failsAt].Span, failsRemovedMessage)
			returnTail = tail[:failsAt]
			if failsAt+1 < len(tail) {
				node.Fails = p.parseTypeRef(tail[failsAt+1:])
			}
		}
		if len(returnTail) > 0 {
			if returnTail[0].Lexeme != ":" || len(returnTail) == 1 {
				p.errorAt(spanOf(returnTail), "fn return type must be written as : Type")
			} else {
				node.ReturnType = p.parseReturnType(returnTail[1:])
			}
		}
	}

	p.pos = next
	node.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
	base.SourceSpan.End = closeSpan.End

	wrapper := append([]token.Token(nil), line[:fnAt+1]...)
	embedded := map[int]ast.Expression{line[fnAt].Span.Start.Offset: node}
	if len(wrapper) > 0 && wrapper[0].Lexeme == "return" {
		if value, valid := p.parseExpressionWithEmbedded(wrapper[1:], embedded); valid {
			return &ast.ReturnStatement{Base: base, Value: value}
		}
	}
	if statement := p.tryVariableWithEmbedded(wrapper, base, embedded); statement != nil {
		return statement
	}
	if statement := p.tryAssignmentWithEmbedded(wrapper, base, embedded); statement != nil {
		return statement
	}
	if value, valid := p.parseExpressionWithEmbedded(wrapper, embedded); valid {
		return &ast.ExpressionStatement{Base: base, Expression: value}
	}
	p.errorAt(base.SourceSpan, "fn is not valid in this expression context")
	return &ast.ExpressionStatement{Base: base, Expression: node}
}

func (p *Parser) tryAttemptBlockStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	attemptAt := topLevelIndex(line, "attempt")
	if attemptAt < 0 || attemptAt+1 >= len(line) || line[attemptAt+1].Lexeme != "do" {
		return nil
	}
	if attemptAt > 0 && (line[attemptAt-1].Lexeme == "." || line[attemptAt-1].Lexeme == "&." || line[attemptAt-1].Lexeme == "::") {
		return nil
	}
	p.migrationErrorAt(line[attemptAt].Span, attemptRemovedMessage)
	if attemptAt+2 != len(line) {
		p.errorAt(spanOf(line[attemptAt+2:]), "attempt block does not take parameters or trailing expressions")
	}

	p.pos = next
	node := &ast.AttemptExpression{Base: ast.Base{SourceSpan: token.Span{Start: line[attemptAt].Span.Start, End: line[len(line)-1].Span.End}}}
	node.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
	base.SourceSpan.End = closeSpan.End

	wrapper := append([]token.Token(nil), line[:attemptAt+1]...)
	embedded := map[int]ast.Expression{line[attemptAt].Span.Start.Offset: node}
	if len(wrapper) > 0 && wrapper[0].Lexeme == "return" {
		value, valid := p.parseExpressionWithEmbedded(wrapper[1:], embedded)
		if valid {
			return &ast.ReturnStatement{Base: base, Value: value}
		}
	}
	if statement := p.tryVariableWithEmbedded(wrapper, base, embedded); statement != nil {
		return statement
	}
	if statement := p.tryAssignmentWithEmbedded(wrapper, base, embedded); statement != nil {
		return statement
	}
	if value, valid := p.parseExpressionWithEmbedded(wrapper, embedded); valid {
		return &ast.ExpressionStatement{Base: base, Expression: value}
	}
	p.errorAt(base.SourceSpan, "attempt block is not valid in this expression context")
	return &ast.ExpressionStatement{Base: base, Expression: node}
}

func (p *Parser) tryCallBlockStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	blockAt := topLevelIndex(line, "do")
	brace := false
	if blockAt <= 0 {
		blockAt = topLevelIndex(line, "{")
		brace = blockAt > 0
	}
	if blockAt <= 0 {
		return nil
	}
	prefix := line[:blockAt]
	wrapper, header := expressionWrapper(prefix)
	expression, ok := p.parseExpression(header)
	if !ok {
		return nil
	}
	call, syntheticCall, ok := callBlockTarget(expression)
	if !ok {
		return nil
	}
	if !brace {
		parameters := []string(nil)
		valid := true
		if blockAt+1 < len(line) {
			parameters, valid = p.blockParameters(line[blockAt+1:])
		}
		if !valid {
			if syntheticCall && blockAt+1 == len(line) {
				return nil
			}
			p.errorAt(spanOf(line[blockAt:]), "call block parameters must be written as |name, ...|")
		}
		p.pos = next
		block := &ast.BlockExpression{Base: ast.Base{SourceSpan: spanOf(line[blockAt:])}, Parameters: parameters}
		block.Body = p.parseStatements(map[string]bool{"end": true})
		closeSpan, catchHeader := p.consumeBlockTerminator()
		block.SourceSpan.End = closeSpan.End
		call.SourceSpan.End = closeSpan.End
		call.Block = block
		extendWrapperSpan(expression, closeSpan.End)
		if catchHeader != nil {
			expression = p.parseCatchBody(expression, *catchHeader)
		}
		base.SourceSpan.End = expression.Span().End
		return p.wrapExpression(prefix, wrapper, base, expression)
	}

	close := matchingIndex(line, blockAt, "{", "}")
	if close < 0 {
		p.errorAt(line[blockAt].Span, "unterminated call block; expected }")
		p.pos = next
		return p.wrapExpression(prefix, wrapper, base, expression)
	}
	firstPipe, secondPipe := -1, -1
	for index := blockAt + 1; index < close; index++ {
		if line[index].Lexeme != "|" {
			continue
		}
		if firstPipe < 0 {
			firstPipe = index
		} else {
			secondPipe = index
			break
		}
	}
	parameters := []string(nil)
	if firstPipe != blockAt+1 || secondPipe < 0 {
		if syntheticCall {
			return nil
		}
		p.errorAt(spanOf(line[blockAt:close+1]), "call block parameters must be written as |name, ...|")
	} else if parsed, valid := p.blockParameters(line[firstPipe : secondPipe+1]); valid {
		parameters = parsed
	} else {
		p.errorAt(spanOf(line[firstPipe:secondPipe+1]), "call block parameters must be identifiers")
	}
	block := &ast.BlockExpression{Base: ast.Base{SourceSpan: token.Span{Start: line[blockAt].Span.Start, End: line[close].Span.End}}, Parameters: parameters, Brace: true}
	if secondPipe >= 0 {
		for _, part := range splitTopLevel(line[secondPipe+1:close], ";") {
			if len(part) == 0 {
				continue
			}
			statement := p.inlineBlockStatement(part)
			if statement == nil {
				p.errorAt(spanOf(part), "unsupported statement in inline call block")
				continue
			}
			block.Body = append(block.Body, statement)
		}
	}
	call.SourceSpan.End = line[close].Span.End
	call.Block = block
	extendWrapperSpan(expression, line[close].Span.End)
	base.SourceSpan.End = line[close].Span.End
	p.pos = next
	return p.wrapExpression(prefix, wrapper, base, expression)
}

func (p *Parser) wrapExpression(prefix []token.Token, wrapper string, base ast.Base, expression ast.Expression) ast.Statement {
	switch wrapper {
	case "return":
		return &ast.ReturnStatement{Base: base, Value: expression}
	case "variable":
		statement, ok := p.tryVariable(prefix, base).(*ast.VariableStatement)
		if ok {
			statement.Value = expression
			return statement
		}
	case "assignment":
		statement, ok := p.tryAssignment(prefix, base).(*ast.AssignmentStatement)
		if ok {
			statement.Value = expression
			return statement
		}
	}
	return &ast.ExpressionStatement{Base: base, Expression: expression}
}

func expressionWrapper(prefix []token.Token) (string, []token.Token) {
	if len(prefix) == 0 {
		return "expression", nil
	}
	if prefix[0].Lexeme == "return" {
		return "return", prefix[1:]
	}
	if assign := topLevelIndex(prefix, ":="); assign > 0 {
		return "variable", prefix[assign+1:]
	}
	if assign := topLevelIndex(prefix, "="); assign > 0 {
		return "assignment", prefix[assign+1:]
	}
	return "expression", prefix
}

func callBlockTarget(expression ast.Expression) (*ast.CallExpression, bool, bool) {
	current := expression
	var replace func(ast.Expression)
	wrapped := false
	for {
		switch node := current.(type) {
		case *ast.AttemptExpression:
			wrapped = true
			replace = func(value ast.Expression) { node.Value = value }
			current = node.Value
		case *ast.TryExpression:
			wrapped = true
			replace = func(value ast.Expression) { node.Value = value }
			current = node.Value
		default:
			goto unwrapped
		}
	}

unwrapped:
	if call, ok := current.(*ast.CallExpression); ok {
		return call, false, true
	}
	if !wrapped {
		return nil, false, false
	}
	switch current.(type) {
	case *ast.Identifier, *ast.MemberExpression:
		call := &ast.CallExpression{Base: ast.Base{SourceSpan: current.Span()}, Callee: current}
		replace(call)
		return call, true, true
	default:
		return nil, false, false
	}
}

func extendWrapperSpan(expression ast.Expression, end token.Position) {
	for {
		switch node := expression.(type) {
		case *ast.AttemptExpression:
			node.SourceSpan.End = end
			expression = node.Value
		case *ast.TryExpression:
			node.SourceSpan.End = end
			expression = node.Value
		default:
			return
		}
	}
}

type catchHeader struct {
	ast.Base
	Binding ast.PatternBinding
}

func (p *Parser) parseCatchHeader(keyword token.Token, tokens []token.Token, comment string) catchHeader {
	end := keyword.Span.End
	if len(tokens) > 0 {
		end = tokens[len(tokens)-1].Span.End
	}
	header := catchHeader{Base: ast.Base{SourceSpan: token.Span{Start: keyword.Span.Start, End: end}, TrailingComment: comment}}
	if len(tokens) == 3 && tokens[0].Lexeme == "|" && tokens[1].Kind == token.Identifier && tokens[2].Lexeme == "|" {
		header.Binding = ast.PatternBinding{Base: ast.Base{SourceSpan: tokens[1].Span}, Name: tokens[1].Lexeme}
		return header
	}

	span := header.SourceSpan
	p.errorAt(span, "catch binding must be written as |error|")
	for _, item := range tokens {
		if item.Kind == token.Identifier {
			header.Binding = ast.PatternBinding{Base: ast.Base{SourceSpan: item.Span}, Name: item.Lexeme}
			break
		}
	}
	return header
}

func (p *Parser) parseCatchBody(value ast.Expression, header catchHeader) ast.Expression {
	node := &ast.CatchExpression{
		Base:    ast.Base{SourceSpan: token.Span{Start: value.Span().Start, End: header.SourceSpan.End}, TrailingComment: header.TrailingComment},
		Value:   value,
		Binding: header.Binding,
	}
	node.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
	return node
}

func (p *Parser) consumeBlockTerminator() (token.Span, *catchHeader) {
	if p.atEOF() || p.current().Lexeme != "end" {
		_, span := p.consumeTerminator("end")
		return span, nil
	}
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	if len(line) >= 2 && line[1].Lexeme == "catch" {
		header := p.parseCatchHeader(line[1], line[2:], comment)
		p.pos = next
		return line[0].Span, &header
	}
	_, span := p.consumeTerminator("end")
	return span, nil
}

func (p *Parser) tryControlFlowExpressionStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	controlAt := -1
	construct := ""
	for index, item := range line {
		if index > 0 && (item.Lexeme == "case" || item.Lexeme == "if") {
			controlAt = index
			construct = item.Lexeme
			break
		}
	}
	if controlAt < 0 {
		return nil
	}

	controlPosition := -1
	for index := p.pos; index < len(p.tokens); index++ {
		if p.tokens[index].Span.Start.Offset == line[controlAt].Span.Start.Offset {
			controlPosition = index
			break
		}
	}
	if controlPosition < 0 {
		return nil
	}

	p.pos = controlPosition
	var controlNode ast.Expression
	if construct == "case" {
		controlNode, _ = p.parseCase().(*ast.CaseStatement)
	} else {
		controlNode, _ = p.parseIf().(*ast.IfStatement)
	}
	if controlNode == nil {
		return nil
	}
	parsedNext := p.pos

	wrapped := append([]token.Token(nil), line[:controlAt]...)
	wrapped = append(wrapped, line[controlAt])
	for _, item := range line[controlAt+1:] {
		if item.Span.Start.Offset >= controlNode.Span().End.Offset {
			wrapped = append(wrapped, item)
		}
	}
	base.SourceSpan.End = controlNode.Span().End
	if len(wrapped) > 0 && wrapped[len(wrapped)-1].Span.End.Offset > base.SourceSpan.End.Offset {
		base.SourceSpan.End = wrapped[len(wrapped)-1].Span.End
	}
	embedded := map[int]ast.Expression{line[controlAt].Span.Start.Offset: controlNode}

	var statement ast.Statement
	if len(wrapped) > 0 && wrapped[0].Lexeme == "return" {
		value, valid := p.parseExpressionWithEmbedded(wrapped[1:], embedded)
		if valid {
			statement = &ast.ReturnStatement{Base: base, Value: value}
		}
	}
	if statement == nil {
		statement = p.tryVariableWithEmbedded(wrapped, base, embedded)
	}
	if statement == nil {
		statement = p.tryAssignmentWithEmbedded(wrapped, base, embedded)
	}
	if statement == nil {
		if value, valid := p.parseExpressionWithEmbedded(wrapped, embedded); valid {
			statement = &ast.ExpressionStatement{Base: base, Expression: value}
		}
	}
	if statement == nil {
		p.errorAt(base.SourceSpan, construct+" expression is not valid in this expression context")
		statement = &ast.ExpressionStatement{Base: base, Expression: controlNode}
	}
	if next > parsedNext {
		p.pos = next
	} else {
		p.pos = parsedNext
	}
	return statement
}

func (p *Parser) tryIterationStatement(line []token.Token, next int, base ast.Base) ast.Statement {
	blockAt := topLevelIndex(line, "do")
	if blockAt <= 0 {
		blockAt = topLevelIndex(line, "{")
	}
	if blockAt <= 0 {
		return nil
	}

	prefix := line[:blockAt]
	header := prefix
	wrapper := "expression"
	if prefix[0].Lexeme == "return" {
		wrapper = "return"
		header = prefix[1:]
	} else if assign := topLevelIndex(prefix, ":="); assign > 0 {
		wrapper = "variable"
		header = prefix[assign+1:]
	} else if assign := topLevelIndex(prefix, "="); assign > 0 {
		wrapper = "assignment"
		header = prefix[assign+1:]
	}
	iteration, ok := p.iterationHeader(header)
	if !ok {
		return nil
	}
	iteration = p.parseIterationBlock(line, next, base, iteration)
	switch wrapper {
	case "return":
		return &ast.ReturnStatement{Base: iteration.Base, Value: iteration}
	case "variable":
		statement, ok := p.tryVariable(prefix, iteration.Base).(*ast.VariableStatement)
		if !ok {
			return &ast.ExpressionStatement{Base: iteration.Base, Expression: iteration}
		}
		statement.Value = iteration
		return statement
	case "assignment":
		statement, ok := p.tryAssignment(prefix, iteration.Base).(*ast.AssignmentStatement)
		if !ok {
			return &ast.ExpressionStatement{Base: iteration.Base, Expression: iteration}
		}
		statement.Value = iteration
		return statement
	default:
		return &ast.ExpressionStatement{Base: iteration.Base, Expression: iteration}
	}
}

func (p *Parser) parseIterationBlock(line []token.Token, next int, base ast.Base, iteration *ast.IterationExpression) *ast.IterationExpression {
	if doAt := topLevelIndex(line, "do"); doAt > 0 {
		parameters, ok := p.blockParameters(line[doAt+1:])
		if !ok {
			p.errorAt(spanOf(line[doAt:]), "iteration block parameters must be written as |name, ...|")
		}
		p.pos = next
		block := &ast.BlockExpression{Base: ast.Base{SourceSpan: spanOf(line[doAt:])}, Parameters: parameters}
		block.Body = p.parseStatements(map[string]bool{"end": true})
		_, closeSpan := p.consumeTerminator("end")
		block.SourceSpan.End = closeSpan.End
		iteration.Base = base
		iteration.SourceSpan.End = closeSpan.End
		iteration.Block = block
		return iteration
	}

	braceAt := topLevelIndex(line, "{")
	if braceAt <= 0 {
		return iteration
	}
	close := matchingIndex(line, braceAt, "{", "}")
	if close < 0 {
		p.errorAt(line[braceAt].Span, "unterminated iteration block; expected }")
		p.pos = next
		return iteration
	}
	parameterEnd := -1
	for index := braceAt + 1; index < close; index++ {
		if line[index].Lexeme == "|" {
			parameterEnd = index
			break
		}
	}
	if parameterEnd != braceAt+1 {
		p.errorAt(spanOf(line[braceAt:close+1]), "iteration block parameters must start with |")
	}
	secondPipe := -1
	for index := parameterEnd + 1; parameterEnd >= 0 && index < close; index++ {
		if line[index].Lexeme == "|" {
			secondPipe = index
			break
		}
	}
	parameters := []string(nil)
	if secondPipe < 0 {
		p.errorAt(spanOf(line[braceAt:close+1]), "iteration block parameters must end with |")
	} else if parsed, valid := p.blockParameters(line[parameterEnd : secondPipe+1]); valid {
		parameters = parsed
	} else {
		p.errorAt(spanOf(line[parameterEnd:secondPipe+1]), "iteration block parameters must be identifiers")
	}
	block := &ast.BlockExpression{Base: ast.Base{SourceSpan: token.Span{Start: line[braceAt].Span.Start, End: line[close].Span.End}}, Parameters: parameters, Brace: true}
	if secondPipe >= 0 {
		for _, part := range splitTopLevel(line[secondPipe+1:close], ";") {
			if len(part) == 0 {
				continue
			}
			statement := p.inlineBlockStatement(part)
			if statement == nil {
				p.errorAt(spanOf(part), "unsupported statement in inline iteration block")
				continue
			}
			block.Body = append(block.Body, statement)
		}
	}
	iteration.Base = base
	iteration.Block = block
	p.pos = next
	return iteration
}

func (p *Parser) inlineBlockStatement(line []token.Token) ast.Statement {
	base := ast.Base{SourceSpan: spanOf(line)}
	if line[0].Lexeme == "return" {
		value, ok := p.parseExpression(line[1:])
		if len(line) > 1 && !ok {
			return nil
		}
		return &ast.ReturnStatement{Base: base, Value: value}
	}
	if line[0].Lexeme == "break" || line[0].Lexeme == "next" {
		if len(line) != 1 {
			p.errorAt(spanOf(line), fmt.Sprintf("%s does not take a value", line[0].Lexeme))
		}
		if line[0].Lexeme == "break" {
			return &ast.BreakStatement{Base: base}
		}
		return &ast.NextStatement{Base: base}
	}
	if variable := p.tryVariable(line, base); variable != nil {
		return variable
	}
	if assignment := p.tryAssignment(line, base); assignment != nil {
		return assignment
	}
	if expression, ok := p.parseExpression(line); ok {
		return &ast.ExpressionStatement{Base: base, Expression: expression}
	}
	return nil
}

func (p *Parser) blockParameters(tokens []token.Token) ([]string, bool) {
	if len(tokens) < 2 || tokens[0].Lexeme != "|" || tokens[len(tokens)-1].Lexeme != "|" {
		return nil, false
	}
	parts := splitTopLevel(tokens[1:len(tokens)-1], ",")
	parameters := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) != 1 || part[0].Kind != token.Identifier {
			return nil, false
		}
		parameters = append(parameters, part[0].Lexeme)
	}
	return parameters, len(parameters) > 0
}

func (p *Parser) iterationHeader(tokens []token.Token) (*ast.IterationExpression, bool) {
	expression, ok := p.parseExpression(tokens)
	if !ok {
		return nil, false
	}
	withIndex := false
	if member, ok := expression.(*ast.MemberExpression); ok && member.Name == "with_index" {
		withIndex = true
		expression = member.Receiver
	} else if call, ok := expression.(*ast.CallExpression); ok {
		if member, memberOK := call.Callee.(*ast.MemberExpression); memberOK && member.Name == "with_index" {
			if len(call.Arguments) != 0 {
				p.errorAt(call.Span(), "with_index does not take arguments in TypeRB v0.1")
			}
			withIndex = true
			expression = member.Receiver
		}
	}

	iteration := &ast.IterationExpression{WithIndex: withIndex}
	switch node := expression.(type) {
	case *ast.MemberExpression:
		if !portableIterationOperation(node.Name) {
			return nil, false
		}
		iteration.Source = node.Receiver
		iteration.Operation = node.Name
	case *ast.CallExpression:
		member, memberOK := node.Callee.(*ast.MemberExpression)
		if !memberOK || !portableIterationOperation(member.Name) {
			return nil, false
		}
		iteration.Source = member.Receiver
		iteration.Operation = member.Name
		if member.Name == "each" || member.Name == "map" || member.Name == "select" || member.Name == "any?" || member.Name == "all?" || member.Name == "none?" || member.Name == "find" || member.Name == "find_index" || member.Name == "sort_by" || member.Name == "sort_by_descending" {
			if len(node.Arguments) != 0 {
				p.errorAt(node.Span(), member.Name+" does not take arguments")
			}
		} else if member.Name == "concurrent_map" {
			if len(node.Arguments) != 0 && (len(node.Arguments) != 1 || node.Arguments[0].Name != "limit" || node.Arguments[0].Splat != "") {
				p.errorAt(node.Span(), "concurrent_map accepts only the named argument limit")
			} else if len(node.Arguments) == 1 {
				iteration.Limit = node.Arguments[0].Value
			}
		} else if member.Name == "reduce" {
			if len(node.Arguments) != 1 || node.Arguments[0].Name != "" || node.Arguments[0].Splat != "" {
				p.errorAt(node.Span(), "reduce expects exactly one positional initial value")
			} else {
				iteration.Initial = node.Arguments[0].Value
			}
		} else if len(node.Arguments) != 1 || node.Arguments[0].Name != "" || node.Arguments[0].Splat != "" {
			p.errorAt(node.Span(), "each_slice expects exactly one positional size argument")
		} else {
			iteration.SliceSize = node.Arguments[0].Value
		}
	default:
		return nil, false
	}
	return iteration, true
}

func portableIterationOperation(name string) bool {
	switch name {
	case "each", "each_slice", "map", "concurrent_map", "select", "reduce", "any?", "all?", "none?", "find", "find_index", "sort_by", "sort_by_descending":
		return true
	default:
		return false
	}
}

func (p *Parser) parseRecord() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	record := &ast.RecordStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	record.Name, record.TypeParameters = p.parseGenericDeclaration(line, "record")
	p.pos = next
	for !p.atEOF() {
		p.skipSeparators()
		if p.current().Lexeme == "end" {
			break
		}
		if p.current().Kind == token.Comment {
			t := p.current()
			p.pos++
			record.Body = append(record.Body, &ast.CommentStatement{Base: ast.Base{SourceSpan: t.Span}, Text: t.Lexeme})
			continue
		}
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		field := p.parseRecordField(parts, trailing)
		if field == nil {
			p.errorAt(spanOf(parts), "record body may only contain typed fields")
		} else {
			record.Body = append(record.Body, field)
		}
		p.pos = nx
	}
	_, closeSpan := p.consumeTerminator("end")
	record.SourceSpan.End = closeSpan.End
	return record
}

func (p *Parser) parseEnum() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	node := &ast.EnumStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	node.Name, node.TypeParameters = p.parseGenericDeclaration(line, "enum")
	p.pos = next
	methodsStarted := false
	for !p.atEOF() {
		p.skipSeparators()
		if p.atEOF() || p.current().Lexeme == "end" {
			break
		}
		if p.current().Kind == token.Comment {
			t := p.current()
			p.pos++
			node.Body = append(node.Body, &ast.CommentStatement{Base: ast.Base{SourceSpan: t.Span}, Text: t.Lexeme})
			continue
		}
		if p.current().Lexeme == "def" {
			methodsStarted = true
			method := p.parseMethod()
			if typed, ok := method.(*ast.MethodStatement); ok && typed.Class {
				p.errorAt(typed.Span(), "enum methods must be instance methods")
			}
			node.Body = append(node.Body, method)
			continue
		}
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		member := p.parseEnumMember(parts, trailing)
		if member == nil {
			p.errorAt(spanOf(parts), "enum body may only contain members followed by instance methods")
		} else if methodsStarted {
			p.errorAt(member.Span(), "enum members must be declared before enum methods")
		} else {
			node.Body = append(node.Body, member)
		}
		p.pos = nx
	}
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
	return node
}

func (p *Parser) parseTypeAlias() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	node := &ast.TypeAliasStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	equal := topLevelIndex(line, "=")
	if equal < 0 {
		p.errorAt(spanOf(line), "type alias must be: alias Name<T> = Target")
		p.pos = next
		return node
	}
	node.Name, node.TypeParameters = p.parseGenericDeclaration(line[:equal], "type alias")
	if equal+1 >= len(line) {
		p.errorAt(line[equal].Span, "type alias target is required after =")
	} else {
		node.Target = p.parseTypeRef(line[equal+1:])
		if node.Target.Empty() {
			p.errorAt(spanOf(line[equal+1:]), "type alias target must be a type")
		}
	}
	p.pos = next
	return node
}

func (p *Parser) parseNewtype() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	node := &ast.NewtypeStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	equal := topLevelIndex(line, "=")
	if equal < 0 {
		p.errorAt(spanOf(line), "newtype must be: newtype Name = Target")
		p.pos = next
		return node
	}
	name, parameters := p.parseGenericDeclaration(line[:equal], "newtype")
	node.Name = name
	if len(parameters) > 0 {
		p.errorAt(spanOf(line[2:equal]), "generic newtype declarations are not supported yet")
	}
	if equal+1 >= len(line) {
		p.errorAt(line[equal].Span, "newtype target is required after =")
	} else {
		node.Target = p.parseTypeRef(line[equal+1:])
		if node.Target.Empty() {
			p.errorAt(spanOf(line[equal+1:]), "newtype target must be a type")
		}
	}
	p.pos = next
	return node
}

func (p *Parser) parseGenericDeclaration(line []token.Token, kind string) (string, []ast.TypeParameter) {
	if len(line) < 2 || line[1].Kind != token.Identifier {
		p.errorAt(spanOf(line), kind+" declaration requires a name")
		return "", nil
	}
	name := line[1].Lexeme
	if len(line) == 2 {
		return name, nil
	}
	if line[2].Lexeme != "<" {
		p.errorAt(spanOf(line[2:]), kind+" declaration has unexpected tokens after its name")
		return name, nil
	}
	close := matchingIndex(line, 2, "<", ">")
	if close != len(line)-1 {
		p.errorAt(spanOf(line[2:]), kind+" type parameter list must end the declaration")
		return name, nil
	}
	parameters := []ast.TypeParameter{}
	for _, part := range splitTopLevel(line[3:close], ",") {
		if len(part) != 1 || part[0].Kind != token.Identifier {
			p.errorAt(spanOf(part), "type parameter must be one identifier")
			continue
		}
		parameters = append(parameters, ast.TypeParameter{Base: ast.Base{SourceSpan: part[0].Span}, Name: part[0].Lexeme})
	}
	if len(parameters) == 0 {
		p.errorAt(spanOf(line[2:]), kind+" type parameter list must not be empty")
	}
	return name, parameters
}

func (p *Parser) parseEnumMember(parts []token.Token, trailing string) *ast.EnumMemberStatement {
	if len(parts) == 0 || parts[0].Kind != token.Identifier {
		return nil
	}
	attributeAt := topLevelAttributeIndex(parts, 1)
	core := parts
	if attributeAt >= 0 {
		core = parts[:attributeAt]
	}
	member := &ast.EnumMemberStatement{
		Base: ast.Base{SourceSpan: spanOf(parts), TrailingComment: trailing},
		Name: parts[0].Lexeme,
	}
	if len(core) == 1 {
		// Payloadless member.
	} else if core[1].Lexeme == "=" {
		if len(core) == 2 {
			return nil
		}
		value, ok := p.parseExpression(core[2:])
		if !ok {
			return nil
		}
		member.RawValue = value
	} else if core[1].Lexeme != "(" {
		return nil
	} else {
		close := matchingIndex(core, 1, "(", ")")
		if close != len(core)-1 {
			return nil
		}
		member.Parameters = p.parseParameters(core[2:close])
	}
	if attributeAt >= 0 {
		attributes, ok := p.parseAttributes(parts[attributeAt:])
		if !ok {
			return nil
		}
		member.Attributes = attributes
	}
	return member
}

func (p *Parser) parseRecordField(line []token.Token, comment string) *ast.RecordFieldStatement {
	if len(line) < 3 || line[0].Kind != token.Identifier || strings.HasPrefix(line[0].Lexeme, "@") || line[1].Lexeme != ":" {
		return nil
	}
	attributeAt := topLevelAttributeIndex(line, 2)
	if attributeAt < 0 {
		attributeAt = len(line)
	}
	equal := topLevelIndex(line[2:attributeAt], "=")
	if equal >= 0 {
		equal += 2
	}
	typeEnd := attributeAt
	if equal >= 0 {
		typeEnd = equal
	}
	if typeEnd == 2 {
		return nil
	}
	field := &ast.RecordFieldStatement{
		Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment},
		Name: line[0].Lexeme,
		Type: p.parseTypeRef(line[2:typeEnd]),
	}
	if equal >= 0 {
		if equal+1 >= attributeAt {
			return nil
		}
		field.Default, _ = p.parseExpression(line[equal+1 : attributeAt])
		if field.Default == nil {
			return nil
		}
	}
	if attributeAt < len(line) {
		attributes, ok := p.parseAttributes(line[attributeAt:])
		if !ok {
			return nil
		}
		field.Attributes = attributes
	}
	return field
}

func topLevelAttributeIndex(tokens []token.Token, start int) int {
	depth := 0
	for index := start; index < len(tokens); index++ {
		switch tokens[index].Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(tokens[index].Lexeme, "@") {
			return index
		}
	}
	return -1
}

func (p *Parser) parseAttributes(tokens []token.Token) ([]ast.Attribute, bool) {
	attributes := []ast.Attribute{}
	for index := 0; index < len(tokens); {
		name := tokens[index]
		if !strings.HasPrefix(name.Lexeme, "@") {
			return nil, false
		}
		attribute := ast.Attribute{Base: ast.Base{SourceSpan: name.Span}, Name: strings.TrimPrefix(name.Lexeme, "@")}
		index++
		if index < len(tokens) && tokens[index].Lexeme == "(" {
			close := matchingIndex(tokens, index, "(", ")")
			if close < 0 {
				return nil, false
			}
			for _, part := range splitTopLevel(tokens[index+1:close], ",") {
				if len(part) == 0 {
					continue
				}
				argument := ast.CallArgument{}
				if len(part) > 2 && part[0].Kind == token.Identifier && part[1].Lexeme == ":" {
					argument.Name = part[0].Lexeme
					part = part[2:]
				}
				argument.Value, _ = p.parseExpression(part)
				if argument.Value == nil {
					return nil, false
				}
				attribute.Arguments = append(attribute.Arguments, argument)
			}
			attribute.SourceSpan.End = tokens[close].Span.End
			index = close + 1
		}
		attributes = append(attributes, attribute)
	}
	return attributes, true
}

func (p *Parser) parseNativeLine() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	p.pos = next
	return p.nativeStatement(base)
}

func (p *Parser) parseNativeBlock() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	header := strings.TrimSpace(p.sliceSpan(base.SourceSpan))
	p.pos = next
	body := p.parseStatements(map[string]bool{"end": true})
	closer, closeSpan := p.consumeTerminator("end")
	base.SourceSpan.End = closeSpan.End
	p.nativeIslands = append(p.nativeIslands, ast.NativeIsland{Span: base.SourceSpan, WholeStatement: true})
	return &ast.NativeBlock{Base: base, Header: headerWithoutComment(header), Body: body, Closer: closer}
}

func (p *Parser) parseClass() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	c := &ast.ClassStatement{Base: base}
	if len(line) < 2 {
		p.errorAt(line[0].Span, "class name is required")
	} else {
		nameEnd := len(line)
		genericEnd := -1
		if len(line) > 2 && line[2].Lexeme == "<" {
			if close := matchingIndex(line, 2, "<", ">"); close >= 0 {
				genericEnd = close
				c.Name, c.TypeParameters = p.parseGenericDeclaration(line[:genericEnd+1], "class")
				nameEnd = genericEnd + 1
			}
		}
		scanAt := 2
		if genericEnd >= 0 {
			scanAt = genericEnd + 1
		}
		for i := scanAt; i < len(line); i++ {
			if line[i].Lexeme == "<" || line[i].Lexeme == "implements" {
				nameEnd = i
				break
			}
		}
		if genericEnd < 0 {
			c.Name = joinLexemes(line[1:nameEnd])
		}
	}
	extendsAt, implementsAt := -1, -1
	searchAt := 2
	if len(c.TypeParameters) > 0 {
		searchAt = matchingIndex(line, 2, "<", ">") + 1
	}
	for i := searchAt; i < len(line); i++ {
		if line[i].Lexeme == "<" && extendsAt < 0 {
			extendsAt = i
		}
		if line[i].Lexeme == "implements" && implementsAt < 0 {
			implementsAt = i
		}
	}
	if extendsAt >= 0 {
		superEnd := len(line)
		if implementsAt > extendsAt {
			superEnd = implementsAt
		}
		if expr, ok := p.parseExpression(line[extendsAt+1 : superEnd]); ok {
			c.Superclass = expr
		}
	}
	if implementsAt >= 0 {
		for _, part := range splitTopLevel(line[implementsAt+1:], ",") {
			if len(part) > 0 {
				implemented := p.parseTypeRef(part)
				if implemented.Empty() {
					p.errorAt(spanOf(part), "implemented interface must be a type")
					continue
				}
				c.Implements = append(c.Implements, implemented)
			}
		}
	}
	p.pos = next
	c.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	c.SourceSpan.End = closeSpan.End
	return c
}

func (p *Parser) parseModule() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	m := &ast.ModuleStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	if len(line) > 1 {
		m.Name = joinLexemes(line[1:])
	} else {
		p.errorAt(line[0].Span, "module name is required")
	}
	p.pos = next
	m.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	m.SourceSpan.End = closeSpan.End
	return m
}

func (p *Parser) parseInterface() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	i := &ast.InterfaceStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	i.Name, i.TypeParameters = p.parseGenericDeclaration(line, "interface")
	p.pos = next
	for !p.atEOF() {
		p.skipSeparators()
		if p.current().Lexeme == "end" {
			break
		}
		if p.current().Kind == token.Comment {
			p.pos++
			continue
		}
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		method := p.parseMethodSignature(parts, trailing)
		if method == nil {
			p.errorAt(spanOf(parts), "interface body may only contain method signatures")
		} else {
			i.Methods = append(i.Methods, method)
		}
		p.pos = nx
	}
	_, closeSpan := p.consumeTerminator("end")
	i.SourceSpan.End = closeSpan.End
	return i
}

func (p *Parser) parseMethodSignature(line []token.Token, comment string) *ast.MethodStatement {
	if len(line) < 3 {
		return nil
	}
	m := &ast.MethodStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}, Name: line[0].Lexeme}
	if line[1].Lexeme != "(" {
		return nil
	}
	close := matchingIndex(line, 1, "(", ")")
	if close < 0 {
		return nil
	}
	m.Parameters = p.parseParameters(line[2:close])
	if !p.parseMethodResultAndEffects(m, line[close+1:]) {
		return nil
	}
	return m
}

func (p *Parser) parseMethod() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	m := &ast.MethodStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	if len(line) < 2 {
		p.errorAt(line[0].Span, "method name is required")
		p.pos = next
		return m
	}
	i := 1
	if len(line) > 3 && line[1].Lexeme == "self" && line[2].Lexeme == "." {
		m.Class = true
		i = 3
	}
	if i+1 < len(line) && line[i].Lexeme == "[" && line[i+1].Lexeme == "]" {
		m.Name = "[]"
		i += 2
	} else {
		m.Name = line[i].Lexeme
		i++
	}
	if i < len(line) && line[i].Lexeme == "=" {
		m.Name += "="
		i++
	}
	if i < len(line) && line[i].Lexeme == "<" {
		close := matchingIndex(line, i, "<", ">")
		if close < 0 {
			p.errorAt(line[i].Span, "unclosed type parameter list")
		} else {
			for _, part := range splitTopLevel(line[i+1:close], ",") {
				if len(part) != 1 || part[0].Kind != token.Identifier {
					p.errorAt(spanOf(part), "type parameter must be one identifier")
					continue
				}
				m.TypeParameters = append(m.TypeParameters, ast.TypeParameter{Base: ast.Base{SourceSpan: part[0].Span}, Name: part[0].Lexeme})
			}
			i = close + 1
		}
	}
	if i < len(line) && line[i].Lexeme == "(" {
		close := matchingIndex(line, i, "(", ")")
		if close < 0 {
			p.errorAt(line[i].Span, "unclosed parameter list")
			m.Parameters = p.parseParameters(line[i+1:])
			i = len(line)
		} else {
			m.Parameters = p.parseParameters(line[i+1 : close])
			i = close + 1
		}
	} else if i < len(line) && line[i].Lexeme != ":" && line[i].Lexeme != "fails" {
		// Ruby-compatible unparenthesized definitions are represented, but the
		// formatter will normalize them to parentheses.
		m.Parameters = p.parseParameters(line[i:])
		i = len(line)
	}
	if !p.parseMethodResultAndEffects(m, line[i:]) {
		p.errorAt(spanOf(line[i:]), "method signature may only contain an optional return type written as : Type")
	}
	p.pos = next
	m.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	m.SourceSpan.End = closeSpan.End
	return m
}

func (p *Parser) parseMethodResultAndEffects(method *ast.MethodStatement, tokens []token.Token) bool {
	if len(tokens) == 0 {
		return true
	}
	failsAt := topLevelIndex(tokens, "fails")
	resultTokens := tokens
	if failsAt >= 0 {
		p.migrationErrorAt(tokens[failsAt].Span, failsRemovedMessage)
		resultTokens = tokens[:failsAt]
		if failsAt+1 >= len(tokens) {
			return true
		}
		method.Fails = p.parseTypeRef(tokens[failsAt+1:])
	}
	if len(resultTokens) == 0 {
		return failsAt == 0
	}
	if resultTokens[0].Lexeme != ":" || len(resultTokens) == 1 {
		return false
	}
	method.ReturnType = p.parseReturnType(resultTokens[1:])
	return !method.ReturnType.Empty()
}

func (p *Parser) parseReturnType(tokens []token.Token) ast.TypeRef {
	result := p.parseTypeRef(tokens)
	if strings.EqualFold(result.Name, "Void") {
		p.errorAt(result.Span(), "Void return type must be omitted")
	}
	return result
}

func (p *Parser) parseParameters(tokens []token.Token) []ast.Parameter {
	var params []ast.Parameter
	namedOnly := false
	namedOnlyParameter := false
	var separatorSpan token.Span
	for _, part := range splitTopLevel(tokens, ",") {
		if len(part) == 0 {
			continue
		}
		if len(part) == 1 && part[0].Lexeme == "*" {
			if namedOnly {
				p.errorAt(part[0].Span, "named-only parameter separator * may appear only once")
			} else {
				namedOnly = true
				separatorSpan = part[0].Span
			}
			continue
		}
		if len(part) == 1 && part[0].Lexeme == "**" {
			p.errorAt(part[0].Span, "bare ** is not supported; only bare * may separate named-only parameters")
			continue
		}
		param := ast.Parameter{Base: ast.Base{SourceSpan: spanOf(part)}, NamedOnly: namedOnly}
		i := 0
		if part[i].Lexeme == "*" || part[i].Lexeme == "**" {
			param.Rest = part[i].Lexeme == "*"
			param.KeywordRest = part[i].Lexeme == "**"
			i++
		}
		if i >= len(part) {
			continue
		}
		param.Name = part[i].Lexeme
		i++
		if i < len(part) && part[i].Lexeme == "::" {
			p.migrationErrorAt(part[i].Span, "typed keyword parameter syntax name:: Type was removed; add bare * and write name: Type")
			param.NamedOnly = true
			i++
			equal := topLevelIndex(part[i:], "=")
			typeEnd := len(part)
			if equal >= 0 {
				equal += i
				typeEnd = equal
			}
			param.Type = p.parseTypeRef(part[i:typeEnd])
			if equal >= 0 {
				param.Default, _ = p.parseExpression(part[equal+1:])
			}
			params = append(params, param)
			namedOnlyParameter = namedOnlyParameter || namedOnly
			continue
		}
		colon := topLevelIndex(part[i:], ":")
		equal := topLevelIndex(part[i:], "=")
		if colon >= 0 {
			colon += i
			if colon+1 >= len(part) {
				param.NativeKeyword = true
				params = append(params, param)
				namedOnlyParameter = namedOnlyParameter || namedOnly
				continue
			}
			typeEnd := len(part)
			if equal >= 0 {
				equal += i
				typeEnd = equal
			}
			param.Type = p.parseTypeRef(part[colon+1 : typeEnd])
			if equal >= 0 {
				param.Default, _ = p.parseExpression(part[equal+1:])
			} else if rubyNativeKeywordCandidate(part[colon+1]) {
				// Keep the deterministic TypeRB type parse above authoritative.
				// Explicit Ruby-native source may reinterpret this separately stored
				// candidate after imports and compilation mode are known.
				param.NativeKeyword = true
				param.NativeKeywordDefault, _ = p.parseExpression(part[colon+1:])
			}
		} else if equal >= 0 {
			equal += i
			param.Default, _ = p.parseExpression(part[equal+1:])
		}
		params = append(params, param)
		namedOnlyParameter = namedOnlyParameter || namedOnly
	}
	if namedOnly && !namedOnlyParameter {
		p.errorAt(separatorSpan, "named-only parameter separator * must be followed by a parameter")
	}
	return params
}

// rubyNativeKeywordCandidate is metadata for the opt-in Ruby-native checker;
// it never decides how portable TypeRB parameter syntax is parsed.
func rubyNativeKeywordCandidate(tok token.Token) bool {
	if tok.Lexeme == "" {
		return true
	}
	if tok.Lexeme == "(" || tok.Lexeme[0] >= 'A' && tok.Lexeme[0] <= 'Z' {
		return false
	}
	switch strings.ToLower(tok.Lexeme) {
	case "string", "int", "integer", "float", "float64", "bool", "boolean", "any", "void", "array", "hash", "map":
		return false
	}
	return true
}

func (p *Parser) parseIf() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	n := &ast.IfStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	conditionTokens := line[1:]
	if len(conditionTokens) >= 2 && conditionTokens[0].Lexeme == "(" && conditionTokens[len(conditionTokens)-1].Lexeme == ")" {
		conditionTokens = conditionTokens[1 : len(conditionTokens)-1]
	}
	n.Condition, _ = p.parseExpression(conditionTokens)
	if n.Condition == nil {
		n.Condition = nativeExpression(conditionTokens, p)
	}
	p.pos = next
	n.Then = p.parseStatements(map[string]bool{"elsif": true, "else": true, "end": true})
	for !p.atEOF() && p.current().Lexeme == "elsif" {
		s, e, nx, _ := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		cond, ok := p.parseExpression(parts[1:])
		if !ok {
			cond = nativeExpression(parts[1:], p)
		}
		p.pos = nx
		body := p.parseStatements(map[string]bool{"elsif": true, "else": true, "end": true})
		n.ElseIf = append(n.ElseIf, ast.IfBranch{Condition: cond, Body: body})
	}
	if !p.atEOF() && p.current().Lexeme == "else" {
		n.HasElse = true
		_, _, nx, _ := p.logicalLine(p.pos)
		p.pos = nx
		n.Else = p.parseStatements(map[string]bool{"end": true})
	}
	_, closeSpan := p.consumeTerminator("end")
	n.SourceSpan.End = closeSpan.End
	return n
}

func (p *Parser) parseCase() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	node := &ast.CaseStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	node.Value, _ = p.parseExpression(line[1:])
	if node.Value == nil {
		p.errorAt(spanOf(line), "case requires a value")
	}
	p.pos = next
	node.Leading = p.parseStatements(map[string]bool{"when": true, "else": true, "end": true})
	for !p.atEOF() && p.current().Lexeme == "when" {
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		branch := ast.CaseBranch{Base: ast.Base{SourceSpan: spanOf(parts), TrailingComment: trailing}}
		for _, alternativeTokens := range splitTopLevel(parts[1:], ",") {
			alternative, ok := p.parseExpression(alternativeTokens)
			if !ok || alternative == nil {
				p.errorAt(spanOf(alternativeTokens), "when requires a valid value")
				continue
			}
			if branch.Value == nil {
				branch.Value = alternative
			} else {
				branch.Alternatives = append(branch.Alternatives, alternative)
			}
		}
		if branch.Value == nil {
			p.errorAt(spanOf(parts), "when requires a value")
		} else if len(branch.Alternatives) == 0 {
			if call, ok := branch.Value.(*ast.CallExpression); ok {
				member, enumPattern := call.Callee.(*ast.MemberExpression)
				_, typePattern := call.Callee.(*ast.Identifier)
				if call.Block != nil || !typePattern && (!enumPattern || !member.Namespace) {
					p.errorAt(call.Span(), "case pattern must be Variant(name, ...) or Type(name)")
				} else {
					valid := true
					if typePattern && len(call.Arguments) != 1 {
						p.errorAt(call.Span(), "union type pattern expects exactly one binding")
						valid = false
					}
					seenNamed := false
					for _, argument := range call.Arguments {
						identifier, identifierOK := argument.Value.(*ast.Identifier)
						if argument.Name != "" {
							seenNamed = true
						} else if seenNamed {
							p.errorAt(argument.Value.Span(), "positional pattern binding cannot follow a named binding")
							valid = false
						}
						if !identifierOK || argument.Splat != "" || typePattern && argument.Name != "" {
							p.errorAt(argument.Value.Span(), "case pattern bindings must be identifiers")
							valid = false
							continue
						}
						branch.Bindings = append(branch.Bindings, ast.PatternBinding{
							Base:  ast.Base{SourceSpan: identifier.Span()},
							Name:  identifier.Name,
							Label: argument.Name,
						})
					}
					if valid {
						branch.Value = call.Callee
					}
				}
			}
		}
		p.pos = nx
		branch.Body = p.parseStatements(map[string]bool{"when": true, "else": true, "end": true})
		node.Branches = append(node.Branches, branch)
	}
	if !p.atEOF() && p.current().Lexeme == "else" {
		s, e, nx, _ := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		if len(parts) != 1 {
			p.errorAt(spanOf(parts), "else does not take a value")
		}
		node.HasElse = true
		p.pos = nx
		node.Else = p.parseStatements(map[string]bool{"end": true})
	}
	if len(node.Branches) == 0 {
		p.errorAt(node.Span(), "case requires at least one when branch")
	}
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
	return node
}

func (p *Parser) parseWhile() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	n := &ast.WhileStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	conditionTokens := line[1:]
	if len(conditionTokens) >= 2 && conditionTokens[0].Lexeme == "(" && conditionTokens[len(conditionTokens)-1].Lexeme == ")" {
		conditionTokens = conditionTokens[1 : len(conditionTokens)-1]
	}
	n.Condition, _ = p.parseExpression(conditionTokens)
	if n.Condition == nil {
		n.Condition = nativeExpression(conditionTokens, p)
	}
	p.pos = next
	n.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	n.SourceSpan.End = closeSpan.End
	return n
}

func (p *Parser) parseReturn() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	r := &ast.ReturnStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	if len(line) > 1 {
		r.Value, _ = p.parseExpression(line[1:])
		if r.Value == nil {
			r.Value = nativeExpression(line[1:], p)
		}
	}
	p.pos = next
	return r
}

func (p *Parser) tryConditionalReturn(line []token.Token, next int, base ast.Base) ast.Statement {
	if len(line) < 2 || line[0].Lexeme != "return" {
		return nil
	}
	conditionalAt := topLevelIndex(line[1:], "if")
	if conditionalAt < 0 {
		return nil
	}
	conditionalAt++
	valueTokens := line[1:conditionalAt]
	conditionTokens := line[conditionalAt+1:]
	var value ast.Expression
	if len(valueTokens) > 0 {
		value, _ = p.parseExpression(valueTokens)
		if value == nil {
			p.errorAt(spanOf(valueTokens), "return before trailing if requires a valid expression")
			value = nativeExpression(valueTokens, p)
		}
	}
	condition, ok := p.parseExpression(conditionTokens)
	if !ok {
		p.errorAt(spanOf(line[conditionalAt:]), "conditional return requires a valid condition after if")
		condition = nativeExpression(conditionTokens, p)
	}
	transferBase := ast.Base{SourceSpan: line[0].Span}
	if len(valueTokens) > 0 {
		transferBase.SourceSpan.End = valueTokens[len(valueTokens)-1].Span.End
	}
	p.pos = next
	return conditionalTransfer(base, condition, &ast.ReturnStatement{Base: transferBase, Value: value})
}

func (p *Parser) tryUnsupportedTrailingCondition(line []token.Token, next int, base ast.Base) ast.Statement {
	for _, keyword := range []string{"if", "unless"} {
		at := topLevelIndex(line[1:], keyword)
		if at < 0 {
			continue
		}
		at++
		if line[0].Lexeme == "return" || line[0].Lexeme == "break" || line[0].Lexeme == "next" {
			if keyword == "unless" {
				p.errorAt(line[at].Span, "conditional transfers use trailing if; unless is not supported")
				p.pos = next
				return p.nativeStatement(base)
			}
			continue
		}
		if !p.completeStatementBeforeTrailingCondition(line[:at]) {
			continue
		}
		p.errorAt(line[at].Span, "trailing "+keyword+" is only allowed on return, break, or next")
		p.pos = next
		return p.nativeStatement(base)
	}
	return nil
}

func (p *Parser) completeStatementBeforeTrailingCondition(tokens []token.Token) bool {
	if _, complete := p.parseExpression(tokens); complete {
		return true
	}
	if at := topLevelIndex(tokens, ":="); at > 0 && at+1 < len(tokens) {
		_, complete := p.parseExpression(tokens[at+1:])
		return complete
	}
	for _, operator := range []string{"=", "+=", "-=", "*=", "/=", "||=", "&&="} {
		at := topLevelIndex(tokens, operator)
		if at <= 0 || at+1 >= len(tokens) {
			continue
		}
		_, left := p.parseExpression(tokens[:at])
		_, right := p.parseExpression(tokens[at+1:])
		if left && right {
			return true
		}
	}
	return false
}

func conditionalTransfer(base ast.Base, condition ast.Expression, transfer ast.Statement) ast.Statement {
	return &ast.IfStatement{
		Base:                base,
		Condition:           condition,
		Then:                []ast.Statement{transfer},
		ConditionalTransfer: true,
	}
}

func (p *Parser) parseLoopControl(keyword string) ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	var transfer ast.Statement
	if keyword == "break" {
		transfer = &ast.BreakStatement{Base: ast.Base{SourceSpan: line[0].Span}}
	} else {
		transfer = &ast.NextStatement{Base: ast.Base{SourceSpan: line[0].Span}}
	}
	conditionalAt := topLevelIndex(line[1:], "if")
	if conditionalAt >= 0 {
		conditionalAt++
		if conditionalAt != 1 {
			p.errorAt(spanOf(line[1:conditionalAt]), fmt.Sprintf("%s does not take a value", keyword))
		}
		condition, ok := p.parseExpression(line[conditionalAt+1:])
		if !ok {
			p.errorAt(spanOf(line[conditionalAt:]), fmt.Sprintf("conditional %s requires a valid condition after if", keyword))
			condition = nativeExpression(line[conditionalAt+1:], p)
		}
		p.pos = next
		return conditionalTransfer(base, condition, transfer)
	}
	if len(line) != 1 {
		p.errorAt(spanOf(line), fmt.Sprintf("%s does not take a value", keyword))
	}
	p.pos = next
	transferBase := ast.Base{SourceSpan: line[0].Span, TrailingComment: comment}
	if keyword == "break" {
		return &ast.BreakStatement{Base: transferBase}
	}
	return &ast.NextStatement{Base: transferBase}
}

func (p *Parser) parseImport() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	n := &ast.ImportStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	if len(line) > 1 && line[1].Lexeme == "{" {
		close := matchingIndex(line, 1, "{", "}")
		if close > 1 {
			for _, part := range splitTopLevel(line[2:close], ",") {
				if len(part) > 0 {
					n.Symbols = append(n.Symbols, part[0].Lexeme)
				}
			}
			if close+2 < len(line) && line[close+1].Lexeme == "from" {
				pathTokens := line[close+2:]
				n.Path = importPath(pathTokens)
				n.PathSpan = spanOf(pathTokens)
			}
		}
	} else if len(line) > 1 {
		aliasAt := -1
		for i := 2; i < len(line); i++ {
			if line[i].Lexeme == "as" {
				aliasAt = i
				break
			}
		}
		pathEnd := len(line)
		if aliasAt >= 0 {
			pathEnd = aliasAt
			if aliasAt+1 < len(line) {
				n.Alias = line[aliasAt+1].Lexeme
			}
		}
		pathTokens := line[1:pathEnd]
		n.Path = importPath(pathTokens)
		n.PathSpan = spanOf(pathTokens)
	}
	if n.Path == "" {
		p.errorAt(n.SourceSpan, "invalid import declaration")
	}
	p.pos = next
	return n
}

func importPath(tokens []token.Token) string {
	if len(tokens) == 1 && tokens[0].Kind == token.String {
		return unquote(tokens[0].Lexeme)
	}
	return joinLexemes(tokens)
}

func (p *Parser) tryField(line []token.Token, base ast.Base) ast.Statement {
	i := 0
	readOnly := false
	if line[i].Lexeme == "readonly" {
		readOnly = true
		i++
	}
	if i >= len(line) || !strings.HasPrefix(line[i].Lexeme, "@") {
		return nil
	}
	field := &ast.FieldStatement{Base: base, Name: line[i].Lexeme, ReadOnly: readOnly}
	i++
	if i < len(line) && line[i].Lexeme == ":" {
		assign := topLevelIndex(line[i+1:], ":=")
		typeEnd := len(line)
		if assign >= 0 {
			assign += i + 1
			typeEnd = assign
		}
		field.Type = p.parseTypeRef(line[i+1 : typeEnd])
		if assign >= 0 {
			field.Value, _ = p.parseExpression(line[assign+1:])
		}
		return field
	}
	return nil
}

func (p *Parser) tryVariable(line []token.Token, base ast.Base) ast.Statement {
	return p.tryVariableWithEmbedded(line, base, nil)
}

func (p *Parser) tryVariableWithEmbedded(line []token.Token, base ast.Base, embedded map[int]ast.Expression) ast.Statement {
	assign := topLevelIndex(line, ":=")
	if assign < 0 || assign == 0 {
		return nil
	}
	left := line[:assign]
	n := &ast.VariableStatement{Base: base}
	if left[0].Lexeme == "mut" {
		n.Mutable = true
		left = left[1:]
	}
	if len(left) == 0 || strings.HasPrefix(left[0].Lexeme, "@") {
		return nil
	}
	n.Name = left[0].Lexeme
	n.Constant = isConstantName(n.Name)
	if len(left) > 1 && left[1].Lexeme == ":" {
		n.Type = p.parseTypeRef(left[2:])
	}
	n.Value, _ = p.parseExpressionWithEmbedded(line[assign+1:], embedded)
	if n.Value == nil {
		n.Value = nativeExpression(line[assign+1:], p)
	}
	return n
}

func (p *Parser) tryAssignment(line []token.Token, base ast.Base) ast.Statement {
	return p.tryAssignmentWithEmbedded(line, base, nil)
}

func (p *Parser) tryAssignmentWithEmbedded(line []token.Token, base ast.Base, embedded map[int]ast.Expression) ast.Statement {
	for _, op := range []string{"=", "+=", "-=", "*=", "/=", "||=", "&&="} {
		at := topLevelIndex(line, op)
		if at <= 0 {
			continue
		}
		left, lok := p.parseExpression(line[:at])
		right, rok := p.parseExpressionWithEmbedded(line[at+1:], embedded)
		if lok && rok {
			return &ast.AssignmentStatement{Base: base, Target: left, Operator: op, Value: right}
		}
	}
	return nil
}

func (p *Parser) opensNativeBlock(line []token.Token) bool {
	if len(line) == 0 {
		return false
	}
	switch line[0].Lexeme {
	case "begin", "case", "unless", "while", "until", "for":
		return true
	}
	depth := 0
	for _, tok := range line {
		switch tok.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			depth--
		case "do":
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

func (p *Parser) consumeTerminator(name string) (string, token.Span) {
	if p.atEOF() || p.current().Lexeme != name {
		span := p.current().Span
		p.errorAt(span, fmt.Sprintf("expected %q", name))
		return name, span
	}
	start, end, next, _ := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	span := spanOf(line)
	text := strings.TrimSpace(p.sliceSpan(span))
	p.pos = next
	return headerWithoutComment(text), span
}

func (p *Parser) logicalLine(from int) (start, end, next int, comment string) {
	start = from
	end = from
	depth := 0
	for end < len(p.tokens) {
		t := p.tokens[end]
		if t.Kind == token.EOF {
			break
		}
		if t.Kind == token.Comment && depth == 0 {
			comment = t.Lexeme
			for end < len(p.tokens) && p.tokens[end].Kind != token.Newline && p.tokens[end].Kind != token.EOF {
				end++
			}
			break
		}
		switch t.Lexeme {
		case "(", "[", "{":
			depth++
		case ")", "]", "}":
			if depth > 0 {
				depth--
			}
		}
		if t.Kind == token.Newline && depth == 0 {
			break
		}
		if t.Lexeme == ";" && depth == 0 {
			break
		}
		end++
	}
	next = end
	if next < len(p.tokens) && (p.tokens[next].Kind == token.Newline || p.tokens[next].Lexeme == ";") {
		next++
	}
	return
}

func (p *Parser) codeTokens(start, end int) []token.Token {
	var out []token.Token
	for _, t := range p.tokens[start:end] {
		if t.Kind != token.Comment && t.Kind != token.Newline {
			out = append(out, t)
		}
	}
	return out
}

func (p *Parser) skipSeparators() {
	for p.current().Kind == token.Newline || p.current().Lexeme == ";" {
		p.pos++
	}
}

func (p *Parser) atStatementStart() bool {
	return p.pos == 0 || p.tokens[p.pos-1].Kind == token.Newline || p.tokens[p.pos-1].Lexeme == ";"
}

func (p *Parser) current() token.Token {
	if p.pos >= len(p.tokens) {
		return p.tokens[len(p.tokens)-1]
	}
	return p.tokens[p.pos]
}

func (p *Parser) atEOF() bool { return p.current().Kind == token.EOF }

func (p *Parser) errorAt(span token.Span, message string) {
	p.diags = append(p.diags, diagnostic.Diagnostic{Severity: diagnostic.Error, Message: message, Span: span})
}

func (p *Parser) sliceSpan(span token.Span) string {
	start, end := span.Start.Offset, span.End.Offset
	if start < 0 || end < start || end > len(p.source) {
		return ""
	}
	return string(p.source[start:end])
}

func spanOf(tokens []token.Token) token.Span {
	if len(tokens) == 0 {
		return token.Span{}
	}
	return token.Span{Start: tokens[0].Span.Start, End: tokens[len(tokens)-1].Span.End}
}

func nativeExpression(tokens []token.Token, p *Parser) ast.Expression {
	span := spanOf(tokens)
	p.nativeIslands = append(p.nativeIslands, ast.NativeIsland{Span: span})
	return &ast.NativeExpression{Base: ast.Base{SourceSpan: span}, Text: strings.TrimSpace(p.sliceSpan(span))}
}

func (p *Parser) nativeStatement(base ast.Base) ast.Statement {
	p.nativeIslands = append(p.nativeIslands, ast.NativeIsland{Span: base.SourceSpan, WholeStatement: true})
	return &ast.NativeStatement{Base: base, Text: strings.TrimSpace(p.sliceSpan(base.SourceSpan))}
}

func headerWithoutComment(s string) string {
	// The lexer already separated trailing comments; this only protects native
	// headers constructed directly from a source span.
	return strings.TrimSpace(s)
}

func isConstantName(name string) bool {
	if name == "" {
		return false
	}
	r := rune(name[0])
	return r >= 'A' && r <= 'Z'
}

func isEndlessDefinition(line []token.Token) bool {
	if len(line) < 3 {
		return false
	}
	at := topLevelIndex(line[2:], "=")
	if at < 0 {
		return false
	}
	at += 2
	// name=(value) and []=(key, value) are setter definitions.
	return at+1 >= len(line) || line[at+1].Lexeme != "("
}

func unquote(s string) string {
	if value, err := strconv.Unquote(s); err == nil {
		return value
	}
	return strings.Trim(s, "'\"")
}
