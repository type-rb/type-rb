// Package stdlib describes compiler-known portable and platform packages.
// Definitions are semantic contracts; backends lower their intrinsic IDs to
// target APIs without leaking those APIs into the TypeRB source language.
package stdlib

import (
	"path"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/types"
)

type Kind string

const (
	Portable Kind = "portable"
	Platform Kind = "platform"
)

type Parameter struct {
	Name     string
	Type     types.Type
	Optional bool
	Mutable  bool
	Exact    bool
}

type Symbol struct {
	Name            string
	Intrinsic       string
	RequiredSymbols []string
	TypeParameters  []string
	EqualityTypes   []types.Type
	Receiver        types.Type
	ReceiverMutable bool
	Parameters      []Parameter
	Return          types.Type
	Variadic        bool
	Inference       string
}

type RuntimeExport struct {
	Name string
	Kind string
}

func (s Symbol) HasReceiver() bool {
	return s.Receiver.Kind != "" && s.Receiver.Kind != types.Invalid
}

type receiverMethodTarget struct {
	PackagePath string
	Symbol      string
}

type Package struct {
	Path           string
	ModulePath     string
	RuntimeAlias   string
	RuntimeExports []RuntimeExport
	Source         string
	Kind           Kind
	Internal       bool
	Targets        map[string]bool
	NativeSyntax   bool
	TypeProvider   string
	Symbols        map[string]Symbol
}

func (p *Package) Supports(mode string) bool {
	return p != nil && (len(p.Targets) == 0 || p.Targets[mode])
}

func (p *Package) DefaultAlias() string {
	if p == nil {
		return ""
	}
	return strings.ReplaceAll(path.Base(p.Path), "-", "_")
}

var stringType = types.FromName("String")
var bytesType = types.FromName("Bytes")
var stringBuilderType = types.FromName("StringBuilder")
var unitType = types.FromName("Unit")
var integerType = types.FromName("Integer")
var floatType = types.FromName("Float")
var booleanType = types.FromName("Boolean")
var voidType = types.FromName("Void")
var anyType = types.FromName("Any")
var typeT = types.FromName("T")
var typeK = types.FromName("K")
var typeV = types.FromName("V")
var fileErrorType = types.FromName("FileError")
var jsonValueType = types.FromName("JsonValue")
var jsonErrorType = types.FromName("JsonError")
var processResultType = types.FromName("ProcessResult")
var processErrorType = types.FromName("ProcessError")
var numberParseErrorType = types.FromName("NumberParseError")
var hexDecodeErrorType = types.FromName("HexDecodeError")
var base64DecodeErrorType = types.FromName("Base64DecodeError")
var indexLookupErrorType = types.FromName("IndexLookupError")
var keyLookupErrorType = types.FromName("KeyLookupError")

