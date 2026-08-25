package web

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

const OpenAPIVersion = "3.1.0"

// OpenAPIOptions contains document metadata owned by the application rather
// than by an individual endpoint contract.
type OpenAPIOptions struct {
	Title   string
	Version string
}

// OpenAPIDocument is the portable OpenAPI representation produced from the
// trb/web endpoint catalog. The model intentionally covers only fields emitted
// by TypeRB so additions can remain deliberate and deterministic.
type OpenAPIDocument struct {
	OpenAPI    string                     `json:"openapi"`
	Info       OpenAPIInfo                `json:"info"`
	Paths      map[string]OpenAPIPathItem `json:"paths"`
	Components *OpenAPIComponents         `json:"components,omitempty"`
}

type OpenAPIInfo struct {
	Title   string `json:"title"`
	Version string `json:"version"`
}

type OpenAPIPathItem map[string]OpenAPIOperation

type OpenAPIOperation struct {
	OperationID string                     `json:"operationId"`
	Parameters  []OpenAPIParameter         `json:"parameters,omitempty"`
	RequestBody *OpenAPIRequestBody        `json:"requestBody,omitempty"`
	Responses   map[string]OpenAPIResponse `json:"responses"`
}

type OpenAPIParameter struct {
	Name     string        `json:"name"`
	In       string        `json:"in"`
	Required bool          `json:"required,omitempty"`
	Schema   OpenAPISchema `json:"schema"`
}

type OpenAPIRequestBody struct {
	Required bool                        `json:"required,omitempty"`
	Content  map[string]OpenAPIMediaType `json:"content"`
}

type OpenAPIResponse struct {
	Description string                      `json:"description"`
	Content     map[string]OpenAPIMediaType `json:"content,omitempty"`
}

type OpenAPIMediaType struct {
	Schema OpenAPISchema `json:"schema"`
}

type OpenAPIComponents struct {
	Schemas map[string]OpenAPISchema `json:"schemas"`
}

type OpenAPISchema struct {
	Ref                  string                   `json:"$ref,omitempty"`
	Type                 string                   `json:"type,omitempty"`
	Format               string                   `json:"format,omitempty"`
	Pattern              string                   `json:"pattern,omitempty"`
	Enum                 []any                    `json:"enum,omitempty"`
	Properties           map[string]OpenAPISchema `json:"properties,omitempty"`
	Required             []string                 `json:"required,omitempty"`
	Items                *OpenAPISchema           `json:"items,omitempty"`
	AdditionalProperties *OpenAPISchema           `json:"additionalProperties,omitempty"`
	AnyOf                []OpenAPISchema          `json:"anyOf,omitempty"`
	Minimum              *int64                   `json:"minimum,omitempty"`
	Maximum              *int64                   `json:"maximum,omitempty"`
}

// OpenAPIIssue is a source-located application contract problem. OpenAPI-only
// restrictions are reported when generation is requested and do not make an
// otherwise portable TypeRB project fail trb check.
type OpenAPIIssue struct {
	ModulePath string
	Message    string
	Span       packageextension.SourceSpan
}

type openAPIGenerator struct {
	aliases        map[string]packageextension.ProjectTypeAlias
	newtypes       map[string]packageextension.ProjectNewtype
	records        map[string]packageextension.ProjectRecord
	enums          map[string]packageextension.ProjectEnum
	components     map[string]OpenAPISchema
	componentOwner map[string]string
	componentState map[string]string
	issues         []OpenAPIIssue
}

