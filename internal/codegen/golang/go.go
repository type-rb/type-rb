package golang

import (
	"go/format"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type generator struct {
	b             strings.Builder
	indent        int
	functionDepth int
	receiver      string
	inConstructor bool
	methods       map[string]bool
	topMethods    map[string]bool
	staticMethods map[string]map[string]bool
	records       map[string]bool
	classes       map[string]bool
	typeAliases   map[string]string
	typeKinds     map[string]string
	imports       map[string]string
	modulePath    string
	goModule      string
	temporary     int
}

func Generate(program *ir.Program) string {
	g := &generator{
		topMethods:    map[string]bool{},
		staticMethods: map[string]map[string]bool{},
		records:       map[string]bool{},
		classes:       map[string]bool{},
		typeAliases:   map[string]string{},
		typeKinds:     map[string]string{},
		imports:       map[string]string{},
		modulePath:    program.ModulePath,
		goModule:      program.GoModule,
	}
	for _, statement := range program.Statements {
		switch n := statement.(type) {
		case *ir.Method:
			g.topMethods[n.Name] = true
		case *ir.Class:
			g.classes[n.Name] = true
			for _, member := range n.Body {
				if method, ok := member.(*ir.Method); ok && method.Class {
					if g.staticMethods[n.Name] == nil {
						g.staticMethods[n.Name] = map[string]bool{}
					}
					g.staticMethods[n.Name][method.Name] = true
				}
			}
		case *ir.Record:
			g.records[n.Name] = true
		case *ir.Enum:
			g.typeKinds[n.Name] = "enum"
		}
	}
	for _, statement := range program.Statements {
		if imp, ok := statement.(*ir.Import); ok {
			g.importStatement(imp)
		}
	}
	for _, statement := range program.Statements {
		if _, ok := statement.(*ir.Import); ok {
			continue
		}
		g.statement(statement)
	}
	packageName := program.Package
	if packageName == "" {
		packageName = "main"
	}
	var output strings.Builder
	output.WriteString("package " + goIdentifier(packageName, false) + "\n\n")
	paths := make([]string, 0, len(g.imports))
	for importPath := range g.imports {
		paths = append(paths, importPath)
	}
	sort.Strings(paths)
	for _, importPath := range paths {
		alias := g.imports[importPath]
		if alias != "" && alias != pathpkg.Base(importPath) {
			output.WriteString("import " + goImportAlias(alias) + " " + strconv.Quote(importPath) + "\n")
		} else {
			output.WriteString("import " + strconv.Quote(importPath) + "\n")
		}
	}
	if len(paths) > 0 {
		output.WriteByte('\n')
	}
	output.WriteString(g.b.String())
	generated := strings.TrimRight(output.String(), "\n") + "\n"
	if formatted, err := format.Source([]byte(generated)); err == nil {
		return string(formatted)
	}
	return generated
}

func (g *generator) importStatement(imported *ir.Import) {
	if imported.Standard && !imported.Runtime {
		return
	}
	directory := pathpkg.Dir(imported.Path)
	if directory == "." {
		directory = ""
	}
	if directory == g.currentDirectory() {
		return
	}
	importPath := directory
	if g.goModule != "" {
		importPath = pathpkg.Join(g.goModule, directory)
	}
	if importPath == "" {
		return
	}
	alias := imported.Alias
	if alias == "" {
		alias = pathpkg.Base(directory)
	}
	g.requireImport(importPath, alias)
	for _, symbol := range imported.Symbols {
		g.typeAliases[symbol] = goImportAlias(alias)
		g.typeKinds[symbol] = imported.SymbolKinds[symbol]
	}
}

func (g *generator) currentDirectory() string {
	directory := pathpkg.Dir(g.modulePath)
	if directory == "." {
		return ""
	}
	return directory
}

func (g *generator) requireImport(importPath, alias string) {
	if importPath != "" {
		g.imports[importPath] = alias
	}
}

func (g *generator) statement(statement ir.Statement) {
	switch n := statement.(type) {
	case *ir.Comment:
		g.line("//" + strings.TrimPrefix(strings.TrimSpace(n.Text), "#"))
	case *ir.Class:
		g.class(n)
	case *ir.Record:
		g.record(n)
	case *ir.Enum:
		g.enum(n)
	case *ir.Module:
		g.line("// module " + n.Name)
		for _, member := range n.Body {
			g.statement(member)
		}
	case *ir.Interface:
		g.line("type " + goIdentifier(n.Name, true) + " interface {")
		g.indent++
		for _, method := range n.Methods {
			g.line(goMethodName(method.Name) + "(" + g.parameters(method.Parameters) + ")" + g.goReturn(method.ReturnType))
		}
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	case *ir.Method:
		g.topLevelMethod(n)
	case *ir.Variable:
		if g.functionDepth == 0 {
			name := goIdentifier(n.Name, n.Constant)
			if n.Constant {
				name = goConstantIdentifier(n.Owner, n.Name)
			}
			g.line("var " + name + " " + g.goType(n.Type) + " = " + g.expr(n.Value))
		} else {
			g.line(goIdentifier(n.Name, false) + " := " + g.exprExpected(n.Value, n.Type))
		}
	case *ir.Assignment:
		target := g.assignmentTarget(n.Target)
		switch n.Operator {
		case "&&=":
			g.line(target + " = " + target + " && " + g.expr(n.Value))
		case "||=":
			g.line(target + " = " + target + " || " + g.expr(n.Value))
		default:
			g.line(target + " " + n.Operator + " " + g.expr(n.Value))
		}
	case *ir.Return:
		if g.inConstructor && n.Value == nil {
			return
		}
		if n.Value == nil {
			g.line("return")
		} else {
			g.line("return " + g.expr(n.Value))
		}
	case *ir.Break:
		g.line("break")
	case *ir.Next:
		g.line("continue")
	case *ir.ExpressionStatement:
		g.line(g.expr(n.Expression))
	case *ir.If:
		g.line("if " + g.expr(n.Condition) + " {")
		g.indent++
		g.statements(n.Then)
		g.indent--
		for _, branch := range n.ElseIf {
			g.line("} else if " + g.expr(branch.Condition) + " {")
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
	case *ir.Case:
		g.statements(n.Leading)
		g.temporary++
		value := "__trbCase" + strconv.Itoa(g.temporary)
		g.line("{")
		g.indent++
		g.line(value + " := " + g.expr(n.Value) + goTrailingComment(n.TrailingComment))
		for index, branch := range n.Branches {
			header := "if "
			if index > 0 {
				header = "} else if "
			}
			condition := value + " == " + g.expr(branch.Value)
			if branch.PayloadEnum {
				condition = value + ".Kind == " + g.enumTag(branch)
			}
			g.line(header + condition + " {" + goTrailingComment(branch.TrailingComment))
			g.indent++
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				field := goIdentifier(branch.Member, true) + goIdentifier(binding.Field, true)
				g.line(goIdentifier(binding.Name, false) + " := " + value + "." + field)
			}
			g.statements(branch.Body)
			g.indent--
		}
		if n.HasElse {
			g.line("} else {")
			g.indent++
			g.statements(n.Else)
			g.indent--
		} else {
			g.line("} else {")
			g.indent++
			g.line("panic(\"unreachable exhaustive case\")")
			g.indent--
		}
		if len(n.Branches) > 0 {
			g.line("}")
		}
		g.indent--
		g.line("}")
	case *ir.While:
		g.line("for " + g.expr(n.Condition) + " {")
		g.indent++
		g.statements(n.Body)
		g.indent--
		g.line("}")
	case *ir.Iterate:
		g.iterate(n)
	}
}

func (g *generator) iterate(iteration *ir.Iterate) {
	binding := func(index int) ir.IterationBinding {
		if index < len(iteration.Bindings) {
			return iteration.Bindings[index]
		}
		return ir.IterationBinding{Name: "_", Type: types.Type{Kind: types.Any, Name: "Any"}}
	}
	if iteration.Source.ExprType().Kind == types.Hash {
		g.requireImport("maps", "")
		keyBinding := binding(0)
		valueBinding := binding(1)
		key := goIdentifier(keyBinding.Name, false)
		value := goIdentifier(valueBinding.Name, false)
		switch {
		case keyBinding.Name != "_" && valueBinding.Name != "_":
			g.line("for " + key + ", " + value + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		case keyBinding.Name != "_":
			g.line("for " + key + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		case valueBinding.Name != "_":
			g.line("for _, " + value + " := range maps.Clone(" + g.expr(iteration.Source) + ") {")
		default:
			g.line("for range maps.Clone(" + g.expr(iteration.Source) + ") {")
		}
		g.indent++
		if keyBinding.Name != "_" {
			g.line("_ = " + key)
		}
		if valueBinding.Name != "_" {
			g.line("_ = " + value)
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		return
	}
	itemBinding := binding(0)
	item := goIdentifier(itemBinding.Name, false)
	if iteration.Operation == "each_slice" {
		g.temporary++
		suffix := strconv.Itoa(g.temporary)
		items := "__trbItems" + suffix
		size := "__trbSize" + suffix
		offset := "__trbOffset" + suffix
		end := "__trbEnd" + suffix
		g.line("{")
		g.indent++
		g.line(items + " := " + g.expr(iteration.Source))
		g.line(size + " := " + g.expr(iteration.SliceSize))
		g.line("if " + size + " <= 0 {")
		g.indent++
		g.line("panic(\"each_slice size must be greater than zero\")")
		g.indent--
		g.line("}")
		g.line("for " + offset + " := 0; " + offset + " < len(" + items + "); " + offset + " += " + size + " {")
		g.indent++
		if itemBinding.Name != "_" {
			g.line(end + " := min(" + offset + "+" + size + ", len(" + items + "))")
			g.line(item + " := " + items + "[" + offset + ":" + end + "]")
			g.line("_ = " + item)
		}
		if iteration.WithIndex {
			indexBinding := binding(1)
			if indexBinding.Name != "_" {
				index := goIdentifier(indexBinding.Name, false)
				g.line(index + " := " + offset + " / " + size)
				g.line("_ = " + index)
			}
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
		g.indent--
		g.line("}")
		return
	}
	indexBinding := binding(1)
	if iteration.WithIndex && indexBinding.Name != "_" && itemBinding.Name != "_" {
		g.line("for " + goIdentifier(indexBinding.Name, false) + ", " + item + " := range " + g.expr(iteration.Source) + " {")
	} else if iteration.WithIndex && indexBinding.Name != "_" {
		g.line("for " + goIdentifier(indexBinding.Name, false) + " := range " + g.expr(iteration.Source) + " {")
	} else if itemBinding.Name != "_" {
		g.line("for _, " + item + " := range " + g.expr(iteration.Source) + " {")
	} else {
		g.line("for range " + g.expr(iteration.Source) + " {")
	}
	g.indent++
	if itemBinding.Name != "_" {
		g.line("_ = " + item)
	}
	if iteration.WithIndex && indexBinding.Name != "_" {
		g.line("_ = " + goIdentifier(indexBinding.Name, false))
	}
	g.statements(iteration.Body)
	g.indent--
	g.line("}")
}

func (g *generator) exprExpected(expression ir.Expression, expected types.Type) string {
	if array, ok := expression.(*ir.Array); ok && len(array.Elements) == 0 && expected.Kind == types.Array {
		return g.goType(expected) + "{}"
	}
	return g.expr(expression)
}

func (g *generator) record(record *ir.Record) {
	g.line("type " + goIdentifier(record.Name, true) + " struct {")
	g.indent++
	for _, member := range record.Body {
		switch field := member.(type) {
		case *ir.Comment:
			g.statement(field)
		case *ir.RecordField:
			tags := []string{"json:" + strconv.Quote(field.Name)}
			for _, attribute := range field.Attributes {
				if attribute.Name != "gorm" && attribute.Name != "json" || len(attribute.Arguments) == 0 {
					continue
				}
				literal, ok := attribute.Arguments[0].Value.(*ir.Literal)
				if !ok || literal.Kind != "string" {
					continue
				}
				value, err := strconv.Unquote(literal.Raw)
				if err != nil {
					continue
				}
				key := attribute.Name
				if key == "json" {
					tags[0] = "json:" + strconv.Quote(value)
				} else {
					tags = append(tags, key+":"+strconv.Quote(value))
				}
			}
			g.line(goIdentifier(field.Name, true) + " " + g.goType(field.Type) + " `" + strings.Join(tags, " ") + "`")
		}
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) enum(enum *ir.Enum) {
	name := goIdentifier(enum.Name, true)
	if enumHasPayload(enum) {
		g.payloadEnum(enum, name)
		return
	}
	g.line("type " + name + " int" + goTrailingComment(enum.TrailingComment))
	g.b.WriteByte('\n')
	g.line("const (")
	g.indent++
	first := true
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			line := goConstantIdentifier(enum.Name, member.Name)
			if first {
				line += " " + name + " = iota"
				first = false
			}
			g.line(line + goTrailingComment(member.TrailingComment))
		}
	}
	g.indent--
	g.line(")")
	g.b.WriteByte('\n')
}

func (g *generator) payloadEnum(enum *ir.Enum, name string) {
	tagType := name + "Tag"
	g.line("type " + tagType + " int" + goTrailingComment(enum.TrailingComment))
	g.b.WriteByte('\n')
	g.line("const (")
	g.indent++
	first := true
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			line := goConstantIdentifier(enum.Name, member.Name) + "Tag"
			if first {
				line += " " + tagType + " = iota"
				first = false
			}
			g.line(line + goTrailingComment(member.TrailingComment))
		}
	}
	g.indent--
	g.line(")")
	g.b.WriteByte('\n')

	g.line("type " + name + goTypeParameterDeclarations(enum.TypeParameters) + " struct {")
	g.indent++
	g.line("Kind " + tagType)
	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		for _, field := range member.Fields {
			fieldName := goIdentifier(member.Name, true) + goIdentifier(field.Name, true)
			g.line(fieldName + " " + g.goType(field.Type))
		}
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	for _, statement := range enum.Body {
		member, ok := statement.(*ir.EnumMember)
		if !ok {
			continue
		}
		constant := goConstantIdentifier(enum.Name, member.Name)
		if len(member.Fields) == 0 {
			g.line("var " + constant + " = " + name + "{Kind: " + constant + "Tag}")
			continue
		}
		constructor := "New" + goIdentifier(enum.Name, true) + goIdentifier(member.Name, true)
		genericDeclarations := goTypeParameterDeclarations(enum.TypeParameters)
		genericArguments := goTypeParameterArguments(enum.TypeParameters)
		g.line("func " + constructor + genericDeclarations + "(" + g.parameters(member.Fields) + ") " + name + genericArguments + " {")
		g.indent++
		fields := []string{"Kind: " + constant + "Tag"}
		for _, field := range member.Fields {
			fieldName := goIdentifier(member.Name, true) + goIdentifier(field.Name, true)
			fields = append(fields, fieldName+": "+goIdentifier(field.Name, false))
		}
		g.line("return " + name + genericArguments + "{" + strings.Join(fields, ", ") + "}")
		g.indent--
		g.line("}")
	}
	g.b.WriteByte('\n')
}

func enumHasPayload(enum *ir.Enum) bool {
	for _, statement := range enum.Body {
		if member, ok := statement.(*ir.EnumMember); ok && len(member.Fields) > 0 {
			return true
		}
	}
	return false
}

func goTypeParameterDeclarations(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = goIdentifier(parameter, true) + " any"
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func goTypeParameterArguments(parameters []string) string {
	if len(parameters) == 0 {
		return ""
	}
	parts := make([]string, len(parameters))
	for index, parameter := range parameters {
		parts[index] = goIdentifier(parameter, true)
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (g *generator) enumTag(branch ir.CaseBranch) string {
	name := goConstantIdentifier(branch.EnumName, branch.Member) + "Tag"
	if member, ok := branch.Value.(*ir.Member); ok {
		if alias := g.referenceAlias(member.Reference); alias != "" {
			return alias + "." + name
		}
	}
	return name
}

func (g *generator) statements(statements []ir.Statement) {
	for _, statement := range statements {
		g.statement(statement)
	}
}

func (g *generator) class(class *ir.Class) {
	name := goIdentifier(class.Name, true)
	fields := []*ir.Field{}
	methods := []*ir.Method{}
	for _, member := range class.Body {
		switch n := member.(type) {
		case *ir.Field:
			fields = append(fields, n)
		case *ir.Method:
			methods = append(methods, n)
		case *ir.Variable:
			g.statement(n)
		}
	}
	previousMethods := g.methods
	g.methods = map[string]bool{}
	for _, method := range methods {
		g.methods[method.Name] = true
	}
	defer func() { g.methods = previousMethods }()
	g.line("type " + name + " struct {")
	g.indent++
	if class.Superclass != nil {
		g.line("*" + g.expr(class.Superclass))
	}
	for _, field := range fields {
		g.line(goFieldName(field.Name) + " " + g.goType(field.Type))
	}
	g.indent--
	g.line("}")
	for _, interfaceName := range class.Implements {
		g.line("var _ " + g.goType(types.FromName(interfaceName)) + " = (*" + name + ")(nil)")
	}
	g.b.WriteByte('\n')

	initialize := findInitialize(methods)
	{
		parameters := ""
		if initialize != nil {
			parameters = g.parameters(initialize.Parameters)
		}
		g.line("func New" + name + "(" + parameters + ") *" + name + " {")
		g.indent++
		g.line("self := &" + name + "{}")
		for _, field := range fields {
			if field.Value != nil {
				g.line("self." + goFieldName(field.Name) + " = " + g.expr(field.Value))
			}
		}
		previousReceiver, previousConstructor := g.receiver, g.inConstructor
		g.receiver, g.inConstructor = "self", true
		g.functionDepth++
		if initialize != nil {
			g.statements(initialize.Body)
		}
		g.functionDepth--
		g.receiver, g.inConstructor = previousReceiver, previousConstructor
		g.line("return self")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}

	for _, method := range methods {
		if method.Name == "initialize" {
			continue
		}
		g.classMethod(name, method)
	}
}

func (g *generator) classMethod(className string, method *ir.Method) {
	name := goMethodName(method.Name)
	if method.Class {
		g.line("func " + className + name + "(" + g.parameters(method.Parameters) + ")" + g.goReturn(method.ReturnType) + " {")
	} else {
		g.line("func (self *" + className + ") " + name + "(" + g.parameters(method.Parameters) + ")" + g.goReturn(method.ReturnType) + " {")
	}
	g.indent++
	previous := g.receiver
	g.receiver = "self"
	g.functionDepth++
	g.statements(method.Body)
	g.functionDepth--
	g.receiver = previous
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) topLevelMethod(method *ir.Method) {
	name := goMethodName(method.Name)
	if method.Name == "main" {
		name = "main"
	}
	g.line("func " + name + goTypeParameterDeclarations(method.TypeParameters) + "(" + g.parameters(method.Parameters) + ")" + g.goReturn(method.ReturnType) + " {")
	g.indent++
	g.functionDepth++
	g.statements(method.Body)
	g.functionDepth--
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) parameters(parameters []ir.Parameter) string {
	parts := make([]string, len(parameters))
	for i, parameter := range parameters {
		name := goIdentifier(parameter.Name, false)
		typ := g.goType(parameter.Type)
		if parameter.Rest {
			typ = "..." + strings.TrimPrefix(typ, "[]")
		}
		parts[i] = name + " " + typ
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
			return "self." + goFieldName(n.Name)
		}
		if n.Name == "self" {
			return "self"
		}
		if strings.HasPrefix(n.Name, "_") && g.receiver != "" {
			return g.receiver + "." + goMethodName(n.Name)
		}
		if n.Reference != nil && n.Reference.Intrinsic == "" && n.Reference.Package != "" {
			if alias := g.referenceAlias(n.Reference); alias != "" {
				return alias + "." + goImportedName(n.Name, n.Reference.ExportKind)
			}
			if n.Reference.ExportKind == "function" {
				return goMethodName(n.Name)
			}
		}
		if n.Owner != "" {
			return goConstantIdentifier(n.Owner, n.Name)
		}
		if isUpper(n.Name) {
			return goConstantIdentifier("", n.Name)
		}
		return goIdentifier(n.Name, isUpper(n.Name))
	case *ir.Literal:
		if n.Kind == "nil" {
			return "nil"
		}
		return n.Raw
	case *ir.InterpolatedString:
		g.requireImport("fmt", "")
		var format strings.Builder
		var arguments []string
		for _, part := range n.Parts {
			if part.Expression != nil {
				format.WriteString("%v")
				arguments = append(arguments, g.expr(part.Expression))
				continue
			}
			text := part.Text
			if decoded, err := strconv.Unquote("\"" + text + "\""); err == nil {
				text = decoded
			}
			format.WriteString(strings.ReplaceAll(text, "%", "%%"))
		}
		args := ""
		if len(arguments) > 0 {
			args = ", " + strings.Join(arguments, ", ")
		}
		return "fmt.Sprintf(" + strconv.Quote(format.String()) + args + ")"
	case *ir.Symbol:
		return strconv.Quote(n.Name)
	case *ir.Array:
		parts := make([]string, len(n.Elements))
		for i, element := range n.Elements {
			parts[i] = g.expr(element)
		}
		return g.goType(n.ExprType()) + "{" + strings.Join(parts, ", ") + "}"
	case *ir.Hash:
		parts := make([]string, len(n.Entries))
		for i, entry := range n.Entries {
			parts[i] = g.expr(entry.Key) + ": " + g.expr(entry.Value)
		}
		return g.goType(n.ExprType()) + "{" + strings.Join(parts, ", ") + "}"
	case *ir.Unary:
		op := n.Operator
		if op == "not" || op == "!" {
			return "!(" + g.expr(n.Operand) + ")"
		}
		return op + g.unaryOperand(n.Operand)
	case *ir.Conversion:
		switch n.Kind {
		case ir.IntegerToFloatConversion:
			return "float64(" + g.expr(n.Value) + ")"
		default:
			return g.expr(n.Value)
		}
	case *ir.Binary:
		op := n.Operator
		if op == "and" {
			op = "&&"
		} else if op == "or" {
			op = "||"
		}
		left := g.binaryOperand(n.Left)
		right := g.binaryOperand(n.Right)
		if op == "**" {
			if n.ExprType().Kind == types.Int {
				return "func(base int, exponent int) int { if exponent < 0 { panic(\"negative Integer exponent\") }; result := 1; for exponent > 0 { if exponent%2 == 1 { result *= base }; base *= base; exponent /= 2 }; return result }(" + left + ", " + right + ")"
			}
			g.requireImport("math", "math")
			return "math.Pow(" + left + ", " + right + ")"
		}
		return left + " " + op + " " + right
	case *ir.Range:
		inclusiveEnd := ""
		if !n.Exclusive {
			inclusiveEnd = "; if start <= end { values = append(values, end) }"
		}
		return "func() []int { start, end := " + g.expr(n.Start) + ", " + g.expr(n.End) + "; values := []int{}; for value := start; value < end; value++ { values = append(values, value) }" + inclusiveEnd + "; return values }()"
	case *ir.Transform:
		return g.transform(n)
	case *ir.Member:
		if n.Namespace && isUpper(n.Name) {
			owner := n.Receiver.ExprType().Name
			if owner == "" {
				owner = irExpressionName(n.Receiver)
			}
			name := goConstantIdentifier(owner, n.Name)
			if alias := g.referenceAlias(n.Reference); alias != "" {
				return alias + "." + name
			}
			return name
		}
		return g.expr(n.Receiver) + "." + goMethodName(n.Name)
	case *ir.Call:
		parts := make([]string, len(n.Arguments))
		for i, argument := range n.Arguments {
			parts[i] = g.expr(argument.Value)
		}
		args := strings.Join(parts, ", ")
		if reference := expressionReference(n.Callee); reference != nil && reference.Intrinsic != "" {
			if reference.ReceiverMethod {
				if member, ok := n.Callee.(*ir.Member); ok {
					parts = append([]string{g.expr(member.Receiver)}, parts...)
				}
			}
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		if member, ok := n.Callee.(*ir.Member); ok && member.Name == "new" {
			if identifier, ok := member.Receiver.(*ir.Identifier); ok {
				if g.records[identifier.Name] || identifier.Reference != nil && identifier.Reference.ExportKind == "record" {
					return g.recordLiteral(identifier, n.Arguments)
				}
				if alias := g.referenceAlias(identifier.Reference); alias != "" {
					return alias + ".New" + goIdentifier(identifier.Name, true) + "(" + args + ")"
				}
			}
			return "New" + goIdentifier(g.expr(member.Receiver), true) + "(" + args + ")"
		}
		if member, ok := n.Callee.(*ir.Member); ok {
			if receiver, ok := member.Receiver.(*ir.Identifier); ok && g.staticMethods[receiver.Name][member.Name] {
				return goIdentifier(receiver.Name, true) + goMethodName(member.Name) + "(" + args + ")"
			}
		}
		if identifier, ok := n.Callee.(*ir.Identifier); ok {
			if g.receiver != "" && g.methods[identifier.Name] {
				return g.receiver + "." + goMethodName(identifier.Name) + "(" + args + ")"
			}
			if g.topMethods[identifier.Name] {
				name := goMethodName(identifier.Name)
				if identifier.Name == "main" {
					name = "main"
				}
				return name + "(" + args + ")"
			}
		}
		return g.expr(n.Callee) + "(" + args + ")"
	case *ir.EnumConstruct:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument)
		}
		name := "New" + goIdentifier(n.EnumName, true) + goIdentifier(n.Member, true)
		if alias := g.referenceAlias(n.Reference); alias != "" {
			name = alias + "." + name
		}
		if len(n.TypeArguments) > 0 {
			types := make([]string, len(n.TypeArguments))
			for index, argument := range n.TypeArguments {
				types[index] = g.goType(argument)
			}
			name += "[" + strings.Join(types, ", ") + "]"
		}
		return name + "(" + strings.Join(parts, ", ") + ")"
	case *ir.TypeApply:
		arguments := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			arguments[index] = g.goType(argument)
		}
		name := g.expr(n.Receiver)
		if identifier, ok := n.Receiver.(*ir.Identifier); ok && g.topMethods[identifier.Name] {
			name = goMethodName(identifier.Name)
		}
		return name + "[" + strings.Join(arguments, ", ") + "]"
	case *ir.Index:
		if n.Receiver.ExprType().Kind == types.Hash && len(n.Receiver.ExprType().Args) == 2 {
			hashType := n.Receiver.ExprType()
			keyType := g.goType(hashType.Args[0])
			valueType := g.goType(hashType.Args[1])
			return "func(values " + g.goType(hashType) + ", key " + keyType + ") " + valueType + " { value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; return value }(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	default:
		return ""
	}
}

func (g *generator) transform(transform *ir.Transform) string {
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	items := "__trbItems" + suffix
	result := "__trbResult" + suffix
	item := goIdentifier(transform.Item, false)
	if item == "" || item == "_" {
		item = "__trbItem" + suffix
	}
	source := g.expr(transform.Source)
	value := g.expr(transform.Result)
	switch transform.Operation {
	case "map":
		index := "_"
		if transform.WithIndex {
			index = goIdentifier(transform.Index, false)
			if index == "" {
				index = "_"
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + items + " := " + source + "; " + result + " := make(" + g.goType(transform.ExprType()) + ", 0, len(" + items + ")); for " + index + ", " + item + " := range " + items + " { " + result + " = append(" + result + ", " + value + ") }; return " + result + " }()"
	case "select":
		index := "_"
		if transform.WithIndex {
			index = goIdentifier(transform.Index, false)
			if index == "" {
				index = "_"
			}
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + items + " := " + source + "; " + result + " := make(" + g.goType(transform.ExprType()) + ", 0, len(" + items + ")); for " + index + ", " + item + " := range " + items + " { if " + value + " { " + result + " = append(" + result + ", " + item + ") } }; return " + result + " }()"
	case "reduce":
		accumulator := goIdentifier(transform.Accumulator, false)
		binding := ""
		if accumulator != "" && accumulator != "_" {
			binding = accumulator + " := " + result + "; "
		}
		return "func() " + g.goType(transform.ExprType()) + " { " + result + " := " + g.expr(transform.Initial) + "; for _, " + item + " := range " + source + " { " + binding + result + " = " + value + " }; return " + result + " }()"
	default:
		return "nil"
	}
}

func (g *generator) assignmentTarget(expression ir.Expression) string {
	if index, ok := expression.(*ir.Index); ok {
		return g.expr(index.Receiver) + "[" + g.expr(index.Index) + "]"
	}
	return g.expr(expression)
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
	name := goIdentifier(record.Name, true)
	if alias := g.referenceAlias(record.Reference); alias != "" {
		name = alias + "." + name
	}
	fields := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			continue
		}
		fields = append(fields, goIdentifier(argument.Name, true)+": "+g.expr(argument.Value))
	}
	return name + "{" + strings.Join(fields, ", ") + "}"
}

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	unicodeAlias := "unicode"
	pathAlias := "path"
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Alias != "" {
		unicodeAlias = goImportAlias(reference.Alias)
		pathAlias = goImportAlias(reference.Alias)
	}
	unicodeCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return unicodeAlias + "." + goMethodName(symbol)
		}
		return unicodeAlias + ".Unicode" + goMethodName(symbol)
	}
	pathCall := func(symbol string) string {
		return pathAlias + "." + goMethodName(symbol)
	}
	filesystemResultType := func() (string, string, string) {
		result := call.ExprType()
		if len(result.Args) != 2 {
			return g.goType(result), "any", "any"
		}
		return g.goType(result), g.goType(result.Args[0]), g.goType(result.Args[1])
	}
	filesystemOK := func(value string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		return alias + ".NewResultOk[" + successType + ", " + errorType + "](" + value + ")"
	}
	resultError := func(value string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		return alias + ".NewResultErr[" + successType + ", " + errorType + "](" + value + ")"
	}
	filesystemError := func(operation, path, message string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		value := errorType + "{Operation: " + strconv.Quote(operation) + ", Path: " + path + ", Message: " + message + "}"
		return alias + ".NewResultErr[" + successType + ", " + errorType + "](" + value + ")"
	}
	processError := func(operation, command, message string) string {
		_, successType, errorType := filesystemResultType()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		value := errorType + "{Operation: " + strconv.Quote(operation) + ", Command: " + command + ", Message: " + message + "}"
		return alias + ".NewResultErr[" + successType + ", " + errorType + "](" + value + ")"
	}
	switch name {
	case "trb.std.io.puts":
		g.requireImport("fmt", "")
		if len(call.Arguments) == 1 && call.Arguments[0].Value.ExprType().Kind == types.Float {
			return "fmt.Println(" + g.portableFloatString(arguments[0]) + ")"
		}
		return "fmt.Println(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.path.separator":
		return pathCall("separator") + "()"
	case "trb.std.path.clean":
		return pathCall("clean") + "(" + arguments[0] + ")"
	case "trb.std.path.join":
		return pathCall("join") + "(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.path.absolute":
		return pathCall("absolute") + "(" + arguments[0] + ")"
	case "trb.std.path.components":
		return pathCall("components") + "(" + arguments[0] + ")"
	case "trb.std.path.base":
		return pathCall("base") + "(" + arguments[0] + ")"
	case "trb.std.path.directory":
		return pathCall("directory") + "(" + arguments[0] + ")"
	case "trb.internal.filesystem.exists":
		g.requireImport("errors", "")
		g.requireImport("os", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; _, err := os.Stat(path); if err == nil { return " + filesystemOK("true") + " }; if errors.Is(err, os.ErrNotExist) { return " + filesystemOK("false") + " }; return " + filesystemError("exists", "path", "err.Error()") + " }()"
	case "trb.internal.filesystem.read_text":
		g.requireImport("os", "")
		g.requireImport("strings", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; data, err := os.ReadFile(path); if err != nil { return " + filesystemError("read_text", "path", "err.Error()") + " }; return " + filesystemOK("strings.ToValidUTF8(string(data), \"�\")") + " }()"
	case "trb.internal.filesystem.read_bytes":
		g.requireImport("os", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; data, err := os.ReadFile(path); if err != nil { return " + filesystemError("read_bytes", "path", "err.Error()") + " }; return " + filesystemOK("data") + " }()"
	case "trb.internal.filesystem.write_text":
		g.requireImport("os", "")
		resultType, successType, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; err := os.WriteFile(path, []byte(" + arguments[1] + "), 0o644); if err != nil { return " + filesystemError("write_text", "path", "err.Error()") + " }; return " + filesystemOK(successType+"{}") + " }()"
	case "trb.internal.filesystem.write_bytes":
		g.requireImport("os", "")
		resultType, successType, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; err := os.WriteFile(path, " + arguments[1] + ", 0o644); if err != nil { return " + filesystemError("write_bytes", "path", "err.Error()") + " }; return " + filesystemOK(successType+"{}") + " }()"
	case "trb.internal.filesystem.create_directory":
		g.requireImport("os", "")
		resultType, successType, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; err := os.MkdirAll(path, 0o755); if err != nil { return " + filesystemError("create_directory", "path", "err.Error()") + " }; return " + filesystemOK(successType+"{}") + " }()"
	case "trb.internal.filesystem.list":
		g.requireImport("os", "")
		g.requireImport("slices", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { path := " + arguments[0] + "; entries, err := os.ReadDir(path); if err != nil { return " + filesystemError("list", "path", "err.Error()") + " }; names := make([]string, 0, len(entries)); for _, entry := range entries { names = append(names, entry.Name()) }; slices.Sort(names); return " + filesystemOK("names") + " }()"
	case "trb.internal.process.arguments":
		g.requireImport("os", "")
		return "append([]string{}, os.Args[1:]...)"
	case "trb.internal.process.environment":
		g.requireImport("os", "")
		return "func() *string { value, found := os.LookupEnv(" + arguments[0] + "); if !found { return nil }; return &value }()"
	case "trb.internal.process.working_directory":
		g.requireImport("os", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { directory, err := os.Getwd(); if err != nil { return " + processError("working_directory", strconv.Quote(""), "err.Error()") + " }; return " + filesystemOK("directory") + " }()"
	case "trb.internal.process.run":
		g.requireImport("bytes", "")
		g.requireImport("errors", "")
		g.requireImport("os/exec", "exec")
		g.requireImport("strings", "")
		resultType, successType, _ := filesystemResultType()
		value := successType + "{Status: status, Stdout: strings.ToValidUTF8(stdout.String(), \"�\"), Stderr: strings.ToValidUTF8(stderr.String(), \"�\"), Success: status == 0}"
		return "func() " + resultType + " { commandName := " + arguments[0] + "; commandArguments := " + arguments[1] + "; process := exec.Command(commandName, commandArguments...); var stdout bytes.Buffer; var stderr bytes.Buffer; process.Stdout = &stdout; process.Stderr = &stderr; err := process.Run(); status := 0; if err != nil { var exitError *exec.ExitError; if errors.As(err, &exitError) { status = exitError.ExitCode() } else { return " + processError("run", "commandName", "err.Error()") + " } }; return " + filesystemOK(value) + " }()"
	case "trb.internal.json.parse":
		return g.jsonParse(call, arguments[0], false)
	case "trb.internal.json.parse_jsonc":
		if reference := expressionReference(call.Callee); reference != nil && reference.Package == "trb/std/jsonc/index" && g.modulePath != reference.Package {
			alias := reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(reference.Package))
			}
			return goImportAlias(alias) + ".Parse(" + arguments[0] + ")"
		}
		return g.jsonParse(call, arguments[0], true)
	case "trb.internal.json.stringify":
		return g.jsonStringify(call, arguments[0])
	case "trb.internal.json.decode":
		return g.jsonDecode(call, arguments[0])
	case "trb.internal.json.encode":
		return g.jsonEncode(call, arguments[0])
	case "trb.std.strings.length":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.RuneCountInString(" + arguments[0] + ")"
	case "trb.std.strings.empty":
		return arguments[0] + " == \"\""
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		g.requireImport("strings", "")
		g.requireImport("unicode", "")
		function := "TrimFunc"
		if name == "trb.std.strings.lstrip" {
			function = "TrimLeftFunc"
		} else if name == "trb.std.strings.rstrip" {
			function = "TrimRightFunc"
		}
		return "strings." + function + "(" + arguments[0] + ", unicode.IsSpace)"
	case "trb.std.strings.uppercase":
		g.requireImport("strings", "")
		return "strings.ToUpper(" + arguments[0] + ")"
	case "trb.std.strings.lowercase":
		g.requireImport("strings", "")
		return "strings.ToLower(" + arguments[0] + ")"
	case "trb.std.strings.starts_with":
		g.requireImport("strings", "")
		return "strings.HasPrefix(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.ends_with":
		g.requireImport("strings", "")
		return "strings.HasSuffix(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.split":
		g.requireImport("strings", "")
		return "func() []string { value := " + arguments[0] + "; separator := " + arguments[1] + "; if separator == \"\" { panic(\"String split separator is empty\") }; return strings.Split(value, separator) }()"
	case "trb.std.strings.contains":
		g.requireImport("strings", "")
		return "strings.Contains(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.codepoints":
		g.requireImport("unicode/utf8", "utf8")
		return "func(value string) []int { result := make([]int, 0, utf8.RuneCountInString(value)); for _, codepoint := range value { result = append(result, int(codepoint)) }; return result }(" + arguments[0] + ")"
	case "trb.std.unicode.version":
		return unicodeCall("version") + "()"
	case "trb.std.unicode.valid_scalar":
		return unicodeCall("valid_scalar") + "(" + arguments[0] + ")"
	case "trb.std.unicode.letter":
		return unicodeCall("letter") + "(" + arguments[0] + ")"
	case "trb.std.unicode.digit":
		return unicodeCall("digit") + "(" + arguments[0] + ")"
	case "trb.std.unicode.uppercase":
		return unicodeCall("uppercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.lowercase":
		return unicodeCall("lowercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.whitespace":
		return unicodeCall("whitespace") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_start":
		return unicodeCall("identifier_start") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_part":
		return unicodeCall("identifier_part") + "(" + arguments[0] + ")"
	case "trb.std.unicode.from_codepoint":
		return unicodeCall("from_codepoint") + "(" + arguments[0] + ")"
	case "trb.std.bytes.from_string":
		return "[]byte(" + arguments[0] + ")"
	case "trb.std.bytes.to_string":
		g.requireImport("strings", "")
		return "strings.ToValidUTF8(string(" + arguments[0] + "), \"�\")"
	case "trb.std.bytes.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.bytes.at":
		return "func(value []byte, index int) int { if index < 0 || index >= len(value) { panic(\"Bytes index is out of bounds\") }; return int(value[index]) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.bytes.concat":
		return "append(append([]byte{}, " + arguments[0] + "...), " + arguments[1] + "...)"
	case "trb.std.bytes.valid_utf8":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.Valid(" + arguments[0] + ")"
	case "trb.std.string_builder.new":
		g.requireImport("strings", "")
		return "&strings.Builder{}"
	case "trb.std.string_builder.from_string":
		g.requireImport("strings", "")
		return "func(value string) *strings.Builder { builder := &strings.Builder{}; builder.WriteString(value); return builder }(" + arguments[0] + ")"
	case "trb.std.string_builder.append":
		return arguments[0] + ".WriteString(" + arguments[1] + ")"
	case "trb.std.string_builder.append_codepoint":
		return "func(builder *strings.Builder, value int) { if value < 0 || value > 0x10ffff || value >= 0xd800 && value <= 0xdfff { panic(\"invalid Unicode code point\") }; builder.WriteRune(rune(value)) }(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.string_builder.length":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.RuneCountInString(" + arguments[0] + ".String())"
	case "trb.std.string_builder.empty":
		return arguments[0] + ".Len() == 0"
	case "trb.std.string_builder.to_string":
		return arguments[0] + ".String()"
	case "trb.std.string_builder.clear":
		return arguments[0] + ".Reset()"
	case "trb.std.arrays.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.arrays.empty":
		return "len(" + arguments[0] + ") == 0"
	case "trb.std.arrays.fetch":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; index := " + arguments[1] + "; if index < 0 || index >= len(values) { panic(\"Array index is out of bounds\") }; return values[index] }()"
	case "trb.std.arrays.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { values := " + arguments[0] + "; index := " + arguments[1] + "; if index < 0 || index >= len(values) { return " + resultError(strconv.Quote("Array index is out of bounds")) + " }; return " + filesystemOK("values[index]") + " }()"
	case "trb.std.arrays.first":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; if len(values) == 0 { panic(\"Array is empty\") }; return values[0] }()"
	case "trb.std.arrays.last":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; if len(values) == 0 { panic(\"Array is empty\") }; return values[len(values)-1] }()"
	case "trb.std.arrays.copy":
		g.requireImport("slices", "")
		return "slices.Clone(" + arguments[0] + ")"
	case "trb.std.arrays.contains":
		g.requireImport("slices", "")
		return "slices.Contains(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.count":
		return "func() int { values := " + arguments[0] + "; target := " + arguments[1] + "; count := 0; for _, value := range values { if value == target { count++ } }; return count }()"
	case "trb.std.arrays.join":
		g.requireImport("strings", "")
		return "strings.Join(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.pop":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; if len(values) == 0 { panic(\"Array is empty\") }; index := len(values) - 1; value := values[index]; " + arguments[0] + " = values[:index]; return value }()"
	case "trb.std.arrays.shift":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; if len(values) == 0 { panic(\"Array is empty\") }; value := values[0]; " + arguments[0] + " = values[1:]; return value }()"
	case "trb.std.arrays.push":
		return arguments[0] + " = append(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.unshift":
		return "func() { values := " + arguments[0] + "; value := " + arguments[1] + "; values = append(values, value); copy(values[1:], values[:len(values)-1]); values[0] = value; " + arguments[0] + " = values }()"
	case "trb.std.arrays.reverse":
		g.requireImport("slices", "")
		return "func() " + g.goType(call.ExprType()) + " { values := slices.Clone(" + arguments[0] + "); slices.Reverse(values); return values }()"
	case "trb.std.hashes.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.hashes.empty":
		return "len(" + arguments[0] + ") == 0"
	case "trb.std.hashes.fetch":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; return value }()"
	case "trb.std.hashes.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { return " + resultError(strconv.Quote("Hash key is missing")) + " }; return " + filesystemOK("value") + " }()"
	case "trb.std.hashes.contains_key":
		return "func() bool { values := " + arguments[0] + "; key := " + arguments[1] + "; _, ok := values[key]; return ok }()"
	case "trb.std.hashes.keys":
		g.requireImport("maps", "")
		g.requireImport("slices", "")
		return "slices.Collect(maps.Keys(" + arguments[0] + "))"
	case "trb.std.hashes.values":
		g.requireImport("maps", "")
		g.requireImport("slices", "")
		return "slices.Collect(maps.Values(" + arguments[0] + "))"
	case "trb.std.hashes.copy":
		g.requireImport("maps", "")
		return "maps.Clone(" + arguments[0] + ")"
	case "trb.std.hashes.delete":
		return "func() " + g.goType(call.ExprType()) + " { values := " + arguments[0] + "; key := " + arguments[1] + "; value, ok := values[key]; if !ok { panic(\"Hash key is missing\") }; delete(values, key); return value }()"
	case "trb.std.hashes.merge":
		g.requireImport("maps", "")
		return "func() " + g.goType(call.ExprType()) + " { values := maps.Clone(" + arguments[0] + "); maps.Copy(values, " + arguments[1] + "); return values }()"
	case "trb.std.hashes.update":
		g.requireImport("maps", "")
		return "maps.Copy(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		g.requireImport("strconv", "")
		return "strconv.Itoa(" + arguments[0] + ")"
	case "trb.std.numbers.integer_to_float":
		return "float64(" + arguments[0] + ")"
	case "trb.std.numbers.integer_absolute":
		return "func(value int) int { if value < 0 { return -value }; return value }(" + arguments[0] + ")"
	case "trb.std.numbers.integer_zero":
		return arguments[0] + " == 0"
	case "trb.std.numbers.integer_positive":
		return arguments[0] + " > 0"
	case "trb.std.numbers.integer_negative":
		return arguments[0] + " < 0"
	case "trb.std.numbers.integer_even":
		return arguments[0] + "%2 == 0"
	case "trb.std.numbers.integer_odd":
		return arguments[0] + "%2 != 0"
	case "trb.std.numbers.float_to_string":
		return g.portableFloatString(arguments[0])
	case "trb.std.numbers.float_to_integer":
		g.requireImport("math", "")
		return "func() int { value := " + arguments[0] + "; if math.IsNaN(value) || math.IsInf(value, 0) { panic(\"Float cannot be converted to Integer\") }; if value < -9007199254740991 || value > 9007199254740991 { panic(\"Integer is outside the portable range\") }; return int(math.Trunc(value)) }()"
	case "trb.std.numbers.float_absolute":
		g.requireImport("math", "")
		return "math.Abs(" + arguments[0] + ")"
	case "trb.std.numbers.float_finite":
		g.requireImport("math", "")
		return "func(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }(" + arguments[0] + ")"
	case "trb.std.numbers.float_infinite":
		g.requireImport("math", "")
		return "math.IsInf(" + arguments[0] + ", 0)"
	case "trb.std.numbers.float_nan":
		g.requireImport("math", "")
		return "math.IsNaN(" + arguments[0] + ")"
	case "trb.std.numbers.parse_integer":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		return "func() int { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?[0-9]+$`, input); if !valid { panic(\"invalid Integer\") }; value, err := strconv.ParseInt(input, 10, 64); if err != nil || value < -9007199254740991 || value > 9007199254740991 { panic(\"Integer is outside the portable range\") }; return int(value) }()"
	case "trb.std.numbers.try_parse_integer":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		resultType, _, _ := filesystemResultType()
		return "func() " + resultType + " { input := " + arguments[0] + "; valid, _ := regexp.MatchString(`^[+-]?[0-9]+$`, input); if !valid { return " + resultError(strconv.Quote("invalid Integer")) + " }; value, err := strconv.ParseInt(input, 10, 64); if err != nil || value < -9007199254740991 || value > 9007199254740991 { return " + resultError(strconv.Quote("Integer is outside the portable range")) + " }; return " + filesystemOK("int(value)") + " }()"
	case "trb.std.booleans.to_string":
		g.requireImport("strconv", "")
		return "strconv.FormatBool(" + arguments[0] + ")"
	case "trb.platform.go.context.background":
		g.requireImport("context", "")
		return "context.Background()"
	case "trb.platform.go.context.todo":
		g.requireImport("context", "")
		return "context.TODO()"
	case "trb.platform.go.http.router":
		g.requireImport("net/http", "http")
		return "http.NewServeMux()"
	case "trb.platform.go.http.handle":
		return arguments[0] + ".HandleFunc(" + arguments[1] + ", " + arguments[2] + ")"
	case "trb.platform.go.http.path":
		return arguments[0] + ".PathValue(" + arguments[1] + ")"
	case "trb.platform.go.http.decode":
		g.requireImport("encoding/json", "json")
		g.requireImport("net/http", "http")
		return "func() bool { if err := json.NewDecoder(" + arguments[1] + ".Body).Decode(&" + arguments[2] + "); err != nil { http.Error(" + arguments[0] + ", err.Error(), http.StatusBadRequest); return false }; return true }()"
	case "trb.platform.go.http.json":
		g.requireImport("encoding/json", "json")
		return "func() { " + arguments[0] + ".Header().Set(\"Content-Type\", \"application/json\"); " + arguments[0] + ".WriteHeader(" + arguments[1] + "); if err := json.NewEncoder(" + arguments[0] + ").Encode(" + arguments[2] + "); err != nil { panic(err) } }()"
	case "trb.platform.go.http.error":
		g.requireImport("net/http", "http")
		return "http.Error(" + strings.Join(arguments, ", ") + ")"
	case "trb.platform.go.http.cors":
		g.requireImport("net/http", "http")
		return "http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) { response.Header().Set(\"Access-Control-Allow-Origin\", " + arguments[1] + "); response.Header().Set(\"Access-Control-Allow-Headers\", \"Content-Type\"); response.Header().Set(\"Access-Control-Allow-Methods\", \"GET, POST, PATCH, OPTIONS\"); if request.Method == http.MethodOptions { response.WriteHeader(http.StatusNoContent); return }; " + arguments[0] + ".ServeHTTP(response, request) })"
	case "trb.platform.go.http.serve":
		g.requireImport("net/http", "http")
		g.requireImport("log", "")
		return "func() { if err := http.ListenAndServe(" + arguments[0] + ", " + arguments[1] + "); err != nil { log.Fatal(err) } }()"
	case "trb.platform.go.gorm.open_sqlite":
		g.requireImport("gorm.io/driver/sqlite", "sqlite")
		g.requireImport("gorm.io/gorm", "gorm")
		g.requireImport("os", "")
		return "func() *gorm.DB { path := os.Getenv(\"TRB_DATABASE\"); if path == \"\" { path = " + arguments[0] + " }; database, err := gorm.Open(sqlite.Open(path), &gorm.Config{}); if err != nil { panic(err) }; return database }()"
	case "trb.platform.go.gorm.find_all":
		return g.gormRead(call, arguments, "find_all")
	case "trb.platform.go.gorm.where":
		return g.gormRead(call, arguments, "where")
	case "trb.platform.go.gorm.raw":
		return g.gormRead(call, arguments, "raw")
	case "trb.platform.go.gorm.first":
		return g.gormRead(call, arguments, "first")
	case "trb.platform.go.gorm.create":
		return g.gormWrite(call, arguments, "Create")
	case "trb.platform.go.gorm.save":
		return g.gormWrite(call, arguments, "Save")
	case "trb.platform.go.gorm.exec":
		args := ""
		if len(arguments) > 2 {
			args = ", " + strings.Join(arguments[2:], ", ")
		}
		return "func() { if err := " + arguments[0] + ".Exec(" + arguments[1] + args + ").Error; err != nil { panic(err) } }()"
	default:
		return "nil"
	}
}

func (g *generator) portableFloatString(value string) string {
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	return "func() string { value := " + value + "; if math.IsNaN(value) { return \"NaN\" }; if math.IsInf(value, 1) { return \"Infinity\" }; if math.IsInf(value, -1) { return \"-Infinity\" }; if value == 0 { return \"0.0\" }; text := strconv.FormatFloat(value, 'f', -1, 64); if !strings.Contains(text, \".\") { text += \".0\" }; return text }()"
}

func (g *generator) jsonParse(call *ir.Call, argument string, comments bool) string {
	g.requireImport("encoding/json", "stdjson")
	g.requireImport("errors", "")
	g.requireImport("io", "")
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	result := call.ExprType()
	resultType := g.goType(result)
	valueType := g.goType(types.FromName("JsonValue"))
	errorType := g.goType(types.FromName("JsonError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	jsonAlias := g.typeAliases["JsonValue"]
	prefix := ""
	if jsonAlias != "" {
		prefix = jsonAlias + "."
	}
	ok := func(value string) string {
		return resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "](" + value + ")"
	}
	errResult := func(kind, message, path, line, column string) string {
		value := errorType + "{Kind: " + prefix + "JsonErrorKind" + kind + ", Message: " + message + ", Path: " + path + ", Line: " + line + ", Column: " + column + "}"
		return resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "](" + value + ")"
	}
	strip := ""
	if comments {
		strip = `stripComments := func(input string) string { result := []byte(input); inString := false; escaped := false; for index := 0; index < len(result); index++ { if inString { if escaped { escaped = false; continue }; if result[index] == '\\' { escaped = true } else if result[index] == '"' { inString = false }; continue }; if result[index] == '"' { inString = true; continue }; if result[index] != '/' || index+1 >= len(result) { continue }; if result[index+1] == '/' { result[index], result[index+1] = ' ', ' '; index += 2; for index < len(result) && result[index] != '\n' { if result[index] != '\r' { result[index] = ' ' }; index++ }; index-- } else if result[index+1] == '*' { result[index], result[index+1] = ' ', ' '; index += 2; for index < len(result) { if index+1 < len(result) && result[index] == '*' && result[index+1] == '/' { result[index], result[index+1] = ' ', ' '; index++; break }; if result[index] != '\n' && result[index] != '\r' { result[index] = ' ' }; index++ } } }; return string(result) }; source = stripComments(source); `
	}
	conversionError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + prefix + "JsonErrorKindDecode, Message: message, Path: path}; return &value }"
	convert := "var convert func(any, string) (" + valueType + ", *" + errorType + "); convert = func(input any, path string) (" + valueType + ", *" + errorType + ") { switch value := input.(type) { case nil: return " + prefix + "JsonValueNull, nil; case bool: return " + prefix + "NewJsonValueBoolean(value), nil; case stdjson.Number: number, parseErr := strconv.ParseFloat(string(value), 64); if parseErr != nil || math.IsInf(number, 0) || math.IsNaN(number) { return " + valueType + "{}, conversionError(path, \"JSON number is not finite\") }; if math.Trunc(number) == number { if number < -9007199254740991 || number > 9007199254740991 { return " + valueType + "{}, conversionError(path, \"JSON integer is outside the portable range\") }; return " + prefix + "NewJsonValueInteger(int(number)), nil }; return " + prefix + "NewJsonValueFloat(number), nil; case string: return " + prefix + "NewJsonValueString(value), nil; case []any: items := make([]" + valueType + ", len(value)); for index, item := range value { converted, conversionErr := convert(item, path+\"/\"+strconv.Itoa(index)); if conversionErr != nil { return " + valueType + "{}, conversionErr }; items[index] = converted }; return " + prefix + "NewJsonValueArray(items), nil; case map[string]any: fields := make(map[string]" + valueType + ", len(value)); for key, item := range value { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); converted, conversionErr := convert(item, path+\"/\"+escaped); if conversionErr != nil { return " + valueType + "{}, conversionErr }; fields[key] = converted }; return " + prefix + "NewJsonValueObject(fields), nil; default: return " + valueType + "{}, conversionError(path, \"unsupported JSON value\") } }"
	location := "func(source string, parseErr error) (*int, *int) { syntax, ok := parseErr.(*stdjson.SyntaxError); if !ok { return nil, nil }; offset := int(syntax.Offset) - 1; if offset < 0 { offset = 0 }; if offset > len(source) { offset = len(source) }; line, column := 1, 1; for _, value := range source[:offset] { if value == '\\n' { line++; column = 1 } else { column++ } }; return &line, &column }"
	return "func() " + resultType + " { source := " + argument + "; " + strip + "sourceLocation := " + location + "; decoder := stdjson.NewDecoder(strings.NewReader(source)); decoder.UseNumber(); var raw any; if err := decoder.Decode(&raw); err != nil { lineValue, columnValue := sourceLocation(source, err); return " + errResult("Syntax", "err.Error()", `""`, "lineValue", "columnValue") + " }; if err := decoder.Decode(&struct{}{}); err != io.EOF { if err == nil { err = errors.New(\"JSON source contains multiple values\") }; lineValue, columnValue := sourceLocation(source, err); return " + errResult("Syntax", "err.Error()", `""`, "lineValue", "columnValue") + " }; conversionError := " + conversionError + "; " + convert + "; value, conversionErr := convert(raw, \"\"); if conversionErr != nil { return " + resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "](*conversionErr) }; return " + ok("value") + " }()"
}

func (g *generator) jsonStringify(call *ir.Call, argument string) string {
	g.requireImport("encoding/json", "stdjson")
	g.requireImport("math", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	result := call.ExprType()
	resultType := g.goType(result)
	valueType := g.goType(types.FromName("JsonValue"))
	errorType := g.goType(types.FromName("JsonError"))
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	jsonAlias := g.typeAliases["JsonValue"]
	prefix := ""
	if jsonAlias != "" {
		prefix = jsonAlias + "."
	}
	ok := resultAlias + ".NewResultOk[string, " + errorType + "]"
	errResult := func(message, path string) string {
		value := errorType + "{Kind: " + prefix + "JsonErrorKindEncode, Message: " + message + ", Path: " + path + "}"
		return resultAlias + ".NewResultErr[string, " + errorType + "](" + value + ")"
	}
	conversionError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + prefix + "JsonErrorKindEncode, Message: message, Path: path}; return &value }"
	convert := "var convert func(" + valueType + ", string) (any, *" + errorType + "); convert = func(value " + valueType + ", path string) (any, *" + errorType + ") { switch value.Kind { case " + prefix + "JsonValueNullTag: return nil, nil; case " + prefix + "JsonValueBooleanTag: return value.BooleanValue, nil; case " + prefix + "JsonValueIntegerTag: if value.IntegerValue < -9007199254740991 || value.IntegerValue > 9007199254740991 { return nil, conversionError(path, \"JSON integer is outside the portable range\") }; return value.IntegerValue, nil; case " + prefix + "JsonValueFloatTag: if math.IsInf(value.FloatValue, 0) || math.IsNaN(value.FloatValue) { return nil, conversionError(path, \"JSON Float must be finite\") }; return value.FloatValue, nil; case " + prefix + "JsonValueStringTag: return value.StringValue, nil; case " + prefix + "JsonValueArrayTag: items := make([]any, len(value.ArrayValue)); for index, item := range value.ArrayValue { converted, conversionErr := convert(item, path+\"/\"+strconv.Itoa(index)); if conversionErr != nil { return nil, conversionErr }; items[index] = converted }; return items, nil; case " + prefix + "JsonValueObjectTag: fields := make(map[string]any, len(value.ObjectValue)); for key, item := range value.ObjectValue { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); converted, conversionErr := convert(item, path+\"/\"+escaped); if conversionErr != nil { return nil, conversionErr }; fields[key] = converted }; return fields, nil; default: return nil, conversionError(path, \"unsupported JSON value\") } }"
	return "func() " + resultType + " { conversionError := " + conversionError + "; " + convert + "; raw, conversionErr := convert(" + argument + ", \"\"); if conversionErr != nil { return " + resultAlias + ".NewResultErr[string, " + errorType + "](*conversionErr) }; encoded, err := stdjson.Marshal(raw); if err != nil { return " + errResult("err.Error()", `""`) + " }; return " + ok + "(string(encoded)) }()"
}

func (g *generator) jsonDecode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	jsonAlias := g.jsonRuntimeAlias(call)
	resultAlias := g.typeAliases["Result"]
	if resultAlias == "" {
		resultAlias = "__trb_result"
	}
	valueType := g.goCodecType(call.Codec)
	errorType := jsonAlias + ".JsonError"
	resultType := g.goType(call.ExprType())
	builder := &goJSONCodecBuilder{generator: g, jsonAlias: jsonAlias, errorType: errorType}
	decoder := builder.decoder(call.Codec)
	decodeError := "func(path, message string) *" + errorType + " { value := " + errorType + "{Kind: " + jsonAlias + ".JsonErrorKindDecode, Message: message, Path: path}; return &value }"
	parse := jsonAlias + ".Parse(" + argument + ")"
	errResult := resultAlias + ".NewResultErr[" + valueType + ", " + errorType + "]"
	okResult := resultAlias + ".NewResultOk[" + valueType + ", " + errorType + "]"
	return "func() " + resultType + " { decodeError := " + decodeError + "; " + builder.source.String() + " parsed := " + parse + "; if parsed.Kind == " + resultAlias + ".ResultErrTag { return " + errResult + "(parsed.ErrError) }; decoded, codecErr := " + decoder + "(parsed.OkValue, \"\"); if codecErr != nil { return " + errResult + "(*codecErr) }; return " + okResult + "(decoded) }()"
}

func (g *generator) jsonEncode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	jsonAlias := g.jsonRuntimeAlias(call)
	builder := &goJSONCodecBuilder{generator: g, jsonAlias: jsonAlias, errorType: jsonAlias + ".JsonError"}
	encoder := builder.encoder(call.Codec)
	return "func() " + g.goType(call.ExprType()) + " { " + builder.source.String() + " return " + jsonAlias + ".Stringify(" + encoder + "(" + argument + ")) }()"
}

func (g *generator) jsonRuntimeAlias(call *ir.Call) string {
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Alias != "" {
		return goImportAlias(reference.Alias)
	}
	if reference != nil && reference.Package != "" {
		return goImportAlias(pathpkg.Base(pathpkg.Dir(reference.Package)))
	}
	return "json"
}

func (g *generator) goCodecType(schema *ir.CodecSchema) string {
	if schema == nil {
		return "any"
	}
	base := schema.Type
	nullable := base.Nullable
	base.Nullable = false
	var result string
	switch schema.Kind {
	case "array":
		result = "[]" + g.goCodecType(schema.Element)
	case "hash":
		result = "map[string]" + g.goCodecType(schema.Element)
	case "record":
		result = goIdentifier(base.Name, true)
		if schema.Reference != nil && schema.Reference.Package != "" && schema.Reference.Package != g.modulePath {
			alias := schema.Reference.Alias
			if alias == "" {
				alias = pathpkg.Base(pathpkg.Dir(schema.Reference.Package))
			}
			result = goImportAlias(alias) + "." + result
		}
	default:
		result = g.goType(base)
	}
	if nullable {
		return "*" + result
	}
	return result
}

type goJSONCodecBuilder struct {
	generator *generator
	jsonAlias string
	errorType string
	source    strings.Builder
	next      int
}

func (b *goJSONCodecBuilder) name(prefix string) string {
	b.next++
	return "__trbJSON" + prefix + strconv.Itoa(b.next)
}

func (b *goJSONCodecBuilder) decoder(schema *ir.CodecSchema) string {
	name := b.name("Decode")
	valueType := b.generator.goCodecType(schema)
	jsonValue := b.jsonAlias + ".JsonValue"
	zero := "var zero " + valueType + "; return zero, decodeError(path, message)"
	expected := func(kind string) string {
		return "message := \"expected " + kind + "\"; " + zero
	}
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.decoder(&nonnull)
		body := "if value.Kind == " + b.jsonAlias + ".JsonValueNullTag { return nil, nil }; decoded, err := " + child + "(value, path); if err != nil { return nil, err }; return &decoded, nil"
		b.source.WriteString(name + " := func(value " + jsonValue + ", path string) (" + valueType + ", *" + b.errorType + ") { " + body + " }; ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueBooleanTag { " + expected("Boolean") + " }; return value.BooleanValue, nil"
	case "integer":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueIntegerTag { " + expected("Integer") + " }; return value.IntegerValue, nil"
	case "float":
		body = "if value.Kind == " + b.jsonAlias + ".JsonValueIntegerTag { return float64(value.IntegerValue), nil }; if value.Kind != " + b.jsonAlias + ".JsonValueFloatTag { " + expected("Float") + " }; return value.FloatValue, nil"
	case "string":
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueStringTag { " + expected("String") + " }; return value.StringValue, nil"
	case "array":
		child := b.decoder(schema.Element)
		elementType := b.generator.goCodecType(schema.Element)
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueArrayTag { " + expected("Array") + " }; decoded := make([]" + elementType + ", len(value.ArrayValue)); for index, item := range value.ArrayValue { child, err := " + child + "(item, path+\"/\"+strconv.Itoa(index)); if err != nil { var zero " + valueType + "; return zero, err }; decoded[index] = child }; return decoded, nil"
	case "hash":
		child := b.decoder(schema.Element)
		elementType := b.generator.goCodecType(schema.Element)
		body = "if value.Kind != " + b.jsonAlias + ".JsonValueObjectTag { " + expected("Object") + " }; decoded := make(map[string]" + elementType + ", len(value.ObjectValue)); for key, item := range value.ObjectValue { escaped := strings.ReplaceAll(strings.ReplaceAll(key, \"~\", \"~0\"), \"/\", \"~1\"); child, err := " + child + "(item, path+\"/\"+escaped); if err != nil { var zero " + valueType + "; return zero, err }; decoded[key] = child }; return decoded, nil"
	case "record":
		var fields strings.Builder
		fields.WriteString("if value.Kind != " + b.jsonAlias + ".JsonValueObjectTag { " + expected(schema.Type.Name) + " }; ")
		constructor := make([]string, 0, len(schema.Fields))
		for index, field := range schema.Fields {
			child := b.decoder(field.Schema)
			variable := "field" + strconv.Itoa(index)
			fieldType := b.generator.goCodecType(field.Schema)
			path := strconv.Quote("/" + jsonPointerEscape(field.WireName))
			fields.WriteString("var " + variable + " " + fieldType + "; if raw, exists := value.ObjectValue[" + strconv.Quote(field.WireName) + "]; exists { decoded, err := " + child + "(raw, path+" + path + "); if err != nil { var zero " + valueType + "; return zero, err }; " + variable + " = decoded }")
			if !field.Schema.Type.Nullable {
				fields.WriteString(" else { message := " + strconv.Quote("missing field "+field.WireName) + "; var zero " + valueType + "; return zero, decodeError(path+" + path + ", message) }")
			}
			fields.WriteString("; ")
			constructor = append(constructor, goIdentifier(field.Name, true)+": "+variable)
		}
		fields.WriteString("return " + valueType + "{" + strings.Join(constructor, ", ") + "}, nil")
		body = fields.String()
	}
	b.source.WriteString(name + " := func(value " + jsonValue + ", path string) (" + valueType + ", *" + b.errorType + ") { " + body + " }; ")
	return name
}

func (b *goJSONCodecBuilder) encoder(schema *ir.CodecSchema) string {
	name := b.name("Encode")
	valueType := b.generator.goCodecType(schema)
	jsonValue := b.jsonAlias + ".JsonValue"
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.encoder(&nonnull)
		b.source.WriteString(name + " := func(value " + valueType + ") " + jsonValue + " { if value == nil { return " + b.jsonAlias + ".JsonValueNull }; return " + child + "(*value) }; ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "return " + b.jsonAlias + ".NewJsonValueBoolean(value)"
	case "integer":
		body = "return " + b.jsonAlias + ".NewJsonValueInteger(value)"
	case "float":
		body = "return " + b.jsonAlias + ".NewJsonValueFloat(value)"
	case "string":
		body = "return " + b.jsonAlias + ".NewJsonValueString(value)"
	case "array":
		child := b.encoder(schema.Element)
		body = "items := make([]" + jsonValue + ", len(value)); for index, item := range value { items[index] = " + child + "(item) }; return " + b.jsonAlias + ".NewJsonValueArray(items)"
	case "hash":
		child := b.encoder(schema.Element)
		body = "fields := make(map[string]" + jsonValue + ", len(value)); for key, item := range value { fields[key] = " + child + "(item) }; return " + b.jsonAlias + ".NewJsonValueObject(fields)"
	case "record":
		parts := make([]string, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			child := b.encoder(field.Schema)
			parts = append(parts, strconv.Quote(field.WireName)+": "+child+"(value."+goIdentifier(field.Name, true)+")")
		}
		body = "return " + b.jsonAlias + ".NewJsonValueObject(map[string]" + jsonValue + "{" + strings.Join(parts, ", ") + "})"
	}
	b.source.WriteString(name + " := func(value " + valueType + ") " + jsonValue + " { " + body + " }; ")
	return name
}

func jsonPointerEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func (g *generator) gormRead(call *ir.Call, arguments []string, operation string) string {
	resultType := g.goType(call.ExprType())
	valueType := resultType
	if call.ExprType().Kind == types.Array && len(call.ExprType().Args) > 0 {
		valueType = g.goType(call.ExprType().Args[0])
	}
	args := ""
	if len(arguments) > 3 {
		args = ", " + strings.Join(arguments[3:], ", ")
	}
	switch operation {
	case "find_all":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Find(&result).Error; err != nil { panic(err) }; return result }()"
	case "where":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Where(" + arguments[2] + args + ").Find(&result).Error; err != nil { panic(err) }; return result }()"
	case "raw":
		return "func() " + resultType + " { var result " + resultType + "; if err := " + arguments[0] + ".Raw(" + arguments[2] + args + ").Scan(&result).Error; err != nil { panic(err) }; return result }()"
	case "first":
		return "func() " + valueType + " { var result " + valueType + "; if err := " + arguments[0] + ".Where(" + arguments[2] + args + ").First(&result).Error; err != nil { panic(err) }; return result }()"
	default:
		return "nil"
	}
}

func (g *generator) gormWrite(call *ir.Call, arguments []string, operation string) string {
	resultType := g.goType(call.ExprType())
	return "func() " + resultType + " { value := " + arguments[1] + "; if err := " + arguments[0] + "." + operation + "(&value).Error; err != nil { panic(err) }; return value }()"
}

func expressionReference(expression ir.Expression) *ir.Reference {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Reference
	case *ir.Member:
		return node.Reference
	case *ir.TypeApply:
		return expressionReference(node.Receiver)
	default:
		return nil
	}
}

func (g *generator) referenceAlias(reference *ir.Reference) string {
	if reference == nil || reference.Intrinsic != "" || reference.Package == "" {
		return ""
	}
	directory := pathpkg.Dir(reference.Package)
	if directory == "." {
		directory = ""
	}
	if directory == g.currentDirectory() {
		return ""
	}
	alias := reference.Alias
	if alias == "" {
		alias = pathpkg.Base(directory)
	}
	return goImportAlias(alias)
}

func goImportAlias(name string) string {
	if strings.HasPrefix(name, "__trb_") {
		return name
	}
	return goIdentifier(name, false)
}

func goImportedName(name, kind string) string {
	if kind == "function" {
		return goMethodName(name)
	}
	if kind == "value" && isUpper(name) {
		return goConstantIdentifier("", name)
	}
	return goIdentifier(name, true)
}

func (g *generator) goType(t types.Type) string {
	var result string
	switch t.Kind {
	case types.Void:
		result = ""
	case types.Any, types.Invalid:
		result = "any"
	case types.Bool:
		result = "bool"
	case types.Int:
		result = "int"
	case types.Float:
		result = "float64"
	case types.String:
		result = "string"
	case types.Bytes:
		result = "[]byte"
	case types.StringBuilder:
		g.requireImport("strings", "")
		result = "*strings.Builder"
	case types.Array, types.Iterable:
		element := "any"
		if len(t.Args) > 0 {
			element = g.goType(t.Args[0])
		}
		result = "[]" + element
	case types.Range:
		element := "int"
		if len(t.Args) > 0 {
			element = g.goType(t.Args[0])
		}
		result = "[]" + element
	case types.Hash:
		key := "any"
		value := "any"
		if len(t.Args) == 2 {
			key = g.goType(t.Args[0])
			value = g.goType(t.Args[1])
		}
		result = "map[" + key + "]" + value
	default:
		if t.Name == "" {
			result = "any"
		} else if t.Name == "GormDB" {
			g.requireImport("gorm.io/gorm", "gorm")
			result = "*gorm.DB"
		} else if t.Name == "HTTPRouter" {
			g.requireImport("net/http", "http")
			result = "*http.ServeMux"
		} else if t.Name == "HTTPRequest" {
			g.requireImport("net/http", "http")
			result = "*http.Request"
		} else if t.Name == "HTTPResponse" {
			g.requireImport("net/http", "http")
			result = "http.ResponseWriter"
		} else if t.Name == "HTTPHandler" {
			g.requireImport("net/http", "http")
			result = "http.Handler"
		} else if alias := g.typeAliases[t.Name]; alias != "" {
			result = alias + "." + goIdentifier(t.Name, true)
			if g.typeKinds[t.Name] == "class" {
				result = "*" + result
			}
		} else {
			result = goIdentifier(t.Name, true)
			if g.classes[t.Name] {
				result = "*" + result
			}
		}
	}
	if t.Kind == types.Named && len(t.Args) > 0 {
		arguments := make([]string, len(t.Args))
		for index, argument := range t.Args {
			arguments[index] = g.goType(argument)
		}
		result += "[" + strings.Join(arguments, ", ") + "]"
	}
	if t.Nullable && result != "" && result != "any" && !strings.HasPrefix(result, "*") {
		return "*" + result
	}
	return result
}

func (g *generator) goReturn(t types.Type) string {
	if t.Kind == types.Void {
		return ""
	}
	return " " + g.goType(t)
}

func goFieldName(name string) string {
	name = strings.TrimPrefix(name, "@")
	name = strings.TrimPrefix(name, "_")
	return "trb" + goIdentifier(name, true)
}

func goMethodName(name string) string {
	private := strings.HasPrefix(name, "_")
	if kind, encoded, ok := naming.CallableSuffix(name); ok {
		prefix := "Trb" + upperFirst(kind) + "_"
		if private {
			prefix = "trb" + upperFirst(kind) + "_"
		}
		return prefix + encoded
	}
	name = strings.TrimPrefix(name, "_")
	return goIdentifier(name, !private && name != "main")
}

func goIdentifier(name string, exported bool) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '_' || r == '-' || r == '/' || r == '.' })
	if len(parts) == 0 {
		return "value"
	}
	for i := range parts {
		if i > 0 || exported {
			parts[i] = upperFirst(parts[i])
		} else {
			parts[i] = lowerFirst(parts[i])
		}
	}
	return strings.Join(parts, "")
}

func goConstantIdentifier(owner, name string) string {
	var result strings.Builder
	if owner != "" {
		for _, part := range strings.Split(owner, "::") {
			result.WriteString(goIdentifier(part, true))
		}
	}
	result.WriteString(goIdentifier(strings.ToLower(name), true))
	return result.String()
}

func goTrailingComment(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return " //" + strings.TrimPrefix(value, "#")
}

func irExpressionName(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Name
	case *ir.Member:
		prefix := irExpressionName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	default:
		return ""
	}
}

func upperFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToUpper(r[0])
	}
	return string(r)
}

func lowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}

func findInitialize(methods []*ir.Method) *ir.Method {
	for _, method := range methods {
		if method.Name == "initialize" {
			return method
		}
	}
	return nil
}

func usesInterpolation(statements []ir.Statement) bool {
	for _, statement := range statements {
		switch n := statement.(type) {
		case *ir.Class:
			if expressionUsesInterpolation(n.Superclass) || usesInterpolation(n.Body) {
				return true
			}
		case *ir.Module:
			if usesInterpolation(n.Body) {
				return true
			}
		case *ir.Interface:
			for _, method := range n.Methods {
				if usesInterpolation(method.Body) {
					return true
				}
			}
		case *ir.Field:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Method:
			for _, parameter := range n.Parameters {
				if expressionUsesInterpolation(parameter.Default) {
					return true
				}
			}
			if usesInterpolation(n.Body) {
				return true
			}
		case *ir.Variable:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Assignment:
			if expressionUsesInterpolation(n.Target) || expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.Return:
			if expressionUsesInterpolation(n.Value) {
				return true
			}
		case *ir.ExpressionStatement:
			if expressionUsesInterpolation(n.Expression) {
				return true
			}
		case *ir.If:
			if expressionUsesInterpolation(n.Condition) || usesInterpolation(n.Then) || usesInterpolation(n.Else) {
				return true
			}
			for _, branch := range n.ElseIf {
				if expressionUsesInterpolation(branch.Condition) || usesInterpolation(branch.Body) {
					return true
				}
			}
		case *ir.Case:
			if expressionUsesInterpolation(n.Value) || usesInterpolation(n.Leading) || usesInterpolation(n.Else) {
				return true
			}
			for _, branch := range n.Branches {
				if expressionUsesInterpolation(branch.Value) || usesInterpolation(branch.Body) {
					return true
				}
			}
		case *ir.While:
			if expressionUsesInterpolation(n.Condition) || usesInterpolation(n.Body) {
				return true
			}
		case *ir.Iterate:
			if expressionUsesInterpolation(n.Source) || expressionUsesInterpolation(n.SliceSize) || usesInterpolation(n.Body) {
				return true
			}
		}
	}
	return false
}

func expressionUsesInterpolation(expression ir.Expression) bool {
	switch n := expression.(type) {
	case nil:
		return false
	case *ir.InterpolatedString:
		return true
	case *ir.Array:
		for _, element := range n.Elements {
			if expressionUsesInterpolation(element) {
				return true
			}
		}
	case *ir.Hash:
		for _, entry := range n.Entries {
			if expressionUsesInterpolation(entry.Key) || expressionUsesInterpolation(entry.Value) {
				return true
			}
		}
	case *ir.Unary:
		return expressionUsesInterpolation(n.Operand)
	case *ir.Conversion:
		return expressionUsesInterpolation(n.Value)
	case *ir.Binary:
		return expressionUsesInterpolation(n.Left) || expressionUsesInterpolation(n.Right)
	case *ir.Range:
		return expressionUsesInterpolation(n.Start) || expressionUsesInterpolation(n.End)
	case *ir.Call:
		if expressionUsesInterpolation(n.Callee) {
			return true
		}
		for _, argument := range n.Arguments {
			if expressionUsesInterpolation(argument.Value) {
				return true
			}
		}
	case *ir.Member:
		return expressionUsesInterpolation(n.Receiver)
	case *ir.Index:
		return expressionUsesInterpolation(n.Receiver) || expressionUsesInterpolation(n.Index)
	case *ir.Block:
		return usesInterpolation(n.Body)
	}
	return false
}

func isUpper(s string) bool { return len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' }

func (g *generator) line(text string) {
	g.b.WriteString(strings.Repeat("\t", g.indent))
	g.b.WriteString(text)
	g.b.WriteByte('\n')
}
