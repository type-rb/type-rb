package golang

import (
	"sort"

	"github.com/type-rb/type-rb/internal/codegen/naming"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

// Interfaces imported from another source file can share a Go package with a
// same-named class. Rename the contract by declaration identity at every use.
func analyzeGoInterfaceNames(programs []*ir.Program) map[identity.Declaration]string {
	counts := map[string]map[string]int{}
	interfaces := []identity.Declaration{}
	for _, program := range programs {
		group := goPackageGroup(program.ModulePath)
		if counts[group] == nil {
			counts[group] = map[string]int{}
		}
		var collect func([]ir.Statement)
		collect = func(statements []ir.Statement) {
			for _, statement := range statements {
				var declaration identity.Declaration
				var name string
				switch node := statement.(type) {
				case *ir.Interface:
					declaration, name = node.Declaration, node.Name
					interfaces = append(interfaces, declaration)
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
					counts[group][goDeclaredTypeName(declaration.Name, name)]++
				}
			}
		}
		collect(program.Statements)
	}
	sort.Slice(interfaces, func(i, j int) bool { return interfaces[i].Key() < interfaces[j].Key() })
	result := map[identity.Declaration]string{}
	for _, declaration := range interfaces {
		occupied := counts[goPackageGroup(declaration.Module)]
		if declaration.Empty() || occupied[goDeclaredTypeName(declaration.Name, "")] < 2 {
			continue
		}
		target := "TrbInterface_" + naming.PrivateSuffix(declaration.Key())
		for occupied[target] > 0 {
			target += "_"
		}
		occupied[target]++
		result[declaration] = target
	}
	return result
}

func (g *generator) namedDeclaration(declaration identity.Declaration, fallback string) string {
	if g.projectNames != nil {
		if name := g.projectNames.interfaces[declaration]; name != "" {
			return name
		}
	}
	return goDeclaredTypeName(declaration.Name, fallback)
}
