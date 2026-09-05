// Package resolver turns syntax imports into project or compiler-known package
// identities before type checking. Backends therefore never interpret a raw
// TypeRB import path on their own.
package resolver

import (
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
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
	NewtypeExport   ExportKind = "newtype"
	ModuleExport    ExportKind = "module"
	InterfaceExport ExportKind = "interface"
	FunctionExport  ExportKind = "function"
	ValueExport     ExportKind = "value"
)

type Export struct {
	Name                   string
	Kind                   ExportKind
	Source                 bool
	Type                   types.Type
	Parameters             []callsignature.Parameter
	ParameterResultBridges []NativeResultBridge
	CallResultBridge       NativeCallResultBridge
	Variadic               bool
	Members                map[string]Member
	Nested                 map[string]Export
	Fields                 []RecordField
	EnumMembers            []string
	EnumVariants           []EnumVariant
	EnumRawType            types.Type
	TypeParameters         []string
	AliasTarget            types.Type
	AliasEnum              bool
	NewtypeTarget          types.Type
	NewtypePrivateNew      bool
	Superclass             string
	Interfaces             []types.Type
	Span                   token.Span
	UnsupportedFields      map[string]string
	// NativeExported distinguishes a real target-package type export from a
	// provider-only structural record used to describe another declaration.
	NativeExported bool
	Runtime        *RuntimeBinding
}

type RuntimeBinding struct {
	Identity                 string
	Dependency               string
	Module                   string
	Symbol                   string
	CallConvention           string
	MaySuspend               bool
	PropagatesExecutionScope bool
}

type RecordField struct {
	Name         string
	JSONName     string
	Type         types.Type
	Optional     bool
	NamedOnly    bool
	HasDefault   bool
	ResultBridge NativeResultBridge
}

// NativeResultBridge keeps both sides of a package-owned callback boundary.
// Type is the native Promise success signature, while Error is the TypeRB
// Result error payload accepted by the adapter.
type NativeResultBridge struct {
	Kind  string
	Type  types.Type
	Error types.Type
}

// NativeCallResultBridge converts one native call's Promise settlement into a
// checked TypeRB Result. Unlike NativeResultBridge, it applies to the callee's
// return boundary rather than to a callback parameter.
type NativeCallResultBridge struct {
	Kind  string
	Error types.Type
}

type EnumVariant struct {
	Name     string
	Fields   []RecordField
	RawValue string
}

type Member struct {
	Name              string
	Kind              ExportKind
	Type              types.Type
	TypeParameters    []string
	Parameters        []callsignature.Parameter
	Variadic          bool
	Class             bool
	Readonly          bool
	EnumOwner         string
	Generated         string
	CallResultBridge  NativeCallResultBridge
	UnsupportedFields map[string]string
}

type Import struct {
	Node                *ast.ImportStatement
	ActivationNode      *ast.ActivateStatement
	Kind                ImportKind
	Path                string
	ModulePath          string
	Alias               string
	Symbols             []string
	SymbolAliases       map[string]string
	Definition          *stdlib.Package
	Exports             map[string]Export
	Filename            string
	CompilerGenerated   bool
	DeclarationProvider bool
	RootStable          bool
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

func (i *Import) LocalName(name string) string {
	if i == nil {
		return name
	}
	if alias := i.SymbolAliases[name]; alias != "" {
		return alias
	}
	return name
}

type Binding struct {
	Import  *Import
	Name    string
	Export  *Export
	Member  *Member
	Library *stdlib.Symbol
}

// DeclarationIdentity returns the declaration that owns this resolved
// binding. Member bindings return their exact type owner; DispatchIdentity
// carries the member name and class/instance namespace separately.
func (b Binding) DeclarationIdentity() identity.Declaration {
	if b.Import == nil {
		return identity.Declaration{}
	}
	name := b.Name
	kind := identity.Value
	if b.Library != nil {
		kind = identity.Function
	}
	if b.Export != nil {
		name = b.Export.Name
		kind = identityKind(b.Export.Kind)
	}
	if name == "" {
		return identity.Declaration{}
	}
	return identity.Declaration{Module: b.Import.RuntimePath(), Name: name, Kind: kind}
}

// DispatchIdentity returns an exact imported member target. Top-level
// functions and values have no dispatch identity.
func (b Binding) DispatchIdentity() identity.Dispatch {
	if b.Member == nil {
		return identity.Dispatch{}
	}
	return identity.Dispatch{
		Owner: b.DeclarationIdentity(),
		Name:  b.Member.Name,
		Class: b.Member.Class,
	}
}

func identityKind(kind ExportKind) identity.Kind {
	switch kind {
	case ClassExport:
		return identity.Class
	case RecordExport:
		return identity.Record
	case EnumExport:
		return identity.Enum
	case TypeAliasExport:
		return identity.TypeAlias
	case NewtypeExport:
		return identity.Newtype
	case ModuleExport:
		return identity.Module
	case InterfaceExport:
		return identity.Interface
	case FunctionExport:
		return identity.Function
	default:
		return identity.Value
	}
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

type Result struct {
	Imports          map[*ast.ImportStatement]*Import
	Activations      map[*ast.ActivateStatement]*Import
	Capabilities     []*Import
	Packages         map[string]*Import
	Symbols          map[string]Binding
	GeneratedSymbols map[string]Binding
	NativeSyntax     bool
	Declarations     *declaration.Catalog
	Catalog          *Catalog
}

type Options struct {
	Mode                   string
	SourceRoot             string
	Filename               string
	PackageAliases         map[string]string
	CompilerOwned          bool
	Official               bool
	Catalog                *Catalog
	Declarations           *declaration.Catalog
	NativePackages         *nativepackage.Catalog
	CompilerGeneratedStart int
}

type Module struct {
	Path                string
	Filename            string
	Program             *ast.Program
	Exports             map[string]Export
	CompilerOwned       bool
	Official            bool
	DeclarationProvider bool
}

type Catalog struct {
	Modules            map[string]*Module
	CompilerOwnedTypes map[string]Export
	typeAliases        map[string]Export
	ambiguousTypes     map[string]bool
}

func NewCatalog(modules []Module) (*Catalog, map[string][]diagnostic.Diagnostic) {
	catalog := &Catalog{
		Modules:            map[string]*Module{},
		CompilerOwnedTypes: map[string]Export{},
		typeAliases:        map[string]Export{},
		ambiguousTypes:     map[string]bool{},
	}
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
		peer := strings.TrimSuffix(clean, "/index")
		if peer == clean {
			peer = pathpkg.Join(clean, "index")
		}
		if previous := catalog.Modules[peer]; previous != nil {
			diagnostics[module.Filename] = append(diagnostics[module.Filename], diagnostic.Diagnostic{
				Severity: diagnostic.Error,
				Message:  fmt.Sprintf("module paths %s and %s define the same import root; provided by %s and %s", clean, peer, module.Filename, previous.Filename),
				Span:     module.Program.Span(),
			})
			continue
		}
		if !module.DeclarationProvider {
			wanted := pathRootKey(logicalImportSegment(clean))
			var roots []string
			for name, exported := range module.Exports {
				if rootEligible(exported.Kind) && declarationRootKey(name) == wanted {
					roots = append(roots, name)
				}
			}
			if len(roots) > 1 {
				sort.Strings(roots)
				diagnostics[module.Filename] = append(diagnostics[module.Filename], diagnostic.Diagnostic{
					Severity: diagnostic.Error,
					Message:  fmt.Sprintf("root-stable module %s declares multiple matching roots: %s", clean, strings.Join(roots, ", ")),
					Span:     module.Program.Span(),
				})
			}
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
		for name, exported := range flattenExports(module.Exports) {
			if exported.Kind != ClassExport && exported.Kind != RecordExport && exported.Kind != EnumExport && exported.Kind != TypeAliasExport && exported.Kind != NewtypeExport && exported.Kind != InterfaceExport {
				continue
			}
			if typeOwners[name] != nil || catalog.ambiguousTypes[name] {
				delete(typeOwners, name)
				delete(typesByName, name)
				catalog.ambiguousTypes[name] = true
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
			for _, implemented := range parent.Interfaces {
				if !containsEquivalentType(exported.Interfaces, implemented) {
					exported.Interfaces = append(exported.Interfaces, implemented)
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
		setExport(owner.Exports, name, *exported)
		if exported.Kind == TypeAliasExport {
			catalog.typeAliases[name] = *exported
		}
		if owner.CompilerOwned || owner.Official {
			catalog.CompilerOwnedTypes[name] = *exported
		}
	}
	for filename, items := range diagnostics {
		diagnostics[filename] = diagnostic.Normalize(items, filename, diagnostic.ResolutionError)
	}
	return catalog, diagnostics
}

func flattenExports(exports map[string]Export) map[string]Export {
	result := map[string]Export{}
	var visit func(map[string]Export)
	visit = func(current map[string]Export) {
		for _, exported := range current {
			result[exported.Name] = exported
			visit(exported.Nested)
		}
	}
	visit(exports)
	return result
}

func setExport(exports map[string]Export, name string, replacement Export) bool {
	for key, exported := range exports {
		if exported.Name == name {
			exports[key] = replacement
			return true
		}
		if setExport(exported.Nested, name, replacement) {
			exports[key] = exported
			return true
		}
	}
	return false
}

func Resolve(program *ast.Program, options Options) (Result, []diagnostic.Diagnostic) {
	result := Result{
		Imports:          map[*ast.ImportStatement]*Import{},
		Activations:      map[*ast.ActivateStatement]*Import{},
		Packages:         map[string]*Import{},
		Symbols:          map[string]Binding{},
		GeneratedSymbols: map[string]Binding{},
		Declarations:     options.Declarations,
		Catalog:          options.Catalog,
	}
	var diagnostics []diagnostic.Diagnostic
	boundIdentities := map[string]string{}
	generatedBoundIdentities := map[string]string{}
	activatedTargets := map[string]ast.Node{}
	for _, statement := range program.Statements {
		switch node := statement.(type) {
		case *ast.ActivateStatement:
			generated := options.CompilerGeneratedStart > 0 && node.Span().Start.Offset >= options.CompilerGeneratedStart
			if generated {
				diagnostics = append(diagnostics, errorAt(node, "compiler-generated source cannot activate capabilities"))
				continue
			}
			resolved, items := resolveActivation(node, options)
			diagnostics = append(diagnostics, items...)
			if resolved == nil {
				continue
			}
			if previous := activatedTargets[resolved.RuntimePath()]; previous != nil {
				diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("capabilities from %s are already enabled in this module", resolved.Path)))
				continue
			}
			activatedTargets[resolved.RuntimePath()] = node
			result.Activations[node] = resolved
			result.Capabilities = append(result.Capabilities, resolved)
			if resolved.Definition != nil && resolved.Definition.NativeSyntax && resolved.Definition.Supports(options.Mode) {
				result.NativeSyntax = true
			}
			continue
		case *ast.ImportStatement:
			resolved, items := resolveImport(node, options)
			diagnostics = append(diagnostics, items...)
			if resolved == nil {
				continue
			}
			resolved.CompilerGenerated = options.CompilerGeneratedStart > 0 && node.Span().Start.Offset >= options.CompilerGeneratedStart
			result.Imports[node] = resolved
			if !resolved.CompilerGenerated && importHasCapability(resolved, options.Mode) {
				if previous := activatedTargets[resolved.RuntimePath()]; previous != nil {
					diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("capabilities from %s are already enabled in this module", resolved.Path)))
				} else {
					activatedTargets[resolved.RuntimePath()] = node
					result.Capabilities = append(result.Capabilities, resolved)
					if resolved.Definition != nil && resolved.Definition.NativeSyntax && resolved.Definition.Supports(options.Mode) {
						result.NativeSyntax = true
					}
				}
			}
			symbols := result.Symbols
			identities := boundIdentities
			if resolved.CompilerGenerated {
				symbols = result.GeneratedSymbols
				identities = generatedBoundIdentities
			}
			for _, name := range resolved.Symbols {
				binding, ok := bindingFor(resolved, name)
				if !ok {
					continue
				}
				localName := resolved.LocalName(name)
				identityKey := bindingIdentityKey(binding)
				if previousLocal := identities[identityKey]; previousLocal != "" {
					diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("declaration %s from %s is already imported as %s", name, resolved.Path, previousLocal)))
					continue
				}
				if previous, exists := symbols[localName]; exists {
					diagnostics = append(diagnostics, errorAt(node, fmt.Sprintf("imported binding %s conflicts with %s", localName, previous.Import.Path)))
					continue
				}
				symbols[localName] = binding
				identities[identityKey] = localName
				bindNestedSymbols(symbols, identities, resolved, binding, localName)
			}
		}
	}
	addPrelude(&result)
	return result, diagnostic.Normalize(diagnostics, options.Filename, diagnostic.ResolutionError)
}