var registry = map[string]*Package{
	"trb/std/unit": {
		Path:       "trb/std/unit",
		ModulePath: "trb/std/unit/index",
		Source: `record Unit
end
`,
		Kind:    Portable,
		Symbols: map[string]Symbol{},
	},
	"trb/std/result": {
		Path:           "trb/std/result",
		ModulePath:     "trb/std/result/index",
		RuntimeAlias:   "__trb_result",
		RuntimeExports: []RuntimeExport{{Name: "Result", Kind: "enum"}},
		Source: `enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end
`,
		Kind:    Portable,
		Symbols: map[string]Symbol{},
	},
	"trb/std/errors": {
		Path:         "trb/std/errors",
		ModulePath:   "trb/std/errors/index",
		RuntimeAlias: "__trb_errors",
		RuntimeExports: []RuntimeExport{
			{Name: "NumberParseErrorKind", Kind: "enum"},
			{Name: "NumberParseError", Kind: "record"},
			{Name: "IndexLookupError", Kind: "record"},
			{Name: "KeyLookupError", Kind: "record"},
		},
		Source:  errorsSource(),
		Kind:    Portable,
		Symbols: map[string]Symbol{},
	},
	"trb/std/io": {
		Path: "trb/std/io",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"puts": {
				Name:       "puts",
				Intrinsic:  "trb.std.io.puts",
				Parameters: []Parameter{{Name: "value", Type: anyType}},
				Return:     voidType,
			},
		},
	},
	"trb/std/path": {
		Path:       "trb/std/path",
		ModulePath: "trb/std/path/index",
		Source:     pathSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"separator": {Name: "separator", Intrinsic: "trb.std.path.separator", Return: stringType},
			"clean":     unary("clean", "trb.std.path.clean", stringType, stringType),
			"join": {
				Name:       "join",
				Intrinsic:  "trb.std.path.join",
				Parameters: []Parameter{{Name: "left", Type: stringType}, {Name: "right", Type: stringType}},
				Return:     stringType,
			},
			"absolute":   unary("absolute", "trb.std.path.absolute", stringType, booleanType),
			"components": unary("components", "trb.std.path.components", stringType, arrayOf(stringType)),
			"base":       unary("base", "trb.std.path.base", stringType, stringType),
			"directory":  unary("directory", "trb.std.path.directory", stringType, stringType),
		},
	},
	"trb/std/filesystem": {
		Path:       "trb/std/filesystem",
		ModulePath: "trb/std/filesystem/index",
		Source:     filesystemSource(),
		Kind:       Portable,
		Symbols:    map[string]Symbol{},
	},
	"trb/std/process": {
		Path:       "trb/std/process",
		ModulePath: "trb/std/process/index",
		Source:     processSource(),
		Kind:       Portable,
		Symbols:    map[string]Symbol{},
	},
	"trb/internal/process": {
		Path:     "trb/internal/process",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"argv": {
				Name:      "argv",
				Intrinsic: "trb.internal.process.arguments",
				Return:    arrayOf(stringType),
			},
			"environment": {
				Name:       "environment",
				Intrinsic:  "trb.internal.process.environment",
				Parameters: []Parameter{{Name: "name", Type: stringType}},
				Return:     nullable(stringType),
			},
			"working_directory": {
				Name:      "working_directory",
				Intrinsic: "trb.internal.process.working_directory",
				Return:    processResult(stringType),
			},
			"run": {
				Name:      "run",
				Intrinsic: "trb.internal.process.run",
				Parameters: []Parameter{
					{Name: "command", Type: stringType},
					{Name: "arguments", Type: arrayOf(stringType)},
				},
				Return: processResult(processResultType),
			},
		},
	},
	"trb/internal/filesystem": {
		Path:     "trb/internal/filesystem",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"exists":      filesystemUnary("exists", booleanType),
			"read_text":   filesystemUnary("read_text", stringType),
			"read_bytes":  filesystemUnary("read_bytes", bytesType),
			"write_text":  filesystemWrite("write_text", stringType),
			"write_bytes": filesystemWrite("write_bytes", bytesType),
			"create_directory": {
				Name:       "create_directory",
				Intrinsic:  "trb.internal.filesystem.create_directory",
				Parameters: []Parameter{{Name: "path", Type: stringType}},
				Return:     filesystemResult(unitType),
			},
			"list": filesystemUnary("list", arrayOf(stringType)),
		},
	},
	"trb/std/json": {
		Path:       "trb/std/json",
		ModulePath: "trb/std/json/index",
		Source:     jsonSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"parse": jsonParse("parse"),
			"decode": {
				Name:            "decode",
				Intrinsic:       "trb.internal.json.decode",
				RequiredSymbols: []string{"JsonError"},
				TypeParameters:  []string{"T"},
				Parameters:      []Parameter{{Name: "source", Type: stringType}},
				Return:          jsonResult(typeT),
			},
			"encode": {
				Name:            "encode",
				Intrinsic:       "trb.internal.json.encode",
				RequiredSymbols: []string{"JsonError"},
				TypeParameters:  []string{"T"},
				Parameters:      []Parameter{{Name: "value", Type: typeT}},
				Return:          jsonResult(stringType),
			},
			"stringify": {
				Name:       "stringify",
				Intrinsic:  "trb.internal.json.stringify",
				Parameters: []Parameter{{Name: "value", Type: jsonValueType}},
				Return:     jsonResult(stringType),
			},
		},
	},
	"trb/std/jsonc": {
		Path:       "trb/std/jsonc",
		ModulePath: "trb/std/jsonc/index",
		Source:     jsoncSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"parse": {
				Name:       "parse",
				Intrinsic:  "trb.internal.json.parse_jsonc",
				Parameters: []Parameter{{Name: "source", Type: stringType}},
				Return:     jsonResult(jsonValueType),
			},
		},
	},
	"trb/internal/json": {
		Path:     "trb/internal/json",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"parse":       jsonParse("parse"),
			"parse_jsonc": jsonParse("parse_jsonc"),
			"stringify": {
				Name:       "stringify",
				Intrinsic:  "trb.internal.json.stringify",
				Parameters: []Parameter{{Name: "value", Type: jsonValueType}},
				Return:     jsonResult(stringType),
			},
		},
	},
	"trb/std/strings": {
		Path: "trb/std/strings",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"length":    unary("length", "trb.std.strings.length", stringType, integerType),
			"empty":     unary("empty", "trb.std.strings.empty", stringType, booleanType),
			"strip":     unary("strip", "trb.std.strings.strip", stringType, stringType),
			"lstrip":    unary("lstrip", "trb.std.strings.lstrip", stringType, stringType),
			"rstrip":    unary("rstrip", "trb.std.strings.rstrip", stringType, stringType),
			"uppercase": unary("uppercase", "trb.std.strings.uppercase", stringType, stringType),
			"lowercase": unary("lowercase", "trb.std.strings.lowercase", stringType, stringType),
			"starts_with": {
				Name:       "starts_with",
				Intrinsic:  "trb.std.strings.starts_with",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "prefix", Type: stringType}},
				Return:     booleanType,
			},
			"ends_with": {
				Name:       "ends_with",
				Intrinsic:  "trb.std.strings.ends_with",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "suffix", Type: stringType}},
				Return:     booleanType,
			},
			"split": {
				Name:       "split",
				Intrinsic:  "trb.std.strings.split",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "separator", Type: stringType}},
				Return:     arrayOf(stringType),
			},
			"codepoints": {
				Name:       "codepoints",
				Intrinsic:  "trb.std.strings.codepoints",
				Parameters: []Parameter{{Name: "value", Type: stringType}},
				Return:     types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{integerType}},
			},
			"contains": {
				Name:      "contains",
				Intrinsic: "trb.std.strings.contains",
				Parameters: []Parameter{
					{Name: "value", Type: stringType},
					{Name: "substring", Type: stringType},
				},
				Return: booleanType,
			},
		},
	},
	"trb/std/unicode": {
		Path:       "trb/std/unicode",
		ModulePath: "trb/std/unicode/index",
		Source:     unicodeSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"version":          {Name: "version", Intrinsic: "trb.std.unicode.version", Return: stringType},
			"valid_scalar":     unary("valid_scalar", "trb.std.unicode.valid_scalar", integerType, booleanType),
			"letter":           unary("letter", "trb.std.unicode.letter", integerType, booleanType),
			"digit":            unary("digit", "trb.std.unicode.digit", integerType, booleanType),
			"uppercase":        unary("uppercase", "trb.std.unicode.uppercase", integerType, booleanType),
			"lowercase":        unary("lowercase", "trb.std.unicode.lowercase", integerType, booleanType),
			"whitespace":       unary("whitespace", "trb.std.unicode.whitespace", integerType, booleanType),
			"identifier_start": unary("identifier_start", "trb.std.unicode.identifier_start", integerType, booleanType),
			"identifier_part":  unary("identifier_part", "trb.std.unicode.identifier_part", integerType, booleanType),
			"from_codepoint":   unary("from_codepoint", "trb.std.unicode.from_codepoint", integerType, stringType),
		},
	},
	"trb/std/bytes": {
		Path: "trb/std/bytes",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"from_string": unary("from_string", "trb.std.bytes.from_string", stringType, bytesType),
			"to_string":   unary("to_string", "trb.std.bytes.to_string", bytesType, stringType),
			"length":      unary("length", "trb.std.bytes.length", bytesType, integerType),
			"at": {
				Name:      "at",
				Intrinsic: "trb.std.bytes.at",
				Parameters: []Parameter{
					{Name: "value", Type: bytesType},
					{Name: "index", Type: integerType},
				},
				Return: integerType,
			},
			"concat": {
				Name:      "concat",
				Intrinsic: "trb.std.bytes.concat",
				Parameters: []Parameter{
					{Name: "left", Type: bytesType},
					{Name: "right", Type: bytesType},
				},
				Return: bytesType,
			},
			"valid_utf8": unary("valid_utf8", "trb.std.bytes.valid_utf8", bytesType, booleanType),
		},
	},
	"trb/std/encoding/hex": {
		Path:       "trb/std/encoding/hex",
		ModulePath: "trb/std/encoding/hex/index",
		RuntimeExports: []RuntimeExport{
			{Name: "HexDecodeErrorKind", Kind: "enum"},
			{Name: "HexDecodeError", Kind: "record"},
		},
		Source: hexSource(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"encode": unary("encode", "trb.std.encoding.hex.encode", bytesType, stringType),
			"decode": unary("decode", "trb.std.encoding.hex.decode", stringType, structuredErrorResult(bytesType, hexDecodeErrorType)),
		},
	},
	"trb/internal/encoding/hex": {
		Path:     "trb/internal/encoding/hex",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"encode": unary("encode", "trb.std.encoding.hex.encode", bytesType, stringType),
			"decode": unary("decode", "trb.std.encoding.hex.decode", stringType, structuredErrorResult(bytesType, hexDecodeErrorType)),
		},
	},
	"trb/std/encoding/base64": {
		Path:       "trb/std/encoding/base64",
		ModulePath: "trb/std/encoding/base64/index",
		RuntimeExports: []RuntimeExport{
			{Name: "Base64DecodeErrorKind", Kind: "enum"},
			{Name: "Base64DecodeError", Kind: "record"},
		},
		Source: base64Source(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"encode":     unary("encode", "trb.std.encoding.base64.encode", bytesType, stringType),
			"decode":     unary("decode", "trb.std.encoding.base64.decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
			"url_encode": unary("url_encode", "trb.std.encoding.base64.url_encode", bytesType, stringType),
			"url_decode": unary("url_decode", "trb.std.encoding.base64.url_decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
		},
	},
	"trb/internal/encoding/base64": {
		Path:     "trb/internal/encoding/base64",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"encode":     unary("encode", "trb.std.encoding.base64.encode", bytesType, stringType),
			"decode":     unary("decode", "trb.std.encoding.base64.decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
			"url_encode": unary("url_encode", "trb.std.encoding.base64.url_encode", bytesType, stringType),
			"url_decode": unary("url_decode", "trb.std.encoding.base64.url_decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
		},
	},
	"trb/std/hash": {
		Path:       "trb/std/hash",
		ModulePath: "trb/std/hash/index",
		Source:     hashSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"sha256": unary("sha256", "", bytesType, bytesType),
			"sha512": unary("sha512", "", bytesType, bytesType),
		},
	},
	"trb/internal/hash": {
		Path:     "trb/internal/hash",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"sha256": unary("sha256", "trb.std.hash.sha256", bytesType, bytesType),
			"sha512": unary("sha512", "trb.std.hash.sha512", bytesType, bytesType),
		},
	},
	"trb/std/string_builder": {
		Path: "trb/std/string_builder",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"new": {
				Name:      "new",
				Intrinsic: "trb.std.string_builder.new",
				Return:    stringBuilderType,
			},
			"from_string": unary("from_string", "trb.std.string_builder.from_string", stringType, stringBuilderType),
			"append": {
				Name:      "append",
				Intrinsic: "trb.std.string_builder.append",
				Parameters: []Parameter{
					{Name: "builder", Type: stringBuilderType, Mutable: true},
					{Name: "value", Type: stringType},
				},
				Return: voidType,
			},
			"append_codepoint": {
				Name:      "append_codepoint",
				Intrinsic: "trb.std.string_builder.append_codepoint",
				Parameters: []Parameter{
					{Name: "builder", Type: stringBuilderType, Mutable: true},
					{Name: "value", Type: integerType},
				},
				Return: voidType,
			},
			"length":    unary("length", "trb.std.string_builder.length", stringBuilderType, integerType),
			"empty":     unary("empty", "trb.std.string_builder.empty", stringBuilderType, booleanType),
			"to_string": unary("to_string", "trb.std.string_builder.to_string", stringBuilderType, stringType),
			"clear": {
				Name:       "clear",
				Intrinsic:  "trb.std.string_builder.clear",
				Parameters: []Parameter{{Name: "builder", Type: stringBuilderType, Mutable: true}},
				Return:     voidType,
			},
		},
	},
	"trb/std/arrays": {
		Path: "trb/std/arrays",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"length": genericUnary("length", "trb.std.arrays.length", []string{"T"}, arrayOf(typeT), integerType),
			"empty":  genericUnary("empty", "trb.std.arrays.empty", []string{"T"}, arrayOf(typeT), booleanType),
			"fetch": {
				Name:           "fetch",
				Intrinsic:      "trb.std.arrays.fetch",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "index", Type: integerType},
				},
				Return: typeT,
			},
			"try_fetch": {
				Name:           "try_fetch",
				Intrinsic:      "trb.std.arrays.try_fetch",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "index", Type: integerType},
				},
				Return: structuredErrorResult(typeT, indexLookupErrorType),
			},
			"first": genericUnary("first", "trb.std.arrays.first", []string{"T"}, arrayOf(typeT), typeT),
			"last":  genericUnary("last", "trb.std.arrays.last", []string{"T"}, arrayOf(typeT), typeT),
			"copy":  genericUnary("copy", "trb.std.arrays.copy", []string{"T"}, arrayOf(typeT), arrayOf(typeT)),
			"contains": {
				Name:           "contains",
				Intrinsic:      "trb.std.arrays.contains",
				TypeParameters: []string{"T"},
				EqualityTypes:  []types.Type{typeT},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "value", Type: typeT},
				},
				Return: booleanType,
			},
			"count": {
				Name:           "count",
				Intrinsic:      "trb.std.arrays.count",
				TypeParameters: []string{"T"},
				EqualityTypes:  []types.Type{typeT},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "value", Type: typeT},
				},
				Return: integerType,
			},
			"join": {
				Name:       "join",
				Intrinsic:  "trb.std.arrays.join",
				Parameters: []Parameter{{Name: "values", Type: arrayOf(stringType)}, {Name: "separator", Type: stringType}},
				Return:     stringType,
			},
			"pop": {
				Name:           "pop",
				Intrinsic:      "trb.std.arrays.pop",
				TypeParameters: []string{"T"},
				Parameters:     []Parameter{{Name: "values", Type: arrayOf(typeT), Mutable: true}},
				Return:         typeT,
			},
			"shift": {
				Name:           "shift",
				Intrinsic:      "trb.std.arrays.shift",
				TypeParameters: []string{"T"},
				Parameters:     []Parameter{{Name: "values", Type: arrayOf(typeT), Mutable: true}},
				Return:         typeT,
			},
			"push": {
				Name:           "push",
				Intrinsic:      "trb.std.arrays.push",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT), Mutable: true},
					{Name: "value", Type: typeT},
				},
				Return: voidType,
			},
			"unshift": {
				Name:           "unshift",
				Intrinsic:      "trb.std.arrays.unshift",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT), Mutable: true},
					{Name: "value", Type: typeT},
				},
				Return: voidType,
			},
			"reverse": genericUnary("reverse", "trb.std.arrays.reverse", []string{"T"}, arrayOf(typeT), arrayOf(typeT)),
		},
	},
	"trb/std/hashes": {
		Path: "trb/std/hashes",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"length": genericUnary("length", "trb.std.hashes.length", []string{"K", "V"}, hashOf(typeK, typeV), integerType),
			"empty":  genericUnary("empty", "trb.std.hashes.empty", []string{"K", "V"}, hashOf(typeK, typeV), booleanType),
			"fetch": {
				Name:           "fetch",
				Intrinsic:      "trb.std.hashes.fetch",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV)},
					{Name: "key", Type: typeK},
				},
				Return: typeV,
			},
			"try_fetch": {
				Name:           "try_fetch",
				Intrinsic:      "trb.std.hashes.try_fetch",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV)},
					{Name: "key", Type: typeK},
				},
				Return: structuredErrorResult(typeV, keyLookupErrorType),
			},
			"contains_key": {
				Name:           "contains_key",
				Intrinsic:      "trb.std.hashes.contains_key",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV)},
					{Name: "key", Type: typeK},
				},
				Return: booleanType,
			},
			"keys":   genericUnary("keys", "trb.std.hashes.keys", []string{"K", "V"}, hashOf(typeK, typeV), arrayOf(typeK)),
			"values": genericUnary("values", "trb.std.hashes.values", []string{"K", "V"}, hashOf(typeK, typeV), arrayOf(typeV)),
			"copy":   genericUnary("copy", "trb.std.hashes.copy", []string{"K", "V"}, hashOf(typeK, typeV), hashOf(typeK, typeV)),
			"delete": {
				Name:           "delete",
				Intrinsic:      "trb.std.hashes.delete",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV), Mutable: true, Exact: true},
					{Name: "key", Type: typeK},
				},
				Return: typeV,
			},
			"merge": {
				Name:           "merge",
				Intrinsic:      "trb.std.hashes.merge",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV), Exact: true},
					{Name: "other", Type: hashOf(typeK, typeV), Exact: true},
				},
				Return: hashOf(typeK, typeV),
			},
			"update": {
				Name:           "update",
				Intrinsic:      "trb.std.hashes.update",
				TypeParameters: []string{"K", "V"},
				Parameters: []Parameter{
					{Name: "values", Type: hashOf(typeK, typeV), Mutable: true, Exact: true},
					{Name: "other", Type: hashOf(typeK, typeV), Exact: true},
				},
				Return: voidType,
			},
		},
	},
	"trb/std/numbers": {
		Path: "trb/std/numbers",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"to_string":         unary("to_string", "trb.std.numbers.to_string", integerType, stringType),
			"to_float":          unary("to_float", "trb.std.numbers.integer_to_float", integerType, floatType),
			"absolute":          unary("absolute", "trb.std.numbers.integer_absolute", integerType, integerType),
			"min":               {Name: "min", Intrinsic: "trb.std.numbers.integer_min", Parameters: []Parameter{{Name: "value", Type: integerType}, {Name: "other", Type: integerType}}, Return: integerType},
			"max":               {Name: "max", Intrinsic: "trb.std.numbers.integer_max", Parameters: []Parameter{{Name: "value", Type: integerType}, {Name: "other", Type: integerType}}, Return: integerType},
			"clamp":             {Name: "clamp", Intrinsic: "trb.std.numbers.integer_clamp", Parameters: []Parameter{{Name: "value", Type: integerType}, {Name: "minimum", Type: integerType}, {Name: "maximum", Type: integerType}}, Return: integerType},
			"zero":              unary("zero", "trb.std.numbers.integer_zero", integerType, booleanType),
			"positive":          unary("positive", "trb.std.numbers.integer_positive", integerType, booleanType),
			"negative":          unary("negative", "trb.std.numbers.integer_negative", integerType, booleanType),
			"even":              unary("even", "trb.std.numbers.integer_even", integerType, booleanType),
			"odd":               unary("odd", "trb.std.numbers.integer_odd", integerType, booleanType),
			"float_to_string":   unary("float_to_string", "trb.std.numbers.float_to_string", floatType, stringType),
			"truncate":          unary("truncate", "trb.std.numbers.float_to_integer", floatType, integerType),
			"floor":             unary("floor", "trb.std.numbers.float_floor", floatType, integerType),
			"ceil":              unary("ceil", "trb.std.numbers.float_ceil", floatType, integerType),
			"round":             unary("round", "trb.std.numbers.float_round", floatType, integerType),
			"float_absolute":    unary("float_absolute", "trb.std.numbers.float_absolute", floatType, floatType),
			"finite":            unary("finite", "trb.std.numbers.float_finite", floatType, booleanType),
			"infinite":          unary("infinite", "trb.std.numbers.float_infinite", floatType, booleanType),
			"nan":               unary("nan", "trb.std.numbers.float_nan", floatType, booleanType),
			"parse_integer":     unary("parse_integer", "trb.std.numbers.parse_integer", stringType, integerType),
			"try_parse_integer": unary("try_parse_integer", "trb.std.numbers.try_parse_integer", stringType, structuredErrorResult(integerType, numberParseErrorType)),
		},
	},
	"trb/std/math": {
		Path: "trb/std/math",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"sqrt":  unary("sqrt", "trb.std.math.sqrt", floatType, floatType),
			"exp":   unary("exp", "trb.std.math.exp", floatType, floatType),
			"log":   unary("log", "trb.std.math.log", floatType, floatType),
			"log2":  unary("log2", "trb.std.math.log2", floatType, floatType),
			"log10": unary("log10", "trb.std.math.log10", floatType, floatType),
		},
	},
	"trb/std/booleans": {
		Path: "trb/std/booleans",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"to_string": unary("to_string", "trb.std.booleans.to_string", booleanType, stringType),
		},
	},
	"trb/platform/ruby/native": {
		Path:         "trb/platform/ruby/native",
		Kind:         Platform,
		Targets:      map[string]bool{"ruby": true},
		NativeSyntax: true,
		Symbols:      map[string]Symbol{},
	},
	"trb/platform/ruby/rails": {
		Path:         "trb/platform/ruby/rails",
		Kind:         Platform,
		Targets:      map[string]bool{"ruby": true},
		NativeSyntax: true,
		TypeProvider: "rails",
		Symbols:      map[string]Symbol{},
	},
	"trb/platform/go/context": {
		Path:    "trb/platform/go/context",
		Kind:    Platform,
		Targets: map[string]bool{"go": true},
		Symbols: map[string]Symbol{
			"background": {
				Name:      "background",
				Intrinsic: "trb.platform.go.context.background",
				Return:    types.FromName("Context"),
			},
			"todo": {
				Name:      "todo",
				Intrinsic: "trb.platform.go.context.todo",
				Return:    types.FromName("Context"),
			},
		},
	},
	"trb/platform/go/http": {
		Path: "trb/platform/go/http", Kind: Platform, Targets: map[string]bool{"go": true},
		Symbols: map[string]Symbol{
			"router": {Name: "router", Intrinsic: "trb.platform.go.http.router", Return: types.FromName("HTTPRouter")},
			"handle": {Name: "handle", Intrinsic: "trb.platform.go.http.handle", Parameters: []Parameter{{Name: "router", Type: types.FromName("HTTPRouter")}, {Name: "pattern", Type: stringType}, {Name: "handler", Type: types.FromName("Any")}}, Return: voidType},
			"path":   {Name: "path", Intrinsic: "trb.platform.go.http.path", Parameters: []Parameter{{Name: "request", Type: types.FromName("HTTPRequest")}, {Name: "name", Type: stringType}}, Return: stringType},
			"decode": {Name: "decode", Intrinsic: "trb.platform.go.http.decode", Parameters: []Parameter{{Name: "response", Type: types.FromName("HTTPResponse")}, {Name: "request", Type: types.FromName("HTTPRequest")}, {Name: "target", Type: types.FromName("Any")}}, Return: booleanType},
			"json":   {Name: "json", Intrinsic: "trb.platform.go.http.json", Parameters: []Parameter{{Name: "response", Type: types.FromName("HTTPResponse")}, {Name: "status", Type: integerType}, {Name: "value", Type: types.FromName("Any")}}, Return: voidType},
			"error":  {Name: "error", Intrinsic: "trb.platform.go.http.error", Parameters: []Parameter{{Name: "response", Type: types.FromName("HTTPResponse")}, {Name: "status", Type: integerType}, {Name: "message", Type: stringType}}, Return: voidType},
			"cors":   {Name: "cors", Intrinsic: "trb.platform.go.http.cors", Parameters: []Parameter{{Name: "router", Type: types.FromName("HTTPRouter")}, {Name: "origin", Type: stringType}}, Return: types.FromName("HTTPHandler")},
			"serve":  {Name: "serve", Intrinsic: "trb.platform.go.http.serve", Parameters: []Parameter{{Name: "address", Type: stringType}, {Name: "handler", Type: types.FromName("HTTPHandler")}}, Return: voidType},
		},
	},
	"trb/platform/go/gorm": {
		Path: "trb/platform/go/gorm", Kind: Platform, Targets: map[string]bool{"go": true},
		Symbols: map[string]Symbol{
			"open_sqlite": {Name: "open_sqlite", Intrinsic: "trb.platform.go.gorm.open_sqlite", Parameters: []Parameter{{Name: "path", Type: stringType}}, Return: types.FromName("GormDB")},
			"find_all":    {Name: "find_all", Intrinsic: "trb.platform.go.gorm.find_all", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "model", Type: types.FromName("Any")}}, Return: types.FromName("Any"), Inference: "array_of_argument_1"},
			"where":       {Name: "where", Intrinsic: "trb.platform.go.gorm.where", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "model", Type: types.FromName("Any")}, {Name: "query", Type: stringType}, {Name: "argument", Type: types.FromName("Any"), Optional: true}}, Return: types.FromName("Any"), Variadic: true, Inference: "array_of_argument_1"},
			"raw":         {Name: "raw", Intrinsic: "trb.platform.go.gorm.raw", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "model", Type: types.FromName("Any")}, {Name: "query", Type: stringType}, {Name: "argument", Type: types.FromName("Any"), Optional: true}}, Return: types.FromName("Any"), Variadic: true, Inference: "array_of_argument_1"},
			"first":       {Name: "first", Intrinsic: "trb.platform.go.gorm.first", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "model", Type: types.FromName("Any")}, {Name: "query", Type: stringType}, {Name: "argument", Type: types.FromName("Any"), Optional: true}}, Return: types.FromName("Any"), Variadic: true, Inference: "argument_1"},
			"create":      {Name: "create", Intrinsic: "trb.platform.go.gorm.create", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "value", Type: types.FromName("Any")}}, Return: types.FromName("Any"), Inference: "argument_1"},
			"save":        {Name: "save", Intrinsic: "trb.platform.go.gorm.save", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "value", Type: types.FromName("Any")}}, Return: types.FromName("Any"), Inference: "argument_1"},
			"exec":        {Name: "exec", Intrinsic: "trb.platform.go.gorm.exec", Parameters: []Parameter{{Name: "db", Type: types.FromName("GormDB")}, {Name: "query", Type: stringType}, {Name: "argument", Type: types.FromName("Any"), Optional: true}}, Return: voidType, Variadic: true},
		},
	},
	"trb/platform/typescript/node": {
		Path:    "trb/platform/typescript/node",
		Kind:    Platform,
		Targets: map[string]bool{"typescript": true},
		Symbols: map[string]Symbol{
			"argv": {
				Name:      "argv",
				Intrinsic: "trb.platform.typescript.node.argv",
				Return:    types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{stringType}},
			},
		},
	},
	"trb/platform/typescript/dom": {
		Path:    "trb/platform/typescript/dom",
		Kind:    Platform,
		Targets: map[string]bool{"typescript": true},
		Symbols: map[string]Symbol{},
	},
	"trb/platform/typescript/react": {
		Path: "trb/platform/typescript/react", Kind: Platform, Targets: map[string]bool{"typescript": true},
		Symbols: map[string]Symbol{
			"element":         {Name: "element", Intrinsic: "trb.platform.typescript.react.element", Parameters: []Parameter{{Name: "tag", Type: types.FromName("Any")}, {Name: "props", Type: types.FromName("Hash")}, {Name: "children", Type: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Any")}}}}, Return: types.FromName("ReactNode")},
			"mount":           {Name: "mount", Intrinsic: "trb.platform.typescript.react.mount", Parameters: []Parameter{{Name: "component", Type: types.FromName("Any")}, {Name: "element_id", Type: stringType}}, Return: voidType},
			"refresh":         {Name: "refresh", Intrinsic: "trb.platform.typescript.react.refresh", Parameters: []Parameter{{Name: "component", Type: types.FromName("Any")}}, Return: voidType},
			"prevent_default": {Name: "prevent_default", Intrinsic: "trb.platform.typescript.react.prevent_default", Parameters: []Parameter{{Name: "event", Type: types.FromName("ReactEvent")}}, Return: voidType},
			"input_value":     {Name: "input_value", Intrinsic: "trb.platform.typescript.react.input_value", Parameters: []Parameter{{Name: "event", Type: types.FromName("ReactEvent")}}, Return: stringType},
			"data_integer":    {Name: "data_integer", Intrinsic: "trb.platform.typescript.react.data_integer", Parameters: []Parameter{{Name: "event", Type: types.FromName("ReactEvent")}, {Name: "name", Type: stringType}}, Return: integerType},
			"data_boolean":    {Name: "data_boolean", Intrinsic: "trb.platform.typescript.react.data_boolean", Parameters: []Parameter{{Name: "event", Type: types.FromName("ReactEvent")}, {Name: "name", Type: stringType}}, Return: booleanType},
		},
	},
	"trb/platform/typescript/web": {
		Path: "trb/platform/typescript/web", Kind: Platform, Targets: map[string]bool{"typescript": true},
		Symbols: map[string]Symbol{
			"get_json":   {Name: "get_json", Intrinsic: "trb.platform.typescript.web.get_json", Parameters: []Parameter{{Name: "url", Type: stringType}, {Name: "callback", Type: types.FromName("Any")}}, Return: voidType},
			"post_json":  {Name: "post_json", Intrinsic: "trb.platform.typescript.web.post_json", Parameters: []Parameter{{Name: "url", Type: stringType}, {Name: "value", Type: types.FromName("Any")}, {Name: "callback", Type: types.FromName("Any")}}, Return: voidType},
			"patch_json": {Name: "patch_json", Intrinsic: "trb.platform.typescript.web.patch_json", Parameters: []Parameter{{Name: "url", Type: stringType}, {Name: "value", Type: types.FromName("Any")}, {Name: "callback", Type: types.FromName("Any")}}, Return: voidType},
		},
	},
}

// receiverMethods maps Ruby-like method syntax to the same compiler-owned
// contract used by the corresponding portable package function. A backend
// therefore lowers 123.to_s() and numbers.to_string(123) through one
// intrinsic instead of maintaining a second, target-specific method table.
var receiverMethods = map[types.Kind]map[string]receiverMethodTarget{
	types.Int: {
		"to_s":      {PackagePath: "trb/std/numbers", Symbol: "to_string"},
		"to_f":      {PackagePath: "trb/std/numbers", Symbol: "to_float"},
		"abs":       {PackagePath: "trb/std/numbers", Symbol: "absolute"},
		"min":       {PackagePath: "trb/std/numbers", Symbol: "min"},
		"max":       {PackagePath: "trb/std/numbers", Symbol: "max"},
		"clamp":     {PackagePath: "trb/std/numbers", Symbol: "clamp"},
		"zero?":     {PackagePath: "trb/std/numbers", Symbol: "zero"},
		"positive?": {PackagePath: "trb/std/numbers", Symbol: "positive"},
		"negative?": {PackagePath: "trb/std/numbers", Symbol: "negative"},
		"even?":     {PackagePath: "trb/std/numbers", Symbol: "even"},
		"odd?":      {PackagePath: "trb/std/numbers", Symbol: "odd"},
	},
	types.Float: {
		"to_s":      {PackagePath: "trb/std/numbers", Symbol: "float_to_string"},
		"to_i":      {PackagePath: "trb/std/numbers", Symbol: "truncate"},
		"truncate":  {PackagePath: "trb/std/numbers", Symbol: "truncate"},
		"floor":     {PackagePath: "trb/std/numbers", Symbol: "floor"},
		"ceil":      {PackagePath: "trb/std/numbers", Symbol: "ceil"},
		"round":     {PackagePath: "trb/std/numbers", Symbol: "round"},
		"abs":       {PackagePath: "trb/std/numbers", Symbol: "float_absolute"},
		"finite?":   {PackagePath: "trb/std/numbers", Symbol: "finite"},
		"infinite?": {PackagePath: "trb/std/numbers", Symbol: "infinite"},
		"nan?":      {PackagePath: "trb/std/numbers", Symbol: "nan"},
	},
	types.Bool: {
		"to_s": {PackagePath: "trb/std/booleans", Symbol: "to_string"},
	},
	types.String: {
		"to_i":        {PackagePath: "trb/std/numbers", Symbol: "parse_integer"},
		"try_to_i":    {PackagePath: "trb/std/numbers", Symbol: "try_parse_integer"},
		"size":        {PackagePath: "trb/std/strings", Symbol: "length"},
		"empty?":      {PackagePath: "trb/std/strings", Symbol: "empty"},
		"strip":       {PackagePath: "trb/std/strings", Symbol: "strip"},
		"lstrip":      {PackagePath: "trb/std/strings", Symbol: "lstrip"},
		"rstrip":      {PackagePath: "trb/std/strings", Symbol: "rstrip"},
		"upcase":      {PackagePath: "trb/std/strings", Symbol: "uppercase"},
		"downcase":    {PackagePath: "trb/std/strings", Symbol: "lowercase"},
		"include?":    {PackagePath: "trb/std/strings", Symbol: "contains"},
		"start_with?": {PackagePath: "trb/std/strings", Symbol: "starts_with"},
		"end_with?":   {PackagePath: "trb/std/strings", Symbol: "ends_with"},
		"split":       {PackagePath: "trb/std/strings", Symbol: "split"},
		"codepoints":  {PackagePath: "trb/std/strings", Symbol: "codepoints"},
		"to_bytes":    {PackagePath: "trb/std/bytes", Symbol: "from_string"},
	},
	types.Bytes: {
		"to_s":       {PackagePath: "trb/std/bytes", Symbol: "to_string"},
		"size":       {PackagePath: "trb/std/bytes", Symbol: "length"},
		"at":         {PackagePath: "trb/std/bytes", Symbol: "at"},
		"concat":     {PackagePath: "trb/std/bytes", Symbol: "concat"},
		"valid_utf8": {PackagePath: "trb/std/bytes", Symbol: "valid_utf8"},
	},
	types.StringBuilder: {
		"append":           {PackagePath: "trb/std/string_builder", Symbol: "append"},
		"append_codepoint": {PackagePath: "trb/std/string_builder", Symbol: "append_codepoint"},
		"size":             {PackagePath: "trb/std/string_builder", Symbol: "length"},
		"empty?":           {PackagePath: "trb/std/string_builder", Symbol: "empty"},
		"to_s":             {PackagePath: "trb/std/string_builder", Symbol: "to_string"},
		"clear":            {PackagePath: "trb/std/string_builder", Symbol: "clear"},
	},
	types.Array: {
		"size":      {PackagePath: "trb/std/arrays", Symbol: "length"},
		"empty?":    {PackagePath: "trb/std/arrays", Symbol: "empty"},
		"fetch":     {PackagePath: "trb/std/arrays", Symbol: "fetch"},
		"try_fetch": {PackagePath: "trb/std/arrays", Symbol: "try_fetch"},
		"first":     {PackagePath: "trb/std/arrays", Symbol: "first"},
		"last":      {PackagePath: "trb/std/arrays", Symbol: "last"},
		"dup":       {PackagePath: "trb/std/arrays", Symbol: "copy"},
		"include?":  {PackagePath: "trb/std/arrays", Symbol: "contains"},
		"count":     {PackagePath: "trb/std/arrays", Symbol: "count"},
		"join":      {PackagePath: "trb/std/arrays", Symbol: "join"},
		"pop":       {PackagePath: "trb/std/arrays", Symbol: "pop"},
		"shift":     {PackagePath: "trb/std/arrays", Symbol: "shift"},
		"push":      {PackagePath: "trb/std/arrays", Symbol: "push"},
		"unshift":   {PackagePath: "trb/std/arrays", Symbol: "unshift"},
		"reverse":   {PackagePath: "trb/std/arrays", Symbol: "reverse"},
	},
	types.Hash: {
		"size":      {PackagePath: "trb/std/hashes", Symbol: "length"},
		"empty?":    {PackagePath: "trb/std/hashes", Symbol: "empty"},
		"fetch":     {PackagePath: "trb/std/hashes", Symbol: "fetch"},
		"try_fetch": {PackagePath: "trb/std/hashes", Symbol: "try_fetch"},
		"key?":      {PackagePath: "trb/std/hashes", Symbol: "contains_key"},
		"keys":      {PackagePath: "trb/std/hashes", Symbol: "keys"},
		"values":    {PackagePath: "trb/std/hashes", Symbol: "values"},
		"dup":       {PackagePath: "trb/std/hashes", Symbol: "copy"},
		"delete":    {PackagePath: "trb/std/hashes", Symbol: "delete"},
		"merge":     {PackagePath: "trb/std/hashes", Symbol: "merge"},
		"update":    {PackagePath: "trb/std/hashes", Symbol: "update"},
	},
}

func unary(name, intrinsic string, parameter, result types.Type) Symbol {
	return Symbol{
		Name:       name,
		Intrinsic:  intrinsic,
		Parameters: []Parameter{{Name: "value", Type: parameter}},
		Return:     result,
	}
}

func genericUnary(name, intrinsic string, typeParameters []string, parameter, result types.Type) Symbol {
	symbol := unary(name, intrinsic, parameter, result)
	symbol.TypeParameters = typeParameters
	return symbol
}

func arrayOf(element types.Type) types.Type {
	return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{element}}
}

func hashOf(key, value types.Type) types.Type {
	return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{key, value}}
}

