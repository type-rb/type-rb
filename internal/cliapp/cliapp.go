// Package cliapp owns the target-independent schema for trb/platform/go/cli applications.
package cliapp

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/packageextension"
)

const (
	PackageName     = "trb/platform/go/cli"
	ModulePath      = "trb/platform/go/cli/index"
	ProjectProvider = "trb.cli.schema"
)

type TypeReference struct {
	ModulePath string
	Name       string
}

type InvocationRequest struct {
	ModulePath string
	Offset     int
	Root       TypeReference
	Span       packageextension.SourceSpan
}

type ValueKind string

const (
	StringValue  ValueKind = "string"
	IntegerValue ValueKind = "integer"
	FloatValue   ValueKind = "float"
	BooleanValue ValueKind = "boolean"
)

type Field struct {
	Name        string
	Long        string
	Short       string
	About       string
	ValueName   string
	Kind        ValueKind
	Positional  bool
	Required    bool
	HasDefault  bool
	Nullable    bool
	ModulePath  string
	TypeName    string
	SourceOrder int
}

type Record struct {
	ModulePath string
	Name       string
	Fields     []Field
	Defaults   bool
}

type Command struct {
	Name       string
	About      string
	MemberName string
	Enum       TypeReference
	Payload    *Record
}

type Schema struct {
	Root            Record
	SubcommandField string
	SubcommandOrder int
	SubcommandEnum  TypeReference
	Commands        []Command
}

type Invocation struct {
	ModulePath string
	Offset     int
	Schema     Schema
}

type Manifest struct {
	Invocations []Invocation
}

func (*Manifest) ExtensionName() string { return ProjectProvider }

func ManifestFrom(extensions []ir.Extension) *Manifest {
	for _, extension := range extensions {
		if manifest, ok := extension.(*Manifest); ok {
			return manifest
		}
	}
	return nil
}

func (m *Manifest) Invocation(modulePath string, offset int) (*Invocation, bool) {
	if m == nil {
		return nil, false
	}
	for index := range m.Invocations {
		invocation := &m.Invocations[index]
		if invocation.ModulePath == modulePath && invocation.Offset == offset {
			return invocation, true
		}
	}
	return nil, false
}

func (m *Manifest) InvocationIndex(modulePath string, offset int) (int, *Invocation, bool) {
	if m == nil {
		return 0, nil, false
	}
	for index := range m.Invocations {
		invocation := &m.Invocations[index]
		if invocation.ModulePath == modulePath && invocation.Offset == offset {
			return index, invocation, true
		}
	}
	return 0, nil, false
}

type Issue struct {
	ModulePath string
	Message    string
	Span       packageextension.SourceSpan
}

type catalog struct {
	records map[TypeReference]packageextension.ProjectRecord
	enums   map[TypeReference]packageextension.ProjectEnum
}

func Analyze(input packageextension.ProjectDeclarationInput, requests []InvocationRequest) (*Manifest, []Issue) {
	catalog := catalog{records: map[TypeReference]packageextension.ProjectRecord{}, enums: map[TypeReference]packageextension.ProjectEnum{}}
	for _, module := range input.Modules {
		for _, record := range module.Records {
			catalog.records[TypeReference{ModulePath: module.ModulePath, Name: record.Name}] = record
		}
		for _, enum := range module.Enums {
			catalog.enums[TypeReference{ModulePath: module.ModulePath, Name: enum.Name}] = enum
		}
	}
	manifest := &Manifest{}
	var issues []Issue
	for _, request := range requests {
		schema, schemaIssues := catalog.schema(request.Root)
		for _, issue := range schemaIssues {
			if issue.ModulePath == "" {
				issue.ModulePath = request.Root.ModulePath
			}
			if zeroSpan(issue.Span) {
				issue.Span = request.Span
			}
			issues = append(issues, issue)
		}
		if len(schemaIssues) == 0 {
			manifest.Invocations = append(manifest.Invocations, Invocation{ModulePath: request.ModulePath, Offset: request.Offset, Schema: schema})
		}
	}
	sort.Slice(manifest.Invocations, func(i, j int) bool {
		if manifest.Invocations[i].ModulePath != manifest.Invocations[j].ModulePath {
			return manifest.Invocations[i].ModulePath < manifest.Invocations[j].ModulePath
		}
		return manifest.Invocations[i].Offset < manifest.Invocations[j].Offset
	})
	return manifest, issues
}

