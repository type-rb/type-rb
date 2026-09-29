package golang

import (
	"sort"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

// Emission groups combine independent TypeRB namespaces. Disambiguate nominal
// declarations by identity without changing their checked source names.
func analyzeGoTypeNames(programs []*ir.Program) map[identity.Declaration]string {
	layout := emissionLayout(programs)
	counts := map[string]map[string]int{}
	declarations := []identity.Declaration{}
	concrete := map[string]map[string]int{}
	for _, program := range programs {
		group := layout.directory(program.ModulePath)
		if counts[group] == nil {
			counts[group] = map[string]int{}
			concrete[group] = map[string]int{}
		}
		var collect func([]ir.Statement)
		collect = func(statements []ir.Statement) {
			for _, statement := range statements {
				var declaration identity.Declaration
				var name string
				switch node := statement.(type) {
				case *ir.Interface:
					declaration, name = node.Declaration, node.Name
				case *ir.Class:
					declaration, name = node.Declaration, node.Name
				case *ir.Record:
					declaration, name = node.Declaration, node.Name
				case *ir.Enum:
					declaration, name = node.Declaration, node.Name
				case *ir.TypeAlias:
					declaration, name = node.Declaration, node.Name
				case *ir.Newtype:
					declaration, name = node.Declaration, node.Name
				case *ir.Module:
					collect(node.Body)
				}
				if name != "" {
					candidate := goDeclaredTypeName(declaration.Name, name)
					counts[group][candidate]++
					declarations = append(declarations, declaration)
					if declaration.Kind != identity.Interface {
						concrete[group][candidate]++
					}
				}
			}
		}
		collect(program.Statements)
	}
	sort.Slice(declarations, func(i, j int) bool { return declarations[i].Key() < declarations[j].Key() })
	result := map[identity.Declaration]string{}
	for _, declaration := range declarations {
		occupied := counts[layout.directory(declaration.Module)]
		candidate := goDeclaredTypeName(declaration.Name, "")
		if declaration.Empty() {
			continue
		}
		result[declaration] = candidate
		if occupied[candidate] < 2 || declaration.Kind != identity.Interface && concrete[layout.directory(declaration.Module)][candidate] < 2 {
			continue
		}
		prefix := "TrbType"
		if declaration.Kind == identity.Interface {
			prefix = "TrbInterface_"
		}
		target := prefix + naming.PrivateSuffix(declaration.Key())
		for occupied[target] > 0 {
			target += "X"
		}
		occupied[target]++
		result[declaration] = target
	}
	return result
}

func (g *generator) namedDeclaration(declaration identity.Declaration, fallback string) string {
	if g.projectNames != nil {
		if name := g.projectNames.types[declaration]; name != "" {
			return name
		}
	}
	return goDeclaredTypeName(declaration.Name, fallback)
}
