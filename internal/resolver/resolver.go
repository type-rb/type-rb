// Package resolver turns syntax imports into project or compiler-known package
// identities before type checking. Backends therefore never interpret a raw
// TypeRB import path on their own.
package resolver

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type ImportKind string

const (
	StandardImport ImportKind = "standard"
	OfficialImport ImportKind = "official"
	ProjectImport  ImportKind = "project"
	NativeImport   ImportKind = "native"
)

type ExportKind string

const (
	ClassExport     ExportKind = "class"
	RecordExport    ExportKind = "record"
	EnumExport      ExportKind = "enum"
	TypeAliasExport ExportKind = "type_alias"
	ModuleExport    ExportKind = "module"
	InterfaceExport ExportKind = "interface"
	FunctionExport  ExportKind = "function"
	ValueExport     ExportKind = "value"
)

type Export struct {
	Name              string
	Kind              ExportKind
	Type              types.Type
	Fails             types.Type
	Parameters        []types.Type
	Required          int
	Variadic          bool
	Members           map[string]Member
	Fields            []RecordField
	EnumMembers       []string
	EnumVariants      []EnumVariant
	EnumRawType       types.Type
	TypeParameters    []string
	AliasTarget       types.Type
	AliasEnum         bool
	Superclass        string
	Interfaces        []string
	Span              token.Span
	UnsupportedFields map[string]string
	// NativeExported distinguishes a real target-package type export from a
	// provider-only structural record used to describe another declaration.
	NativeExported bool
}

type RecordField struct {
	Name     string
	JSONName string
	Type     types.Type
	Optional bool
}

type EnumVariant struct {
	Name     string
	Fields   []RecordField
	RawValue string
}

type Member struct {
	Name           string
	Kind           ExportKind
	Type           types.Type
	Fails          types.Type
	TypeParameters []string
	Parameters     []types.Type
	Required       int
	Variadic       bool
	Class          bool
	Readonly       bool
	EnumOwner      string
	Generated      string
}

type Import struct {
	Node       *ast.ImportStatement
	Kind       ImportKind
	Path       string
	ModulePath string
	Alias      string
	Symbols    []string
	Definition *stdlib.Package
	Exports    map[string]Export
	Filename   string
}

func (i *Import) RuntimePath() string {
	if i != nil && i.ModulePath != "" {
		return i.ModulePath
	}
	if i == nil {
		return ""
	}
	return i.Path
}

type Binding struct {
	Import  *Import
	Name    string
	Export  *Export
	Member  *Member
	Library *stdlib.Symbol
}

func (b Binding) Type() types.Type {
	if b.Library != nil {
		return b.Library.Return
	}
	if b.Member != nil {
		return b.Member.Type
	}
	if b.Export != nil {
		return b.Export.Type
	}
	return types.FromName("Any")
}

func (b Binding) FailureType() types.Type {
	if b.Library != nil {
		return b.Library.Fails
	}
	if b.Member != nil {
		return b.Member.Fails
	}
	if b.Export != nil {
		return b.Export.Fails
	}
	return types.Type{Kind: types.Never, Name: "Never"}
}

type Result struct {
	Imports      map[*ast.ImportStatement]*Import
	Packages     map[string]*Import
	Symbols      map[string]Binding
	NativeSyntax bool
	Declarations *declaration.Catalog
	Catalog      *Catalog
}

type Options struct {
	Mode           string
	SourceRoot     string
	Filename       string
	PackageAliases map[string]string
	CompilerOwned  bool
	Official       bool
	Catalog        *Catalog
	Declarations   *declaration.Catalog
	NativePackages *nativepackage.Catalog
}

type Module struct {
	Path          string
	Filename      string
	Program       *ast.Program
	Exports       map[string]Export
	CompilerOwned bool
	Official      bool
}

type Catalog struct {
	Modules            map[string]*Module
	CompilerOwnedTypes map[string]Export
}

