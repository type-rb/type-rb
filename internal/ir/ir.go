// Package ir defines the resolved, target-independent representation consumed
// by every backend. Unlike syntax AST nodes, IR expressions carry semantic
// types and declarations contain normalized names.
package ir

import (
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type Base struct {
	Span            token.Span
	TrailingComment string
}

type Program struct {
	Mode       string
	Package    string
	ModulePath string
	GoModule   string
	RubyLoader string
	Statements []Statement
}

type Statement interface{ irStatement() }
type Expression interface {
	irExpression()
	ExprType() types.Type
}

type Comment struct {
	Base
	Text string
}

func (*Comment) irStatement() {}

type Import struct {
	Base
	Path      string
	Symbols   []string
	Alias     string
	Namespace bool
	Kind      string
	Standard  bool
	Platform  bool
	Runtime   bool
	// Implicit identifies compiler-injected runtime dependencies that are not
	// source-visible imports. Language tooling must not offer their exports as
	// though the application imported them explicitly.
	Implicit bool
	// IntrinsicSymbols are resolved at compile time and have no corresponding
	// runtime export in a compiler-owned source module.
	IntrinsicSymbols map[string]bool
	// SymbolKinds distinguishes value records from reference classes in
	// backends whose representation makes that distinction explicit.
	SymbolKinds map[string]string
}

func (*Import) irStatement() {}

type Class struct {
	Base
	Name       string
	Superclass Expression
	Implements []string
	Body       []Statement
}

func (*Class) irStatement() {}

type Record struct {
	Base
	Name string
	Body []Statement
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
}

func (*Enum) irStatement() {}

type EnumMember struct {
	Base
	Name   string
	Fields []Parameter
}

func (*EnumMember) irStatement() {}

type Module struct {
	Base
	Name string
	Body []Statement
}

func (*Module) irStatement() {}

type Interface struct {
	Base
	Name    string
	Methods []*Method
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
	Name        string
	Type        types.Type
	Default     Expression
	Keyword     bool
	Rest        bool
	KeywordRest bool
}

type Method struct {
	Base
	Name           string
	TypeParameters []string
	Parameters     []Parameter
	ReturnType     types.Type
	Body           []Statement
	Class          bool
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
}
type If struct {
	Base
	Condition Expression
	Then      []Statement
	ElseIf    []IfBranch
	Else      []Statement
}

func (*If) irStatement() {}

type CaseBranch struct {
	Base
	Value       Expression
	EnumName    string
	Member      string
	Bindings    []CaseBinding
	PayloadEnum bool
	Body        []Statement
}

type CaseBinding struct {
	Name  string
	Field string
	Type  types.Type
}

type Case struct {
	Base
	Value    Expression
	Leading  []Statement
	Branches []CaseBranch
	Else     []Statement
	HasElse  bool
}

func (*Case) irStatement() {}

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

type Iterate struct {
	Base
	Source    Expression
	Operation string
	SliceSize Expression
	WithIndex bool
	Bindings  []IterationBinding
	Body      []Statement
}

func (*Iterate) irStatement() {}

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

type Unary struct {
	ExprBase
	Operator string
	Operand  Expression
}

func (*Unary) irExpression() {}

type ConversionKind string

const IntegerToFloatConversion ConversionKind = "integer_to_float"

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
	Callee    Expression
	Arguments []CallArgument
	Block     *Block
	Codec     *CodecSchema
}

func (*Call) irExpression() {}

// CodecSchema is the checked, target-independent shape used by typed JSON
// encode/decode intrinsics. Backends consume this schema instead of reflecting
// over a generated target type.
type CodecSchema struct {
	Type      types.Type
	Kind      string
	Module    string
	Reference *Reference
	Element   *CodecSchema
	Fields    []CodecField
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

type TypeApply struct {
	ExprBase
	Receiver  Expression
	Arguments []types.Type
}

func (*TypeApply) irExpression() {}

type Member struct {
	ExprBase
	Receiver  Expression
	Name      string
	Safe      bool
	Namespace bool
	Reference *Reference
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
}
