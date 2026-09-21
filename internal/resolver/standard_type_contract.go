package resolver

import (
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/stdlib"
)

// Standalone compilation has no project catalog. Generated operations may
// still return a standard type, such as a raw enum conversion's error record.
// Resolve its exact contract without introducing a source-visible import.
func standardTypeIdentity(declaration identity.Declaration) (Binding, bool) {
	definition, _, found := stdlib.LookupRuntimeExport(declaration.Name)
	if !found || definition.ModulePath != declaration.Module || definition.Source == "" {
		return Binding{}, false
	}
	program, diagnostics := parser.Parse([]byte(definition.Source))
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return Binding{}, false
		}
	}
	exports := CollectExports(program.Statements)
	exported, found := exportNamed(exports, declaration.Name)
	if !found || identityKind(exported.Kind) != declaration.Kind {
		return Binding{}, false
	}
	imported := &Import{Kind: StandardImport, Path: definition.Path, ModulePath: definition.ModulePath, Definition: definition, Exports: exports}
	return Binding{Import: imported, Name: exported.Name, Export: &exported}, true
}
