// Package toolingprotocol defines the versioned, read-only JSON snapshot
// exposed by explicit compiler tooling commands. It deliberately keeps
// compiler AST, typed IR, backend hooks, and mutation outside the protocol.
package toolingprotocol

import (
	"encoding/base64"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

const ProtocolVersion = 3

type Report struct {
	ProtocolVersion int                         `json:"protocolVersion"`
	CompilerVersion string                      `json:"compilerVersion"`
	Mode            string                      `json:"mode,omitempty"`
	Sources         []Source                    `json:"sources"`
	Modules         []Module                    `json:"modules"`
	Declarations    []Declaration               `json:"declarations"`
	Diagnostics     []diagnostic.JSONDiagnostic `json:"diagnostics"`
	Summary         diagnostic.JSONSummary      `json:"summary"`
}

type Source struct {
	Path     string `json:"path"`
	Encoding string `json:"encoding"`
	Content  string `json:"content"`
}

type Module struct {
	ModulePath      string   `json:"modulePath"`
	SourcePath      string   `json:"sourcePath"`
	CompilerOwned   bool     `json:"compilerOwned,omitempty"`
	Official        bool     `json:"official,omitempty"`
	ExternalPackage bool     `json:"externalPackage,omitempty"`
	Imports         []Import `json:"imports"`
}

type Import struct {
	Path       string                  `json:"path"`
	ModulePath string                  `json:"modulePath"`
	Symbols    []string                `json:"symbols,omitempty"`
	Alias      string                  `json:"alias,omitempty"`
	Namespace  bool                    `json:"namespace,omitempty"`
	Location   diagnostic.JSONLocation `json:"location"`
}

type DeclarationKind string

const (
	DeclarationFunction    DeclarationKind = "function"
	DeclarationClass       DeclarationKind = "class"
	DeclarationRecord      DeclarationKind = "record"
	DeclarationEnum        DeclarationKind = "enum"
	DeclarationEnumMember  DeclarationKind = "enum_member"
	DeclarationInterface   DeclarationKind = "interface"
	DeclarationTypeAlias   DeclarationKind = "type_alias"
	DeclarationNewtype     DeclarationKind = "newtype"
	DeclarationModule      DeclarationKind = "module"
	DeclarationConstant    DeclarationKind = "constant"
	DeclarationField       DeclarationKind = "field"
	DeclarationRecordField DeclarationKind = "record_field"
	DeclarationMethod      DeclarationKind = "method"
)

type Declaration struct {
	ID             string                  `json:"id"`
	ModulePath     string                  `json:"modulePath"`
	OwnerID        string                  `json:"ownerId,omitempty"`
	Kind           DeclarationKind         `json:"kind"`
	Name           string                  `json:"name"`
	QualifiedName  string                  `json:"qualifiedName"`
	Visibility     string                  `json:"visibility"`
	Location       diagnostic.JSONLocation `json:"location"`
	TypeParameters []string                `json:"typeParameters,omitempty"`
	Parameters     []Parameter             `json:"parameters,omitempty"`
	Alternatives   []Signature             `json:"alternatives,omitempty"`
	Type           *Type                   `json:"type,omitempty"`
	ReturnType     *Type                   `json:"returnType,omitempty"`
	Superclass     *Type                   `json:"superclass,omitempty"`
	Implements     []Type                  `json:"implements,omitempty"`
	RawType        *Type                   `json:"rawType,omitempty"`
	ClassMember    bool                    `json:"classMember,omitempty"`
	Readonly       bool                    `json:"readonly,omitempty"`
	External       bool                    `json:"external,omitempty"`
}

type Parameter struct {
	Name        string `json:"name"`
	Type        Type   `json:"type"`
	NamedOnly   bool   `json:"namedOnly,omitempty"`
	Keyword     bool   `json:"keyword,omitempty"`
	Optional    bool   `json:"optional,omitempty"`
	Rest        bool   `json:"rest,omitempty"`
	KeywordRest bool   `json:"keywordRest,omitempty"`
}

type Signature struct {
	Parameters []Parameter `json:"parameters"`
	ReturnType Type        `json:"returnType"`
	Variadic   bool        `json:"variadic,omitempty"`
}

type Type struct {
	Kind      string `json:"kind"`
	Name      string `json:"name,omitempty"`
	Nullable  bool   `json:"nullable,omitempty"`
	Readonly  bool   `json:"readonly,omitempty"`
	Arguments []Type `json:"arguments,omitempty"`
}

type BuildOptions struct {
	CompilerVersion string
	Mode            string
}

// Build creates a deterministic snapshot from the exact compiler-service
// inputs and its immutable analysis result. Authored source remains available
// when diagnostics prevent a checked declaration snapshot from being built.
func Build(options BuildOptions, units []compiler.SourceUnit, snapshot compilerservice.Snapshot) Report {
	diagnosticReport := diagnostic.NewJSONReport(snapshot.Diagnostics)
	report := Report{
		ProtocolVersion: ProtocolVersion,
		CompilerVersion: options.CompilerVersion,
		Mode:            options.Mode,
		Sources:         []Source{},
		Modules:         []Module{},
		Declarations:    []Declaration{},
		Diagnostics:     diagnosticReport.Diagnostics,
		Summary:         diagnosticReport.Summary,
	}

	artifacts := artifactsByModule(snapshot.Artifacts)
	for _, unit := range sortedUnits(units) {
		path := cleanPath(unit.Filename)
		text := unit.Source
		if analyzed, ok := snapshot.Source(path); ok {
			text = analyzed
		}
		report.Sources = append(report.Sources, protocolSource(path, text))

		module := Module{
			ModulePath: unit.ModulePath, SourcePath: path,
			CompilerOwned: unit.CompilerOwned, Official: unit.Official, ExternalPackage: unit.ExternalPackage,
			Imports: []Import{},
		}
		if artifact := artifacts[unit.ModulePath]; artifact != nil && artifact.IR != nil {
			module.Imports = moduleImports(artifact)
			report.Declarations = append(report.Declarations, moduleDeclarations(artifact)...)
		}
		report.Modules = append(report.Modules, module)
	}
	return report
}

func protocolSource(path string, contents []byte) Source {
	result := Source{Path: path, Encoding: "utf-8", Content: string(contents)}
	if utf8.Valid(contents) {
		return result
	}
	result.Encoding = "base64"
	result.Content = base64.StdEncoding.EncodeToString(contents)
	return result
}

func sortedUnits(units []compiler.SourceUnit) []compiler.SourceUnit {
	result := append([]compiler.SourceUnit(nil), units...)
	sort.SliceStable(result, func(left, right int) bool {
		if result[left].ModulePath != result[right].ModulePath {
			return result[left].ModulePath < result[right].ModulePath
		}
		return cleanPath(result[left].Filename) < cleanPath(result[right].Filename)
	})
	return result
}

func artifactsByModule(artifacts []*compiler.Artifact) map[string]*compiler.Artifact {
	result := make(map[string]*compiler.Artifact, len(artifacts))
	for _, artifact := range artifacts {
		if artifact != nil && artifact.IR != nil {
			result[artifact.IR.ModulePath] = artifact
		}
	}
	return result
}

func moduleImports(artifact *compiler.Artifact) []Import {
	result := []Import{}
	for _, statement := range artifact.IR.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Implicit || compilerGenerated(artifact, imported.SourceSpan()) {
			continue
		}
		path := imported.DeclaredPath
		if path == "" {
			path = imported.Path
		}
		result = append(result, Import{
			Path: path, ModulePath: imported.Path, Symbols: append([]string(nil), imported.Symbols...),
			Alias: imported.Alias, Namespace: imported.Namespace,
			Location: jsonLocation(artifact.Filename, imported.SourceSpan()),
		})
	}
	return result
}

