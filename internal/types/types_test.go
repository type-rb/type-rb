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
