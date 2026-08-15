package parser

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ast"
)

func TestParseStructuredJSXExpression(t *testing.T) {
	program, diagnostics := Parse([]byte(`def view(): ReactNode
	return <>
		<Card title="TypeRB" selected={true} />
		<p>{message}</p>
	</>
end
`))
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	method, ok := program.Statements[0].(*ast.MethodStatement)
	if !ok || len(method.Body) != 1 {
		t.Fatalf("unexpected method AST: %#v", program.Statements)
	}
	returned := method.Body[0].(*ast.ReturnStatement)
	fragment, ok := returned.Value.(*ast.JSXElement)
	if !ok || !fragment.Fragment || len(fragment.Children) != 2 {
		t.Fatalf("unexpected JSX fragment: %#v", returned.Value)
	}
	card := fragment.Children[0].(*ast.JSXElement)
	if card.Name != "Card" || len(card.Attributes) != 2 || card.Attributes[0].Name != "title" || card.Attributes[1].Name != "selected" {
		t.Fatalf("unexpected JSX component: %#v", card)
	}
}

func TestParseJSXMemberComponent(t *testing.T) {
	program, diagnostics := Parse([]byte(`def view(): ReactNode
	return <Table.Row><Table.Cell>value</Table.Cell></Table.Row>
end
`))
	if len(diagnostics) > 0 {
		t.Fatal(diagnostics)
	}
	method := program.Statements[0].(*ast.MethodStatement)
	returned := method.Body[0].(*ast.ReturnStatement)
	row := returned.Value.(*ast.JSXElement)
	member, ok := row.Component.(*ast.MemberExpression)
	if !ok || member.Name != "Row" || row.Name != "Table.Row" {
		t.Fatalf("unexpected JSX member component: %#v", row)
	}
}

func TestParseRejectsMismatchedJSXClosingElement(t *testing.T) {
	_, diagnostics := Parse([]byte("def view(): Any\n\treturn <div><span /></main>\nend\n"))
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "mismatched JSX closing element") {
		t.Fatalf("expected mismatched closing element diagnostic, got %#v", diagnostics)
	}
}

func TestParseRejectsUnterminatedJSXElement(t *testing.T) {
	_, diagnostics := Parse([]byte("def view(): Any\n\treturn <div>missing\nend\n"))
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "unterminated JSX element") {
		t.Fatalf("expected unterminated JSX diagnostic, got %#v", diagnostics)
	}
}