func NewCatalog(modules []Module) (*Catalog, map[string][]diagnostic.Diagnostic) {
	catalog := &Catalog{Modules: map[string]*Module{}, CompilerOwnedTypes: map[string]Export{}}
	diagnostics := map[string][]diagnostic.Diagnostic{}
	for i := range modules {
		module := &modules[i]
		clean := pathpkg.Clean(strings.TrimSuffix(module.Path, ".trb"))
		module.Path = clean
		module.Exports = CollectExports(module.Program.Statements)
		if previous := catalog.Modules[clean]; previous != nil {
			diagnostics[module.Filename] = append(diagnostics[module.Filename], diagnostic.Diagnostic{
				Severity: diagnostic.Error,
				Message:  fmt.Sprintf("module path %s is already provided by %s", clean, previous.Filename),
				Span:     module.Program.Span(),
			})
			continue
		}
		catalog.Modules[clean] = module
	}
	typeOwners := map[string]*Module{}
	typesByName := map[string]*Export{}
	modulePaths := make([]string, 0, len(catalog.Modules))
	for modulePath := range catalog.Modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		module := catalog.Modules[modulePath]
		for name := range module.Exports {
			exported := module.Exports[name]
			if exported.Kind != ClassExport && exported.Kind != RecordExport && exported.Kind != EnumExport && exported.Kind != TypeAliasExport && exported.Kind != InterfaceExport {
				continue
			}
			if previous := typeOwners[name]; previous != nil {
				diagnostics[module.Filename] = append(diagnostics[module.Filename], diagnostic.Diagnostic{
					Severity: diagnostic.Error,
					Message:  fmt.Sprintf("exported type %s is already declared in %s", name, previous.Filename),
					Span:     exported.Span,
				})
				continue
			}
			copy := exported
			typeOwners[name] = module
			typesByName[name] = &copy
		}
	}
	state := map[string]int{}
	var link func(string)
	link = func(name string) {
		exported := typesByName[name]
		if exported == nil || state[name] == 2 {
			return
		}
		if state[name] == 1 {
			owner := typeOwners[name]
			diagnostics[owner.Filename] = append(diagnostics[owner.Filename], diagnostic.Diagnostic{Severity: diagnostic.Error, Message: "inheritance cycle involving " + name, Span: exported.Span})
			return
		}
		state[name] = 1
		if parent := typesByName[exported.Superclass]; parent != nil {
			link(parent.Name)
			for _, interfaceName := range parent.Interfaces {
				if !slices.Contains(exported.Interfaces, interfaceName) {
					exported.Interfaces = append(exported.Interfaces, interfaceName)
				}
			}
			for memberName, member := range parent.Members {
				if _, overridden := exported.Members[memberName]; !overridden {
					exported.Members[memberName] = member
				}
			}
		}
		state[name] = 2
	}
	typeNames := make([]string, 0, len(typesByName))
	for name := range typesByName {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)
	for _, name := range typeNames {
		link(name)
	}
	aliasState := map[string]int{}
	var linkAlias func(string)
	linkAlias = func(name string) {
		exported := typesByName[name]
		if exported == nil || exported.Kind != TypeAliasExport || aliasState[name] == 2 {
			return
		}
		if aliasState[name] == 1 {
			owner := typeOwners[name]
			diagnostics[owner.Filename] = append(diagnostics[owner.Filename], diagnostic.Diagnostic{Severity: diagnostic.Error, Message: "type alias cycle involving " + name, Span: exported.Span})
			return
		}
		aliasState[name] = 1
		target := typesByName[exported.AliasTarget.Name]
		if target != nil && target.Kind == TypeAliasExport {
			linkAlias(target.Name)
		}
		if target != nil {
			substitutions := typeSubstitutions(target.TypeParameters, exported.AliasTarget.Args)
			if target.Kind == TypeAliasExport {
				exported.AliasTarget = substituteType(target.AliasTarget, substitutions)
				target = typesByName[exported.AliasTarget.Name]
			}
			if target != nil {
				substitutions = typeSubstitutions(target.TypeParameters, exported.AliasTarget.Args)
				exported.Members = substituteMembers(target.Members, substitutions)
				exported.EnumMembers = append([]string(nil), target.EnumMembers...)
				exported.EnumVariants = substituteEnumVariants(target.EnumVariants, substitutions)
				exported.AliasEnum = target.Kind == EnumExport || target.Kind == TypeAliasExport && target.AliasEnum
			}
		}
		aliasState[name] = 2
	}
	for _, name := range typeNames {
		linkAlias(name)
	}
	for _, name := range typeNames {
		exported := typesByName[name]
		owner := typeOwners[name]
		owner.Exports[name] = *exported
		if owner.CompilerOwned || owner.Official {
			catalog.CompilerOwnedTypes[name] = *exported
		}
	}
	return catalog, diagnostics
}

func Resolve(program *ast.Program, options Options) (Result, []diagnostic.Diagnostic) {
	result := Result{
		Imports:      map[*ast.ImportStatement]*Import{},
		Packages:     map[string]*Import{},
		Symbols:      map[string]Binding{},
		Declarations: options.Declarations,
		Catalog:      options.Catalog,
	}
	var diagnostics []diagnostic.Diagnostic
	for _, statement := range program.Statements {
		node, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		resolved, items := resolveImport(node, options)
		diagnostics = append(diagnostics, items...)
		if resolved == nil {
			continue
		}
		result.Imports[node] = resolved
		if resolved.Definition != nil && resolved.Definition.NativeSyntax && resolved.Definition.Supports(options.Mode) {
			result.NativeSyntax = true
		}

		if resolved.Alias != "" {
			if previous := result.Packages[resolved.Alias]; previous != nil {
				diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("import alias %s is already used by %s", resolved.Alias, previous.Path)))
			} else {
				result.Packages[resolved.Alias] = resolved
			}
		}
		if resolved.Alias == "" || len(node.Symbols) > 0 {
			for _, name := range resolved.Symbols {
				binding, ok := bindingFor(resolved, name)
				if !ok {
					continue
				}
				if previous, exists := result.Symbols[name]; exists {
					diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("imported symbol %s conflicts with %s", name, previous.Import.Path)))
					continue
				}
				result.Symbols[name] = binding
			}
		}
	}
	addPrelude(&result)
	return result, diagnostics
}

// CompilerOwnedType returns declarations that were inferred from a portable
// library result. It supports member checking on inferred values without
// making the type name available to source annotations that omitted an import.
func (r Result) CompilerOwnedType(name string) (Export, bool) {
	if r.Catalog == nil {
		return Export{}, false
	}
	exported, ok := r.Catalog.CompilerOwnedTypes[name]
	return exported, ok
}

