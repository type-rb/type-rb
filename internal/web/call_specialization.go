package web

import (
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
)

func init() {
	packageextension.RegisterCallProvider("trb.web.bind", specializeBind)
}

func specializeBind(request packageextension.SpecializeCallRequest) packageextension.SpecializeCallResponse {
	response := packageextension.SpecializeCallResponse{ProtocolVersion: packageextension.ProtocolVersion}
	if len(request.TypeArguments) != 1 {
		response.Issues = []packageextension.Issue{{Message: "Context#bind<T>() requires exactly one type argument"}}
		return response
	}
	input := request.TypeArguments[0]
	if input.Kind != "named" || input.Nullable || input.Record == nil {
		response.Issues = []packageextension.Issue{{Message: fmt.Sprintf("endpoint input type %s must be a non-nullable record", displaySpecializationType(input))}}
		return response
	}
	if len(input.Record.Fields) == 0 {
		response.Issues = []packageextension.Issue{{Message: fmt.Sprintf("endpoint input record %s must declare at least one of params, query, or body", input.Name)}}
		return response
	}
	fields := map[string]packageextension.Type{}
	for _, field := range input.Record.Fields {
		if _, duplicate := fields[field.Name]; duplicate {
			response.Issues = []packageextension.Issue{{Message: fmt.Sprintf("endpoint input record %s declares %s more than once", input.Name, field.Name)}}
			return response
		}
		switch field.Name {
		case "params", "query", "body":
			fields[field.Name] = field.Type
		default:
			response.Issues = []packageextension.Issue{{Message: fmt.Sprintf("endpoint input record %s has unsupported field %q; expected params, query, or body", input.Name, field.Name)}}
			return response
		}
	}

	generator := bindSpecializationGenerator{
		callID: request.CallSite.ID, modulePath: request.CallSite.ModulePath,
		imports: map[string]map[string]bool{}, input: input, fields: fields,
	}
	helper := "__trb_specialize_bind_" + request.CallSite.ID
	response.GeneratedSource = &packageextension.GeneratedSource{ID: request.Provider + "#" + request.CallSite.ID, Source: generator.source(helper)}
	response.Replacement = &packageextension.Replacement{Callee: helper, Arguments: []packageextension.ValueSource{packageextension.ReceiverValue}}
	response.RequiredImports = generator.requiredImports()
	return response
}

type bindSpecializationGenerator struct {
	callID     string
	modulePath string
	imports    map[string]map[string]bool
	input      packageextension.Type
	fields     map[string]packageextension.Type
}

func (g *bindSpecializationGenerator) source(helper string) string {
	g.requireImport("trb/std/result", "Result")
	g.requireImport("trb/web", "Context")
	g.requireImport("trb/web", "EndpointInputError")
	inputType := g.typeSource(g.input)
	errorType := "EndpointInputError"
	resultType := "Result<" + inputType + ", " + errorType + ">"

	var body strings.Builder
	values := make([]string, 0, len(g.fields))
	for _, name := range []string{"params", "query", "body"} {
		fieldType, exists := g.fields[name]
		if !exists {
			continue
		}
		variable := "__trb_specialize_" + name + "_" + g.callID
		errorName := "__trb_specialize_error_" + name + "_" + g.callID
		operation := "__trb_specialize_context_" + g.callID + ".params<" + g.typeSource(fieldType) + ">()"
		variant := "Params"
		if name == "query" {
			operation = "__trb_specialize_context_" + g.callID + ".request.query<" + g.typeSource(fieldType) + ">()"
			variant = "Query"
		} else if name == "body" {
			operation = "__trb_specialize_context_" + g.callID + ".request.json<" + g.typeSource(fieldType) + ">()"
			variant = "Body"
		}
		body.WriteString("\t" + variable + " := " + operation + " catch |" + errorName + "|\n")
		body.WriteString("\t\treturn " + resultType + "::Err(" + errorType + "::" + variant + "(" + errorName + "))\n")
		body.WriteString("\tend\n")
		values = append(values, name+": "+variable)
	}
	body.WriteString("\treturn " + resultType + "::Ok(" + inputType + ".new(" + strings.Join(values, ", ") + "))\n")
	return "def " + helper + "(__trb_specialize_context_" + g.callID + ": Context): " + resultType + "\n" + body.String() + "end\n"
}

func (g *bindSpecializationGenerator) typeSource(typ packageextension.Type) string {
	if typ.Kind == "function" && len(typ.Arguments) > 0 {
		parts := make([]string, len(typ.Arguments)-1)
		for index := range parts {
			parts[index] = g.typeSource(typ.Arguments[index])
		}
		value := "(" + strings.Join(parts, ", ") + ") -> " + g.typeSource(typ.Arguments[len(typ.Arguments)-1])
		if typ.Nullable {
			return "(" + value + ")?"
		}
		return value
	}
	if typ.Kind == "union" {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = g.typeSource(argument)
		}
		value := strings.Join(parts, " | ")
		if typ.Nullable {
			return "(" + value + ")?"
		}
		return value
	}
	name := typ.Name
	if name == "" {
		name = typ.Kind
	}
	if typ.Definition != nil && typ.Definition.ModulePath != "" && typ.Definition.ModulePath != g.modulePath && typ.Definition.ImportPath != "" {
		g.requireImport(typ.Definition.ImportPath, name)
	}
	if len(typ.Arguments) > 0 {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = g.typeSource(argument)
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	if typ.Nullable {
		name += "?"
	}
	return name
}

func (g *bindSpecializationGenerator) requireImport(path, symbol string) {
	if path == "" || symbol == "" {
		return
	}
	if g.imports[path] == nil {
		g.imports[path] = map[string]bool{}
	}
	g.imports[path][symbol] = true
}

func (g *bindSpecializationGenerator) requiredImports() []packageextension.RequiredImport {
	paths := make([]string, 0, len(g.imports))
	for path := range g.imports {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	result := make([]packageextension.RequiredImport, 0, len(paths))
	for _, path := range paths {
		symbols := make([]string, 0, len(g.imports[path]))
		for symbol := range g.imports[path] {
			symbols = append(symbols, symbol)
		}
		sort.Strings(symbols)
		result = append(result, packageextension.RequiredImport{Path: path, Symbols: symbols})
	}
	return result
}

func displaySpecializationType(typ packageextension.Type) string {
	if typ.Kind == "union" {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = displaySpecializationType(argument)
		}
		value := strings.Join(parts, " | ")
		if typ.Nullable {
			return "(" + value + ")?"
		}
		return value
	}
	name := typ.Name
	if name == "" {
		name = typ.Kind
	}
	if len(typ.Arguments) > 0 {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = displaySpecializationType(argument)
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	if typ.Nullable {
		name += "?"
	}
	return name
}
