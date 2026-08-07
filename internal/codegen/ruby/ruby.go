package ruby

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type generator struct {
	b            strings.Builder
	indent       int
	loader       string
	modulePath   string
	topFunctions map[string]bool
	nativeSyntax bool
	temporary    int
}

func Generate(program *ir.Program) string {
	g := &generator{loader: program.RubyLoader, modulePath: program.ModulePath, topFunctions: map[string]bool{}}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok {
			g.topFunctions[method.Name] = true
		}
		if imported, ok := statement.(*ir.Import); ok && (imported.Path == "trb/platform/ruby/native" || imported.Path == "trb/platform/ruby/rails") {
			g.nativeSyntax = true
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
	case *ir.Class, *ir.Record, *ir.Enum, *ir.Module, *ir.Interface, *ir.Method:
		return true
	}
	return false
}

func (g *generator) statement(statement ir.Statement) {
	switch n := statement.(type) {
	case *ir.Comment:
		g.line(n.Text, "")
	case *ir.Import:
		if (n.Standard || g.loader == "zeitwerk") && !n.Runtime {
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
	case *ir.Enum:
		if enumHasPayload(n) {
			g.payloadEnum(n)
			break
		}
		g.line(n.Name+" = Data.define(:name)", n.TrailingComment)
		for _, statement := range n.Body {
			switch member := statement.(type) {
			case *ir.Comment:
				g.statement(member)
			case *ir.EnumMember:
				g.line(n.Name+"::"+member.Name+" = "+n.Name+".new(:"+member.Name+")", member.TrailingComment)
			}
		}
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
		target := g.assignmentTarget(n.Target)
		if n.Operator == "/=" && n.Target.ExprType().Kind == types.Int {
			g.line(target+" = ("+target+").quo("+g.expr(n.Value)+").truncate", n.TrailingComment)
		} else {
			g.line(target+" "+n.Operator+" "+g.expr(n.Value), n.TrailingComment)
		}
	case *ir.Return:
		text := "return"
		if n.Value != nil {
			text += " " + g.expr(n.Value)
		}
		g.line(text, n.TrailingComment)
	case *ir.Break:
		g.line("break", n.TrailingComment)
	case *ir.Next:
		g.line("next", n.TrailingComment)
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
	case *ir.Case:
		g.statements(n.Leading)
		caseValue := g.expr(n.Value)
		payload := caseHasPayload(n)
		value := ""
		if payload {
			g.temporary++
			value = "__trb_case" + strconv.Itoa(g.temporary)
			caseValue = "(" + value + " = " + caseValue + ")"
		}
		g.line("case "+caseValue, n.TrailingComment)
		for _, branch := range n.Branches {
			g.line("when "+g.expr(branch.Value), branch.TrailingComment)
			g.indent++
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				g.line(binding.Name+" = "+value+"."+binding.Field, "")
			}
			g.statements(branch.Body)
			g.indent--
		}
		if n.HasElse {
			g.line("else", "")
			g.indent++
			g.statements(n.Else)
			g.indent--
		} else {
			g.line("else", "")
			g.indent++
			g.line(`raise "unreachable exhaustive case"`, "")
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

func enumHasPayload(enum *ir.Enum) bool {
	for _, statement := range enum.Body {
		if member, ok := statement.(*ir.EnumMember); ok && len(member.Fields) > 0 {
			return true
		}
	}
	return false
}

func caseHasPayload(value *ir.Case) bool {
	for _, branch := range value.Branches {
		if branch.PayloadEnum {
			return true
		}
	}
	return false
}

func (g *generator) payloadEnum(enum *ir.Enum) {
	g.line("module "+enum.Name, enum.TrailingComment)
	g.indent++
	for _, statement := range enum.Body {
		switch member := statement.(type) {
		case *ir.Comment:
			g.statement(member)
		case *ir.EnumMember:
			fields := make([]string, len(member.Fields))
			for index, field := range member.Fields {
				fields[index] = ":" + field.Name
			}
			definition := "Data.define(" + strings.Join(fields, ", ") + ")"
			if len(fields) == 0 {
				definition += ".new"
			}
			g.line(member.Name+" = "+definition, member.TrailingComment)
		}
	}
	g.indent--
	g.line("end", "")
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
		if !g.nativeSyntax {
			return strconv.Quote(n.Name)
		}
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
		op := n.Operator
		if op == "not" || op == "!" {
			return "!(" + g.expr(n.Operand) + ")"
		}
		return op + g.unaryOperand(n.Operand)
	case *ir.Conversion:
		switch n.Kind {
		case ir.IntegerToFloatConversion:
			return "(" + g.expr(n.Value) + ").to_f"
		default:
			return g.expr(n.Value)
		}
	case *ir.Binary:
		left := g.binaryOperand(n.Left)
		right := g.binaryOperand(n.Right)
		op := n.Operator
		if op == "and" {
			op = "&&"
		} else if op == "or" {
			op = "||"
		}
		if op == "**" && n.ExprType().Kind == types.Int {
			return "->(base, exponent) { raise RangeError, \"negative Integer exponent\" if exponent < 0; base ** exponent }.call(" + left + ", " + right + ")"
		}
		if op == "/" && n.ExprType().Kind == types.Int {
			return "(" + left + ").quo(" + right + ").truncate"
		}
		if op == "%" && n.ExprType().Kind == types.Int {
			return "(" + left + ").remainder(" + right + ")"
		}
		return left + " " + op + " " + right
	case *ir.Range:
		operator := ".."
		if n.Exclusive {
			operator = "..."
		}
		return g.expr(n.Start) + operator + g.expr(n.End)
	case *ir.Transform:
		return g.transform(n)
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
			if reference.ReceiverMethod {
				if member, ok := n.Callee.(*ir.Member); ok {
					parts = append([]string{g.expr(member.Receiver)}, parts...)
				}
			}
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		return g.expr(n.Callee) + "(" + strings.Join(parts, ", ") + ")"
	case *ir.EnumConstruct:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument)
		}
		return n.EnumName + "::" + n.Member + ".new(" + strings.Join(parts, ", ") + ")"
	case *ir.TypeApply:
		return g.expr(n.Receiver)
	case *ir.Index:
		if n.Receiver.ExprType().Kind == types.Hash && len(n.Receiver.ExprType().Args) == 2 {
			return g.expr(n.Receiver) + ".fetch(" + g.expr(n.Index) + ")"
		}
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	case *ir.NativeExpression:
		return n.Text
	default:
		return ""
	}
}

