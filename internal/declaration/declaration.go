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
}

type Member struct {
	Name           string
	Kind           MemberKind
	Intrinsic      string
	Parameters     []Parameter
	Return         types.Type
	Variadic       bool
	Class          bool
	TypeParameters []string
	Provider       string
}

type Type struct {
	Name            string
	Superclass      string
	InstanceMembers map[string]Member
	ClassMembers    map[string]Member
}

type Module struct {
	Name            string
	InstanceMembers map[string]Member
}

type Catalog struct {
	Types   map[string]*Type
	Modules map[string]*Module
}

func NewCatalog() *Catalog {
	return &Catalog{Types: map[string]*Type{}, Modules: map[string]*Module{}}
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
