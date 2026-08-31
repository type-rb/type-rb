package golang

import (
	"sort"
	"strconv"
	"strings"

	cliapp "github.com/type-rb/type-rb/internal/cliapp"
	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/ir"
)

// Package-scope CLI support uses the trb__cli prefix. Source identifier
// lowering removes separator runs, so TypeRB declarations cannot produce this
// consecutive-underscore spelling and collide with compiler-owned support.
func (g *generator) cliRun(call *ir.Call) string {
	index, _, ok := g.cli.InvocationIndex(g.modulePath, call.SourceSpan().Start.Offset)
	if !ok {
		return "func() " + g.goType(call.ExprType()) + " { panic(\"trb/cli schema is unavailable\") }()"
	}
	g.cliInvocations[index] = true
	arguments := map[string]string{}
	statements := make([]string, 0, len(call.Arguments)+1)
	for _, argument := range call.Arguments {
		if argument.Name != "" {
			g.temporary++
			name := "__trb__cliArgument" + strconv.Itoa(g.temporary)
			typeName := "*string"
			if argument.Name == "name" {
				typeName = "string"
			}
			statements = append(statements, "var "+name+" "+typeName+" = "+g.expr(argument.Value))
			arguments[argument.Name] = name
		}
	}
	name := arguments["name"]
	version := arguments["version"]
	about := arguments["about"]
	if name == "" {
		name = strconv.Quote("app")
	}
	if version == "" {
		version = "nil"
	}
	if about == "" {
		about = "nil"
	}
	invocation := "trb__cliRun" + strconv.Itoa(index) + "(" + strings.Join([]string{g.executionScopeArgument(), name, version, about}, ", ") + ")"
	if len(statements) == 0 {
		return invocation
	}
	statements = append(statements, "return "+invocation)
	return "func() " + g.goType(call.ExprType()) + " { " + strings.Join(statements, "; ") + " }()"
}

func (g *generator) cliIntegrations() {
	if len(g.cliInvocations) == 0 || g.cli == nil {
		return
	}
	g.requireImport("fmt", "")
	g.requireImport("context", "trbcontext")
	g.requireImport("os", "")
	g.requireImport("strconv", "")
	g.requireImport("strings", "")
	indexes := make([]int, 0, len(g.cliInvocations))
	for index := range g.cliInvocations {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		g.cliInvocation(index, &g.cli.Invocations[index])
	}
	if g.cliRuntimeOwner() {
		g.cliRuntimeSupport()
	}
}

func (g *generator) cliRuntimeOwner() bool {
	directory := moduleDirectory(g.modulePath)
	owner := ""
	for _, invocation := range g.cli.Invocations {
		if moduleDirectory(invocation.ModulePath) != directory {
			continue
		}
		if owner == "" || invocation.ModulePath < owner {
			owner = invocation.ModulePath
		}
	}
	return owner == g.modulePath
}

