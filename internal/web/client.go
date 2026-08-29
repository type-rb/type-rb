package web

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/packageextension"
)

const DefaultBrowserClientName = "ApiClient"

var browserClientClassName = regexp.MustCompile(`^[A-Z][A-Za-z0-9_]*$`)

// BrowserClientOptions selects application-owned names in the generated
// TypeRB source. The transport and endpoint surface remain package-owned.
type BrowserClientOptions struct {
	Name string
}

// BrowserClientIssue is a source-located endpoint contract problem that only
// prevents browser client generation. It does not make trb check fail.
type BrowserClientIssue struct {
	ModulePath string
	Message    string
	Span       packageextension.SourceSpan
}

type browserClientGenerator struct {
	schema        *openAPIGenerator
	name          string
	imports       map[string]map[string]bool
	rootImports   map[string]bool
	generated     map[string]bool
	methodNames   map[string]string
	issues        []BrowserClientIssue
	issueKeys     map[string]bool
	querySequence int
	usesNoBody    bool
	usesJSONBody  bool
	usesQuery     bool
	usesPathValue bool
}

type browserClientInput struct {
	paramsModule string
	params       *packageextension.ProjectRecord
	queryModule  string
	query        *packageextension.ProjectRecord
	body         *packageextension.ProjectRecordField
}

// BuildBrowserClient generates a checked TypeRB facade over the existing
// TypeScript browser HttpClient. Endpoint records stay in their authored
// shared modules; the generated source imports rather than copies them.
func BuildBrowserClient(catalog EndpointCatalog, input packageextension.ProjectDeclarationInput, options BrowserClientOptions) (string, []BrowserClientIssue, error) {
	if err := ValidateEndpointCatalog(catalog); err != nil {
		return "", nil, err
	}
	if err := validateEndpointInput(input); err != nil {
		return "", nil, err
	}
	name := strings.TrimSpace(options.Name)
	if name == "" {
		name = DefaultBrowserClientName
	}
	if !browserClientClassName.MatchString(name) {
		return "", nil, fmt.Errorf("browser client name %q must be a TypeRB class identifier", name)
	}
	// Client generation and OpenAPI intentionally share one JSON wire subset.
	// Validate it once before source generation so the two tools cannot silently
	// disagree about an endpoint contract.
	_, schemaIssues, err := BuildOpenAPI(catalog, input, OpenAPIOptions{Title: "TypeRB browser client", Version: "1"})
	if err != nil {
		return "", nil, err
	}
	if len(schemaIssues) != 0 {
		issues := make([]BrowserClientIssue, len(schemaIssues))
		for index, issue := range schemaIssues {
			issues[index] = BrowserClientIssue{
				ModulePath: issue.ModulePath,
				Message:    strings.ReplaceAll(issue.Message, "OpenAPI", "browser client"),
				Span:       issue.Span,
			}
		}
		return "", issues, nil
	}

	generator := &browserClientGenerator{
		schema: newOpenAPIGenerator(input), name: name,
		imports: map[string]map[string]bool{}, rootImports: map[string]bool{}, generated: map[string]bool{name: true}, methodNames: map[string]string{}, issueKeys: map[string]bool{},
	}
	endpoints := append([]EndpointContract(nil), catalog.Endpoints...)
	sort.Slice(endpoints, func(left, right int) bool {
		if endpoints[left].Path != endpoints[right].Path {
			return endpoints[left].Path < endpoints[right].Path
		}
		if endpoints[left].Method != endpoints[right].Method {
			return endpoints[left].Method < endpoints[right].Method
		}
		return endpoints[left].Name < endpoints[right].Name
	})

	var enums strings.Builder
	var methods strings.Builder
	for _, endpoint := range endpoints {
		resultName := endpoint.Name + "Result"
		if generator.generated[resultName] {
			generator.issue(endpoint, endpoint.Span, fmt.Sprintf("generated browser client type %s is declared more than once", resultName))
			continue
		}
		generator.generated[resultName] = true
		methodName := browserClientMethodName(endpoint.Name)
		if methodName == "initialize" {
			generator.issue(endpoint, endpoint.Span, fmt.Sprintf("endpoint %s generates reserved browser client method initialize; rename the endpoint contract", endpoint.Name))
			continue
		}
		if previous := generator.methodNames[methodName]; previous != "" {
			generator.issue(endpoint, endpoint.Span, fmt.Sprintf("endpoint names %s and %s both generate browser client method %s", previous, endpoint.Name, methodName))
			continue
		}
		generator.methodNames[methodName] = endpoint.Name

		clientInput, ok := generator.input(endpoint)
		if !ok {
			continue
		}
		enums.WriteString(generator.resultEnum(endpoint, resultName))
		methods.WriteString(generator.method(endpoint, resultName, methodName, clientInput))
	}
	if len(generator.issues) != 0 {
		sortBrowserClientIssues(generator.issues)
		return "", generator.issues, nil
	}

	generator.requireImport("trb/http", "Headers")
	generator.requireImport("trb/http", "HttpMethod")
	for _, symbol := range []string{"HttpClient", "RequestError", "RequestErrorKind", "Response"} {
		generator.requireImport("trb/platform/typescript/browser", symbol)
	}
	if generator.usesNoBody {
		generator.requireImport("trb/platform/typescript/browser", "NoBody")
	}
	if generator.usesJSONBody {
		generator.requireImport("trb/platform/typescript/browser", "json_body")
	}
	generator.requireRootImport("trb/std/result")
	if generator.usesQuery {
		generator.requireRootImport("trb/std/url")
	}
	if generator.usesPathValue {
		generator.requireRootImport("trb/std/url")
	}
	for generated := range generator.generated {
		for path, symbols := range generator.imports {
			if symbols[generated] {
				generator.issues = append(generator.issues, BrowserClientIssue{
					Message: fmt.Sprintf("generated browser client type %s conflicts with import from %s; choose another client name or rename the endpoint contract", generated, path),
				})
			}
		}
	}
	if len(generator.issues) != 0 {
		sortBrowserClientIssues(generator.issues)
		return "", generator.issues, nil
	}

	var source strings.Builder
	source.WriteString("# Generated by trb web client. Do not edit.\n\n")
	source.WriteString(generator.importSource())
	source.WriteByte('\n')
	source.WriteString(enums.String())
	source.WriteString("class " + name + "\n")
	source.WriteString("\treadonly @_http: HttpClient\n\n")
	source.WriteString("\tdef initialize(http: HttpClient)\n")
	source.WriteString("\t\t@_http = http\n")
	source.WriteString("\t\treturn\n")
	source.WriteString("\tend\n")
	source.WriteString(methods.String())
	source.WriteString("end\n")
	formatted, diagnostics := formatter.Format([]byte(source.String()))
	if len(diagnostics) != 0 {
		return "", nil, fmt.Errorf("format generated browser client: %s", diagnostics[0].Message)
	}
	return string(formatted), nil, nil
}

