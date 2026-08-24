package types

import (
	"math"
	"strings"
	"testing"
)

func TestPortableFloatLiteralsAreFiniteBinary64Values(t *testing.T) {
	if value, ok := ParsePortableFloatLiteral("1.7976931348623157"); !ok || value != 1.7976931348623157 {
		t.Fatalf("ordinary Float literal=%v, accepted=%t", value, ok)
	}
	if value, ok := ParsePortableFloatLiteral("0." + strings.Repeat("0", 400) + "1"); !ok || value != 0 {
		t.Fatalf("underflowing Float literal=%v, accepted=%t", value, ok)
	}
	if value, ok := ParsePortableFloatLiteral(strings.Repeat("9", 400) + ".0"); ok || !math.IsInf(value, 1) {
		t.Fatalf("overflowing Float literal=%v, accepted=%t", value, ok)
	}
	if value, ok := ParsePortableFloatLiteral("not-a-float"); ok || value != 0 {
		t.Fatalf("invalid Float literal=%v, accepted=%t", value, ok)
	}
}

func TestHashAssignabilityIsInvariantButIgnoresBindingReadonly(t *testing.T) {
	stringType := FromName("String")
	integerType := FromName("Integer")
	anyType := FromName("Any")
	integers := Type{Kind: Hash, Name: "Hash", Args: []Type{stringType, integerType}}
	readonlyIntegers := integers
	readonlyIntegers.Readonly = true
	anyValues := Type{Kind: Hash, Name: "Hash", Args: []Type{stringType, anyType}}

	if !Assignable(integers, readonlyIntegers) {
		t.Fatal("binding readonly state must not change the semantic Hash type")
	}
	if Assignable(anyValues, integers) || Assignable(integers, anyValues) {
		t.Fatal("mutable Hash type arguments must be invariant")
	}
	if !Assignable(integers, FromName("Hash")) {
		t.Fatal("an untyped empty Hash must remain contextually assignable")
	}
}

func TestEquivalentComparesNestedGenericArguments(t *testing.T) {
	stringType := FromName("String")
	integerType := FromName("Integer")
	inner := Type{Kind: Hash, Name: "Hash", Args: []Type{stringType, integerType}}
	left := Type{Kind: Hash, Name: "Hash", Args: []Type{stringType, inner}}
	right := left
	right.Args = append([]Type(nil), left.Args...)
	right.Readonly = true

	if !Equivalent(left, right) {
		t.Fatal("readonly is a binding property and must be ignored")
	}
	right.Args[1] = Type{Kind: Hash, Name: "Hash", Args: []Type{stringType, stringType}}
	if Equivalent(left, right) {
		t.Fatal("different nested generic arguments must not be equivalent")
	}
}

func TestAssignableNamedGenericIsInvariant(t *testing.T) {
	strings := Type{Kind: Named, Name: "Box", Args: []Type{FromName("String")}}
	integers := Type{Kind: Named, Name: "Box", Args: []Type{FromName("Integer")}}
	if !Assignable(strings, strings) {
		t.Fatal("same generic instantiation should be assignable")
	}
	if Assignable(strings, integers) || Assignable(Type{Kind: Named, Name: "Box"}, strings) {
		t.Fatal("different or missing named generic arguments must not be assignable")
	}
}

func TestFunctionTypesAreInvariant(t *testing.T) {
	integer := FromName("Integer")
	float := FromName("Float")
	stringType := FromName("String")
	integerToString := FunctionOf([]Type{integer}, stringType)
	integerToInteger := FunctionOf([]Type{integer}, integer)
	integerToFloat := FunctionOf([]Type{integer}, float)
	floatToString := FunctionOf([]Type{float}, stringType)

	if integerToString.String() != "(Integer) -> String" {
		t.Fatalf("function type string=%s", integerToString)
	}
	if !Assignable(integerToString, integerToString) {
		t.Fatal("equivalent function types must be assignable")
	}
	if Assignable(integerToString, floatToString) {
		t.Fatal("function parameters must be invariant")
	}
	if Assignable(integerToFloat, integerToInteger) {
		t.Fatal("function results remain invariant until backends insert adapters")
	}
}

func TestResultReturningFunctionsUseOrdinaryFunctionTypes(t *testing.T) {
	integer := FromName("Integer")
	appError := FromName("AppError")
	pure := FunctionOf(nil, integer)
	result := Type{Kind: Named, Name: "Result", Args: []Type{integer, appError}}
	resultReturning := FunctionOf(nil, result)

	if resultReturning.String() != "() -> Result<Integer, AppError>" {
		t.Fatalf("Result function type string=%s", resultReturning)
	}
	if Assignable(resultReturning, pure) || Assignable(pure, resultReturning) {
		t.Fatal("Result and non-Result return types must remain distinct")
	}
	if Equivalent(resultReturning, pure) {
		t.Fatal("ordinary function return types are part of semantic identity")
	}
}