func bindNestedSymbols(symbols map[string]Binding, identities map[string]string, imported *Import, owner Binding, localOwner string) {
	if owner.Export == nil {
		return
	}
	names := make([]string, 0, len(owner.Export.Nested))
	for name := range owner.Export.Nested {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		nested := owner.Export.Nested[name]
		copy := nested
		binding := Binding{Import: imported, Name: nested.Name, Export: &copy}
		localName := localOwner + "::" + name
		if _, exists := symbols[localName]; !exists {
			symbols[localName] = binding
			identities[bindingIdentityKey(binding)] = localName
		}
		bindNestedSymbols(symbols, identities, imported, binding, localName)
	}
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

// CatalogTypeAlias resolves a transparent alias from the complete project
// catalog for semantic expansion only. Imported value contracts retain their
// declared alias names even when the alias is owned by another module, so the
// checker needs this lookup to interpret the value without making the alias
// name available to source annotations. NewCatalog rejects duplicate exported
// type names, which keeps this lookup unambiguous.
func (r Result) CatalogTypeAlias(name string) (Export, bool) {
	if r.Catalog == nil {
		return Export{}, false
	}
	if r.Catalog.typeAliases != nil {
		exported, ok := r.Catalog.typeAliases[name]
		return exported, ok
	}
	for _, module := range r.Catalog.Modules {
		exported, ok := exportNamed(module.Exports, name)
		if ok && exported.Kind == TypeAliasExport {
			return exported, true
		}
	}
	return Export{}, false
}

// CatalogType resolves a type declaration and its owner from the complete
// project catalog. NewCatalog rejects duplicate exported type names, so the
// result is unambiguous. This lookup is for compiler-generated references;
// source annotations must continue to use ImportedType.
func (r Result) CatalogType(name string) (Binding, bool) {
	if r.Catalog == nil {
		return Binding{}, false
	}
	if r.Catalog.ambiguousTypes[name] {
		return Binding{}, false
	}
	modulePaths := make([]string, 0, len(r.Catalog.Modules))
	for modulePath := range r.Catalog.Modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		module := r.Catalog.Modules[modulePath]
		if module == nil {
			continue
		}
		exported, exists := exportNamed(module.Exports, name)
		if !exists || !typeExport(exported.Kind) {
			continue
		}
		kind := ProjectImport
		if module.Official {
			kind = OfficialImport
		} else if module.CompilerOwned {
			kind = StandardImport
		}
		imported := &Import{
			Kind:                kind,
			Path:                modulePath,
			ModulePath:          modulePath,
			Symbols:             []string{name},
			Exports:             module.Exports,
			Filename:            module.Filename,
			DeclarationProvider: module.DeclarationProvider,
		}
		copy := exported
		return Binding{Import: imported, Name: name, Export: &copy}, true
	}
	return Binding{}, false
}