func (g *generator) transform(transform *ir.Transform) string {
	source := g.expr(transform.Source)
	if _, rangeSource := transform.Source.(*ir.Range); rangeSource {
		source = "(" + source + ")"
	}
	result := g.expr(transform.Result)
	switch transform.Operation {
	case "map", "select":
		operation := transform.Operation
		parameters := []string{transform.Item}
		if transform.WithIndex {
			operation += ".with_index"
			parameters = append(parameters, transform.Index)
		}
		return source + "." + operation + " { |" + strings.Join(parameters, ", ") + "| " + result + " }"
	case "reduce":
		return source + ".reduce(" + g.expr(transform.Initial) + ") { |" + transform.Accumulator + ", " + transform.Item + "| " + result + " }"
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

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	unicodeCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return "Unicode." + symbol
	}
	pathCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return "Path." + symbol
	}
	filesystemOK := func(value string) string {
		return "Result::Ok.new(" + value + ")"
	}
	filesystemError := func(operation, path, message string) string {
		value := "FileError.new(operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + ")"
		return "Result::Err.new(" + value + ")"
	}
	processError := func(operation, command, message string) string {
		value := "ProcessError.new(operation: " + strconv.Quote(operation) + ", command: " + command + ", message: " + message + ")"
		return "Result::Err.new(" + value + ")"
	}
	switch name {
	case "trb.std.io.puts":
		if len(call.Arguments) == 1 && call.Arguments[0].Value.ExprType().Kind == types.Float {
			return "$stdout.puts(" + portableFloatString(arguments[0]) + ")"
		}
		return "$stdout.puts(" + strings.Join(arguments, ", ") + ")"
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
		return "->(path) { begin; File.stat(path); " + filesystemOK("true") + "; rescue Errno::ENOENT; " + filesystemOK("false") + "; rescue StandardError => error; " + filesystemError("exists", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.read_text":
		value := "File.binread(path).force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace)"
		return "->(path) { begin; " + filesystemOK(value) + "; rescue StandardError => error; " + filesystemError("read_text", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.read_bytes":
		return "->(path) { begin; " + filesystemOK("File.binread(path).b") + "; rescue StandardError => error; " + filesystemError("read_bytes", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.write_text":
		return "->(path, value) { begin; File.binwrite(path, value.encode(Encoding::UTF_8)); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("write_text", "path", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.filesystem.write_bytes":
		return "->(path, value) { begin; File.binwrite(path, value); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("write_bytes", "path", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.filesystem.create_directory":
		return "->(path) { begin; require \"fileutils\"; FileUtils.mkdir_p(path); " + filesystemOK("Unit.new") + "; rescue StandardError => error; " + filesystemError("create_directory", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.filesystem.list":
		return "->(path) { begin; " + filesystemOK("Dir.children(path).sort") + "; rescue StandardError => error; " + filesystemError("list", "path", "error.message") + "; end }.call(" + arguments[0] + ")"
	case "trb.internal.process.arguments":
		return "ARGV.dup"
	case "trb.internal.process.environment":
		return "ENV[" + arguments[0] + "]"
	case "trb.internal.process.working_directory":
		return "-> { begin; " + filesystemOK("Dir.pwd") + "; rescue StandardError => error; " + processError("working_directory", strconv.Quote(""), "error.message") + "; end }.call"
	case "trb.internal.process.run":
		text := "->(value) { value.force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace) }"
		value := "ProcessResult.new(status: status.exitstatus || -1, stdout: text.call(stdout), stderr: text.call(stderr), success: status.success?)"
		return "->(command, arguments) { begin; require \"open3\"; stdout, stderr, status = Open3.capture3(command, *arguments); text = " + text + "; " + filesystemOK(value) + "; rescue StandardError => error; " + processError("run", "command", "error.message") + "; end }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.internal.json.parse":
		return rubyJSONParse(arguments[0], false)
	case "trb.internal.json.parse_jsonc":
		return rubyJSONParse(arguments[0], true)
	case "trb.internal.json.stringify":
		return rubyJSONStringify(arguments[0])
	case "trb.internal.json.decode":
		return rubyJSONDecode(call, arguments[0])
	case "trb.internal.json.encode":
		return rubyJSONEncode(call, arguments[0])
	case "trb.std.strings.length":
		return arguments[0] + ".each_codepoint.count"
	case "trb.std.strings.empty":
		return arguments[0] + ".empty?"
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		whitespace := `[\u{0009}-\u{000D}\u{0020}\u{0085}\u{00A0}\u{1680}\u{2000}-\u{200A}\u{2028}-\u{2029}\u{202F}\u{205F}\u{3000}]`
		value := "(" + arguments[0] + ")"
		if name != "trb.std.strings.rstrip" {
			value += `.sub(/\A` + whitespace + `+/u, "")`
		}
		if name != "trb.std.strings.lstrip" {
			value += `.sub(/` + whitespace + `+\z/u, "")`
		}
		return value
	case "trb.std.strings.uppercase":
		return arguments[0] + ".upcase"
	case "trb.std.strings.lowercase":
		return arguments[0] + ".downcase"
	case "trb.std.strings.starts_with":
		return arguments[0] + ".start_with?(" + arguments[1] + ")"
	case "trb.std.strings.ends_with":
		return arguments[0] + ".end_with?(" + arguments[1] + ")"
	case "trb.std.strings.split":
		return "->(value, separator) { raise ArgumentError, \"String split separator is empty\" if separator.empty?; value.split(separator, -1) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.contains":
		return arguments[0] + ".include?(" + arguments[1] + ")"
	case "trb.std.strings.codepoints":
		return arguments[0] + ".codepoints"
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
		return "(" + arguments[0] + ").encode(Encoding::UTF_8).b"
	case "trb.std.bytes.to_string":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8).encode(Encoding::UTF_8, invalid: :replace, undef: :replace)"
	case "trb.std.bytes.length":
		return arguments[0] + ".bytesize"
	case "trb.std.bytes.at":
		return arguments[0] + ".bytes.fetch(" + arguments[1] + ")"
	case "trb.std.bytes.concat":
		return arguments[0] + " + " + arguments[1]
	case "trb.std.bytes.valid_utf8":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8).valid_encoding?"
	case "trb.std.string_builder.new":
		return "String.new(encoding: Encoding::UTF_8)"
	case "trb.std.string_builder.from_string":
		return "(" + arguments[0] + ").dup.force_encoding(Encoding::UTF_8)"
	case "trb.std.string_builder.append":
		return arguments[0] + " << " + arguments[1]
	case "trb.std.string_builder.append_codepoint":
		return arguments[0] + " << (" + arguments[1] + ").chr(Encoding::UTF_8)"
	case "trb.std.string_builder.length":
		return arguments[0] + ".each_codepoint.count"
	case "trb.std.string_builder.empty":
		return arguments[0] + ".empty?"
	case "trb.std.string_builder.to_string":
		return arguments[0] + ".dup"
	case "trb.std.string_builder.clear":
		return arguments[0] + ".clear"
	case "trb.std.arrays.length":
		return arguments[0] + ".length"
	case "trb.std.arrays.empty":
		return arguments[0] + ".empty?"
	case "trb.std.arrays.fetch":
		return "->(values, index) { raise IndexError, \"Array index is out of bounds\" if index < 0 || index >= values.length; values.fetch(index) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.try_fetch":
		return "->(values, index) { index < 0 || index >= values.length ? Result::Err.new(\"Array index is out of bounds\") : Result::Ok.new(values.fetch(index)) }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.first":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.fetch(0) }.call(" + arguments[0] + ")"
	case "trb.std.arrays.last":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.fetch(values.length - 1) }.call(" + arguments[0] + ")"
	case "trb.std.arrays.copy":
		return arguments[0] + ".dup"
	case "trb.std.arrays.contains":
		return arguments[0] + ".include?(" + arguments[1] + ")"
	case "trb.std.arrays.count":
		return arguments[0] + ".count(" + arguments[1] + ")"
	case "trb.std.arrays.join":
		return arguments[0] + ".join(" + arguments[1] + ")"
	case "trb.std.arrays.pop":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.pop }.call(" + arguments[0] + ")"
	case "trb.std.arrays.shift":
		return "->(values) { raise IndexError, \"Array is empty\" if values.empty?; values.shift }.call(" + arguments[0] + ")"
	case "trb.std.arrays.push":
		return arguments[0] + " << " + arguments[1]
	case "trb.std.arrays.unshift":
		return arguments[0] + ".unshift(" + arguments[1] + ")"
	case "trb.std.arrays.reverse":
		return arguments[0] + ".reverse"
	case "trb.std.hashes.length":
		return arguments[0] + ".length"
	case "trb.std.hashes.empty":
		return arguments[0] + ".empty?"
	case "trb.std.hashes.fetch":
		return arguments[0] + ".fetch(" + arguments[1] + ")"
	case "trb.std.hashes.try_fetch":
		return "->(values, key) { values.key?(key) ? Result::Ok.new(values[key]) : Result::Err.new(\"Hash key is missing\") }.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hashes.contains_key":
		return arguments[0] + ".key?(" + arguments[1] + ")"
	case "trb.std.hashes.keys":
		return arguments[0] + ".keys"
	case "trb.std.hashes.values":
		return arguments[0] + ".values"
	case "trb.std.hashes.copy":
		return arguments[0] + ".dup"
	case "trb.std.numbers.to_string":
		return arguments[0] + ".to_s"
	case "trb.std.numbers.integer_to_float":
		return "(" + arguments[0] + ").to_f"
	case "trb.std.numbers.integer_absolute":
		return "(" + arguments[0] + ").abs"
	case "trb.std.numbers.integer_zero":
		return "(" + arguments[0] + ").zero?"
	case "trb.std.numbers.integer_positive":
		return "(" + arguments[0] + ").positive?"
	case "trb.std.numbers.integer_negative":
		return "(" + arguments[0] + ").negative?"
	case "trb.std.numbers.integer_even":
		return "(" + arguments[0] + ").even?"
	case "trb.std.numbers.integer_odd":
		return "(" + arguments[0] + ").odd?"
	case "trb.std.numbers.float_to_string":
		return portableFloatString(arguments[0])
	case "trb.std.numbers.float_to_integer":
		return "->(value) { raise FloatDomainError, \"Float cannot be converted to Integer\" unless value.finite?; integer = value.truncate; raise RangeError, \"Integer is outside the portable range\" if integer < -9007199254740991 || integer > 9007199254740991; integer }.call(" + arguments[0] + ")"
	case "trb.std.numbers.float_absolute":
		return "(" + arguments[0] + ").abs"
	case "trb.std.numbers.float_finite":
		return "(" + arguments[0] + ").finite?"
	case "trb.std.numbers.float_infinite":
		return "!((" + arguments[0] + ").infinite?).nil?"
	case "trb.std.numbers.float_nan":
		return "(" + arguments[0] + ").nan?"
	case "trb.std.numbers.parse_integer":
		return "->(input) { raise ArgumentError, \"invalid Integer\" unless /\\A[+-]?[0-9]+\\z/.match?(input); value = Integer(input, 10); raise RangeError, \"Integer is outside the portable range\" if value < -9007199254740991 || value > 9007199254740991; value }.call(" + arguments[0] + ")"
	case "trb.std.numbers.try_parse_integer":
		return "->(input) { if !/\\A[+-]?[0-9]+\\z/.match?(input); Result::Err.new(\"invalid Integer\"); else; value = Integer(input, 10); if value < -9007199254740991 || value > 9007199254740991; Result::Err.new(\"Integer is outside the portable range\"); else; Result::Ok.new(value); end; end }.call(" + arguments[0] + ")"
	case "trb.std.booleans.to_string":
		return "(" + arguments[0] + ").to_s"
	default:
		return "nil"
	}
}

