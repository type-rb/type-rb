package packageextension

import (
	"fmt"
	"strings"
)

const ProjectDeclarationInputProtocolVersion = 7

// ProjectDeclarationInput is a versioned, read-only snapshot of source
// declarations that a declaration provider may inspect. It intentionally
// excludes parser nodes, method bodies, resolver state, and filesystem access.
type ProjectDeclarationInput struct {
	ProtocolVersion int             `json:"protocolVersion"`
	Provider        string          `json:"provider"`
	Modules         []ProjectModule `json:"modules,omitempty"`
}

type ProjectModule struct {
	ModulePath  string             `json:"modulePath"`
	Imports     []ProjectImport    `json:"imports,omitempty"`
	TypeAliases []ProjectTypeAlias `json:"typeAliases,omitempty"`
	Newtypes    []ProjectNewtype   `json:"newtypes,omitempty"`
	Records     []ProjectRecord    `json:"records,omitempty"`
	Enums       []ProjectEnum      `json:"enums,omitempty"`
	Classes     []ProjectClass     `json:"classes,omitempty"`
	// Functions reuse the method-signature DTO, with Class always false.
	Functions []ProjectMethod `json:"functions,omitempty"`
}

// ProjectDeclarationIdentity identifies a source declaration independently
// from its display name and from any generated backend identifier. Name uses
// :: for declarations nested in authored modules.
type ProjectDeclarationIdentity struct {
	ModulePath string `json:"modulePath"`
	Name       string `json:"name"`
}

type ProjectImport struct {
	Path       string     `json:"path"`
	ModulePath string     `json:"modulePath"`
	Symbols    []string   `json:"symbols,omitempty"`
	Alias      string     `json:"alias,omitempty"`
	Span       SourceSpan `json:"span"`
}

type ProjectTypeAlias struct {
	Identity       ProjectDeclarationIdentity `json:"identity"`
	Name           string                     `json:"name"`
	TypeParameters []string                   `json:"typeParameters,omitempty"`
	Target         ProjectTypeUse             `json:"target"`
	Span           SourceSpan                 `json:"span"`
}

type ProjectNewtype struct {
	Identity ProjectDeclarationIdentity `json:"identity"`
	Name     string                     `json:"name"`
	Target   ProjectTypeUse             `json:"target"`
	Span     SourceSpan                 `json:"span"`
}

type ProjectRecord struct {
	Identity       ProjectDeclarationIdentity  `json:"identity"`
	Owner          *ProjectDeclarationIdentity `json:"owner,omitempty"`
	Name           string                      `json:"name"`
	TypeParameters []string                    `json:"typeParameters,omitempty"`
	Fields         []ProjectRecordField        `json:"fields,omitempty"`
	Span           SourceSpan                  `json:"span"`
}

type ProjectRecordField struct {
	Name       string             `json:"name"`
	Type       ProjectTypeUse     `json:"type"`
	HasDefault bool               `json:"hasDefault,omitempty"`
	Attributes []ProjectAttribute `json:"attributes,omitempty"`
	Span       SourceSpan         `json:"span"`
}

type ProjectAttribute struct {
	Name      string                     `json:"name"`
	Arguments []ProjectDirectiveArgument `json:"arguments,omitempty"`
	Span      SourceSpan                 `json:"span"`
}

type ProjectClass struct {
	Identity       ProjectDeclarationIdentity `json:"identity"`
	Name           string                     `json:"name"`
	TypeParameters []string                   `json:"typeParameters,omitempty"`
	Superclass     *ProjectTypeUse            `json:"superclass,omitempty"`
	Methods        []ProjectMethod            `json:"methods,omitempty"`
	Directives     []ProjectDirective         `json:"directives,omitempty"`
	Span           SourceSpan                 `json:"span"`
}

