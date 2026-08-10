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
	source []byte
	tokens []token.Token
	pos    int
	diags  []diagnostic.Diagnostic
}

func Parse(source []byte) (*ast.Program, []diagnostic.Diagnostic) {
	tokens, lexDiags := lexer.Lex(source)
	p := &Parser{source: source, tokens: tokens, diags: append([]diagnostic.Diagnostic(nil), lexDiags...)}
	program := &ast.Program{Tokens: tokens}
	if len(tokens) > 0 {
		program.SourceSpan.Start = tokens[0].Span.Start
	}
	program.Statements = p.parseStatements(nil)
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
	return program, p.diags
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
	if expression := p.tryControlFlowExpressionStatement(line, next, base); expression != nil {
		return expression
	}
	if blockOperation := p.tryIterationStatement(line, next, base); blockOperation != nil {
		return blockOperation
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
	if callBlock := p.tryCallBlockStatement(line, next, base); callBlock != nil {
		return callBlock
	}
	if p.opensNativeBlock(line) {
		return p.parseNativeBlock()
	}
	if expression, ok := parseExpressionTokens(line); ok {
		p.pos = next
		return &ast.ExpressionStatement{Base: base, Expression: expression}
	}

	p.pos = next
	return &ast.NativeStatement{Base: base, Text: strings.TrimSpace(p.sliceSpan(base.SourceSpan))}
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
	expression, ok := parseExpressionTokens(line[:blockAt])
	if !ok {
		return nil
	}
	call, ok := expression.(*ast.CallExpression)
	if !ok {
		return nil
	}
	if !brace {
		parameters, valid := p.blockParameters(line[blockAt+1:])
		if !valid {
			p.errorAt(spanOf(line[blockAt:]), "call block parameters must be written as |name, ...|")
		}
		p.pos = next
		block := &ast.BlockExpression{Base: ast.Base{SourceSpan: spanOf(line[blockAt:])}, Parameters: parameters}
		block.Body = p.parseStatements(map[string]bool{"end": true})
		_, closeSpan := p.consumeTerminator("end")
		block.SourceSpan.End = closeSpan.End
		call.SourceSpan.End = closeSpan.End
		call.Block = block
		base.SourceSpan.End = closeSpan.End
		return &ast.ExpressionStatement{Base: base, Expression: call}
	}

	close := matchingIndex(line, blockAt, "{", "}")
	if close < 0 {
		p.errorAt(line[blockAt].Span, "unterminated call block; expected }")
		p.pos = next
		return &ast.ExpressionStatement{Base: base, Expression: call}
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
	base.SourceSpan.End = line[close].Span.End
	p.pos = next
	return &ast.ExpressionStatement{Base: base, Expression: call}
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
		value, valid := parseExpressionTokensWithEmbedded(wrapped[1:], embedded)
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
		if value, valid := parseExpressionTokensWithEmbedded(wrapped, embedded); valid {
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
		value, ok := parseExpressionTokens(line[1:])
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
	if expression, ok := parseExpressionTokens(line); ok {
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
	expression, ok := parseExpressionTokens(tokens)
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
		if node.Name != "each" && node.Name != "map" && node.Name != "select" && node.Name != "reduce" {
			return nil, false
		}
		iteration.Source = node.Receiver
		iteration.Operation = node.Name
	case *ast.CallExpression:
		member, memberOK := node.Callee.(*ast.MemberExpression)
		if !memberOK || (member.Name != "each" && member.Name != "each_slice" && member.Name != "map" && member.Name != "select" && member.Name != "reduce") {
			return nil, false
		}
		iteration.Source = member.Receiver
		iteration.Operation = member.Name
		if member.Name == "each" || member.Name == "map" || member.Name == "select" {
			if len(node.Arguments) != 0 {
				p.errorAt(node.Span(), member.Name+" does not take arguments")
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

func (p *Parser) parseRecord() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	record := &ast.RecordStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	if len(line) != 2 {
		p.errorAt(spanOf(line), "record declaration must be: record Name")
	} else {
		record.Name = line[1].Lexeme
	}
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
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		member := p.parseEnumMember(parts, trailing)
		if member == nil {
			p.errorAt(spanOf(parts), "enum body may only contain members such as Ready or Value(value: String)")
		} else {
			node.Body = append(node.Body, member)
		}
		p.pos = nx
	}
	_, closeSpan := p.consumeTerminator("end")
	node.SourceSpan.End = closeSpan.End
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
	member := &ast.EnumMemberStatement{
		Base: ast.Base{SourceSpan: spanOf(parts), TrailingComment: trailing},
		Name: parts[0].Lexeme,
	}
	if len(parts) == 1 {
		return member
	}
	if parts[1].Lexeme != "(" {
		return nil
	}
	close := matchingIndex(parts, 1, "(", ")")
	if close != len(parts)-1 {
		return nil
	}
	member.Parameters = p.parseParameters(parts[2:close])
	return member
}

func (p *Parser) parseRecordField(line []token.Token, comment string) *ast.RecordFieldStatement {
	if len(line) < 3 || line[0].Kind != token.Identifier || strings.HasPrefix(line[0].Lexeme, "@") || line[1].Lexeme != ":" {
		return nil
	}
	attributeAt := len(line)
	depth := 0
	for index := 2; index < len(line); index++ {
		switch line[index].Lexeme {
		case "<", "[":
			depth++
		case ">", "]":
			if depth > 0 {
				depth--
			}
		}
		if depth == 0 && strings.HasPrefix(line[index].Lexeme, "@") {
			attributeAt = index
			break
		}
	}
	if attributeAt == 2 {
		return nil
	}
	field := &ast.RecordFieldStatement{
		Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment},
		Name: line[0].Lexeme,
		Type: parseType(line[2:attributeAt]),
	}
	for index := attributeAt; index < len(line); {
		name := line[index]
		if !strings.HasPrefix(name.Lexeme, "@") {
			return nil
		}
		attribute := ast.Attribute{Base: ast.Base{SourceSpan: name.Span}, Name: strings.TrimPrefix(name.Lexeme, "@")}
		index++
		if index < len(line) && line[index].Lexeme == "(" {
			close := matchingIndex(line, index, "(", ")")
			if close < 0 {
				return nil
			}
			for _, part := range splitTopLevel(line[index+1:close], ",") {
				if len(part) == 0 {
					continue
				}
				argument := ast.CallArgument{}
				if len(part) > 2 && part[0].Kind == token.Identifier && part[1].Lexeme == ":" {
					argument.Name = part[0].Lexeme
					part = part[2:]
				}
				argument.Value, _ = parseExpressionTokens(part)
				if argument.Value == nil {
					return nil
				}
				attribute.Arguments = append(attribute.Arguments, argument)
			}
			attribute.SourceSpan.End = line[close].Span.End
			index = close + 1
		}
		field.Attributes = append(field.Attributes, attribute)
	}
	return field
}

func (p *Parser) parseNativeLine() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	p.pos = next
	return &ast.NativeStatement{Base: base, Text: strings.TrimSpace(p.sliceSpan(base.SourceSpan))}
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
		for i := 2; i < len(line); i++ {
			if line[i].Lexeme == "<" || line[i].Lexeme == "implements" {
				nameEnd = i
				break
			}
		}
		c.Name = joinLexemes(line[1:nameEnd])
	}
	extendsAt, implementsAt := -1, -1
	for i := 2; i < len(line); i++ {
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
		if expr, ok := parseExpressionTokens(line[extendsAt+1 : superEnd]); ok {
			c.Superclass = expr
		}
	}
	if implementsAt >= 0 {
		for _, part := range splitTopLevel(line[implementsAt+1:], ",") {
			if len(part) > 0 {
				c.Implements = append(c.Implements, joinLexemes(part))
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
	if len(line) > 1 {
		i.Name = line[1].Lexeme
	} else {
		p.errorAt(line[0].Span, "interface name is required")
	}
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
	if close+1 < len(line) && line[close+1].Lexeme == ":" {
		m.ReturnType = p.parseReturnType(line[close+2:])
	} else if close+1 != len(line) {
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
			close = len(line) - 1
		}
		m.Parameters = p.parseParameters(line[i+1 : close])
		i = close + 1
	} else if i < len(line) && line[i].Lexeme != ":" {
		// Ruby-compatible unparenthesized definitions are represented, but the
		// formatter will normalize them to parentheses.
		m.Parameters = p.parseParameters(line[i:])
		i = len(line)
	}
	if i < len(line) && line[i].Lexeme == ":" {
		m.ReturnType = p.parseReturnType(line[i+1:])
	}
	p.pos = next
	m.Body = p.parseStatements(map[string]bool{"end": true})
	_, closeSpan := p.consumeTerminator("end")
	m.SourceSpan.End = closeSpan.End
	return m
}

func (p *Parser) parseReturnType(tokens []token.Token) ast.TypeRef {
	result := parseType(tokens)
	if strings.EqualFold(result.Name, "Void") {
		p.errorAt(result.Span(), "Void return type must be omitted")
	}
	return result
}

func (p *Parser) parseParameters(tokens []token.Token) []ast.Parameter {
	var params []ast.Parameter
	for _, part := range splitTopLevel(tokens, ",") {
		if len(part) == 0 {
			continue
		}
		param := ast.Parameter{Base: ast.Base{SourceSpan: spanOf(part)}}
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
			param.Keyword = true
			i++
			equal := topLevelIndex(part[i:], "=")
			typeEnd := len(part)
			if equal >= 0 {
				equal += i
				typeEnd = equal
			}
			param.Type = parseType(part[i:typeEnd])
			if equal >= 0 {
				param.Default, _ = parseExpressionTokens(part[equal+1:])
			}
			params = append(params, param)
			continue
		}
		colon := topLevelIndex(part[i:], ":")
		equal := topLevelIndex(part[i:], "=")
		if colon >= 0 {
			colon += i
			if colon+1 >= len(part) {
				param.Keyword = true
				params = append(params, param)
				continue
			}
			if !looksLikeType(part[colon+1]) {
				param.Keyword = true
				param.Default, _ = parseExpressionTokens(part[colon+1:])
				params = append(params, param)
				continue
			}
			typeEnd := len(part)
			if equal >= 0 {
				equal += i
				typeEnd = equal
			}
			param.Type = parseType(part[colon+1 : typeEnd])
			if equal >= 0 {
				param.Default, _ = parseExpressionTokens(part[equal+1:])
			}
		} else if equal >= 0 {
			equal += i
			param.Default, _ = parseExpressionTokens(part[equal+1:])
		}
		params = append(params, param)
	}
	return params
}

func looksLikeType(tok token.Token) bool {
	if tok.Lexeme == "" {
		return false
	}
	if tok.Lexeme[0] >= 'A' && tok.Lexeme[0] <= 'Z' {
		return true
	}
	switch strings.ToLower(tok.Lexeme) {
	case "string", "int", "integer", "float", "float64", "bool", "boolean", "any", "void", "array", "hash", "map":
		return true
	}
	return false
}

func (p *Parser) parseIf() ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	n := &ast.IfStatement{Base: ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}}
	conditionTokens := line[1:]
	if len(conditionTokens) >= 2 && conditionTokens[0].Lexeme == "(" && conditionTokens[len(conditionTokens)-1].Lexeme == ")" {
		conditionTokens = conditionTokens[1 : len(conditionTokens)-1]
	}
	n.Condition, _ = parseExpressionTokens(conditionTokens)
	if n.Condition == nil {
		n.Condition = nativeExpression(conditionTokens, p)
	}
	p.pos = next
	n.Then = p.parseStatements(map[string]bool{"elsif": true, "else": true, "end": true})
	for !p.atEOF() && p.current().Lexeme == "elsif" {
		s, e, nx, _ := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		cond, ok := parseExpressionTokens(parts[1:])
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
	node.Value, _ = parseExpressionTokens(line[1:])
	if node.Value == nil {
		p.errorAt(spanOf(line), "case requires a value")
	}
	p.pos = next
	node.Leading = p.parseStatements(map[string]bool{"when": true, "else": true, "end": true})
	for !p.atEOF() && p.current().Lexeme == "when" {
		s, e, nx, trailing := p.logicalLine(p.pos)
		parts := p.codeTokens(s, e)
		branch := ast.CaseBranch{Base: ast.Base{SourceSpan: spanOf(parts), TrailingComment: trailing}}
		branch.Value, _ = parseExpressionTokens(parts[1:])
		if branch.Value == nil {
			p.errorAt(spanOf(parts), "when requires exactly one enum member")
		} else if call, ok := branch.Value.(*ast.CallExpression); ok {
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
				for _, argument := range call.Arguments {
					identifier, identifierOK := argument.Value.(*ast.Identifier)
					if !identifierOK || argument.Name != "" || argument.Splat != "" {
						p.errorAt(argument.Value.Span(), "case pattern bindings must be identifiers")
						valid = false
						continue
					}
					branch.Bindings = append(branch.Bindings, ast.PatternBinding{
						Base: ast.Base{SourceSpan: identifier.Span()},
						Name: identifier.Name,
					})
				}
				if valid {
					branch.Value = call.Callee
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
	n.Condition, _ = parseExpressionTokens(conditionTokens)
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
		r.Value, _ = parseExpressionTokens(line[1:])
		if r.Value == nil {
			r.Value = nativeExpression(line[1:], p)
		}
	}
	p.pos = next
	return r
}

func (p *Parser) parseLoopControl(keyword string) ast.Statement {
	start, end, next, comment := p.logicalLine(p.pos)
	line := p.codeTokens(start, end)
	base := ast.Base{SourceSpan: spanOf(line), TrailingComment: comment}
	if len(line) != 1 {
		p.errorAt(spanOf(line), fmt.Sprintf("%s does not take a value", keyword))
	}
	p.pos = next
	if keyword == "break" {
		return &ast.BreakStatement{Base: base}
	}
	return &ast.NextStatement{Base: base}
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
				n.Path = importPath(line[close+2:])
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
		n.Path = importPath(line[1:pathEnd])
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
		field.Type = parseType(line[i+1 : typeEnd])
		if assign >= 0 {
			field.Value, _ = parseExpressionTokens(line[assign+1:])
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
		n.Type = parseType(left[2:])
	}
	n.Value, _ = parseExpressionTokensWithEmbedded(line[assign+1:], embedded)
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
		left, lok := parseExpressionTokens(line[:at])
		right, rok := parseExpressionTokensWithEmbedded(line[at+1:], embedded)
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
	return &ast.NativeExpression{Base: ast.Base{SourceSpan: span}, Text: strings.TrimSpace(p.sliceSpan(span))}
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