// addPrelude exposes the small set of Ruby-like, target-independent helpers
// that TypeRB programs can use without an import. The binding still points at
// a standard package so lowering produces the same intrinsic as io.puts().
func addPrelude(result *Result) {
	if _, exists := result.Symbols["puts"]; exists {
		return
	}
	definition, ok := stdlib.Lookup("trb/std/io")
	if !ok {
		return
	}
	imported := &Import{
		Kind:       StandardImport,
		Path:       definition.Path,
		Symbols:    []string{"puts"},
		Definition: definition,
		Exports:    map[string]Export{},
	}
	if binding, exists := bindingFor(imported, "puts"); exists {
		result.Symbols["puts"] = binding
	}
}

func (r Result) Member(alias, name string) (Binding, bool) {
	imported := r.Packages[alias]
	if imported == nil {
		return Binding{}, false
	}
	return bindingFor(imported, name)
}

// ReceiverMethod returns an implicit portable method binding. The binding is
// deliberately backed by the same standard package definition as its
// function form, even though TypeRB source does not need to import the method.
func (r Result) ReceiverMethod(receiver types.Type, name string) (Binding, bool) {
	imports := make([]*Import, 0, len(r.Imports))
	seen := map[*Import]bool{}
	for _, imported := range r.Imports {
		if imported == nil || imported.Definition == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		imports = append(imports, imported)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].Path < imports[j].Path })
	for _, imported := range imports {
		symbol, exists := imported.Definition.Symbols[name]
		if !exists || !symbol.HasReceiver() || receiver.Nullable || !stdlib.ReceiverMatches(symbol.Receiver, receiver, symbol.TypeParameters) {
			continue
		}
		copy := symbol
		copy.Receiver = receiver
		return Binding{Import: imported, Name: name, Library: &copy}, true
	}
	definition, symbol, ok := stdlib.LookupReceiverMethod(receiver, name)
	if !ok {
		return Binding{}, false
	}
	imported := &Import{
		Kind:       StandardImport,
		Path:       definition.Path,
		ModulePath: definition.ModulePath,
		Alias:      definition.DefaultAlias(),
		Definition: definition,
		Exports:    map[string]Export{},
	}
	return Binding{Import: imported, Name: name, Library: &symbol}, true
}

func (r Result) TypeMember(typeName, name string) (Binding, bool) {
	for _, binding := range r.Symbols {
		if binding.Export == nil || binding.Export.Name != typeName {
			continue
		}
		member, ok := binding.Export.Members[name]
		if !ok {
			return Binding{}, false
		}
		copy := member
		exported := *binding.Export
		return Binding{Import: binding.Import, Name: name, Export: &exported, Member: &copy}, true
	}
	for _, imported := range r.Packages {
		exported, exists := imported.Exports[typeName]
		if !exists {
			continue
		}
		member, exists := exported.Members[name]
		if !exists {
			return Binding{}, false
		}
		copy := member
		exportCopy := exported
		return Binding{Import: imported, Name: name, Export: &exportCopy, Member: &copy}, true
	}
	return Binding{}, false
}

func (r Result) ImportedType(typeName string) (Binding, bool) {
	for _, binding := range r.Symbols {
		if binding.Export != nil && binding.Export.Name == typeName && (binding.Export.Kind == ClassExport || binding.Export.Kind == RecordExport || binding.Export.Kind == EnumExport || binding.Export.Kind == TypeAliasExport || binding.Export.Kind == InterfaceExport) {
			return binding, true
		}
	}
	for _, imported := range r.Packages {
		exported, exists := imported.Exports[typeName]
		if exists && (exported.Kind == ClassExport || exported.Kind == RecordExport || exported.Kind == EnumExport || exported.Kind == TypeAliasExport || exported.Kind == InterfaceExport) {
			copy := exported
			return Binding{Import: imported, Name: typeName, Export: &copy}, true
		}
	}
	return Binding{}, false
}

// InferredType resolves a type that appears in the contract of an explicitly
// imported symbol without making that type available to source annotations.
// Source-visible type names continue to use ImportedType and therefore still
// require an explicit named or namespace import.
func (r Result) InferredType(typeName string) (Binding, bool) {
	imports := make([]*Import, 0, len(r.Imports))
	seen := map[*Import]bool{}
	for _, imported := range r.Imports {
		if imported == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		imports = append(imports, imported)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].Path < imports[j].Path })
	for _, imported := range imports {
		exported, exists := imported.Exports[typeName]
		if !exists || exported.Kind != ClassExport && exported.Kind != RecordExport && exported.Kind != EnumExport && exported.Kind != TypeAliasExport && exported.Kind != InterfaceExport {
			continue
		}
		copy := exported
		return Binding{Import: imported, Name: typeName, Export: &copy}, true
	}
	return Binding{}, false
}

// InferredTypeMember resolves a member on a type produced by an explicitly
// imported package even when the source did not import that type by name. This
// keeps inferred library values usable without weakening named-import rules
// for source annotations.
func (r Result) InferredTypeMember(typeName, memberName string) (Binding, bool) {
	imports := make([]*Import, 0, len(r.Imports))
	seen := map[*Import]bool{}
	for _, imported := range r.Imports {
		if imported == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		imports = append(imports, imported)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].Path < imports[j].Path })
	for _, imported := range imports {
		exported, exists := imported.Exports[typeName]
		if !exists {
			continue
		}
		member, exists := exported.Members[memberName]
		if !exists {
			return Binding{}, false
		}
		exportCopy := exported
		memberCopy := member
		return Binding{Import: imported, Name: memberName, Export: &exportCopy, Member: &memberCopy}, true
	}
	return Binding{}, false
}

