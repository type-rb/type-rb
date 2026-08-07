package types

import "testing"

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

func TestArrayAssignabilityIsInvariant(t *testing.T) {
	integers := Type{Kind: Array, Name: "Array", Args: []Type{FromName("Integer")}}
	values := Type{Kind: Array, Name: "Array", Args: []Type{UnionOf(FromName("Integer"), FromName("String"))}}
	if Assignable(values, integers) || Assignable(integers, values) {
		t.Fatal("mutable Array type arguments must be invariant")
	}
}
