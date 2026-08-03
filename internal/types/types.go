// Package types contains target-independent semantic types.
package types

import "strings"

type Kind string

const (
	Invalid  Kind = "invalid"
	Any      Kind = "any"
	Void     Kind = "void"
	Bool     Kind = "bool"
	Int      Kind = "int"
	Float    Kind = "float"
	String   Kind = "string"
	Array    Kind = "array"
	Range    Kind = "range"
	Iterable Kind = "iterable"
	Hash     Kind = "hash"
	Named    Kind = "named"
	Nil      Kind = "nil"
)

type Type struct {
	Kind     Kind
	Name     string
	Args     []Type
	Nullable bool
	Readonly bool
}

func (t Type) String() string {
	name := t.Name
	if name == "" {
		name = string(t.Kind)
	}
	if len(t.Args) > 0 {
		parts := make([]string, len(t.Args))
		for i, arg := range t.Args {
			parts[i] = arg.String()
		}
		name += "<" + strings.Join(parts, ", ") + ">"
	}
	if t.Nullable {
		name += "?"
	}
	return name
}

func FromName(name string) Type {
	switch strings.ToLower(name) {
	case "", "void":
		return Type{Kind: Void, Name: "Void"}
	case "any", "untyped":
		return Type{Kind: Any, Name: "Any"}
	case "bool", "boolean":
		return Type{Kind: Bool, Name: "Boolean"}
	case "int", "integer":
		return Type{Kind: Int, Name: "Integer"}
	case "float", "float64", "number":
		return Type{Kind: Float, Name: "Float"}
	case "string", "symbol":
		return Type{Kind: String, Name: "String"}
	case "nil", "null":
		return Type{Kind: Nil, Name: "Nil"}
	case "array":
		return Type{Kind: Array, Name: "Array"}
	case "range":
		return Type{Kind: Range, Name: "Range"}
	case "iterable":
		return Type{Kind: Iterable, Name: "Iterable"}
	case "hash", "map":
		return Type{Kind: Hash, Name: "Hash"}
	default:
		return Type{Kind: Named, Name: name}
	}
}

func Assignable(target, value Type) bool {
	if target.Kind == Any || value.Kind == Any || target.Kind == Invalid || value.Kind == Invalid {
		return true
	}
	if value.Kind == Nil {
		return target.Nullable
	}
	if target.Kind == Float && value.Kind == Int {
		return true
	}
	if target.Kind == Iterable && (value.Kind == Iterable || value.Kind == Array || value.Kind == Range) {
		if len(target.Args) == 0 || len(value.Args) == 0 {
			return true
		}
		return len(target.Args) == len(value.Args) && Assignable(target.Args[0], value.Args[0])
	}
	if target.Kind == Hash && value.Kind == Hash {
		if len(target.Args) == 0 || len(value.Args) == 0 {
			return true
		}
		return len(target.Args) == 2 && len(value.Args) == 2 && Equivalent(target.Args[0], value.Args[0]) && Equivalent(target.Args[1], value.Args[1])
	}
	if target.Kind != value.Kind {
		return false
	}
	if target.Kind == Named && target.Name != value.Name {
		return false
	}
	return true
}

// Equivalent compares semantic types while ignoring binding-level readonly
// state. Mutable generic containers use it to avoid unsound argument widening.
func Equivalent(left, right Type) bool {
	if left.Kind != right.Kind || left.Name != right.Name || left.Nullable != right.Nullable || len(left.Args) != len(right.Args) {
		return false
	}
	for index := range left.Args {
		if !Equivalent(left.Args[index], right.Args[index]) {
			return false
		}
	}
	return true
}