// ValidateImportGraph rejects project import cycles with a deterministic path.
// A single cross-mode rule keeps initialization order independent of Ruby,
// TypeScript, or Go runtime loader behavior.
func ValidateImportGraph(catalog *Catalog, results map[string]Result) map[string][]diagnostic.Diagnostic {
	diagnostics := map[string][]diagnostic.Diagnostic{}
	if catalog == nil {
		return diagnostics
	}
	state := map[string]int{}
	var stack []string
	var visit func(string)
	visit = func(modulePath string) {
		state[modulePath] = 1
		stack = append(stack, modulePath)
		result := results[modulePath]
		imports := make([]*Import, 0, len(result.Imports))
		for _, imported := range result.Imports {
			if imported.Kind == ProjectImport && catalog.Modules[imported.Path] != nil {
				imports = append(imports, imported)
			}
		}
		sort.Slice(imports, func(i, j int) bool { return imports[i].Path < imports[j].Path })
		for _, imported := range imports {
			switch state[imported.Path] {
			case 0:
				visit(imported.Path)
			case 1:
				start := 0
				for start < len(stack) && stack[start] != imported.Path {
					start++
				}
				cycle := append(append([]string(nil), stack[start:]...), imported.Path)
				module := catalog.Modules[modulePath]
				diagnostics[module.Filename] = append(diagnostics[module.Filename], errorAt(imported.Node, "import cycle: "+strings.Join(cycle, " -> ")))
			}
		}
		stack = stack[:len(stack)-1]
		state[modulePath] = 2
	}
	paths := make([]string, 0, len(catalog.Modules))
	for modulePath := range catalog.Modules {
		paths = append(paths, modulePath)
	}
	sort.Strings(paths)
	for _, modulePath := range paths {
		if state[modulePath] == 0 {
			visit(modulePath)
		}
	}
	return diagnostics
}

func resolveImport(node *ast.ImportStatement, options Options) (*Import, []diagnostic.Diagnostic) {
	if definition, ok := stdlib.Lookup(node.Path); ok {
		return resolveDefinedImport(node, definition, StandardImport, options)
	}
	if bundled, ok := official.Lookup(node.Path); ok {
		return resolveDefinedImport(node, bundled.Definition, OfficialImport, options)
	}
	if stdlib.IsReservedPath(node.Path) {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("unknown TypeRB package %s", node.Path))}
	}
	if options.NativePackages != nil && options.NativePackages.Owns(node.Path) {
		if options.Mode != "typescript" {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("native package %s is only available in mode typescript", node.Path))}
		}
		if options.Catalog != nil && (options.Catalog.Modules[node.Path] != nil || options.Catalog.Modules[pathpkg.Join(node.Path, "index")] != nil) {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("import %s is provided by both a TypeRB package and a native TypeScript dependency", node.Path))}
		}
		return resolveNativeImport(node, options.NativePackages)
	}
	return resolveProjectImport(node, options)
}

func resolveNativeImport(node *ast.ImportStatement, catalog *nativepackage.Catalog) (*Import, []diagnostic.Diagnostic) {
	if catalog.UnavailableReason != "" {
		return nil, []diagnostic.Diagnostic{errorAt(node, catalog.UnavailableReason)}
	}
	module, ok := catalog.Module(node.Path)
	if !ok {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("native TypeScript package module %s is not indexed; run trb install", node.Path))}
	}
	if issue := module.Unsupported["*"]; issue != "" {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("native package %s cannot be represented safely: %s", node.Path, issue))}
	}
	resolved := &Import{Node: node, Kind: NativeImport, Path: node.Path, ModulePath: node.Path, Alias: node.Alias, Exports: map[string]Export{}}
	for name, exported := range module.Exports {
		resolved.Exports[name] = nativeExport(name, exported, true)
	}
	for name, exported := range module.Records {
		resolved.Exports[name] = nativeExport(name, exported, false)
	}
	if resolved.Alias == "" && len(node.Symbols) == 0 {
		resolved.Alias = defaultAlias(node.Path)
	}
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else if resolved.Alias == "" {
		for name := range module.Exports {
			resolved.Symbols = append(resolved.Symbols, name)
		}
	}
	for _, name := range resolved.Symbols {
		if issue := module.Unsupported[name]; issue != "" {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("native export %s from %s cannot be represented safely: %s; use a TypeRB provider for this package", name, node.Path, issue))}
		}
		if _, ok := module.Exports[name]; !ok {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("native package %s does not export %s", node.Path, name))}
		}
	}
	sort.Strings(resolved.Symbols)
	return resolved, nil
}