func (c catalog) schema(rootReference TypeReference) (Schema, []Issue) {
	root, ok := c.records[rootReference]
	if !ok {
		return Schema{}, []Issue{{Message: fmt.Sprintf("trb/platform/go/cli run root %s must be a record", rootReference.Name)}}
	}
	schema := Schema{Root: Record{ModulePath: rootReference.ModulePath, Name: root.Name}}
	var issues []Issue
	for index, field := range root.Fields {
		metadata, metadataIssues := cliMetadata(field.Attributes, field.Span)
		issues = append(issues, metadataIssues...)
		if metadata.kind == "subcommand" {
			if schema.SubcommandField != "" {
				issues = append(issues, Issue{Message: "trb/platform/go/cli root record may declare only one @cli(:subcommand) field", Span: field.Span})
				continue
			}
			reference, found := typeReference(field.Type)
			enum, enumFound := c.enums[reference]
			if !found || !enumFound {
				issues = append(issues, Issue{Message: fmt.Sprintf("trb/platform/go/cli subcommand field %s must use a payload enum", field.Name), Span: field.Span})
				continue
			}
			schema.SubcommandField = field.Name
			schema.SubcommandOrder = index
			schema.SubcommandEnum = reference
			commands, commandIssues := c.commands(reference, enum)
			schema.Commands = commands
			issues = append(issues, commandIssues...)
			continue
		}
		converted, fieldIssues := scalarField(rootReference.ModulePath, field, metadata, index)
		schema.Root.Fields = append(schema.Root.Fields, converted)
		schema.Root.Defaults = schema.Root.Defaults || field.HasDefault
		issues = append(issues, fieldIssues...)
	}
	if schema.SubcommandField != "" {
		for _, field := range schema.Root.Fields {
			if field.Positional {
				issues = append(issues, Issue{Message: "trb/platform/go/cli root fields used with subcommands must be options", Span: root.Span})
				break
			}
		}
	}
	issues = append(issues, validateFields(schema.Root.Fields)...)
	for index := range issues {
		if issues[index].ModulePath == "" {
			issues[index].ModulePath = rootReference.ModulePath
		}
	}
	return schema, issues
}

func (c catalog) commands(reference TypeReference, enum packageextension.ProjectEnum) ([]Command, []Issue) {
	commands := make([]Command, 0, len(enum.Members))
	var issues []Issue
	seen := map[string]bool{}
	for _, member := range enum.Members {
		metadata, metadataIssues := cliMetadata(member.Attributes, member.Span)
		issues = append(issues, metadataIssues...)
		if metadata.kind != "" {
			issues = append(issues, Issue{Message: "enum member @cli accepts metadata but no positional kind", Span: member.Span})
		}
		name := metadata.name
		if name == "" {
			name = kebab(member.Name)
		}
		command := Command{Name: name, About: metadata.about, MemberName: member.Name, Enum: reference}
		if seen[name] {
			issues = append(issues, Issue{Message: fmt.Sprintf("duplicate trb/platform/go/cli subcommand %q", name), Span: member.Span})
		}
		seen[name] = true
		switch len(member.Parameters) {
		case 0:
		case 1:
			payloadReference, found := typeReference(member.Parameters[0].Type)
			record, recordFound := c.records[payloadReference]
			if !found || !recordFound {
				issues = append(issues, Issue{Message: fmt.Sprintf("trb/platform/go/cli subcommand %s payload must be one record", member.Name), Span: member.Span})
				break
			}
			payload := &Record{ModulePath: payloadReference.ModulePath, Name: record.Name}
			for index, field := range record.Fields {
				fieldMetadata, fieldMetadataIssues := cliMetadata(field.Attributes, field.Span)
				issues = append(issues, fieldMetadataIssues...)
				if fieldMetadata.kind == "subcommand" {
					issues = append(issues, Issue{Message: "nested trb/platform/go/cli subcommands are not supported in the initial contract", Span: field.Span})
					continue
				}
				converted, fieldIssues := scalarField(payloadReference.ModulePath, field, fieldMetadata, index)
				payload.Fields = append(payload.Fields, converted)
				payload.Defaults = payload.Defaults || field.HasDefault
				issues = append(issues, fieldIssues...)
			}
			issues = append(issues, validateFields(payload.Fields)...)
			command.Payload = payload
		default:
			issues = append(issues, Issue{Message: fmt.Sprintf("trb/platform/go/cli subcommand %s must be payloadless or contain one record payload", member.Name), Span: member.Span})
		}
		commands = append(commands, command)
	}
	for index := range issues {
		if issues[index].ModulePath == "" {
			issues[index].ModulePath = reference.ModulePath
		}
	}
	return commands, issues
}

type metadata struct {
	kind      string
	name      string
	short     string
	about     string
	valueName string
}

