package typescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// Nested declarations are exported under their owned names rather than as
// properties of the enclosing module object. Preserve that distinction when
// importing their type and, where present, runtime value.
func (g *generator) importProjectNestedTypes(imported *ir.Import, nested map[string]string, importPath string) {
	if imported.Namespace && imported.Alias != "" {
		return
	}
	var names []string
	for canonical := range nested {
		names = append(names, canonical)
	}
	sort.Strings(names)
	var values, annotations []string
	for _, canonical := range names {
		local := nested[canonical]
		specifier := tsImportSpecifier(tsOwnedTypeName(canonical), tsOwnedTypeName(local))
		switch imported.SymbolKinds[canonical] {
		case "class", "enum", "enum_alias":
			values = append(values, specifier)
		case "newtype":
			if newtypeContractHasMethods(imported.TypeContracts[canonical]) {
				values = append(values, specifier)
			} else {
				annotations = append(annotations, specifier)
			}
		case "record", "interface", "type_alias":
			annotations = append(annotations, specifier)
			if imported.SymbolKinds[canonical] == "record" && imported.RecordDefaults[canonical] {
				values = append(values, tsImportSpecifier(tsRecordConstructorName(tsOwnedTypeName(canonical)), tsRecordConstructorName(tsOwnedTypeName(local))))
				if g.suspension != nil && g.suspension.RecordDefault(imported.Path, canonical) {
					values = append(values, tsImportSpecifier(tsRecordSyncConstructorName(tsOwnedTypeName(canonical)), tsRecordSyncConstructorName(tsOwnedTypeName(local))))
				}
			}
		}
	}
	if len(values) > 0 {
		g.line("import { " + strings.Join(values, ", ") + " } from " + strconv.Quote(importPath) + ";")
	}
	if len(annotations) > 0 {
		g.line("import type { " + strings.Join(annotations, ", ") + " } from " + strconv.Quote(importPath) + ";")
	}
}