// BuildOpenAPI turns package-owned endpoint and declaration protocols into an
// OpenAPI 3.1 document without reading compiler AST or backend IR.
func BuildOpenAPI(catalog EndpointCatalog, input packageextension.ProjectDeclarationInput, options OpenAPIOptions) (OpenAPIDocument, []OpenAPIIssue, error) {
	document := OpenAPIDocument{
		OpenAPI: OpenAPIVersion,
		Info:    OpenAPIInfo{Title: strings.TrimSpace(options.Title), Version: strings.TrimSpace(options.Version)},
		Paths:   map[string]OpenAPIPathItem{},
	}
	if err := ValidateEndpointCatalog(catalog); err != nil {
		return document, nil, err
	}
	if err := validateEndpointInput(input); err != nil {
		return document, nil, err
	}
	if document.Info.Title == "" {
		return document, nil, fmt.Errorf("OpenAPI title is missing")
	}
	if document.Info.Version == "" {
		return document, nil, fmt.Errorf("OpenAPI API version is missing")
	}
	generator := newOpenAPIGenerator(input)
	if len(catalog.Endpoints) == 0 {
		generator.issue("", packageextension.SourceSpan{}, "trb/web project declares no endpoint contracts")
		return document, generator.issues, nil
	}

	operationIDs := map[string]bool{}
	for _, endpoint := range catalog.Endpoints {
		if operationIDs[endpoint.Name] {
			generator.issue(endpoint.ModulePath, endpoint.Span, fmt.Sprintf("OpenAPI operationId %q is declared more than once", endpoint.Name))
			continue
		}
		operationIDs[endpoint.Name] = true
		path, parameters, requestBody, ok := generator.request(endpoint)
		if !ok {
			continue
		}
		operation := OpenAPIOperation{
			OperationID: endpoint.Name,
			Parameters:  parameters,
			RequestBody: requestBody,
			Responses:   map[string]OpenAPIResponse{},
		}
		for _, response := range endpoint.Responses {
			description := http.StatusText(response.Status)
			if description == "" {
				description = "Response"
			}
			openAPIResponse := OpenAPIResponse{Description: description}
			if statusForbidsBody(response.Status) {
				if !generator.unitType(endpoint.ModulePath, response.Type) {
					generator.issue(endpoint.ModulePath, response.Type.Span,
						fmt.Sprintf("OpenAPI response status %d cannot declare a response body; use Unit", response.Status))
					continue
				}
			} else if !generator.unitType(endpoint.ModulePath, response.Type) {
				schema, schemaOK := generator.jsonSchema(endpoint.ModulePath, response.Type)
				if !schemaOK {
					continue
				}
				openAPIResponse.Content = jsonContent(schema)
			}
			operation.Responses[strconv.Itoa(response.Status)] = openAPIResponse
		}
		if len(operation.Responses) != len(endpoint.Responses) {
			continue
		}
		method := strings.ToLower(endpoint.Method)
		if document.Paths[path] == nil {
			document.Paths[path] = OpenAPIPathItem{}
		}
		if _, duplicate := document.Paths[path][method]; duplicate {
			generator.issue(endpoint.ModulePath, endpoint.Span,
				fmt.Sprintf("OpenAPI route %s %s is declared more than once", endpoint.Method, path))
			continue
		}
		document.Paths[path][method] = operation
	}
	if len(generator.components) != 0 {
		document.Components = &OpenAPIComponents{Schemas: generator.components}
	}
	if len(generator.issues) != 0 {
		sortOpenAPIIssues(generator.issues)
		return document, generator.issues, nil
	}
	return document, nil, nil
}

func newOpenAPIGenerator(input packageextension.ProjectDeclarationInput) *openAPIGenerator {
	result := &openAPIGenerator{
		aliases: map[string]packageextension.ProjectTypeAlias{}, newtypes: map[string]packageextension.ProjectNewtype{},
		records: map[string]packageextension.ProjectRecord{}, enums: map[string]packageextension.ProjectEnum{},
		components: map[string]OpenAPISchema{}, componentOwner: map[string]string{}, componentState: map[string]string{},
	}
	for _, module := range input.Modules {
		for _, alias := range module.TypeAliases {
			result.aliases[typeIdentity(module.ModulePath, alias.Name)] = alias
		}
		for _, newtype := range module.Newtypes {
			result.newtypes[typeIdentity(module.ModulePath, newtype.Name)] = newtype
		}
		for _, record := range module.Records {
			result.records[typeIdentity(module.ModulePath, record.Name)] = record
		}
		for _, enum := range module.Enums {
			result.enums[typeIdentity(module.ModulePath, enum.Name)] = enum
		}
	}
	return result
}