func TestCommonTypeUsesPortableNumericWidening(t *testing.T) {
	integer := FromName("Integer")
	float := FromName("Float")
	stringType := FromName("String")

	if common, ok := CommonType(integer, integer); !ok || !Equivalent(common, integer) {
		t.Fatalf("equivalent types should retain their type: %s, %v", common, ok)
	}
	for _, pair := range [][2]Type{{integer, float}, {float, integer}} {
		if common, ok := CommonType(pair[0], pair[1]); !ok || !Equivalent(common, float) {
			t.Fatalf("Integer and Float should join as Float: %s, %v", common, ok)
		}
	}
	if common, ok := CommonType(integer, stringType); !ok || common.String() != "Integer | String" {
		t.Fatalf("unrelated types should form a normalized union, got %s, %v", common, ok)
	}
}

func TestNeverIsTheInternalBottomType(t *testing.T) {
	never := Type{Kind: Never, Name: "Never"}
	stringType := FromName("String")
	if !Assignable(stringType, never) {
		t.Fatal("Never must be assignable to every value type")
	}
	for _, pair := range [][2]Type{{never, stringType}, {stringType, never}} {
		common, ok := CommonType(pair[0], pair[1])
		if !ok || !Equivalent(common, stringType) {
			t.Fatalf("Never and String should join as String, got %s, %v", common, ok)
		}
	}
	if union := UnionOf(never, stringType); !Equivalent(union, stringType) {
		t.Fatalf("Never must not add an alternative to a union, got %s", union)
	}
}

func TestUnionNormalizationAndAssignability(t *testing.T) {
	integer := FromName("Integer")
	float := FromName("Float")
	stringType := FromName("String")
	union := UnionOf(stringType, UnionOf(integer, stringType))
	if union.String() != "Integer | String" {
		t.Fatalf("union was not flattened, deduplicated, and sorted: %s", union)
	}
	if widened := UnionOf(stringType, integer, float); widened.String() != "Float | String" {
		t.Fatalf("Integer should be subsumed by Float in a union: %s", widened)
	}
	if !Assignable(union, integer) || !Assignable(union, stringType) {
		t.Fatal("each alternative should be assignable to its union")
	}
	if Assignable(integer, union) || Assignable(stringType, union) {
		t.Fatal("a union should not be assignable to one of its alternatives")
	}
	wider := UnionOf(integer, stringType, FromName("Boolean"))
	if !Assignable(wider, union) || Assignable(union, wider) {
		t.Fatal("union assignability should follow alternative inclusion")
	}
	numericWidened := UnionOf(float, stringType)
	if !Assignable(numericWidened, union) {
		t.Fatal("each source union alternative should permit a safe target conversion")
	}
}

func TestLiteralTypesRetainDiscriminantsAndWidenToScalars(t *testing.T) {
	created, ok := LiteralFromSource("2_01")
	if !ok || created.String() != "201" {
		t.Fatalf("Integer literal type was not canonicalized: %s, %v", created, ok)
	}
	invalid, ok := LiteralFromSource(`"invalid"`)
	if !ok || invalid.String() != `"invalid"` {
		t.Fatalf("String literal type was not retained: %s, %v", invalid, ok)
	}
	for _, source := range []string{"9007199254740991", "-9007199254740991"} {
		if _, ok := LiteralFromSource(source); !ok {
			t.Fatalf("portable boundary literal %s was rejected", source)
		}
	}
	for _, source := range []string{"9007199254740992", "-9007199254740992", "9223372036854775808"} {
		if literal, ok := LiteralFromSource(source); ok {
			t.Fatalf("non-portable literal %s was accepted as %s", source, literal)
		}
	}
	statuses := UnionOf(created, FromName("422"))
	if statuses.String() != "201 | 422" {
		t.Fatalf("literal union lost its alternatives: %s", statuses)
	}
	if !Assignable(FromName("Integer"), created) || Assignable(created, FromName("Integer")) {
		t.Fatal("a literal must widen to its scalar, while a scalar must not narrow implicitly")
	}
	if widened := UnionOf(created, FromName("Integer")); !Equivalent(widened, FromName("Integer")) {
		t.Fatalf("a scalar alternative must subsume its literals: %s", widened)
	}
}

func TestArrayAssignabilityIsInvariant(t *testing.T) {
	integers := Type{Kind: Array, Name: "Array", Args: []Type{FromName("Integer")}}
	values := Type{Kind: Array, Name: "Array", Args: []Type{UnionOf(FromName("Integer"), FromName("String"))}}
	if Assignable(values, integers) || Assignable(integers, values) {
		t.Fatal("mutable Array type arguments must be invariant")
	}
}
