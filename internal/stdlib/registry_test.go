package stdlib

import (
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func TestGenericPackageContractsInferFromEarlierArguments(t *testing.T) {
	definition, ok := Lookup("trb/std/arrays")
	if !ok {
		t.Fatal("arrays package is missing")
	}
	values := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Integer")}}
	symbol := Instantiate(definition.Symbols["push"], []types.Type{values, types.FromName("String")})
	if got := symbol.Parameters[0].Type.String(); got != "Array<Integer>" {
		t.Fatalf("array parameter was not specialized: %s", got)
	}
	if got := symbol.Parameters[1].Type.String(); got != "Integer" {
		t.Fatalf("later use did not retain the first T binding: %s", got)
	}
}

func TestGenericReceiverContractsSpecializeReturnTypes(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
	_, fetch, ok := LookupReceiverMethod(arrayType, "fetch")
	if !ok {
		t.Fatal("Array#fetch is missing")
	}
	if got := fetch.Return.String(); got != "String" {
		t.Fatalf("Array#fetch return was not specialized: %s", got)
	}
	if len(fetch.Parameters) != 1 || fetch.Parameters[0].Type.Kind != types.Int {
		t.Fatalf("Array#fetch parameters are wrong: %#v", fetch.Parameters)
	}

	hashType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Integer"), types.FromName("String")}}
	_, keys, ok := LookupReceiverMethod(hashType, "keys")
	if !ok {
		t.Fatal("Hash#keys is missing")
	}
	if got := keys.Return.String(); got != "Array<Integer>" {
		t.Fatalf("Hash#keys return was not specialized: %s", got)
	}
}

func TestArrayPushReceiverUsesPackageMutabilityContract(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Integer")}}
	_, push, ok := LookupReceiverMethod(arrayType, "push")
	if !ok {
		t.Fatal("Array#push is missing")
	}
	if !push.ReceiverMutable {
		t.Fatal("Array#push lost the package receiver mutability requirement")
	}
	if len(push.Parameters) != 1 || push.Parameters[0].Type.Kind != types.Int {
		t.Fatalf("Array#push value parameter was not specialized: %#v", push.Parameters)
	}
}
