// Package ir defines the resolved, target-independent representation consumed
// by every backend. Unlike syntax AST nodes, IR expressions carry semantic
// types and declarations contain normalized names.
package ir

import (
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type Base struct {
	Span            token.Span
	TrailingComment string
}

func (b Base) SourceSpan() token.Span { return b.Span }

type Program struct {
	Mode              string
	SourcePath        string
	Package           string
	ModulePath        string
	GoModule          string
	RubyLoader        string
	TypeScriptRuntime string
	UsesJSX           bool
	Declarations      *declaration.Catalog
	Extensions        []Extension
	Statements        []Statement
}

// Extension is typed compile-time data contributed by a package integration.
// The core IR transports it without knowing package-specific schemas.
type Extension interface {
	ExtensionName() string
}

type Statement interface {
	irStatement()
	SourceSpan() token.Span
}
type Expression interface {
	irExpression()
	ExprType() types.Type
	SourceSpan() token.Span
}

type Comment struct {
	Base
	Text string
}

func (*Comment) irStatement() {}

type Import struct {
	Base
	Path string
	// DeclaredPath retains the source spelling when Path is resolved to a
	// canonical module identity. Language tooling uses it to link imports.
	DeclaredPath string
	Symbols      []string
	Alias        string
	Namespace    bool
	Kind         string
	Standard     bool
	Official     bool
	Platform     bool
	Native       bool
	Runtime      bool
	// RuntimeRequired records that generated code must load the source module
	// for compiler-owned runtime behavior. Fully lowered intrinsics can leave
	// this false.
	RuntimeRequired bool
	// Implicit identifies compiler-injected runtime dependencies that are not
	// source-visible imports. Language tooling must not offer their exports as
	// though the application imported them explicitly.
	Implicit bool
	// IntrinsicSymbols are lowered by the backend instead of being imported as
	// named functions from a compiler-owned source module.
	IntrinsicSymbols map[string]bool
	// RuntimeIndependentSymbols are the intrinsic subset that does not
	// reference the compiler-owned source module on any backend.
	RuntimeIndependentSymbols map[string]bool
	// SymbolKinds distinguishes value records from reference classes in
	// backends whose representation makes that distinction explicit.
	SymbolKinds map[string]string
	// SymbolTypes, SymbolParameters, and SymbolTypeParameters retain imported
	// declaration contracts for editor tooling even when the imported module has
	// no generated TypeRB IR.
	SymbolTypes          map[string]types.Type
	SymbolParameters     map[string][]callsignature.Parameter
	SymbolTypeParameters map[string][]string
	// TypeContracts retain the structural declarations referenced by native
	// package signatures so editor tooling can instantiate their members.
	TypeContracts map[string]TypeContract
	// GeneratedTypeSymbols are target-package type exports referenced only by
	// imported declaration contracts. They are emitted as type-only imports but
	// stay invisible to source name resolution and editor completion.
	GeneratedTypeSymbols []string
	// RuntimeSymbols map semantic declaration exports to separately validated
	// target-native runtime functions. Their signatures remain in SymbolTypes
	// and SymbolParameters.
	RuntimeSymbols map[string]RuntimeBinding
}

func (*Import) irStatement() {}

type TypeContract struct {
	TypeParameters []string
	AliasTarget    *types.Type
	Members        map[string]MemberContract
}

type MemberContract struct {
	Kind           string
	Type           types.Type
	TypeParameters []string
	Parameters     []callsignature.Parameter
	Variadic       bool
	Class          bool
	Readonly       bool
}

type RuntimeBinding struct {
	Identity                 string
	Dependency               string
	Module                   string
	Symbol                   string
	CallConvention           string
	MaySuspend               bool
	PropagatesExecutionScope bool
}

type Class struct {
	Base
	Name           string
	TypeParameters []string
	External       bool
	Superclass     Expression
	Implements     []types.Type
	Body           []Statement
}

func (*Class) irStatement() {}

type Record struct {
	Base
	Name           string
	TypeParameters []string
	Body           []Statement
}

func (*Record) irStatement() {}

type Attribute struct {
	Name      string
	Arguments []CallArgument
}

type RecordField struct {
	Base
	Name       string
	Type       types.Type
	Attributes []Attribute
}

func (*RecordField) irStatement() {}

type Enum struct {
	Base
	Name           string
	TypeParameters []string
	Body           []Statement
	RawType        types.Type
}

func (*Enum) irStatement() {}

type EnumMember struct {
	Base
	Name     string
	Fields   []Parameter
	RawValue Expression
}

func (*EnumMember) irStatement() {}

type TypeAlias struct {
	Base
	Name           string
	TypeParameters []string
	Target         types.Type
	Variants       []EnumMember
}

func (*TypeAlias) irStatement() {}

type Newtype struct {
	Base
	Name   string
	Target types.Type
}

func (*Newtype) irStatement() {}

type Module struct {
	Base
	Name string
	Body []Statement
}

func (*Module) irStatement() {}

type Interface struct {
	Base
	Name           string
	TypeParameters []string
	Methods        []*Method
}

func (*Interface) irStatement() {}

type Field struct {
	Base
	Name     string
	Type     types.Type
	Value    Expression
	ReadOnly bool
}

func (*Field) irStatement() {}

type Parameter struct {
	Name                 string
	Type                 types.Type
	Default              Expression
	NamedOnly            bool
	Keyword              bool
	Rest                 bool
	KeywordRest          bool
	LiteralValues        []string
	LiteralArrays        [][]string
	LiteralArrayElements []string
}

type MethodSignature struct {
	Parameters []Parameter
	ReturnType types.Type
	Variadic   bool
}

type Method struct {
	Base
	Name           string
	External       bool
	TargetName     string
	TypeParameters []string
	Parameters     []Parameter
	Alternatives   []MethodSignature
	ReturnType     types.Type
	Body           []Statement
	Class          bool
	// Property exposes an external member without call syntax. Loadable
	// properties additionally provide load(), reload(), and loaded?() controls.
	Property bool
	Loadable bool
}

func (*Method) irStatement() {}

type Variable struct {
	Base
	Name     string
	Type     types.Type
	Value    Expression
	Mutable  bool
	Constant bool
	Owner    string
}

func (*Variable) irStatement() {}

// Temporary declares an uninitialized compiler-owned local. Portable source
// cannot construct this node; backend normalization uses it when a value-
// producing control-flow expression must be emitted as enclosing statements.
type Temporary struct {
	Base
	Name string
	Type types.Type
}

func (*Temporary) irStatement() {}

type Assignment struct {
	Base
	Target   Expression
	Operator string
	Value    Expression
}

func (*Assignment) irStatement() {}

type Return struct {
	Base
	Value Expression
}

func (*Return) irStatement() {}

type Break struct{ Base }

func (*Break) irStatement() {}

type Next struct{ Base }

func (*Next) irStatement() {}

type ExpressionStatement struct {
	Base
	Expression Expression
}

func (*ExpressionStatement) irStatement() {}

type IfBranch struct {
	Condition Expression
	Body      []Statement
	Result    Expression
	Diverges  bool
}
type If struct {
	ExprBase
	Condition    Expression
	Then         []Statement
	ThenResult   Expression
	ThenDiverges bool
	ElseIf       []IfBranch
	Else         []Statement
	ElseResult   Expression
	ElseDiverges bool
	HasElse      bool
}

func (*If) irStatement()  {}
func (*If) irExpression() {}

type CaseBranch struct {
	Base
	Value        Expression
	Alternatives []Expression
	EnumName     string
	Member       string
	Bindings     []CaseBinding
	PayloadEnum  bool
	TypePattern  bool
	MatchType    types.Type
	Narrowings   []CaseBinding
	Body         []Statement
	Result       Expression
	Diverges     bool
}

type CaseBinding struct {
	Name      string
	Field     string
	Type      types.Type
	Generated bool
}

type Case struct {
	ExprBase
	Value          Expression
	Leading        []Statement
	Branches       []CaseBranch
	Else           []Statement
	HasElse        bool
	TypeUnion      bool
	ElseResult     Expression
	ElseDiverges   bool
	ElseNarrowings []CaseBinding
}

func (*Case) irStatement()  {}
func (*Case) irExpression() {}

type While struct {
	Base
	Condition Expression
	Body      []Statement
}

func (*While) irStatement() {}

// Iterate is a structured loop rather than a callback invocation. Backends
// can therefore preserve TypeRB control-flow semantics inside an each block.
// Bindings retain the checked type of every source-level block parameter.
type IterationBinding struct {
	Name string
	Type types.Type
}

type IterationResult struct {
	Variable *Variable
	Target   Expression
	Return   bool
	Type     types.Type
}

type Iterate struct {
	Base
	Source    Expression
	Operation string
	Intrinsic string
	SliceSize Expression
	WithIndex bool
	Bindings  []IterationBinding
	Body      []Statement
	Result    *IterationResult
	// Fails, EffectSuccess, and CaptureEffect describe only a compiler-declared
	// structured Result boundary. They do not represent function effects.
	Fails          types.Type
	EffectSuccess  types.Type
	CaptureEffect  bool
	ResultBoundary bool
}

func (*Iterate) irStatement() {}

// StructuredBlock keeps a compiler-owned, value-producing block call as a
// typed control-flow boundary. Backends decide how the intrinsic acquires and
// releases its scoped resource, while the block body and result remain normal
// TypeRB IR.
type StructuredBlockResult struct {
	Variable *Variable
	Target   Expression
	Return   bool
	Type     types.Type
}

type StructuredBlock struct {
	Base
	Call      *Call
	Intrinsic string
	Bindings  []IterationBinding
	Body      []Statement
	Value     Expression
	Result    *StructuredBlockResult
	// Fails, EffectSuccess, and CaptureEffect describe only the structured
	// Result boundary declared by the provider.
	Fails            types.Type
	EffectSuccess    types.Type
	PropagateSuccess types.Type
	CaptureEffect    bool
}

func (*StructuredBlock) irStatement() {}

// Transform is a value-producing collection operation. It is distinct from a
// target callback so checker-derived item/result types and block semantics are
// retained until backend lowering.
type Transform struct {
	ExprBase
	Source      Expression
	Operation   string
	Initial     Expression
	WithIndex   bool
	Item        string
	Index       string
	Accumulator string
	ItemType    types.Type
	Body        []Statement
	Result      Expression
}

func (*Transform) irExpression() {}

type Native struct {
	Base
	Text string
}

func (*Native) irStatement() {}

type NativeBlock struct {
	Base
	Header string
	Body   []Statement
	Closer string
}

func (*NativeBlock) irStatement() {}

type ExprBase struct {
	Base
	Type types.Type
}

func (e ExprBase) ExprType() types.Type { return e.Type }

type Identifier struct {
	ExprBase
	Name      string
	Owner     string
	Lexical   bool // Resolved to a lexical binding rather than a same-named member.
	Generated bool // Compiler-owned name that must bypass source identifier rewriting.
	Reference *Reference
}

func (*Identifier) irExpression() {}

type Literal struct {
	ExprBase
	Kind string
	Raw  string
}

func (*Literal) irExpression() {}

type StringPart struct {
	Text       string
	Expression Expression
}

type InterpolatedString struct {
	ExprBase
	Raw   string
	Parts []StringPart
}

func (*InterpolatedString) irExpression() {}

type Symbol struct {
	ExprBase
	Name string
	Raw  string
}

func (*Symbol) irExpression() {}

type Array struct {
	ExprBase
	Elements []Expression
}

func (*Array) irExpression() {}

type HashEntry struct {
	Key   Expression
	Value Expression
}
type Hash struct {
	ExprBase
	Entries []HashEntry
}

func (*Hash) irExpression() {}

type JSXAttribute struct {
	Name    string
	Value   Expression
	Boolean bool
}

type JSXChild interface{ jsxChild() }

type JSXElement struct {
	ExprBase
	Name       string
	Component  Expression
	Attributes []JSXAttribute
	Children   []JSXChild
	Fragment   bool
}

func (*JSXElement) irExpression() {}
func (*JSXElement) jsxChild()     {}

type JSXText struct {
	Text string
}

func (*JSXText) jsxChild() {}

type JSXExpression struct {
	Value Expression
}

func (*JSXExpression) jsxChild() {}

type Unary struct {
	ExprBase
	Operator string
	Operand  Expression
}

func (*Unary) irExpression() {}

type ConversionKind string

const IntegerToFloatConversion ConversionKind = "integer_to_float"
const UnionIntegerToFloatConversion ConversionKind = "union_integer_to_float"
const NonNullableToNullableConversion ConversionKind = "non_nullable_to_nullable"
const NullableToNonNullableConversion ConversionKind = "nullable_to_non_nullable"
const ResultFunctionToPromiseRejectionConversion ConversionKind = "result_function_to_promise_rejection"
const PromiseRejectionToResultConversion ConversionKind = "promise_rejection_to_result"
const RangeToIterableConversion ConversionKind = "range_to_iterable"
const NewtypeConstructionConversion ConversionKind = "newtype_construction"
const NewtypeValueConversion ConversionKind = "newtype_value"

type Conversion struct {
	ExprBase
	Kind  ConversionKind
	Value Expression
}

func (*Conversion) irExpression() {}

type Binary struct {
	ExprBase
	Left     Expression
	Operator string
	Right    Expression
}

func (*Binary) irExpression() {}

type Range struct {
	ExprBase
	Start     Expression
	End       Expression
	Exclusive bool
}

func (*Range) irExpression() {}

type CallArgument struct {
	Name  string
	Value Expression
	Splat string
}
type Call struct {
	ExprBase
	Callee          Expression
	Arguments       []CallArgument
	CallSignature   []callsignature.Parameter
	Block           *Block
	Codec           *CodecSchema
	DeclarationOnly bool
}

func (*Call) irExpression() {}

// Lambda is a first-class lexical function. Parameters and result remain
// target-independent so every backend can emit its native closure form.
type Lambda struct {
	ExprBase
	Parameters []Parameter
	ReturnType types.Type
	Body       []Statement
}

func (*Lambda) irExpression() {}

// CodecSchema is the checked, target-independent value shape used by typed
// codecs and protocol bindings. Backends consume this schema instead of
// reflecting over a generated target type.
type CodecSchema struct {
	Type      types.Type
	Kind      string
	Module    string
	Reference *Reference
	Element   *CodecSchema
	Fields    []CodecField
	RawType   types.Type
	RawValues []EnumRawValue
}

type CodecField struct {
	Name     string
	WireName string
	Schema   *CodecSchema
}

// EnumConstruct preserves nominal variant construction through lowering. It
// must not become an ordinary call because every backend uses a different
// runtime representation for payload enums.
type EnumConstruct struct {
	ExprBase
	EnumName      string
	Member        string
	TypeArguments []types.Type
	Arguments     []Expression
	Reference     *Reference
}

func (*EnumConstruct) irExpression() {}

// EnumCall preserves source-level enum methods independently from backend
// enum representations. Generated raw_value/from_raw operations use the same
// node so every backend and the REPL share one checked semantic boundary.
type EnumCall struct {
	ExprBase
	EnumName      string
	Method        string
	Receiver      Expression
	Arguments     []CallArgument
	CallSignature []callsignature.Parameter
	Reference     *Reference
	RawType       types.Type
	RawValues     []EnumRawValue
}

type EnumRawValue struct {
	Member string
	Raw    string
}

func (*EnumCall) irExpression() {}

type TypeApply struct {
	ExprBase
	Receiver       Expression
	Arguments      []types.Type
	Owner          string
	OwnerArguments []types.Type
	Kind           string
}

func (*TypeApply) irExpression() {}

type Member struct {
	ExprBase
	Receiver  Expression
	Name      string
	Safe      bool
	Namespace bool
	// ClassField distinguishes storage-backed class properties from methods and
	// record fields so backends can preserve both `value.name` and `value.name()`.
	ClassField bool
	// UnionAlternatives asks representation-sensitive backends to project a
	// common data member from an erased union value.
	UnionAlternatives []UnionMemberAlternative
	Reference         *Reference
}

type UnionMemberAlternative struct {
	Type       types.Type
	MemberType types.Type
}

func (*Member) irExpression() {}

type Index struct {
	ExprBase
	Receiver Expression
	Index    Expression
}

func (*Index) irExpression() {}

type Block struct {
	ExprBase
	Parameters []string
	Body       []Statement
	Brace      bool
}

func (*Block) irExpression() {}

type NativeExpression struct {
	ExprBase
	Text string
}

func (*NativeExpression) irExpression() {}

func NewExprBase(span token.Span, typ types.Type) ExprBase {
	return ExprBase{Base: Base{Span: span}, Type: typ}
}

// Reference identifies a symbol resolved from an import or the portable
// prelude. Intrinsic is non-empty for compiler-known standard/platform calls;
// project references use Package, Alias, Symbol, and ExportKind for
// target-specific qualification.
type Reference struct {
	Package        string
	Alias          string
	Symbol         string
	ExportKind     string
	Intrinsic      string
	ReceiverMethod bool
	Runtime        *RuntimeBinding
}
