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

type goInlineDeclaration struct {
	parameters []string
	fields     []types.Type
}

func analyzeGoEnumLayout(programs []*ir.Program) goEnumLayout {
	declarations := map[identity.Declaration]goInlineDeclaration{}
	enums := []*ir.Enum{}
	var collect func([]ir.Statement)
	collect = func(statements []ir.Statement) {
		for _, statement := range statements {
			switch node := statement.(type) {
			case *ir.Module:
				collect(node.Body)
			case *ir.Record:
				entry := goInlineDeclaration{parameters: node.TypeParameters}
				for _, item := range node.Body {
					if field, ok := item.(*ir.RecordField); ok {
						entry.fields = append(entry.fields, field.Type)
					}
				}
				declarations[node.Declaration] = entry
			case *ir.Enum:
				enums = append(enums, node)
				entry := goInlineDeclaration{parameters: node.TypeParameters}
				for _, item := range node.Body {
					if variant, ok := item.(*ir.EnumMember); ok {
						for _, field := range variant.Fields {
							entry.fields = append(entry.fields, field.Type)
						}
					}
				}
				declarations[node.Declaration] = entry
			case *ir.TypeAlias:
				declarations[node.Declaration] = goInlineDeclaration{node.TypeParameters, []types.Type{node.Target}}
			case *ir.Newtype:
				declarations[node.Declaration] = goInlineDeclaration{fields: []types.Type{node.Target}}
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
				if goInlineReaches(field.Type, enum.Declaration, declarations, map[identity.Declaration]bool{}) {
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

func goInlineReaches(value types.Type, target identity.Declaration, declarations map[identity.Declaration]goInlineDeclaration, active map[identity.Declaration]bool) bool {
	// These types already carry indirection in Go. Classes are absent from the
	// inline catalog because their generated type is a pointer as well.
	if value.Nullable || value.Kind != types.Named || value.Declaration.Empty() {
		return false
	}
	if value.Declaration == target {
		return true
	}
	entry, ok := declarations[value.Declaration]
	if !ok {
		return false
	}
	if active[value.Declaration] {
		// A repeated generic wrapper can expose its own nested argument. Follow
		// the finite authored arguments without re-expanding the declaration.
		for _, argument := range value.Args {
			if goInlineReaches(argument, target, declarations, active) {
				return true
			}
		}
		return false
	}
	active[value.Declaration] = true
	defer delete(active, value.Declaration)
	arguments := map[string]types.Type{}
	for index, parameter := range entry.parameters {
		if index < len(value.Args) {
			arguments[parameter] = value.Args[index]
		}
	}
	for _, field := range entry.fields {
		if goInlineReaches(goInlineSubstitute(field, arguments), target, declarations, active) {
			return true
		}
	}
	return false
}

func goInlineSubstitute(value types.Type, arguments map[string]types.Type) types.Type {
	if value.Kind == types.Named && value.Declaration.Empty() {
		if argument, ok := arguments[value.Name]; ok {
			argument.Nullable = argument.Nullable || value.Nullable
			return argument
		}
	}
	args := make([]types.Type, len(value.Args))
	for index, argument := range value.Args {
		args[index] = goInlineSubstitute(argument, arguments)
	}
	value.Args = args
	return value
}

func (layout goEnumLayout) indirect(owner identity.Declaration, variant, field string) bool {
	return layout[owner][variant+"\x00"+field]
}
