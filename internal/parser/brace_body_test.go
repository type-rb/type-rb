package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestBraceBodiesPreserveStatementsCommentsAndOrigins(t *testing.T) {
	for _, call := range []string{"[1].each", "service.visit()"} {
		source := "def main()\n" + call + " { |value|\n# body comment\nlocal := value\nif local > 0\nputs(local)\nend\n}\nend\n"
		program, diagnostics := Parse([]byte(source))
		if len(diagnostics) != 0 {
			t.Fatal(diagnostics)
		}
		method := program.Statements[0].(*ast.MethodStatement)
		expression := method.Body[0].(*ast.ExpressionStatement).Expression
		var block *ast.BlockExpression
		switch node := expression.(type) {
		case *ast.IterationExpression:
			block = node.Block
		case *ast.CallExpression:
			block = node.Block
		default:
			t.Fatalf("unexpected expression %T", expression)
		}
		if len(block.Body) != 3 {
			t.Fatalf("body=%#v", block.Body)
		}
		if _, ok := block.Body[0].(*ast.CommentStatement); !ok {
			t.Fatalf("comment=%T", block.Body[0])
		}
		condition, ok := block.Body[2].(*ast.IfStatement)
		if !ok || condition.Span().Start.Line != 5 || condition.Then[0].Span().Start.Line != 6 {
			t.Fatalf("lost statement or source position: %#v", block.Body[2])
		}
	}
}

func TestBraceBodyRejectsMissingInnerTerminator(t *testing.T) {
	_, diagnostics := Parse([]byte("def main()\n[1].each { |value|\nif value > 0\nputs(value)\n}\nend\n"))
	if len(diagnostics) == 0 || diagnostics[0].Span.Start.Line != 5 {
		t.Fatalf("expected diagnostic at closing brace, got %v", diagnostics)
	}
}
