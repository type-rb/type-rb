// Package stdlib describes compiler-known portable and platform packages.
// Definitions are semantic contracts; backends lower their intrinsic IDs to
// target APIs without leaking those APIs into the TypeRB source language.
package stdlib

import (
	"path"
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
}

type Symbol struct {
	Name            string
	Intrinsic       string
	Receiver        types.Type
	ReceiverMutable bool
	Parameters      []Parameter
	Return          types.Type
	Variadic        bool
	Inference       string
}

func (s Symbol) HasReceiver() bool {
	return s.Receiver.Kind != "" && s.Receiver.Kind != types.Invalid
}

type receiverMethodTarget struct {
	PackagePath string
	Symbol      string
}

type Package struct {
	Path         string
	ModulePath   string
	RuntimeAlias string
	Source       string
	Kind         Kind
	Targets      map[string]bool
	NativeSyntax bool
	TypeProvider string
	Symbols      map[string]Symbol
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
var integerType = types.FromName("Integer")
var booleanType = types.FromName("Boolean")
var voidType = types.FromName("Void")
var anyType = types.FromName("Any")

var registry = map[string]*Package{
	"trb/std/result": {
		Path:         "trb/std/result",
		ModulePath:   "trb/std/result/index",
		RuntimeAlias: "__trb_result",
		Source: `enum Result<T, E>
	Ok(value: T)
	Err(error: E)
end
`,
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
	"trb/std/strings": {
		Path: "trb/std/strings",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"length":    unary("length", "trb.std.strings.length", stringType, integerType),
			"uppercase": unary("uppercase", "trb.std.strings.uppercase", stringType, stringType),
			"lowercase": unary("lowercase", "trb.std.strings.lowercase", stringType, stringType),
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
			"length": unary("length", "trb.std.arrays.length", types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Any")}}, integerType),
			"push": {
				Name: "push", Intrinsic: "trb.std.arrays.push",
				Parameters: []Parameter{{Name: "values", Type: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName("Any")}}, Mutable: true}, {Name: "value", Type: types.FromName("Any")}},
				Return:     voidType,
			},
		},
	},
	"trb/std/numbers": {
		Path: "trb/std/numbers",
		Kind: Portable,
		Symbols: map[string]Symbol{
			"to_string":     unary("to_string", "trb.std.numbers.to_string", integerType, stringType),
			"parse_integer": unary("parse_integer", "trb.std.numbers.parse_integer", stringType, integerType),
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
		"to_s": {PackagePath: "trb/std/numbers", Symbol: "to_string"},
	},
	types.String: {
		"to_i":     {PackagePath: "trb/std/numbers", Symbol: "parse_integer"},
		"size":     {PackagePath: "trb/std/strings", Symbol: "length"},
		"to_bytes": {PackagePath: "trb/std/bytes", Symbol: "from_string"},
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
		"to_s":             {PackagePath: "trb/std/string_builder", Symbol: "to_string"},
		"clear":            {PackagePath: "trb/std/string_builder", Symbol: "clear"},
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

func Lookup(packagePath string) (*Package, bool) {
	definition, ok := registry[packagePath]
	return definition, ok
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
	symbol.Name = name
	symbol.Receiver = receiver
	symbol.ReceiverMutable = symbol.Parameters[0].Mutable
	symbol.Parameters = append([]Parameter(nil), symbol.Parameters[1:]...)
	return definition, symbol, true
}

func IsReservedPath(packagePath string) bool {
	return strings.HasPrefix(packagePath, "trb/")
}
