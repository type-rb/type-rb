// Package stdlib describes compiler-known portable and platform packages.
// Definitions are semantic contracts; backends lower their intrinsic IDs to
// target APIs without leaking those APIs into the TypeRB source language.
package stdlib

import (
	"path"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
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
	Keyword  bool
	Mutable  bool
	Exact    bool
}

// Block describes the callback accepted by a compiler-owned package
// function. Structured blocks keep resource and control-flow ownership in the
// compiler instead of lowering to an ordinary backend callback.
type Block struct {
	Parameters      []types.Type
	ControlBoundary bool
	Return          types.Type
	ResultBoundary  types.Type
	Structured      bool
	// ScopedParameters marks acquisition-owned resources. Source borrows and
	// immutable local aliases preserve that origin; storage, return, native
	// escape and retaining callbacks are forbidden by shared checking.
	ScopedParameters []bool
}

type Symbol struct {
	Name      string
	Intrinsic string
	// StaticOwner names the class or module that owns this compiler-known
	// operation. Unlike Receiver, it represents a static member and does not
	// pass the owner declaration as a runtime call argument.
	StaticOwner string
	// CompilerOnly keeps runner protocol operations out of user imports and
	// language-service completion while allowing compiler-owned source to use
	// the same checked package boundary.
	CompilerOnly bool
	// trustedScopedFileOrigin is an internal capability token copied only from
	// the bundled File.open registry symbol. Declaration and native providers
	// cannot manufacture it through their public protocols.
	trustedScopedFileOrigin bool
	// RuntimeIndependent marks an intrinsic that is fully lowered by every
	// backend even when its public package also provides a source wrapper.
	RuntimeIndependent  bool
	RequiredSymbols     []string
	TypeParameters      []string
	EqualityTypes       []types.Type
	OrderingTypes       []types.Type
	Receiver            types.Type
	ReceiverMutable     bool
	Parameters          []Parameter
	Return              types.Type
	Variadic            bool
	Inference           string
	RuntimeDependencies []types.Type
	Block               *Block
}

type RuntimeExport struct {
	Name string
	Kind string
}

// JSXProvider declares the node and intrinsic-attribute types contributed by
// an explicitly imported package. Grammar remains shared while each frontend
// runtime owns its JSX semantics.
type JSXProvider struct {
	Node                types.Type
	IntrinsicAttributes map[string]types.Type
}

func (s Symbol) HasReceiver() bool {
	return s.Receiver.Kind != "" && s.Receiver.Kind != types.Invalid
}

func (s Symbol) HasStaticOwner() bool {
	return s.StaticOwner != ""
}

func testAssertion(name string, expected bool) Symbol {
	parameters := []Parameter{{Name: "actual", Type: anyType}}
	if expected {
		parameters = append(parameters, Parameter{Name: "expected", Type: anyType})
	}
	parameters = append(parameters,
		Parameter{Name: "path", Type: stringType},
		Parameter{Name: "line", Type: integerType},
		Parameter{Name: "column", Type: integerType},
	)
	return Symbol{Name: name, Intrinsic: "trb.internal.test." + name, RuntimeIndependent: true, Parameters: parameters, Return: voidType}
}

type receiverMethodTarget struct {
	PackagePath string
	Symbol      string
}

type Package struct {
	Path       string
	ModulePath string
	// Root names the public declaration that owns this package's qualified
	// operations. Its members are backed by Symbols but are not themselves
	// top-level named exports.
	Root string
	// BuiltinRoot marks the narrow compiler-owned case where Root is the
	// canonical imported static API for an existing built-in type.
	BuiltinRoot    bool
	RuntimeAlias   string
	RuntimeExports []RuntimeExport
	Source         string
	Kind           Kind
	Internal       bool
	Targets        map[string]bool
	NativeSyntax   bool
	TypeProvider   string
	JSX            *JSXProvider
	Capability     bool
	// OpaqueTypes are compiler-owned declarations that source code may receive
	// or name but cannot construct directly. Each entry provides a
	// source-facing construction diagnostic.
	OpaqueTypes map[identity.Declaration]string
	Symbols     map[string]Symbol
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
var typeE = types.FromName("E")
var typeK = types.FromName("K")
var typeV = types.FromName("V")
var fileSystemErrorType = declaredType(fileSystemErrorDeclaration)
var fileType = FileResourceType()
var fileModeType = declaredType(fileModeDeclaration)
var dirEntryType = DirEntryType()
var jsonValueType = types.FromName("JSON::Value")
var jsonErrorType = types.FromName("JSON::Error")
var processResultType = types.FromName("Process::Output")
var processErrorType = types.FromName("Process::Error")
var numberParseErrorType = types.FromName("NumberParseError")
var hexDecodeErrorType = types.FromName("Hex::DecodeError")
var base64DecodeErrorType = types.FromName("Base64::DecodeError")
var indexLookupErrorType = types.FromName("IndexLookupError")
var sliceRangeErrorType = types.FromName("SliceRangeError")
var keyLookupErrorType = types.FromName("KeyLookupError")
var percentDecodeErrorType = types.FromName("URL::DecodeError")
var queryParameterType = types.FromName("URL::QueryParameter")
var dateType = types.FromName("Date")
var timeOfDayType = types.FromName("TimeOfDay")
var dateTimeType = types.FromName("DateTime")
var instantType = types.FromName("Instant")
var durationType = types.FromName("Duration")
var timeZoneType = types.FromName("TimeZone")
var dateTimeErrorType = types.FromName("DateTimeError")
var expectationTType = types.Type{Kind: types.Named, Name: "Expectation", Args: []types.Type{typeT}}
var resultTEType = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{typeT, typeE}}

