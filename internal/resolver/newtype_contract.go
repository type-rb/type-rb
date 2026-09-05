package resolver

import (
	"path"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

// Bind nominal signatures in their defining module before callers see them.
// Include the scoped File/Dir contracts: an imported borrow parameter must retain
// its exact resource identity. A source import alias is not a global type name;
// another module's same-named newtype must not change a returned value's
// methods or construction.
func canonicalizeNewtypeContracts(catalog *Catalog) {
	for _, module := range catalog.Modules {
		scope := map[string]Binding{}
		for _, statement := range module.Program.Statements {
			node, ok := statement.(*ast.ImportStatement)
			if !ok {
				continue
			}
			candidates, _ := ProjectImportModuleCandidates(node.Path)
			for _, candidate := range candidates {
				candidate = CanonicalPackageImport(candidate, module.PackageAliases)
				dependency := catalog.Modules[candidate]
				if dependency == nil {
					dependency = catalog.Modules[path.Join(candidate, "index")]
				}
				if dependency == nil {
					continue
				}
				names := node.Symbols
				if len(names) == 0 {
					root, diagnostics := selectBareRoot(node, &Import{Path: dependency.Path, Exports: dependency.Exports})
					if len(diagnostics) == 0 {
						names = []string{root}
					}
				}
				for _, name := range names {
					exported, found := exportNamed(dependency.Exports, name)
					if !found || exported.Kind != NewtypeExport && !catalogScopedResourceExport(dependency, exported) {
						continue
					}
					local := name
					if alias := node.SymbolAliases[name]; alias != "" {
						local = alias
					} else if len(node.Symbols) == 0 && node.Alias != "" {
						local = node.Alias
					}
					scope[local] = catalogNewtypeBinding(dependency, exported)
				}
				break
			}
		}
		for name, exported := range flattenExports(module.Exports) {
			if exported.Kind == NewtypeExport {
				scope[name] = catalogNewtypeBinding(module, exported)
			}
		}
		for name, exported := range module.Exports {
			module.Exports[name] = canonicalNewtypeExport(exported, scope, nil)
		}
	}
}

func catalogScopedResourceExport(module *Module, exported Export) bool {
	if !module.CompilerOwned {
		return false
	}
	binding := catalogNewtypeBinding(module, exported)
	return binding.DeclarationIdentity() == stdlib.FileResourceType().Declaration || binding.DeclarationIdentity() == stdlib.DirResourceType().Declaration
}

func catalogNewtypeBinding(module *Module, exported Export) Binding {
	kind := ProjectImport
	if module.CompilerOwned {
		kind = StandardImport
	} else if module.Official {
		kind = OfficialImport
	}
	return Binding{Import: &Import{Kind: kind, Path: module.Path, ModulePath: module.Path, Filename: module.Filename, Exports: module.Exports}, Name: exported.Name, Export: &exported}
}

func canonicalNewtypeExport(exported Export, scope map[string]Binding, outerParameters []string) Export {
	parameters := append(append([]string(nil), outerParameters...), exported.TypeParameters...)
	qualify := func(typ types.Type) types.Type { return canonicalNewtypeType(typ, scope, parameters) }
	exported.Type = qualify(exported.Type)
	exported.NewtypeTarget = qualify(exported.NewtypeTarget)
	exported.AliasTarget = qualify(exported.AliasTarget)
	exported.EnumRawType = qualify(exported.EnumRawType)
	exported.Parameters = append([]callsignature.Parameter(nil), exported.Parameters...)
	for index := range exported.Parameters {
		exported.Parameters[index].Type = qualify(exported.Parameters[index].Type)
	}
	exported.Fields = append([]RecordField(nil), exported.Fields...)
	for index := range exported.Fields {
		exported.Fields[index].Type = qualify(exported.Fields[index].Type)
	}
	exported.Interfaces = append([]types.Type(nil), exported.Interfaces...)
	for index := range exported.Interfaces {
		exported.Interfaces[index] = qualify(exported.Interfaces[index])
	}
	exported.EnumVariants = append([]EnumVariant(nil), exported.EnumVariants...)
	for index := range exported.EnumVariants {
		fields := append([]RecordField(nil), exported.EnumVariants[index].Fields...)
		for field := range fields {
			fields[field].Type = qualify(fields[field].Type)
		}
		exported.EnumVariants[index].Fields = fields
	}
	exported.Members = cloneMembers(exported.Members)
	for name, member := range exported.Members {
		memberParameters := append(append([]string(nil), parameters...), member.TypeParameters...)
		member.Type = canonicalNewtypeType(member.Type, scope, memberParameters)
		member.Parameters = append([]callsignature.Parameter(nil), member.Parameters...)
		for index := range member.Parameters {
			member.Parameters[index].Type = canonicalNewtypeType(member.Parameters[index].Type, scope, memberParameters)
		}
		exported.Members[name] = member
	}
	exported.Nested = cloneExports(exported.Nested)
	for name, nested := range exported.Nested {
		exported.Nested[name] = canonicalNewtypeExport(nested, scope, parameters)
	}
	return exported
}

func canonicalNewtypeType(typ types.Type, scope map[string]Binding, parameters []string) types.Type {
	typ.Args = append([]types.Type(nil), typ.Args...)
	for index := range typ.Args {
		typ.Args[index] = canonicalNewtypeType(typ.Args[index], scope, parameters)
	}
	if typ.Kind != types.Named || !typ.Declaration.Empty() {
		return typ
	}
	for _, parameter := range parameters {
		if typ.Name == parameter {
			return typ
		}
	}
	if binding, found := scope[typ.Name]; found {
		typ.Name = binding.Export.Name
		typ.Declaration = binding.DeclarationIdentity()
	}
	return typ
}