func filesystemResult(value types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{value, fileErrorType}}
}

func filesystemUnary(name string, result types.Type) Symbol {
	return Symbol{
		Name:       name,
		Intrinsic:  "trb.internal.filesystem." + name,
		Parameters: []Parameter{{Name: "path", Type: stringType}},
		Return:     filesystemResult(result),
	}
}

func filesystemWrite(name string, value types.Type) Symbol {
	return Symbol{
		Name:       name,
		Intrinsic:  "trb.internal.filesystem." + name,
		Parameters: []Parameter{{Name: "path", Type: stringType}, {Name: "value", Type: value}},
		Return:     filesystemResult(unitType),
	}
}

func jsonResult(value types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{value, jsonErrorType}}
}

func structuredErrorResult(value, failure types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{value, failure}}
}

func processResult(value types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{value, processErrorType}}
}

func nullable(value types.Type) types.Type {
	result := value
	result.Nullable = true
	return result
}

func jsonParse(name string) Symbol {
	result := Symbol{
		Name:       name,
		Intrinsic:  "trb.internal.json." + name,
		Parameters: []Parameter{{Name: "source", Type: stringType}},
		Return:     jsonResult(jsonValueType),
	}
	if name == "parse_jsonc" {
		result.RequiredSymbols = []string{"JsonErrorKind"}
	}
	return result
}

