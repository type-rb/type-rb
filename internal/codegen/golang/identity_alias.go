package golang

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Go cannot declare an alias whose target is a bare type parameter. Preserve
// the checked declaration identity, but emit its selected argument at each use.
// The semantic target also covers aliases that reach a parameter through other
// transparent aliases; nullable and container targets remain Go declarations.
type goIdentityAliases map[identity.Declaration]int

func analyzeGoIdentityAliases(programs []*ir.Program) goIdentityAliases {
	result := goIdentityAliases{}
	var collect func([]ir.Statement)
	collect = func(statements []ir.Statement) {
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ir.Module:
				collect(node.Body)
			case *ir.TypeAlias:
				target := node.Target
				if node.Declaration.Empty() || target.Kind != types.Named || !target.Declaration.Empty() || target.Nullable || len(target.Args) != 0 {
					continue
				}
				for index, parameter := range node.TypeParameters {
					if target.Name == parameter {
						result[node.Declaration] = index
						break
					}
				}
			}
		}
	}
	for _, program := range programs {
		collect(program.Statements)
	}
	return result
}

func (aliases goIdentityAliases) expand(value types.Type) types.Type {
	for value.Kind == types.Named {
		index, ok := aliases[value.Declaration]
		if !ok || index >= len(value.Args) {
			break
		}
		argument := value.Args[index]
		argument.Nullable = argument.Nullable || value.Nullable
		argument.Readonly = argument.Readonly || value.Readonly
		value = argument
	}
	return value
}
