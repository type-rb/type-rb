package packageextension

import (
	"fmt"
	"strings"
)

const DeclarationProtocolVersion = 2

// DeclarationCatalog is the versioned, mode-independent output of a package
// declaration provider. It contains semantic data only: the compiler host
// validates and converts it before resolution and checking.
type DeclarationCatalog struct {
	ProtocolVersion                int                                     `json:"protocolVersion"`
	Provider                       string                                  `json:"provider"`
	Types                          []DeclaredType                          `json:"types,omitempty"`
	Modules                        []DeclaredModule                        `json:"modules,omitempty"`
	FunctionBlockRules             []DeclaredFunctionBlockRule             `json:"functionBlockRules,omitempty"`
	FunctionArgumentReferenceRules []DeclaredFunctionArgumentReferenceRule `json:"functionArgumentReferenceRules,omitempty"`
	RuntimeTypes                   []DeclaredModuleRuntimeTypes            `json:"runtimeTypes,omitempty"`
}

type DeclaredType struct {
	Name            string           `json:"name"`
	TypeParameters  []string         `json:"typeParameters,omitempty"`
	Superclass      string           `json:"superclass,omitempty"`
	SourceModule    string           `json:"sourceModule,omitempty"`
	InstanceMembers []DeclaredMember `json:"instanceMembers,omitempty"`
	ClassMembers    []DeclaredMember `json:"classMembers,omitempty"`
}

type DeclaredModule struct {
	Name            string           `json:"name"`
	InstanceMembers []DeclaredMember `json:"instanceMembers,omitempty"`
}

type DeclaredMember struct {
	Name             string              `json:"name"`
	Kind             string              `json:"kind"`
	RuntimeOperation string              `json:"runtimeOperation,omitempty"`
	CallSpecializer  string              `json:"callSpecializer,omitempty"`
	Parameters       []DeclaredParameter `json:"parameters,omitempty"`
	MinimumArguments int                 `json:"minimumArguments,omitempty"`
	MaximumArguments int                 `json:"maximumArguments,omitempty"`
	Return           Type                `json:"return"`
	Variadic         bool                `json:"variadic,omitempty"`
	TypeParameters   []string            `json:"typeParameters,omitempty"`
	Alternatives     []DeclaredSignature `json:"alternatives,omitempty"`
	Block            *DeclaredBlock      `json:"block,omitempty"`
}

type DeclaredParameter struct {
	Name                   string     `json:"name"`
	Type                   Type       `json:"type"`
	Keyword                bool       `json:"keyword,omitempty"`
	Optional               bool       `json:"optional,omitempty"`
	RepresentationBoundary bool       `json:"representationBoundary,omitempty"`
	LiteralValues          []string   `json:"literalValues,omitempty"`
	LiteralArrays          [][]string `json:"literalArrays,omitempty"`
	LiteralArrayElements   []string   `json:"literalArrayElements,omitempty"`
}

type DeclaredSignature struct {
	Parameters []DeclaredParameter `json:"parameters,omitempty"`
	Return     Type                `json:"return"`
	Variadic   bool                `json:"variadic,omitempty"`
}

type DeclaredBlock struct {
	Parameters      []Type `json:"parameters,omitempty"`
	ControlBoundary bool   `json:"controlBoundary,omitempty"`
	Return          Type   `json:"return,omitempty"`
	ResultBoundary  Type   `json:"resultBoundary,omitempty"`
	Structured      bool   `json:"structured,omitempty"`
}

type DeclaredFunctionBlockRule struct {
	Package             string `json:"package"`
	Function            string `json:"function"`
	EnclosingSuperclass string `json:"enclosingSuperclass,omitempty"`
	TypeArgument        int    `json:"typeArgument"`
	ParameterTypeSuffix string `json:"parameterTypeSuffix,omitempty"`
}

type DeclaredReference struct {
	ModulePath string `json:"modulePath"`
	Name       string `json:"name"`
}

type DeclaredFunctionArgumentReferenceRule struct {
	Package  string              `json:"package"`
	Function string              `json:"function"`
	Argument int                 `json:"argument"`
	Owner    DeclaredReference   `json:"owner"`
	Targets  []DeclaredReference `json:"targets,omitempty"`
}

type DeclaredModuleRuntimeTypes struct {
	ModulePath string `json:"modulePath"`
	Types      []Type `json:"types"`
}

