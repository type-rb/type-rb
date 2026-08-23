package packageextension

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

const DeclarationAdapterProtocolVersion = 2

// DeclarationAdapterCatalog is the versioned, mode-independent declaration
// input consumed by a native-ecosystem adapter. It contains semantic data
// only; an adapter validates target-specific bridge kinds before importing the
// declarations into the compiler.
type DeclarationAdapterCatalog struct {
	ProtocolVersion int                                 `json:"protocolVersion"`
	Modules         map[string]DeclarationAdapterModule `json:"modules"`
}

type DeclarationAdapterModule struct {
	Exports map[string]DeclarationAdapterExport `json:"exports,omitempty"`
	Records map[string]DeclarationAdapterExport `json:"records,omitempty"`
}

type DeclarationAdapterExport struct {
	Kind              string                              `json:"kind"`
	Type              DeclarationAdapterType              `json:"type"`
	AliasTarget       *DeclarationAdapterType             `json:"aliasTarget,omitempty"`
	Parameters        []DeclarationAdapterType            `json:"parameters,omitempty"`
	Required          int                                 `json:"required,omitempty"`
	Variadic          bool                                `json:"variadic,omitempty"`
	TypeParameters    []string                            `json:"typeParameters,omitempty"`
	Fields            []DeclarationAdapterField           `json:"fields,omitempty"`
	Members           map[string]DeclarationAdapterExport `json:"members,omitempty"`
	InstanceMembers   map[string]DeclarationAdapterExport `json:"instanceMembers,omitempty"`
	ClassMembers      map[string]DeclarationAdapterExport `json:"classMembers,omitempty"`
	UnsupportedFields map[string]string                   `json:"unsupportedFields,omitempty"`
}

type DeclarationAdapterField struct {
	Name     string                 `json:"name"`
	Type     DeclarationAdapterType `json:"type"`
	Optional bool                   `json:"optional,omitempty"`
}

// DeclarationAdapterType is a backend-independent semantic TypeRB type. A
// result bridge is adapter metadata on a function boundary rather than a
// second TypeRB failure model.
type DeclarationAdapterType struct {
	Kind         string                          `json:"kind"`
	Name         string                          `json:"name,omitempty"`
	Arguments    []DeclarationAdapterType        `json:"arguments,omitempty"`
	ResultBridge *DeclarationAdapterResultBridge `json:"resultBridge,omitempty"`
	Nullable     bool                            `json:"nullable,omitempty"`
	Readonly     bool                            `json:"readonly,omitempty"`
}

type DeclarationAdapterResultBridge struct {
	Kind  string                 `json:"kind"`
	Error DeclarationAdapterType `json:"error"`
}