func (g *browserClientGenerator) input(endpoint EndpointContract) (browserClientInput, bool) {
	if endpoint.Input == nil {
		return browserClientInput{}, true
	}
	modulePath, envelope, ok := g.schema.recordForUse(endpoint.ModulePath, *endpoint.Input, "endpoint input")
	if !ok {
		g.copySchemaIssues(endpoint)
		return browserClientInput{}, false
	}
	result := browserClientInput{}
	for index := range envelope.Fields {
		field := envelope.Fields[index]
		switch field.Name {
		case "params":
			fieldModule, record, fieldOK := g.schema.recordForUse(modulePath, field.Type, "path input")
			if !fieldOK {
				g.copySchemaIssues(endpoint)
				return browserClientInput{}, false
			}
			result.paramsModule, result.params = fieldModule, &record
		case "query":
			fieldModule, record, fieldOK := g.schema.recordForUse(modulePath, field.Type, "query input")
			if !fieldOK {
				g.copySchemaIssues(endpoint)
				return browserClientInput{}, false
			}
			result.queryModule, result.query = fieldModule, &record
		case "body":
			copy := field
			result.body = &copy
		}
	}
	return result, true
}

func (g *browserClientGenerator) resultEnum(endpoint EndpointContract, resultName string) string {
	responses := append([]EndpointResponse(nil), endpoint.Responses...)
	sort.Slice(responses, func(left, right int) bool { return responses[left].Status < responses[right].Status })
	var source strings.Builder
	source.WriteString("enum " + resultName + "\n")
	for _, response := range responses {
		bodyType := "NoBody"
		if !statusForbidsBody(response.Status) {
			bodyType = g.typeSource(endpoint, endpoint.ModulePath, response.Type.Authored, response.Type.Span)
		} else {
			g.usesNoBody = true
		}
		source.WriteString("\tStatus" + strconv.Itoa(response.Status) + "(response: Response<" + bodyType + ">)\n")
	}
	source.WriteString("end\n\n")
	return source.String()
}