func (g *generator) cliInvocation(index int, invocation *cliapp.Invocation) {
	if invocation == nil {
		return
	}
	schema := invocation.Schema
	resultType := g.cliTypeName(schema.Root.ModulePath, schema.Root.Name)
	g.line("func trb__cliRun" + strconv.Itoa(index) + "(__trbScope trbcontext.Context, name string, version *string, about *string) " + resultType + " {")
	g.indent++
	g.line("spec := " + g.cliSpecLiteral(schema))
	g.line("parsed := trb__cliParse(os.Args[1:], name, version, about, spec)")
	if len(schema.Root.Fields) == 0 && len(schema.Commands) == 0 {
		g.line("_ = parsed")
	}
	rootValues := g.cliFieldValues(schema.Root, "root", "root")
	commandValue := ""
	if schema.SubcommandField != "" {
		commandValue = "trb__cliCommand"
		enumType := g.cliTypeName(schema.SubcommandEnum.ModulePath, schema.SubcommandEnum.Name)
		g.line("var " + commandValue + " " + enumType)
		g.line("switch parsed.Command {")
		g.indent++
		for commandIndex, command := range schema.Commands {
			g.line("case " + strconv.Quote(command.Name) + ":")
			g.indent++
			value := g.cliEnumValue(command, commandIndex)
			g.line(commandValue + " = " + value)
			g.indent--
		}
		g.indent--
		g.line("}")
	}
	expression := g.cliRecordExpression(schema.Root, rootValues, cliConstructField{
		Order: schema.SubcommandOrder, Name: schema.SubcommandField, Value: commandValue,
	})
	g.line("return " + expression)
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

type cliConstructField struct {
	Order      int
	Name       string
	Value      string
	Provided   string
	HasDefault bool
}

func (g *generator) cliEnumValue(command cliapp.Command, commandIndex int) string {
	qualifier := g.cliQualifier(command.Enum.ModulePath)
	if command.Payload == nil {
		return qualifier + goConstantIdentifier(command.Enum.Name, command.MemberName)
	}
	prefix := "command" + strconv.Itoa(commandIndex)
	values := g.cliFieldValues(*command.Payload, prefix, cliCommandKeyPrefix(commandIndex))
	payload := g.cliRecordExpression(*command.Payload, values, cliConstructField{})
	if command.PayloadNamedOnly {
		payload = "map[string]any{" + strconv.Quote(command.PayloadName) + ": " + payload + "}"
	}
	return qualifier + "New" + goIdentifier(command.Enum.Name, true) + goIdentifier(command.MemberName, true) + "(" + payload + ")"
}

func (g *generator) cliFieldValues(record cliapp.Record, prefix, keyPrefix string) []cliConstructField {
	result := make([]cliConstructField, 0, len(record.Fields))
	for _, field := range record.Fields {
		name := "trb__cli" + goIdentifier(prefix, true) + goIdentifier(field.Name, true)
		provided := name + "Provided"
		key := keyPrefix + "." + field.Name
		g.line(provided + " := parsed.Provided[" + strconv.Quote(key) + "]")
		baseType := cliGoScalarType(field.Kind)
		if field.Repeated {
			values := name + "Values"
			if !field.HasDefault {
				g.line("_ = " + provided)
			}
			g.line(values + " := make([]" + baseType + ", 0, len(parsed.Values[" + strconv.Quote(key) + "]))")
			g.line("for " + name + "Index, " + name + "Raw := range parsed.Values[" + strconv.Quote(key) + "] {")
			g.indent++
			g.line("_ = " + name + "Index")
			parsed := g.cliParsedScalar(field, name+"Raw", name+"Value")
			for _, line := range parsed.Lines {
				g.line(line)
			}
			g.line(values + " = append(" + values + ", " + parsed.Value + ")")
			g.indent--
			g.line("}")
			g.line(name + " := " + g.arrayReference(values))
		} else if field.Nullable {
			g.line("var " + name + " *" + baseType)
			g.line("if " + provided + " {")
			g.indent++
			parsed := g.cliParsedScalar(field, "parsed.Values["+strconv.Quote(key)+"][len(parsed.Values["+strconv.Quote(key)+"])-1]", name+"Value")
			for _, line := range parsed.Lines {
				g.line(line)
			}
			g.line(name + " = &" + parsed.Value)
			g.indent--
			g.line("}")
		} else {
			g.line("var " + name + " " + baseType)
			g.line("if " + provided + " {")
			g.indent++
			parsed := g.cliParsedScalar(field, "parsed.Values["+strconv.Quote(key)+"][len(parsed.Values["+strconv.Quote(key)+"])-1]", name+"Value")
			for _, line := range parsed.Lines {
				g.line(line)
			}
			g.line(name + " = " + parsed.Value)
			g.indent--
			g.line("}")
		}
		result = append(result, cliConstructField{Order: field.SourceOrder, Name: field.Name, Value: name, Provided: provided, HasDefault: field.HasDefault})
	}
	return result
}

type cliParsedScalar struct {
	Lines []string
	Value string
}

func (g *generator) cliParsedScalar(field cliapp.Field, raw, name string) cliParsedScalar {
	switch field.Kind {
	case cliapp.StringValue:
		return cliParsedScalar{Lines: []string{name + " := " + raw}, Value: name}
	case cliapp.IntegerValue:
		return cliParsedScalar{Lines: []string{
			name + "Parsed, " + name + "Err := strconv.ParseInt(" + raw + ", 10, 64)",
			"if " + name + "Err != nil || " + name + "Parsed < -9007199254740991 || " + name + "Parsed > 9007199254740991 { trb__cliInvalidValue(" + strconv.Quote(field.Name) + ", " + raw + ") }",
			name + " := int(" + name + "Parsed)",
		}, Value: name}
	case cliapp.FloatValue:
		return cliParsedScalar{Lines: []string{
			name + ", " + name + "Err := strconv.ParseFloat(" + raw + ", 64)",
			"if " + name + "Err != nil { trb__cliInvalidValue(" + strconv.Quote(field.Name) + ", " + raw + ") }",
		}, Value: name}
	case cliapp.BooleanValue:
		return cliParsedScalar{Lines: []string{
			name + ", " + name + "Err := strconv.ParseBool(" + raw + ")",
			"if " + name + "Err != nil { trb__cliInvalidValue(" + strconv.Quote(field.Name) + ", " + raw + ") }",
		}, Value: name}
	default:
		return cliParsedScalar{Lines: []string{name + " := " + raw}, Value: name}
	}
}

func cliGoScalarType(kind cliapp.ValueKind) string {
	switch kind {
	case cliapp.StringValue:
		return "string"
	case cliapp.IntegerValue:
		return "int"
	case cliapp.FloatValue:
		return "float64"
	case cliapp.BooleanValue:
		return "bool"
	default:
		return "any"
	}
}

func (g *generator) cliRecordExpression(record cliapp.Record, fields []cliConstructField, extra cliConstructField) string {
	if extra.Name != "" {
		fields = append(fields, extra)
	}
	sort.Slice(fields, func(i, j int) bool { return fields[i].Order < fields[j].Order })
	qualifier := g.cliQualifier(record.ModulePath)
	if record.Defaults {
		arguments := make([]string, 0, len(fields)*2)
		if g.execution != nil && g.execution.RecordDefault(record.ModulePath, record.Name) {
			arguments = append(arguments, "__trbScope")
		}
		for _, field := range fields {
			arguments = append(arguments, field.Value)
			if field.HasDefault {
				arguments = append(arguments, field.Provided)
			}
		}
		return qualifier + goRecordConstructorName(record.Name) + "(" + strings.Join(arguments, ", ") + ")"
	}
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		items = append(items, goIdentifier(field.Name, true)+": "+field.Value)
	}
	return qualifier + goIdentifier(record.Name, true) + "{" + strings.Join(items, ", ") + "}"
}