func exportNamed(exports map[string]Export, name string) (Export, bool) {
	for _, exported := range exports {
		if exported.Name == name {
			return exported, true
		}
		if nested, ok := exportNamed(exported.Nested, name); ok {
			return nested, true
		}
	}
	return Export{}, false
}

// ContractType resolves a catalog-owned type only when it is reachable from a
// source-selected import contract. This is narrower than source visibility:
// it lets the checker interpret returned values while preventing an unrelated
// project type from replacing an opaque type with the same name.
func (r Result) ContractType(name string) (Binding, bool) {
	target, exists := r.CatalogType(name)
	if !exists || target.Import == nil || target.Import.Kind != ProjectImport {
		return Binding{}, false
	}
	imports := make([]*Import, 0, len(r.Imports))
	seen := map[*Import]bool{}
	for _, imported := range r.Imports {
		if imported == nil || imported.Kind != ProjectImport || seen[imported] {
			continue
		}
		seen[imported] = true
		imports = append(imports, imported)
	}
	sort.Slice(imports, func(i, j int) bool { return imports[i].RuntimePath() < imports[j].RuntimePath() })
	for _, imported := range imports {
		names := imported.Symbols
		if len(names) == 0 {
			names = make([]string, 0, len(imported.Exports))
			for exportedName := range imported.Exports {
				names = append(names, exportedName)
			}
			sort.Strings(names)
		}
		visited := map[string]bool{}
		for _, exportedName := range names {
			exported, selected := imported.Exports[exportedName]
			if selected && r.exportContractReferencesType(imported, exported, target, visited) {
				return target, true
			}
		}
	}
	return Binding{}, false
}

// ContractTypeAlias resolves a catalog-owned transparent alias only when it
// is reachable from a source-selected import contract. This is narrower than
// source visibility: it lets the checker interpret a returned value while
// preventing an unrelated project alias from changing an opaque native type
// that happens to use the same name.
func (r Result) ContractTypeAlias(name string) (Export, bool) {
	alias, exists := r.CatalogTypeAlias(name)
	if !exists {
		return Export{}, false
	}
	seen := map[*Import]bool{}
	for _, imported := range r.Imports {
		if imported == nil || seen[imported] {
			continue
		}
		seen[imported] = true
		names := imported.Symbols
		if len(names) == 0 {
			names = make([]string, 0, len(imported.Exports))
			for exportedName := range imported.Exports {
				names = append(names, exportedName)
			}
		}
		visiting := map[string]bool{}
		for _, exportedName := range names {
			exported, selected := imported.Exports[exportedName]
			if selected && r.exportContractReferencesAlias(imported, exported, name, visiting) {
				return alias, true
			}
		}
	}
	return Export{}, false
}

func (r Result) exportContractReferencesAlias(imported *Import, exported Export, name string, visiting map[string]bool) bool {
	typesToVisit := []types.Type{exported.Type, exported.AliasTarget}
	for _, parameter := range exported.Parameters {
		typesToVisit = append(typesToVisit, parameter.Type)
	}
	for _, field := range exported.Fields {
		typesToVisit = append(typesToVisit, field.Type)
	}
	for _, member := range exported.Members {
		typesToVisit = append(typesToVisit, member.Type)
		for _, parameter := range member.Parameters {
			typesToVisit = append(typesToVisit, parameter.Type)
		}
	}
	for _, typ := range typesToVisit {
		if r.contractTypeReferencesAlias(imported, typ, name, visiting) {
			return true
		}
	}
	return false
}

func (r Result) contractTypeReferencesAlias(imported *Import, typ types.Type, name string, visiting map[string]bool) bool {
	for _, argument := range typ.Args {
		if r.contractTypeReferencesAlias(imported, argument, name, visiting) {
			return true
		}
	}
	if typ.Kind != types.Named || typ.Name == "" || visiting[typ.Name] {
		return false
	}
	exported, directlyOwned := imported.Exports[typ.Name]
	if directlyOwned && exported.Kind != TypeAliasExport {
		return false
	}
	if !directlyOwned {
		var exists bool
		exported, exists = r.CatalogTypeAlias(typ.Name)
		if !exists {
			return false
		}
	}
	if typ.Name == name {
		return true
	}
	visiting[typ.Name] = true
	result := r.contractTypeReferencesAlias(imported, exported.AliasTarget, name, visiting)
	delete(visiting, typ.Name)
	return result
}

func (r Result) exportContractReferencesType(imported *Import, exported Export, target Binding, visited map[string]bool) bool {
	typesToVisit := []types.Type{exported.Type, exported.AliasTarget, exported.NewtypeTarget, exported.EnumRawType}
	typesToVisit = append(typesToVisit, exported.Interfaces...)
	for _, parameter := range exported.Parameters {
		typesToVisit = append(typesToVisit, parameter.Type)
	}
	for _, field := range exported.Fields {
		typesToVisit = append(typesToVisit, field.Type)
	}
	for _, variant := range exported.EnumVariants {
		for _, field := range variant.Fields {
			typesToVisit = append(typesToVisit, field.Type)
		}
	}
	for _, member := range exported.Members {
		typesToVisit = append(typesToVisit, member.Type)
		for _, parameter := range member.Parameters {
			typesToVisit = append(typesToVisit, parameter.Type)
		}
	}
	for _, typ := range typesToVisit {
		if r.contractTypeReferencesType(imported, typ, target, visited) {
			return true
		}
	}
	return false
}

func (r Result) contractTypeReferencesType(imported *Import, typ types.Type, target Binding, visited map[string]bool) bool {
	for _, argument := range typ.Args {
		if r.contractTypeReferencesType(imported, argument, target, visited) {
			return true
		}
	}
	if typ.Kind != types.Named || typ.Name == "" {
		return false
	}
	var binding Binding
	if exported, directlyOwned := imported.Exports[typ.Name]; directlyOwned && typeExport(exported.Kind) {
		copy := exported
		binding = Binding{Import: imported, Name: typ.Name, Export: &copy}
	} else {
		var exists bool
		binding, exists = r.CatalogType(typ.Name)
		if !exists {
			return false
		}
	}
	if sameTypeBinding(binding, target) {
		return true
	}
	key := binding.Import.RuntimePath() + "#" + binding.Name
	if visited[key] || binding.Export == nil {
		return false
	}
	visited[key] = true
	return r.exportContractReferencesType(binding.Import, *binding.Export, target, visited)
}

func sameTypeBinding(left, right Binding) bool {
	return left.Import != nil && right.Import != nil && left.Export != nil && right.Export != nil &&
		left.Import.RuntimePath() == right.Import.RuntimePath() && left.Name == right.Name && left.Export.Kind == right.Export.Kind
}

func typeExport(kind ExportKind) bool {
	switch kind {
	case ClassExport, RecordExport, EnumExport, TypeAliasExport, NewtypeExport, InterfaceExport:
		return true
	default:
		return false
	}
}