var registry = map[string]*Package{
	"trb/std/unit": {
		Path:           "trb/std/unit",
		ModulePath:     "trb/std/unit/index",
		RuntimeExports: []RuntimeExport{{Name: "Unit", Kind: "record"}},
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
	"trb/std/test": {
		Path:         "trb/std/test",
		ModulePath:   "trb/std/test/index",
		RuntimeAlias: "__trb_test",
		RuntimeExports: []RuntimeExport{
			{Name: "Expectation", Kind: "class"},
			{Name: "ResultExpectation", Kind: "class"},
		},
		Source: `import { assert_equal, assert_false, assert_nil, assert_not_equal, assert_result_err, assert_result_ok, assert_true } from trb/internal/test
import trb/std/result

class Expectation<T>
	readonly @actual: T
	readonly @path: String
	readonly @line: Integer
	readonly @column: Integer

	def initialize(actual: T, path: String, line: Integer, column: Integer)
		@actual = actual
		@path = path
		@line = line
		@column = column
		return
	end

	def to_equal(expected: T)
		assert_equal(@actual, expected, @path, @line, @column)
		return
	end

	def to_not_equal(expected: T)
		assert_not_equal(@actual, expected, @path, @line, @column)
		return
	end

	def to_be_true()
		assert_true(@actual, @path, @line, @column)
		return
	end

	def to_be_false()
		assert_false(@actual, @path, @line, @column)
		return
	end

	def to_be_nil()
		assert_nil(@actual, @path, @line, @column)
		return
	end
end

class ResultExpectation<T, E>
	readonly @actual: Result<T, E>
	readonly @path: String
	readonly @line: Integer
	readonly @column: Integer

	def initialize(actual: Result<T, E>, path: String, line: Integer, column: Integer)
		@actual = actual
		@path = path
		@line = line
		@column = column
		return
	end

	def ok(): T
		return assert_result_ok<T, E>(@actual, @path, @line, @column)
	end

	def err(): E
		return assert_result_err<T, E>(@actual, @path, @line, @column)
	end
end
`,
		Kind: Portable,
		Symbols: map[string]Symbol{
			"describe": {Name: "describe", Intrinsic: "trb.std.test.describe", Parameters: []Parameter{{Name: "name", Type: stringType}}, Return: voidType, Block: &Block{ControlBoundary: true}},
			"test":     {Name: "test", Intrinsic: "trb.std.test.test", Parameters: []Parameter{{Name: "name", Type: stringType}}, Return: voidType, Block: &Block{ControlBoundary: true}},
			"expect": {
				Name: "expect", Intrinsic: "trb.std.test.expect", RequiredSymbols: []string{"Expectation"}, TypeParameters: []string{"T"},
				Parameters: []Parameter{{Name: "actual", Type: typeT}}, Return: expectationTType,
			},
			"expect_ok": {
				Name: "expect_ok", Intrinsic: "trb.std.test.expect_ok", RequiredSymbols: []string{"ResultExpectation"}, TypeParameters: []string{"T", "E"},
				Parameters: []Parameter{{Name: "actual", Type: resultTEType}}, Return: typeT,
			},
			"expect_err": {
				Name: "expect_err", Intrinsic: "trb.std.test.expect_err", RequiredSymbols: []string{"ResultExpectation"}, TypeParameters: []string{"T", "E"},
				Parameters: []Parameter{{Name: "actual", Type: resultTEType}}, Return: typeE,
			},
			"finish": {Name: "finish", Intrinsic: "trb.std.test.finish", CompilerOnly: true, Return: voidType},
		},
	},
	"trb/internal/test": {
		Path:     "trb/internal/test",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"assert_equal":     testAssertion("assert_equal", true),
			"assert_not_equal": testAssertion("assert_not_equal", true),
			"assert_true":      testAssertion("assert_true", false),
			"assert_false":     testAssertion("assert_false", false),
			"assert_nil":       testAssertion("assert_nil", false),
			"assert_result_ok": {
				Name: "assert_result_ok", Intrinsic: "trb.internal.test.assert_result_ok", RuntimeIndependent: true, TypeParameters: []string{"T", "E"},
				Parameters: []Parameter{{Name: "actual", Type: resultTEType}, {Name: "path", Type: stringType}, {Name: "line", Type: integerType}, {Name: "column", Type: integerType}}, Return: typeT,
			},
			"assert_result_err": {
				Name: "assert_result_err", Intrinsic: "trb.internal.test.assert_result_err", RuntimeIndependent: true, TypeParameters: []string{"T", "E"},
				Parameters: []Parameter{{Name: "actual", Type: resultTEType}, {Name: "path", Type: stringType}, {Name: "line", Type: integerType}, {Name: "column", Type: integerType}}, Return: typeE,
			},
		},
	},
	"trb/std/errors": {
		Path:         "trb/std/errors",
		ModulePath:   "trb/std/errors/index",
		RuntimeAlias: "__trb_errors",
		RuntimeExports: []RuntimeExport{
			{Name: "NumberParseErrorKind", Kind: "enum"},
			{Name: "NumberParseError", Kind: "record"},
			{Name: "IndexLookupError", Kind: "record"},
			{Name: "SliceRangeError", Kind: "record"},
			{Name: "KeyLookupError", Kind: "record"},
			{Name: "EnumValueError", Kind: "record"},
			{Name: "FileSystemErrorKind", Kind: "enum"},
			{Name: "FileSystemTarget", Kind: "enum"},
			{Name: "FileSystemError", Kind: "record"},
		},
		Source:  errorsSource(),
		Kind:    Portable,
		Symbols: map[string]Symbol{},
	},
	"trb/std/io": {
		Path: "trb/std/io", Root: "IO",
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
	"trb/std/time": {
		Path:       "trb/std/time",
		ModulePath: "trb/std/time/index",
		RuntimeExports: []RuntimeExport{
			{Name: "DateTimeErrorKind", Kind: "enum"},
			{Name: "DateTimeError", Kind: "record"},
			{Name: "Date", Kind: "class"},
			{Name: "TimeOfDay", Kind: "class"},
			{Name: "DateTime", Kind: "class"},
			{Name: "Instant", Kind: "class"},
			{Name: "Duration", Kind: "class"},
			{Name: "TimeZone", Kind: "class"},
		},
		Source:  timeSource(),
		Kind:    Portable,
		Symbols: map[string]Symbol{},
	},
	"trb/internal/time": {
		Path: "trb/internal/time", Root: "Time",
		Kind:     Portable,
		Internal: true,
		Symbols:  timeIntrinsicSymbols(),
	},
	"trb/internal/runtime": {
		Path: "trb/internal/runtime", Root: "Runtime",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"fail": {
				Name:               "fail",
				Intrinsic:          "trb.internal.runtime.fail",
				RuntimeIndependent: true,
				TypeParameters:     []string{"T"},
				Parameters:         []Parameter{{Name: "message", Type: stringType}},
				Return:             typeT,
			},
		},
	},
	"trb/internal/jobs/sql": {
		Path: "trb/internal/jobs/sql", Root: "SQL",
		Kind:     Portable,
		Internal: true,
		Symbols:  jobsSQLIntrinsicSymbols(),
	},
	"trb/internal/auth/oidc": {
		Path: "trb/internal/auth/oidc", Root: "OIDC",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"verify_bearer": {
				Name:               "verify_bearer",
				Intrinsic:          "trb.internal.auth.oidc.verify_bearer",
				RuntimeIndependent: true,
				Parameters: []Parameter{
					{Name: "request", Type: types.FromName("Request")},
					{Name: "options", Type: types.FromName("OidcBearerOptions")},
				},
				Return: types.Type{
					Kind: types.Named,
					Name: "Result",
					Args: []types.Type{types.FromName("OidcPrincipal"), types.FromName("OidcAuthError")},
				},
			},
		},
	},
	"trb/internal/web/logger": {
		Path: "trb/internal/web/logger", Root: "Logger",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"call": {
				Name:      "call",
				Intrinsic: "trb.web.middleware.logger.call",
				Parameters: []Parameter{
					{Name: "context", Type: types.FromName("Context")},
					{Name: "next_handler", Type: types.FromName("Next")},
					{Name: "options", Type: types.FromName("LoggerOptions")},
				},
				Return: types.FromName("Response"),
			},
		},
	},
	"trb/internal/web/compression": {
		Path: "trb/internal/web/compression", Root: "Compression",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"gzip": {
				Name:               "gzip",
				Intrinsic:          "trb.web.middleware.compression.gzip",
				RuntimeIndependent: true,
				Parameters:         []Parameter{{Name: "value", Type: bytesType}},
				Return:             bytesType,
			},
		},
	},
	"trb/internal/web/timeout": {
		Path: "trb/internal/web/timeout", Root: "Timeout",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"call": {
				Name:      "call",
				Intrinsic: "trb.web.middleware.timeout.call",
				Parameters: []Parameter{
					{Name: "context", Type: types.FromName("Context")},
					{Name: "next_handler", Type: types.FromName("Next")},
					{Name: "milliseconds", Type: integerType},
					{Name: "timeout_response", Type: types.FromName("Response")},
				},
				Return: types.FromName("Response"),
			},
		},
	},
	"trb/std/url": {
		Path:       "trb/std/url",
		Root:       "URL",
		ModulePath: "trb/std/url/index",
		RuntimeExports: []RuntimeExport{
			{Name: "URL::DecodeErrorKind", Kind: "enum"},
			{Name: "URL::DecodeError", Kind: "record"},
			{Name: "URL::QueryParameter", Kind: "record"},
		},
		Source: urlSource(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"encode_component": runtimeIndependent(unary("encode_component", "trb.std.url.encode_component", stringType, stringType)),
			"decode_component": runtimeIndependent(unary("decode_component", "trb.std.url.decode_component", stringType, structuredErrorResult(stringType, percentDecodeErrorType))),
			"parse_query":      unary("parse_query", "", stringType, structuredErrorResult(arrayOf(queryParameterType), percentDecodeErrorType)),
			"build_query":      unary("build_query", "", arrayOf(queryParameterType), stringType),
		},
	},
	"trb/internal/url": {
		Path: "trb/internal/url", Root: "URL",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"encode_component": unary("encode_component", "trb.std.url.encode_component", stringType, stringType),
			"decode_component": unary("decode_component", "trb.std.url.decode_component", stringType, structuredErrorResult(stringType, percentDecodeErrorType)),
		},
	},
	"trb/std/path": {
		Path: "trb/std/path", Root: "Path", ModulePath: pathModulePath,
		RuntimeAlias: "__trb_path",
		RuntimeExports: []RuntimeExport{
			{Name: "Path", Kind: "newtype"},
			{Name: "RelativePath", Kind: "newtype"},
			{Name: "RelativePathError", Kind: "enum"},
		},
		Source: pathSource(), Kind: Portable,
		Symbols: map[string]Symbol{"join": pathJoinSymbol()},
	},
	"trb/std/file": {
		Path:       "trb/std/file",
		Root:       "File",
		ModulePath: fileModulePath,
		RuntimeExports: []RuntimeExport{
			{Name: "File", Kind: "class"},
			{Name: "FileMode", Kind: "enum"},
		},
		Source: fileSource(),
		Kind:   Portable,
		OpaqueTypes: map[identity.Declaration]string{
			fileDeclaration: "File cannot be constructed directly; use File.open() with a block",
		},
		Symbols: map[string]Symbol{
			"open": {
				Name:                    "open",
				Intrinsic:               "trb.std.file.open",
				StaticOwner:             "File",
				TypeParameters:          []string{"T"},
				trustedScopedFileOrigin: true,
				Parameters: []Parameter{
					{Name: "path", Type: PathType()},
					{Name: "mode", Type: fileModeType, Optional: true, Keyword: true},
				},
				Return:              filesystemResult(typeT),
				RuntimeDependencies: []types.Type{fileModeType},
				Block: &Block{
					Parameters:       []types.Type{fileType},
					Return:           typeT,
					ResultBoundary:   fileSystemErrorType,
					Structured:       true,
					ScopedParameters: []bool{true},
				},
			},
			"read":       fileRead("read", bytesType),
			"read_text":  fileRead("read_text", stringType),
			"write":      fileWrite("write", bytesType),
			"write_text": fileWrite("write_text", stringType),
		},
	},
	"trb/std/dir": {
		Path:       "trb/std/dir",
		Root:       "Dir",
		ModulePath: dirModulePath,
		RuntimeExports: []RuntimeExport{
			{Name: "Dir", Kind: "class"},
			{Name: "DirEntryKind", Kind: "enum"},
			{Name: "DirEntry", Kind: "record"},
		},
		Source: dirSource(),
		Kind:   Portable,
		OpaqueTypes: map[identity.Declaration]string{
			dirDeclaration: "Dir cannot be constructed directly; use Dir.children()",
		},
		Symbols: map[string]Symbol{
			"children": {
				Name:        "children",
				Intrinsic:   "trb.std.dir.children",
				StaticOwner: "Dir",
				Parameters:  []Parameter{{Name: "path", Type: PathType()}, {Name: "max_entries", Type: integerType, Keyword: true}},
				Return:      filesystemResult(arrayOf(dirEntryType)),
			},
			"create_all": {
				Name: "create_all", Intrinsic: "trb.std.dir.create_all", StaticOwner: "Dir",
				Parameters: []Parameter{{Name: "path", Type: PathType()}},
				Return:     filesystemResult(unitType),
			},
		},
	},
	"trb/std/process": {
		Path: "trb/std/process", Root: "Process",
		ModulePath: "trb/std/process/index",
		Source:     processSource(),
		Kind:       Portable,
		Symbols:    map[string]Symbol{},
	},
	"trb/internal/process": {
		Path: "trb/internal/process", Root: "Process",
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
	"trb/std/json": {
		Path:       "trb/std/json",
		Root:       "JSON",
		ModulePath: "trb/std/json/index",
		RuntimeExports: []RuntimeExport{
			{Name: "JSON::ErrorKind", Kind: "enum"},
			{Name: "JSON::Error", Kind: "record"},
			{Name: "JSON::Value", Kind: "enum"},
		},
		Source: jsonSource(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"parse": jsonParse("parse"),
			"decode": {
				Name:            "decode",
				Intrinsic:       "trb.internal.json.decode",
				RequiredSymbols: []string{"JSON"},
				TypeParameters:  []string{"T"},
				Parameters:      []Parameter{{Name: "source", Type: stringType}},
				Return:          jsonResult(typeT),
			},
			"encode": {
				Name:            "encode",
				Intrinsic:       "trb.internal.json.encode",
				RequiredSymbols: []string{"JSON"},
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
		Root:       "JSONC",
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
		Path: "trb/internal/json", Root: "JSON",
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
	"trb/internal/strings": {
		Path:     "trb/internal/strings",
		Kind:     Portable,
		Internal: true,
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
			"characters": {
				Name:       "characters",
				Intrinsic:  "trb.std.strings.characters",
				Parameters: []Parameter{{Name: "value", Type: stringType}},
				Return:     arrayOf(stringType),
			},
			"reverse": unary("reverse", "trb.std.strings.reverse", stringType, stringType),
			"try_fetch": {
				Name:       "try_fetch",
				Intrinsic:  "trb.std.strings.try_fetch",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "index", Type: integerType}},
				Return:     structuredErrorResult(stringType, indexLookupErrorType),
			},
			"slice": {
				Name:       "slice",
				Intrinsic:  "trb.std.strings.slice",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "range", Type: rangeOf(integerType)}},
				Return:     stringType,
			},
			"try_slice": {
				Name:       "try_slice",
				Intrinsic:  "trb.std.strings.try_slice",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "range", Type: rangeOf(integerType)}},
				Return:     structuredErrorResult(stringType, sliceRangeErrorType),
			},
			"index": {
				Name:       "index",
				Intrinsic:  "trb.std.strings.index",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "substring", Type: stringType}},
				Return:     nullable(integerType),
			},
			"rindex": {
				Name:       "rindex",
				Intrinsic:  "trb.std.strings.rindex",
				Parameters: []Parameter{{Name: "value", Type: stringType}, {Name: "substring", Type: stringType}},
				Return:     nullable(integerType),
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
			"replace_all": {
				Name:      "replace_all",
				Intrinsic: "trb.std.strings.replace_all",
				Parameters: []Parameter{
					{Name: "value", Type: stringType},
					{Name: "pattern", Type: stringType},
					{Name: "replacement", Type: stringType},
				},
				Return: stringType,
			},
		},
	},
	"trb/std/unicode": {
		Path:       "trb/std/unicode",
		Root:       "Unicode",
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
	"trb/internal/bytes": {
		Path:     "trb/internal/bytes",
		Kind:     Portable,
		Internal: true,
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
		Root:       "Hex",
		ModulePath: "trb/std/encoding/hex/index",
		RuntimeExports: []RuntimeExport{
			{Name: "Hex::DecodeErrorKind", Kind: "enum"},
			{Name: "Hex::DecodeError", Kind: "record"},
		},
		Source: hexSource(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"encode": runtimeIndependent(unary("encode", "trb.std.encoding.hex.encode", bytesType, stringType)),
			"decode": runtimeIndependent(unary("decode", "trb.std.encoding.hex.decode", stringType, structuredErrorResult(bytesType, hexDecodeErrorType))),
		},
	},
	"trb/internal/encoding/hex": {
		Path: "trb/internal/encoding/hex", Root: "Hex",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"encode": unary("encode", "trb.std.encoding.hex.encode", bytesType, stringType),
			"decode": unary("decode", "trb.std.encoding.hex.decode", stringType, structuredErrorResult(bytesType, hexDecodeErrorType)),
		},
	},
	"trb/std/encoding/base64": {
		Path:       "trb/std/encoding/base64",
		Root:       "Base64",
		ModulePath: "trb/std/encoding/base64/index",
		RuntimeExports: []RuntimeExport{
			{Name: "Base64::DecodeErrorKind", Kind: "enum"},
			{Name: "Base64::DecodeError", Kind: "record"},
		},
		Source: base64Source(),
		Kind:   Portable,
		Symbols: map[string]Symbol{
			"encode":     runtimeIndependent(unary("encode", "trb.std.encoding.base64.encode", bytesType, stringType)),
			"decode":     runtimeIndependent(unary("decode", "trb.std.encoding.base64.decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType))),
			"url_encode": runtimeIndependent(unary("url_encode", "trb.std.encoding.base64.url_encode", bytesType, stringType)),
			"url_decode": runtimeIndependent(unary("url_decode", "trb.std.encoding.base64.url_decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType))),
		},
	},
	"trb/internal/encoding/base64": {
		Path: "trb/internal/encoding/base64", Root: "Base64",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"encode":     unary("encode", "trb.std.encoding.base64.encode", bytesType, stringType),
			"decode":     unary("decode", "trb.std.encoding.base64.decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
			"url_encode": unary("url_encode", "trb.std.encoding.base64.url_encode", bytesType, stringType),
			"url_decode": unary("url_decode", "trb.std.encoding.base64.url_decode", stringType, structuredErrorResult(bytesType, base64DecodeErrorType)),
		},
	},
	"trb/std/digest": {
		Path:       "trb/std/digest",
		Root:       "Digest",
		ModulePath: "trb/std/digest/index",
		Source:     hashSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"md5":    unary("md5", "", bytesType, bytesType),
			"sha1":   unary("sha1", "", bytesType, bytesType),
			"sha256": unary("sha256", "", bytesType, bytesType),
			"sha512": unary("sha512", "", bytesType, bytesType),
		},
	},
	"trb/internal/hash": {
		Path: "trb/internal/hash", Root: "Hash",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"md5":    unary("md5", "trb.std.hash.md5", bytesType, bytesType),
			"sha1":   unary("sha1", "trb.std.hash.sha1", bytesType, bytesType),
			"sha256": unary("sha256", "trb.std.hash.sha256", bytesType, bytesType),
			"sha512": unary("sha512", "trb.std.hash.sha512", bytesType, bytesType),
		},
	},
	"trb/std/hmac": {
		Path:       "trb/std/hmac",
		Root:       "HMAC",
		ModulePath: "trb/std/hmac/index",
		Source:     hmacSource(),
		Kind:       Portable,
		Symbols: map[string]Symbol{
			"sha256": bytesBinary("sha256", "", bytesType),
			"sha512": bytesBinary("sha512", "", bytesType),
			"equal":  bytesBinary("equal", "", booleanType),
		},
	},
	"trb/internal/hmac": {
		Path: "trb/internal/hmac", Root: "HMAC",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"sha256": bytesBinary("sha256", "trb.std.hmac.sha256", bytesType),
			"sha512": bytesBinary("sha512", "trb.std.hmac.sha512", bytesType),
			"equal":  bytesBinary("equal", "trb.std.hmac.equal", booleanType),
		},
	},
	"trb/std/secure_compare": {
		Path: "trb/std/secure_compare", Root: "SecureCompare",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"equal": bytesBinary("equal", "trb.std.secure_compare.equal", booleanType),
		},
	},
	"trb/std/random": {
		Path: "trb/std/random", Root: "Random",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"float":   {Name: "float", Intrinsic: "trb.std.random.float", Return: floatType},
			"integer": unary("integer", "trb.std.random.integer", integerType, integerType),
		},
	},
	"trb/std/secure_random": {
		Path: "trb/std/secure_random", Root: "SecureRandom",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"bytes": unary("bytes", "trb.std.secure_random.bytes", integerType, bytesType),
		},
	},
	"trb/std/string_builder": {
		Path: "trb/std/string_builder", Root: "StringBuilder", BuiltinRoot: true,
		Kind: Portable,
		Symbols: map[string]Symbol{
			"new": {
				Name:      "new",
				Intrinsic: "trb.std.string_builder.new",
				Return:    stringBuilderType,
			},
			"from_string": unary("from_string", "trb.std.string_builder.from_string", stringType, stringBuilderType),
		},
	},
	"trb/internal/string_builder": {
		Path:     "trb/internal/string_builder",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
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
	"trb/internal/arrays": {
		Path:     "trb/internal/arrays",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"length": genericUnary("length", "trb.std.arrays.length", []string{"T"}, arrayOf(typeT), integerType),
			"empty":  genericUnary("empty", "trb.std.arrays.empty", []string{"T"}, arrayOf(typeT), booleanType),
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
			"slice": {
				Name:           "slice",
				Intrinsic:      "trb.std.arrays.slice",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "range", Type: rangeOf(integerType)},
				},
				Return: arrayOf(typeT),
			},
			"try_slice": {
				Name:           "try_slice",
				Intrinsic:      "trb.std.arrays.try_slice",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "range", Type: rangeOf(integerType)},
				},
				Return: structuredErrorResult(arrayOf(typeT), sliceRangeErrorType),
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
			"index": {
				Name:           "index",
				Intrinsic:      "trb.std.arrays.index",
				TypeParameters: []string{"T"},
				EqualityTypes:  []types.Type{typeT},
				Parameters: []Parameter{
					{Name: "values", Type: arrayOf(typeT)},
					{Name: "value", Type: typeT},
				},
				Return: nullable(integerType),
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
			"uniq": {
				Name:           "uniq",
				Intrinsic:      "trb.std.arrays.uniq",
				TypeParameters: []string{"T"},
				EqualityTypes:  []types.Type{typeT},
				Parameters:     []Parameter{{Name: "values", Type: arrayOf(typeT)}},
				Return:         arrayOf(typeT),
			},
			"concat": {
				Name:           "concat",
				Intrinsic:      "trb.std.arrays.concat",
				TypeParameters: []string{"T"},
				Parameters: []Parameter{
					{Name: "left", Type: arrayOf(typeT)},
					{Name: "right", Type: arrayOf(typeT)},
				},
				Return: arrayOf(typeT),
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
			"sort": {
				Name:           "sort",
				Intrinsic:      "trb.std.arrays.sort",
				TypeParameters: []string{"T"},
				OrderingTypes:  []types.Type{typeT},
				Parameters:     []Parameter{{Name: "values", Type: arrayOf(typeT)}},
				Return:         arrayOf(typeT),
			},
			"sort_descending": {
				Name:           "sort_descending",
				Intrinsic:      "trb.std.arrays.sort_descending",
				TypeParameters: []string{"T"},
				OrderingTypes:  []types.Type{typeT},
				Parameters:     []Parameter{{Name: "values", Type: arrayOf(typeT)}},
				Return:         arrayOf(typeT),
			},
		},
	},
	"trb/internal/ranges": {
		Path:     "trb/internal/ranges",
		Kind:     Portable,
		Internal: true,
		Symbols: map[string]Symbol{
			"to_array": unary("to_array", "trb.std.ranges.to_array", rangeOf(integerType), arrayOf(integerType)),
		},
	},
	"trb/internal/hashes": {
		Path:     "trb/internal/hashes",
		Kind:     Portable,
		Internal: true,
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
	"trb/internal/numbers": {
		Path:     "trb/internal/numbers",
		Kind:     Portable,
		Internal: true,
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
			"parse_float":       unary("parse_float", "trb.std.numbers.parse_float", stringType, floatType),
			"try_parse_float":   unary("try_parse_float", "trb.std.numbers.try_parse_float", stringType, structuredErrorResult(floatType, numberParseErrorType)),
		},
	},
	"trb/std/math": {
		Path: "trb/std/math", Root: "Math",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"sqrt":  unary("sqrt", "trb.std.math.sqrt", floatType, floatType),
			"exp":   unary("exp", "trb.std.math.exp", floatType, floatType),
			"log":   unary("log", "trb.std.math.log", floatType, floatType),
			"log2":  unary("log2", "trb.std.math.log2", floatType, floatType),
			"log10": unary("log10", "trb.std.math.log10", floatType, floatType),
		},
	},
	"trb/internal/booleans": {
		Path:     "trb/internal/booleans",
		Kind:     Portable,
		Internal: true,
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
		Path: "trb/platform/go/context", Root: "Context",
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
		Path: "trb/platform/go/http", Root: "HTTP", Kind: Platform, Targets: map[string]bool{"go": true},
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
		Path: "trb/platform/typescript/node", Root: "Node",
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
}

