package golang

import (
	"go/format"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
	"unicode"

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
			output.WriteString("import " + goIdentifier(alias, false) + " " + strconv.Quote(importPath) + "\n")
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
	if imported.Standard {
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
		g.typeAliases[symbol] = goIdentifier(alias, false)
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
		target := g.expr(n.Target)
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
	item := goIdentifier(iteration.Item, false)
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
		if iteration.Item != "_" {
			g.line(end + " := min(" + offset + "+" + size + ", len(" + items + "))")
			g.line(item + " := " + items + "[" + offset + ":" + end + "]")
			g.line("_ = " + item)
		}
		if iteration.WithIndex {
			if iteration.Index != "_" {
				index := goIdentifier(iteration.Index, false)
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
	if iteration.WithIndex && iteration.Index != "_" && iteration.Item != "_" {
		g.line("for " + goIdentifier(iteration.Index, false) + ", " + item + " := range " + g.expr(iteration.Source) + " {")
	} else if iteration.WithIndex && iteration.Index != "_" {
		g.line("for " + goIdentifier(iteration.Index, false) + " := range " + g.expr(iteration.Source) + " {")
	} else if iteration.Item != "_" {
		g.line("for _, " + item + " := range " + g.expr(iteration.Source) + " {")
	} else {
		g.line("for range " + g.expr(iteration.Source) + " {")
	}
	g.indent++
	if iteration.Item != "_" {
		g.line("_ = " + item)
	}
	if iteration.WithIndex && iteration.Index != "_" {
		g.line("_ = " + goIdentifier(iteration.Index, false))
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
	g.line("func " + name + "(" + g.parameters(method.Parameters) + ")" + g.goReturn(method.ReturnType) + " {")
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
		}
		if n.Owner != "" {
			return goConstantIdentifier(n.Owner, n.Name)
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
		return "map[any]any{" + strings.Join(parts, ", ") + "}"
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
	case *ir.Member:
		if n.Namespace && isUpper(n.Name) {
			name := goConstantIdentifier(irExpressionName(n.Receiver), n.Name)
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
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		if member, ok := n.Callee.(*ir.Member); ok && member.Name == "push" && member.Receiver.ExprType().Kind == types.Array && len(parts) == 1 {
			receiver := g.expr(member.Receiver)
			return receiver + " = append(" + receiver + ", " + parts[0] + ")"
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
	switch name {
	case "trb.std.io.puts":
		g.requireImport("fmt", "")
		return "fmt.Println(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.strings.length":
		g.requireImport("unicode/utf8", "utf8")
		return "utf8.RuneCountInString(" + arguments[0] + ")"
	case "trb.std.strings.uppercase":
		g.requireImport("strings", "")
		return "strings.ToUpper(" + arguments[0] + ")"
	case "trb.std.strings.lowercase":
		g.requireImport("strings", "")
		return "strings.ToLower(" + arguments[0] + ")"
	case "trb.std.strings.contains":
		g.requireImport("strings", "")
		return "strings.Contains(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.length":
		return "len(" + arguments[0] + ")"
	case "trb.std.arrays.push":
		return arguments[0] + " = append(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		g.requireImport("strconv", "")
		return "strconv.Itoa(" + arguments[0] + ")"
	case "trb.std.numbers.parse_integer":
		g.requireImport("strconv", "")
		return "func() int { value, err := strconv.Atoi(" + arguments[0] + "); if err != nil { panic(err) }; return value }()"
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
	return goIdentifier(alias, false)
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
		result = "map[any]any"
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
