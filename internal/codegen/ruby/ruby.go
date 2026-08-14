package ruby

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

type generator struct {
	b               strings.Builder
	indent          int
	loader          string
	modulePath      string
	topFunctions    map[string]bool
	topTargets      map[string]string
	nativeSyntax    bool
	temporary       int
	jobs            *jobsintegration.Manifest
	jobsSQL         *jobssql.Manifest
	orm             *ormintegration.Manifest
	breakTarget     string
	execution       *effectplan.Plan
	executionActive bool
	oidcRuntime     bool
}

func Generate(program *ir.Program) string {
	return GenerateProject([]*ir.Program{program})[0]
}

func GenerateProject(programs []*ir.Program) []string {
	execution := effectplan.ExecutionScope(programs)
	result := make([]string, len(programs))
	for index, program := range programs {
		result[index] = generate(program, execution)
	}
	return result
}

func generate(program *ir.Program, execution *effectplan.Plan) string {
	g := &generator{
		loader: program.RubyLoader, modulePath: program.ModulePath,
		topFunctions: map[string]bool{}, topTargets: map[string]string{},
		jobs:    jobsintegration.ManifestFrom(program.Extensions),
		jobsSQL: jobssql.ManifestFrom(program.Extensions),
		orm:     ormintegration.ManifestFrom(program.Extensions), execution: execution,
	}
	for _, statement := range program.Statements {
		if method, ok := statement.(*ir.Method); ok {
			g.topFunctions[method.Name] = true
			if method.TargetName != "" {
				g.topTargets[method.Name] = method.TargetName
			}
		}
		if imported, ok := statement.(*ir.Import); ok && (imported.Path == "trb/platform/ruby/native" || imported.Path == "trb/platform/ruby/rails") {
			g.nativeSyntax = true
		}
	}
	if g.programUsesExecutionScope(program.Statements) || webintegration.ManifestFrom(program.Extensions) != nil || g.jobs != nil {
		g.executionScopeRuntime()
	}
	g.statements(program.Statements)
	g.integrations(program.Extensions)
	if g.oidcRuntime {
		g.oidcBearerRuntimeSupport()
	}
	if g.topFunctions["main"] {
		if len(program.Statements) > 0 {
			g.b.WriteByte('\n')
		}
		main := topLevelRubyMethod(program.Statements, "main")
		call := "main()"
		if g.methodUsesExecutionScope(main) {
			call = "main(TrbExecutionScope.root)"
		}
		if g.jobs != nil && len(g.jobs.Jobs) > 0 {
			g.line(call+" unless trb_jobs_run_worker_or_command", "")
		} else {
			g.line(call, "")
		}
	}
	return strings.TrimRight(g.b.String(), "\n") + "\n"
}

func (g *generator) statements(statements []ir.Statement) {
	if g.executionActive && len(statements) > 0 {
		g.line("__trb_scope.check!", "")
	}
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
	case *ir.Class, *ir.Record, *ir.Enum, *ir.TypeAlias, *ir.Module, *ir.Interface, *ir.Method:
		return true
	}
	return false
}

