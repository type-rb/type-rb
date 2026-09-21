package checker

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

func (c *Checker) uniqueNestedRecord(name string) identity.Declaration {
	// An explicitly imported name takes precedence over the nested shorthand.
	if _, imported := c.resolution.Symbols[name]; imported {
		return identity.Declaration{}
	}
	if declaration := c.uniqueAuthoredTypes[name]; declaration.Kind == identity.Record {
		return declaration
	}
	return identity.Declaration{}
}

func (c *Checker) localRecord(typ types.Type) *recordInfo {
	typ = c.canonicalType(typ, c.activeTypeParameterSet())
	if typ.Declaration.Kind != identity.Record || typ.Declaration.Module != c.result.Program.ModulePath {
		return nil
	}
	return c.records[typ.Declaration.Name]
}

// Field annotations belong to the record, even when construction or access
// occurs in a namespace with different declarations or type parameters.
func (c *Checker) recordFieldType(record *recordInfo, field *ast.RecordFieldStatement) types.Type {
	popOwner := c.pushActiveTypeOwner(record.declaration.Name)
	defer popOwner()
	parameters := map[string]bool{}
	for _, name := range record.typeParameters {
		parameters[name] = true
	}
	return c.typeFromRefWithParameters(field.Type, parameters)
}

func (c *Checker) localRecordFields(record *recordInfo) []resolver.RecordField {
	fields := make([]resolver.RecordField, len(record.fields))
	for index, field := range record.fields {
		fields[index] = resolver.RecordField{
			Name: field.Name, JSONName: checkerRecordJSONName(field),
			Type: c.recordFieldType(record, field), HasDefault: field.Default != nil,
		}
	}
	return fields
}

func (c *Checker) codecRecord(typ types.Type) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	return c.codecRecordResolved(typ, false)
}

func (c *Checker) codecRecordResolved(typ types.Type, catalogContext bool) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	if !typ.Declaration.Empty() {
		if record := c.localRecord(typ); record != nil {
			return c.localRecordFields(record), c.result.Program.ModulePath, nil, true
		}
		for _, lookup := range []func(identity.Declaration) (resolver.Binding, bool){c.resolution.ImportedTypeIdentity, c.resolution.ContractTypeIdentity} {
			if binding, found := lookup(typ.Declaration); found {
				return c.resolvedRecordFields(binding)
			}
		}
		return nil, "", nil, false
	}
	if catalogContext {
		if binding, ok := c.resolution.CatalogType(typ.Name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
			return c.resolvedRecordFields(binding)
		}
	}
	if binding, ok := c.resolution.ImportedType(typ.Name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
		return c.resolvedRecordFields(binding)
	}
	if record := c.localRecord(typ); record != nil {
		return c.localRecordFields(record), c.result.Program.ModulePath, nil, true
	}
	for _, lookup := range []func(string) (resolver.Binding, bool){c.resolution.InferredType, c.resolution.ContractType} {
		if binding, ok := lookup(typ.Name); ok && binding.Export != nil && binding.Export.Kind == resolver.RecordExport {
			return c.resolvedRecordFields(binding)
		}
	}
	return nil, "", nil, false
}

func (c *Checker) resolvedRecordFields(binding resolver.Binding) ([]resolver.RecordField, string, *resolver.Binding, bool) {
	if binding.Export == nil || binding.Export.Kind != resolver.RecordExport {
		return nil, "", nil, false
	}
	parameters := map[string]bool{}
	for _, name := range binding.Export.TypeParameters {
		parameters[name] = true
	}
	fields := append([]resolver.RecordField(nil), binding.Export.Fields...)
	for index := range fields {
		fields[index].Type = c.canonicalContractType(fields[index].Type, parameters, binding.Import)
	}
	return fields, binding.Import.RuntimePath(), &binding, true
}
