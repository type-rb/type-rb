package ruby

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

type generator struct {
	b            strings.Builder
	indent       int
	loader       string
	modulePath   string
	topFunctions map[string]bool
}

func Generate(program *ir.Program) string {
	g := &generator{loader: program.RubyLoader, modulePath: program.ModulePath, topFunctions: map[string]bool{}}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok {
			g.topFunctions[method.Name] = true
		}
	}
	g.statements(program.Statements)
	if g.topFunctions["main"] {
		if len(program.Statements) > 0 {
			g.b.WriteByte('\n')
		}
		g.line("main()", "")
	}
	return strings.TrimRight(g.b.String(), "\n") + "\n"
}

func (g *generator) statements(statements []ir.Statement) {
	for i, statement := range statements {
		if i > 0 && wantsSeparation(statements[i-1], statement) {
			g.b.WriteByte('\n')
		}
		g.statement(statement)
	}
}

func wantsSeparation(previous, current ir.Statement) bool {
	if _, ok := previous.(*ir.Comment); ok {
		return false
	}
	switch current.(type) {
	case *ir.Class, *ir.Record, *ir.Module, *ir.Interface, *ir.Method:
		return true
	}
	return false
}

func (g *generator) statement(statement ir.Statement) {
	switch n := statement.(type) {
	case *ir.Comment:
		g.line(n.Text, "")
	case *ir.Import:
		if n.Standard || g.loader == "zeitwerk" {
			return
		}
		g.line("require_relative "+strconv.Quote(rubyImportPath(g.modulePath, n.Path)), n.TrailingComment)
	case *ir.Class:
		header := "class " + n.Name
		if n.Superclass != nil {
			header += " < " + g.expr(n.Superclass)
		}
		g.line(header, n.TrailingComment)
		g.indent++
		fields := classFields(n.Body)
		foundInitialize := false
		for _, member := range n.Body {
			if method, ok := member.(*ir.Method); ok && method.Name == "initialize" {
				foundInitialize = true
				g.method(method, fields)
				continue
			}
			if _, isField := member.(*ir.Field); !isField {
				g.statement(member)
			}
		}
		if !foundInitialize && hasDefaults(fields) {
			g.line("def initialize(...)", "")
			g.indent++
			g.line("super", "")
			g.fieldDefaults(fields)
			g.indent--
			g.line("end", "")
		}
		g.indent--
		g.line("end", "")
	case *ir.Record:
		fields := []string{}
		for _, member := range n.Body {
			if field, ok := member.(*ir.RecordField); ok {
				fields = append(fields, ":"+field.Name)
			}
		}
		g.line(n.Name+" = Data.define("+strings.Join(fields, ", ")+")", n.TrailingComment)
	case *ir.Module:
		g.line("module "+n.Name, n.TrailingComment)
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("end", "")
	case *ir.Interface:
		g.line("module "+n.Name, n.TrailingComment)
		g.indent++
		for _, method := range n.Methods {
			g.line("def "+method.Name+"("+g.parameters(method.Parameters)+")", method.TrailingComment)
			g.indent++
			g.line("raise NotImplementedError", "")
			g.indent--
			g.line("end", "")
		}
		g.indent--
		g.line("end", "")
	case *ir.Method:
		g.method(n, nil)
	case *ir.Variable:
		g.line(n.Name+" = "+g.expr(n.Value), n.TrailingComment)
	case *ir.Assignment:
		g.line(g.expr(n.Target)+" "+n.Operator+" "+g.expr(n.Value), n.TrailingComment)
	case *ir.Return:
		text := "return"
		if n.Value != nil {
			text += " " + g.expr(n.Value)
		}
		g.line(text, n.TrailingComment)
	case *ir.ExpressionStatement:
		g.line(g.expr(n.Expression), n.TrailingComment)
	case *ir.If:
		g.line("if "+g.expr(n.Condition), n.TrailingComment)
		g.indent++
		g.statements(n.Then)
		g.indent--
		for _, branch := range n.ElseIf {
			g.line("elsif "+g.expr(branch.Condition), "")
			g.indent++
			g.statements(branch.Body)
			g.indent--
		}
		if len(n.Else) > 0 {
			g.line("else", "")
			g.indent++
			g.statements(n.Else)
			g.indent--
		}
		g.line("end", "")
	case *ir.While:
		g.line("while "+g.expr(n.Condition), n.TrailingComment)
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("end", "")
	case *ir.Iterate:
		source := g.expr(n.Source)
		if _, rangeSource := n.Source.(*ir.Range); rangeSource {
			source = "(" + source + ")"
		}
		header := source + "." + n.Operation
		if n.Operation == "each_slice" {
			header += "(" + g.expr(n.SliceSize) + ")"
		}
		if n.WithIndex {
			header += ".with_index"
		}
		parameters := []string{n.Item}
		if n.WithIndex {
			parameters = append(parameters, n.Index)
		}
		g.line(header+" do |"+strings.Join(parameters, ", ")+"|", n.TrailingComment)
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("end", "")
	case *ir.Native:
		g.nativeLines(n.Text, n.TrailingComment)
	case *ir.NativeBlock:
		g.nativeLines(n.Header, n.TrailingComment)
		g.indent++
		g.statements(n.Body)
		g.indent--
		closer := n.Closer
		if closer == "" {
			closer = "end"
		}
		g.line(closer, "")
	}
}

func (g *generator) method(method *ir.Method, fields []*ir.Field) {
	name := method.Name
	if method.Class {
		name = "self." + name
	}
	g.line("def "+name+"("+g.parameters(method.Parameters)+")", method.TrailingComment)
	g.indent++
	if method.Name == "initialize" {
		g.fieldDefaults(fields)
	}
	g.statements(method.Body)
	g.indent--
	g.line("end", "")
}

