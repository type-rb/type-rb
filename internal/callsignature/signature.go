// Package callsignature defines the target-independent parameter contract used
// to bind TypeRB calls across source modules, interfaces, and backends.
package callsignature

import "github.com/type-rb/type-rb/internal/types"

type ParameterKind string

const (
	Positional ParameterKind = "positional"
	NamedOnly  ParameterKind = "named-only"
)

type Presence string

const (
	Required  Presence = "required"
	Omittable Presence = "omittable"
)

// Parameter is the public call contract for one parameter. Label is populated
// only for named-only parameters; positional source names are not part of the
// call signature.
type Parameter struct {
	Kind     ParameterKind
	Label    string
	Type     types.Type
	Presence Presence
}

func FromPositionalTypes(parameters []types.Type, required int) []Parameter {
	result := make([]Parameter, len(parameters))
	for index, parameter := range parameters {
		presence := Omittable
		if index < required {
			presence = Required
		}
		result[index] = Parameter{Kind: Positional, Type: parameter, Presence: presence}
	}
	return result
}

func Types(parameters []Parameter) []types.Type {
	result := make([]types.Type, len(parameters))
	for index, parameter := range parameters {
		result[index] = parameter.Type
	}
	return result
}

func HasNamedOnly(parameters []Parameter) bool {
	for _, parameter := range parameters {
		if parameter.Kind == NamedOnly {
			return true
		}
	}
	return false
}
