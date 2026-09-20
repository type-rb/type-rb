package checker

import (
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
)

func (c *Checker) authoredModuleInScope(name string, sc *scope) identity.Declaration {
	for owner := scopeConstantOwner(sc); ; {
		declaration := c.authoredOwnerIdentities[identity.Qualify(owner, name)]
		if declaration.Kind == identity.Module {
			return declaration
		}
		if owner == "" {
			return identity.Declaration{}
		}
		if separator := strings.LastIndex(owner, "::"); separator >= 0 {
			owner = owner[:separator]
		} else {
			owner = ""
		}
	}
}
