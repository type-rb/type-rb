package packageextension

import (
	"fmt"
	"sort"
	"strings"
)

const DeclarationAdapterProtocolVersion = 1

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
	if exported.Kind != "component" && exported.Kind != "function" && exported.Kind != "class" && exported.Kind != "record" && exported.Kind != "type_alias" {
		return fmt.Errorf("declaration adapter %s %s from %s has unsupported kind %q", category, name, moduleName, exported.Kind)
	}
	if category == "record" && exported.Kind != "record" {
		return fmt.Errorf("declaration adapter record %s from %s must use kind record", name, moduleName)
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
	if !allowMembers && len(exported.Members) != 0 {
		return fmt.Errorf("declaration adapter record %s from %s cannot declare members", name, moduleName)
	}
	for _, memberName := range sortedDeclarationAdapterKeys(exported.Members) {
		member := exported.Members[memberName]
		if strings.TrimSpace(memberName) == "" {
			return fmt.Errorf("declaration adapter export %s from %s contains an empty member name", name, moduleName)
		}
		if member.Kind != "component" && member.Kind != "function" {
			return fmt.Errorf("declaration adapter member %s.%s from %s has unsupported kind %q", name, memberName, moduleName, member.Kind)
		}
		if len(member.Members) != 0 {
			return fmt.Errorf("declaration adapter member %s.%s from %s cannot declare nested members", name, memberName, moduleName)
		}
		if err := validateDeclarationAdapterExport(moduleName, "member "+name+".", memberName, member, false); err != nil {
			return err
		}
	}
	return nil
}

func validateDeclarationAdapterType(typ DeclarationAdapterType) error {
	switch typ.Kind {
	case "array", "bool", "bytes", "float", "function", "hash", "int", "int_literal", "named", "never", "nil", "range", "string", "string_literal", "union", "void":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if (typ.Kind == "named" || typ.Kind == "function") && strings.TrimSpace(typ.Name) == "" {
		return fmt.Errorf("type kind %s requires a name", typ.Kind)
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