func (g *generator) statement(statement ir.Statement) {
	switch n := statement.(type) {
	case *ir.Comment:
		g.line(n.Text, "")
	case *ir.Import:
		if (n.Standard || n.Official) && (!n.Runtime || !n.RuntimeRequired) || g.loader == "zeitwerk" && !n.Runtime {
			return
		}
		g.line("require_relative "+strconv.Quote(rubyImportPath(g.modulePath, n.Path)), n.TrailingComment)
	case *ir.Class:
		if n.External {
			return
		}
		header := "class " + g.rubyClassName(n.Name, nil)
		if n.Superclass != nil {
			header += " < " + g.expr(n.Superclass)
		}
		g.line(header, n.TrailingComment)
		g.indent++
		fields := classFields(n.Body)
		for _, field := range fields {
			name := strings.TrimPrefix(field.Name, "@")
			if strings.HasPrefix(name, "_") {
				continue
			}
			accessor := "__trb_field_" + name
			g.line("def "+accessor+"; @"+name+"; end", "")
			if !field.ReadOnly {
				g.line("def "+accessor+"=(value); @"+name+" = value; end", "")
			}
		}
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
		fields := ":name"
		if n.RawType.Kind != "" {
			fields += ", :raw_value"
		}
		methods := enumMethods(n)
		if len(methods) == 0 {
			g.line(n.Name+" = Data.define("+fields+")", n.TrailingComment)
		} else {
			g.line(n.Name+" = Data.define("+fields+") do", n.TrailingComment)
			g.indent++
			for _, method := range methods {
				g.method(method, nil)
			}
			g.indent--
			g.line("end", "")
		}
		for _, statement := range n.Body {
			switch member := statement.(type) {
			case *ir.Comment:
				g.statement(member)
			case *ir.EnumMember:
				arguments := ":" + member.Name
				if member.RawValue != nil {
					arguments += ", " + g.expr(member.RawValue)
				}
				g.line(n.Name+"::"+member.Name+" = "+n.Name+".new("+arguments+")", member.TrailingComment)
			}
		}
	case *ir.TypeAlias:
		if len(n.Variants) > 0 {
			g.line(n.Name+" = "+n.Target.Name, n.TrailingComment)
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
			g.line("def "+method.Name+"("+g.methodParameters(method)+")", method.TrailingComment)
			g.indent++
			g.line("raise NotImplementedError", "")
			g.indent--
			g.line("end", "")
		}
		g.indent--
		g.line("end", "")
	case *ir.Method:
		if n.External {
			return
		}
		g.method(n, nil)
	case *ir.Variable:
		g.line(n.Name+" = "+g.expr(n.Value), n.TrailingComment)
	case *ir.Temporary:
		g.line(n.Name+" = nil", n.TrailingComment)
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
		if g.breakTarget != "" {
			g.line("throw "+strconv.Quote(g.breakTarget), n.TrailingComment)
		} else {
			g.line("break", n.TrailingComment)
		}
	case *ir.Next:
		g.line("next", n.TrailingComment)
	case *ir.ExpressionStatement:
		if call, ok := n.Expression.(*ir.Call); ok && call.Block != nil {
			if g.ormAssociationDeclaration(call) || g.jobsDeclaration(call) {
				break
			}
			g.callBlock(call, n.TrailingComment)
			break
		}
		if call, ok := n.Expression.(*ir.Call); ok && (g.ormAssociationDeclaration(call) || g.jobsDeclaration(call)) {
			break
		}
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
		if n.TypeUnion {
			g.typeUnionCase(n)
			break
		}
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
		if strings.HasPrefix(n.Intrinsic, "trb.orm.") {
			g.ormBatchIterate(n)
			break
		}
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
		if n.Source.ExprType().Kind == types.Hash {
			header = source + ".to_a.each"
		}
		parameters := make([]string, 0, len(n.Bindings))
		for _, binding := range n.Bindings {
			parameters = append(parameters, binding.Name)
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
	case *ir.StructuredBlock:
		g.ormStructuredBlock(n)
	}
}

func (g *generator) callBlock(call *ir.Call, trailingComment string) {
	withoutBlock := *call
	withoutBlock.Block = nil
	header := g.expr(&withoutBlock)
	if g.nativeSyntax && len(call.Arguments) == 0 && expressionReference(call.Callee) == nil {
		header = g.expr(call.Callee)
	}
	header += " do"
	if len(call.Block.Parameters) > 0 {
		header += " |" + strings.Join(call.Block.Parameters, ", ") + "|"
	}
	g.line(header, trailingComment)
	g.indent++
	g.statements(call.Block.Body)
	g.indent--
	g.line("end", "")
}

func (g *generator) typeUnionCase(node *ir.Case) {
	g.statements(node.Leading)
	g.temporary++
	value := "__trb_case" + strconv.Itoa(g.temporary)
	g.line(value+" = "+g.expr(node.Value), node.TrailingComment)
	g.line("case "+value, "")
	for _, branch := range node.Branches {
		g.line("when "+rubyTypePattern(branch.MatchType), branch.TrailingComment)
		g.indent++
		for _, binding := range branch.Bindings {
			if binding.Name != "_" {
				g.line(binding.Name+" = "+value, "")
			}
		}
		g.statements(branch.Body)
		g.indent--
	}
	g.line("else", "")
	g.indent++
	if node.HasElse {
		g.statements(node.Else)
	} else {
		g.line(`raise "unreachable exhaustive case"`, "")
	}
	g.indent--
	g.line("end", "")
}

func rubyTypePattern(typ types.Type) string {
	switch typ.Kind {
	case types.Bool:
		return "TrueClass, FalseClass"
	case types.Int:
		return "Integer"
	case types.Float:
		return "Float"
	case types.String:
		return "String"
	default:
		return "Object"
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
	methods := enumMethods(enum)
	if len(methods) > 0 {
		g.line("module Methods", "")
		g.indent++
		for _, method := range methods {
			g.method(method, nil)
		}
		g.indent--
		g.line("end", "")
	}
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
			if len(methods) > 0 {
				definition += " { include Methods }"
			}
			if len(fields) == 0 {
				definition += ".new"
			}
			g.line(member.Name+" = "+definition, member.TrailingComment)
		}
	}
	g.indent--
	g.line("end", "")
}