func (g *generator) cliTypeName(modulePath, name string) string {
	return g.cliQualifier(modulePath) + goIdentifier(name, true)
}

func (g *generator) cliQualifier(modulePath string) string {
	if modulePath == "" || g.currentDirectory() == moduleDirectory(modulePath) {
		return ""
	}
	reference := &ir.Reference{Package: modulePath, Symbol: "__trb_cli_schema"}
	if alias := g.referenceAlias(reference); alias != "" {
		return alias + "."
	}
	return ""
}

func moduleDirectory(modulePath string) string {
	index := strings.LastIndexByte(modulePath, '/')
	if index < 0 {
		return ""
	}
	return modulePath[:index]
}

func (g *generator) cliSpecLiteral(schema cliapp.Schema) string {
	fields := make([]string, len(schema.Root.Fields))
	for index, field := range schema.Root.Fields {
		fields[index] = cliFieldLiteral("root."+field.Name, field)
	}
	commands := make([]string, len(schema.Commands))
	for index, command := range schema.Commands {
		var commandFields []string
		if command.Payload != nil {
			commandFields = make([]string, len(command.Payload.Fields))
			for fieldIndex, field := range command.Payload.Fields {
				commandFields[fieldIndex] = cliFieldLiteral(cliCommandKeyPrefix(index)+"."+field.Name, field)
			}
		}
		commands[index] = "{Name: " + strconv.Quote(command.Name) + ", About: " + strconv.Quote(command.About) + ", Fields: []trb__cliField{" + strings.Join(commandFields, ", ") + "}}"
	}
	return "trb__cliSpec{Fields: []trb__cliField{" + strings.Join(fields, ", ") + "}, Commands: []trb__cliCommand{" + strings.Join(commands, ", ") + "}}"
}