func portableFloatString(value string) string {
	return "->(value) { if value.nan?; \"NaN\"; elsif value.infinite? == 1; \"Infinity\"; elsif value.infinite? == -1; \"-Infinity\"; elsif value.zero?; \"0.0\"; else; raw = value.to_s; if raw.downcase.include?(\"e\"); mantissa, exponent_text = raw.downcase.split(\"e\", 2); negative = mantissa.start_with?(\"-\"); unsigned = negative ? mantissa[1..] : mantissa; whole, fraction = unsigned.split(\".\", 2); digits = whole + (fraction || \"\"); decimal = whole.length + exponent_text.to_i; if decimal <= 0; text = \"0.\" + (\"0\" * -decimal) + digits; elsif decimal >= digits.length; text = digits + (\"0\" * (decimal - digits.length)) + \".0\"; else; text = digits[0, decimal] + \".\" + digits[decimal..]; end; text = text.sub(/(\\.\\d*?)0+\\z/, '\\1').sub(/\\.\\z/, \".0\"); negative ? \"-\" + text : text; else; raw.include?(\".\") ? raw : raw + \".0\"; end; end }.call(" + value + ")"
}

func rubyJSONParse(argument string, comments bool) string {
	strip := ""
	if comments {
		strip = `strip_comments = ->(input) { result = input.dup; in_string = false; escaped = false; index = 0; while index < result.bytesize; byte = result.getbyte(index); if in_string; if escaped; escaped = false; elsif byte == 92; escaped = true; elsif byte == 34; in_string = false; end; index += 1; next; end; if byte == 34; in_string = true; index += 1; next; end; if byte == 47 && index + 1 < result.bytesize; following = result.getbyte(index + 1); if following == 47; result.setbyte(index, 32); result.setbyte(index + 1, 32); index += 2; while index < result.bytesize && result.getbyte(index) != 10; result.setbyte(index, 32) unless result.getbyte(index) == 13; index += 1; end; next; elsif following == 42; result.setbyte(index, 32); result.setbyte(index + 1, 32); index += 2; while index < result.bytesize; if index + 1 < result.bytesize && result.getbyte(index) == 42 && result.getbyte(index + 1) == 47; result.setbyte(index, 32); result.setbyte(index + 1, 32); index += 2; break; end; byte = result.getbyte(index); result.setbyte(index, 32) unless byte == 10 || byte == 13; index += 1; end; next; end; end; index += 1; end; result }; source = strip_comments.call(source); `
	}
	errorValue := func(kind, message, path, line, column string) string {
		return "JsonError.new(kind: JsonErrorKind::" + kind + ", message: " + message + ", path: " + path + ", line: " + line + ", column: " + column + ")"
	}
	conversionError := errorValue("Decode", "message", "path", "nil", "nil")
	convert := "convert = nil; convert = ->(value, path) { if value.nil?; JsonValue::Null; elsif value == true || value == false; JsonValue::Boolean.new(value); elsif value.is_a?(Integer) || value.is_a?(Float); number = value.to_f; unless number.finite?; message = \"JSON number is not finite\"; throw :__trb_json_error, [:error, " + conversionError + "]; end; if number == number.to_i; if number < -9007199254740991 || number > 9007199254740991; message = \"JSON integer is outside the portable range\"; throw :__trb_json_error, [:error, " + conversionError + "]; end; JsonValue::Integer.new(number.to_i); else; JsonValue::Float.new(number); end; elsif value.is_a?(String); JsonValue::String.new(value); elsif value.is_a?(Array); JsonValue::Array.new(value.each_with_index.map { |item, index| convert.call(item, path + \"/\" + index.to_s) }); elsif value.is_a?(Hash); fields = {}; value.each { |key, item| escaped = key.gsub(\"~\", \"~0\").gsub(\"/\", \"~1\"); fields[key] = convert.call(item, path + \"/\" + escaped) }; JsonValue::Object.new(fields); else; message = \"unsupported JSON value\"; throw :__trb_json_error, [:error, " + conversionError + "]; end }"
	syntaxError := errorValue("Syntax", "error.message", `""`, "line", "column")
	genericError := errorValue("Syntax", "error.message", `""`, "nil", "nil")
	return "->(source) { begin; require \"json\"; " + strip + convert + "; raw = JSON.parse(source); outcome = catch(:__trb_json_error) { [:ok, convert.call(raw, \"\")] }; if outcome[0] == :error; Result::Err.new(outcome[1]); else; Result::Ok.new(outcome[1]); end; rescue JSON::ParserError => error; line_match = error.message.match(/line (\\d+)/); column_match = error.message.match(/column (\\d+)/); line = line_match && line_match[1].to_i; column = column_match && column_match[1].to_i; Result::Err.new(" + syntaxError + "); rescue StandardError => error; Result::Err.new(" + genericError + "); end }.call(" + argument + ")"
}