func (g *browserClientGenerator) method(endpoint EndpointContract, resultName, methodName string, input browserClientInput) string {
	resultType := "Result<" + resultName + ", RequestError>"
	var source strings.Builder
	source.WriteString("\n\tdef " + methodName + "(")
	if endpoint.Input != nil {
		source.WriteString("input: " + g.typeSource(endpoint, endpoint.ModulePath, endpoint.Input.Authored, endpoint.Input.Span) + ", ")
	}
	source.WriteString("*, headers: Headers = Headers.new(), timeout_milliseconds: Integer? = nil): " + resultType + "\n")
	source.WriteString("\t\tpath := " + g.pathSource(endpoint, input) + "\n")
	if input.query != nil {
		g.usesQuery = true
		source.WriteString("\t\tmut query: Array<URL::QueryParameter> := []\n")
		for _, field := range input.query.Fields {
			g.writeQueryField(&source, input.queryModule, field.Type, "input.query."+field.Name, field.Name, "\t\t")
		}
	}
	if input.body != nil {
		g.usesJSONBody = true
		source.WriteString("\t\tbody := try json_body(input.body)\n")
	}
	source.WriteString("\t\traw := try @_http.request(\n")
	source.WriteString("\t\t\tpath,\n")
	source.WriteString("\t\t\tmethod: HttpMethod." + strings.ToLower(endpoint.Method) + "(),\n")
	if input.query != nil {
		source.WriteString("\t\t\tquery: query,\n")
	}
	source.WriteString("\t\t\theaders: headers,\n")
	if input.body != nil {
		source.WriteString("\t\t\tbody: body,\n")
	}
	source.WriteString("\t\t\ttimeout_milliseconds: timeout_milliseconds,\n")
	source.WriteString("\t\t)\n")
	source.WriteString("\t\tcase raw.status\n")
	responses := append([]EndpointResponse(nil), endpoint.Responses...)
	sort.Slice(responses, func(left, right int) bool { return responses[left].Status < responses[right].Status })
	for _, response := range responses {
		status := strconv.Itoa(response.Status)
		source.WriteString("\t\twhen " + status + "\n")
		if statusForbidsBody(response.Status) {
			source.WriteString("\t\t\tdecoded := try raw.no_body()\n")
		} else {
			typeName := g.typeSource(endpoint, endpoint.ModulePath, response.Type.Authored, response.Type.Span)
			source.WriteString("\t\t\tdecoded := try raw.json<" + typeName + ">()\n")
		}
		source.WriteString("\t\t\treturn " + resultType + "::Ok(" + resultName + "::Status" + status + "(decoded))\n")
	}
	source.WriteString("\t\telse\n")
	source.WriteString("\t\t\treturn " + resultType + "::Err(RequestError.new(\n")
	source.WriteString("\t\t\t\tRequestErrorKind::Contract,\n")
	source.WriteString("\t\t\t\t\"unexpected response status \" + raw.status.to_s() + \" for " + endpoint.Name + "\",\n")
	source.WriteString("\t\t\t\traw,\n")
	source.WriteString("\t\t\t))\n")
	source.WriteString("\t\tend\n")
	source.WriteString("\tend\n")
	return source.String()
}

