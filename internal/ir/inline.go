package ir

import (
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/types"
)

type InlineDeclaration struct {
	Parameters []string
	Fields     []types.Type
}

func InlineReaches(value types.Type, target identity.Declaration, declarations map[identity.Declaration]InlineDeclaration, active map[identity.Declaration]bool) bool {
	// These types already carry indirection in the portable representation. Classes are absent from the
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
			if InlineReaches(argument, target, declarations, active) {
				return true
			}
		}
		return false
	}
	active[value.Declaration] = true
	defer delete(active, value.Declaration)
	arguments := map[string]types.Type{}
	for index, parameter := range entry.Parameters {
		if index < len(value.Args) {
			arguments[parameter] = value.Args[index]
		}
	}
	for _, field := range entry.Fields {
		if InlineReaches(SubstituteInlineType(field, arguments), target, declarations, active) {
			return true
		}
	}
	return false
}

func SubstituteInlineType(value types.Type, arguments map[string]types.Type) types.Type {
	if value.Kind == types.Named && value.Declaration.Empty() {
		if argument, ok := arguments[value.Name]; ok {
			argument.Nullable = argument.Nullable || value.Nullable
			return argument
		}
	}
	args := make([]types.Type, len(value.Args))
	for index, argument := range value.Args {
		args[index] = SubstituteInlineType(argument, arguments)
	}
	value.Args = args
	return value
}
