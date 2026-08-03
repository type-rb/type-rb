package typescript

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type generator struct {
	b             strings.Builder
	indent        int
	inClass       int
	functionDepth int
	methods       map[string]bool
	modulePath    string
	topFunctions  map[string]bool
	records       map[string]bool
	temporary     int
}

func Generate(program *ir.Program) string {
	g := &generator{modulePath: program.ModulePath, topFunctions: map[string]bool{}, records: map[string]bool{}}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok {
			g.topFunctions[method.Name] = true
		}
		if record, ok := statement.(*ir.Record); ok {
			g.records[record.Name] = true
		}
	}
	for i, statement := range program.Statements {
		if i > 0 {
			g.b.WriteByte('\n')
		}
		g.statement(statement)
	}
	if g.topFunctions["main"] {
		if len(program.Statements) > 0 {
			g.b.WriteByte('\n')
		}
		g.line("main();")
	}
	return strings.TrimRight(g.b.String(), "\n") + "\n"
}

func (g *generator) statements(statements []ir.Statement) {
	for _, statement := range statements {
		g.statement(statement)
	}
}

func (g *generator) statement(statement ir.Statement) {
	switch n := statement.(type) {
	case *ir.Comment:
		g.line(comment(n.Text))
	case *ir.Import:
		if n.Standard {
			if n.Path == "trb/platform/typescript/react" {
				g.line(`import React from "react";`)
				g.line(`import { createRoot } from "react-dom/client";`)
			}
			return
		}
		importPath := tsImportPath(g.modulePath, n.Path)
		if len(n.Symbols) > 0 {
			g.line("import { " + strings.Join(n.Symbols, ", ") + " } from " + strconv.Quote(importPath) + ";")
		} else if n.Alias != "" {
			g.line("import * as " + n.Alias + " from " + strconv.Quote(importPath) + ";")
		} else {
			g.line("import " + strconv.Quote(importPath) + ";")
		}
	case *ir.Class:
		header := "export class " + n.Name
		if n.Superclass != nil {
			if identifier, ok := n.Superclass.(*ir.Identifier); ok && identifier.Name == "ReactComponent" {
				header += " extends React.Component<Record<string, never>>"
			} else {
				header += " extends " + g.expr(n.Superclass)
			}
		}
		if len(n.Implements) > 0 {
			header += " implements " + strings.Join(n.Implements, ", ")
		}
		g.line(header + " {")
		g.indent++
		g.inClass++
		previousMethods := g.methods
		g.methods = classMethods(n.Body)
		for _, member := range n.Body {
			g.statement(member)
		}
		g.methods = previousMethods
		g.inClass--
		g.indent--
		g.line("}")
	case *ir.Record:
		g.line("export interface " + n.Name + " {")
		g.indent++
		for _, member := range n.Body {
			switch field := member.(type) {
			case *ir.Comment:
				g.statement(field)
			case *ir.RecordField:
				g.line(field.Name + ": " + tsType(field.Type) + ";")
			}
		}
		g.indent--
		g.line("}")
	case *ir.Module:
		g.line("export namespace " + n.Name + " {")
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("}")
	case *ir.Interface:
		g.line("export interface " + n.Name + " {")
		g.indent++
		for _, method := range n.Methods {
			g.line(method.Name + "(" + g.parameters(method.Parameters) + "): " + tsType(method.ReturnType) + ";")
		}
		g.indent--
		g.line("}")
	case *ir.Field:
		name := strings.TrimPrefix(n.Name, "@")
		prefix := ""
		if strings.HasPrefix(name, "_") {
			prefix = "private "
			name = strings.TrimPrefix(name, "_")
		}
		name = "__trb_" + name
		if n.ReadOnly {
			prefix += "readonly "
		}
		text := prefix + name + ": " + tsType(n.Type)
		if n.Value != nil {
			text += " = " + g.expr(n.Value)
		}
		g.line(text + ";")
	case *ir.Method:
		if g.inClass > 0 {
			g.method(n)
		} else {
			g.function(n)
		}
	case *ir.Variable:
		if g.inClass > 0 && g.functionDepth == 0 && n.Constant {
			g.line("static readonly " + n.Name + ": " + tsType(n.Type) + " = " + g.expr(n.Value) + ";")
			break
		}
		keyword := "const"
		if n.Mutable {
			keyword = "let"
		}
		if n.Constant {
			keyword = "const"
		}
		prefix := ""
		if g.functionDepth == 0 && n.Constant {
			prefix = "export "
		}
		g.line(prefix + keyword + " " + n.Name + ": " + tsType(n.Type) + " = " + g.expr(n.Value) + ";")
	case *ir.Assignment:
		target := g.expr(n.Target)
		if n.Operator == "/=" && n.Target.ExprType().Kind == types.Int {
			g.line(target + " = Math.trunc(" + target + " / " + g.expr(n.Value) + ");")
		} else {
			g.line(target + " " + n.Operator + " " + g.expr(n.Value) + ";")
		}
	case *ir.Return:
		if n.Value == nil {
			g.line("return;")
		} else {
			g.line("return " + g.expr(n.Value) + ";")
		}
	case *ir.ExpressionStatement:
		g.line(g.expr(n.Expression) + ";")
	case *ir.If:
		g.line("if (" + g.expr(n.Condition) + ") {")
		g.indent++
		g.statements(n.Then)
		g.indent--
		for _, branch := range n.ElseIf {
			g.line("} else if (" + g.expr(branch.Condition) + ") {")
			g.indent++
			g.statements(branch.Body)
			g.indent--
		}
		if len(n.Else) > 0 {
			g.line("} else {")
			g.indent++
			g.statements(n.Else)
			g.indent--
		}
		g.line("}")
	case *ir.While:
		g.line("while (" + g.expr(n.Condition) + ") {")
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("}")
	case *ir.Iterate:
		g.iterate(n)
	}
}