func moduleDeclarations(artifact *compiler.Artifact) []Declaration {
	walker := declarationWalker{artifact: artifact, modulePath: artifact.IR.ModulePath, sourcePath: cleanPath(artifact.Filename)}
	walker.statements(artifact.IR.Statements, nil, "")
	return walker.result
}

type declarationWalker struct {
	artifact   *compiler.Artifact
	modulePath string
	sourcePath string
	result     []Declaration
}

func (w *declarationWalker) statements(statements []ir.Statement, owner *Declaration, ownerName string) {
	for _, statement := range statements {
		if statement == nil || compilerGenerated(w.artifact, statement.SourceSpan()) {
			continue
		}
		switch node := statement.(type) {
		case *ir.Variable:
			if !node.Constant {
				continue
			}
			declared := w.base(owner, ownerName, DeclarationConstant, node.Name, node.SourceSpan())
			declared.Type = typePointer(node.Type)
			w.add(declared)
		case *ir.Method:
			kind := DeclarationFunction
			if owner != nil {
				kind = DeclarationMethod
			}
			declared := w.method(owner, ownerName, kind, node)
			w.add(declared)
		case *ir.Class:
			declared := w.base(owner, ownerName, DeclarationClass, node.Name, node.SourceSpan())
			declared.TypeParameters = append([]string(nil), node.TypeParameters...)
			declared.External = node.External
			if node.Superclass != nil {
				declared.Superclass = typePointer(node.Superclass.ExprType())
			}
			declared.Implements = protocolTypes(node.Implements)
			w.add(declared)
			w.statements(node.Body, &declared, declared.QualifiedName)
		case *ir.Record:
			declared := w.base(owner, ownerName, DeclarationRecord, node.Name, node.SourceSpan())
			declared.TypeParameters = append([]string(nil), node.TypeParameters...)
			w.add(declared)
			w.statements(node.Body, &declared, declared.QualifiedName)
		case *ir.Enum:
			declared := w.base(owner, ownerName, DeclarationEnum, node.Name, node.SourceSpan())
			declared.TypeParameters = append([]string(nil), node.TypeParameters...)
			if node.RawType.Kind != "" {
				declared.RawType = typePointer(node.RawType)
			}
			w.add(declared)
			w.statements(node.Body, &declared, declared.QualifiedName)
		case *ir.EnumMember:
			declared := w.base(owner, ownerName, DeclarationEnumMember, node.Name, node.SourceSpan())
			declared.Parameters = parameters(node.Fields)
			w.add(declared)
		case *ir.Interface:
			declared := w.base(owner, ownerName, DeclarationInterface, node.Name, node.SourceSpan())
			declared.TypeParameters = append([]string(nil), node.TypeParameters...)
			w.add(declared)
			for _, method := range node.Methods {
				if method == nil || compilerGenerated(w.artifact, method.SourceSpan()) {
					continue
				}
				child := w.method(&declared, declared.QualifiedName, DeclarationMethod, method)
				w.add(child)
			}
		case *ir.TypeAlias:
			declared := w.base(owner, ownerName, DeclarationTypeAlias, node.Name, node.SourceSpan())
			declared.TypeParameters = append([]string(nil), node.TypeParameters...)
			declared.Type = typePointer(node.Target)
			w.add(declared)
			for index := range node.Variants {
				variant := &node.Variants[index]
				if compilerGenerated(w.artifact, variant.SourceSpan()) {
					continue
				}
				child := w.base(&declared, declared.QualifiedName, DeclarationEnumMember, variant.Name, variant.SourceSpan())
				child.Parameters = parameters(variant.Fields)
				w.add(child)
			}
		case *ir.Newtype:
			declared := w.base(owner, ownerName, DeclarationNewtype, node.Name, node.SourceSpan())
			declared.Type = typePointer(node.Target)
			w.add(declared)
		case *ir.Module:
			declared := w.base(owner, ownerName, DeclarationModule, node.Name, node.SourceSpan())
			w.add(declared)
			w.statements(node.Body, &declared, declared.QualifiedName)
		case *ir.Field:
			declared := w.base(owner, ownerName, DeclarationField, node.Name, node.SourceSpan())
			declared.Type = typePointer(node.Type)
			declared.Readonly = node.ReadOnly
			w.add(declared)
		case *ir.RecordField:
			declared := w.base(owner, ownerName, DeclarationRecordField, node.Name, node.SourceSpan())
			declared.Type = typePointer(node.Type)
			declared.Readonly = true
			w.add(declared)
		}
	}
}