func cliMetadata(attributes []packageextension.ProjectAttribute, span packageextension.SourceSpan) (metadata, []Issue) {
	result := metadata{}
	var issues []Issue
	for _, attribute := range attributes {
		if attribute.Name != "cli" {
			continue
		}
		for _, argument := range attribute.Arguments {
			if argument.Name == "" {
				if argument.Value.Kind != "symbol" || result.kind != "" {
					issues = append(issues, Issue{Message: "@cli positional metadata must be one of :option or :subcommand", Span: attribute.Span})
					continue
				}
				result.kind = argument.Value.Name
				if result.kind != "option" && result.kind != "subcommand" {
					issues = append(issues, Issue{Message: "@cli positional metadata must be one of :option or :subcommand", Span: attribute.Span})
				}
				continue
			}
			value, ok := staticString(argument.Value)
			if !ok {
				issues = append(issues, Issue{Message: fmt.Sprintf("@cli %s must be a string literal", argument.Name), Span: argument.Span})
				continue
			}
			switch argument.Name {
			case "name", "long":
				result.name = value
			case "short":
				result.short = value
			case "about":
				result.about = value
			case "value_name":
				result.valueName = value
			default:
				issues = append(issues, Issue{Message: fmt.Sprintf("unknown @cli metadata %s", argument.Name), Span: argument.Span})
			}
		}
	}
	return result, issues
}

func scalarField(modulePath string, field packageextension.ProjectRecordField, metadata metadata, order int) (Field, []Issue) {
	result := Field{
		Name: field.Name, About: metadata.about, Short: metadata.short, ValueName: metadata.valueName,
		Required: !field.HasDefault && !field.Type.Resolved.Nullable, HasDefault: field.HasDefault,
		Nullable: field.Type.Resolved.Nullable, ModulePath: modulePath, SourceOrder: order,
	}
	if metadata.kind == "" {
		result.Positional = true
	} else if metadata.kind != "option" {
		return result, []Issue{{Message: fmt.Sprintf("record field %s has unsupported @cli kind %s", field.Name, metadata.kind), Span: field.Span}}
	}
	result.Long = metadata.name
	if result.Long == "" {
		result.Long = kebab(field.Name)
	}
	if result.ValueName == "" {
		result.ValueName = strings.ToUpper(strings.ReplaceAll(field.Name, "-", "_"))
	}
	typ := field.Type.Resolved
	result.TypeName = typ.Name
	switch typ.Kind {
	case "string":
		result.Kind = StringValue
	case "int":
		result.Kind = IntegerValue
	case "float":
		result.Kind = FloatValue
	case "bool":
		result.Kind = BooleanValue
	default:
		return result, []Issue{{Message: fmt.Sprintf("trb/platform/go/cli field %s must use String, Integer, Float, or Boolean in the initial contract", field.Name), Span: field.Span}}
	}
	if result.Short != "" && len([]rune(result.Short)) != 1 {
		return result, []Issue{{Message: fmt.Sprintf("trb/platform/go/cli short option for %s must contain one character", field.Name), Span: field.Span}}
	}
	return result, nil
}

func validateFields(fields []Field) []Issue {
	var issues []Issue
	seenLong := map[string]bool{}
	seenShort := map[string]bool{}
	generatedLong := map[string]bool{"help": true, "version": true}
	generatedShort := map[string]bool{"h": true}
	seenOptionalPositional := false
	for _, field := range fields {
		if field.Positional {
			if !field.Required {
				seenOptionalPositional = true
			} else if seenOptionalPositional {
				issues = append(issues, Issue{Message: "required trb/platform/go/cli positional fields cannot follow optional positional fields"})
			}
			continue
		}
		if seenLong[field.Long] {
			issues = append(issues, Issue{Message: fmt.Sprintf("duplicate trb/platform/go/cli option --%s", field.Long)})
		}
		if generatedLong[field.Long] {
			issues = append(issues, Issue{Message: fmt.Sprintf("trb/platform/go/cli option --%s conflicts with a generated option", field.Long)})
		}
		seenLong[field.Long] = true
		if field.Short != "" {
			if seenShort[field.Short] {
				issues = append(issues, Issue{Message: fmt.Sprintf("duplicate trb/platform/go/cli option -%s", field.Short)})
			}
			if generatedShort[field.Short] {
				issues = append(issues, Issue{Message: fmt.Sprintf("trb/platform/go/cli option -%s conflicts with a generated option", field.Short)})
			}
			seenShort[field.Short] = true
		}
	}
	return issues
}

func typeReference(use packageextension.ProjectTypeUse) (TypeReference, bool) {
	typ := use.Resolved
	if typ.Definition == nil || typ.Definition.ModulePath == "" || typ.Name == "" || typ.Nullable || len(typ.Arguments) > 0 {
		return TypeReference{}, false
	}
	return TypeReference{ModulePath: typ.Definition.ModulePath, Name: typ.Name}, true
}

func staticString(value packageextension.ProjectValue) (string, bool) {
	if value.Kind != "string" {
		return "", false
	}
	parsed, err := strconv.Unquote(value.Raw)
	return parsed, err == nil
}

func kebab(value string) string {
	var result strings.Builder
	for index, r := range value {
		if r == '_' {
			result.WriteByte('-')
			continue
		}
		if index > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('-')
		}
		result.WriteRune(r)
	}
	return strings.ToLower(result.String())
}

func zeroSpan(span packageextension.SourceSpan) bool {
	return span.Start == (packageextension.SourcePosition{}) && span.End == (packageextension.SourcePosition{})
}