func nativeExport(name string, exported nativepackage.Export, nativeExported bool) Export {
	kind := ExportKind(exported.Kind)
	if exported.Kind == "component" {
		kind = FunctionExport
	}
	result := Export{
		Name:              name,
		Kind:              kind,
		Type:              exported.Type.Semantic(),
		Required:          exported.Required,
		Variadic:          exported.Variadic,
		TypeParameters:    append([]string(nil), exported.TypeParameters...),
		Members:           map[string]Member{},
		UnsupportedFields: cloneStrings(exported.UnsupportedFields),
		NativeExported:    nativeExported,
	}
	if exported.AliasTarget != nil {
		result.AliasTarget = exported.AliasTarget.Semantic()
	}
	for _, parameter := range exported.Parameters {
		result.Parameters = append(result.Parameters, parameter.Semantic())
	}
	for _, field := range exported.Fields {
		result.Fields = append(result.Fields, RecordField{Name: field.Name, JSONName: field.Name, Type: field.Type.Semantic(), Optional: field.Optional})
		result.Members[field.Name] = Member{Name: field.Name, Kind: ValueExport, Type: field.Type.Semantic(), Readonly: true}
	}
	return result
}

func cloneStrings(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for name, value := range input {
		result[name] = value
	}
	return result
}

func defaultAlias(importPath string) string {
	base := pathpkg.Base(importPath)
	base = strings.TrimPrefix(base, "@")
	base = strings.ReplaceAll(base, "-", "_")
	if base == "" || base == "." {
		return "package"
	}
	return base
}

func resolveDefinedImport(node *ast.ImportStatement, definition *stdlib.Package, kind ImportKind, options Options) (*Import, []diagnostic.Diagnostic) {
	if definition.Internal && !options.CompilerOwned && !options.Official {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s is internal to the TypeRB standard library", node.Path))}
	}
	resolved := &Import{
		Node:       node,
		Kind:       kind,
		Path:       node.Path,
		ModulePath: definition.ModulePath,
		Alias:      node.Alias,
		Definition: definition,
		Exports:    map[string]Export{},
	}
	if !definition.Supports(options.Mode) {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s does not support mode %s", node.Path, options.Mode))}
	}
	if definition.Source != "" {
		program, sourceDiagnostics := parser.Parse([]byte(definition.Source))
		for _, item := range sourceDiagnostics {
			if item.Severity == diagnostic.Error {
				return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s is invalid: %s", node.Path, item.Message))}
			}
		}
		resolved.Exports = CollectExports(program.Statements)
		if options.Catalog != nil {
			if module := options.Catalog.Modules[definition.ModulePath]; module != nil {
				resolved.Exports = module.Exports
			}
		}
	}
	if definition.RuntimeAlias != "" {
		resolved.Alias = definition.RuntimeAlias
	} else if resolved.Alias == "" && len(node.Symbols) == 0 {
		resolved.Alias = definition.DefaultAlias()
	}
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else {
		seen := map[string]bool{}
		for name := range definition.Symbols {
			resolved.Symbols = append(resolved.Symbols, name)
			seen[name] = true
		}
		for name := range resolved.Exports {
			if seen[name] {
				continue
			}
			resolved.Symbols = append(resolved.Symbols, name)
		}
	}
	for _, name := range resolved.Symbols {
		_, librarySymbol := definition.Symbols[name]
		_, sourceExport := resolved.Exports[name]
		if !librarySymbol && !sourceExport {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s does not export %s", node.Path, name))}
		}
	}
	sort.Strings(resolved.Symbols)
	return resolved, nil
}

func resolveProjectImport(node *ast.ImportStatement, options Options) (*Import, []diagnostic.Diagnostic) {
	clean := pathpkg.Clean(strings.TrimSuffix(node.Path, ".trb"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || pathpkg.IsAbs(clean) {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("invalid project import path %q", node.Path))}
	}
	canonical := canonicalPackageImport(clean, options.PackageAliases)
	resolved := &Import{Node: node, Kind: ProjectImport, Path: canonical, Alias: node.Alias, Exports: map[string]Export{}}
	if options.Catalog != nil {
		module := options.Catalog.Modules[canonical]
		if module == nil {
			module = options.Catalog.Modules[pathpkg.Join(canonical, "index")]
		}
		if module != nil {
			resolved.Path = module.Path
			resolved.Filename = module.Filename
			resolved.Exports = module.Exports
			return finalizeProjectImport(resolved)
		}
		// Ruby projects may explicitly import an opaque .rb file which is not a
		// TypeRB catalog unit; fall through to the filesystem lookup for it.
		if options.Mode != "ruby" {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("cannot resolve project import %s", node.Path))}
		}
	}
	if options.SourceRoot == "" {
		// The standalone compiler API can still lower a project import. Project
		// builds pass SourceRoot and receive existence/export validation.
		resolved.Symbols = append([]string(nil), node.Symbols...)
		return resolved, nil
	}
	candidates := []string{filepath.Join(options.SourceRoot, filepath.FromSlash(clean)+".trb")}
	if options.Mode == "ruby" {
		candidates = append(candidates, filepath.Join(options.SourceRoot, filepath.FromSlash(clean)+".rb"))
	}
	var data []byte
	for _, candidate := range candidates {
		contents, err := os.ReadFile(candidate)
		if err == nil {
			resolved.Filename = candidate
			data = contents
			break
		}
		if !os.IsNotExist(err) {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("cannot import %s: %v", node.Path, err))}
		}
	}
	if resolved.Filename == "" {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("cannot resolve project import %s", node.Path))}
	}
	if strings.HasSuffix(resolved.Filename, ".trb") {
		imported, parseDiagnostics := parser.Parse(data)
		if hasErrors(parseDiagnostics) {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("cannot import %s because it contains syntax errors", node.Path))}
		}
		resolved.Exports = CollectExports(imported.Statements)
	} else {
		// Opaque Ruby files need named imports for reliable constant typing. For
		// the common Zeitwerk case, infer the constant from the file basename.
		name := constantName(pathpkg.Base(clean))
		resolved.Exports[name] = Export{Name: name, Kind: ClassExport, Type: types.FromName(name)}
	}

	return finalizeProjectImport(resolved)
}