func (g *openAPIGenerator) request(endpoint EndpointContract) (string, []OpenAPIParameter, *OpenAPIRequestBody, bool) {
	path, routeParameters, pathOK := openAPIPath(endpoint.Path)
	if !pathOK {
		g.issue(endpoint.ModulePath, endpoint.Span,
			fmt.Sprintf("OpenAPI generation does not support catch-all route %s yet", endpoint.Path))
		return "", nil, nil, false
	}
	if endpoint.Input == nil {
		if len(routeParameters) != 0 {
			g.issue(endpoint.ModulePath, endpoint.Span,
				fmt.Sprintf("OpenAPI endpoint %s must declare input<T>() with params for route %s", endpoint.Name, endpoint.Path))
			return "", nil, nil, false
		}
		return path, nil, nil, true
	}
	envelopeModule, envelope, ok := g.recordForUse(endpoint.ModulePath, *endpoint.Input, "endpoint input")
	if !ok {
		return "", nil, nil, false
	}
	fields := map[string]packageextension.ProjectRecordField{}
	for _, field := range envelope.Fields {
		if _, duplicate := fields[field.Name]; duplicate {
			g.issue(envelopeModule, field.Span, fmt.Sprintf("endpoint input record %s declares %s more than once", envelope.Name, field.Name))
			return "", nil, nil, false
		}
		switch field.Name {
		case "params", "query", "body":
			fields[field.Name] = field
		default:
			g.issue(envelopeModule, field.Span,
				fmt.Sprintf("endpoint input record %s has unsupported field %q; expected params, query, or body", envelope.Name, field.Name))
			return "", nil, nil, false
		}
	}

	var parameters []OpenAPIParameter
	paramsField, hasParams := fields["params"]
	if len(routeParameters) != 0 && !hasParams {
		g.issue(endpoint.ModulePath, endpoint.Input.Span,
			fmt.Sprintf("OpenAPI endpoint %s input is missing params for route %s", endpoint.Name, endpoint.Path))
		return "", nil, nil, false
	}
	if hasParams {
		paramsModule, paramsRecord, paramsOK := g.recordForUse(envelopeModule, paramsField.Type, "path parameter")
		if !paramsOK {
			return "", nil, nil, false
		}
		byName := map[string]packageextension.ProjectRecordField{}
		for _, field := range paramsRecord.Fields {
			byName[field.Name] = field
		}
		if len(byName) != len(routeParameters) {
			g.issue(paramsModule, paramsField.Span,
				fmt.Sprintf("OpenAPI path parameter record %s must match route %s exactly", paramsRecord.Name, endpoint.Path))
			return "", nil, nil, false
		}
		for _, name := range routeParameters {
			field, exists := byName[name]
			if !exists {
				g.issue(paramsModule, paramsField.Span,
					fmt.Sprintf("OpenAPI path parameter record %s is missing route parameter %q", paramsRecord.Name, name))
				return "", nil, nil, false
			}
			schema, _, scalarOK := g.parameterSchema(paramsModule, field.Type, false)
			if !scalarOK {
				return "", nil, nil, false
			}
			parameters = append(parameters, OpenAPIParameter{Name: name, In: "path", Required: true, Schema: schema})
		}
	}

	if queryField, exists := fields["query"]; exists {
		queryModule, queryRecord, queryOK := g.recordForUse(envelopeModule, queryField.Type, "query parameter")
		if !queryOK {
			return "", nil, nil, false
		}
		for _, field := range queryRecord.Fields {
			schema, required, scalarOK := g.parameterSchema(queryModule, field.Type, true)
			if !scalarOK {
				return "", nil, nil, false
			}
			parameters = append(parameters, OpenAPIParameter{Name: field.Name, In: "query", Required: required, Schema: schema})
		}
	}

	var requestBody *OpenAPIRequestBody
	if bodyField, exists := fields["body"]; exists {
		schema, bodyOK := g.jsonSchema(envelopeModule, bodyField.Type)
		if !bodyOK {
			return "", nil, nil, false
		}
		requestBody = &OpenAPIRequestBody{Required: true, Content: jsonContent(schema)}
	}
	return path, parameters, requestBody, true
}

func (g *openAPIGenerator) recordForUse(modulePath string, use packageextension.ProjectTypeUse, label string) (string, packageextension.ProjectRecord, bool) {
	modulePath, typ, ok := g.expandAliases(modulePath, preferredType(use), map[string]bool{})
	if !ok {
		g.issue(modulePath, use.Span, fmt.Sprintf("OpenAPI %s type %s contains a recursive or generic alias", label, displayProjectType(preferredType(use))))
		return modulePath, packageextension.ProjectRecord{}, false
	}
	if typ.Nullable || typ.Kind != "named" || len(typ.Arguments) != 0 {
		g.issue(modulePath, use.Span, fmt.Sprintf("OpenAPI %s type %s must be a non-nullable record", label, displayProjectType(typ)))
		return modulePath, packageextension.ProjectRecord{}, false
	}
	definitionModule := definitionModule(modulePath, typ)
	record, exists := g.records[typeIdentity(definitionModule, typ.Name)]
	if !exists || len(record.TypeParameters) != 0 {
		g.issue(modulePath, use.Span, fmt.Sprintf("OpenAPI %s type %s must be a non-generic record", label, displayProjectType(typ)))
		return modulePath, packageextension.ProjectRecord{}, false
	}
	return definitionModule, record, true
}

