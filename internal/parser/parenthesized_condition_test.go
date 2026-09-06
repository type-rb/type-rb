package parser

import (
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParenthesizedOperandPreservesCompleteCondition(t *testing.T) {
	for _, keyword := range []string{"if", "while"} {
		for _, condition := range []string{"(false) || ready()", "(true) && ready()", "((false) || ready())"} {
			t.Run(keyword+"/"+condition, func(t *testing.T) {
				program, diagnostics := Parse([]byte(keyword + " " + condition + "\nend\n"))
				if len(diagnostics) != 0 {
					t.Fatal(diagnostics)
				}
				var expression ast.Expression
				switch statement := program.Statements[0].(type) {
				case *ast.IfStatement:
					expression = statement.Condition
				case *ast.WhileStatement:
					expression = statement.Condition
				}
				binary, ok := expression.(*ast.BinaryExpression)
				if !ok {
					t.Fatalf("complete condition was lost: %#v", expression)
				}
				if _, ok := binary.Right.(*ast.CallExpression); !ok {
					t.Fatalf("right call was lost: %#v", binary.Right)
				}
			})
		}
	}
}