func enumMethods(enum *ir.Enum) []*ir.Method {
	result := []*ir.Method{}
	for _, statement := range enum.Body {
		if method, ok := statement.(*ir.Method); ok && !method.External {
			result = append(result, method)
		}
	}
	return result
}

func (g *generator) method(method *ir.Method, fields []*ir.Field) {
	name := method.Name
	if !method.Class && method.TargetName != "" {
		name = method.TargetName
	}
	if method.Class {
		name = "self." + name
	}
	g.line("def "+name+"("+g.methodParameters(method)+")", method.TrailingComment)
	g.indent++
	if method.Name == "initialize" {
		g.fieldDefaults(fields)
	}
	previousExecution := g.executionActive
	g.executionActive = g.methodUsesExecutionScope(method)
	g.statements(method.Body)
	g.executionActive = previousExecution
	g.indent--
	g.line("end", "")
}

func topLevelRubyMethod(statements []ir.Statement, name string) *ir.Method {
	for _, statement := range statements {
		if method, ok := statement.(*ir.Method); ok && method.Name == name {
			return method
		}
	}
	return nil
}

func (g *generator) methodUsesExecutionScope(method *ir.Method) bool {
	return method != nil && (strings.HasPrefix(method.TargetName, "trb_web_route_") ||
		strings.HasPrefix(method.TargetName, "trb_web_middleware_") ||
		g.execution != nil && g.execution.Methods[method])
}

func (g *generator) methodParameters(method *ir.Method) string {
	parameters := g.parameters(method.Parameters)
	if !g.methodUsesExecutionScope(method) {
		return parameters
	}
	if parameters == "" {
		return "__trb_scope"
	}
	return "__trb_scope, " + parameters
}