func (g *openAPIGenerator) parameterSchema(modulePath string, use packageextension.ProjectTypeUse, allowArray bool) (OpenAPISchema, bool, bool) {
	normalized, shapeOK := g.parameterShape(modulePath, preferredType(use), allowArray, map[string]bool{})
	if !shapeOK {
		kind := "path"
		if allowArray {
			kind = "query"
		}
		g.issue(modulePath, use.Span,
			fmt.Sprintf("OpenAPI %s parameter has unsupported type %s", kind, displayProjectType(preferredType(use))))
		return OpenAPISchema{}, false, false
	}
	if !allowArray && normalized.Nullable {
		g.issue(modulePath, use.Span,
			fmt.Sprintf("OpenAPI path parameter type %s must be non-nullable", displayProjectType(preferredType(use))))
		return OpenAPISchema{}, false, false
	}
	schema, ok := g.jsonSchema(modulePath, use)
	if !ok {
		return OpenAPISchema{}, false, false
	}
	required := !normalized.Nullable && normalized.Kind != "array"
	return schema, required, true
}

func (g *openAPIGenerator) parameterShape(modulePath string, typ packageextension.Type, allowArray bool, visiting map[string]bool) (packageextension.Type, bool) {
	modulePath, typ, ok := g.expandAliases(modulePath, typ, visiting)
	if !ok {
		return typ, false
	}
	base := typ
	base.Nullable = false
	switch base.Kind {
	case "bool", "int", "float", "string":
		return typ, true
	case "array":
		if !allowArray || len(base.Arguments) != 1 || base.Arguments[0].Nullable || base.Arguments[0].Kind == "array" {
			return typ, false
		}
		_, itemOK := g.parameterShape(modulePath, base.Arguments[0], false, cloneOpenAPISet(visiting))
		return typ, itemOK
	case "named":
		if _, timeScalar := g.timeScalar(base); timeScalar {
			return typ, true
		}
		identity := typeIdentity(definitionModule(modulePath, base), base.Name)
		if newtype, exists := g.newtypes[identity]; exists {
			if visiting[identity] {
				return typ, false
			}
			visiting[identity] = true
			target := preferredType(newtype.Target)
			target.Nullable = target.Nullable || typ.Nullable
			return g.parameterShape(definitionModule(modulePath, base), target, allowArray, visiting)
		}
		if enum, exists := g.enums[identity]; exists && rawEnumValues(enum) != nil {
			return typ, true
		}
	}
	return typ, false
}

func (g *openAPIGenerator) jsonSchema(modulePath string, use packageextension.ProjectTypeUse) (OpenAPISchema, bool) {
	return g.schemaForType(modulePath, preferredType(use), use.Span)
}

func (g *openAPIGenerator) schemaForType(modulePath string, typ packageextension.Type, span packageextension.SourceSpan) (OpenAPISchema, bool) {
	return g.schemaForTypeWithAliases(modulePath, typ, span, map[string]bool{})
}

