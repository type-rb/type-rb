package resolver

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/types"
)

// WithCheckedValues publishes checked constant types without mutating a cached
// resolution or its syntax-derived catalog. Nominal identities were established
// in the defining checker; consumers must not infer them from import spellings.
func (r Result) WithCheckedValues(values map[identity.Declaration]types.Type) Result {
	if len(values) == 0 {
		return r
	}
	modules := map[string]bool{}
	for declaration := range values {
		modules[declaration.Module] = true
	}
	imports := map[*Import]*Import{}
	var copyExport func(Export, string) Export
	copyExport = func(exported Export, module string) Export {
		if exported.Kind == ValueExport {
			if typ, ok := values[identity.Declaration{Module: module, Name: exported.Name, Kind: identity.Value}]; ok {
				exported.Type = typ
			}
		}
		if exported.Members != nil {
			members := make(map[string]Member, len(exported.Members))
			for name, member := range exported.Members {
				if member.Kind == ValueExport && member.Class {
					declaration := identity.Declaration{Module: module, Name: identity.Qualify(exported.Name, name), Kind: identity.Value}
					if typ, ok := values[declaration]; ok {
						member.Type = typ
					}
				}
				members[name] = member
			}
			exported.Members = members
		}
		if exported.Nested != nil {
			nested := make(map[string]Export, len(exported.Nested))
			for name, value := range exported.Nested {
				nested[name] = copyExport(value, module)
			}
			exported.Nested = nested
		}
		return exported
	}
	copyImport := func(imported *Import) *Import {
		if imported == nil {
			return nil
		}
		if !modules[imported.RuntimePath()] {
			return imported
		}
		if previous := imports[imported]; previous != nil {
			return previous
		}
		result := *imported
		result.Exports = make(map[string]Export, len(imported.Exports))
		for name, exported := range imported.Exports {
			result.Exports[name] = copyExport(exported, imported.RuntimePath())
		}
		imports[imported] = &result
		return &result
	}
	copyBindings := func(bindings map[string]Binding) map[string]Binding {
		result := make(map[string]Binding, len(bindings))
		for name, binding := range bindings {
			previous := binding.Import
			binding.Import = copyImport(binding.Import)
			if binding.Export != nil && binding.Import != nil && binding.Import != previous {
				exported := copyExport(*binding.Export, binding.Import.RuntimePath())
				binding.Export = &exported
				if binding.Member != nil {
					if member, found := exported.Members[binding.Member.Name]; found {
						binding.Member = &member
					}
				}
			}
			result[name] = binding
		}
		return result
	}
	result := r
	result.Imports = make(map[*ast.ImportStatement]*Import, len(r.Imports))
	for node, imported := range r.Imports {
		result.Imports[node] = copyImport(imported)
	}
	result.Activations = make(map[*ast.ActivateStatement]*Import, len(r.Activations))
	for node, imported := range r.Activations {
		result.Activations[node] = copyImport(imported)
	}
	result.Packages = make(map[string]*Import, len(r.Packages))
	for name, imported := range r.Packages {
		result.Packages[name] = copyImport(imported)
	}
	result.Capabilities = make([]*Import, len(r.Capabilities))
	for index, imported := range r.Capabilities {
		result.Capabilities[index] = copyImport(imported)
	}
	result.Symbols = copyBindings(r.Symbols)
	result.GeneratedSymbols = copyBindings(r.GeneratedSymbols)
	return result
}