func ValidateDeclarationCatalog(catalog DeclarationCatalog) error {
	if catalog.ProtocolVersion != DeclarationProtocolVersion {
		return fmt.Errorf("unsupported declaration protocol version %d", catalog.ProtocolVersion)
	}
	if strings.TrimSpace(catalog.Provider) == "" {
		return fmt.Errorf("declaration catalog provider is missing")
	}
	seenTypes := map[string]bool{}
	for _, declared := range catalog.Types {
		if declared.Name == "" || seenTypes[declared.Name] {
			return fmt.Errorf("declaration catalog contains an empty or duplicate type %q", declared.Name)
		}
		seenTypes[declared.Name] = true
		if err := validateDeclaredMembers(declared.Name+" instance", declared.InstanceMembers); err != nil {
			return err
		}
		if err := validateDeclaredMembers(declared.Name+" class", declared.ClassMembers); err != nil {
			return err
		}
	}
	seenModules := map[string]bool{}
	for _, declared := range catalog.Modules {
		if declared.Name == "" || seenModules[declared.Name] {
			return fmt.Errorf("declaration catalog contains an empty or duplicate module %q", declared.Name)
		}
		seenModules[declared.Name] = true
		if err := validateDeclaredMembers(declared.Name+" module", declared.InstanceMembers); err != nil {
			return err
		}
	}
	for _, rule := range catalog.FunctionBlockRules {
		if strings.TrimSpace(rule.Package) == "" || strings.TrimSpace(rule.Function) == "" {
			return fmt.Errorf("declaration catalog contains a block rule without a package or function")
		}
		if rule.Package != catalog.Provider {
			return fmt.Errorf("declaration catalog block rule %s.%s does not belong to provider %s", rule.Package, rule.Function, catalog.Provider)
		}
		if rule.TypeArgument < 0 {
			return fmt.Errorf("declaration catalog block rule %s.%s has a negative type argument", rule.Package, rule.Function)
		}
	}
	for _, rule := range catalog.FunctionArgumentReferenceRules {
		if strings.TrimSpace(rule.Package) == "" || strings.TrimSpace(rule.Function) == "" {
			return fmt.Errorf("declaration catalog contains a reference rule without a package or function")
		}
		if rule.Package != catalog.Provider {
			return fmt.Errorf("declaration catalog reference rule %s.%s does not belong to provider %s", rule.Package, rule.Function, catalog.Provider)
		}
		if rule.Argument < 0 {
			return fmt.Errorf("declaration catalog reference rule %s.%s has a negative argument", rule.Package, rule.Function)
		}
		if !validDeclaredReference(rule.Owner) {
			return fmt.Errorf("declaration catalog reference rule %s.%s has an invalid owner", rule.Package, rule.Function)
		}
		for _, target := range rule.Targets {
			if !validDeclaredReference(target) {
				return fmt.Errorf("declaration catalog reference rule %s.%s has an invalid target", rule.Package, rule.Function)
			}
		}
	}
	seenRuntimeModules := map[string]bool{}
	for _, runtime := range catalog.RuntimeTypes {
		if runtime.ModulePath == "" || seenRuntimeModules[runtime.ModulePath] {
			return fmt.Errorf("declaration catalog contains an empty or duplicate runtime type module %q", runtime.ModulePath)
		}
		seenRuntimeModules[runtime.ModulePath] = true
		for _, typ := range runtime.Types {
			if err := validateDeclarationType(typ); err != nil {
				return fmt.Errorf("runtime type in %s: %w", runtime.ModulePath, err)
			}
		}
	}
	return nil
}

func validateDeclaredMembers(owner string, members []DeclaredMember) error {
	seen := map[string]bool{}
	for _, member := range members {
		if member.Name == "" || seen[member.Name] {
			return fmt.Errorf("declaration catalog %s members contain an empty or duplicate name %q", owner, member.Name)
		}
		seen[member.Name] = true
		if member.Kind != "method" && member.Kind != "property" {
			return fmt.Errorf("declaration catalog member %s.%s has unsupported kind %q", owner, member.Name, member.Kind)
		}
		if member.MinimumArguments < 0 || member.MaximumArguments < 0 || member.MaximumArguments > 0 && member.MaximumArguments < member.MinimumArguments {
			return fmt.Errorf("declaration catalog member %s.%s has an invalid argument range", owner, member.Name)
		}
		if err := validateDeclarationType(member.Return); err != nil {
			return fmt.Errorf("declaration catalog member %s.%s return: %w", owner, member.Name, err)
		}
		for _, parameter := range member.Parameters {
			if parameter.Name == "" {
				return fmt.Errorf("declaration catalog member %s.%s has an unnamed parameter", owner, member.Name)
			}
			if err := validateDeclarationType(parameter.Type); err != nil {
				return fmt.Errorf("declaration catalog member %s.%s parameter %s: %w", owner, member.Name, parameter.Name, err)
			}
		}
		for _, signature := range member.Alternatives {
			if err := validateDeclarationType(signature.Return); err != nil {
				return fmt.Errorf("declaration catalog member %s.%s alternative return: %w", owner, member.Name, err)
			}
			for _, parameter := range signature.Parameters {
				if parameter.Name == "" {
					return fmt.Errorf("declaration catalog member %s.%s has an unnamed alternative parameter", owner, member.Name)
				}
				if err := validateDeclarationType(parameter.Type); err != nil {
					return fmt.Errorf("declaration catalog member %s.%s alternative parameter %s: %w", owner, member.Name, parameter.Name, err)
				}
			}
		}
		if member.Block != nil {
			for _, parameter := range member.Block.Parameters {
				if err := validateDeclarationType(parameter); err != nil {
					return fmt.Errorf("declaration catalog member %s.%s block parameter: %w", owner, member.Name, err)
				}
			}
			for _, typ := range []Type{member.Block.Return, member.Block.ResultBoundary} {
				if typ.Kind != "" {
					if err := validateDeclarationType(typ); err != nil {
						return fmt.Errorf("declaration catalog member %s.%s block: %w", owner, member.Name, err)
					}
				}
			}
		}
	}
	return nil
}

func validDeclaredReference(reference DeclaredReference) bool {
	return strings.TrimSpace(reference.ModulePath) != "" && strings.TrimSpace(reference.Name) != ""
}

func validateDeclarationType(typ Type) error {
	if typ.Kind == "" {
		return fmt.Errorf("type kind is missing")
	}
	switch typ.Kind {
	case "invalid", "never", "any", "void", "bool", "int", "int_literal", "float", "string", "string_literal", "bytes", "string_builder", "array", "range", "iterable", "hash", "function", "union", "named", "nil":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if typ.Definition != nil {
		return fmt.Errorf("source definition metadata is not valid in a declaration type")
	}
	if typ.Record != nil {
		return fmt.Errorf("record inspection metadata is not valid in a declaration type")
	}
	for _, argument := range typ.Arguments {
		if err := validateDeclarationType(argument); err != nil {
			return err
		}
	}
	return nil
}
