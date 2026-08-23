package languageservice

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/token"
)

type DocumentSymbolKind string

const (
	DocumentSymbolModule     DocumentSymbolKind = "module"
	DocumentSymbolClass      DocumentSymbolKind = "class"
	DocumentSymbolMethod     DocumentSymbolKind = "method"
	DocumentSymbolField      DocumentSymbolKind = "field"
	DocumentSymbolEnum       DocumentSymbolKind = "enum"
	DocumentSymbolInterface  DocumentSymbolKind = "interface"
	DocumentSymbolFunction   DocumentSymbolKind = "function"
	DocumentSymbolVariable   DocumentSymbolKind = "variable"
	DocumentSymbolConstant   DocumentSymbolKind = "constant"
	DocumentSymbolEnumMember DocumentSymbolKind = "enum_member"
	DocumentSymbolRecord     DocumentSymbolKind = "record"
	DocumentSymbolType       DocumentSymbolKind = "type"
)

// DocumentSymbol describes one declaration in a source file without exposing
// an editor protocol. Range covers the declaration and SelectionRange covers
// the authored name.
type DocumentSymbol struct {
	Name           string
	Detail         string
	Kind           DocumentSymbolKind
	Range          OffsetRange
	SelectionRange OffsetRange
	Children       []DocumentSymbol
}

// DocumentSymbols returns the structural outline available from a single
// source file. It intentionally uses the lossless syntax tree so an editor can
// keep showing an outline while the project has type errors.
func DocumentSymbols(source string) []DocumentSymbol {
	program, _ := parser.Parse([]byte(source))
	return collectDocumentSymbols(program.Statements, program.Tokens, false)
}

func collectDocumentSymbols(statements []ast.Statement, tokens []token.Token, member bool) []DocumentSymbol {
	result := []DocumentSymbol{}
	for _, statement := range statements {
		var symbol DocumentSymbol
		switch node := statement.(type) {
		case *ast.ClassStatement:
			symbol = structuralSymbol(node.Name, "class", DocumentSymbolClass, node.Span(), tokens, node.Body)
		case *ast.RecordStatement:
			symbol = structuralSymbol(node.Name, "record", DocumentSymbolRecord, node.Span(), tokens, node.Body)
		case *ast.EnumStatement:
			symbol = structuralSymbol(node.Name, "enum", DocumentSymbolEnum, node.Span(), tokens, node.Body)
		case *ast.ModuleStatement:
			symbol = structuralSymbol(node.Name, "module", DocumentSymbolModule, node.Span(), tokens, node.Body)
		case *ast.InterfaceStatement:
			body := make([]ast.Statement, 0, len(node.Methods))
			for _, method := range node.Methods {
				body = append(body, method)
			}
			symbol = structuralSymbol(node.Name, "interface", DocumentSymbolInterface, node.Span(), tokens, body)
		case *ast.TypeAliasStatement:
			symbol = leafDocumentSymbol(node.Name, "alias "+documentTypeDetail(node.Target), DocumentSymbolType, node.Span(), tokens)
		case *ast.NewtypeStatement:
			symbol = leafDocumentSymbol(node.Name, "newtype "+documentTypeDetail(node.Target), DocumentSymbolType, node.Span(), tokens)
		case *ast.MethodStatement:
			kind := DocumentSymbolFunction
			if member {
				kind = DocumentSymbolMethod
			}
			symbol = leafDocumentSymbol(node.Name, methodDocumentDetail(node), kind, node.Span(), tokens)
		case *ast.FieldStatement:
			symbol = leafDocumentSymbol(node.Name, documentTypeDetail(node.Type), DocumentSymbolField, node.Span(), tokens)
		case *ast.RecordFieldStatement:
			symbol = leafDocumentSymbol(node.Name, documentTypeDetail(node.Type), DocumentSymbolField, node.Span(), tokens)
		case *ast.EnumMemberStatement:
			symbol = leafDocumentSymbol(node.Name, "enum member", DocumentSymbolEnumMember, node.Span(), tokens)
		case *ast.VariableStatement:
			kind := DocumentSymbolVariable
			if node.Constant {
				kind = DocumentSymbolConstant
			}
			detail := documentTypeDetail(node.Type)
			if detail == "" {
				detail = "inferred"
			}
			symbol = leafDocumentSymbol(node.Name, detail, kind, node.Span(), tokens)
		default:
			continue
		}
		result = append(result, symbol)
	}
	return result
}

