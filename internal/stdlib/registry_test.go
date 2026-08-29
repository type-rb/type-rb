package stdlib

import (
	"testing"

	"github.com/type-rb/type-rb/internal/types"
)

func TestGenericCollectionContractsInferFromEarlierArguments(t *testing.T) {
	definition, ok := Lookup("trb/internal/arrays")
	if !ok {
		t.Fatal("internal arrays contract is missing")
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

func TestCollectionContractsAreNotPublicPackages(t *testing.T) {
	for _, packagePath := range []string{"trb/std/arrays", "trb/std/hashes"} {
		if _, ok := Lookup(packagePath); ok {
			t.Fatalf("%s remains available as a public package", packagePath)
		}
	}
	for _, packagePath := range []string{"trb/internal/arrays", "trb/internal/hashes"} {
		definition, ok := Lookup(packagePath)
		if !ok || !definition.Internal {
			t.Fatalf("%s is not an internal collection contract: %#v", packagePath, definition)
		}
	}
}

func TestRuntimeExportPackagesArePublicPortableAndSorted(t *testing.T) {
	packages := RuntimeExportPackages("go")
	if len(packages) == 0 {
		t.Fatal("runtime export package catalog is empty")
	}
	previous := ""
	foundTime := false
	for _, definition := range packages {
		if definition.Internal || definition.Kind != Portable || len(definition.RuntimeExports) == 0 {
			t.Fatalf("unexpected runtime export package: %#v", definition)
		}
		if definition.Path < previous {
			t.Fatalf("runtime export packages are not sorted: %q before %q", previous, definition.Path)
		}
		previous = definition.Path
		foundTime = foundTime || definition.Path == "trb/std/time"
	}
	if !foundTime {
		t.Fatal("trb/std/time is missing from runtime export packages")
	}
}

func TestPublicPortablePackagesExcludeInternalContracts(t *testing.T) {
	packages := PublicPortablePackages("go")
	paths := map[string]bool{}
	previous := ""
	for _, definition := range packages {
		if definition.Internal || definition.Kind != Portable {
			t.Fatalf("unexpected public package: %#v", definition)
		}
		if definition.Path < previous {
			t.Fatalf("public packages are not sorted: %q before %q", previous, definition.Path)
		}
		previous = definition.Path
		paths[definition.Path] = true
	}
	if !paths["trb/std/math"] || !paths["trb/std/random"] || !paths["trb/std/digest"] {
		t.Fatalf("public package catalog is missing REPL candidates: %#v", paths)
	}
	if paths["trb/internal/arrays"] || paths["trb/internal/hashes"] {
		t.Fatalf("internal collection contracts are public: %#v", paths)
	}
}

func TestGenericReceiverContractsSpecializeReturnTypes(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
	_, slice, ok := LookupReceiverMethod(arrayType, "slice")
	if !ok {
		t.Fatal("Array#slice is missing")
	}
	if got := slice.Return.String(); got != "Array<String>" {
		t.Fatalf("Array#slice return was not specialized: %s", got)
	}
	if len(slice.Parameters) != 1 || slice.Parameters[0].Type.Kind != types.Range {
		t.Fatalf("Array#slice parameters are wrong: %#v", slice.Parameters)
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

func TestArraySortingReceiverContractsRequirePortableOrdering(t *testing.T) {
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}
	for _, name := range []string{"sort", "sort_descending"} {
		_, method, ok := LookupReceiverMethod(arrayType, name)
		if !ok || method.Return.String() != "Array<String>" {
			t.Fatalf("Array<String>#%s contract=%#v", name, method)
		}
		if len(method.OrderingTypes) != 1 || method.OrderingTypes[0].String() != "String" {
			t.Fatalf("Array<String>#%s ordering requirement=%#v", name, method.OrderingTypes)
		}
	}
}

func TestSafeReceiverContractsUseStructuredErrors(t *testing.T) {
	tests := []struct {
		receiver types.Type
		name     string
		want     string
	}{
		{receiver: types.FromName("String"), name: "try_to_i", want: "Result<Integer, NumberParseError>"},
		{receiver: types.FromName("String"), name: "try_to_f", want: "Result<Float, NumberParseError>"},
		{receiver: types.FromName("String"), name: "try_fetch", want: "Result<String, IndexLookupError>"},
		{receiver: types.FromName("String"), name: "try_slice", want: "Result<String, SliceRangeError>"},
		{receiver: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}, name: "try_fetch", want: "Result<String, IndexLookupError>"},
		{receiver: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("String")}}, name: "try_slice", want: "Result<Array<String>, SliceRangeError>"},
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

func TestStringCharacterReceiverContracts(t *testing.T) {
	for _, test := range []struct {
		name string
		want string
	}{
		{name: "chars", want: "Array<String>"},
		{name: "reverse", want: "String"},
		{name: "replace_all", want: "String"},
		{name: "slice", want: "String"},
		{name: "index", want: "Integer?"},
		{name: "rindex", want: "Integer?"},
	} {
		_, method, ok := LookupReceiverMethod(types.FromName("String"), test.name)
		if !ok || method.Return.String() != test.want {
			t.Fatalf("String#%s contract=%#v, want %s", test.name, method, test.want)
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

func TestPortableURLContract(t *testing.T) {
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
	if definition.Symbols["parse_query"].Intrinsic != "" || definition.Symbols["build_query"].Intrinsic != "" {
		t.Fatal("URL query operations must remain compiler-owned TypeRB source")
	}
	internal, ok := Lookup("trb/internal/url")
	if !ok {
		t.Fatal("internal URL package is missing")
	}
	if _, exists := internal.Symbols["parse_query"]; exists {
		t.Fatal("internal URL package must not expose query parsing as an intrinsic")
	}
	if _, exists := internal.Symbols["build_query"]; exists {
		t.Fatal("internal URL package must not expose query building as an intrinsic")
	}
	build := definition.Symbols["build_query"]
	if len(build.Parameters) != 1 || build.Parameters[0].Type.String() != "Array<QueryParameter>" || build.Return.String() != "String" {
		t.Fatalf("url.build_query contract=%#v, want Array<QueryParameter> -> String", build)
	}
}

func TestPortableHashContract(t *testing.T) {
	definition, ok := Lookup("trb/std/digest")
	if !ok {
		t.Fatal("hash package is missing")
	}
	for _, name := range []string{"md5", "sha1", "sha256", "sha512"} {
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

func TestPortableSecureCompareContract(t *testing.T) {
	definition, ok := Lookup("trb/std/secure_compare")
	if !ok {
		t.Fatal("secure_compare package is missing")
	}
	equal, ok := definition.Symbols["equal"]
	if !ok || len(equal.Parameters) != 2 || equal.Parameters[0].Type.String() != "Bytes" || equal.Parameters[1].Type.String() != "Bytes" || equal.Return.String() != "Boolean" {
		t.Fatalf("secure_compare.equal contract=%#v, want (Bytes, Bytes) -> Boolean", equal)
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
	for _, name := range []string{"include?", "index", "count", "uniq"} {
		_, method, ok := LookupReceiverMethod(arrayType, name)
		if !ok {
			t.Fatalf("Array#%s is missing", name)
		}
		if name != "uniq" && (len(method.Parameters) != 1 || method.Parameters[0].Type.String() != "String") {
			t.Fatalf("Array#%s value parameter was not specialized: %#v", name, method.Parameters)
		}
		if name == "uniq" && (len(method.Parameters) != 0 || method.Return.String() != "Array<String>" || method.ReceiverMutable) {
			t.Fatalf("Array#uniq has the wrong non-destructive contract: %#v", method)
		}
		if name == "index" && method.Return.String() != "Integer?" {
			t.Fatalf("Array#index return=%s, want Integer?", method.Return)
		}
		if len(method.EqualityTypes) != 1 || method.EqualityTypes[0].String() != "String" {
			t.Fatalf("Array#%s equality requirement was not specialized: %#v", name, method.EqualityTypes)
		}
	}
	_, concat, ok := LookupReceiverMethod(arrayType, "concat")
	if !ok || concat.ReceiverMutable || concat.Return.String() != "Array<String>" || len(concat.Parameters) != 1 || concat.Parameters[0].Type.String() != "Array<String>" {
		t.Fatalf("Array#concat has the wrong non-destructive contract: %#v", concat)
	}
}

func TestIntegerRangeCanMaterializeAnArray(t *testing.T) {
	rangeType := types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{types.FromName("Integer")}}
	definition, method, ok := LookupReceiverMethod(rangeType, "to_a")
	if !ok {
		t.Fatal("Range<Integer>#to_a is missing")
	}
	if definition.Path != "trb/std/ranges" || len(method.Parameters) != 0 || method.Return.String() != "Array<Integer>" {
		t.Fatalf("Range<Integer>#to_a contract=%#v from %#v", method, definition)
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