func cliCommandKeyPrefix(index int) string {
	return "command." + strconv.Itoa(index)
}

func cliFieldLiteral(key string, field cliapp.Field) string {
	return "{Key: " + strconv.Quote(key) +
		", Name: " + strconv.Quote(field.Name) +
		", Long: " + strconv.Quote(field.Long) +
		", Short: " + strconv.Quote(field.Short) +
		", About: " + strconv.Quote(field.About) +
		", ValueName: " + strconv.Quote(field.ValueName) +
		", Positional: " + strconv.FormatBool(field.Positional) +
		", Boolean: " + strconv.FormatBool(field.Kind == cliapp.BooleanValue) +
		", Repeated: " + strconv.FormatBool(field.Repeated) +
		", Required: " + strconv.FormatBool(field.Required) + "}"
}

func (g *generator) cliRuntimeSupport() {
	g.line("type trb__cliField struct { Key, Name, Long, Short, About, ValueName string; Positional, Boolean, Repeated, Required bool }")
	g.line("type trb__cliCommand struct { Name, About string; Fields []trb__cliField }")
	g.line("type trb__cliSpec struct { Fields []trb__cliField; Commands []trb__cliCommand }")
	g.line("type trb__cliParsed struct { Values map[string][]string; Provided map[string]bool; Command string }")
	g.b.WriteByte('\n')
	g.line("func trb__cliParse(args []string, name string, version *string, about *string, spec trb__cliSpec) trb__cliParsed {")
	g.indent++
	g.line("result := trb__cliParsed{Values: map[string][]string{}, Provided: map[string]bool{}}")
	g.line("fields := spec.Fields")
	g.line("position := 0")
	g.line("positionalOnly := false")
	g.line("var command *trb__cliCommand")
	g.line("for index := 0; index < len(args); index++ {")
	g.indent++
	g.line("argument := args[index]")
	g.line("if !positionalOnly && (argument == \"--help\" || argument == \"-h\") { trb__cliPrintHelp(name, version, about, spec, command); os.Exit(0) }")
	g.line("if !positionalOnly && argument == \"--version\" && version != nil { fmt.Fprintln(os.Stdout, name+\" \"+*version); os.Exit(0) }")
	g.line("if !positionalOnly && argument == \"--\" { positionalOnly = true; continue }")
	g.line("if !positionalOnly && result.Command == \"\" && len(spec.Commands) > 0 && !strings.HasPrefix(argument, \"-\") {")
	g.indent++
	g.line("command = trb__cliFindCommand(spec.Commands, argument)")
	g.line("if command == nil { trb__cliFail(name, \"unknown command \"+strconv.Quote(argument)) }")
	g.line("result.Command = command.Name; fields = command.Fields; position = 0; positionalOnly = false; continue")
	g.indent--
	g.line("}")
	g.line("if !positionalOnly && strings.HasPrefix(argument, \"--\") {")
	g.indent++
	g.line("parts := strings.SplitN(strings.TrimPrefix(argument, \"--\"), \"=\", 2)")
	g.line("field := trb__cliFindLong(fields, parts[0])")
	g.line("if field == nil { trb__cliFail(name, \"unknown option --\"+parts[0]) }")
	g.line("value := \"true\"")
	g.line("if len(parts) == 2 { value = parts[1] } else if !field.Boolean { if index+1 >= len(args) { trb__cliFail(name, \"option --\"+field.Long+\" requires a value\") }; index++; value = args[index] }")
	g.line("result.Values[field.Key] = append(result.Values[field.Key], value); result.Provided[field.Key] = true; continue")
	g.indent--
	g.line("}")
	g.line("if !positionalOnly && strings.HasPrefix(argument, \"-\") && argument != \"-\" {")
	g.indent++
	g.line("short := strings.TrimPrefix(argument, \"-\")")
	g.line("field := trb__cliFindShort(fields, short)")
	g.line("if field == nil { trb__cliFail(name, \"unknown option -\"+short) }")
	g.line("value := \"true\"")
	g.line("if !field.Boolean { if index+1 >= len(args) { trb__cliFail(name, \"option -\"+field.Short+\" requires a value\") }; index++; value = args[index] }")
	g.line("result.Values[field.Key] = append(result.Values[field.Key], value); result.Provided[field.Key] = true; continue")
	g.indent--
	g.line("}")
	g.line("field := trb__cliPositional(fields, position)")
	g.line("if field == nil { trb__cliFail(name, \"unexpected argument \"+strconv.Quote(argument)) }")
	g.line("result.Values[field.Key] = append(result.Values[field.Key], argument); result.Provided[field.Key] = true; position++")
	g.indent--
	g.line("}")
	g.line("if len(spec.Commands) > 0 && result.Command == \"\" { trb__cliFail(name, \"a command is required\") }")
	g.line("trb__cliRequire(name, spec.Fields, result.Provided)")
	g.line("if command != nil { trb__cliRequire(name, command.Fields, result.Provided) }")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.cliRuntimeHelpers()
}