// addPrelude exposes the small set of receiver-style, target-independent helpers
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
// backed by the compiler-owned contract catalog even when that contract has no
// public package form.
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
		if !exists || !symbol.HasReceiver() || receiver.Nullable {
			continue
		}
		pattern := importedReceiverPattern(imported, symbol.Receiver)
		if !stdlib.ReceiverMatches(pattern, receiver, symbol.TypeParameters) {
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

// importedReceiverPattern attaches the declaration identity available from a
// package's source exports to its semantic receiver contracts. Official
// packages describe intrinsics separately from their TypeRB declarations, so
// matching by display name alone could otherwise lend a method to an unrelated
// declaration with the same name.
func importedReceiverPattern(imported *Import, pattern types.Type) types.Type {
	result := pattern
	if imported != nil && result.Kind == types.Named && result.Declaration.Empty() {
		if exported, ok := exportNamed(imported.Exports, result.Name); ok && typeExport(exported.Kind) {
			result.Declaration = identity.Declaration{
				Module: imported.RuntimePath(),
				Name:   exported.Name,
				Kind:   identityKind(exported.Kind),
			}
		}
	}
	result.Args = make([]types.Type, len(pattern.Args))
	for index, argument := range pattern.Args {
		result.Args[index] = importedReceiverPattern(imported, argument)
	}
	return result
}

func (r Result) TypeMember(typeName, name string) (Binding, bool) {
	if binding, exists := r.Symbols[typeName]; exists && binding.Export != nil {
		if result, ok := typeMemberBinding(binding, name); ok {
			return result, true
		}
	}
	for _, binding := range r.Symbols {
		if binding.Export == nil || binding.Export.Name != typeName {
			continue
		}
		return typeMemberBinding(binding, name)
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
		result := Binding{Import: imported, Name: name, Export: &exportCopy, Member: &copy}
		attachDefinedStaticMember(&result)
		return result, true
	}
	return Binding{}, false
}

// ImportedTypeIdentity resolves an already-canonical declaration identity
// across authored and compiler-generated import scopes. It does not make a
// generated binding visible by source spelling; callers must already hold the
// semantic identity from a resolved expression.
func (r Result) ImportedTypeIdentity(declaration identity.Declaration) (Binding, bool) {
	if declaration.Empty() {
		return Binding{}, false
	}
	for _, symbols := range []map[string]Binding{r.Symbols, r.GeneratedSymbols} {
		for _, binding := range symbols {
			if binding.Export != nil && typeExport(binding.Export.Kind) && binding.DeclarationIdentity() == declaration {
				return binding, true
			}
		}
	}
	return Binding{}, false
}

// TypeMemberIdentity resolves a member through an already-resolved type
// identity, preserving aliases and authored/generated scope separation.
func (r Result) TypeMemberIdentity(declaration identity.Declaration, name string) (Binding, bool) {
	binding, ok := r.ImportedTypeIdentity(declaration)
	if ok {
		return typeMemberBinding(binding, name)
	}
	for _, symbols := range []map[string]Binding{r.Symbols, r.GeneratedSymbols} {
		for _, candidate := range symbols {
			if candidate.Export != nil && candidate.DeclarationIdentity() == declaration {
				return typeMemberBinding(candidate, name)
			}
		}
	}
	return Binding{}, false
}

func typeMemberBinding(binding Binding, name string) (Binding, bool) {
	if binding.Export == nil || binding.Import == nil {
		return Binding{}, false
	}
	if nested, ok := binding.Export.Nested[name]; ok {
		copy := nested
		return Binding{Import: binding.Import, Name: nested.Name, Export: &copy}, true
	}
	member, ok := binding.Export.Members[name]
	if !ok {
		return Binding{}, false
	}
	memberCopy := member
	exportCopy := *binding.Export
	result := Binding{Import: binding.Import, Name: name, Export: &exportCopy, Member: &memberCopy}
	attachDefinedStaticMember(&result)
	return result, true
}

func attachDefinedStaticMember(binding *Binding) {
	if binding == nil || binding.Import == nil || binding.Import.Definition == nil || binding.Export == nil || binding.Member == nil {
		return
	}
	symbol, exists := binding.Import.Definition.Symbols[binding.Name]
	if !exists {
		return
	}
	rootMember := binding.Import.Definition.Root == binding.Export.Name && !symbol.HasReceiver() && !symbol.HasStaticOwner()
	ownedMember := symbol.StaticOwner == binding.Export.Name
	if !rootMember && !ownedMember {
		return
	}
	symbolCopy := symbol
	binding.Library = &symbolCopy
}

func (r Result) ImportedType(typeName string) (Binding, bool) {
	if binding, exists := r.Symbols[typeName]; exists && binding.Export != nil && typeExport(binding.Export.Kind) {
		return binding, true
	}
	for _, binding := range r.Symbols {
		if binding.Export != nil && binding.Export.Name == typeName && (binding.Export.Kind == ClassExport || binding.Export.Kind == RecordExport || binding.Export.Kind == EnumExport || binding.Export.Kind == TypeAliasExport || binding.Export.Kind == NewtypeExport || binding.Export.Kind == InterfaceExport) {
			return binding, true
		}
	}
	for _, imported := range r.Packages {
		exported, exists := imported.Exports[typeName]
		if exists && (exported.Kind == ClassExport || exported.Kind == RecordExport || exported.Kind == EnumExport || exported.Kind == TypeAliasExport || exported.Kind == NewtypeExport || exported.Kind == InterfaceExport) {
			copy := exported
			return Binding{Import: imported, Name: typeName, Export: &copy}, true
		}
	}
	return Binding{}, false
}

// ImportedTypeAt resolves source-visible types from the authored or generated
// import scope selected by the caller. Generated imports never satisfy an
// authored reference, even when both scopes use the same local spelling.
func (r Result) ImportedTypeAt(typeName string, generated bool) (Binding, bool) {
	if generated {
		if binding, exists := r.GeneratedSymbols[typeName]; exists && binding.Export != nil && typeExport(binding.Export.Kind) {
			return binding, true
		}
		for _, binding := range r.GeneratedSymbols {
			if binding.Export != nil && binding.Export.Name == typeName && typeExport(binding.Export.Kind) {
				return binding, true
			}
		}
		return Binding{}, false
	}
	return r.ImportedType(typeName)
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
		if !exists || !typeExport(exported.Kind) {
			continue
		}
		copy := exported
		return Binding{Import: imported, Name: typeName, Export: &copy}, true
	}
	return Binding{}, false
}

// InferredTypeMember resolves a member on a type produced by an explicitly
// imported package or exposed through one of its selected value contracts,
// even when the source did not import that type by name. This keeps inferred
// values usable without weakening named-import rules for source annotations.
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
		exportCopy := exported
		binding, exists := typeMemberBinding(Binding{Import: imported, Name: typeName, Export: &exportCopy}, memberName)
		if !exists || binding.Member == nil {
			return Binding{}, false
		}
		return binding, true
	}
	contract, exists := r.ContractType(typeName)
	if !exists || contract.Export == nil {
		return Binding{}, false
	}
	binding, exists := typeMemberBinding(contract, memberName)
	if !exists || binding.Member == nil {
		return Binding{}, false
	}
	return binding, true
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
	for filename, items := range diagnostics {
		diagnostics[filename] = diagnostic.Normalize(items, filename, diagnostic.ResolutionError)
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
		if options.Catalog != nil && (options.Catalog.Modules[node.Path] != nil || options.Catalog.Modules[pathpkg.Join(node.Path, "index")] != nil) {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("import %s is provided by both a TypeRB source module and a native declaration adapter", node.Path))}
		}
		return resolveNativeImport(node, options.NativePackages)
	}
	return resolveProjectImport(node, options)
}

