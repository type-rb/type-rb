package checker

import (
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

// Alias construction keeps the underlying record identity and substitutions.
// Backends and the evaluator consume the same field and default contract.
func (c *Checker) aliasRecordConstruction(typ types.Type) (RecordConstruction, bool) {
	if _, _, alias := c.aliasDefinition(typ); !alias {
		return RecordConstruction{}, false
	}
	target := c.canonicalType(c.expandAlias(typ, map[string]bool{}), c.activeTypeParameterSet())
	if target.Kind != types.Named || target.Nullable {
		return RecordConstruction{}, false
	}
	var fields []resolver.RecordField
	var parameters []string
	var targetBinding *resolver.Binding
	name := target.Name
	if target.Declaration.Name != "" {
		name = target.Declaration.Name
	}
	if record := c.records[name]; record != nil && (target.Declaration.Empty() || target.Declaration == c.authoredTypeIdentities[name]) {
		parameters = record.typeParameters
		for _, field := range record.fields {
			fields = append(fields, resolver.RecordField{Name: field.Name, Type: c.typeFromRef(field.Type), HasDefault: field.Default != nil})
		}
	} else {
		binding, found := c.resolution.ImportedTypeIdentity(target.Declaration)
		if !found {
			binding, found = c.resolution.ContractType(target.Name)
		}
		if !found {
			binding, found = c.resolution.InferredType(target.Name)
		}
		if !found || binding.Export == nil || binding.Export.Kind != resolver.RecordExport || !target.Declaration.Empty() && target.Declaration != binding.DeclarationIdentity() {
			return RecordConstruction{}, false
		}
		fields = append(fields, binding.Export.Fields...)
		parameters = binding.Export.TypeParameters
		targetBinding = &binding
		target.Declaration = binding.DeclarationIdentity()
		c.markImportUsed(binding)
	}
	if len(target.Args) != len(parameters) {
		return RecordConstruction{}, false
	}
	substitutions := typeSubstitutions(parameters, target.Args)
	for index := range fields {
		fields[index].Type = substituteType(fields[index].Type, substitutions)
		fields[index].ResultBridge = substituteNativeResultBridge(fields[index].ResultBridge, substitutions)
	}
	return RecordConstruction{Fields: fields, Declaration: target.Declaration, ResolvedType: target, TargetBinding: targetBinding}, true
}