// receiverMethods maps receiver method syntax to compiler-owned contracts. A
// backend therefore lowers every receiver spelling through the same intrinsic
// catalog instead of maintaining a second, target-specific method table.
var receiverMethods = map[types.Kind]map[string]receiverMethodTarget{
	types.Int: {
		"to_s":      {PackagePath: "trb/internal/numbers", Symbol: "to_string"},
		"to_f":      {PackagePath: "trb/internal/numbers", Symbol: "to_float"},
		"abs":       {PackagePath: "trb/internal/numbers", Symbol: "absolute"},
		"min":       {PackagePath: "trb/internal/numbers", Symbol: "min"},
		"max":       {PackagePath: "trb/internal/numbers", Symbol: "max"},
		"clamp":     {PackagePath: "trb/internal/numbers", Symbol: "clamp"},
		"zero?":     {PackagePath: "trb/internal/numbers", Symbol: "zero"},
		"positive?": {PackagePath: "trb/internal/numbers", Symbol: "positive"},
		"negative?": {PackagePath: "trb/internal/numbers", Symbol: "negative"},
		"even?":     {PackagePath: "trb/internal/numbers", Symbol: "even"},
		"odd?":      {PackagePath: "trb/internal/numbers", Symbol: "odd"},
	},
	types.Float: {
		"to_s":      {PackagePath: "trb/internal/numbers", Symbol: "float_to_string"},
		"to_i":      {PackagePath: "trb/internal/numbers", Symbol: "truncate"},
		"floor":     {PackagePath: "trb/internal/numbers", Symbol: "floor"},
		"ceil":      {PackagePath: "trb/internal/numbers", Symbol: "ceil"},
		"round":     {PackagePath: "trb/internal/numbers", Symbol: "round"},
		"abs":       {PackagePath: "trb/internal/numbers", Symbol: "float_absolute"},
		"finite?":   {PackagePath: "trb/internal/numbers", Symbol: "finite"},
		"infinite?": {PackagePath: "trb/internal/numbers", Symbol: "infinite"},
		"nan?":      {PackagePath: "trb/internal/numbers", Symbol: "nan"},
	},
	types.Bool: {
		"to_s": {PackagePath: "trb/internal/booleans", Symbol: "to_string"},
	},
	types.String: {
		"to_i":        {PackagePath: "trb/internal/numbers", Symbol: "parse_integer"},
		"try_to_i":    {PackagePath: "trb/internal/numbers", Symbol: "try_parse_integer"},
		"to_f":        {PackagePath: "trb/internal/numbers", Symbol: "parse_float"},
		"try_to_f":    {PackagePath: "trb/internal/numbers", Symbol: "try_parse_float"},
		"size":        {PackagePath: "trb/internal/strings", Symbol: "length"},
		"empty?":      {PackagePath: "trb/internal/strings", Symbol: "empty"},
		"strip":       {PackagePath: "trb/internal/strings", Symbol: "strip"},
		"lstrip":      {PackagePath: "trb/internal/strings", Symbol: "lstrip"},
		"rstrip":      {PackagePath: "trb/internal/strings", Symbol: "rstrip"},
		"upcase":      {PackagePath: "trb/internal/strings", Symbol: "uppercase"},
		"downcase":    {PackagePath: "trb/internal/strings", Symbol: "lowercase"},
		"include?":    {PackagePath: "trb/internal/strings", Symbol: "contains"},
		"start_with?": {PackagePath: "trb/internal/strings", Symbol: "starts_with"},
		"end_with?":   {PackagePath: "trb/internal/strings", Symbol: "ends_with"},
		"split":       {PackagePath: "trb/internal/strings", Symbol: "split"},
		"codepoints":  {PackagePath: "trb/internal/strings", Symbol: "codepoints"},
		"chars":       {PackagePath: "trb/internal/strings", Symbol: "characters"},
		"reverse":     {PackagePath: "trb/internal/strings", Symbol: "reverse"},
		"replace_all": {PackagePath: "trb/internal/strings", Symbol: "replace_all"},
		"try_fetch":   {PackagePath: "trb/internal/strings", Symbol: "try_fetch"},
		"slice":       {PackagePath: "trb/internal/strings", Symbol: "slice"},
		"try_slice":   {PackagePath: "trb/internal/strings", Symbol: "try_slice"},
		"index":       {PackagePath: "trb/internal/strings", Symbol: "index"},
		"rindex":      {PackagePath: "trb/internal/strings", Symbol: "rindex"},
		"to_bytes":    {PackagePath: "trb/internal/bytes", Symbol: "from_string"},
	},
	types.Bytes: {
		"to_s":        {PackagePath: "trb/internal/bytes", Symbol: "to_string"},
		"size":        {PackagePath: "trb/internal/bytes", Symbol: "length"},
		"at":          {PackagePath: "trb/internal/bytes", Symbol: "at"},
		"concat":      {PackagePath: "trb/internal/bytes", Symbol: "concat"},
		"valid_utf8?": {PackagePath: "trb/internal/bytes", Symbol: "valid_utf8"},
	},
	types.StringBuilder: {
		"append":           {PackagePath: "trb/internal/string_builder", Symbol: "append"},
		"append_codepoint": {PackagePath: "trb/internal/string_builder", Symbol: "append_codepoint"},
		"size":             {PackagePath: "trb/internal/string_builder", Symbol: "length"},
		"empty?":           {PackagePath: "trb/internal/string_builder", Symbol: "empty"},
		"to_s":             {PackagePath: "trb/internal/string_builder", Symbol: "to_string"},
		"clear":            {PackagePath: "trb/internal/string_builder", Symbol: "clear"},
	},
	types.Array: {
		"size":            {PackagePath: "trb/internal/arrays", Symbol: "length"},
		"empty?":          {PackagePath: "trb/internal/arrays", Symbol: "empty"},
		"try_fetch":       {PackagePath: "trb/internal/arrays", Symbol: "try_fetch"},
		"slice":           {PackagePath: "trb/internal/arrays", Symbol: "slice"},
		"try_slice":       {PackagePath: "trb/internal/arrays", Symbol: "try_slice"},
		"first":           {PackagePath: "trb/internal/arrays", Symbol: "first"},
		"last":            {PackagePath: "trb/internal/arrays", Symbol: "last"},
		"dup":             {PackagePath: "trb/internal/arrays", Symbol: "copy"},
		"include?":        {PackagePath: "trb/internal/arrays", Symbol: "contains"},
		"index":           {PackagePath: "trb/internal/arrays", Symbol: "index"},
		"count":           {PackagePath: "trb/internal/arrays", Symbol: "count"},
		"uniq":            {PackagePath: "trb/internal/arrays", Symbol: "uniq"},
		"concat":          {PackagePath: "trb/internal/arrays", Symbol: "concat"},
		"join":            {PackagePath: "trb/internal/arrays", Symbol: "join"},
		"pop":             {PackagePath: "trb/internal/arrays", Symbol: "pop"},
		"shift":           {PackagePath: "trb/internal/arrays", Symbol: "shift"},
		"push":            {PackagePath: "trb/internal/arrays", Symbol: "push"},
		"unshift":         {PackagePath: "trb/internal/arrays", Symbol: "unshift"},
		"reverse":         {PackagePath: "trb/internal/arrays", Symbol: "reverse"},
		"sort":            {PackagePath: "trb/internal/arrays", Symbol: "sort"},
		"sort_descending": {PackagePath: "trb/internal/arrays", Symbol: "sort_descending"},
	},
	types.Range: {
		"to_a": {PackagePath: "trb/internal/ranges", Symbol: "to_array"},
	},
	types.Hash: {
		"size":      {PackagePath: "trb/internal/hashes", Symbol: "length"},
		"empty?":    {PackagePath: "trb/internal/hashes", Symbol: "empty"},
		"fetch":     {PackagePath: "trb/internal/hashes", Symbol: "fetch"},
		"try_fetch": {PackagePath: "trb/internal/hashes", Symbol: "try_fetch"},
		"key?":      {PackagePath: "trb/internal/hashes", Symbol: "contains_key"},
		"keys":      {PackagePath: "trb/internal/hashes", Symbol: "keys"},
		"values":    {PackagePath: "trb/internal/hashes", Symbol: "values"},
		"dup":       {PackagePath: "trb/internal/hashes", Symbol: "copy"},
		"delete":    {PackagePath: "trb/internal/hashes", Symbol: "delete"},
		"merge":     {PackagePath: "trb/internal/hashes", Symbol: "merge"},
		"update":    {PackagePath: "trb/internal/hashes", Symbol: "update"},
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

func runtimeIndependent(symbol Symbol) Symbol {
	symbol.RuntimeIndependent = true
	return symbol
}

func bytesBinary(name, intrinsic string, result types.Type) Symbol {
	return Symbol{
		Name:      name,
		Intrinsic: intrinsic,
		Parameters: []Parameter{
			{Name: "left", Type: bytesType},
			{Name: "right", Type: bytesType},
		},
		Return: result,
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

func rangeOf(element types.Type) types.Type {
	return types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{element}}
}

func hashOf(key, value types.Type) types.Type {
	return types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{key, value}}
}

func filesystemResult(value types.Type) types.Type {
	return ResultType(value, fileSystemErrorType)
}

func fileRead(name string, value types.Type) Symbol {
	return Symbol{
		Name:       name,
		Intrinsic:  "trb.std.file." + name,
		Receiver:   fileType,
		Parameters: []Parameter{{Name: "max_bytes", Type: integerType, Keyword: true}},
		Return:     filesystemResult(value),
	}
}

func fileWrite(name string, value types.Type) Symbol {
	return Symbol{
		Name:       name,
		Intrinsic:  "trb.std.file." + name,
		Receiver:   fileType,
		Parameters: []Parameter{{Name: "value", Type: value}},
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
		result.RequiredSymbols = []string{"JSON"}
	}
	return result
}

func Lookup(packagePath string) (*Package, bool) {
	definition, ok := registry[packagePath]
	return definition, ok
}

// PublicPortablePackages returns public compiler-owned packages that authored
// source may import in the selected backend mode.
func PublicPortablePackages(mode string) []*Package {
	paths := make([]string, 0, len(registry))
	for packagePath, definition := range registry {
		if definition == nil || definition.Internal || definition.Kind != Portable || !definition.Supports(mode) {
			continue
		}
		paths = append(paths, packagePath)
	}
	sort.Strings(paths)
	result := make([]*Package, 0, len(paths))
	for _, packagePath := range paths {
		result = append(result, registry[packagePath])
	}
	return result
}

// RuntimeExportPackages returns public portable packages that declare source-
// visible runtime types. Interactive tooling uses this catalog without
// exposing internal packages or target-specific APIs.
func RuntimeExportPackages(mode string) []*Package {
	result := []*Package{}
	for _, definition := range PublicPortablePackages(mode) {
		if len(definition.RuntimeExports) > 0 {
			result = append(result, definition)
		}
	}
	return result
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

// OpaqueType reports whether an exact compiler-owned package type can only be
// introduced by that package's checked operations. Declaration identity is
// required so an unrelated type with the same display name remains ordinary.
func OpaqueType(typ types.Type) bool {
	return OpaqueTypeConstructionMessage(typ) != ""
}

// OpaqueTypeConstructionMessage returns the source-facing diagnostic for an
// exact compiler-owned type whose values cannot be directly constructed.
func OpaqueTypeConstructionMessage(typ types.Type) string {
	if typ.Declaration.Empty() {
		return ""
	}
	for _, definition := range registry {
		if message := definition.OpaqueTypes[typ.Declaration]; message != "" {
			return message
		}
	}
	return ""
}

// RuntimeDependenciesForType returns compiler-owned modules whose runtime
// declarations are named by a library intrinsic's result type. Source code
// still needs an explicit import to refer to those declarations directly.
func RuntimeDependenciesForType(typ types.Type) []*Package {
	declarations := map[identity.Declaration]bool{}
	unresolvedNames := map[string]bool{}
	var collect func(types.Type)
	collect = func(current types.Type) {
		if !current.Declaration.Empty() {
			declarations[current.Declaration] = true
		} else if current.Name != "" {
			unresolvedNames[current.Name] = true
		}
		for _, argument := range current.Args {
			collect(argument)
		}
	}
	collect(typ)

	dependencies := []*Package{}
	for _, definition := range registry {
		for _, exported := range definition.RuntimeExports {
			declaration := identity.Declaration{Module: definition.ModulePath, Name: exported.Name, Kind: identity.Kind(exported.Kind)}
			if declarations[declaration] || unresolvedNames[exported.Name] {
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
		// Exact nominal receiver contracts also apply when the value was
		// returned by another module and its package was not directly imported.
		if receiver.Kind == types.Named && !receiver.Declaration.Empty() {
			for _, definition := range registry {
				symbol, found := definition.Symbols[name]
				if found && symbol.HasReceiver() && symbol.Receiver.Declaration == receiver.Declaration && receiverMatches(symbol.Receiver, receiver, symbol.TypeParameters) {
					symbol.Receiver = receiver
					return definition, symbol, true
				}
			}
		}
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
	methodsByName := map[string]Symbol{}
	names := make([]string, 0, len(targets))
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		_, method, ok := LookupReceiverMethod(receiver, name)
		if ok {
			methodsByName[name] = method
		}
	}
	// Named resource types cannot share the built-in kind table: their
	// declaration identity, rather than their display name, owns the method.
	// Discover those contracts directly and retain the same exact matching used
	// by the checker so an unrelated File declaration receives no host methods.
	for _, definition := range registry {
		for name, method := range definition.Symbols {
			if !method.HasReceiver() || !ReceiverMatches(method.Receiver, receiver, method.TypeParameters) {
				continue
			}
			method.Name = name
			method.Receiver = receiver
			methodsByName[name] = method
		}
	}
	names = names[:0]
	for name := range methodsByName {
		names = append(names, name)
	}
	sort.Strings(names)
	methods := make([]Symbol, 0, len(names))
	for _, name := range names {
		methods = append(methods, methodsByName[name])
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
		if expected.Kind == types.Named {
			if !expected.Declaration.Empty() {
				if expected.Declaration != value.Declaration {
					return false
				}
			} else if expected.Name != value.Name {
				return false
			}
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

// ReceiverMatches reports whether a compiler-known receiver pattern applies to
// a checked value type. Official packages use the same matching rule as the
// portable standard library, while remaining unavailable until explicitly
// imported.
func ReceiverMatches(pattern, actual types.Type, typeParameterNames []string) bool {
	return receiverMatches(pattern, actual, typeParameterNames)
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
	if symbol.Block != nil {
		block := *symbol.Block
		block.Parameters = append([]types.Type(nil), symbol.Block.Parameters...)
		for index := range block.Parameters {
			block.Parameters[index] = substituteType(block.Parameters[index], bindings)
		}
		block.Return = substituteType(block.Return, bindings)
		block.ResultBoundary = substituteType(block.ResultBoundary, bindings)
		block.ScopedParameters = append([]bool(nil), symbol.Block.ScopedParameters...)
		result.Block = &block
	}
	result.EqualityTypes = append([]types.Type(nil), symbol.EqualityTypes...)
	for index := range result.EqualityTypes {
		result.EqualityTypes[index] = substituteType(result.EqualityTypes[index], bindings)
	}
	result.OrderingTypes = append([]types.Type(nil), symbol.OrderingTypes...)
	for index := range result.OrderingTypes {
		result.OrderingTypes[index] = substituteType(result.OrderingTypes[index], bindings)
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