func (g *openAPIGenerator) schemaForTypeWithAliases(modulePath string, typ packageextension.Type, span packageextension.SourceSpan, visitingAliases map[string]bool) (OpenAPISchema, bool) {
	modulePath, typ, aliasesOK := g.expandAliases(modulePath, typ, visitingAliases)
	if !aliasesOK {
		g.issue(modulePath, span, fmt.Sprintf("OpenAPI schema type %s contains a recursive or generic alias", displayProjectType(typ)))
		return OpenAPISchema{}, false
	}
	nullable := typ.Nullable
	base := typ
	base.Nullable = false
	var schema OpenAPISchema
	switch base.Kind {
	case "bool":
		schema.Type = "boolean"
	case "int":
		minimum, maximum := int64(types.MinPortableInteger), int64(types.MaxPortableInteger)
		schema = OpenAPISchema{Type: "integer", Minimum: &minimum, Maximum: &maximum}
	case "float":
		schema = OpenAPISchema{Type: "number", Format: "double"}
	case "string":
		schema.Type = "string"
	case "array":
		if len(base.Arguments) != 1 {
			g.issue(modulePath, span, "OpenAPI JSON schema requires Array<T>")
			return OpenAPISchema{}, false
		}
		item, ok := g.schemaForTypeWithAliases(modulePath, base.Arguments[0], span, cloneOpenAPISet(visitingAliases))
		if !ok {
			return OpenAPISchema{}, false
		}
		schema = OpenAPISchema{Type: "array", Items: &item}
	case "hash":
		if len(base.Arguments) != 2 || base.Arguments[0].Kind != "string" || base.Arguments[0].Nullable {
			g.issue(modulePath, span, "OpenAPI JSON schema requires Hash<String, V>")
			return OpenAPISchema{}, false
		}
		value, ok := g.schemaForTypeWithAliases(modulePath, base.Arguments[1], span, cloneOpenAPISet(visitingAliases))
		if !ok {
			return OpenAPISchema{}, false
		}
		schema = OpenAPISchema{Type: "object", AdditionalProperties: &value}
	case "named":
		if timeSchema, timeScalar := g.timeScalar(base); timeScalar {
			schema = timeSchema
			break
		}
		identity := typeIdentity(definitionModule(modulePath, base), base.Name)
		if len(base.Arguments) != 0 {
			g.issue(modulePath, span, fmt.Sprintf("OpenAPI schema does not support generic type %s yet", displayProjectType(base)))
			return OpenAPISchema{}, false
		}
		if _, exists := g.newtypes[identity]; exists {
			if !g.ensureComponent(identity, span) {
				return OpenAPISchema{}, false
			}
			schema.Ref = componentReference(base.Name)
		} else if _, exists := g.records[identity]; exists {
			if !g.ensureComponent(identity, span) {
				return OpenAPISchema{}, false
			}
			schema.Ref = componentReference(base.Name)
		} else if _, exists := g.enums[identity]; exists {
			if !g.ensureComponent(identity, span) {
				return OpenAPISchema{}, false
			}
			schema.Ref = componentReference(base.Name)
		} else {
			g.issue(modulePath, span, fmt.Sprintf("OpenAPI JSON schema type %s must be a record, raw-value enum, newtype, or JSON-compatible built-in type", displayProjectType(base)))
			return OpenAPISchema{}, false
		}
	default:
		g.issue(modulePath, span, fmt.Sprintf("OpenAPI JSON schema type %s is not supported", displayProjectType(base)))
		return OpenAPISchema{}, false
	}
	return nullableSchema(schema, nullable), true
}

func (g *openAPIGenerator) ensureComponent(identity string, span packageextension.SourceSpan) bool {
	if g.componentState[identity] == "done" {
		return true
	}
	if g.componentState[identity] == "failed" {
		return false
	}
	modulePath, name := splitTypeIdentity(identity)
	if g.componentState[identity] == "visiting" {
		g.issue(modulePath, span, fmt.Sprintf("recursive OpenAPI record %s is not supported yet", name))
		return false
	}
	if owner := g.componentOwner[name]; owner != "" && owner != identity {
		g.issue(modulePath, span,
			fmt.Sprintf("OpenAPI component name %s is shared by %s and %s", name, displayTypeIdentity(owner), displayTypeIdentity(identity)))
		return false
	}
	g.componentOwner[name] = identity
	g.componentState[identity] = "visiting"
	succeeded := false
	defer func() {
		if !succeeded {
			g.componentState[identity] = "failed"
		}
	}()
	var schema OpenAPISchema
	switch {
	case g.newtypes[identity].Name != "":
		definition := g.newtypes[identity]
		var ok bool
		schema, ok = g.schemaForType(modulePath, preferredType(definition.Target), definition.Span)
		if !ok {
			return false
		}
	case g.records[identity].Name != "":
		definition := g.records[identity]
		if len(definition.TypeParameters) != 0 {
			g.issue(modulePath, definition.Span, fmt.Sprintf("OpenAPI schema does not support generic record %s yet", name))
			return false
		}
		schema = OpenAPISchema{Type: "object", Properties: map[string]OpenAPISchema{}}
		seenWireNames := map[string]bool{}
		for _, field := range definition.Fields {
			wireName, ok := recordJSONName(field)
			if !ok || wireName == "" || wireName == "-" {
				g.issue(modulePath, field.Span, fmt.Sprintf("record field %s has unsupported JSON name %q", field.Name, wireName))
				return false
			}
			if seenWireNames[wireName] {
				g.issue(modulePath, field.Span, fmt.Sprintf("record %s maps more than one field to JSON name %q", name, wireName))
				return false
			}
			seenWireNames[wireName] = true
			fieldSchema, fieldOK := g.schemaForType(modulePath, preferredType(field.Type), field.Type.Span)
			if !fieldOK {
				return false
			}
			schema.Properties[wireName] = fieldSchema
			if !preferredType(field.Type).Nullable {
				schema.Required = append(schema.Required, wireName)
			}
		}
	case g.enums[identity].Name != "":
		definition := g.enums[identity]
		if len(definition.TypeParameters) != 0 {
			g.issue(modulePath, definition.Span, fmt.Sprintf("OpenAPI schema does not support generic enum %s yet", name))
			return false
		}
		values := rawEnumValues(definition)
		if values == nil {
			g.issue(modulePath, definition.Span, fmt.Sprintf("OpenAPI JSON schema enum %s must use String or Integer raw values", name))
			return false
		}
		schema.Enum = values
		if _, ok := values[0].(string); ok {
			schema.Type = "string"
		} else {
			minimum, maximum := int64(types.MinPortableInteger), int64(types.MaxPortableInteger)
			schema.Type, schema.Minimum, schema.Maximum = "integer", &minimum, &maximum
		}
	default:
		g.issue(modulePath, span, fmt.Sprintf("OpenAPI component %s has no declaration", name))
		return false
	}
	g.components[name] = schema
	g.componentState[identity] = "done"
	succeeded = true
	return true
}