func canonicalPackageImport(importPath string, aliases map[string]string) string {
	selected := ""
	canonical := ""
	for alias, packageName := range aliases {
		if importPath != alias && !strings.HasPrefix(importPath, alias+"/") {
			continue
		}
		if len(alias) > len(selected) {
			selected = alias
			canonical = packageName + strings.TrimPrefix(importPath, alias)
		}
	}
	if canonical != "" {
		return canonical
	}
	return importPath
}

func finalizeProjectImport(resolved *Import) (*Import, []diagnostic.Diagnostic) {
	node := resolved.Node
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else if resolved.Alias == "" {
		for name := range resolved.Exports {
			resolved.Symbols = append(resolved.Symbols, name)
		}
	}
	for _, name := range resolved.Symbols {
		if _, ok := resolved.Exports[name]; !ok {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("project module %s does not export %s", node.Path, name))}
		}
	}
	sort.Strings(resolved.Symbols)
	return resolved, nil
}

func bindingFor(imported *Import, name string) (Binding, bool) {
	if imported.Definition != nil {
		if symbol, ok := imported.Definition.Symbols[name]; ok {
			copy := symbol
			return Binding{Import: imported, Name: name, Library: &copy}, true
		}
	}
	if exported, ok := imported.Exports[name]; ok {
		copy := exported
		return Binding{Import: imported, Name: name, Export: &copy}, true
	}
	return Binding{}, false
}

func CollectExports(statements []ast.Statement) map[string]Export {
	result := map[string]Export{}
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ast.ClassStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: ClassExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Superclass: expressionName(node.Superclass), Interfaces: append([]string(nil), node.Implements...), Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				for _, member := range node.Body {
					switch item := member.(type) {
					case *ast.MethodStatement:
						parameterTypes, required, variadic := parameters(item.Parameters)
						if item.Name == "initialize" {
							exported.Parameters, exported.Required, exported.Variadic = parameterTypes, required, variadic
							continue
						}
						if public(item.Name) {
							method := Member{Name: item.Name, Kind: FunctionExport, Type: returnTypeRef(item.ReturnType), Fails: failureTypeRef(item.Fails), Parameters: parameterTypes, Required: required, Variadic: variadic, Class: item.Class}
							for _, parameter := range item.TypeParameters {
								method.TypeParameters = append(method.TypeParameters, parameter.Name)
							}
							exported.Members[item.Name] = method
						}
					case *ast.FieldStatement:
						name := strings.TrimPrefix(item.Name, "@")
						if public(name) {
							exported.Members[name] = Member{Name: name, Kind: ValueExport, Type: typeRef(item.Type), Readonly: item.ReadOnly}
						}
					case *ast.VariableStatement:
						if item.Constant && public(item.Name) {
							exported.Members[item.Name] = Member{Name: item.Name, Kind: ValueExport, Type: variableType(item), Class: true}
						}
					}
				}
				result[node.Name] = exported
			}
		case *ast.RecordStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: RecordExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				for _, member := range node.Body {
					field, ok := member.(*ast.RecordFieldStatement)
					if !ok {
						continue
					}
					typ := typeRef(field.Type)
					exported.Fields = append(exported.Fields, RecordField{Name: field.Name, JSONName: recordJSONName(field), Type: typ})
					exported.Members[field.Name] = Member{Name: field.Name, Kind: ValueExport, Type: typ}
				}
				result[node.Name] = exported
			}
		case *ast.EnumStatement:
			if public(node.Name) {
				typ := types.FromName(node.Name)
				exported := Export{Name: node.Name, Kind: EnumExport, Type: typ, Members: map[string]Member{}, Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				raw := false
				for _, statement := range node.Body {
					if member, ok := statement.(*ast.EnumMemberStatement); ok && member.RawValue != nil {
						raw = true
						break
					}
				}
				for _, statement := range node.Body {
					switch member := statement.(type) {
					case *ast.EnumMemberStatement:
						if !public(member.Name) {
							continue
						}
						exported.EnumMembers = append(exported.EnumMembers, member.Name)
						variant := EnumVariant{Name: member.Name, RawValue: rawExpression(member.RawValue)}
						parameterTypes := make([]types.Type, 0, len(member.Parameters))
						for _, parameter := range member.Parameters {
							fieldType := typeRef(parameter.Type)
							variant.Fields = append(variant.Fields, RecordField{Name: parameter.Name, Type: fieldType})
							parameterTypes = append(parameterTypes, fieldType)
						}
						exported.EnumVariants = append(exported.EnumVariants, variant)
						kind := ValueExport
						if len(parameterTypes) > 0 {
							kind = FunctionExport
						}
						exported.Members[member.Name] = Member{Name: member.Name, Kind: kind, Type: typ, Parameters: parameterTypes, Required: len(parameterTypes), Class: true}
					case *ast.MethodStatement:
						if public(member.Name) {
							parameterTypes, required, variadic := parameters(member.Parameters)
							exported.Members[member.Name] = Member{Name: member.Name, Kind: FunctionExport, Type: returnTypeRef(member.ReturnType), Fails: failureTypeRef(member.Fails), Parameters: parameterTypes, Required: required, Variadic: variadic, EnumOwner: node.Name}
						}
					}
				}
				if raw {
					exported.EnumRawType = enumRawType(node)
					exported.Members["raw_value"] = Member{Name: "raw_value", Kind: FunctionExport, Type: exported.EnumRawType, EnumOwner: node.Name, Generated: "raw_value"}
					exported.Members["from_raw"] = Member{Name: "from_raw", Kind: FunctionExport, Type: types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typ, types.FromName("EnumValueError")}}, Parameters: []types.Type{exported.EnumRawType}, Required: 1, Class: true, EnumOwner: node.Name, Generated: "from_raw"}
				}
				result[node.Name] = exported
			}
		case *ast.TypeAliasStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: TypeAliasExport, Type: types.FromName(node.Name), AliasTarget: typeRef(node.Target), Members: map[string]Member{}, Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				result[node.Name] = exported
			}
		case *ast.ModuleStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: ModuleExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Span: node.Span()}
				for _, statement := range node.Body {
					if value, ok := statement.(*ast.VariableStatement); ok && value.Constant && public(value.Name) {
						exported.Members[value.Name] = Member{Name: value.Name, Kind: ValueExport, Type: variableType(value), Class: true}
					}
				}
				result[node.Name] = exported
			}
		case *ast.InterfaceStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: InterfaceExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Span: node.Span()}
				for _, method := range node.Methods {
					parameterTypes, required, variadic := parameters(method.Parameters)
					exported.Members[method.Name] = Member{Name: method.Name, Kind: FunctionExport, Type: returnTypeRef(method.ReturnType), Fails: failureTypeRef(method.Fails), Parameters: parameterTypes, Required: required, Variadic: variadic}
				}
				result[node.Name] = exported
			}
		case *ast.MethodStatement:
			if public(node.Name) {
				parameterTypes, required, variadic := parameters(node.Parameters)
				exported := Export{Name: node.Name, Kind: FunctionExport, Type: returnTypeRef(node.ReturnType), Fails: failureTypeRef(node.Fails), Parameters: parameterTypes, Required: required, Variadic: variadic, Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				result[node.Name] = exported
			}
		case *ast.VariableStatement:
			if node.Constant && public(node.Name) {
				result[node.Name] = Export{Name: node.Name, Kind: ValueExport, Type: variableType(node), Span: node.Span()}
			}
		}
	}
	return result
}