func rubyJSONStringify(argument string) string {
	errorValue := func(message, path string) string {
		return "JsonError.new(kind: JsonErrorKind::Encode, message: " + message + ", path: " + path + ", line: nil, column: nil)"
	}
	conversionError := errorValue("message", "path")
	convert := "convert = nil; convert = ->(value, path) { if value.equal?(JsonValue::Null); nil; elsif value.is_a?(JsonValue::Boolean); value.value; elsif value.is_a?(JsonValue::Integer); if value.value < -9007199254740991 || value.value > 9007199254740991; message = \"JSON integer is outside the portable range\"; throw :__trb_json_error, [:error, " + conversionError + "]; end; value.value; elsif value.is_a?(JsonValue::Float); unless value.value.finite?; message = \"JSON Float must be finite\"; throw :__trb_json_error, [:error, " + conversionError + "]; end; value.value; elsif value.is_a?(JsonValue::String); value.value; elsif value.is_a?(JsonValue::Array); value.value.each_with_index.map { |item, index| convert.call(item, path + \"/\" + index.to_s) }; elsif value.is_a?(JsonValue::Object); fields = {}; value.value.each { |key, item| escaped = key.gsub(\"~\", \"~0\").gsub(\"/\", \"~1\"); fields[key] = convert.call(item, path + \"/\" + escaped) }; fields; else; message = \"unsupported JSON value\"; throw :__trb_json_error, [:error, " + conversionError + "]; end }"
	encodeError := errorValue("error.message", `""`)
	return "->(value) { begin; require \"json\"; " + convert + "; outcome = catch(:__trb_json_error) { [:ok, convert.call(value, \"\")] }; if outcome[0] == :error; Result::Err.new(outcome[1]); else; Result::Ok.new(JSON.generate(outcome[1])); end; rescue StandardError => error; Result::Err.new(" + encodeError + "); end }.call(" + argument + ")"
}