func structuralSymbol(name, detail string, kind DocumentSymbolKind, span token.Span, tokens []token.Token, body []ast.Statement) DocumentSymbol {
	symbol := leafDocumentSymbol(name, detail, kind, span, tokens)
	symbol.Children = collectDocumentSymbols(body, tokens, true)
	return symbol
}

func leafDocumentSymbol(name, detail string, kind DocumentSymbolKind, span token.Span, tokens []token.Token) DocumentSymbol {
	range_ := tokenOffsetRange(span)
	return DocumentSymbol{
		Name: name, Detail: detail, Kind: kind, Range: range_,
		SelectionRange: declarationNameRange(tokens, span, name, range_),
	}
}

func methodDocumentDetail(method *ast.MethodStatement) string {
	parameters := make([]string, 0, len(method.Parameters))
	for _, parameter := range method.Parameters {
		parameters = append(parameters, parameter.Name+": "+documentTypeDetail(parameter.Type))
	}
	detail := "(" + strings.Join(parameters, ", ") + ")"
	returnType := normalizeDocumentType(method.ReturnType)
	if !method.Fails.Empty() {
		returnType = documentResultType(returnType, normalizeDocumentType(method.Fails))
	}
	if text := returnType.String(); text != "" {
		detail += ": " + text
	}
	return detail
}

func documentTypeDetail(input ast.TypeRef) string {
	return normalizeDocumentType(input).String()
}

func normalizeDocumentType(input ast.TypeRef) ast.TypeRef {
	result := input
	result.Arguments = make([]ast.TypeRef, len(input.Arguments))
	for index, argument := range input.Arguments {
		result.Arguments[index] = normalizeDocumentType(argument)
	}
	result.Union = make([]ast.TypeRef, len(input.Union))
	for index, alternative := range input.Union {
		result.Union[index] = normalizeDocumentType(alternative)
	}
	result.FunctionParameters = make([]ast.TypeRef, len(input.FunctionParameters))
	for index, parameter := range input.FunctionParameters {
		result.FunctionParameters[index] = normalizeDocumentType(parameter)
	}
	if input.FunctionReturn != nil {
		valueType := normalizeDocumentType(*input.FunctionReturn)
		if input.FunctionFails != nil {
			valueType = documentResultType(valueType, normalizeDocumentType(*input.FunctionFails))
		}
		result.FunctionReturn = &valueType
		result.FunctionFails = nil
	}
	return result
}

func documentResultType(success, failure ast.TypeRef) ast.TypeRef {
	if success.Name == "Result" && len(success.Arguments) == 2 {
		return success
	}
	if success.Empty() || success.Name == "Void" {
		success = ast.TypeRef{Name: "Unit"}
	}
	return ast.TypeRef{Name: "Result", Arguments: []ast.TypeRef{success, failure}}
}

func tokenOffsetRange(span token.Span) OffsetRange {
	return OffsetRange{Start: span.Start.Offset, End: span.End.Offset}
}

func declarationNameRange(tokens []token.Token, span token.Span, name string, fallback OffsetRange) OffsetRange {
	for _, item := range tokens {
		if item.Span.Start.Offset < span.Start.Offset || item.Span.End.Offset > span.End.Offset {
			continue
		}
		if item.Lexeme == name {
			return tokenOffsetRange(item.Span)
		}
	}
	return fallback
}