func resolveActivation(node *ast.ActivateStatement, options Options) (*Import, []diagnostic.Diagnostic) {
	activationError := func(message string) (*Import, []diagnostic.Diagnostic) {
		return nil, []diagnostic.Diagnostic{errorAt(node, message)}
	}
	if definition, ok := stdlib.Lookup(node.Path); ok {
		if !definition.Supports(options.Mode) {
			return activationError(fmt.Sprintf("package %s does not support mode %s", node.Path, options.Mode))
		}
		resolved := &Import{ActivationNode: node, Kind: StandardImport, Path: node.Path, ModulePath: definition.ModulePath, Definition: definition, Exports: map[string]Export{}, RootStable: true}
		if !importHasCapability(resolved, options.Mode) {
			return activationError(fmt.Sprintf("package %s does not declare an activatable capability", node.Path))
		}
		return resolved, nil
	}
	if bundled, ok := official.Lookup(node.Path); ok {
		definition := bundled.Definition
		if !definition.Supports(options.Mode) {
			return activationError(fmt.Sprintf("package %s does not support mode %s", node.Path, options.Mode))
		}
		resolved := &Import{ActivationNode: node, Kind: OfficialImport, Path: node.Path, ModulePath: definition.ModulePath, Definition: definition, Exports: map[string]Export{}, RootStable: true}
		if !importHasCapability(resolved, options.Mode) {
			return activationError(fmt.Sprintf("package %s does not declare an activatable capability", node.Path))
		}
		return resolved, nil
	}
	if stdlib.IsReservedPath(node.Path) {
		return activationError(fmt.Sprintf("unknown TypeRB package %s", node.Path))
	}
	candidates, valid := ProjectImportModuleCandidates(node.Path)
	if !valid {
		return activationError(fmt.Sprintf("invalid activation path %q", node.Path))
	}
	canonical := CanonicalPackageImport(candidates[0], options.PackageAliases)
	if options.Catalog != nil {
		module := options.Catalog.Modules[canonical]
		if module == nil && len(candidates) > 1 {
			module = options.Catalog.Modules[pathpkg.Join(canonical, "index")]
		}
		if module == nil {
			return activationError(fmt.Sprintf("cannot resolve activation target %s", node.Path))
		}
		resolved := &Import{
			ActivationNode: node, Kind: ProjectImport, Path: module.Path, ModulePath: module.Path,
			Exports: module.Exports, Filename: module.Filename, DeclarationProvider: module.DeclarationProvider,
			RootStable: !module.DeclarationProvider,
		}
		if !importHasCapability(resolved, options.Mode) {
			return activationError(fmt.Sprintf("package %s does not declare an activatable capability", node.Path))
		}
		return resolved, nil
	}
	return activationError(fmt.Sprintf("cannot resolve activation target %s", node.Path))
}

func importHasCapability(imported *Import, mode string) bool {
	if imported == nil {
		return false
	}
	if imported.DeclarationProvider {
		return true
	}
	definition := imported.Definition
	return definition != nil && definition.Supports(mode) && (definition.NativeSyntax || definition.TypeProvider != "" || definition.JSX != nil || definition.Capability)
}

func bindingIdentityKey(binding Binding) string {
	declaration := binding.DeclarationIdentity()
	return fmt.Sprintf("%s\x00%v\x00%s", declaration.Module, declaration.Kind, declaration.Name)
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
	resolved := &Import{
		Node: node, Kind: NativeImport, Path: node.Path, ModulePath: node.Path,
		SymbolAliases: cloneStringMap(node.SymbolAliases), Exports: map[string]Export{}, RootStable: false,
	}
	for name, exported := range module.Exports {
		resolved.Exports[name] = nativeExport(name, exported, true)
	}
	for name, exported := range module.Records {
		resolved.Exports[name] = nativeExport(name, exported, false)
	}
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else {
		root, diagnostics := selectBareRoot(node, resolved)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		resolved.Symbols = []string{root}
		if node.Alias != "" {
			resolved.SymbolAliases[root] = node.Alias
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
		Variadic:          exported.Variadic,
		TypeParameters:    append([]string(nil), exported.TypeParameters...),
		Members:           map[string]Member{},
		UnsupportedFields: cloneStrings(exported.UnsupportedFields),
		NativeExported:    nativeExported,
	}
	if exported.Runtime != nil {
		result.Runtime = &RuntimeBinding{
			Identity: exported.Runtime.Identity, Dependency: exported.Runtime.Dependency,
			Module: exported.Runtime.Module, Symbol: exported.Runtime.Symbol, CallConvention: exported.Runtime.CallConvention,
			MaySuspend: exported.Runtime.MaySuspend, PropagatesExecutionScope: exported.Runtime.PropagatesExecutionScope,
		}
	}
	if exported.ResultBridge != nil {
		result.CallResultBridge = NativeCallResultBridge{Kind: exported.ResultBridge.Kind, Error: exported.ResultBridge.Error.Semantic()}
	}
	if exported.AliasTarget != nil {
		result.AliasTarget = exported.AliasTarget.Semantic()
	}
	for _, parameter := range exported.Parameters {
		parameterType, resultBridge := nativeResultCallbackType(parameter)
		presence := callsignature.Omittable
		if len(result.Parameters) < exported.Required {
			presence = callsignature.Required
		}
		result.Parameters = append(result.Parameters, callsignature.Parameter{Kind: callsignature.Positional, Type: parameterType, Presence: presence})
		result.ParameterResultBridges = append(result.ParameterResultBridges, resultBridge)
	}
	for _, field := range exported.Fields {
		fieldType, resultBridge := nativeResultCallbackType(field.Type)
		result.Fields = append(result.Fields, RecordField{Name: field.Name, JSONName: field.Name, Type: fieldType, Optional: field.Optional, ResultBridge: resultBridge})
		result.Members[field.Name] = Member{Name: field.Name, Kind: ValueExport, Type: fieldType, Readonly: true}
	}
	importMembers := func(members map[string]nativepackage.Export, class bool) {
		for name, exportedMember := range members {
			memberExport := nativeExport(name, exportedMember, false)
			result.Members[name] = Member{
				Name:              name,
				Kind:              memberExport.Kind,
				Type:              memberExport.Type,
				TypeParameters:    append([]string(nil), memberExport.TypeParameters...),
				Parameters:        append([]callsignature.Parameter(nil), memberExport.Parameters...),
				Variadic:          memberExport.Variadic,
				Class:             class,
				CallResultBridge:  memberExport.CallResultBridge,
				UnsupportedFields: cloneStrings(memberExport.UnsupportedFields),
			}
		}
	}
	importMembers(exported.Members, true)
	importMembers(exported.InstanceMembers, false)
	importMembers(exported.ClassMembers, true)
	return result
}

func nativeResultCallbackType(providerType nativepackage.Type) (types.Type, NativeResultBridge) {
	nativeType := providerType.Semantic()
	if providerType.ResultBridge == nil {
		return nativeType, NativeResultBridge{}
	}
	parameters, success, ok := types.FunctionSignature(nativeType)
	if !ok {
		return nativeType, NativeResultBridge{}
	}
	errorType := providerType.ResultBridge.Error.Semantic()
	resultSuccess := success
	if resultSuccess.Kind == types.Void {
		resultSuccess = types.FromName("Unit")
	}
	resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{resultSuccess, errorType}}
	return types.FunctionOf(parameters, resultType), NativeResultBridge{
		Kind:  providerType.ResultBridge.Kind,
		Type:  nativeType,
		Error: errorType,
	}
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
		Node:          node,
		Kind:          kind,
		Path:          node.Path,
		ModulePath:    definition.ModulePath,
		SymbolAliases: cloneStringMap(node.SymbolAliases),
		Definition:    definition,
		Exports:       map[string]Export{},
		RootStable:    true,
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
				resolved.Exports = cloneExports(module.Exports)
			}
		}
	}
	addDefinedRoot(resolved, definition)
	addDefinedStaticMembers(resolved, definition)
	if definition.RuntimeAlias != "" {
		resolved.Alias = definition.RuntimeAlias
	}
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else {
		root, diagnostics := selectBareRoot(node, resolved)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		resolved.Symbols = []string{root}
		if node.Alias != "" {
			resolved.SymbolAliases[root] = node.Alias
		}
	}
	for _, name := range resolved.Symbols {
		symbol, librarySymbol := definition.Symbols[name]
		if symbol.HasStaticOwner() {
			librarySymbol = false
		}
		_, sourceExport := resolved.Exports[name]
		if definition.Root != "" {
			librarySymbol = false
		}
		if !librarySymbol && !sourceExport {
			if symbol.HasStaticOwner() {
				return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s does not export %s as a top-level declaration; use %s.%s through its owning declaration", node.Path, name, symbol.StaticOwner, name))}
			}
			if root := resolved.Exports[definition.Root]; root.Members[name].Name != "" {
				return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s does not export %s as a top-level declaration; use %s.%s through the root import", node.Path, name, definition.Root, name))}
			}
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("package %s does not export %s", node.Path, name))}
		}
		if librarySymbol && symbol.CompilerOnly && !options.CompilerOwned {
			return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("%s.%s is internal to the TypeRB compiler", node.Path, name))}
		}
	}
	sort.Strings(resolved.Symbols)
	return resolved, nil
}

