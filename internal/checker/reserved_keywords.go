package checker

import (
	"fmt"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/token"
)

// validateReservedKeywords keeps Result control flow and compiler-owned names
// distinct from user-authored declarations and bindings. The lexer is
// intentionally lossless and represents keywords as identifiers, so this
// validation belongs at the checked source boundary rather than in
// target-specific codegen.
func (c *Checker) validateReservedKeywords(statements []ast.Statement) {
	for _, statement := range statements {
		c.validateReservedKeywordStatement(statement)
	}
}

func (c *Checker) validateReservedKeywordName(name, kind string, span token.Span) {
	if strings.HasPrefix(name, "__trb") {
		if c.compilerGeneratedStart > 0 && span.Start.Offset >= c.compilerGeneratedStart {
			return
		}
		c.error(span, fmt.Sprintf("%s uses the compiler-reserved __trb prefix and cannot be used as %s", name, kind))
		return
	}
	if name == "try" || name == "catch" || name == "alias" || name == "newtype" {
		c.error(span, fmt.Sprintf("%s is a reserved keyword and cannot be used as %s", name, kind))
	}
}

func (c *Checker) validateReservedKeywordTypeParameters(parameters []ast.TypeParameter) {
	for _, parameter := range parameters {
		c.validateReservedKeywordName(parameter.Name, "a type parameter", parameter.Span())
	}
}

func (c *Checker) validateReservedKeywordParameters(parameters []ast.Parameter, kind string) {
	for _, parameter := range parameters {
		c.validateReservedKeywordName(parameter.Name, kind, parameter.Span())
		c.validateReservedKeywordType(parameter.Type)
		c.validateReservedKeywordExpression(parameter.Default)
	}
}