type ProjectEnum struct {
	Identity       ProjectDeclarationIdentity  `json:"identity"`
	Owner          *ProjectDeclarationIdentity `json:"owner,omitempty"`
	Name           string                      `json:"name"`
	TypeParameters []string                    `json:"typeParameters,omitempty"`
	Members        []ProjectEnumMember         `json:"members,omitempty"`
	Span           SourceSpan                  `json:"span"`
}

type ProjectEnumMember struct {
	Name       string             `json:"name"`
	Parameters []ProjectParameter `json:"parameters,omitempty"`
	RawValue   *ProjectValue      `json:"rawValue,omitempty"`
	Attributes []ProjectAttribute `json:"attributes,omitempty"`
	Span       SourceSpan         `json:"span"`
}

type ProjectMethod struct {
	// Identity is present for a top-level function. Methods are identified by
	// their owning class identity, source name, and Class discriminator.
	Identity       *ProjectDeclarationIdentity `json:"identity,omitempty"`
	Name           string                      `json:"name"`
	Class          bool                        `json:"class,omitempty"`
	TypeParameters []string                    `json:"typeParameters,omitempty"`
	Parameters     []ProjectParameter          `json:"parameters,omitempty"`
	Return         *ProjectTypeUse             `json:"return,omitempty"`
	Span           SourceSpan                  `json:"span"`
}

type ProjectParameter struct {
	Name        string         `json:"name"`
	Type        ProjectTypeUse `json:"type"`
	NamedOnly   bool           `json:"namedOnly,omitempty"`
	Keyword     bool           `json:"keyword,omitempty"`
	Rest        bool           `json:"rest,omitempty"`
	KeywordRest bool           `json:"keywordRest,omitempty"`
	Optional    bool           `json:"optional,omitempty"`
	Span        SourceSpan     `json:"span"`
}

// ProjectDirective is a direct class-body call. It retains only declarative
// argument values and a structural block summary, never executable block
// statements.
type ProjectDirective struct {
	Name          string                     `json:"name"`
	TypeArguments []ProjectTypeUse           `json:"typeArguments,omitempty"`
	Arguments     []ProjectDirectiveArgument `json:"arguments,omitempty"`
	Block         *ProjectDirectiveBlock     `json:"block,omitempty"`
	Span          SourceSpan                 `json:"span"`
}

type ProjectDirectiveArgument struct {
	Name  string       `json:"name,omitempty"`
	Splat string       `json:"splat,omitempty"`
	Value ProjectValue `json:"value"`
	Span  SourceSpan   `json:"span"`
}

type ProjectValue struct {
	Kind      string                       `json:"kind"`
	Raw       string                       `json:"raw,omitempty"`
	Name      string                       `json:"name,omitempty"`
	Reference *ProjectDeclarationReference `json:"reference,omitempty"`
}

type ProjectDirectiveBlock struct {
	Parameters       []string   `json:"parameters,omitempty"`
	StatementCount   int        `json:"statementCount"`
	ResultExpression bool       `json:"resultExpression,omitempty"`
	Span             SourceSpan `json:"span"`
}

// ProjectTypeUse separates the source spelling from the best resolved type
// available before checking. ResolutionPath records namespace-stable type
// identities traversed while resolving transparent aliases.
type ProjectTypeUse struct {
	Authored       Type                          `json:"authored"`
	Resolved       Type                          `json:"resolved"`
	Representation *Type                         `json:"representation,omitempty"`
	ResolutionPath []ProjectDeclarationReference `json:"resolutionPath,omitempty"`
	Span           SourceSpan                    `json:"span"`
}

// ProjectDeclarationReference keeps the canonical semantic identity separate
// from the import path through which source reached it.
type ProjectDeclarationReference struct {
	Identity   ProjectDeclarationIdentity `json:"identity"`
	ImportPath string                     `json:"importPath,omitempty"`
}

// SourceSpan is a half-open source range. An all-zero span means that source
// location is unavailable, which keeps programmatically assembled inputs valid.
type SourceSpan struct {
	Start SourcePosition `json:"start"`
	End   SourcePosition `json:"end"`
}

type SourcePosition struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

