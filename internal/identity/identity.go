// Package identity defines canonical semantic declaration identities shared by
// resolution, checking, typed IR, and backend analysis.
package identity

import "strings"

type Kind string

const (
	Class     Kind = "class"
	Record    Kind = "record"
	Enum      Kind = "enum"
	TypeAlias Kind = "type_alias"
	Newtype   Kind = "newtype"
	Module    Kind = "module"
	Interface Kind = "interface"
	Function  Kind = "function"
	Value     Kind = "value"
)

func (k Kind) IsType() bool {
	switch k {
	case Class, Record, Enum, TypeAlias, Newtype, Interface:
		return true
	default:
		return false
	}
}

// Declaration identifies one semantic declaration independently from the
// spelling used at a source reference and from any backend-generated name.
// Module is the canonical compiler module path. Name is qualified with :: for
// declarations nested in source modules.
type Declaration struct {
	Module string
	Name   string
	Kind   Kind
}

func (d Declaration) Empty() bool {
	return d.Module == "" && d.Name == "" && d.Kind == ""
}

func (d Declaration) Key() string {
	if d.Empty() {
		return ""
	}
	return d.Module + "\x00" + string(d.Kind) + "\x00" + d.Name
}

func (d Declaration) LeafName() string {
	if index := strings.LastIndex(d.Name, "::"); index >= 0 {
		return d.Name[index+2:]
	}
	return d.Name
}

// Dispatch identifies a source member independently from target-language
// mangling. Class distinguishes class and instance namespaces.
type Dispatch struct {
	Owner Declaration
	Name  string
	Class bool
}

func (d Dispatch) Empty() bool {
	return d.Owner.Empty() && d.Name == ""
}

func Qualify(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + "::" + name
}