func (g *generator) parameters(parameters []ir.Parameter) string {
	parts := make([]string, 0, len(parameters))
	for _, parameter := range parameters {
		name := parameter.Name
		if parameter.KeywordRest {
			name = "**" + name
		} else if parameter.Rest {
			name = "*" + name
		}
		if parameter.Keyword {
			name += ":"
		}
		if parameter.Default != nil {
			if parameter.Keyword {
				name += " " + g.expr(parameter.Default)
			} else {
				name += " = " + g.expr(parameter.Default)
			}
		}
		parts = append(parts, name)
	}
	return strings.Join(parts, ", ")
}

func (g *generator) expr(expression ir.Expression) string {
	if expression == nil {
		return ""
	}
	switch n := expression.(type) {
	case *ir.Identifier:
		return n.Name
	case *ir.Literal:
		return n.Raw
	case *ir.InterpolatedString:
		return n.Raw
	case *ir.Symbol:
		if n.Raw != "" {
			return ":" + n.Raw
		}
		return ":" + n.Name
	case *ir.Array:
		parts := make([]string, len(n.Elements))
		for i, element := range n.Elements {
			parts[i] = g.expr(element)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *ir.Hash:
		parts := make([]string, len(n.Entries))
		for i, entry := range n.Entries {
			parts[i] = g.expr(entry.Key) + " => " + g.expr(entry.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *ir.Unary:
		return n.Operator + g.expr(n.Operand)
	case *ir.Binary:
		return g.expr(n.Left) + " " + n.Operator + " " + g.expr(n.Right)
	case *ir.Range:
		operator := ".."
		if n.Exclusive {
			operator = "..."
		}
		return g.expr(n.Start) + operator + g.expr(n.End)
	case *ir.Member:
		op := "."
		if n.Namespace {
			op = "::"
		} else if n.Safe {
			op = "&."
		}
		return g.expr(n.Receiver) + op + n.Name
	case *ir.Call:
		parts := make([]string, len(n.Arguments))
		for i, argument := range n.Arguments {
			value := g.expr(argument.Value)
			if argument.Splat != "" {
				value = argument.Splat + value
			}
			if argument.Name != "" {
				value = argument.Name + ": " + value
			}
			parts[i] = value
		}
		if reference := expressionReference(n.Callee); reference != nil && reference.Intrinsic != "" {
			return g.intrinsic(reference.Intrinsic, parts)
		}
		return g.expr(n.Callee) + "(" + strings.Join(parts, ", ") + ")"
	case *ir.Index:
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	case *ir.NativeExpression:
		return n.Text
	default:
		return ""
	}
}

func (g *generator) intrinsic(name string, arguments []string) string {
	switch name {
	case "trb.std.io.puts":
		return "$stdout.puts(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.strings.length":
		return arguments[0] + ".each_codepoint.count"
	case "trb.std.strings.uppercase":
		return arguments[0] + ".upcase"
	case "trb.std.strings.lowercase":
		return arguments[0] + ".downcase"
	case "trb.std.strings.contains":
		return arguments[0] + ".include?(" + arguments[1] + ")"
	case "trb.std.arrays.length":
		return arguments[0] + ".length"
	case "trb.std.arrays.push":
		return arguments[0] + " << " + arguments[1]
	case "trb.std.numbers.to_string":
		return arguments[0] + ".to_s"
	case "trb.std.numbers.parse_integer":
		return "Integer(" + arguments[0] + ")"
	default:
		return "nil"
	}
}

func expressionReference(expression ir.Expression) *ir.Reference {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Reference
	case *ir.Member:
		return node.Reference
	default:
		return nil
	}
}

func rubyImportPath(modulePath, imported string) string {
	currentDirectory := pathpkg.Dir(modulePath)
	relative, _ := slashRelative(currentDirectory, imported)
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative
}

func slashRelative(base, target string) (string, error) {
	baseParts := strings.Split(pathpkg.Clean(base), "/")
	targetParts := strings.Split(pathpkg.Clean(target), "/")
	if len(baseParts) == 1 && baseParts[0] == "." {
		baseParts = nil
	}
	for len(baseParts) > 0 && len(targetParts) > 0 && baseParts[0] == targetParts[0] {
		baseParts = baseParts[1:]
		targetParts = targetParts[1:]
	}
	parts := make([]string, len(baseParts))
	for i := range parts {
		parts[i] = ".."
	}
	parts = append(parts, targetParts...)
	return strings.Join(parts, "/"), nil
}

func classFields(statements []ir.Statement) []*ir.Field {
	var fields []*ir.Field
	for _, statement := range statements {
		if field, ok := statement.(*ir.Field); ok {
			fields = append(fields, field)
		}
	}
	return fields
}

func hasDefaults(fields []*ir.Field) bool {
	for _, field := range fields {
		if field.Value != nil {
			return true
		}
	}
	return false
}

func (g *generator) fieldDefaults(fields []*ir.Field) {
	for _, field := range fields {
		if field.Value != nil {
			g.line(field.Name+" = "+g.expr(field.Value), field.TrailingComment)
		}
	}
}

func (g *generator) nativeLines(text, trailing string) {
	lines := strings.Split(strings.TrimSpace(text), "\n")
	for i, line := range lines {
		comment := ""
		if i == len(lines)-1 {
			comment = trailing
		}
		g.line(strings.TrimSpace(line), comment)
	}
}

func (g *generator) line(text, trailing string) {
	g.b.WriteString(strings.Repeat("  ", g.indent))
	g.b.WriteString(text)
	if trailing != "" {
		if text != "" {
			g.b.WriteByte(' ')
		}
		g.b.WriteString(trailing)
	}
	g.b.WriteByte('\n')
}
