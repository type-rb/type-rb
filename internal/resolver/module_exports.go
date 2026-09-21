package resolver

import "github.com/type-rb/type-rb/internal/ast"

// Export collection sees every opening of a source namespace together, so a
// later opening cannot discard earlier members or their owned type contracts.
// Keep the authored AST unchanged: its statement order still governs execution.
func mergeExportModules(statements []ast.Statement) []ast.Statement {
	result := make([]ast.Statement, 0, len(statements))
	modules := map[string]*ast.ModuleStatement{}
	for _, statement := range statements {
		module, ok := statement.(*ast.ModuleStatement)
		if !ok {
			result = append(result, statement)
			continue
		}
		if previous := modules[module.Name]; previous != nil {
			previous.Body = append(previous.Body, module.Body...)
			continue
		}
		merged := *module
		merged.Body = append([]ast.Statement(nil), module.Body...)
		modules[module.Name] = &merged
		result = append(result, &merged)
	}
	return result
}
