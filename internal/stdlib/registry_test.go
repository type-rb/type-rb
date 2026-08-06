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

func TestArrayMutationReceiversUsePackageMutabilityContracts(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Integer")}}
	for _, name := range []string{"pop", "push", "shift", "unshift"} {
		_, method, ok := LookupReceiverMethod(arrayType, name)
		if !ok {
			t.Fatalf("Array#%s is missing", name)
		}
		if !method.ReceiverMutable {
			t.Fatalf("Array#%s lost the package receiver mutability requirement", name)
		}
	}
	for _, name := range []string{"push", "unshift"} {
		_, method, _ := LookupReceiverMethod(arrayType, name)
		if len(method.Parameters) != 1 || method.Parameters[0].Type.Kind != types.Int {
			t.Fatalf("Array#%s value parameter was not specialized: %#v", name, method.Parameters)
		}
	}
	_, reverse, ok := LookupReceiverMethod(arrayType, "reverse")
	if !ok || reverse.ReceiverMutable || reverse.Return.String() != "Array<Integer>" {
		t.Fatalf("Array#reverse has the wrong contract: %#v", reverse)
	}
}

func TestReceiverContractsCanConstrainCollectionArguments(t *testing.T) {
	stringsType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
	if _, _, ok := LookupReceiverMethod(stringsType, "join"); !ok {
		t.Fatal("Array<String>#join is missing")
	}
	integersType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Integer")}}
	if _, _, ok := LookupReceiverMethod(integersType, "join"); ok {
		t.Fatal("Array<Integer>#join incorrectly matched Array<String>")
	}
}

func TestStringTrimmingReceiversSharePackageContracts(t *testing.T) {
	definition, ok := Lookup("trb/std/strings")
	if !ok {
		t.Fatal("strings package is missing")
	}
	for _, name := range []string{"strip", "lstrip", "rstrip"} {
		packageSymbol, ok := definition.Symbols[name]
		if !ok {
			t.Fatalf("strings.%s is missing", name)
		}
		pkg, receiverSymbol, ok := LookupReceiverMethod(types.FromName("String"), name)
		if !ok {
			t.Fatalf("String#%s is missing", name)
		}
		if pkg != definition || receiverSymbol.Intrinsic != packageSymbol.Intrinsic {
			t.Fatalf("String#%s does not share its package contract: package=%v receiver=%s expected=%s", name, pkg, receiverSymbol.Intrinsic, packageSymbol.Intrinsic)
		}
		if len(receiverSymbol.Parameters) != 0 || receiverSymbol.Return.Kind != types.String {
			t.Fatalf("String#%s has the wrong signature: %#v", name, receiverSymbol)
		}
	}
}