func ValidateProjectDeclarationInput(input ProjectDeclarationInput) error {
	if input.ProtocolVersion != ProjectDeclarationInputProtocolVersion {
		return fmt.Errorf("unsupported project declaration input protocol version %d", input.ProtocolVersion)
	}
	if strings.TrimSpace(input.Provider) == "" {
		return fmt.Errorf("project declaration input provider is missing")
	}
	seenModules := map[string]bool{}
	for _, module := range input.Modules {
		if strings.TrimSpace(module.ModulePath) == "" || seenModules[module.ModulePath] {
			return fmt.Errorf("project declaration input contains an empty or duplicate module %q", module.ModulePath)
		}
		seenModules[module.ModulePath] = true
		for _, imported := range module.Imports {
			if strings.TrimSpace(imported.Path) == "" {
				return fmt.Errorf("project declaration input module %s contains an import without a path", module.ModulePath)
			}
			if strings.TrimSpace(imported.ModulePath) == "" {
				return fmt.Errorf("project declaration input module %s import %s has no resolved module path", module.ModulePath, imported.Path)
			}
			for _, symbol := range imported.Symbols {
				if strings.TrimSpace(symbol) == "" {
					return fmt.Errorf("project declaration input module %s import %s contains an empty symbol", module.ModulePath, imported.Path)
				}
			}
			if err := validateSourceSpan(imported.Span); err != nil {
				return fmt.Errorf("project declaration input module %s import %s: %w", module.ModulePath, imported.Path, err)
			}
		}
		for _, alias := range module.TypeAliases {
			if strings.TrimSpace(alias.Name) == "" {
				return fmt.Errorf("project declaration input module %s contains an unnamed type alias", module.ModulePath)
			}
			if err := validateProjectDeclarationIdentity(module.ModulePath, alias.Name, alias.Identity, false); err != nil {
				return fmt.Errorf("project declaration input type alias %s.%s: %w", module.ModulePath, alias.Name, err)
			}
			if err := validateProjectTypeParameters(alias.TypeParameters); err != nil {
				return fmt.Errorf("project declaration input type alias %s.%s: %w", module.ModulePath, alias.Name, err)
			}
			if err := validateProjectTypeUse(alias.Target); err != nil {
				return fmt.Errorf("project declaration input type alias %s.%s target: %w", module.ModulePath, alias.Name, err)
			}
			if err := validateSourceSpan(alias.Span); err != nil {
				return fmt.Errorf("project declaration input type alias %s.%s: %w", module.ModulePath, alias.Name, err)
			}
		}
		for _, newtype := range module.Newtypes {
			if strings.TrimSpace(newtype.Name) == "" {
				return fmt.Errorf("project declaration input module %s contains an unnamed newtype", module.ModulePath)
			}
			if err := validateProjectDeclarationIdentity(module.ModulePath, newtype.Name, newtype.Identity, false); err != nil {
				return fmt.Errorf("project declaration input newtype %s.%s: %w", module.ModulePath, newtype.Name, err)
			}
			if err := validateProjectTypeUse(newtype.Target); err != nil {
				return fmt.Errorf("project declaration input newtype %s.%s target: %w", module.ModulePath, newtype.Name, err)
			}
			if err := validateSourceSpan(newtype.Span); err != nil {
				return fmt.Errorf("project declaration input newtype %s.%s: %w", module.ModulePath, newtype.Name, err)
			}
		}
		for _, record := range module.Records {
			if err := validateProjectRecord(module.ModulePath, record); err != nil {
				return err
			}
		}
		for _, enum := range module.Enums {
			if err := validateProjectEnum(module.ModulePath, enum); err != nil {
				return err
			}
		}
		for _, class := range module.Classes {
			if err := validateProjectClass(module.ModulePath, class); err != nil {
				return err
			}
		}
		for _, function := range module.Functions {
			if err := validateProjectMethod(module.ModulePath, "function", "", function); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProjectRecord(modulePath string, record ProjectRecord) error {
	if strings.TrimSpace(record.Name) == "" {
		return fmt.Errorf("project declaration input module %s contains an unnamed record", modulePath)
	}
	if err := validateProjectDeclarationIdentity(modulePath, record.Name, record.Identity, true); err != nil {
		return fmt.Errorf("project declaration input record %s.%s: %w", modulePath, record.Name, err)
	}
	if err := validateProjectDeclarationOwner(record.Identity, record.Owner); err != nil {
		return fmt.Errorf("project declaration input record %s.%s: %w", modulePath, record.Name, err)
	}
	if err := validateProjectTypeParameters(record.TypeParameters); err != nil {
		return fmt.Errorf("project declaration input record %s.%s: %w", modulePath, record.Name, err)
	}
	if err := validateSourceSpan(record.Span); err != nil {
		return fmt.Errorf("project declaration input record %s.%s: %w", modulePath, record.Name, err)
	}
	for _, field := range record.Fields {
		if strings.TrimSpace(field.Name) == "" {
			return fmt.Errorf("project declaration input record %s.%s contains an unnamed field", modulePath, record.Name)
		}
		if err := validateProjectTypeUse(field.Type); err != nil {
			return fmt.Errorf("project declaration input record %s.%s field %s: %w", modulePath, record.Name, field.Name, err)
		}
		context := fmt.Sprintf("project declaration input record %s.%s field %s", modulePath, record.Name, field.Name)
		if err := validateProjectAttributes(context, field.Attributes); err != nil {
			return err
		}
		if err := validateSourceSpan(field.Span); err != nil {
			return fmt.Errorf("project declaration input record %s.%s field %s: %w", modulePath, record.Name, field.Name, err)
		}
	}
	return nil
}

func validateProjectEnum(modulePath string, enum ProjectEnum) error {
	if strings.TrimSpace(enum.Name) == "" {
		return fmt.Errorf("project declaration input module %s contains an unnamed enum", modulePath)
	}
	if err := validateProjectDeclarationIdentity(modulePath, enum.Name, enum.Identity, true); err != nil {
		return fmt.Errorf("project declaration input enum %s.%s: %w", modulePath, enum.Name, err)
	}
	if err := validateProjectDeclarationOwner(enum.Identity, enum.Owner); err != nil {
		return fmt.Errorf("project declaration input enum %s.%s: %w", modulePath, enum.Name, err)
	}
	if err := validateProjectTypeParameters(enum.TypeParameters); err != nil {
		return fmt.Errorf("project declaration input enum %s.%s: %w", modulePath, enum.Name, err)
	}
	if err := validateSourceSpan(enum.Span); err != nil {
		return fmt.Errorf("project declaration input enum %s.%s: %w", modulePath, enum.Name, err)
	}
	for _, member := range enum.Members {
		if strings.TrimSpace(member.Name) == "" {
			return fmt.Errorf("project declaration input enum %s.%s contains an unnamed member", modulePath, enum.Name)
		}
		for _, parameter := range member.Parameters {
			if strings.TrimSpace(parameter.Name) == "" {
				return fmt.Errorf("project declaration input enum %s.%s member %s contains an unnamed parameter", modulePath, enum.Name, member.Name)
			}
			if err := validateProjectTypeUse(parameter.Type); err != nil {
				return fmt.Errorf("project declaration input enum %s.%s member %s parameter %s: %w", modulePath, enum.Name, member.Name, parameter.Name, err)
			}
			if err := validateSourceSpan(parameter.Span); err != nil {
				return fmt.Errorf("project declaration input enum %s.%s member %s parameter %s: %w", modulePath, enum.Name, member.Name, parameter.Name, err)
			}
		}
		if member.RawValue != nil {
			if err := validateProjectValue(*member.RawValue); err != nil {
				return fmt.Errorf("project declaration input enum %s.%s member %s raw value: %w", modulePath, enum.Name, member.Name, err)
			}
		}
		context := fmt.Sprintf("project declaration input enum %s.%s member %s", modulePath, enum.Name, member.Name)
		if err := validateProjectAttributes(context, member.Attributes); err != nil {
			return err
		}
		if err := validateSourceSpan(member.Span); err != nil {
			return fmt.Errorf("project declaration input enum %s.%s member %s: %w", modulePath, enum.Name, member.Name, err)
		}
	}
	return nil
}

func validateProjectAttributes(context string, attributes []ProjectAttribute) error {
	for _, attribute := range attributes {
		if strings.TrimSpace(attribute.Name) == "" {
			return fmt.Errorf("%s contains an unnamed attribute", context)
		}
		for _, argument := range attribute.Arguments {
			if err := validateProjectValue(argument.Value); err != nil {
				return fmt.Errorf("%s attribute %s argument: %w", context, attribute.Name, err)
			}
			if err := validateSourceSpan(argument.Span); err != nil {
				return fmt.Errorf("%s attribute %s argument: %w", context, attribute.Name, err)
			}
		}
		if err := validateSourceSpan(attribute.Span); err != nil {
			return fmt.Errorf("%s attribute %s: %w", context, attribute.Name, err)
		}
	}
	return nil
}

func validateProjectClass(modulePath string, class ProjectClass) error {
	if strings.TrimSpace(class.Name) == "" {
		return fmt.Errorf("project declaration input module %s contains an unnamed class", modulePath)
	}
	if err := validateProjectDeclarationIdentity(modulePath, class.Name, class.Identity, false); err != nil {
		return fmt.Errorf("project declaration input class %s.%s: %w", modulePath, class.Name, err)
	}
	if err := validateProjectTypeParameters(class.TypeParameters); err != nil {
		return fmt.Errorf("project declaration input class %s.%s: %w", modulePath, class.Name, err)
	}
	if class.Superclass != nil {
		if err := validateProjectTypeUse(*class.Superclass); err != nil {
			return fmt.Errorf("project declaration input class %s.%s superclass: %w", modulePath, class.Name, err)
		}
	}
	if err := validateSourceSpan(class.Span); err != nil {
		return fmt.Errorf("project declaration input class %s.%s: %w", modulePath, class.Name, err)
	}
	for _, method := range class.Methods {
		if err := validateProjectMethod(modulePath, "method", class.Name, method); err != nil {
			return err
		}
	}
	for _, directive := range class.Directives {
		if strings.TrimSpace(directive.Name) == "" {
			return fmt.Errorf("project declaration input class %s.%s contains an unnamed directive", modulePath, class.Name)
		}
		for _, argument := range directive.TypeArguments {
			if err := validateProjectTypeUse(argument); err != nil {
				return fmt.Errorf("project declaration input directive %s.%s.%s type argument: %w", modulePath, class.Name, directive.Name, err)
			}
		}
		for _, argument := range directive.Arguments {
			if err := validateProjectValue(argument.Value); err != nil {
				return fmt.Errorf("project declaration input directive %s.%s.%s argument: %w", modulePath, class.Name, directive.Name, err)
			}
			if err := validateSourceSpan(argument.Span); err != nil {
				return fmt.Errorf("project declaration input directive %s.%s.%s argument: %w", modulePath, class.Name, directive.Name, err)
			}
		}
		if directive.Block != nil {
			if directive.Block.StatementCount < 0 {
				return fmt.Errorf("project declaration input directive %s.%s.%s block has a negative statement count", modulePath, class.Name, directive.Name)
			}
			if directive.Block.ResultExpression && directive.Block.StatementCount != 1 {
				return fmt.Errorf("project declaration input directive %s.%s.%s block result expression requires one statement", modulePath, class.Name, directive.Name)
			}
			for _, parameter := range directive.Block.Parameters {
				if strings.TrimSpace(parameter) == "" {
					return fmt.Errorf("project declaration input directive %s.%s.%s block contains an unnamed parameter", modulePath, class.Name, directive.Name)
				}
			}
			if err := validateSourceSpan(directive.Block.Span); err != nil {
				return fmt.Errorf("project declaration input directive %s.%s.%s block: %w", modulePath, class.Name, directive.Name, err)
			}
		}
		if err := validateSourceSpan(directive.Span); err != nil {
			return fmt.Errorf("project declaration input directive %s.%s.%s: %w", modulePath, class.Name, directive.Name, err)
		}
	}
	return nil
}

func validateProjectMethod(modulePath, kind, owner string, method ProjectMethod) error {
	location := modulePath + "." + method.Name
	if owner != "" {
		location = modulePath + "." + owner + "#" + method.Name
	}
	if strings.TrimSpace(method.Name) == "" {
		if owner == "" {
			return fmt.Errorf("project declaration input module %s contains an unnamed %s", modulePath, kind)
		}
		return fmt.Errorf("project declaration input class %s.%s contains an unnamed %s", modulePath, owner, kind)
	}
	if owner == "" && method.Class {
		return fmt.Errorf("project declaration input function %s cannot be a class method", location)
	}
	if owner == "" {
		if method.Identity == nil {
			return fmt.Errorf("project declaration input function %s has no declaration identity", location)
		}
		if err := validateProjectDeclarationIdentity(modulePath, method.Name, *method.Identity, false); err != nil {
			return fmt.Errorf("project declaration input function %s: %w", location, err)
		}
	} else if method.Identity != nil {
		return fmt.Errorf("project declaration input method %s must not carry a top-level declaration identity", location)
	}
	if err := validateProjectTypeParameters(method.TypeParameters); err != nil {
		return fmt.Errorf("project declaration input %s %s: %w", kind, location, err)
	}
	for _, parameter := range method.Parameters {
		if strings.TrimSpace(parameter.Name) == "" {
			return fmt.Errorf("project declaration input %s %s contains an unnamed parameter", kind, location)
		}
		if err := validateProjectTypeUse(parameter.Type); err != nil {
			return fmt.Errorf("project declaration input %s %s parameter %s: %w", kind, location, parameter.Name, err)
		}
		if err := validateSourceSpan(parameter.Span); err != nil {
			return fmt.Errorf("project declaration input %s %s parameter %s: %w", kind, location, parameter.Name, err)
		}
	}
	if method.Return != nil {
		if err := validateProjectTypeUse(*method.Return); err != nil {
			return fmt.Errorf("project declaration input %s %s return: %w", kind, location, err)
		}
	}
	if err := validateSourceSpan(method.Span); err != nil {
		return fmt.Errorf("project declaration input %s %s: %w", kind, location, err)
	}
	return nil
}

func validateProjectValue(value ProjectValue) error {
	switch value.Kind {
	case "string", "integer", "float", "boolean", "nil":
		if value.Raw == "" {
			return fmt.Errorf("contains an empty literal")
		}
	case "symbol":
		if strings.TrimSpace(value.Name) == "" {
			return fmt.Errorf("contains an empty symbol")
		}
	case "reference":
		if strings.TrimSpace(value.Name) == "" {
			return fmt.Errorf("contains an empty reference")
		}
		if value.Reference != nil {
			if err := validateProjectDeclarationReference(*value.Reference); err != nil {
				return fmt.Errorf("contains an invalid resolved reference")
			}
		}
	case "unsupported":
	default:
		return fmt.Errorf("contains unsupported value kind %q", value.Kind)
	}
	return nil
}

func validateProjectTypeParameters(parameters []string) error {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter) == "" || seen[parameter] {
			return fmt.Errorf("contains an empty or duplicate type parameter %q", parameter)
		}
		seen[parameter] = true
	}
	return nil
}

func validateProjectTypeUse(use ProjectTypeUse) error {
	if err := validateProjectInputType(use.Authored); err != nil {
		return fmt.Errorf("authored type: %w", err)
	}
	if err := validateProjectInputType(use.Resolved); err != nil {
		return fmt.Errorf("resolved type: %w", err)
	}
	if use.Representation != nil {
		if err := validateProjectInputType(*use.Representation); err != nil {
			return fmt.Errorf("representation type: %w", err)
		}
	}
	for _, reference := range use.ResolutionPath {
		if err := validateProjectDeclarationReference(reference); err != nil {
			return fmt.Errorf("resolution path contains an invalid type reference")
		}
	}
	return validateSourceSpan(use.Span)
}

func validateProjectInputType(typ Type) error {
	if typ.Kind == "" {
		return fmt.Errorf("type kind is missing")
	}
	switch typ.Kind {
	case "invalid", "never", "any", "void", "bool", "int", "int_literal", "float", "string", "string_literal", "bytes", "string_builder", "array", "range", "iterable", "hash", "function", "union", "named", "nil":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if typ.Definition != nil {
		if strings.TrimSpace(typ.Definition.ModulePath) == "" {
			return fmt.Errorf("source definition module path is missing")
		}
		if strings.TrimSpace(typ.Definition.Name) == "" {
			return fmt.Errorf("source definition name is missing")
		}
	}
	if typ.Record != nil {
		return fmt.Errorf("record inspection metadata is not valid in a project declaration type")
	}
	for _, argument := range typ.Arguments {
		if err := validateProjectInputType(argument); err != nil {
			return err
		}
	}
	return nil
}

func validateProjectDeclarationIdentity(modulePath, displayName string, identity ProjectDeclarationIdentity, allowNested bool) error {
	if strings.TrimSpace(identity.ModulePath) == "" || strings.TrimSpace(identity.Name) == "" {
		return fmt.Errorf("declaration identity is missing")
	}
	if identity.ModulePath != modulePath {
		return fmt.Errorf("declaration identity module %q does not match enclosing module %q", identity.ModulePath, modulePath)
	}
	parts := strings.Split(identity.Name, "::")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("declaration identity name %q is malformed", identity.Name)
		}
	}
	if !allowNested && len(parts) != 1 {
		return fmt.Errorf("declaration identity %q must be top-level", identity.Name)
	}
	if parts[len(parts)-1] != displayName {
		return fmt.Errorf("declaration identity %q does not match display name %q", identity.Name, displayName)
	}
	return nil
}