func rubyJSONDecode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	builder := &rubyJSONCodecBuilder{}
	decoder := builder.decoder(call.Codec)
	parsed := rubyJSONParse(argument, false)
	errorValue := "JsonError.new(kind: JsonErrorKind::Decode, message: message, path: path, line: nil, column: nil)"
	return "-> { fail = ->(path, message) { throw :__trb_json_codec_error, [:error, " + errorValue + "] }; " + builder.source.String() + " parsed = " + parsed + "; if parsed.is_a?(Result::Err); parsed; else; outcome = catch(:__trb_json_codec_error) { [:ok, " + decoder + ".call(parsed.value, \"\")] }; if outcome[0] == :error; Result::Err.new(outcome[1]); else; Result::Ok.new(outcome[1]); end; end }.call"
}

func rubyJSONEncode(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "nil"
	}
	builder := &rubyJSONCodecBuilder{}
	encoder := builder.encoder(call.Codec)
	return "-> { " + builder.source.String() + " encoded = " + encoder + ".call(" + argument + "); " + rubyJSONStringify("encoded") + " }.call"
}

type rubyJSONCodecBuilder struct {
	source strings.Builder
	next   int
}

func (b *rubyJSONCodecBuilder) name(prefix string) string {
	b.next++
	return "__trb_json_" + strings.ToLower(prefix) + "_" + strconv.Itoa(b.next)
}