func resolveProjectImport(node *ast.ImportStatement, options Options) (*Import, []diagnostic.Diagnostic) {
	moduleCandidates, valid := ProjectImportModuleCandidates(node.Path)
	if !valid {
		return nil, []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("invalid project import path %q", node.Path))}
	}
	clean := moduleCandidates[0]
	canonical := CanonicalPackageImport(clean, options.PackageAliases)
	resolved := &Import{Node: node, Kind: ProjectImport, Path: canonical, SymbolAliases: cloneStringMap(node.SymbolAliases), Exports: map[string]Export{}, RootStable: true}
	if options.Catalog != nil {
		module := options.Catalog.Modules[canonical]
		if module == nil && len(moduleCandidates) > 1 {
			module = options.Catalog.Modules[pathpkg.Join(canonical, "index")]
		}
		if module != nil {
			resolved.Path = module.Path
			resolved.Filename = module.Filename
			resolved.Exports = cloneExports(module.Exports)
			resolved.DeclarationProvider = module.DeclarationProvider
			resolved.RootStable = !module.DeclarationProvider
			if module.DeclarationProvider {
				addDeclarationProviderRoots(resolved, options.Declarations)
			}
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
	var fileCandidates []string
	if options.Catalog == nil {
		fileCandidates = append(fileCandidates, filepath.Join(options.SourceRoot, filepath.FromSlash(clean)+".trb"))
	}
	if options.Mode == "ruby" {
		fileCandidates = append(fileCandidates, filepath.Join(options.SourceRoot, filepath.FromSlash(clean)+".rb"))
	}
	var data []byte
	for _, candidate := range fileCandidates {
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
		resolved.RootStable = false
	}

	return finalizeProjectImport(resolved)
}

// addDeclarationProviderRoots exposes a provider's real top-level namespace as
// the package declaration root. Qualified declarations such as Pagy::Offset
// and Pagy::Method establish Pagy without inventing an activation-only symbol.
func addDeclarationProviderRoots(imported *Import, catalog *declaration.Catalog) {
	if imported == nil || catalog == nil {
		return
	}
	provider := strings.TrimSuffix(imported.RuntimePath(), "/index")
	add := func(name, owner string) {
		if owner != provider {
			return
		}
		root, _, qualified := strings.Cut(name, "::")
		if !qualified || root == "" {
			return
		}
		if _, exists := imported.Exports[root]; !exists {
			imported.Exports[root] = Export{Name: root, Kind: ModuleExport, Type: types.FromName(root)}
		}
	}
	for name, declared := range catalog.Types {
		if declared != nil {
			add(name, declared.Provider)
		}
	}
	for name, declared := range catalog.Modules {
		if declared != nil {
			add(name, declared.Provider)
		}
	}
}

// ProjectImportModuleCandidates returns the canonical module identities that
// may satisfy one project import. File-root discovery and the resolver share
// this function so extension trimming, path validation, and directory-index
// fallback cannot drift between the source graph and semantic resolution.
func ProjectImportModuleCandidates(importPath string) ([]string, bool) {
	if strings.ContainsAny(importPath, `\:`) {
		return nil, false
	}
	clean := pathpkg.Clean(strings.TrimSuffix(importPath, ".trb"))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || pathpkg.IsAbs(clean) {
		return nil, false
	}
	return []string{clean, pathpkg.Join(clean, "index")}, true
}

// CanonicalProjectImportPath removes a terminal /index only when the shorter
// authored path resolves to the same known module. Callers provide the module
// identities available in their project snapshot so formatting and import
// completion cannot change the selected module when both name.trb and
// name/index.trb exist.
func CanonicalProjectImportPath(importPath string, modulePaths map[string]bool, aliases map[string]string) string {
	if stdlib.IsReservedPath(importPath) || !strings.HasSuffix(importPath, "/index") {
		return importPath
	}
	short := strings.TrimSuffix(importPath, "/index")
	if short == "" {
		return importPath
	}
	resolved, ok := resolveKnownProjectModule(importPath, modulePaths, aliases)
	if !ok {
		return importPath
	}
	shortResolved, ok := resolveKnownProjectModule(short, modulePaths, aliases)
	if !ok || shortResolved != resolved {
		return importPath
	}
	return short
}

func resolveKnownProjectModule(importPath string, modulePaths map[string]bool, aliases map[string]string) (string, bool) {
	candidates, valid := ProjectImportModuleCandidates(importPath)
	if !valid {
		return "", false
	}
	canonical := CanonicalPackageImport(candidates[0], aliases)
	if modulePaths[canonical] {
		return canonical, true
	}
	indexed := pathpkg.Join(canonical, "index")
	if len(candidates) > 1 && modulePaths[indexed] {
		return indexed, true
	}
	return "", false
}

// CanonicalPackageImport applies the longest matching TypeRB package alias to
// a source import path. Compile-time providers use the same mapping before the
// ordinary resolver has produced its import graph.
func CanonicalPackageImport(importPath string, aliases map[string]string) string {
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

func cloneStringMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for name, value := range input {
		result[name] = value
	}
	return result
}

func cloneExports(input map[string]Export) map[string]Export {
	result := make(map[string]Export, len(input))
	for name, exported := range input {
		exported.Members = cloneMembers(exported.Members)
		exported.Nested = cloneExports(exported.Nested)
		result[name] = exported
	}
	return result
}

func cloneMembers(input map[string]Member) map[string]Member {
	if input == nil {
		return nil
	}
	result := make(map[string]Member, len(input))
	for name, member := range input {
		result[name] = member
	}
	return result
}

func addDefinedRoot(imported *Import, definition *stdlib.Package) {
	if imported == nil || definition == nil || definition.Root == "" {
		return
	}
	members := map[string]Member{}
	root, existingRoot := imported.Exports[definition.Root]
	if existingRoot {
		for name, member := range root.Members {
			members[name] = member
		}
	}
	for name, symbol := range definition.Symbols {
		if symbol.CompilerOnly || symbol.HasReceiver() || symbol.HasStaticOwner() {
			continue
		}
		parameters := make([]callsignature.Parameter, len(symbol.Parameters))
		for index, parameter := range symbol.Parameters {
			kind := callsignature.Positional
			label := ""
			if parameter.Keyword {
				kind = callsignature.NamedOnly
				label = parameter.Name
			}
			presence := callsignature.Required
			if parameter.Optional {
				presence = callsignature.Omittable
			}
			parameters[index] = callsignature.Parameter{Kind: kind, Label: label, Type: parameter.Type, Presence: presence}
		}
		members[name] = Member{
			Name: name, Kind: FunctionExport, Type: symbol.Return, TypeParameters: append([]string(nil), symbol.TypeParameters...),
			Parameters: parameters, Variadic: symbol.Variadic, Class: true,
		}
	}
	for name, exported := range imported.Exports {
		if name == definition.Root || exported.Kind != FunctionExport {
			continue
		}
		if _, libraryMember := members[name]; !libraryMember {
			members[name] = Member{
				Name: name, Kind: FunctionExport, Type: exported.Type,
				TypeParameters: append([]string(nil), exported.TypeParameters...),
				Parameters:     append([]callsignature.Parameter(nil), exported.Parameters...),
				Variadic:       exported.Variadic, Class: true, Generated: "package_root",
				CallResultBridge: exported.CallResultBridge,
			}
		} else {
			member := members[name]
			member.Generated = "package_root"
			members[name] = member
		}
		delete(imported.Exports, name)
	}
	if !existingRoot {
		root = Export{Name: definition.Root, Kind: ModuleExport, Type: types.FromName(definition.Root)}
	} else {
		root.Members = nil
	}
	root.Members = members
	imported.Exports[definition.Root] = root
}

func addDefinedStaticMembers(imported *Import, definition *stdlib.Package) {
	if imported == nil || definition == nil {
		return
	}
	for name, symbol := range definition.Symbols {
		if symbol.CompilerOnly || !symbol.HasStaticOwner() {
			continue
		}
		owner, exists := exportNamed(imported.Exports, symbol.StaticOwner)
		if !exists || (owner.Kind != ClassExport && owner.Kind != ModuleExport && owner.Kind != RecordExport) {
			continue
		}
		parameters := make([]callsignature.Parameter, len(symbol.Parameters))
		for index, parameter := range symbol.Parameters {
			kind := callsignature.Positional
			label := ""
			if parameter.Keyword {
				kind = callsignature.NamedOnly
				label = parameter.Name
			}
			presence := callsignature.Required
			if parameter.Optional {
				presence = callsignature.Omittable
			}
			parameters[index] = callsignature.Parameter{Kind: kind, Label: label, Type: parameter.Type, Presence: presence}
		}
		owner.Members[name] = Member{
			Name: name, Kind: FunctionExport, Type: symbol.Return, TypeParameters: append([]string(nil), symbol.TypeParameters...),
			Parameters: parameters, Variadic: symbol.Variadic, Class: true,
		}
		setExport(imported.Exports, owner.Name, owner)
	}
}

func selectBareRoot(node *ast.ImportStatement, imported *Import) (string, []diagnostic.Diagnostic) {
	segment := logicalImportSegment(imported.RuntimePath())
	wanted := pathRootKey(segment)
	var candidates []string
	var available []string
	for name, exported := range imported.Exports {
		if imported.Kind == NativeImport && !exported.NativeExported {
			continue
		}
		available = append(available, name)
		if rootEligible(exported.Kind) && declarationRootKey(name) == wanted {
			candidates = append(candidates, name)
		}
	}
	if imported.Definition != nil && imported.Definition.Root == "" {
		for name, symbol := range imported.Definition.Symbols {
			if !symbol.CompilerOnly && !symbol.HasStaticOwner() {
				available = append(available, name)
			}
		}
	}
	sort.Strings(candidates)
	sort.Strings(available)
	switch len(candidates) {
	case 1:
		return candidates[0], nil
	case 0:
		message := fmt.Sprintf("import %s has no root declaration matching %s", node.Path, segment)
		if len(available) > 0 {
			message += "; available top-level declarations: " + strings.Join(available, ", ")
		}
		return "", []diagnostic.Diagnostic{errorAt(node, message)}
	default:
		return "", []diagnostic.Diagnostic{errorAt(node, fmt.Sprintf("import %s has multiple root declarations matching %s: %s; use an exact named import", node.Path, segment, strings.Join(candidates, ", ")))}
	}
}

// RootDeclaration returns the unique current declaration-root candidate for
// tooling. RootStable separately determines whether a formatter may rely on
// that uniqueness for canonical form conversion.
func RootDeclaration(imported *Import) (string, bool) {
	if imported == nil {
		return "", false
	}
	wanted := pathRootKey(logicalImportSegment(imported.RuntimePath()))
	root := ""
	for name, exported := range imported.Exports {
		if imported.Kind == NativeImport && !exported.NativeExported {
			continue
		}
		if !rootEligible(exported.Kind) || declarationRootKey(name) != wanted {
			continue
		}
		if root != "" {
			return "", false
		}
		root = name
	}
	return root, root != ""
}

// MatchesDeclarationRoot reports whether name has the declaration-root key
// derived from modulePath. Callers remain responsible for restricting name to
// a root-eligible declaration kind.
func MatchesDeclarationRoot(modulePath, name string) bool {
	return pathRootKey(logicalImportSegment(modulePath)) == declarationRootKey(name)
}

func logicalImportSegment(modulePath string) string {
	clean := pathpkg.Clean(strings.TrimSuffix(modulePath, ".trb"))
	if pathpkg.Base(clean) == "index" {
		clean = pathpkg.Dir(clean)
	}
	return pathpkg.Base(clean)
}

func pathRootKey(name string) string {
	return asciiFold(strings.ReplaceAll(name, "_", ""))
}

func declarationRootKey(name string) string {
	return asciiFold(name)
}

func asciiFold(value string) string {
	var result strings.Builder
	result.Grow(len(value))
	for _, char := range value {
		if char >= 'A' && char <= 'Z' {
			char += 'a' - 'A'
		}
		result.WriteRune(char)
	}
	return result.String()
}

func rootEligible(kind ExportKind) bool {
	switch kind {
	case ClassExport, RecordExport, EnumExport, TypeAliasExport, NewtypeExport, ModuleExport, InterfaceExport, ValueExport:
		return true
	default:
		return false
	}
}

func finalizeProjectImport(resolved *Import) (*Import, []diagnostic.Diagnostic) {
	node := resolved.Node
	if len(node.Symbols) > 0 {
		resolved.Symbols = append([]string(nil), node.Symbols...)
	} else {
		root, diagnostics := selectBareRoot(node, resolved)
		if len(diagnostics) > 0 {
			return nil, diagnostics
		}
		resolved.Symbols = []string{root}
		if node.Alias != "" {
			resolved.SymbolAliases[root] = node.Alias
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
		if symbol, ok := imported.Definition.Symbols[name]; ok && !symbol.HasStaticOwner() {
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
				exported := Export{Name: node.Name, Kind: ClassExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Superclass: expressionName(node.Superclass), Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				for _, implemented := range node.Implements {
					exported.Interfaces = append(exported.Interfaces, typeRef(implemented))
				}
				for _, member := range node.Body {
					switch item := member.(type) {
					case *ast.MethodStatement:
						parameterTypes, variadic := parameters(item.Parameters)
						if item.Name == "initialize" {
							exported.Parameters, exported.Variadic = parameterTypes, variadic
							continue
						}
						if public(item.Name) {
							method := Member{Name: item.Name, Kind: FunctionExport, Type: returnTypeRef(item.ReturnType), Parameters: parameterTypes, Variadic: variadic, Class: item.Class}
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
					exported.Fields = append(exported.Fields, RecordField{Name: field.Name, JSONName: recordJSONName(field), Type: typ, HasDefault: field.Default != nil})
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
						for _, parameter := range member.Parameters {
							fieldType := typeRef(parameter.Type)
							variant.Fields = append(variant.Fields, RecordField{Name: parameter.Name, Type: fieldType, NamedOnly: parameter.NamedOnly})
						}
						exported.EnumVariants = append(exported.EnumVariants, variant)
						kind := ValueExport
						if len(member.Parameters) > 0 {
							kind = FunctionExport
						}
						parameterSignature, variadic := parameters(member.Parameters)
						exported.Members[member.Name] = Member{Name: member.Name, Kind: kind, Type: typ, Parameters: parameterSignature, Variadic: variadic, Class: true}
					case *ast.MethodStatement:
						if public(member.Name) {
							parameterTypes, variadic := parameters(member.Parameters)
							exported.Members[member.Name] = Member{Name: member.Name, Kind: FunctionExport, Type: returnTypeRef(member.ReturnType), Parameters: parameterTypes, Variadic: variadic, EnumOwner: node.Name}
						}
					}
				}
				if raw {
					exported.EnumRawType = enumRawType(node)
					exported.Members["raw_value"] = Member{Name: "raw_value", Kind: FunctionExport, Type: exported.EnumRawType, EnumOwner: node.Name, Generated: "raw_value"}
					exported.Members["from_raw"] = Member{Name: "from_raw", Kind: FunctionExport, Type: types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typ, types.FromName("EnumValueError")}}, Parameters: callsignature.FromPositionalTypes([]types.Type{exported.EnumRawType}, 1), Class: true, EnumOwner: node.Name, Generated: "from_raw"}
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
		case *ast.NewtypeStatement:
			if public(node.Name) {
				typ := types.FromName(node.Name)
				target := typeRef(node.Target)
				exported := Export{
					Name: node.Name, Kind: NewtypeExport, Type: typ, NewtypeTarget: target,
					NewtypePrivateNew: node.PrivateNew,
					Members: map[string]Member{
						"new":   {Name: "new", Kind: FunctionExport, Type: typ, Parameters: callsignature.FromPositionalTypes([]types.Type{target}, 1), Class: true, Generated: "newtype_new"},
						"value": {Name: "value", Kind: FunctionExport, Type: target, Generated: "newtype_value"},
					},
					Span: node.Span(),
				}
				if node.PrivateNew {
					delete(exported.Members, "new")
				}
				for _, statement := range node.Body {
					if method, ok := statement.(*ast.MethodStatement); ok && public(method.Name) {
						params, variadic := parameters(method.Parameters)
						member := Member{Name: method.Name, Kind: FunctionExport, Type: returnTypeRef(method.ReturnType), Parameters: params, Variadic: variadic, Class: method.Class}
						for _, parameter := range method.TypeParameters {
							member.TypeParameters = append(member.TypeParameters, parameter.Name)
						}
						exported.Members[method.Name] = member
					}
				}
				result[node.Name] = exported
			}
		case *ast.ModuleStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: ModuleExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Nested: map[string]Export{}, Span: node.Span()}
				owned := ownedTypeNames(node.Name, node.Body)
				for _, statement := range node.Body {
					switch item := statement.(type) {
					case *ast.VariableStatement:
						if item.Constant && public(item.Name) {
							exported.Members[item.Name] = Member{Name: item.Name, Kind: ValueExport, Type: qualifyOwnedType(variableType(item), owned), Class: true}
						}
					case *ast.MethodStatement:
						if item.Class && public(item.Name) {
							parameterTypes, variadic := parameters(item.Parameters)
							for index := range parameterTypes {
								parameterTypes[index].Type = qualifyOwnedType(parameterTypes[index].Type, owned)
							}
							member := Member{Name: item.Name, Kind: FunctionExport, Type: qualifyOwnedType(returnTypeRef(item.ReturnType), owned), Parameters: parameterTypes, Variadic: variadic, Class: true}
							for _, parameter := range item.TypeParameters {
								member.TypeParameters = append(member.TypeParameters, parameter.Name)
							}
							exported.Members[item.Name] = member
						}
					}
				}
				for name, nested := range CollectExports(node.Body) {
					if !rootEligible(nested.Kind) || nested.Kind == ValueExport {
						continue
					}
					nested = qualifyNestedExport(nested, node.Name, owned)
					exported.Nested[name] = nested
					exported.Members[name] = Member{Name: name, Kind: nested.Kind, Type: nested.Type, Class: true}
				}
				result[node.Name] = exported
			}
		case *ast.InterfaceStatement:
			if public(node.Name) {
				exported := Export{Name: node.Name, Kind: InterfaceExport, Type: types.FromName(node.Name), Members: map[string]Member{}, Span: node.Span()}
				for _, parameter := range node.TypeParameters {
					exported.TypeParameters = append(exported.TypeParameters, parameter.Name)
				}
				for _, method := range node.Methods {
					parameterTypes, variadic := parameters(method.Parameters)
					exported.Members[method.Name] = Member{Name: method.Name, Kind: FunctionExport, Type: returnTypeRef(method.ReturnType), Parameters: parameterTypes, Variadic: variadic}
				}
				result[node.Name] = exported
			}
		case *ast.MethodStatement:
			if public(node.Name) {
				parameterTypes, variadic := parameters(node.Parameters)
				exported := Export{Name: node.Name, Kind: FunctionExport, Type: returnTypeRef(node.ReturnType), Parameters: parameterTypes, Variadic: variadic, Span: node.Span()}
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
	for name, exported := range result {
		exported.Source = true
		result[name] = exported
	}
	return result
}

func ownedTypeNames(owner string, statements []ast.Statement) map[string]string {
	result := map[string]string{}
	for _, statement := range statements {
		var name string
		switch node := statement.(type) {
		case *ast.ClassStatement:
			name = node.Name
		case *ast.RecordStatement:
			name = node.Name
		case *ast.EnumStatement:
			name = node.Name
		case *ast.TypeAliasStatement:
			name = node.Name
		case *ast.NewtypeStatement:
			name = node.Name
		case *ast.InterfaceStatement:
			name = node.Name
		case *ast.ModuleStatement:
			name = node.Name
		}
		if public(name) {
			result[name] = owner + "::" + name
		}
	}
	return result
}

func qualifyNestedExport(exported Export, owner string, owned map[string]string) Export {
	originalName := exported.Name
	exported.Name = owner + "::" + originalName
	exported.Type = qualifyOwnedType(exported.Type, owned)
	if exported.Type.Kind == types.Named && exported.Type.Name == originalName {
		exported.Type.Name = exported.Name
	}
	exported.AliasTarget = qualifyOwnedType(exported.AliasTarget, owned)
	exported.NewtypeTarget = qualifyOwnedType(exported.NewtypeTarget, owned)
	exported.EnumRawType = qualifyOwnedType(exported.EnumRawType, owned)
	for index := range exported.Interfaces {
		exported.Interfaces[index] = qualifyOwnedType(exported.Interfaces[index], owned)
	}
	for index := range exported.Parameters {
		exported.Parameters[index].Type = qualifyOwnedType(exported.Parameters[index].Type, owned)
	}
	for index := range exported.Fields {
		exported.Fields[index].Type = qualifyOwnedType(exported.Fields[index].Type, owned)
	}
	for index := range exported.EnumVariants {
		for fieldIndex := range exported.EnumVariants[index].Fields {
			exported.EnumVariants[index].Fields[fieldIndex].Type = qualifyOwnedType(exported.EnumVariants[index].Fields[fieldIndex].Type, owned)
		}
	}
	for name, member := range exported.Members {
		member.Type = qualifyOwnedType(member.Type, owned)
		for index := range member.Parameters {
			member.Parameters[index].Type = qualifyOwnedType(member.Parameters[index].Type, owned)
		}
		exported.Members[name] = member
	}
	for name, nested := range exported.Nested {
		exported.Nested[name] = qualifyNestedExport(nested, owner, owned)
	}
	return exported
}

func qualifyOwnedType(typ types.Type, owned map[string]string) types.Type {
	for index := range typ.Args {
		typ.Args[index] = qualifyOwnedType(typ.Args[index], owned)
	}
	if typ.Kind != types.Named || typ.Name == "" {
		return typ
	}
	root, suffix, qualified := strings.Cut(typ.Name, "::")
	if replacement := owned[root]; replacement != "" {
		typ.Name = replacement
		if qualified {
			typ.Name += "::" + suffix
		}
	}
	return typ
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

func parameters(nodes []ast.Parameter) ([]callsignature.Parameter, bool) {
	result := make([]callsignature.Parameter, len(nodes))
	variadic := false
	for i, parameter := range nodes {
		kind := callsignature.Positional
		label := ""
		if parameter.NamedOnly || parameter.Keyword {
			kind = callsignature.NamedOnly
			label = parameter.Name
		}
		presence := callsignature.Omittable
		if parameter.Default == nil {
			presence = callsignature.Required
		}
		result[i] = callsignature.Parameter{Kind: kind, Label: label, Type: typeRef(parameter.Type), Presence: presence}
		variadic = variadic || parameter.Rest || parameter.KeywordRest
	}
	return result, variadic
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
		result := types.FunctionOf(parameters, typeRef(*ref.FunctionReturn))
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
	return result
}

func containsEquivalentType(values []types.Type, candidate types.Type) bool {
	for _, value := range values {
		if types.Equivalent(value, candidate) {
			return true
		}
	}
	return false
}

func substituteMembers(input map[string]Member, substitutions map[string]types.Type) map[string]Member {
	result := make(map[string]Member, len(input))
	for name, member := range input {
		copy := member
		copy.Type = substituteType(member.Type, substitutions)
		copy.Parameters = make([]callsignature.Parameter, len(member.Parameters))
		for index, parameter := range member.Parameters {
			copy.Parameters[index] = parameter
			copy.Parameters[index].Type = substituteType(parameter.Type, substitutions)
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
