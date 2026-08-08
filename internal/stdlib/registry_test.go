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

func TestSafeReceiverContractsUseStructuredErrors(t *testing.T) {
	tests := []struct {
		receiver types.Type
		name     string
		want     string
	}{
		{receiver: types.FromName("String"), name: "try_to_i", want: "Result<Integer, NumberParseError>"},
		{receiver: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}, name: "try_fetch", want: "Result<String, IndexLookupError>"},
		{receiver: types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Integer"), types.FromName("String")}}, name: "try_fetch", want: "Result<String, KeyLookupError>"},
	}
	for _, test := range tests {
		_, method, ok := LookupReceiverMethod(test.receiver, test.name)
		if !ok {
			t.Fatalf("%s#%s is missing", test.receiver, test.name)
		}
		if got := method.Return.String(); got != test.want {
			t.Fatalf("%s#%s return=%s, want %s", test.receiver, test.name, got, test.want)
		}
	}
}

func TestNumericReceiverAndMathContracts(t *testing.T) {
	for _, test := range []struct {
		receiver types.Type
		name     string
		want     string
	}{
		{receiver: types.FromName("Integer"), name: "min", want: "Integer"},
		{receiver: types.FromName("Integer"), name: "max", want: "Integer"},
		{receiver: types.FromName("Integer"), name: "clamp", want: "Integer"},
		{receiver: types.FromName("Float"), name: "floor", want: "Integer"},
		{receiver: types.FromName("Float"), name: "ceil", want: "Integer"},
		{receiver: types.FromName("Float"), name: "round", want: "Integer"},
		{receiver: types.FromName("Float"), name: "truncate", want: "Integer"},
	} {
		_, method, ok := LookupReceiverMethod(test.receiver, test.name)
		if !ok || method.Return.String() != test.want {
			t.Fatalf("%s#%s contract=%#v, want %s", test.receiver, test.name, method, test.want)
		}
	}

	definition, ok := Lookup("trb/std/math")
	if !ok {
		t.Fatal("math package is missing")
	}
	for _, name := range []string{"sqrt", "exp", "log", "log2", "log10"} {
		symbol, ok := definition.Symbols[name]
		if !ok || len(symbol.Parameters) != 1 || symbol.Parameters[0].Type.Kind != types.Float || symbol.Return.Kind != types.Float {
			t.Fatalf("math.%s contract=%#v", name, symbol)
		}
	}
}

func TestPortableHexContract(t *testing.T) {
	definition, ok := Lookup("trb/std/encoding/hex")
	if !ok {
		t.Fatal("hex package is missing")
	}
	if got := definition.Symbols["encode"].Return.String(); got != "String" {
		t.Fatalf("hex.encode return=%s, want String", got)
	}
	if got := definition.Symbols["decode"].Return.String(); got != "Result<Bytes, HexDecodeError>" {
		t.Fatalf("hex.decode return=%s, want Result<Bytes, HexDecodeError>", got)
	}
}

func TestPortableBase64Contract(t *testing.T) {
	definition, ok := Lookup("trb/std/encoding/base64")
	if !ok {
		t.Fatal("base64 package is missing")
	}
	for _, name := range []string{"encode", "url_encode"} {
		if got := definition.Symbols[name].Return.String(); got != "String" {
			t.Fatalf("base64.%s return=%s, want String", name, got)
		}
	}
	for _, name := range []string{"decode", "url_decode"} {
		if got := definition.Symbols[name].Return.String(); got != "Result<Bytes, Base64DecodeError>" {
			t.Fatalf("base64.%s return=%s, want Result<Bytes, Base64DecodeError>", name, got)
		}
	}
}

func TestPortableURLComponentContract(t *testing.T) {
	definition, ok := Lookup("trb/std/url")
	if !ok {
		t.Fatal("url package is missing")
	}
	if got := definition.Symbols["encode_component"].Return.String(); got != "String" {
		t.Fatalf("url.encode_component return=%s, want String", got)
	}
	if got := definition.Symbols["decode_component"].Return.String(); got != "Result<String, PercentDecodeError>" {
		t.Fatalf("url.decode_component return=%s, want Result<String, PercentDecodeError>", got)
	}
	if got := definition.Symbols["parse_query"].Return.String(); got != "Result<Array<QueryParameter>, PercentDecodeError>" {
		t.Fatalf("url.parse_query return=%s, want Result<Array<QueryParameter>, PercentDecodeError>", got)
	}
	build := definition.Symbols["build_query"]
	if len(build.Parameters) != 1 || build.Parameters[0].Type.String() != "Array<QueryParameter>" || build.Return.String() != "String" {
		t.Fatalf("url.build_query contract=%#v, want Array<QueryParameter> -> String", build)
	}
}