func (g *generator) executionArguments(call *ir.Call, arguments []string) []string {
	if g.execution == nil || !g.execution.Calls[call] {
		return arguments
	}
	return append([]string{"__trb_scope"}, arguments...)
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
	case *ir.If:
		return g.ifExpression(n)
	case *ir.Case:
		return g.caseExpression(n)
	case *ir.Attempt:
		return g.attemptExpression(n)
	case *ir.Lambda:
		parts := make([]string, len(n.Parameters))
		for index, parameter := range n.Parameters {
			parts[index] = parameter.Name
		}
		child := *g
		child.b = strings.Builder{}
		child.indent = g.indent + 1
		child.statements(n.Body)
		return "->(" + strings.Join(parts, ", ") + ") do\n" + child.b.String() + strings.Repeat("  ", g.indent) + "end"
	case *ir.UnhandledEffect:
		return g.expr(n.Value)
	case *ir.Identifier:
		if !n.Lexical && g.topTargets[n.Name] != "" {
			return g.topTargets[n.Name]
		}
		return g.rubyClassName(n.Name, n.Reference)
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
		case ir.RangeToIterableConversion:
			return g.expr(n.Value)
		case ir.PureFunctionToFallibleConversion:
			return g.pureFunctionToFallible(n)
		case ir.IntegerToFloatConversion:
			return "(" + g.expr(n.Value) + ").to_f"
		case ir.UnionIntegerToFloatConversion:
			return "(->(value) { value.is_a?(Integer) ? value.to_f : value }).call(" + g.expr(n.Value) + ")"
		case ir.NonNullableToNullableConversion:
			if n.Value.ExprType().Kind == types.Int && n.ExprType().Kind == types.Float {
				return "(" + g.expr(n.Value) + ").to_f"
			}
			return g.expr(n.Value)
		case ir.NullableToNonNullableConversion:
			return g.expr(n.Value)
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
		if receiver, ok := n.Receiver.(*ir.Identifier); ok && n.Reference != nil && n.Reference.Intrinsic == "" && n.Reference.Package != "" && n.Reference.Alias != "" && receiver.Name == n.Reference.Alias && n.Reference.ExportKind == "function" {
			return n.Name
		}
		op := "."
		if n.Namespace {
			op = "::"
		} else if n.Safe {
			op = "&."
		}
		if n.ClassField {
			return g.expr(n.Receiver) + op + "__trb_field_" + n.Name
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
				if member, ok := receiverMember(n.Callee); ok {
					parts = append([]string{g.expr(member.Receiver)}, parts...)
				}
			}
			return g.intrinsic(reference.Intrinsic, n, parts)
		}
		parts = g.executionArguments(n, parts)
		callee := g.expr(n.Callee)
		if n.Callee.ExprType().Kind == types.Function {
			return callee + ".call(" + strings.Join(parts, ", ") + ")"
		}
		return callee + "(" + strings.Join(parts, ", ") + ")"
	case *ir.EnumCall:
		parts := make([]string, len(n.Arguments))
		for index, argument := range n.Arguments {
			parts[index] = g.expr(argument.Value)
		}
		switch n.Method {
		case "raw_value":
			return g.expr(n.Receiver) + ".raw_value"
		case "from_raw":
			branches := make([]string, 0, len(n.RawValues))
			for _, item := range n.RawValues {
				branches = append(branches, "when "+item.Raw+" then Result::Ok.new("+n.EnumName+"::"+item.Member+")")
			}
			message := strconv.Quote("unknown raw value for " + n.EnumName)
			return "begin; value = " + parts[0] + "; case value; " + strings.Join(branches, "; ") + "; else Result::Err.new(EnumValueError.new(value: value, message: " + message + ")); end; end"
		default:
			if g.execution != nil && g.execution.EnumCalls[n] {
				parts = append([]string{"__trb_scope"}, parts...)
			}
			return g.expr(n.Receiver) + "." + n.Method + "(" + strings.Join(parts, ", ") + ")"
		}
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
		if n.Receiver.ExprType().Kind == types.String {
			return "->(value, index) { characters = value.each_char.to_a; raise IndexError, \"String index is out of bounds\" if index < 0 || index >= characters.length; characters.fetch(index) }.call(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		if n.Receiver.ExprType().Kind == types.Array {
			return "->(values, index) { raise IndexError, \"Array index is out of bounds\" if index < 0 || index >= values.length; values.fetch(index) }.call(" + g.expr(n.Receiver) + ", " + g.expr(n.Index) + ")"
		}
		return g.expr(n.Receiver) + "[" + g.expr(n.Index) + "]"
	case *ir.NativeExpression:
		return n.Text
	default:
		return ""
	}
}

func rubyTimeRuntimeClass(name string) string {
	switch name {
	case "Date", "TimeOfDay", "DateTime", "Duration", "TimeZone", "Instant":
		return "TrbTime" + name
	default:
		return name
	}
}

func (g *generator) rubyClassName(name string, reference *ir.Reference) string {
	if g.modulePath == "trb/std/time/index" || reference != nil && reference.Package == "trb/std/time/index" {
		return rubyTimeRuntimeClass(name)
	}
	return name
}

