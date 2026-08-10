// Package ast defines the lossless syntax tree. It intentionally contains a
// NativeStatement node: Ruby/Rails DSLs are open ended, so Ruby-only constructs
// can survive parsing without weakening the typed, portable subset.
package ast

import (
	"strings"

	"github.com/type-rb/type-rb/internal/token"
)

type Node interface {
	Span() token.Span
}

type Base struct {
	SourceSpan      token.Span
	TrailingComment string
}

func (b Base) Span() token.Span { return b.SourceSpan }

type Program struct {
	Base
	Mode              string
	Package           string
	ModulePath        string
	GoModule          string
	RubyLoader        string
	TypeScriptRuntime string
	Statements        []Statement
	Tokens            []token.Token
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

type CommentStatement struct {
	Base
	Text string
}

func (*CommentStatement) statementNode() {}

type BlankStatement struct{ Base }

func (*BlankStatement) statementNode() {}

type ImportStatement struct {
	Base
	Path    string
	Symbols []string
	Alias   string
}

func (*ImportStatement) statementNode() {}

type ClassStatement struct {
	Base
	Name       string
	Superclass Expression
	Implements []string
	Body       []Statement
}

func (*ClassStatement) statementNode() {}

// RecordStatement is a closed product type. Unlike ClassStatement it has
// keyword-only construction, public data fields, value semantics, and no
// inheritance. Backends lower it to their native data representation.
type RecordStatement struct {
	Base
	Name string
	Body []Statement
}

func (*RecordStatement) statementNode() {}

type Attribute struct {
	Base
	Name      string
	Arguments []CallArgument
}

type RecordFieldStatement struct {
	Base
	Name       string
	Type       TypeRef
	Attributes []Attribute
}

func (*RecordFieldStatement) statementNode() {}

// EnumStatement is a closed nominal set of values. Members remain statements
// so their source spans and comments survive the syntax and formatting passes.
type EnumStatement struct {
	Base
	Name           string
	TypeParameters []TypeParameter
	Body           []Statement
}

func (*EnumStatement) statementNode() {}

type EnumMemberStatement struct {
	Base
	Name       string
	Parameters []Parameter
}

func (*EnumMemberStatement) statementNode() {}

// TypeAliasStatement declares a transparent source-level alias. The target is
// retained in the syntax tree so tooling can show the authored name while the
// checker compares the expanded type.
type TypeAliasStatement struct {
	Base
	Name           string
	TypeParameters []TypeParameter
	Target         TypeRef
}

func (*TypeAliasStatement) statementNode() {}

type ModuleStatement struct {
	Base
	Name string
	Body []Statement
}

func (*ModuleStatement) statementNode() {}

type InterfaceStatement struct {
	Base
	Name    string
	Methods []*MethodStatement
}

func (*InterfaceStatement) statementNode() {}

type FieldStatement struct {
	Base
	Name     string
	Type     TypeRef
	Value    Expression
	ReadOnly bool
}

func (*FieldStatement) statementNode() {}

type Parameter struct {
	Base
	Name        string
	Type        TypeRef
	Default     Expression
	Keyword     bool
	Rest        bool
	KeywordRest bool
}

type TypeParameter struct {
	Base
	Name string
}

type MethodStatement struct {
	Base
	Name           string
	TypeParameters []TypeParameter
	Parameters     []Parameter
	ReturnType     TypeRef
	Fails          TypeRef
	Body           []Statement
	Class          bool
}

func (*MethodStatement) statementNode() {}

type VariableStatement struct {
	Base
	Name     string
	Type     TypeRef
	Value    Expression
	Mutable  bool
	Constant bool
}

func (*VariableStatement) statementNode() {}

type AssignmentStatement struct {
	Base
	Target   Expression
	Operator string
	Value    Expression
}

func (*AssignmentStatement) statementNode() {}

type ReturnStatement struct {
	Base
	Value Expression
}

func (*ReturnStatement) statementNode() {}

type BreakStatement struct{ Base }

func (*BreakStatement) statementNode() {}

type NextStatement struct{ Base }

func (*NextStatement) statementNode() {}

type ExpressionStatement struct {
	Base
	Expression Expression
}

func (*ExpressionStatement) statementNode() {}

type IfBranch struct {
	Condition Expression
	Body      []Statement
}

type IfStatement struct {
	Base
	Condition Expression
	Then      []Statement
	ElseIf    []IfBranch
	Else      []Statement
	HasElse   bool
}

func (*IfStatement) statementNode()  {}
func (*IfStatement) expressionNode() {}

type CaseBranch struct {
	Base
	Value    Expression
	Bindings []PatternBinding
	Body     []Statement
}

// PatternBinding is a name introduced by a payload enum pattern. The payload
// type comes from the matched enum member and is attached in typed IR.
type PatternBinding struct {
	Base
	Name string
}

type CaseStatement struct {
	Base
	Value    Expression
	Leading  []Statement
	Branches []CaseBranch
	Else     []Statement
	HasElse  bool
}

func (*CaseStatement) statementNode()  {}
func (*CaseStatement) expressionNode() {}

type WhileStatement struct {
	Base
	Condition Expression
	Body      []Statement
}

func (*WhileStatement) statementNode() {}

// IterationExpression is the portable, Ruby-shaped collection iteration
// syntax. It remains an expression in the syntax tree so the original block
// delimiters can be retained, then lowers to structured iteration IR instead
// of a target-language callback.
type IterationExpression struct {
	Base
	Source    Expression
	Operation string
	SliceSize Expression
	Initial   Expression
	WithIndex bool
	Block     *BlockExpression
}

func (*IterationExpression) expressionNode() {}

// NativeStatement and NativeBlock are explicit Ruby interoperability nodes.
// They are rejected by portable backends with a precise diagnostic.
type NativeStatement struct {
	Base
	Text string
}

func (*NativeStatement) statementNode() {}

type NativeBlock struct {
	Base
	Header string
	Body   []Statement
	Closer string
}

func (*NativeBlock) statementNode() {}

type TypeRef struct {
	Base
	Name      string
	Arguments []TypeRef
	Union     []TypeRef
	Nullable  bool
	Array     bool
}

func (t TypeRef) Empty() bool { return t.Name == "" && len(t.Union) == 0 }

func (t TypeRef) String() string {
	if len(t.Union) > 0 {
		parts := make([]string, len(t.Union))
		for index, alternative := range t.Union {
			parts[index] = alternative.String()
		}
		return strings.Join(parts, " | ")
	}
	if t.Name == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString(t.Name)
	if len(t.Arguments) > 0 {
		b.WriteByte('<')
		for i, arg := range t.Arguments {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(arg.String())
		}
		b.WriteByte('>')
	}
	if t.Array {
		b.WriteString("[]")
	}
	if t.Nullable {
		b.WriteByte('?')
	}
	return b.String()
}

type Identifier struct {
	Base
	Name string
}

func (*Identifier) expressionNode() {}

type LiteralKind string

const (
	StringLiteral  LiteralKind = "string"
	IntegerLiteral LiteralKind = "integer"
	FloatLiteral   LiteralKind = "float"
	BooleanLiteral LiteralKind = "boolean"
	NilLiteral     LiteralKind = "nil"
)

type Literal struct {
	Base
	Kind LiteralKind
	Raw  string
}

func (*Literal) expressionNode() {}

type StringPart struct {
	Text       string
	Expression Expression
}

type InterpolatedString struct {
	Base
	Raw   string
	Parts []StringPart
}

func (*InterpolatedString) expressionNode() {}

type SymbolLiteral struct {
	Base
	Name string
	Raw  string
}

func (*SymbolLiteral) expressionNode() {}

type ArrayLiteral struct {
	Base
	Elements []Expression
}

func (*ArrayLiteral) expressionNode() {}

type HashEntry struct {
	Key   Expression
	Value Expression
}

type HashLiteral struct {
	Base
	Entries []HashEntry
}

func (*HashLiteral) expressionNode() {}

type UnaryExpression struct {
	Base
	Operator string
	Operand  Expression
}

func (*UnaryExpression) expressionNode() {}

type BinaryExpression struct {
	Base
	Left     Expression
	Operator string
	Right    Expression
}

func (*BinaryExpression) expressionNode() {}

type RangeExpression struct {
	Base
	Start     Expression
	End       Expression
	Exclusive bool
}

func (*RangeExpression) expressionNode() {}

// AttemptExpression turns the fallible effects produced while evaluating a
// single expression or a statement block into a Result value. Exactly one of
// Value and Body is populated.
type AttemptExpression struct {
	Base
	Value Expression
	Body  []Statement
}

func (*AttemptExpression) expressionNode() {}

type CallArgument struct {
	Name  string
	Value Expression
	Splat string
}

type CallExpression struct {
	Base
	Callee    Expression
	Arguments []CallArgument
	Block     *BlockExpression
}

func (*CallExpression) expressionNode() {}

type GenericExpression struct {
	Base
	Receiver  Expression
	Arguments []TypeRef
}

func (*GenericExpression) expressionNode() {}

type MemberExpression struct {
	Base
	Receiver  Expression
	Name      string
	Safe      bool
	Namespace bool
}

func (*MemberExpression) expressionNode() {}

type IndexExpression struct {
	Base
	Receiver Expression
	Index    Expression
}

func (*IndexExpression) expressionNode() {}

type BlockExpression struct {
	Base
	Parameters []string
	Body       []Statement
	Brace      bool
}

func (*BlockExpression) expressionNode() {}

type NativeExpression struct {
	Base
	Text string
}

func (*NativeExpression) expressionNode() {}