func (g *openAPIGenerator) expandAliases(modulePath string, typ packageextension.Type, visiting map[string]bool) (string, packageextension.Type, bool) {
	if typ.Kind != "named" {
		return modulePath, typ, true
	}
	definitionModule := definitionModule(modulePath, typ)
	identity := typeIdentity(definitionModule, typ.Name)
	alias, exists := g.aliases[identity]
	if !exists {
		return definitionModule, typ, true
	}
	if visiting[identity] || len(alias.TypeParameters) != 0 || len(typ.Arguments) != 0 {
		return definitionModule, typ, false
	}
	visiting[identity] = true
	target := preferredType(alias.Target)
	target.Nullable = target.Nullable || typ.Nullable
	return g.expandAliases(definitionModule, target, visiting)
}

func (g *openAPIGenerator) timeScalar(typ packageextension.Type) (OpenAPISchema, bool) {
	if typ.Kind != "named" || typ.Definition == nil || typ.Definition.ImportPath != "trb/std/time" {
		return OpenAPISchema{}, false
	}
	switch typ.Name {
	case "Date":
		return OpenAPISchema{Type: "string", Format: "date"}, true
	case "Instant":
		return OpenAPISchema{Type: "string", Format: "date-time"}, true
	case "Duration":
		return OpenAPISchema{Type: "string", Format: "duration"}, true
	case "TimeOfDay":
		return OpenAPISchema{Type: "string", Pattern: `^(?:[01][0-9]|2[0-3]):[0-5][0-9](?::[0-5][0-9](?:\.[0-9]{1,9})?)?$`}, true
	case "DateTime":
		return OpenAPISchema{Type: "string", Pattern: `^[0-9]{4}-[0-9]{2}-[0-9]{2}T(?:[01][0-9]|2[0-3]):[0-5][0-9](?::[0-5][0-9](?:\.[0-9]{1,9})?)?$`}, true
	case "TimeZone":
		return OpenAPISchema{Type: "string"}, true
	default:
		return OpenAPISchema{}, false
	}
}

func (g *openAPIGenerator) unitType(modulePath string, use packageextension.ProjectTypeUse) bool {
	_, typ, ok := g.expandAliases(modulePath, preferredType(use), map[string]bool{})
	if !ok || typ.Kind != "named" || typ.Name != "Unit" || typ.Nullable || len(typ.Arguments) != 0 {
		return false
	}
	return typ.Definition == nil || typ.Definition.ImportPath == "trb/std/unit"
}

func (g *openAPIGenerator) issue(modulePath string, span packageextension.SourceSpan, message string) {
	g.issues = append(g.issues, OpenAPIIssue{ModulePath: modulePath, Span: span, Message: message})
}

