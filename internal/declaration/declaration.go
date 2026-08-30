// Package declaration defines compiler-owned type information supplied by
// runtime-library providers. It is deliberately independent of syntax ASTs:
// RBS, .d.ts, Go export data, and dynamic framework indexers can all target the
// same representation without making application authors maintain shadow
// declaration files.
package declaration

import "github.com/type-rb/type-rb/internal/types"

type MemberKind string

const (
	Method   MemberKind = "method"
	Property MemberKind = "property"
)

type Parameter struct {
	Name     string
	Type     types.Type
	Keyword  bool
	Optional bool
	// RepresentationBoundary accepts a source type when its runtime
	// representation is assignable to Type. Package providers use this only at
	// typed serialization or persistence boundaries; ordinary TypeRB calls keep
	// nominal newtypes distinct.
	RepresentationBoundary bool
	LiteralValues          []string
	LiteralArrays          [][]string
	LiteralArrayElements   []string
}

type Signature struct {
	Parameters []Parameter
	Return     types.Type
	Variadic   bool
}

type Block struct {
	Parameters []types.Type
	// ScopedParameters marks compiler-owned values that cannot escape their
	// structured block. A scoped value is valid only as a direct method receiver
	// in the block that introduced it.
	ScopedParameters []bool
	// ControlBoundary prevents return, break, and next from crossing a
	// backend callback whose target-language ownership would otherwise differ.
	// Nested functions and loops still own their local control transfers.
	ControlBoundary bool
	// Return makes the block value-producing. The final expression must be
	// assignable to this type.
	Return types.Type
	// ResultBoundary makes prefix try abort this structured operation with its
	// declared Result error. A value-producing block uses Return as the success
	// type; a structured iteration uses the enclosing member's Result success.
	// It is not an authored block parameter. Authored return is rejected until
	// lexical transfer and resource cleanup share one portable contract; use try
	// to abort this boundary with Err.
	ResultBoundary types.Type
	// Structured keeps the block in typed IR instead of lowering it to a
	// backend callback. Structured blocks may be assigned or returned while
	// preserving return, break, and next in their lexical owner.
	Structured bool
}

type Member struct {
	Name      string
	Kind      MemberKind
	Intrinsic string
	// Specializer identifies a package-extension call provider. The provider
	// receives resolved, serializable semantic facts and returns ordinary
	// TypeRB helper source plus a narrow call replacement.
	Specializer      string
	Parameters       []Parameter
	MinimumArguments int
	MaximumArguments int
	Return           types.Type
	Variadic         bool
	Class            bool
	TypeParameters   []string
	Provider         string
	Alternatives     []Signature
	Block            *Block
}

type Type struct {
	Name string
	// Provider identifies the package-owned declaration provider that supplied
	// this type. Built-in and project-derived declarations leave it empty.
	Provider       string
	TypeParameters []string
	Superclass     string
	// SourceModule identifies a provider declaration backed by a project
	// source type. Outside that module, ordinary source references still need
	// an explicit import even though provider members remain available after
	// the type flows through another declaration.
	SourceModule    string
	InstanceMembers map[string]Member
	ClassMembers    map[string]Member
}

type Module struct {
	Name string
	// Provider identifies the package-owned declaration provider that supplied
	// this module. Built-in and project-derived declarations leave it empty.
	Provider        string
	InstanceMembers map[string]Member
}

// FunctionBlockRule describes a package function whose block type is derived
// from one of the call arguments. Providers use this for declarative DSLs
// without teaching the checker package-specific syntax.
type FunctionBlockRule struct {
	Package             string
	Function            string
	EnclosingSuperclass string
	TypeArgument        int
	ParameterTypeSuffix string
}

// DeclarationReference identifies a project declaration that a provider makes
// visible only in one declarative call position. It does not create a source
// import or make the declaration generally visible in the module.
type DeclarationReference struct {
	ModulePath string
	Name       string
}

// FunctionArgumentReferenceRule describes a positional argument whose values
// are compiler-resolved project declarations. Language tooling uses the same
// provider metadata as the compiler instead of hard-coding individual DSLs.
type FunctionArgumentReferenceRule struct {
	Package  string
	Function string
	Argument int
	Owner    DeclarationReference
	Targets  []DeclarationReference
}

// ClassBodyDeclarationRule marks a package function call directly inside one
// project class as declarative metadata. The checker still validates the
// ordinary call signature, but backends do not execute the call at runtime.
// Exact owner identity keeps this capability scoped to declarations already
// discovered by a bundled project-aware provider.
type ClassBodyDeclarationRule struct {
	Package  string
	Function string
	Owner    DeclarationReference
}

type Catalog struct {
	Types                          map[string]*Type
	Modules                        map[string]*Module
	FunctionBlockRules             []FunctionBlockRule
	FunctionArgumentReferenceRules []FunctionArgumentReferenceRule
	ClassBodyDeclarationRules      []ClassBodyDeclarationRule
	// RuntimeTypesByModule names compiler-owned value representations required
	// by generated declarations in a particular application module.
	RuntimeTypesByModule map[string][]types.Type
}

func NewCatalog() *Catalog {
	return &Catalog{Types: map[string]*Type{}, Modules: map[string]*Module{}, RuntimeTypesByModule: map[string][]types.Type{}}
}

func (c *Catalog) Merge(other *Catalog) {
	if c == nil || other == nil {
		return
	}
	for name, declaration := range other.Types {
		c.Types[name] = declaration
	}
	for name, declaration := range other.Modules {
		c.Modules[name] = declaration
	}
	c.FunctionBlockRules = append(c.FunctionBlockRules, other.FunctionBlockRules...)
	c.FunctionArgumentReferenceRules = append(c.FunctionArgumentReferenceRules, other.FunctionArgumentReferenceRules...)
	c.ClassBodyDeclarationRules = append(c.ClassBodyDeclarationRules, other.ClassBodyDeclarationRules...)
	for module, runtimeTypes := range other.RuntimeTypesByModule {
		c.RuntimeTypesByModule[module] = append(c.RuntimeTypesByModule[module], runtimeTypes...)
	}
}

func (c *Catalog) Type(name string) (*Type, bool) {
	if c == nil {
		return nil, false
	}
	declaration, ok := c.Types[name]
	return declaration, ok
}

func (c *Catalog) Module(name string) (*Module, bool) {
	if c == nil {
		return nil, false
	}
	declaration, ok := c.Modules[name]
	return declaration, ok
}

func (c *Catalog) Member(typeName, name string, class bool) (Member, bool) {
	return c.member(typeName, name, class, map[string]bool{})
}

func (c *Catalog) member(typeName, name string, class bool, seen map[string]bool) (Member, bool) {
	if c == nil || typeName == "" || seen[typeName] {
		return Member{}, false
	}
	seen[typeName] = true
	declaration := c.Types[typeName]
	if declaration == nil {
		return Member{}, false
	}
	members := declaration.InstanceMembers
	if class {
		members = declaration.ClassMembers
	}
	if member, ok := members[name]; ok {
		return member, true
	}
	return c.member(declaration.Superclass, name, class, seen)
}

func NewType(name, superclass string) *Type {
	return &Type{Name: name, Superclass: superclass, InstanceMembers: map[string]Member{}, ClassMembers: map[string]Member{}}
}

func NewModule(name string) *Module {
	return &Module{Name: name, InstanceMembers: map[string]Member{}}
}
