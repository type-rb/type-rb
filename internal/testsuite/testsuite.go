// Package testsuite discovers and prepares TypeRB test declarations without
// coupling the parser or the language server to the test runner.
package testsuite

import (
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/token"
)

const PackagePath = "trb/std/test"

type Kind string

const (
	Suite Kind = "suite"
	Case  Kind = "test"
)

type Item struct {
	ID       string
	ParentID string
	Kind     Kind
	Name     string
	FullName string
	Span     token.Span
}

func IsTestFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filepath.Base(filename)), "_test.trb")
}

// Prepare validates the compiler-owned test DSL. When registrationName is
// non-empty, top-level suites move into a generated function that a temporary
// test entrypoint can call after loading every project module.
func Prepare(program *ast.Program, filename, registrationName string) ([]Item, []diagnostic.Diagnostic) {
	if program == nil || !IsTestFile(filename) {
		return nil, nil
	}
	imports := importedNames(program.Statements)
	if imports["namespace"] && !imports["describe"] && !imports["test"] {
		return nil, []diagnostic.Diagnostic{errorAt(program.Span(), "test suites require named imports from trb/std/test")}
	}

	var suites []ast.Statement
	var items []Item
	var diagnostics []diagnostic.Diagnostic
	seen := map[string]token.Span{}
	for _, statement := range program.Statements {
		expression, ok := statement.(*ast.ExpressionStatement)
		if !ok || !isCallNamed(expression.Expression, "describe", imports) {
			continue
		}
		suites = append(suites, statement)
		discoverCall(filename, expression.Expression.(*ast.CallExpression), "", "", imports, seen, &items, &diagnostics)
	}
	if len(suites) == 0 {
		diagnostics = append(diagnostics, errorAt(program.Span(), "test files must declare at least one top-level describe() suite"))
		return nil, diagnostics
	}

	for _, statement := range program.Statements {
		expression, ok := statement.(*ast.ExpressionStatement)
		if !ok {
			continue
		}
		call, ok := expression.Expression.(*ast.CallExpression)
		if ok && isCallNamed(call, "test", imports) {
			diagnostics = append(diagnostics, errorAt(call.Span(), "test() must be nested inside describe()"))
		}
	}
	if len(diagnostics) > 0 || registrationName == "" {
		return items, diagnostics
	}

	kept := make([]ast.Statement, 0, len(program.Statements)-len(suites)+1)
	for _, statement := range program.Statements {
		remove := false
		for _, suite := range suites {
			if statement == suite {
				remove = true
				break
			}
		}
		if !remove {
			kept = append(kept, statement)
		}
	}
	span := suites[0].Span()
	span.End = suites[len(suites)-1].Span().End
	kept = append(kept, &ast.MethodStatement{
		Base: ast.Base{SourceSpan: span}, Name: registrationName, Body: suites,
	})
	program.Statements = kept
	return items, nil
}

func importedNames(statements []ast.Statement) map[string]bool {
	result := map[string]bool{}
	for _, statement := range statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok || strings.TrimSuffix(imported.Path, "/index") != PackagePath {
			continue
		}
		for _, name := range imported.Symbols {
			result[name] = true
		}
		if len(imported.Symbols) == 0 {
			result["namespace"] = true
		}
	}
	return result
}

func discoverCall(filename string, call *ast.CallExpression, parentID, prefix string, imports map[string]bool, seen map[string]token.Span, items *[]Item, diagnostics *[]diagnostic.Diagnostic) {
	kind := Suite
	name := "describe"
	if isCallNamed(call, "test", imports) {
		kind, name = Case, "test"
	} else if !isCallNamed(call, "describe", imports) {
		return
	}
	if len(call.Arguments) != 1 || call.Arguments[0].Name != "" {
		*diagnostics = append(*diagnostics, errorAt(call.Span(), name+"() requires exactly one positional String literal"))
		return
	}
	literal, ok := call.Arguments[0].Value.(*ast.Literal)
	if !ok || literal.Kind != ast.StringLiteral {
		*diagnostics = append(*diagnostics, errorAt(call.Arguments[0].Value.Span(), name+"() name must be a String literal"))
		return
	}
	label := literal.Raw
	if unquoted, err := strconv.Unquote(literal.Raw); err == nil {
		label = unquoted
	}
	if label == "" {
		*diagnostics = append(*diagnostics, errorAt(literal.Span(), name+"() name cannot be empty"))
		return
	}
	if call.Block == nil || len(call.Block.Parameters) != 0 {
		*diagnostics = append(*diagnostics, errorAt(call.Span(), name+"() requires a parameterless block"))
		return
	}
	fullName := label
	if prefix != "" {
		fullName = prefix + " / " + label
	}
	if previous, duplicate := seen[fullName]; duplicate {
		item := errorAt(literal.Span(), fmt.Sprintf("test name %q is already declared", fullName))
		item.Related = []diagnostic.RelatedInformation{{Message: "first declaration", Location: diagnostic.Location{Path: filename, Span: previous}}}
		*diagnostics = append(*diagnostics, item)
	} else {
		seen[fullName] = literal.Span()
	}
	id := fmt.Sprintf("%s:%d", filepath.Clean(filename), call.Span().Start.Offset)
	*items = append(*items, Item{ID: id, ParentID: parentID, Kind: kind, Name: label, FullName: fullName, Span: call.Span()})
	if kind == Case {
		for _, statement := range call.Block.Body {
			if expression, ok := statement.(*ast.ExpressionStatement); ok {
				if nested, ok := expression.Expression.(*ast.CallExpression); ok && (isCallNamed(nested, "describe", imports) || isCallNamed(nested, "test", imports)) {
					*diagnostics = append(*diagnostics, errorAt(nested.Span(), "describe() and test() cannot be nested inside test()"))
				}
			}
		}
		return
	}
	children := 0
	for _, statement := range call.Block.Body {
		expression, ok := statement.(*ast.ExpressionStatement)
		if !ok {
			if _, harmless := statement.(*ast.CommentStatement); !harmless {
				if _, blank := statement.(*ast.BlankStatement); !blank {
					*diagnostics = append(*diagnostics, errorAt(statement.Span(), "describe() may contain only nested describe() or test() declarations"))
				}
			}
			continue
		}
		nested, ok := expression.Expression.(*ast.CallExpression)
		if !ok || (!isCallNamed(nested, "describe", imports) && !isCallNamed(nested, "test", imports)) {
			*diagnostics = append(*diagnostics, errorAt(statement.Span(), "describe() may contain only nested describe() or test() declarations"))
			continue
		}
		children++
		discoverCall(filename, nested, id, fullName, imports, seen, items, diagnostics)
	}
	if children == 0 {
		*diagnostics = append(*diagnostics, errorAt(call.Span(), "describe() must contain at least one test() or nested describe()"))
	}
}

func isCallNamed(expression ast.Expression, name string, imports map[string]bool) bool {
	call, ok := expression.(*ast.CallExpression)
	if !ok || call.Block == nil {
		return false
	}
	identifier, direct := call.Callee.(*ast.Identifier)
	return direct && imports[name] && identifier.Name == name
}

func errorAt(span token.Span, message string) diagnostic.Diagnostic {
	return diagnostic.Diagnostic{Code: diagnostic.TypeError, Severity: diagnostic.Error, Message: message, Span: span}
}

func Sorted(items []Item) []Item {
	result := append([]Item(nil), items...)
	sort.SliceStable(result, func(i, j int) bool { return result[i].Span.Start.Offset < result[j].Span.Start.Offset })
	return result
}
