package checker

import (
	"strings"

	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

// A resolved alias belongs to its declaring module. Looking it up again by its
// leaf spelling can select a different import or reinterpret a type argument.
func (c *Checker) aliasDefinition(typ types.Type) ([]string, types.Type, bool) {
	if typ.Kind != types.Named {
		return nil, types.Type{}, false
	}
	if typ.Declaration.Empty() && c.activeTypeParameters[typ.Name] > 0 {
		return nil, types.Type{}, false
	}
	if alias := c.aliases[typ.Name]; alias != nil && (typ.Declaration.Empty() || typ.Declaration == c.result.Declarations[alias.statement]) {
		parameters := map[string]bool{}
		for _, parameter := range alias.typeParameters {
			parameters[parameter] = true
		}
		owner := ""
		declaration := c.result.Declarations[alias.statement]
		if separator := strings.LastIndex(declaration.Name, "::"); separator >= 0 {
			owner = declaration.Name[:separator]
		}
		popOwner := c.pushActiveTypeOwner(owner)
		target := c.canonicalType(alias.target, parameters)
		popOwner()
		return alias.typeParameters, target, true
	}
	if !typ.Declaration.Empty() {
		if binding, ok := c.resolution.ImportedTypeIdentity(typ.Declaration); ok && binding.Export != nil && binding.Export.Kind == resolver.TypeAliasExport {
			parameters := map[string]bool{}
			for _, parameter := range binding.Export.TypeParameters {
				parameters[parameter] = true
			}
			return binding.Export.TypeParameters, c.canonicalContractType(binding.Export.AliasTarget, parameters, binding.Import), true
		}
		return nil, types.Type{}, false
	}
	for _, lookup := range []func(string) (resolver.Binding, bool){c.resolution.ImportedType, c.resolution.InferredType} {
		if binding, ok := lookup(typ.Name); ok && binding.Export != nil && binding.Export.Kind == resolver.TypeAliasExport {
			return binding.Export.TypeParameters, binding.Export.AliasTarget, true
		}
	}
	if exported, exists := c.resolution.CompilerOwnedType(typ.Name); exists && exported.Kind == resolver.TypeAliasExport {
		return exported.TypeParameters, exported.AliasTarget, true
	}
	if exported, exists := c.resolution.ContractTypeAlias(typ.Name); exists {
		return exported.TypeParameters, exported.AliasTarget, true
	}
	return nil, types.Type{}, false
}