func recordJSONName(field *ast.RecordFieldStatement) string {
	for _, attribute := range field.Attributes {
		if attribute.Name != "json" || len(attribute.Arguments) == 0 {
			continue
		}
		literal, ok := attribute.Arguments[0].Value.(*ast.Literal)
		if !ok || literal.Kind != ast.StringLiteral {
			continue
		}
		value, err := strconv.Unquote(literal.Raw)
		if err == nil {
			return strings.Split(value, ",")[0]
		}
	}
	return field.Name
}

func variableType(node *ast.VariableStatement) types.Type {
	if node == nil {
		return types.FromName("Any")
	}
	if !node.Type.Empty() {
		return typeRef(node.Type)
	}
	switch value := node.Value.(type) {
	case *ast.Literal:
		switch value.Kind {
		case ast.StringLiteral:
			return types.FromName("String")
		case ast.IntegerLiteral:
			return types.FromName("Integer")
		case ast.FloatLiteral:
			return types.FromName("Float")
		case ast.BooleanLiteral:
			return types.FromName("Boolean")
		case ast.NilLiteral:
			return types.FromName("Nil")
		}
	case *ast.ArrayLiteral:
		element := types.FromName("Any")
		if len(value.Elements) > 0 {
			element = expressionLiteralType(value.Elements[0])
			for _, expression := range value.Elements[1:] {
				current := expressionLiteralType(expression)
				joined, ok := types.CommonType(element, current)
				if !ok {
					element = types.FromName("Any")
					break
				}
				element = joined
			}
		}
		return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{element}}
	case *ast.HashLiteral:
		if len(value.Entries) == 0 {
			return types.FromName("Hash")
		}
		key := expressionLiteralType(value.Entries[0].Key)
		item := expressionLiteralType(value.Entries[0].Value)
		for _, entry := range value.Entries[1:] {
			if current := expressionLiteralType(entry.Key); !types.Equivalent(key, current) {
				key = types.FromName("Any")
			}
			if current := expressionLiteralType(entry.Value); !types.Equivalent(item, current) {
				joined, ok := types.CommonType(item, current)
				if !ok {
					item = types.FromName("Any")
				} else {
					item = joined
				}
			}
		}
		return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{key, item}}
	case *ast.RangeExpression:
		return types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}
	case *ast.InterpolatedString, *ast.SymbolLiteral:
		return types.FromName("String")
	}
	return types.FromName("Any")
}

func expressionLiteralType(expression ast.Expression) types.Type {
	node := &ast.VariableStatement{Value: expression}
	return variableType(node)
}

func parameters(nodes []ast.Parameter) ([]types.Type, int, bool) {
	result := make([]types.Type, len(nodes))
	required := 0
	variadic := false
	for i, parameter := range nodes {
		result[i] = typeRef(parameter.Type)
		variadic = variadic || parameter.Rest || parameter.KeywordRest
		if parameter.Default == nil && !parameter.Rest && !parameter.KeywordRest {
			required++
		}
	}
	return result, required, variadic
}