func (w *declarationWalker) method(owner *Declaration, ownerName string, kind DeclarationKind, node *ir.Method) Declaration {
	declared := w.base(owner, ownerName, kind, node.Name, node.SourceSpan())
	declared.TypeParameters = append([]string(nil), node.TypeParameters...)
	declared.Parameters = parameters(node.Parameters)
	declared.ReturnType = typePointer(node.ReturnType)
	declared.ClassMember = node.Class
	declared.External = node.External
	for _, alternative := range node.Alternatives {
		declared.Alternatives = append(declared.Alternatives, Signature{
			Parameters: parameters(alternative.Parameters), ReturnType: protocolType(alternative.ReturnType), Variadic: alternative.Variadic,
		})
	}
	return declared
}

func (w *declarationWalker) base(owner *Declaration, ownerName string, kind DeclarationKind, name string, span token.Span) Declaration {
	qualified := name
	ownerID := ""
	if owner != nil {
		ownerID = owner.ID
		qualified = ownerName + "::" + name
	}
	scope := ""
	if kind == DeclarationMethod {
		scope = "instance:"
	}
	id := w.modulePath + "#" + string(kind) + ":" + scope + qualified
	return Declaration{
		ID: id, ModulePath: w.modulePath, OwnerID: ownerID, Kind: kind, Name: name, QualifiedName: qualified,
		Visibility: visibility(name), Location: jsonLocation(w.sourcePath, span),
	}
}

