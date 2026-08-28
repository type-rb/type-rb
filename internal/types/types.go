// Package types contains target-independent semantic types.
package types

import (
	"errors"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
)

type Kind string

const (
	MinPortableInteger int64 = -9007199254740991
	MaxPortableInteger int64 = 9007199254740991
)

const (
	Invalid Kind = "invalid"
	// Never is the compiler-internal bottom type for expressions that cannot
	// produce a value because control leaves the current flow.
	Never         Kind = "never"
	Any           Kind = "any"
	Void          Kind = "void"
	Bool          Kind = "bool"
	Int           Kind = "int"
	IntLiteral    Kind = "int_literal"
	Float         Kind = "float"
	String        Kind = "string"
	StringLiteral Kind = "string_literal"
	Bytes         Kind = "bytes"
	StringBuilder Kind = "string_builder"
	Array         Kind = "array"
	Range         Kind = "range"
	Iterable      Kind = "iterable"
	Hash          Kind = "hash"
	Function      Kind = "function"
	Union         Kind = "union"
	Named         Kind = "named"
	Nil           Kind = "nil"
)

type Type struct {
	Kind        Kind
	Name        string
	Args        []Type
	Nullable    bool
	Readonly    bool
	Declaration identity.Declaration
}

func (t Type) String() string {
	if t.Kind == Function && len(t.Args) > 0 {
		parts := make([]string, len(t.Args)-1)
		for index := range parts {
			parts[index] = t.Args[index].String()
		}
		name := "(" + strings.Join(parts, ", ") + ") -> " + t.Args[len(t.Args)-1].String()
		if t.Nullable {
			return "(" + name + ")?"
		}
		return name
	}
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

func FunctionOf(parameters []Type, result Type) Type {
	arguments := append([]Type(nil), parameters...)
	arguments = append(arguments, result)
	return Type{Kind: Function, Name: "Function", Args: arguments}
}

func FunctionSignature(function Type) ([]Type, Type, bool) {
	if function.Kind != Function || len(function.Args) == 0 {
		return nil, Type{}, false
	}
	return function.Args[:len(function.Args)-1], function.Args[len(function.Args)-1], true
}

func FromName(name string) Type {
	if literal, ok := LiteralFromSource(name); ok {
		return literal
	}
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

// LiteralFromSource returns the compile-time type denoted by an explicit
// Integer or String literal in a type position. Literal types are erased to
// their ordinary scalar representation by non-TypeScript backends.
func LiteralFromSource(source string) (Type, bool) {
	if strings.HasPrefix(source, `"`) {
		value, err := strconv.Unquote(source)
		if err != nil {
			return Type{}, false
		}
		return Type{Kind: StringLiteral, Name: strconv.Quote(value)}, true
	}
	value, ok := ParsePortableIntegerLiteral(source)
	if !ok {
		return Type{}, false
	}
	return Type{Kind: IntLiteral, Name: strconv.FormatInt(value, 10)}, true
}

func ParsePortableIntegerLiteral(source string) (int64, bool) {
	value, err := strconv.ParseInt(strings.ReplaceAll(source, "_", ""), 10, 64)
	return value, err == nil && value >= MinPortableInteger && value <= MaxPortableInteger
}

// ParsePortableFloatLiteral accepts finite binary64 literals. Runtime Float
// arithmetic may produce infinities and NaN, but source literals never spell
// non-finite values directly. Underflow follows binary64 and rounds to zero.
func ParsePortableFloatLiteral(source string) (float64, bool) {
	value, err := strconv.ParseFloat(strings.ReplaceAll(source, "_", ""), 64)
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return value, false
	}
	return value, err == nil || value == 0 && errors.Is(err, strconv.ErrRange)
}

func IsIntegerLiteralSource(source string) bool {
	normalized := strings.ReplaceAll(source, "_", "")
	if len(normalized) > 0 && (normalized[0] == '+' || normalized[0] == '-') {
		normalized = normalized[1:]
	}
	if normalized == "" {
		return false
	}
	for _, character := range normalized {
		if character < '0' || character > '9' {
			return false
		}
	}
	return true
}

func IsLiteral(typ Type) bool {
	return typ.Kind == IntLiteral || typ.Kind == StringLiteral
}

func LiteralBase(typ Type) (Type, bool) {
	switch typ.Kind {
	case IntLiteral:
		return FromName("Integer"), true
	case StringLiteral:
		return FromName("String"), true
	default:
		return Type{}, false
	}
}

func LiteralUnionBase(typ Type) (Type, bool) {
	if typ.Kind != Union || len(typ.Args) == 0 {
		return Type{}, false
	}
	base, literal := LiteralBase(typ.Args[0])
	if !literal {
		return Type{}, false
	}
	for _, alternative := range typ.Args[1:] {
		current, ok := LiteralBase(alternative)
		if !ok || !Equivalent(base, current) {
			return Type{}, false
		}
	}
	return base, true
}

// CommonType returns the most-specific type that can represent both inputs
// through portable implicit conversions. It deliberately does not fall back to
// Any: callers choose whether a missing common type is an error, a union, or a
// dynamic boundary.
func CommonType(left, right Type) (Type, bool) {
	if left.Kind == Never {
		return right, true
	}
	if right.Kind == Never {
		return left, true
	}
	if Equivalent(left, right) {
		return left, true
	}
	if base, literal := LiteralBase(left); literal && Equivalent(base, right) {
		return right, true
	}
	if base, literal := LiteralBase(right); literal && Equivalent(left, base) {
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
	sawNever := false
	var appendType func(Type)
	appendType = func(candidate Type) {
		if candidate.Kind == Never {
			sawNever = true
			return
		}
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
	hasInteger := false
	hasString := false
	for _, alternative := range alternatives {
		hasFloat = hasFloat || alternative.Kind == Float && !alternative.Nullable
		hasInteger = hasInteger || alternative.Kind == Int && !alternative.Nullable
		hasString = hasString || alternative.Kind == String && !alternative.Nullable
	}
	if hasFloat || hasInteger || hasString {
		filtered := alternatives[:0]
		for _, alternative := range alternatives {
			subsumed := hasFloat && !alternative.Nullable && (alternative.Kind == Int || alternative.Kind == IntLiteral)
			subsumed = subsumed || hasInteger && !alternative.Nullable && alternative.Kind == IntLiteral
			subsumed = subsumed || hasString && !alternative.Nullable && alternative.Kind == StringLiteral
			if !subsumed {
				filtered = append(filtered, alternative)
			}
		}
		alternatives = filtered
	}
	slices.SortFunc(alternatives, func(left, right Type) int {
		if compared := strings.Compare(left.String(), right.String()); compared != 0 {
			return compared
		}
		return strings.Compare(left.Declaration.Key(), right.Declaration.Key())
	})
	if len(alternatives) == 0 {
		if sawNever {
			return Type{Kind: Never, Name: "Never"}
		}
		return Type{Kind: Invalid, Name: "Invalid"}
	}
	if len(alternatives) == 1 {
		return alternatives[0]
	}
	return Type{Kind: Union, Name: "Union", Args: alternatives}
}

func Assignable(target, value Type) bool {
	if value.Kind == Never {
		return true
	}
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
	if Equivalent(target, value) {
		return true
	}
	if base, literal := LiteralBase(value); literal && Equivalent(target, base) {
		return true
	}
	if base, literal := LiteralBase(value); literal && base.Kind == Int && target.Kind == Float {
		return true
	}
	if IsLiteral(target) || IsLiteral(value) {
		return false
	}
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
	if target.Kind == Function && value.Kind == Function {
		targetParameters, targetReturn, targetOK := FunctionSignature(target)
		valueParameters, valueReturn, valueOK := FunctionSignature(value)
		if !targetOK || !valueOK || len(targetParameters) != len(valueParameters) {
			return false
		}
		for index := range targetParameters {
			if !Equivalent(targetParameters[index], valueParameters[index]) {
				return false
			}
		}
		return Equivalent(targetReturn, valueReturn)
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
	if !left.Declaration.Empty() && !right.Declaration.Empty() && left.Declaration != right.Declaration {
		return false
	}
	for index := range left.Args {
		if !Equivalent(left.Args[index], right.Args[index]) {
			return false
		}
	}
	return true
}