func expressionName(expression ast.Expression) string {
	switch node := expression.(type) {
	case *ast.Identifier:
		return node.Name
	case *ast.MemberExpression:
		prefix := expressionName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	default:
		return ""
	}
}

func typeRef(ref ast.TypeRef) types.Type {
	if ref.Empty() {
		return types.FromName("Any")
	}
	if len(ref.Union) > 0 {
		alternatives := make([]types.Type, len(ref.Union))
		for index, alternative := range ref.Union {
			alternatives[index] = typeRef(alternative)
		}
		return types.UnionOf(alternatives...)
	}
	if ref.FunctionReturn != nil {
		parameters := make([]types.Type, len(ref.FunctionParameters))
		for index, parameter := range ref.FunctionParameters {
			parameters[index] = typeRef(parameter)
		}
		failure := types.Type{Kind: types.Never, Name: "Never"}
		if ref.FunctionFails != nil {
			failure = typeRef(*ref.FunctionFails)
		}
		result := types.FunctionWithEffect(parameters, typeRef(*ref.FunctionReturn), failure)
		result.Nullable = ref.Nullable
		return result
	}
	result := types.FromName(ref.Name)
	result.Nullable = ref.Nullable
	for _, argument := range ref.Arguments {
		result.Args = append(result.Args, typeRef(argument))
	}
	if ref.Array {
		result = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{result}, Nullable: ref.Nullable}
	}
	return result
}

func typeSubstitutions(parameters []string, arguments []types.Type) map[string]types.Type {
	result := map[string]types.Type{}
	for index, parameter := range parameters {
		if index < len(arguments) {
			result[parameter] = arguments[index]
		}
	}
	return result
}

func substituteType(typ types.Type, substitutions map[string]types.Type) types.Type {
	if replacement, ok := substitutions[typ.Name]; ok && typ.Kind == types.Named && len(typ.Args) == 0 {
		replacement.Nullable = replacement.Nullable || typ.Nullable
		replacement.Readonly = replacement.Readonly || typ.Readonly
		return replacement
	}
	result := typ
	result.Args = make([]types.Type, len(typ.Args))
	for index, argument := range typ.Args {
		result.Args[index] = substituteType(argument, substitutions)
	}
	if typ.Fails != nil {
		failure := substituteType(*typ.Fails, substitutions)
		result.Fails = &failure
	}
	return result
}

func substituteMembers(input map[string]Member, substitutions map[string]types.Type) map[string]Member {
	result := make(map[string]Member, len(input))
	for name, member := range input {
		copy := member
		copy.Type = substituteType(member.Type, substitutions)
		copy.Parameters = make([]types.Type, len(member.Parameters))
		for index, parameter := range member.Parameters {
			copy.Parameters[index] = substituteType(parameter, substitutions)
		}
		result[name] = copy
	}
	return result
}

func substituteEnumVariants(input []EnumVariant, substitutions map[string]types.Type) []EnumVariant {
	result := make([]EnumVariant, len(input))
	for index, variant := range input {
		result[index].Name = variant.Name
		result[index].RawValue = variant.RawValue
		result[index].Fields = make([]RecordField, len(variant.Fields))
		for fieldIndex, field := range variant.Fields {
			copy := field
			copy.Type = substituteType(field.Type, substitutions)
			result[index].Fields[fieldIndex] = copy
		}
	}
	return result
}

func rawExpression(expression ast.Expression) string {
	switch value := expression.(type) {
	case *ast.Literal:
		if value.Kind == ast.StringLiteral || value.Kind == ast.IntegerLiteral {
			return value.Raw
		}
	case *ast.UnaryExpression:
		if literal, ok := value.Operand.(*ast.Literal); value.Operator == "-" && ok && literal.Kind == ast.IntegerLiteral {
			return "-" + literal.Raw
		}
	}
	return ""
}

func enumRawType(enum *ast.EnumStatement) types.Type {
	for _, statement := range enum.Body {
		member, ok := statement.(*ast.EnumMemberStatement)
		if !ok || member.RawValue == nil {
			continue
		}
		switch value := member.RawValue.(type) {
		case *ast.Literal:
			if value.Kind == ast.StringLiteral {
				return types.FromName("String")
			}
			if value.Kind == ast.IntegerLiteral {
				return types.FromName("Integer")
			}
		case *ast.UnaryExpression:
			return types.FromName("Integer")
		}
	}
	return types.Type{}
}

func returnTypeRef(ref ast.TypeRef) types.Type {
	if ref.Empty() {
		return types.FromName("Void")
	}
	return typeRef(ref)
}

func failureTypeRef(ref ast.TypeRef) types.Type {
	if ref.Empty() {
		return types.Type{Kind: types.Never, Name: "Never"}
	}
	return typeRef(ref)
}

func constantName(value string) string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == '_' || r == '-' })
	for i := range parts {
		if parts[i] != "" {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

func public(name string) bool { return name != "" && !strings.HasPrefix(name, "_") }

func hasErrors(items []diagnostic.Diagnostic) bool {
	for _, item := range items {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}

func errorAt(node ast.Node, message string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Severity: diagnostic.Error, Message: message, Span: node.Span()}
}