func (b *rubyJSONCodecBuilder) decoder(schema *ir.CodecSchema) string {
	name := b.name("decode")
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.decoder(&nonnull)
		b.source.WriteString(name + " = ->(value, path) { value.equal?(JsonValue::Null) ? nil : " + child + ".call(value, path) }; ")
		return name
	}
	expected := func(kind string) string { return "fail.call(path, " + strconv.Quote("expected "+kind) + ")" }
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "unless value.is_a?(JsonValue::Boolean); " + expected("Boolean") + "; end; value.value"
	case "integer":
		body = "unless value.is_a?(JsonValue::Integer); " + expected("Integer") + "; end; value.value"
	case "float":
		body = "if value.is_a?(JsonValue::Integer); value.value.to_f; elsif value.is_a?(JsonValue::Float); value.value; else; " + expected("Float") + "; end"
	case "string":
		body = "unless value.is_a?(JsonValue::String); " + expected("String") + "; end; value.value"
	case "array":
		child := b.decoder(schema.Element)
		body = "unless value.is_a?(JsonValue::Array); " + expected("Array") + "; end; value.value.each_with_index.map { |item, index| " + child + ".call(item, path + \"/\" + index.to_s) }"
	case "hash":
		child := b.decoder(schema.Element)
		body = "unless value.is_a?(JsonValue::Object); " + expected("Object") + "; end; decoded = {}; value.value.each { |key, item| escaped = key.gsub(\"~\", \"~0\").gsub(\"/\", \"~1\"); decoded[key] = " + child + ".call(item, path + \"/\" + escaped) }; decoded"
	case "record":
		var fields strings.Builder
		fields.WriteString("unless value.is_a?(JsonValue::Object); " + expected(schema.Type.Name) + "; end; ")
		parts := make([]string, 0, len(schema.Fields))
		for index, field := range schema.Fields {
			child := b.decoder(field.Schema)
			variable := "field" + strconv.Itoa(index)
			path := "path + " + strconv.Quote("/"+rubyJSONPointerEscape(field.WireName))
			fields.WriteString("if value.value.key?(" + strconv.Quote(field.WireName) + "); " + variable + " = " + child + ".call(value.value[" + strconv.Quote(field.WireName) + "], " + path + "); ")
			if field.Schema.Type.Nullable {
				fields.WriteString("else; " + variable + " = nil; end; ")
			} else {
				fields.WriteString("else; " + variable + " = fail.call(" + path + ", " + strconv.Quote("missing field "+field.WireName) + "); end; ")
			}
			parts = append(parts, field.Name+": "+variable)
		}
		fields.WriteString(schema.Type.Name + ".new(" + strings.Join(parts, ", ") + ")")
		body = fields.String()
	}
	b.source.WriteString(name + " = ->(value, path) { " + body + " }; ")
	return name
}