func Lookup(packagePath string) (*Package, bool) {
	definition, ok := registry[packagePath]
	return definition, ok
}

// LookupRuntimeExport returns the compiler-owned package that declares name.
// Inferred library result types may use these declarations internally, while
// source annotations still require an explicit import.
func LookupRuntimeExport(name string) (*Package, RuntimeExport, bool) {
	for _, definition := range registry {
		for _, exported := range definition.RuntimeExports {
			if exported.Name == name {
				return definition, exported, true
			}
		}
	}
	return nil, RuntimeExport{}, false
}

// RuntimeDependenciesForType returns compiler-owned modules whose runtime
// declarations are named by a library intrinsic's result type. Source code
// still needs an explicit import to refer to those declarations directly.
func RuntimeDependenciesForType(typ types.Type) []*Package {
	names := map[string]bool{}
	var collect func(types.Type)
	collect = func(current types.Type) {
		if current.Name != "" {
			names[current.Name] = true
		}
		for _, argument := range current.Args {
			collect(argument)
		}
	}
	collect(typ)

	dependencies := []*Package{}
	for _, definition := range registry {
		for _, exported := range definition.RuntimeExports {
			if names[exported.Name] {
				dependencies = append(dependencies, definition)
				break
			}
		}
	}
	return dependencies
}

