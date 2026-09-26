// Package target names the output modes a TypeRB project can declare and
// distinguishes them from the modes this implementation can build.
package target

import "fmt"

var declared = [...]string{"go", "ruby", "typescript", "trb"}

var built = map[string]bool{"go": true, "ruby": true, "typescript": true}

// Declared returns every output mode a project may declare, in a stable order.
// A mode selects the backend, toolchain, and package ecosystem; it never
// changes grammar or portable semantics.
func Declared() []string {
	return append([]string(nil), declared[:]...)
}

// IsDeclared reports whether mode is a project output mode.
func IsDeclared(mode string) bool {
	for _, candidate := range declared {
		if candidate == mode {
			return true
		}
	}
	return false
}

// IsBuilt reports whether this implementation has a backend for mode. Declared
// modes without a backend support analysis, formatting, linting, and TypeRB
// package installation, but not generation or execution.
func IsBuilt(mode string) bool {
	return built[mode]
}

// DeclaredList returns the declared modes as an English list for diagnostics.
func DeclaredList() string {
	return "go, ruby, typescript, or trb"
}

// Unavailable reports that a command needs a backend this implementation does
// not provide for a declared mode.
func Unavailable(command, mode string) error {
	return fmt.Errorf("%s is not available for mode %s in this implementation", command, mode)
}