func rubyJSONPointerEscape(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

func (b *rubyJSONCodecBuilder) encoder(schema *ir.CodecSchema) string {
	name := b.name("encode")
	if schema.Type.Nullable {
		nonnull := *schema
		nonnull.Type.Nullable = false
		child := b.encoder(&nonnull)
		b.source.WriteString(name + " = ->(value) { value.nil? ? JsonValue::Null : " + child + ".call(value) }; ")
		return name
	}
	body := ""
	switch schema.Kind {
	case "boolean":
		body = "JsonValue::Boolean.new(value)"
	case "integer":
		body = "JsonValue::Integer.new(value)"
	case "float":
		body = "JsonValue::Float.new(value)"
	case "string":
		body = "JsonValue::String.new(value)"
	case "array":
		child := b.encoder(schema.Element)
		body = "JsonValue::Array.new(value.map { |item| " + child + ".call(item) })"
	case "hash":
		child := b.encoder(schema.Element)
		body = "fields = {}; value.each { |key, item| fields[key] = " + child + ".call(item) }; JsonValue::Object.new(fields)"
	case "record":
		parts := make([]string, 0, len(schema.Fields))
		for _, field := range schema.Fields {
			child := b.encoder(field.Schema)
			parts = append(parts, strconv.Quote(field.WireName)+" => "+child+".call(value."+field.Name+")")
		}
		body = "JsonValue::Object.new({" + strings.Join(parts, ", ") + "})"
	}
	b.source.WriteString(name + " = ->(value) { " + body + " }; ")
	return name
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