func (g *generator) ifExpression(node *ir.If) string {
	child := &generator{
		loader:          g.loader,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		nativeSyntax:    g.nativeSyntax,
		temporary:       g.temporary,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		execution:       g.execution,
		executionActive: g.executionActive,
	}
	child.line("begin", "")
	child.indent++
	child.line("if "+child.expr(node.Condition), node.TrailingComment)
	child.indent++
	child.statements(node.Then)
	if node.ThenResult != nil {
		child.line(child.expr(node.ThenResult), "")
	}
	child.indent--
	for _, branch := range node.ElseIf {
		child.line("elsif "+child.expr(branch.Condition), "")
		child.indent++
		child.statements(branch.Body)
		if branch.Result != nil {
			child.line(child.expr(branch.Result), "")
		}
		child.indent--
	}
	child.line("else", "")
	child.indent++
	child.statements(node.Else)
	if node.ElseResult != nil {
		child.line(child.expr(node.ElseResult), "")
	}
	child.indent--
	child.line("end", "")
	child.indent--
	child.line("end", "")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) attemptExpression(node *ir.Attempt) string {
	child := &generator{
		loader:          g.loader,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		topTargets:      g.topTargets,
		nativeSyntax:    g.nativeSyntax,
		temporary:       g.temporary,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		execution:       g.execution,
		executionActive: g.executionActive,
	}
	child.line("-> do", "")
	child.indent++
	child.statements(node.Body)
	result := node.Value
	if result == nil {
		result = node.BodyResult
	}
	child.line(child.expr(result), "")
	child.indent--
	child.line("end.call", "")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) caseExpression(node *ir.Case) string {
	child := &generator{
		loader:          g.loader,
		modulePath:      g.modulePath,
		topFunctions:    g.topFunctions,
		nativeSyntax:    g.nativeSyntax,
		temporary:       g.temporary,
		jobs:            g.jobs,
		jobsSQL:         g.jobsSQL,
		orm:             g.orm,
		breakTarget:     g.breakTarget,
		execution:       g.execution,
		executionActive: g.executionActive,
	}
	child.line("begin", "")
	child.indent++
	child.statements(node.Leading)
	payload := caseHasPayload(node)
	caseValue := child.expr(node.Value)
	value := ""
	caseComment := node.TrailingComment
	if payload || node.TypeUnion {
		child.temporary++
		value = "__trb_case" + strconv.Itoa(child.temporary)
		child.line(value+" = "+caseValue, node.TrailingComment)
		caseValue = value
		caseComment = ""
	}
	child.line("case "+caseValue, caseComment)
	for _, branch := range node.Branches {
		pattern := child.expr(branch.Value)
		if node.TypeUnion {
			pattern = rubyTypePattern(branch.MatchType)
		}
		child.line("when "+pattern, branch.TrailingComment)
		child.indent++
		for _, binding := range branch.Bindings {
			if binding.Name == "_" {
				continue
			}
			bindingValue := value
			if branch.PayloadEnum {
				bindingValue += "." + binding.Field
			}
			child.line(binding.Name+" = "+bindingValue, "")
		}
		child.statements(branch.Body)
		if branch.Result != nil {
			child.line(child.expr(branch.Result), "")
		}
		child.indent--
	}
	child.line("else", "")
	child.indent++
	if node.HasElse {
		child.statements(node.Else)
		if node.ElseResult != nil {
			child.line(child.expr(node.ElseResult), "")
		}
	} else {
		child.line(`raise "unreachable exhaustive case"`, "")
	}
	child.indent--
	child.line("end", "")
	child.indent--
	child.line("end", "")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func (g *generator) transform(transform *ir.Transform) string {
	source := g.expr(transform.Source)
	if _, rangeSource := transform.Source.(*ir.Range); rangeSource {
		source = "(" + source + ")"
	}
	result := g.transformResult(transform)
	switch transform.Operation {
	case "sort_by", "sort_by_descending":
		comparison := rubyPortableSortComparison("left[1]", "right[1]", transform.Result.ExprType(), transform.Operation == "sort_by_descending")
		return source + ".each_with_index.map { |" + transform.Item + ", index| [" + transform.Item + ", " + result + ", index] }.sort { |left, right| compared = " + comparison + "; compared.zero? ? left[2] <=> right[2] : compared }.map(&:first)"
	case "map", "select", "any?", "all?", "none?", "find", "find_index":
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

func (g *generator) transformResult(transform *ir.Transform) string {
	if len(transform.Body) == 0 {
		return g.expr(transform.Result)
	}
	child := *g
	child.b = strings.Builder{}
	child.indent = 0
	child.line("-> do", "")
	child.indent++
	child.statements(transform.Body)
	child.line(child.expr(transform.Result), "")
	child.indent--
	child.line("end.call", "")
	g.temporary = child.temporary
	return strings.TrimSpace(child.b.String())
}

func rubyPortableSortComparison(left, right string, typ types.Type, descending bool) string {
	if base, literal := types.LiteralBase(typ); literal {
		typ = base
	}
	if typ.Kind == types.Float {
		operator := left + " <=> " + right
		if descending {
			operator = right + " <=> " + left
		}
		return "(" + left + ".nan? ? (" + right + ".nan? ? 0 : 1) : (" + right + ".nan? ? -1 : (" + operator + ")))"
	}
	if descending {
		return right + " <=> " + left
	}
	return left + " <=> " + right
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

func portableFloatInteger(value, operation string) string {
	return "->(value) { raise FloatDomainError, \"Float cannot be converted to Integer\" unless value.finite?; integer = value." + operation + "; raise RangeError, \"Integer is outside the portable range\" if integer < -9007199254740991 || integer > 9007199254740991; integer }.call(" + value + ")"
}

func portableLog(value, operation string) string {
	return "->(value) { if value < 0; Float::NAN; elsif value.zero?; -Float::INFINITY; else; Math." + operation + "(value); end }.call(" + value + ")"
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
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		method := "try_parse"
		if schema.Kind == "time_zone" {
			method = "try_get"
		}
		body = "unless value.is_a?(JsonValue::String); " + expected("String") + "; end; parsed = " + rubyTimeRuntimeClass(schema.Type.Name) + "." + method + "(value.value); parsed.is_a?(Result::Err) ? fail.call(path, " + strconv.Quote("invalid "+schema.Type.Name) + ") : parsed.value"
	case "raw_enum":
		kind := "String"
		if schema.RawType.Kind == types.Int {
			kind = "Integer"
		}
		branches := make([]string, 0, len(schema.RawValues))
		for _, item := range schema.RawValues {
			branches = append(branches, "when "+item.Raw+" then "+schema.Type.Name+"::"+item.Member)
		}
		body = "unless value.is_a?(JsonValue::" + kind + "); " + expected(kind) + "; end; case value.value; " + strings.Join(branches, "; ") + "; else; fail.call(path, " + strconv.Quote("unknown raw value for "+schema.Type.Name) + "); end"
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
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration":
		body = "JsonValue::String.new(value.to_s())"
	case "time_zone":
		body = "JsonValue::String.new(value.identifier())"
	case "raw_enum":
		kind := "String"
		if schema.RawType.Kind == types.Int {
			kind = "Integer"
		}
		body = "JsonValue::" + kind + ".new(value.raw_value)"
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

func receiverMember(expression ir.Expression) (*ir.Member, bool) {
	switch node := expression.(type) {
	case *ir.Member:
		return node, true
	case *ir.TypeApply:
		return receiverMember(node.Receiver)
	default:
		return nil, false
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

func (g *generator) pureFunctionToFallible(conversion *ir.Conversion) string {
	parameters, success, ok := types.FunctionSignature(conversion.ExprType())
	if !ok {
		return g.expr(conversion.Value)
	}
	arguments := make([]string, len(parameters))
	for index := range parameters {
		arguments[index] = "__trb_arg" + strconv.Itoa(index)
	}
	call := "__trb_value.call(" + strings.Join(arguments, ", ") + ")"
	value := call
	prefix := ""
	if success.Kind == types.Void {
		prefix = call + "; "
		value = "Unit.new"
	}
	wrapped := "->(" + strings.Join(arguments, ", ") + ") { " + prefix + "Result::Ok.new(" + value + ") }"
	return "->(__trb_value) { " + wrapped + " }.call(" + g.expr(conversion.Value) + ")"
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
