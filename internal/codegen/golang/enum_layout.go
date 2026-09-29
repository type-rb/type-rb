package golang

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Only enum payload edges that close an inline value cycle need indirection.
// Declaration identities and substituted field types keep imported aliases and
// generic record wrappers independent from their authored spellings.
type goEnumLayout map[identity.Declaration]map[string]bool

func analyzeGoEnumLayout(programs []*ir.Program) goEnumLayout {
	declarations := map[identity.Declaration]ir.InlineDeclaration{}
	enums := []*ir.Enum{}
	var collect func([]ir.Statement)
	collect = func(statements []ir.Statement) {
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ir.Module:
				collect(node.Body)
			case *ir.Record:
				entry := ir.InlineDeclaration{Parameters: node.TypeParameters}
				for _, item := range node.Body {
					if field, ok := item.(*ir.RecordField); ok {
						entry.Fields = append(entry.Fields, field.Type)
					}
				}
				declarations[node.Declaration] = entry
			case *ir.Enum:
				enums = append(enums, node)
				entry := ir.InlineDeclaration{Parameters: node.TypeParameters}
				for _, item := range node.Body {
					if variant, ok := item.(*ir.EnumMember); ok {
						for _, field := range variant.Fields {
							entry.Fields = append(entry.Fields, field.Type)
						}
					}
				}
				declarations[node.Declaration] = entry
			case *ir.TypeAlias:
				declarations[node.Declaration] = ir.InlineDeclaration{node.TypeParameters, []types.Type{node.Target}}
			case *ir.Newtype:
				declarations[node.Declaration] = ir.InlineDeclaration{Fields: []types.Type{node.Target}}
			}
		}
	}
	for _, program := range programs {
		collect(program.Statements)
	}
	result := goEnumLayout{}
	for _, enum := range enums {
		for _, item := range enum.Body {
			variant, ok := item.(*ir.EnumMember)
			if !ok {
				continue
			}
			for _, field := range variant.Fields {
				if ir.InlineReaches(field.Type, enum.Declaration, declarations, map[identity.Declaration]bool{}) {
					if result[enum.Declaration] == nil {
						result[enum.Declaration] = map[string]bool{}
					}
					result[enum.Declaration][variant.Name+"\x00"+field.Name] = true
				}
			}
		}
	}
	return result
}

func (layout goEnumLayout) indirect(owner identity.Declaration, variant, field string) bool {
	return layout[owner][variant+"\x00"+field]
}