func TestPortableHashContract(t *testing.T) {
	definition, ok := Lookup("trb/std/hash")
	if !ok {
		t.Fatal("hash package is missing")
	}
	for _, name := range []string{"sha256", "sha512"} {
		symbol, ok := definition.Symbols[name]
		if !ok {
			t.Fatalf("hash.%s is missing", name)
		}
		if len(symbol.Parameters) != 1 || symbol.Parameters[0].Type.String() != "Bytes" || symbol.Return.String() != "Bytes" {
			t.Fatalf("hash.%s contract=%#v, want Bytes -> Bytes", name, symbol)
		}
	}
}

func TestPortableHMACContract(t *testing.T) {
	definition, ok := Lookup("trb/std/hmac")
	if !ok {
		t.Fatal("hmac package is missing")
	}
	for _, name := range []string{"sha256", "sha512"} {
		symbol, ok := definition.Symbols[name]
		if !ok || len(symbol.Parameters) != 2 || symbol.Return.String() != "Bytes" {
			t.Fatalf("hmac.%s contract=%#v, want (Bytes, Bytes) -> Bytes", name, symbol)
		}
	}
	equal, ok := definition.Symbols["equal"]
	if !ok || len(equal.Parameters) != 2 || equal.Return.String() != "Boolean" {
		t.Fatalf("hmac.equal contract=%#v, want (Bytes, Bytes) -> Boolean", equal)
	}
}

func TestPortableRandomContracts(t *testing.T) {
	random, ok := Lookup("trb/std/random")
	if !ok {
		t.Fatal("random package is missing")
	}
	if symbol := random.Symbols["float"]; len(symbol.Parameters) != 0 || symbol.Return.String() != "Float" {
		t.Fatalf("random.float contract=%#v, want () -> Float", symbol)
	}
	if symbol := random.Symbols["integer"]; len(symbol.Parameters) != 1 || symbol.Parameters[0].Type.String() != "Integer" || symbol.Return.String() != "Integer" {
		t.Fatalf("random.integer contract=%#v, want Integer -> Integer", symbol)
	}

	secure, ok := Lookup("trb/std/secure_random")
	if !ok {
		t.Fatal("secure_random package is missing")
	}
	if symbol := secure.Symbols["bytes"]; len(symbol.Parameters) != 1 || symbol.Parameters[0].Type.String() != "Integer" || symbol.Return.String() != "Bytes" {
		t.Fatalf("secure_random.bytes contract=%#v, want Integer -> Bytes", symbol)
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

func TestArrayValueQueryReceiversPreserveEqualityRequirements(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
	for _, name := range []string{"include?", "count"} {
		_, method, ok := LookupReceiverMethod(arrayType, name)
		if !ok {
			t.Fatalf("Array#%s is missing", name)
		}
		if len(method.Parameters) != 1 || method.Parameters[0].Type.String() != "String" {
			t.Fatalf("Array#%s value parameter was not specialized: %#v", name, method.Parameters)
		}
		if len(method.EqualityTypes) != 1 || method.EqualityTypes[0].String() != "String" {
			t.Fatalf("Array#%s equality requirement was not specialized: %#v", name, method.EqualityTypes)
		}
	}
}

func TestHashMutationReceiversPreserveMutabilityAndExactTypes(t *testing.T) {
	hashType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("Integer"), types.FromName("String")}}
	for _, name := range []string{"delete", "update"} {
		_, method, ok := LookupReceiverMethod(hashType, name)
		if !ok {
			t.Fatalf("Hash#%s is missing", name)
		}
		if !method.ReceiverMutable {
			t.Fatalf("Hash#%s lost the package receiver mutability requirement", name)
		}
	}
	_, deleteMethod, _ := LookupReceiverMethod(hashType, "delete")
	if deleteMethod.Return.String() != "String" || len(deleteMethod.Parameters) != 1 || deleteMethod.Parameters[0].Type.String() != "Integer" {
		t.Fatalf("Hash#delete has the wrong specialized contract: %#v", deleteMethod)
	}
	_, updateMethod, _ := LookupReceiverMethod(hashType, "update")
	if len(updateMethod.Parameters) != 1 || !updateMethod.Parameters[0].Exact || updateMethod.Parameters[0].Type.String() != "Hash<Integer, String>" {
		t.Fatalf("Hash#update has the wrong exact argument contract: %#v", updateMethod)
	}
	_, mergeMethod, ok := LookupReceiverMethod(hashType, "merge")
	if !ok || mergeMethod.ReceiverMutable || mergeMethod.Return.String() != "Hash<Integer, String>" || len(mergeMethod.Parameters) != 1 || !mergeMethod.Parameters[0].Exact {
		t.Fatalf("Hash#merge has the wrong contract: %#v", mergeMethod)
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
