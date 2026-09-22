package ruby

import (
	"encoding/hex"

	"github.com/type-rb/type-rb/internal/ir"
)

// Keep portable bindings out of Ruby's keyword namespace. The TypeRB checker
// reserves the __trb prefix, so a source binding cannot collide with the
// generated name.
func rubyBindingName(name string) string {
	if !rubyReservedBinding(name) {
		return name
	}
	return "__trb_binding_" + hex.EncodeToString([]byte(name))
}

func rubyReservedBinding(name string) bool {
	switch name {
	case "alias", "and", "begin", "break", "case", "class", "def", "defined", "do", "else", "elsif", "end", "ensure", "false", "for", "if", "in", "module", "next", "nil", "not", "or", "redo", "rescue", "retry", "return", "self", "super", "then", "true", "undef", "unless", "until", "when", "while", "yield":
		return true
	}
	return false
}

// Ruby accepts a keyword label such as next: in a signature, but the resulting
// local cannot be referenced as `next`. Copy such parameters to a safe local at
// entry. This preserves Ruby's native required/default/unknown-keyword checks.
func (g *generator) parameterAliases(parameters []ir.Parameter) {
	for _, parameter := range parameters {
		if (parameter.Keyword || parameter.NamedOnly) && rubyBindingName(parameter.Name) != parameter.Name {
			g.line(rubyBindingName(parameter.Name)+" = ::Kernel.binding.local_variable_get(:"+parameter.Name+")", "")
		}
	}
}