func (g *generator) cliRuntimeHelpers() {
	g.line("func trb__cliFindCommand(commands []trb__cliCommand, name string) *trb__cliCommand { for index := range commands { if commands[index].Name == name { return &commands[index] } }; return nil }")
	g.line("func trb__cliFindLong(fields []trb__cliField, name string) *trb__cliField { for index := range fields { if !fields[index].Positional && fields[index].Long == name { return &fields[index] } }; return nil }")
	g.line("func trb__cliFindShort(fields []trb__cliField, name string) *trb__cliField { for index := range fields { if !fields[index].Positional && fields[index].Short == name { return &fields[index] } }; return nil }")
	g.line("func trb__cliPositional(fields []trb__cliField, position int) *trb__cliField { for index := range fields { if fields[index].Positional { if position == 0 { return &fields[index] }; position-- } }; return nil }")
	g.line("func trb__cliRequire(name string, fields []trb__cliField, provided map[string]bool) { for _, field := range fields { if field.Required && !provided[field.Key] { if field.Positional { trb__cliFail(name, \"missing argument \"+field.Name) } else { trb__cliFail(name, \"missing option --\"+field.Long) } } } }")
	g.line("func trb__cliInvalidValue(field string, value string) { fmt.Fprintln(os.Stderr, \"error: invalid value \"+strconv.Quote(value)+\" for \"+field); os.Exit(2) }")
	g.line("func trb__cliFail(name string, message string) { fmt.Fprintln(os.Stderr, \"error: \"+message); fmt.Fprintln(os.Stderr, \"Try '\"+name+\" --help' for more information.\"); os.Exit(2) }")
	g.line("func trb__cliPrintHelp(name string, version *string, about *string, spec trb__cliSpec, command *trb__cliCommand) {")
	g.indent++
	g.line("fields := spec.Fields; commandName := \"\"; description := about")
	g.line("if command != nil { fields = command.Fields; commandName = \" \"+command.Name; if command.About != \"\" { value := command.About; description = &value } }")
	g.line("options := false; positionals := false; for _, field := range fields { options = options || !field.Positional; positionals = positionals || field.Positional }")
	g.line("fmt.Fprint(os.Stdout, \"Usage: \"+name+commandName)")
	g.line("if options { fmt.Fprint(os.Stdout, \" [OPTIONS]\") }")
	g.line("if command == nil && len(spec.Commands) > 0 { fmt.Fprint(os.Stdout, \" <COMMAND>\") }")
	g.line("for _, field := range fields { if field.Positional { if field.Required { fmt.Fprint(os.Stdout, \" <\"+field.ValueName+\">\") } else { fmt.Fprint(os.Stdout, \" [\"+field.ValueName+\"]\") } } }")
	g.line("fmt.Fprintln(os.Stdout); if description != nil && *description != \"\" { fmt.Fprintln(os.Stdout); fmt.Fprintln(os.Stdout, *description) }")
	g.line("if positionals { fmt.Fprintln(os.Stdout); fmt.Fprintln(os.Stdout, \"Arguments:\"); for _, field := range fields { if !field.Positional { continue }; label := \"<\"+field.ValueName+\">\"; if !field.Required { label = \"[\"+field.ValueName+\"]\" }; fmt.Fprintf(os.Stdout, \"  %-24s %s\\n\", label, field.About) } }")
	g.line("if options || version != nil { fmt.Fprintln(os.Stdout); fmt.Fprintln(os.Stdout, \"Options:\"); for _, field := range fields { if field.Positional { continue }; label := \"    --\"+field.Long; if field.Short != \"\" { label = \"-\"+field.Short+\", --\"+field.Long }; if !field.Boolean { label += \" <\"+field.ValueName+\">\" }; if field.Repeated { label += \"...\" }; fmt.Fprintf(os.Stdout, \"  %-24s %s\\n\", label, field.About) }; fmt.Fprintln(os.Stdout, \"  -h, --help               Print help\"); if version != nil { fmt.Fprintln(os.Stdout, \"  --version                Print version\") } } else { fmt.Fprintln(os.Stdout); fmt.Fprintln(os.Stdout, \"Options:\"); fmt.Fprintln(os.Stdout, \"  -h, --help               Print help\") }")
	g.line("if command == nil && len(spec.Commands) > 0 { fmt.Fprintln(os.Stdout); fmt.Fprintln(os.Stdout, \"Commands:\"); for _, item := range spec.Commands { fmt.Fprintf(os.Stdout, \"  %-24s %s\\n\", item.Name, item.About) } }")
	g.indent--
	g.line("}")
}