func ValidateDeclarationAdapterCatalog(catalog DeclarationAdapterCatalog) error {
	if catalog.ProtocolVersion != DeclarationAdapterProtocolVersion {
		return fmt.Errorf("unsupported declaration adapter protocol version %d", catalog.ProtocolVersion)
	}
	if catalog.Modules == nil {
		return fmt.Errorf("declaration adapter modules are required")
	}
	for _, moduleName := range sortedDeclarationAdapterKeys(catalog.Modules) {
		module := catalog.Modules[moduleName]
		if strings.TrimSpace(moduleName) == "" {
			return fmt.Errorf("declaration adapter contains an empty module name")
		}
		for _, name := range sortedDeclarationAdapterKeys(module.Exports) {
			exported := module.Exports[name]
			if err := validateDeclarationAdapterExport(moduleName, "export", name, exported, true); err != nil {
				return err
			}
		}
		for _, name := range sortedDeclarationAdapterKeys(module.Records) {
			if _, exists := module.Exports[name]; exists {
				return fmt.Errorf("declaration adapter module %s declares %s as both an export and a supporting record", moduleName, name)
			}
			record := module.Records[name]
			if err := validateDeclarationAdapterExport(moduleName, "record", name, record, false); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateDeclarationAdapterExport(moduleName, category, name string, exported DeclarationAdapterExport, allowMembers bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("declaration adapter module %s contains an empty %s name", moduleName, category)
	}
	if exported.Kind != "component" && exported.Kind != "function" && exported.Kind != "class" && exported.Kind != "interface" && exported.Kind != "record" && exported.Kind != "type_alias" {
		return fmt.Errorf("declaration adapter %s %s from %s has unsupported kind %q", category, name, moduleName, exported.Kind)
	}
	if category == "record" && exported.Kind != "record" {
		return fmt.Errorf("declaration adapter record %s from %s must use kind record", name, moduleName)
	}
	if err := validateDeclarationAdapterExportShape(exported); err != nil {
		return fmt.Errorf("declaration adapter %s %s from %s: %w", category, name, moduleName, err)
	}
	if (exported.Kind == "class" || exported.Kind == "interface" || exported.Kind == "record" || exported.Kind == "type_alias") &&
		(exported.Type.Kind != "named" || exported.Type.Name != name) {
		return fmt.Errorf("declaration adapter %s %s from %s: kind %s requires a named self type", category, name, moduleName, exported.Kind)
	}
	if err := validateDeclarationAdapterTypeParameters(exported.TypeParameters); err != nil {
		return fmt.Errorf("declaration adapter %s %s from %s: %w", category, name, moduleName, err)
	}
	if err := validateDeclarationAdapterType(exported.Type); err != nil {
		return fmt.Errorf("declaration adapter %s %s from %s: %w", category, name, moduleName, err)
	}
	if exported.Kind == "type_alias" {
		if exported.AliasTarget == nil || exported.AliasTarget.Kind == "" {
			return fmt.Errorf("declaration adapter %s %s from %s: type alias requires aliasTarget", category, name, moduleName)
		}
		if err := validateDeclarationAdapterType(*exported.AliasTarget); err != nil {
			return fmt.Errorf("declaration adapter %s %s from %s: %w", category, name, moduleName, err)
		}
	} else if exported.AliasTarget != nil {
		return fmt.Errorf("declaration adapter %s %s from %s: aliasTarget is only valid for type aliases", category, name, moduleName)
	}
	for _, parameter := range exported.Parameters {
		if err := validateDeclarationAdapterType(parameter); err != nil {
			return fmt.Errorf("declaration adapter %s %s from %s: %w", category, name, moduleName, err)
		}
	}
	if exported.Required < 0 || exported.Required > len(exported.Parameters) {
		return fmt.Errorf("declaration adapter %s %s from %s has an invalid required parameter count", category, name, moduleName)
	}
	seenFields := map[string]bool{}
	for _, field := range exported.Fields {
		if strings.TrimSpace(field.Name) == "" {
			return fmt.Errorf("declaration adapter %s %s from %s contains an empty field name", category, name, moduleName)
		}
		if seenFields[field.Name] {
			return fmt.Errorf("declaration adapter %s %s from %s contains duplicate field %s", category, name, moduleName, field.Name)
		}
		seenFields[field.Name] = true
		if err := validateDeclarationAdapterType(field.Type); err != nil {
			return fmt.Errorf("declaration adapter %s %s.%s from %s: %w", category, name, field.Name, moduleName, err)
		}
	}
	for _, fieldName := range sortedDeclarationAdapterKeys(exported.UnsupportedFields) {
		if strings.TrimSpace(fieldName) == "" || strings.TrimSpace(exported.UnsupportedFields[fieldName]) == "" {
			return fmt.Errorf("declaration adapter %s %s from %s contains an empty unsupported field or reason", category, name, moduleName)
		}
	}
	if !allowMembers && (len(exported.Members) != 0 || len(exported.InstanceMembers) != 0 || len(exported.ClassMembers) != 0) {
		return fmt.Errorf("declaration adapter record %s from %s cannot declare members", name, moduleName)
	}
	memberKinds := []struct {
		name    string
		members map[string]DeclarationAdapterExport
	}{
		{name: "member", members: exported.Members},
		{name: "instance member", members: exported.InstanceMembers},
		{name: "class member", members: exported.ClassMembers},
	}
	seenMemberNames := map[string]string{}
	for _, memberKind := range memberKinds {
		for memberName := range memberKind.members {
			if seenFields[memberName] {
				return fmt.Errorf("declaration adapter export %s from %s declares %s as both field and %s", name, moduleName, memberName, memberKind.name)
			}
			if previous, exists := seenMemberNames[memberName]; exists {
				return fmt.Errorf("declaration adapter export %s from %s declares %s as both %s and %s", name, moduleName, memberName, previous, memberKind.name)
			}
			seenMemberNames[memberName] = memberKind.name
		}
	}
	if err := validateDeclarationAdapterMembers(moduleName, name, exported.Members); err != nil {
		return err
	}
	if err := validateDeclarationAdapterMembers(moduleName, name, exported.InstanceMembers); err != nil {
		return err
	}
	if err := validateDeclarationAdapterMembers(moduleName, name, exported.ClassMembers); err != nil {
		return err
	}
	return nil
}

func validateDeclarationAdapterMembers(moduleName, ownerName string, members map[string]DeclarationAdapterExport) error {
	for _, memberName := range sortedDeclarationAdapterKeys(members) {
		member := members[memberName]
		if strings.TrimSpace(memberName) == "" {
			return fmt.Errorf("declaration adapter export %s from %s contains an empty member name", ownerName, moduleName)
		}
		if member.Kind != "component" && member.Kind != "function" {
			return fmt.Errorf("declaration adapter member %s.%s from %s has unsupported kind %q", ownerName, memberName, moduleName, member.Kind)
		}
		if len(member.Members) != 0 || len(member.InstanceMembers) != 0 || len(member.ClassMembers) != 0 {
			return fmt.Errorf("declaration adapter member %s.%s from %s cannot declare nested members", ownerName, memberName, moduleName)
		}
		if err := validateDeclarationAdapterExport(moduleName, "member "+ownerName+".", memberName, member, false); err != nil {
			return err
		}
	}
	return nil
}

func validateDeclarationAdapterExportShape(exported DeclarationAdapterExport) error {
	callable := exported.Kind == "component" || exported.Kind == "function" || exported.Kind == "class"
	if !callable && (len(exported.Parameters) != 0 || exported.Required != 0 || exported.Variadic) {
		return fmt.Errorf("kind %s cannot declare call parameters", exported.Kind)
	}
	if exported.Variadic && len(exported.Parameters) == 0 {
		return fmt.Errorf("a variadic declaration requires a parameter")
	}
	if exported.Variadic && exported.Required >= len(exported.Parameters) {
		return fmt.Errorf("a variadic parameter cannot be required")
	}
	if exported.Kind == "component" {
		if len(exported.Parameters) > 1 {
			return fmt.Errorf("a component accepts at most one props parameter")
		}
		if exported.Variadic {
			return fmt.Errorf("a component cannot be variadic")
		}
		if len(exported.TypeParameters) != 0 {
			return fmt.Errorf("a component cannot declare type parameters")
		}
	}
	if exported.Kind != "record" && exported.Kind != "class" && len(exported.Fields) != 0 {
		return fmt.Errorf("fields are only valid for records and classes")
	}
	if exported.Kind == "class" && len(exported.Members) != 0 {
		return fmt.Errorf("kind class uses instanceMembers or classMembers instead of members")
	}
	if exported.Kind == "interface" && len(exported.Members) != 0 {
		return fmt.Errorf("kind interface uses instanceMembers instead of members")
	}
	if exported.Kind != "class" && exported.Kind != "interface" && len(exported.InstanceMembers) != 0 {
		return fmt.Errorf("instanceMembers are only valid for classes and interfaces")
	}
	if exported.Kind != "class" && len(exported.ClassMembers) != 0 {
		return fmt.Errorf("classMembers are only valid for classes")
	}
	if (exported.Kind == "record" || exported.Kind == "type_alias") && len(exported.Members) != 0 {
		return fmt.Errorf("kind %s cannot declare members", exported.Kind)
	}
	return nil
}

func validateDeclarationAdapterType(typ DeclarationAdapterType) error {
	switch typ.Kind {
	case "array", "bool", "bytes", "float", "function", "hash", "int", "int_literal", "named", "never", "nil", "range", "string", "string_literal", "union", "void":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if strings.TrimSpace(typ.Name) == "" {
		return fmt.Errorf("type kind %s requires a name", typ.Kind)
	}
	if canonical, exists := declarationAdapterCanonicalTypeNames[typ.Kind]; exists && typ.Name != canonical {
		return fmt.Errorf("type kind %s requires name %s", typ.Kind, canonical)
	}
	switch typ.Kind {
	case "array", "range":
		if len(typ.Arguments) != 1 {
			return fmt.Errorf("type kind %s requires exactly one argument", typ.Kind)
		}
	case "hash":
		if len(typ.Arguments) != 2 {
			return fmt.Errorf("type kind hash requires exactly two arguments")
		}
	case "function":
		if len(typ.Arguments) == 0 {
			return fmt.Errorf("type kind function requires a return type")
		}
	case "union":
		if len(typ.Arguments) < 2 {
			return fmt.Errorf("type kind union requires at least two alternatives")
		}
	case "named":
		// Named declarations may carry any number of explicit generic arguments.
	case "int_literal":
		value, err := strconv.ParseInt(strings.ReplaceAll(typ.Name, "_", ""), 10, 64)
		if err != nil || value < -9007199254740991 || value > 9007199254740991 {
			return fmt.Errorf("type kind int_literal requires a portable Integer literal name")
		}
		if typ.Name != strconv.FormatInt(value, 10) {
			return fmt.Errorf("type kind int_literal requires a canonical Integer literal name")
		}
		if len(typ.Arguments) != 0 || typ.Nullable {
			return fmt.Errorf("type kind int_literal cannot have arguments or be nullable")
		}
	case "string_literal":
		value, err := strconv.Unquote(typ.Name)
		if err != nil || len(typ.Name) < 2 || typ.Name[0] != '"' || typ.Name[len(typ.Name)-1] != '"' {
			return fmt.Errorf("type kind string_literal requires a quoted String literal name")
		}
		if typ.Name != strconv.Quote(value) {
			return fmt.Errorf("type kind string_literal requires a canonical String literal name")
		}
		if len(typ.Arguments) != 0 || typ.Nullable {
			return fmt.Errorf("type kind string_literal cannot have arguments or be nullable")
		}
	default:
		if len(typ.Arguments) != 0 {
			return fmt.Errorf("type kind %s cannot have arguments", typ.Kind)
		}
		if typ.Nullable && (typ.Kind == "never" || typ.Kind == "nil" || typ.Kind == "void") {
			return fmt.Errorf("type kind %s cannot be nullable", typ.Kind)
		}
	}
	if typ.ResultBridge != nil {
		if typ.Kind != "function" {
			return fmt.Errorf("resultBridge is only valid on function types")
		}
		if len(typ.Arguments) == 0 {
			return fmt.Errorf("resultBridge requires a function return type")
		}
		if strings.TrimSpace(typ.ResultBridge.Kind) == "" {
			return fmt.Errorf("resultBridge kind is required")
		}
		if typ.ResultBridge.Error.Kind == "" {
			return fmt.Errorf("resultBridge error is required")
		}
		if err := validateDeclarationAdapterType(typ.ResultBridge.Error); err != nil {
			return fmt.Errorf("invalid resultBridge error type: %w", err)
		}
	}
	for _, argument := range typ.Arguments {
		if err := validateDeclarationAdapterType(argument); err != nil {
			return err
		}
	}
	return nil
}

var declarationAdapterCanonicalTypeNames = map[string]string{
	"array":    "Array",
	"bool":     "Boolean",
	"bytes":    "Bytes",
	"float":    "Float",
	"function": "Function",
	"hash":     "Hash",
	"int":      "Integer",
	"never":    "Never",
	"nil":      "Nil",
	"range":    "Range",
	"string":   "String",
	"union":    "Union",
	"void":     "Void",
}

func validateDeclarationAdapterTypeParameters(parameters []string) error {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter) == "" {
			return fmt.Errorf("type parameter name is empty")
		}
		if seen[parameter] {
			return fmt.Errorf("type parameter %s is duplicated", parameter)
		}
		seen[parameter] = true
	}
	return nil
}

func sortedDeclarationAdapterKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