func (c *Checker) validateReservedKeywordStatement(statement ast.Statement) {
	switch node := statement.(type) {
	case *ast.ImportStatement:
		for _, name := range node.Symbols {
			c.validateReservedKeywordName(name, "an imported name", node.Span())
		}
		c.validateReservedKeywordName(node.Alias, "an import alias", node.Span())
	case *ast.ClassStatement:
		c.validateReservedKeywordName(node.Name, "a class name", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		c.validateReservedKeywordExpression(node.Superclass)
		for _, implemented := range node.Implements {
			c.validateReservedKeywordType(implemented)
		}
		c.validateReservedKeywords(node.Body)
	case *ast.RecordStatement:
		c.validateReservedKeywordName(node.Name, "a record name", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		c.validateReservedKeywords(node.Body)
	case *ast.RecordFieldStatement:
		c.validateReservedKeywordName(node.Name, "a record field", node.Span())
		c.validateReservedKeywordType(node.Type)
		for _, attribute := range node.Attributes {
			for _, argument := range attribute.Arguments {
				c.validateReservedKeywordName(argument.Name, "a named argument", attribute.Span())
				c.validateReservedKeywordExpression(argument.Value)
			}
		}
	case *ast.EnumStatement:
		c.validateReservedKeywordName(node.Name, "an enum name", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		c.validateReservedKeywords(node.Body)
	case *ast.EnumMemberStatement:
		c.validateReservedKeywordName(node.Name, "an enum member", node.Span())
		c.validateReservedKeywordParameters(node.Parameters, "an enum payload field")
		c.validateReservedKeywordExpression(node.RawValue)
	case *ast.TypeAliasStatement:
		c.validateReservedKeywordName(node.Name, "a type alias", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		c.validateReservedKeywordType(node.Target)
	case *ast.NewtypeStatement:
		c.validateReservedKeywordName(node.Name, "a newtype", node.Span())
		c.validateReservedKeywordType(node.Target)
	case *ast.ModuleStatement:
		c.validateReservedKeywordName(node.Name, "a module name", node.Span())
		c.validateReservedKeywords(node.Body)
	case *ast.InterfaceStatement:
		c.validateReservedKeywordName(node.Name, "an interface name", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		for _, method := range node.Methods {
			c.validateReservedKeywordStatement(method)
		}
	case *ast.FieldStatement:
		c.validateReservedKeywordName(node.Name, "a field name", node.Span())
		c.validateReservedKeywordType(node.Type)
		c.validateReservedKeywordExpression(node.Value)
	case *ast.MethodStatement:
		c.validateReservedKeywordName(node.Name, "a function or method name", node.Span())
		c.validateReservedKeywordTypeParameters(node.TypeParameters)
		c.validateReservedKeywordParameters(node.Parameters, "a parameter name")
		c.validateReservedKeywordType(node.ReturnType)
		c.validateReservedKeywords(node.Body)
	case *ast.VariableStatement:
		c.validateReservedKeywordName(node.Name, "a variable name", node.Span())
		c.validateReservedKeywordType(node.Type)
		c.validateReservedKeywordExpression(node.Value)
	case *ast.AssignmentStatement:
		c.validateReservedKeywordExpression(node.Target)
		c.validateReservedKeywordExpression(node.Value)
	case *ast.ReturnStatement:
		c.validateReservedKeywordExpression(node.Value)
	case *ast.ExpressionStatement:
		c.validateReservedKeywordExpression(node.Expression)
	case *ast.IfStatement:
		c.validateReservedKeywordIf(node)
	case *ast.CaseStatement:
		c.validateReservedKeywordCase(node)
	case *ast.WhileStatement:
		c.validateReservedKeywordExpression(node.Condition)
		c.validateReservedKeywords(node.Body)
	case *ast.NativeBlock:
		c.validateReservedKeywords(node.Body)
	}
}

func (c *Checker) validateReservedKeywordExpression(expression ast.Expression) {
	if expression == nil {
		return
	}
	switch node := expression.(type) {
	case *ast.Identifier:
		c.validateReservedKeywordName(node.Name, "an identifier", node.Span())
	case *ast.InterpolatedString:
		for _, part := range node.Parts {
			c.validateReservedKeywordExpression(part.Expression)
		}
	case *ast.ArrayLiteral:
		for _, element := range node.Elements {
			c.validateReservedKeywordExpression(element)
		}
	case *ast.HashLiteral:
		for _, entry := range node.Entries {
			c.validateReservedKeywordExpression(entry.Key)
			c.validateReservedKeywordExpression(entry.Value)
		}
	case *ast.JSXElement:
		c.validateReservedKeywordExpression(node.Component)
		for _, attribute := range node.Attributes {
			c.validateReservedKeywordName(attribute.Name, "a JSX attribute", attribute.Span())
			c.validateReservedKeywordExpression(attribute.Value)
		}
		for _, child := range node.Children {
			switch value := child.(type) {
			case *ast.JSXElement:
				c.validateReservedKeywordExpression(value)
			case *ast.JSXExpression:
				c.validateReservedKeywordExpression(value.Value)
			}
		}
	case *ast.UnaryExpression:
		c.validateReservedKeywordExpression(node.Operand)
	case *ast.BinaryExpression:
		c.validateReservedKeywordExpression(node.Left)
		c.validateReservedKeywordExpression(node.Right)
	case *ast.RangeExpression:
		c.validateReservedKeywordExpression(node.Start)
		c.validateReservedKeywordExpression(node.End)
	case *ast.AttemptExpression:
		c.validateReservedKeywordExpression(node.Value)
		c.validateReservedKeywords(node.Body)
	case *ast.TryExpression:
		c.validateReservedKeywordExpression(node.Value)
	case *ast.CatchExpression:
		c.validateReservedKeywordExpression(node.Value)
		c.validateReservedKeywordName(node.Binding.Name, "a catch binding", node.Binding.Span())
		c.validateReservedKeywords(node.Body)
	case *ast.LambdaExpression:
		c.validateReservedKeywordParameters(node.Parameters, "a function parameter")
		c.validateReservedKeywordType(node.ReturnType)
		c.validateReservedKeywords(node.Body)
	case *ast.CallExpression:
		c.validateReservedKeywordExpression(node.Callee)
		for _, argument := range node.Arguments {
			c.validateReservedKeywordName(argument.Name, "a named argument", node.Span())
			c.validateReservedKeywordExpression(argument.Value)
		}
		if node.Block != nil {
			c.validateReservedKeywordBlock(node.Block)
		}
	case *ast.GenericExpression:
		c.validateReservedKeywordExpression(node.Receiver)
		for _, argument := range node.Arguments {
			c.validateReservedKeywordType(argument)
		}
	case *ast.MemberExpression:
		c.validateReservedKeywordExpression(node.Receiver)
		c.validateReservedKeywordName(node.Name, "a member name", node.Span())
	case *ast.IndexExpression:
		c.validateReservedKeywordExpression(node.Receiver)
		c.validateReservedKeywordExpression(node.Index)
	case *ast.BlockExpression:
		c.validateReservedKeywordBlock(node)
	case *ast.IterationExpression:
		c.validateReservedKeywordExpression(node.Source)
		c.validateReservedKeywordExpression(node.SliceSize)
		c.validateReservedKeywordExpression(node.Initial)
		c.validateReservedKeywordExpression(node.Limit)
		if node.Block != nil {
			c.validateReservedKeywordBlock(node.Block)
		}
	case *ast.IfStatement:
		c.validateReservedKeywordIf(node)
	case *ast.CaseStatement:
		c.validateReservedKeywordCase(node)
	}
}

func (c *Checker) validateReservedKeywordBlock(block *ast.BlockExpression) {
	for _, name := range block.Parameters {
		c.validateReservedKeywordName(name, "a block parameter", block.Span())
	}
	c.validateReservedKeywords(block.Body)
}

func (c *Checker) validateReservedKeywordIf(node *ast.IfStatement) {
	c.validateReservedKeywordExpression(node.Condition)
	c.validateReservedKeywords(node.Then)
	for _, branch := range node.ElseIf {
		c.validateReservedKeywordExpression(branch.Condition)
		c.validateReservedKeywords(branch.Body)
	}
	c.validateReservedKeywords(node.Else)
}

func (c *Checker) validateReservedKeywordCase(node *ast.CaseStatement) {
	c.validateReservedKeywordExpression(node.Value)
	c.validateReservedKeywords(node.Leading)
	for _, branch := range node.Branches {
		c.validateReservedKeywordExpression(branch.Value)
		for _, alternative := range branch.Alternatives {
			c.validateReservedKeywordExpression(alternative)
		}
		for _, binding := range branch.Bindings {
			c.validateReservedKeywordName(binding.Name, "a pattern binding", binding.Span())
		}
		c.validateReservedKeywords(branch.Body)
	}
	c.validateReservedKeywords(node.Else)
}

func (c *Checker) validateReservedKeywordType(ref ast.TypeRef) {
	if ref.Empty() {
		return
	}
	c.validateReservedKeywordName(ref.Name, "a type name", ref.Span())
	for _, argument := range ref.Arguments {
		c.validateReservedKeywordType(argument)
	}
	for _, alternative := range ref.Union {
		c.validateReservedKeywordType(alternative)
	}
	for _, parameter := range ref.FunctionParameters {
		c.validateReservedKeywordType(parameter)
	}
	if ref.FunctionReturn != nil {
		c.validateReservedKeywordType(*ref.FunctionReturn)
	}
}