func (g *generator) iterate(iteration *ir.Iterate) {
	if iteration.Operation == "each_slice" {
		g.temporary++
		suffix := strconv.Itoa(g.temporary)
		items := "__trbItems" + suffix
		size := "__trbSize" + suffix
		offset := "__trbOffset" + suffix
		g.line("{")
		g.indent++
		g.line("const " + items + " = " + g.expr(iteration.Source) + ";")
		g.line("const " + size + " = " + g.expr(iteration.SliceSize) + ";")
		g.line("if (" + size + " <= 0) throw new Error(\"each_slice size must be greater than zero\");")
		g.line("for (let " + offset + " = 0; " + offset + " < " + items + ".length; " + offset + " += " + size + ") {")
		g.indent++
		g.line("let " + iteration.Item + " = " + items + ".slice(" + offset + ", " + offset + " + " + size + ");")
		g.line("void " + iteration.Item + ";")
		if iteration.WithIndex {
			g.line("let " + iteration.Index + " = Math.floor(" + offset + " / " + size + ");")
			g.line("void " + iteration.Index + ";")
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		g.indent--
		g.line("}")
		return
	}
	if iteration.WithIndex {
		g.line("for (let [" + iteration.Index + ", " + iteration.Item + "] of " + g.expr(iteration.Source) + ".entries()) {")
	} else {
		g.line("for (let " + iteration.Item + " of " + g.expr(iteration.Source) + ") {")
	}
	g.indent++
	g.line("void " + iteration.Item + ";")
	if iteration.WithIndex {
		g.line("void " + iteration.Index + ";")
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
}

func (g *generator) method(method *ir.Method) {
	if method.Name == "initialize" {
		g.line("constructor(" + g.parameters(method.Parameters) + ") {")
	} else {
		prefix := ""
		name := method.Name
		if name == "component_did_mount" {
			name = "componentDidMount"
		}
		if method.Class {
			prefix = "static "
		}
		if strings.HasPrefix(name, "_") {
			prefix += "private "
			name = strings.TrimPrefix(name, "_")
		}
		g.line(prefix + name + "(" + g.parameters(method.Parameters) + "): " + tsType(method.ReturnType) + " {")
	}
	g.indent++
	g.functionDepth++
	g.statements(method.Body)
	g.functionDepth--
	g.indent--
	g.line("}")
}

func (g *generator) function(method *ir.Method) {
	name := method.Name
	prefix := "export function "
	if name == "main" {
		prefix = "function "
	}
	g.line(prefix + name + "(" + g.parameters(method.Parameters) + "): " + tsType(method.ReturnType) + " {")
	g.indent++
	g.functionDepth++
	g.statements(method.Body)
	g.functionDepth--
	g.indent--
	g.line("}")
}

func (g *generator) parameters(parameters []ir.Parameter) string {
	parts := make([]string, len(parameters))
	for i, parameter := range parameters {
		name := parameter.Name
		if parameter.Rest || parameter.KeywordRest {
			name = "..." + name
		}
		part := name + ": " + tsType(parameter.Type)
		if parameter.Default != nil {
			part += " = " + g.expr(parameter.Default)
		}
		parts[i] = part
	}
	return strings.Join(parts, ", ")
}

func (g *generator) expr(expression ir.Expression) string {
	if expression == nil {
		return ""
	}
	switch n := expression.(type) {
	case *ir.Identifier:
		if strings.HasPrefix(n.Name, "@") {
			return "this.__trb_" + strings.TrimPrefix(strings.TrimPrefix(n.Name, "@"), "_")
		}
		if n.Name == "nil" {
			return "null"
		}
		if n.Name == "self" {
			return "this"
		}
		if g.inClass > 0 && g.methods[n.Name] {
			return "this." + strings.TrimPrefix(n.Name, "_") + ".bind(this)"
		}
		if n.Owner != "" {
			return strings.ReplaceAll(n.Owner, "::", ".") + "." + n.Name
		}
		return n.Name
	case *ir.Literal:
		if n.Kind == "nil" {
			return "null"
		}
		return n.Raw
	case *ir.InterpolatedString:
		var value strings.Builder
		value.WriteByte('`')
		for _, part := range n.Parts {
			if part.Expression != nil {
				value.WriteString("${")
				value.WriteString(g.expr(part.Expression))
				value.WriteByte('}')
			} else {
				value.WriteString(strings.ReplaceAll(part.Text, "`", "\\`"))
			}
		}
		value.WriteByte('`')
		return value.String()
	case *ir.Symbol:
		return strconv.Quote(n.Name)
	case *ir.Array:
		parts := make([]string, len(n.Elements))
		for i, element := range n.Elements {
			parts[i] = g.expr(element)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *ir.Hash:
		parts := make([]string, len(n.Entries))
		for i, entry := range n.Entries {
			parts[i] = "[" + g.expr(entry.Key) + "]: " + g.expr(entry.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *ir.Unary:
		op := n.Operator
		if op == "not" {
			op = "!"
		}
		return op + g.unaryOperand(n.Operand)
	case *ir.Binary:
		op := n.Operator
		if op == "and" {
			op = "&&"
		} else if op == "or" {
			op = "||"
		}
		left := g.binaryOperand(n.Left)
		right := g.binaryOperand(n.Right)
		if op == "**" && n.ExprType().Kind == types.Int {
			return "((base: number, exponent: number): number => { if (exponent < 0) { throw new RangeError(\"negative Integer exponent\"); } return Math.trunc(base ** exponent); })(" + left + ", " + right + ")"
		}
		if op == "/" && n.ExprType().Kind == types.Int {
			return "Math.trunc(" + left + " / " + right + ")"
		}
		return left + " " + op + " " + right
	case *ir.Range:
		extra := "1"
		if n.Exclusive {
			extra = "0"
		}
		return "((start: number, end: number) => Array.from({ length: Math.max(0, end - start + " + extra + ") }, (_, index) => start + index))(" + g.expr(n.Start) + ", " + g.expr(n.End) + ")"
	case *ir.Member:
		receiver := g.expr(n.Receiver)
		name := n.Name
		if name == "to_s" {
			name = "toString"
		}
		op := "."
		if n.Safe {
			op = "?."
		}
		return receiver + op + name
	case *ir.Call:
		parts := make([]string, len(n.Arguments))
		for i, argument := range n.Arguments {
			value := g.expr(argument.Value)
			if argument.Splat != "" {
				value = "..." + value
			}
			if argument.Name != "" {
				value = argument.Name + ": " + value
			}
			parts[i] = value
		}
		args := strings.Join(parts, ", ")
		if reference := expressionReference(n.Callee); reference != nil && reference.Intrinsic != "" {
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		if member, ok := n.Callee.(*ir.Member); ok && member.Name == "new" {
			if identifier, ok := member.Receiver.(*ir.Identifier); ok && (g.records[identifier.Name] || identifier.Reference != nil && identifier.Reference.ExportKind == "record") {
				return g.recordLiteral(identifier, n.Arguments)
			}
			return "new " + g.expr(member.Receiver) + "(" + args + ")"
		}
		if identifier, ok := n.Callee.(*ir.Identifier); ok && g.inClass > 0 && g.methods[identifier.Name] {
			return "this." + strings.TrimPrefix(identifier.Name, "_") + "(" + args + ")"
		}
		return g.expr(n.Callee) + "(" + args + ")"
	case *ir.Index:
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	default:
		return ""
	}
}

func (g *generator) binaryOperand(expression ir.Expression) string {
	value := g.expr(expression)
	switch expression.(type) {
	case *ir.Binary, *ir.Range, *ir.Unary:
		return "(" + value + ")"
	default:
		return value
	}
}

func (g *generator) unaryOperand(expression ir.Expression) string {
	value := g.expr(expression)
	switch expression.(type) {
	case *ir.Binary, *ir.Range:
		return "(" + value + ")"
	default:
		return value
	}
}

func (g *generator) recordLiteral(record *ir.Identifier, arguments []ir.CallArgument) string {
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, argument.Name+": "+g.expr(argument.Value))
	}
	return "({" + strings.Join(fields, ", ") + "} satisfies " + record.Name + ")"
}

func (g *generator) intrinsic(name string, _ *ir.Call, arguments []string) string {
	switch name {
	case "trb.std.io.puts":
		return "console.log(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.strings.length":
		return "Array.from(" + arguments[0] + ").length"
	case "trb.std.strings.uppercase":
		return arguments[0] + ".toUpperCase()"
	case "trb.std.strings.lowercase":
		return arguments[0] + ".toLowerCase()"
	case "trb.std.strings.contains":
		return arguments[0] + ".includes(" + arguments[1] + ")"
	case "trb.std.arrays.length":
		return arguments[0] + ".length"
	case "trb.std.arrays.push":
		return arguments[0] + ".push(" + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		return "String(" + arguments[0] + ")"
	case "trb.std.numbers.parse_integer":
		return "Number.parseInt(" + arguments[0] + ", 10)"
	case "trb.platform.typescript.node.argv":
		return "process.argv.slice(2)"
	case "trb.platform.typescript.react.element":
		return "React.createElement(" + arguments[0] + ", " + arguments[1] + " as any, ..." + arguments[2] + ")"
	case "trb.platform.typescript.react.mount":
		return "createRoot(document.getElementById(" + arguments[1] + ")!).render(React.createElement(" + arguments[0] + "))"
	case "trb.platform.typescript.react.refresh":
		return arguments[0] + ".forceUpdate()"
	case "trb.platform.typescript.react.prevent_default":
		return arguments[0] + ".preventDefault()"
	case "trb.platform.typescript.react.input_value":
		return "(" + arguments[0] + ".currentTarget as HTMLInputElement).value"
	case "trb.platform.typescript.react.data_integer":
		return "Number((" + arguments[0] + ".currentTarget as HTMLElement).dataset[" + arguments[1] + "])"
	case "trb.platform.typescript.react.data_boolean":
		return "((" + arguments[0] + ".currentTarget as HTMLElement).dataset[" + arguments[1] + "] === \"true\")"
	case "trb.platform.typescript.web.get_json":
		return "void fetch(" + arguments[0] + ").then((response) => { if (!response.ok) throw new Error(`HTTP ${response.status}`); return response.json(); }).then(" + arguments[1] + " as any)"
	case "trb.platform.typescript.web.post_json":
		return g.fetchJSON("POST", arguments)
	case "trb.platform.typescript.web.patch_json":
		return g.fetchJSON("PATCH", arguments)
	default:
		return "undefined"
	}
}

func (g *generator) fetchJSON(method string, arguments []string) string {
	return "void fetch(" + arguments[0] + ", { method: \"" + method + "\", headers: { \"Content-Type\": \"application/json\" }, body: JSON.stringify(" + arguments[1] + ") }).then((response) => { if (!response.ok) throw new Error(`HTTP ${response.status}`); return response.json(); }).then(" + arguments[2] + " as any)"
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

func tsImportPath(modulePath, imported string) string {
	currentDirectory := pathpkg.Dir(modulePath)
	if currentDirectory == "." || currentDirectory == "" {
		currentDirectory = "."
	}
	relative, err := filepathRel(currentDirectory, imported)
	if err != nil {
		relative = imported
	}
	if !strings.HasPrefix(relative, ".") {
		relative = "./" + relative
	}
	return relative + ".ts"
}

func filepathRel(base, target string) (string, error) {
	// path.Rel is unavailable; clean slash paths and compute the small
	// module-relative form without involving host path separators.
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
	if len(parts) == 0 {
		return ".", nil
	}
	return strings.Join(parts, "/"), nil
}

func tsType(t types.Type) string {
	var result string
	switch t.Kind {
	case types.Void:
		result = "void"
	case types.Any, types.Invalid:
		result = "unknown"
	case types.Bool:
		result = "boolean"
	case types.Int, types.Float:
		result = "number"
	case types.String:
		result = "string"
	case types.Array, types.Iterable:
		element := "unknown"
		if len(t.Args) > 0 {
			element = tsType(t.Args[0])
		}
		result = "Array<" + element + ">"
	case types.Range:
		result = "Array<number>"
	case types.Hash:
		result = "Record<string, unknown>"
	case types.Nil:
		result = "null"
	default:
		switch t.Name {
		case "ReactNode":
			result = "React.ReactNode"
		case "ReactEvent":
			result = "React.SyntheticEvent"
		case "ReactComponent":
			result = "React.Component<Record<string, never>>"
		case "Callback":
			argument := "unknown"
			if len(t.Args) > 0 {
				argument = tsType(t.Args[0])
			}
			result = "(value: " + argument + ") => void"
		default:
			result = t.Name
		}
	}
	if t.Nullable && result != "null" {
		result += " | null"
	}
	return result
}

func comment(text string) string {
	return "//" + strings.TrimPrefix(strings.TrimSpace(text), "#")
}

func classMethods(statements []ir.Statement) map[string]bool {
	methods := map[string]bool{}
	for _, statement := range statements {
		if method, ok := statement.(*ir.Method); ok {
			methods[method.Name] = true
		}
	}
	return methods
}

func (g *generator) line(text string) {
	g.b.WriteString(strings.Repeat("  ", g.indent))
	g.b.WriteString(text)
	g.b.WriteByte('\n')
}
