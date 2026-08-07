// Package types contains target-independent semantic types.
package types

import (
	"slices"
	"strings"
)

type Kind string

const (
	Invalid       Kind = "invalid"
	Any           Kind = "any"
	Void          Kind = "void"
	Bool          Kind = "bool"
	Int           Kind = "int"
	Float         Kind = "float"
	String        Kind = "string"
	Bytes         Kind = "bytes"
	StringBuilder Kind = "string_builder"
	Array         Kind = "array"
	Range         Kind = "range"
	Iterable      Kind = "iterable"
	Hash          Kind = "hash"
	Union         Kind = "union"
	Named         Kind = "named"
	Nil           Kind = "nil"
)

type Type struct {
	Kind     Kind
	Name     string
	Args     []Type
	Nullable bool
	Readonly bool
}

func (t Type) String() string {
	if t.Kind == Union {
		parts := make([]string, len(t.Args))
		for index, alternative := range t.Args {
			parts[index] = alternative.String()
		}
		name := strings.Join(parts, " | ")
		if t.Nullable {
			return "(" + name + ")?"
		}
		return name
	}
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
	case "bytes":
		return Type{Kind: Bytes, Name: "Bytes"}
	case "stringbuilder", "string_builder":
		return Type{Kind: StringBuilder, Name: "StringBuilder"}
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

// CommonType returns the most-specific type that can represent both inputs
// through portable implicit conversions. It deliberately does not fall back to
// Any: callers choose whether a missing common type is an error, a union, or a
// dynamic boundary.
func CommonType(left, right Type) (Type, bool) {
	if Equivalent(left, right) {
		return left, true
	}
	if left.Kind == Any || right.Kind == Any {
		return FromName("Any"), true
	}
	if !left.Nullable && !right.Nullable &&
		((left.Kind == Int && right.Kind == Float) || (left.Kind == Float && right.Kind == Int)) {
		return FromName("Float"), true
	}
	return UnionOf(left, right), true
}

// UnionOf constructs a canonical union. Nested unions are flattened,
// equivalent alternatives are removed, and Integer is subsumed by Float
// because portable Integer-to-Float widening is safe.
func UnionOf(input ...Type) Type {
	var alternatives []Type
	var appendType func(Type)
	appendType = func(candidate Type) {
		if candidate.Kind == Any {
			alternatives = []Type{FromName("Any")}
			return
		}
		if len(alternatives) == 1 && alternatives[0].Kind == Any {
			return
		}
		if candidate.Kind == Union && !candidate.Nullable {
			for _, nested := range candidate.Args {
				appendType(nested)
			}
			return
		}
		for _, existing := range alternatives {
			if Equivalent(existing, candidate) {
				return
			}
		}
		alternatives = append(alternatives, candidate)
	}
	for _, candidate := range input {
		appendType(candidate)
	}

	hasFloat := false
	for _, alternative := range alternatives {
		hasFloat = hasFloat || alternative.Kind == Float && !alternative.Nullable
	}
	if hasFloat {
		filtered := alternatives[:0]
		for _, alternative := range alternatives {
			if alternative.Kind != Int || alternative.Nullable {
				filtered = append(filtered, alternative)
			}
		}
		alternatives = filtered
	}
	slices.SortFunc(alternatives, func(left, right Type) int {
		return strings.Compare(left.String(), right.String())
	})
	if len(alternatives) == 0 {
		return Type{Kind: Invalid, Name: "Invalid"}
	}
	if len(alternatives) == 1 {
		return alternatives[0]
	}
	return Type{Kind: Union, Name: "Union", Args: alternatives}
}

func Assignable(target, value Type) bool {
	if target.Kind == Any || value.Kind == Any || target.Kind == Invalid || value.Kind == Invalid {
		return true
	}
	if target.Kind == Nil {
		return value.Kind == Nil
	}
	if value.Kind == Nil {
		return target.Nullable
	}
	if value.Nullable && !target.Nullable {
		return false
	}
	target.Nullable = false
	value.Nullable = false
	if target.Kind == Union {
		values := []Type{value}
		if value.Kind == Union {
			values = value.Args
		}
		for _, candidate := range values {
			accepted := false
			for _, alternative := range target.Args {
				if Assignable(alternative, candidate) {
					accepted = true
					break
				}
			}
			if !accepted {
				return false
			}
		}
		return true
	}
	if value.Kind == Union {
		for _, alternative := range value.Args {
			if !Assignable(target, alternative) {
				return false
			}
		}
		return true
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
	if (target.Kind == Array || target.Kind == Hash) && value.Kind == target.Kind {
		if len(target.Args) == 0 || len(value.Args) == 0 {
			return true
		}
		if len(target.Args) != len(value.Args) {
			return false
		}
		for index := range target.Args {
			if !Equivalent(target.Args[index], value.Args[index]) {
				return false
			}
		}
		return true
	}
	if target.Kind != value.Kind {
		return false
	}
	if target.Kind == Named && target.Name != value.Name {
		return false
	}
	if target.Kind == Named && (len(target.Args) > 0 || len(value.Args) > 0) {
		if len(target.Args) != len(value.Args) {
			return false
		}
		for index := range target.Args {
			if !Equivalent(target.Args[index], value.Args[index]) {
				return false
			}
		}
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