func (g *generator) cliApplicationFailureRuntimeSupport() {
	typeName := g.cliApplicationFailureTypeName()
	g.line("type " + typeName + " string")
	g.line("func (failure " + typeName + ") TrbCLIApplicationFailure() string { return string(failure) }")
}

func (g *generator) cliApplicationFailureBoundarySupport() {
	g.requireImport("fmt", "")
	g.requireImport("os", "")
	name := g.cliApplicationFailureBoundaryName()
	g.line("func " + name + "() {")
	g.indent++
	g.line("recovered := recover()")
	g.line("if recovered == nil { return }")
	g.line("failure, ok := recovered.(interface{ TrbCLIApplicationFailure() string })")
	g.line("if !ok { panic(recovered) }")
	g.line("fmt.Fprintln(os.Stderr, failure.TrbCLIApplicationFailure())")
	g.line("os.Exit(1)")
	g.indent--
	g.line("}")
}

func (g *generator) cliApplicationFailureBoundaryName() string {
	return "trb__cliApplicationFailureBoundary_" + naming.PrivateSuffix("cli-application-failure-boundary:"+g.modulePath)
}

func (g *generator) cliApplicationFailureTypeName() string {
	return "trb__cliApplicationFailure_" + naming.PrivateSuffix("cli-application-failure:"+g.modulePath)
}