func validateProjectDeclarationOwner(identity ProjectDeclarationIdentity, owner *ProjectDeclarationIdentity) error {
	separator := strings.LastIndex(identity.Name, "::")
	if separator < 0 {
		if owner != nil {
			return fmt.Errorf("top-level declaration must not have an owner identity")
		}
		return nil
	}
	if owner == nil {
		return fmt.Errorf("nested declaration has no owner identity")
	}
	if owner.ModulePath != identity.ModulePath || owner.Name != identity.Name[:separator] {
		return fmt.Errorf("owner identity does not match declaration identity %q", identity.Name)
	}
	return nil
}

func validateProjectDeclarationReference(reference ProjectDeclarationReference) error {
	if strings.TrimSpace(reference.Identity.ModulePath) == "" || strings.TrimSpace(reference.Identity.Name) == "" {
		return fmt.Errorf("declaration reference identity is missing")
	}
	return nil
}

func validateSourceSpan(span SourceSpan) error {
	if span == (SourceSpan{}) {
		return nil
	}
	if err := validateSourcePosition(span.Start); err != nil {
		return fmt.Errorf("invalid source span start: %w", err)
	}
	if err := validateSourcePosition(span.End); err != nil {
		return fmt.Errorf("invalid source span end: %w", err)
	}
	if span.End.Offset < span.Start.Offset {
		return fmt.Errorf("source span ends before it starts")
	}
	return nil
}

func validateSourcePosition(position SourcePosition) error {
	if position.Offset < 0 || position.Line < 1 || position.Column < 1 {
		return fmt.Errorf("offset must be non-negative and line and column must be positive")
	}
	return nil
}
