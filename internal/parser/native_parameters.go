package parser

import "github.com/type-rb/type-rb/internal/ast"

// NormalizeRubyNativeParameters applies Ruby's untyped keyword-parameter
// interpretation only after an explicit Ruby-native import has selected that
// syntax. Portable parsing remains deterministic and keeps the candidate
// metadata separate until this point.
func NormalizeRubyNativeParameters(statements []ast.Statement) {
	normalize := func(parameters []ast.Parameter) {
		for index := range parameters {
			parameter := &parameters[index]
			if !parameter.NativeKeyword {
				continue
			}
			parameter.Keyword = true
			parameter.Type = ast.TypeRef{}
			parameter.Default = parameter.NativeKeywordDefault
		}
	}
	var visit func([]ast.Statement)
	visit = func(items []ast.Statement) {
		for _, statement := range items {
			switch node := statement.(type) {
			case *ast.ClassStatement:
				visit(node.Body)
			case *ast.RecordStatement:
				visit(node.Body)
			case *ast.NewtypeStatement:
				visit(node.Body)
			case *ast.EnumStatement:
				for _, member := range node.Body {
					if payload, ok := member.(*ast.EnumMemberStatement); ok {
						normalize(payload.Parameters)
					}
				}
				visit(node.Body)
			case *ast.ModuleStatement:
				visit(node.Body)
			case *ast.InterfaceStatement:
				for _, method := range node.Methods {
					normalize(method.Parameters)
				}
			case *ast.MethodStatement:
				normalize(node.Parameters)
			case *ast.NativeBlock:
				visit(node.Body)
			}
		}
	}
	visit(statements)
}