func LookupReceiverMethod(receiver types.Type, name string) (*Package, Symbol, bool) {
	if receiver.Nullable {
		return nil, Symbol{}, false
	}
	target, ok := receiverMethods[receiver.Kind][name]
	if !ok {
		return nil, Symbol{}, false
	}
	definition, ok := Lookup(target.PackagePath)
	if !ok {
		return nil, Symbol{}, false
	}
	symbol, ok := definition.Symbols[target.Symbol]
	if !ok {
		return nil, Symbol{}, false
	}
	if len(symbol.Parameters) == 0 {
		return nil, Symbol{}, false
	}
	if !receiverMatches(symbol.Parameters[0].Type, receiver, symbol.TypeParameters) {
		return nil, Symbol{}, false
	}
	symbol = Instantiate(symbol, []types.Type{receiver})
	symbol.Name = name
	symbol.Receiver = receiver
	symbol.ReceiverMutable = symbol.Parameters[0].Mutable
	symbol.Parameters = append([]Parameter(nil), symbol.Parameters[1:]...)
	return definition, symbol, true
}

// ReceiverMethods returns the portable receiver methods available for a
// checked type. Language tooling uses the same contracts as the checker so
// completion cannot advertise target-native or otherwise invalid members.
func ReceiverMethods(receiver types.Type) []Symbol {
	targets := receiverMethods[receiver.Kind]
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	methods := make([]Symbol, 0, len(names))
	for _, name := range names {
		_, method, ok := LookupReceiverMethod(receiver, name)
		if ok {
			methods = append(methods, method)
		}
	}
	return methods
}