func openAPIPath(path string) (string, []string, bool) {
	segments := strings.Split(path, "/")
	parameters := make([]string, 0)
	for index, segment := range segments {
		if strings.HasPrefix(segment, "*") {
			return "", nil, false
		}
		if strings.HasPrefix(segment, ":") {
			name := strings.TrimPrefix(segment, ":")
			if name == "" {
				return "", nil, false
			}
			segments[index] = "{" + name + "}"
			parameters = append(parameters, name)
		}
	}
	return strings.Join(segments, "/"), parameters, true
}

func rawEnumValues(enum packageextension.ProjectEnum) []any {
	if len(enum.Members) == 0 {
		return nil
	}
	values := make([]any, 0, len(enum.Members))
	kind := ""
	seen := map[string]bool{}
	for _, member := range enum.Members {
		if len(member.Parameters) != 0 || member.RawValue == nil {
			return nil
		}
		value := member.RawValue
		if kind == "" {
			kind = value.Kind
		}
		if value.Kind != kind {
			return nil
		}
		key := value.Kind + "\x00" + value.Raw
		if seen[key] {
			return nil
		}
		seen[key] = true
		switch value.Kind {
		case "string":
			parsed, err := strconv.Unquote(value.Raw)
			if err != nil {
				return nil
			}
			values = append(values, parsed)
		case "integer":
			parsed, err := strconv.ParseInt(strings.ReplaceAll(value.Raw, "_", ""), 10, 64)
			if err != nil || parsed < types.MinPortableInteger || parsed > types.MaxPortableInteger {
				return nil
			}
			values = append(values, parsed)
		default:
			return nil
		}
	}
	return values
}

func recordJSONName(field packageextension.ProjectRecordField) (string, bool) {
	for _, attribute := range field.Attributes {
		if attribute.Name != "json" || len(attribute.Arguments) == 0 {
			continue
		}
		value := attribute.Arguments[0].Value
		if value.Kind != "string" {
			continue
		}
		parsed, err := strconv.Unquote(value.Raw)
		if err != nil {
			return "", false
		}
		return strings.Split(parsed, ",")[0], true
	}
	return field.Name, true
}

func preferredType(use packageextension.ProjectTypeUse) packageextension.Type {
	if use.Resolved.Kind != "" {
		return use.Resolved
	}
	return use.Authored
}

func nullableSchema(schema OpenAPISchema, nullable bool) OpenAPISchema {
	if !nullable {
		return schema
	}
	return OpenAPISchema{AnyOf: []OpenAPISchema{schema, {Type: "null"}}}
}

func jsonContent(schema OpenAPISchema) map[string]OpenAPIMediaType {
	return map[string]OpenAPIMediaType{"application/json": {Schema: schema}}
}

func statusForbidsBody(status int) bool {
	return status >= 100 && status < 200 || status == 204 || status == 205 || status == 304
}

func componentReference(name string) string {
	return "#/components/schemas/" + name
}

func definitionModule(fallback string, typ packageextension.Type) string {
	if typ.Definition != nil && typ.Definition.ModulePath != "" {
		return typ.Definition.ModulePath
	}
	return fallback
}

func typeIdentity(modulePath, name string) string {
	return modulePath + "\x00" + name
}

func splitTypeIdentity(identity string) (string, string) {
	parts := strings.SplitN(identity, "\x00", 2)
	if len(parts) != 2 {
		return "", identity
	}
	return parts[0], parts[1]
}

func displayTypeIdentity(identity string) string {
	modulePath, name := splitTypeIdentity(identity)
	return modulePath + "." + name
}

func displayProjectType(typ packageextension.Type) string {
	name := typ.Name
	if name == "" {
		name = typ.Kind
	}
	if len(typ.Arguments) != 0 {
		parts := make([]string, len(typ.Arguments))
		for index, argument := range typ.Arguments {
			parts[index] = displayProjectType(argument)
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	if typ.Nullable {
		name += "?"
	}
	return name
}

func cloneOpenAPISet(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func sortOpenAPIIssues(issues []OpenAPIIssue) {
	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].ModulePath != issues[j].ModulePath {
			return issues[i].ModulePath < issues[j].ModulePath
		}
		if issues[i].Span.Start.Offset != issues[j].Span.Start.Offset {
			return issues[i].Span.Start.Offset < issues[j].Span.Start.Offset
		}
		return issues[i].Message < issues[j].Message
	})
}