func (w *declarationWalker) add(declared Declaration) {
	if declared.Kind == DeclarationMethod && declared.ClassMember {
		declared.ID = strings.Replace(declared.ID, ":instance:", ":class:", 1)
	}
	w.result = append(w.result, declared)
}

func parameters(input []ir.Parameter) []Parameter {
	result := make([]Parameter, len(input))
	for index, parameter := range input {
		result[index] = Parameter{
			Name: parameter.Name, Type: protocolType(parameter.Type), NamedOnly: parameter.NamedOnly, Keyword: parameter.Keyword,
			Optional: parameter.Default != nil, Rest: parameter.Rest, KeywordRest: parameter.KeywordRest,
		}
	}
	return result
}

func protocolTypes(input []types.Type) []Type {
	result := make([]Type, len(input))
	for index, typ := range input {
		result[index] = protocolType(typ)
	}
	return result
}

func typePointer(input types.Type) *Type {
	result := protocolType(input)
	return &result
}

func protocolType(input types.Type) Type {
	result := Type{Kind: string(input.Kind), Name: input.Name, Nullable: input.Nullable, Readonly: input.Readonly}
	for _, argument := range input.Args {
		result.Arguments = append(result.Arguments, protocolType(argument))
	}
	return result
}

func visibility(name string) string {
	if strings.HasPrefix(strings.TrimPrefix(name, "@"), "_") {
		return "private"
	}
	return "public"
}

func compilerGenerated(artifact *compiler.Artifact, span token.Span) bool {
	return artifact.CompilerGeneratedStart > 0 && span.Start.Offset >= artifact.CompilerGeneratedStart
}

func jsonLocation(path string, span token.Span) diagnostic.JSONLocation {
	return diagnostic.JSONLocation{Path: cleanPath(path), Span: diagnostic.JSONSpan{
		Start: jsonPosition(span.Start), End: jsonPosition(span.End),
	}}
}

func jsonPosition(position token.Position) diagnostic.JSONPosition {
	return diagnostic.JSONPosition{Offset: position.Offset, Line: position.Line, Column: position.Column}
}

func cleanPath(path string) string {
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return filepath.Clean(path)
}