func receiverMatches(pattern, actual types.Type, typeParameterNames []string) bool {
	typeParameters := map[string]bool{}
	for _, name := range typeParameterNames {
		typeParameters[name] = true
	}
	var matches func(types.Type, types.Type) bool
	matches = func(expected, value types.Type) bool {
		if typeParameters[expected.Name] && expected.Kind == types.Named && len(expected.Args) == 0 {
			return true
		}
		if expected.Kind == types.Any {
			return true
		}
		if expected.Kind != value.Kind || expected.Nullable != value.Nullable {
			return false
		}
		if expected.Kind == types.Named && expected.Name != value.Name {
			return false
		}
		if len(expected.Args) != len(value.Args) {
			if len(value.Args) == 0 {
				for _, argument := range expected.Args {
					if !typeParameters[argument.Name] || argument.Kind != types.Named || len(argument.Args) != 0 {
						return false
					}
				}
				return true
			}
			return false
		}
		for index := range expected.Args {
			if !matches(expected.Args[index], value.Args[index]) {
				return false
			}
		}
		return true
	}
	return matches(pattern, actual)
}

// Instantiate substitutes compiler-owned type parameters inferred from call
// arguments. The first occurrence fixes a type variable; later occurrences
// are checked against that substitution by the ordinary checker.
func Instantiate(symbol Symbol, arguments []types.Type) Symbol {
	typeParameters := map[string]bool{}
	for _, name := range symbol.TypeParameters {
		typeParameters[name] = true
	}
	bindings := map[string]types.Type{}
	for index, actual := range arguments {
		parameterIndex := index
		if parameterIndex >= len(symbol.Parameters) {
			if !symbol.Variadic || len(symbol.Parameters) == 0 {
				break
			}
			parameterIndex = len(symbol.Parameters) - 1
		}
		bindType(symbol.Parameters[parameterIndex].Type, actual, typeParameters, bindings)
	}
	result := symbol
	result.Parameters = append([]Parameter(nil), symbol.Parameters...)
	for index := range result.Parameters {
		result.Parameters[index].Type = substituteType(result.Parameters[index].Type, bindings)
	}
	result.Return = substituteType(result.Return, bindings)
	result.EqualityTypes = append([]types.Type(nil), symbol.EqualityTypes...)
	for index := range result.EqualityTypes {
		result.EqualityTypes[index] = substituteType(result.EqualityTypes[index], bindings)
	}
	return result
}

func bindType(pattern, actual types.Type, typeParameters map[string]bool, bindings map[string]types.Type) {
	if typeParameters[pattern.Name] && pattern.Kind == types.Named && len(pattern.Args) == 0 {
		if _, exists := bindings[pattern.Name]; !exists && actual.Kind != types.Invalid {
			bindings[pattern.Name] = actual
		}
		return
	}
	if pattern.Kind != actual.Kind || len(pattern.Args) != len(actual.Args) {
		return
	}
	for index := range pattern.Args {
		bindType(pattern.Args[index], actual.Args[index], typeParameters, bindings)
	}
}

func substituteType(input types.Type, bindings map[string]types.Type) types.Type {
	if replacement, exists := bindings[input.Name]; exists && input.Kind == types.Named && len(input.Args) == 0 {
		replacement.Nullable = replacement.Nullable || input.Nullable
		replacement.Readonly = replacement.Readonly || input.Readonly
		return replacement
	}
	result := input
	result.Args = make([]types.Type, len(input.Args))
	for index, argument := range input.Args {
		result.Args[index] = substituteType(argument, bindings)
	}
	return result
}

func IsReservedPath(packagePath string) bool {
	return strings.HasPrefix(packagePath, "trb/")
}
