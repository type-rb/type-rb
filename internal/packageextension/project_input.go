package packageextension

import (
	"fmt"
	"strings"
)

const ProjectDeclarationInputProtocolVersion = 1

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
	Classes     []ProjectClass     `json:"classes,omitempty"`
}

type ProjectImport struct {
	Path    string     `json:"path"`
	Symbols []string   `json:"symbols,omitempty"`
	Alias   string     `json:"alias,omitempty"`
	Span    SourceSpan `json:"span"`
}

type ProjectTypeAlias struct {
	Name           string         `json:"name"`
	TypeParameters []string       `json:"typeParameters,omitempty"`
	Target         ProjectTypeUse `json:"target"`
	Span           SourceSpan     `json:"span"`
}

type ProjectClass struct {
	Name           string             `json:"name"`
	TypeParameters []string           `json:"typeParameters,omitempty"`
	Superclass     *ProjectTypeUse    `json:"superclass,omitempty"`
	Methods        []ProjectMethod    `json:"methods,omitempty"`
	Directives     []ProjectDirective `json:"directives,omitempty"`
	Span           SourceSpan         `json:"span"`
}

type ProjectMethod struct {
	Name           string             `json:"name"`
	Class          bool               `json:"class,omitempty"`
	TypeParameters []string           `json:"typeParameters,omitempty"`
	Parameters     []ProjectParameter `json:"parameters,omitempty"`
	Return         *ProjectTypeUse    `json:"return,omitempty"`
	Span           SourceSpan         `json:"span"`
}

type ProjectParameter struct {
	Name        string         `json:"name"`
	Type        ProjectTypeUse `json:"type"`
	Keyword     bool           `json:"keyword,omitempty"`
	Rest        bool           `json:"rest,omitempty"`
	KeywordRest bool           `json:"keywordRest,omitempty"`
	Optional    bool           `json:"optional,omitempty"`
	Span        SourceSpan     `json:"span"`
}

// ProjectDirective is a direct class-body call with no block. Arguments retain
// only literal values; non-literal expressions are marked unsupported.
type ProjectDirective struct {
	Name      string                     `json:"name"`
	Arguments []ProjectDirectiveArgument `json:"arguments,omitempty"`
	Span      SourceSpan                 `json:"span"`
}

type ProjectDirectiveArgument struct {
	Name    string         `json:"name,omitempty"`
	Splat   string         `json:"splat,omitempty"`
	Literal ProjectLiteral `json:"literal"`
}

type ProjectLiteral struct {
	Kind string `json:"kind"`
	Raw  string `json:"raw,omitempty"`
}

// ProjectTypeUse separates the source spelling from the best resolved type
// available before checking. ResolutionPath records namespace-stable type
// identities traversed while resolving transparent aliases.
type ProjectTypeUse struct {
	Authored       Type                   `json:"authored"`
	Resolved       Type                   `json:"resolved"`
	ResolutionPath []ProjectTypeReference `json:"resolutionPath,omitempty"`
	Span           SourceSpan             `json:"span"`
}

type ProjectTypeReference struct {
	Name       string `json:"name"`
	ModulePath string `json:"modulePath"`
	ImportPath string `json:"importPath,omitempty"`
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
		for _, class := range module.Classes {
			if err := validateProjectClass(module.ModulePath, class); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateProjectClass(modulePath string, class ProjectClass) error {
	if strings.TrimSpace(class.Name) == "" {
		return fmt.Errorf("project declaration input module %s contains an unnamed class", modulePath)
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
		if strings.TrimSpace(method.Name) == "" {
			return fmt.Errorf("project declaration input class %s.%s contains an unnamed method", modulePath, class.Name)
		}
		if err := validateProjectTypeParameters(method.TypeParameters); err != nil {
			return fmt.Errorf("project declaration input method %s.%s#%s: %w", modulePath, class.Name, method.Name, err)
		}
		for _, parameter := range method.Parameters {
			if strings.TrimSpace(parameter.Name) == "" {
				return fmt.Errorf("project declaration input method %s.%s#%s contains an unnamed parameter", modulePath, class.Name, method.Name)
			}
			if err := validateProjectTypeUse(parameter.Type); err != nil {
				return fmt.Errorf("project declaration input method %s.%s#%s parameter %s: %w", modulePath, class.Name, method.Name, parameter.Name, err)
			}
			if err := validateSourceSpan(parameter.Span); err != nil {
				return fmt.Errorf("project declaration input method %s.%s#%s parameter %s: %w", modulePath, class.Name, method.Name, parameter.Name, err)
			}
		}
		if method.Return != nil {
			if err := validateProjectTypeUse(*method.Return); err != nil {
				return fmt.Errorf("project declaration input method %s.%s#%s return: %w", modulePath, class.Name, method.Name, err)
			}
		}
		if err := validateSourceSpan(method.Span); err != nil {
			return fmt.Errorf("project declaration input method %s.%s#%s: %w", modulePath, class.Name, method.Name, err)
		}
	}
	for _, directive := range class.Directives {
		if strings.TrimSpace(directive.Name) == "" {
			return fmt.Errorf("project declaration input class %s.%s contains an unnamed directive", modulePath, class.Name)
		}
		for _, argument := range directive.Arguments {
			switch argument.Literal.Kind {
			case "string", "integer", "float", "boolean", "nil":
				if argument.Literal.Raw == "" {
					return fmt.Errorf("project declaration input directive %s.%s.%s contains an empty literal", modulePath, class.Name, directive.Name)
				}
			case "unsupported":
			default:
				return fmt.Errorf("project declaration input directive %s.%s.%s contains unsupported literal kind %q", modulePath, class.Name, directive.Name, argument.Literal.Kind)
			}
		}
		if err := validateSourceSpan(directive.Span); err != nil {
			return fmt.Errorf("project declaration input directive %s.%s.%s: %w", modulePath, class.Name, directive.Name, err)
		}
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
	for _, reference := range use.ResolutionPath {
		if strings.TrimSpace(reference.Name) == "" || strings.TrimSpace(reference.ModulePath) == "" {
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
	if typ.Definition != nil && strings.TrimSpace(typ.Definition.ModulePath) == "" {
		return fmt.Errorf("source definition module path is missing")
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