func (g *browserClientGenerator) pathSource(endpoint EndpointContract, input browserClientInput) string {
	if input.params == nil {
		return strconv.Quote(endpoint.Path)
	}
	fields := map[string]packageextension.ProjectRecordField{}
	for _, field := range input.params.Fields {
		fields[field.Name] = field
	}
	segments := strings.Split(strings.TrimPrefix(endpoint.Path, "/"), "/")
	parts := []string{strconv.Quote("/")}
	for index, segment := range segments {
		if index > 0 {
			parts = append(parts, strconv.Quote("/"))
		}
		if strings.HasPrefix(segment, ":") {
			g.usesPathValue = true
			name := strings.TrimPrefix(segment, ":")
			field := fields[name]
			parts = append(parts, "URL.encode_component("+g.parameterValue(input.paramsModule, field.Type, "input.params."+field.Name)+")")
		} else {
			parts = append(parts, strconv.Quote(segment))
		}
	}
	return strings.Join(parts, " + ")
}

func (g *browserClientGenerator) writeQueryField(source *strings.Builder, modulePath string, use packageextension.ProjectTypeUse, expression, wireName, indent string) {
	g.writeQueryType(source, modulePath, preferredType(use), expression, wireName, indent, map[string]bool{})
}

func (g *browserClientGenerator) writeQueryType(source *strings.Builder, modulePath string, typ packageextension.Type, expression, wireName, indent string, visiting map[string]bool) {
	modulePath, typ, ok := g.schema.expandAliases(modulePath, typ, visiting)
	if !ok {
		return
	}
	if typ.Nullable {
		sequence := g.querySequence
		g.querySequence++
		local := "query_value_" + strconv.Itoa(sequence)
		source.WriteString(indent + local + " := " + expression + "\n")
		source.WriteString(indent + "if " + local + " != nil\n")
		typ.Nullable = false
		g.writeQueryType(source, modulePath, typ, local, wireName, indent+"\t", visiting)
		source.WriteString(indent + "end\n")
		return
	}
	if typ.Kind == "named" {
		identity := typeIdentity(definitionModule(modulePath, typ), definitionName(typ))
		if newtype, exists := g.schema.newtypes[identity]; exists {
			if visiting[identity] {
				return
			}
			visiting[identity] = true
			g.writeQueryType(source, definitionModule(modulePath, typ), preferredType(newtype.Target), expression+".value()", wireName, indent, visiting)
			return
		}
	}
	if typ.Kind == "array" && len(typ.Arguments) == 1 {
		sequence := g.querySequence
		g.querySequence++
		item := "query_item_" + strconv.Itoa(sequence)
		source.WriteString(indent + expression + ".each do |" + item + "|\n")
		g.writeQueryType(source, modulePath, typ.Arguments[0], item, wireName, indent+"\t", cloneOpenAPISet(visiting))
		source.WriteString(indent + "end\n")
		return
	}
	source.WriteString(indent + "query.push(URL::QueryParameter.new(name: " + strconv.Quote(wireName) + ", value: " + g.parameterTypeValue(modulePath, typ, expression, visiting) + "))\n")
}

func (g *browserClientGenerator) parameterValue(modulePath string, use packageextension.ProjectTypeUse, expression string) string {
	return g.parameterTypeValue(modulePath, preferredType(use), expression, map[string]bool{})
}

func (g *browserClientGenerator) parameterTypeValue(modulePath string, typ packageextension.Type, expression string, visiting map[string]bool) string {
	modulePath, typ, ok := g.schema.expandAliases(modulePath, typ, visiting)
	if !ok {
		return expression + ".to_s()"
	}
	typ.Nullable = false
	if typ.Kind == "named" {
		identity := typeIdentity(definitionModule(modulePath, typ), definitionName(typ))
		if newtype, exists := g.schema.newtypes[identity]; exists && !visiting[identity] {
			visiting[identity] = true
			return g.parameterTypeValue(definitionModule(modulePath, typ), preferredType(newtype.Target), expression+".value()", visiting)
		}
		if enum, exists := g.schema.enums[identity]; exists && rawEnumValues(enum) != nil {
			if enum.Members[0].RawValue.Kind == "string" {
				return expression + ".raw_value()"
			}
			return expression + ".raw_value().to_s()"
		}
	}
	if typ.Name == "String" || typ.Kind == "string" {
		return expression
	}
	return expression + ".to_s()"
}

