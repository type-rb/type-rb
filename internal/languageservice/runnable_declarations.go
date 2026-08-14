package languageservice

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
)

type RunnableDeclarationKind string

const RunnableDeclarationMain RunnableDeclarationKind = "main"

// RunnableDeclaration identifies a source declaration that an editor can run.
// Range covers the authored declaration name rather than its complete body.
type RunnableDeclaration struct {
	Kind  RunnableDeclarationKind
	Range OffsetRange
}

// RunnableDeclarations returns executable declarations from the lossless
// syntax tree. It remains available while unrelated project code has errors.
func RunnableDeclarations(source string) []RunnableDeclaration {
	program, _ := parser.Parse([]byte(source))
	result := []RunnableDeclaration{}
	for _, statement := range program.Statements {
		method, ok := statement.(*ast.MethodStatement)
		if !ok || !runnableMain(method) {
			continue
		}
		fallback := tokenOffsetRange(method.Span())
		result = append(result, RunnableDeclaration{
			Kind:  RunnableDeclarationMain,
			Range: declarationNameRange(program.Tokens, method.Span(), method.Name, fallback),
		})
	}
	return result
}

func runnableMain(method *ast.MethodStatement) bool {
	return method.Name == "main" &&
		!method.Class &&
		len(method.TypeParameters) == 0 &&
		len(method.Parameters) == 0 &&
		method.ReturnType.Empty() &&
		method.Fails.Empty()
}