func (g *browserClientGenerator) typeSource(endpoint EndpointContract, modulePath string, typ packageextension.Type, span packageextension.SourceSpan) string {
	if typ.Kind == "union" {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = g.typeSource(endpoint, modulePath, argument, span)
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
	if typ.Definition != nil && typ.Definition.ImportPath != "" {
		g.requireImport(typ.Definition.ImportPath, name)
	} else if typ.Definition != nil && typ.Definition.ModulePath != "" {
		g.issue(endpoint, span, fmt.Sprintf("browser client type %s must be imported by the endpoint from a shared module; route-local contract types cannot be generated", name))
	}
	if len(typ.Arguments) != 0 {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = g.typeSource(endpoint, modulePath, argument, span)
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	if typ.Nullable {
		name += "?"
	}
	return name
}

func (g *browserClientGenerator) importSource() string {
	paths := make([]string, 0, len(g.imports)+len(g.rootImports))
	seen := map[string]bool{}
	for path := range g.imports {
		paths = append(paths, path)
		seen[path] = true
	}
	for path := range g.rootImports {
		if !seen[path] {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths)
	var source strings.Builder
	for _, path := range paths {
		if g.rootImports[path] {
			source.WriteString("import " + path + "\n")
		}
		symbols := make([]string, 0, len(g.imports[path]))
		for symbol := range g.imports[path] {
			symbols = append(symbols, symbol)
		}
		sort.Strings(symbols)
		if len(symbols) > 0 {
			source.WriteString("import { " + strings.Join(symbols, ", ") + " } from " + path + "\n")
		}
	}
	return source.String()
}

func (g *browserClientGenerator) requireRootImport(path string) {
	if path != "" {
		g.rootImports[path] = true
	}
}

func (g *browserClientGenerator) requireImport(path, symbol string) {
	if path == "" || symbol == "" {
		return
	}
	if g.imports[path] == nil {
		g.imports[path] = map[string]bool{}
	}
	g.imports[path][symbol] = true
}

func (g *browserClientGenerator) issue(endpoint EndpointContract, span packageextension.SourceSpan, message string) {
	g.addIssue(BrowserClientIssue{ModulePath: endpoint.ModulePath, Span: span, Message: message})
}

func (g *browserClientGenerator) copySchemaIssues(endpoint EndpointContract) {
	for _, issue := range g.schema.issues {
		modulePath := issue.ModulePath
		if modulePath == "" {
			modulePath = endpoint.ModulePath
		}
		g.addIssue(BrowserClientIssue{
			ModulePath: modulePath,
			Span:       issue.Span,
			Message:    strings.ReplaceAll(issue.Message, "OpenAPI", "browser client"),
		})
	}
	g.schema.issues = nil
}

func (g *browserClientGenerator) addIssue(issue BrowserClientIssue) {
	key := issue.ModulePath + "\x00" + strconv.Itoa(issue.Span.Start.Offset) + "\x00" + issue.Message
	if g.issueKeys[key] {
		return
	}
	g.issueKeys[key] = true
	g.issues = append(g.issues, issue)
}

func browserClientMethodName(name string) string {
	name = strings.TrimSuffix(name, "Endpoint")
	if name == "" {
		return "endpoint"
	}
	var result []rune
	input := []rune(name)
	for index, current := range input {
		if unicode.IsUpper(current) {
			previousLower := index > 0 && (unicode.IsLower(input[index-1]) || unicode.IsDigit(input[index-1]))
			nextLower := index+1 < len(input) && unicode.IsLower(input[index+1])
			if len(result) > 0 && (previousLower || nextLower) && result[len(result)-1] != '_' {
				result = append(result, '_')
			}
			result = append(result, unicode.ToLower(current))
		} else {
			result = append(result, current)
		}
	}
	return string(result)
}

func sortBrowserClientIssues(issues []BrowserClientIssue) {
	sort.SliceStable(issues, func(left, right int) bool {
		if issues[left].ModulePath != issues[right].ModulePath {
			return issues[left].ModulePath < issues[right].ModulePath
		}
		if issues[left].Span.Start.Offset != issues[right].Span.Start.Offset {
			return issues[left].Span.Start.Offset < issues[right].Span.Start.Offset
		}
		return issues[left].Message < issues[right].Message
	})
}
