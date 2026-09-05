// Package nativefs contains compiler-owned host filesystem operations. The
// same source is embedded into emitted Go projects and used by the evaluator.
package nativefs

import "embed"

//go:embed lock_unix.go lock_other.go open_leaf_unix.go errors.go
var sources embed.FS

// Sources returns independent copies of the fixed compiler-owned support
// files. Callers cannot supply paths or register additional native authority.
func Sources() map[string][]byte {
	result := map[string][]byte{}
	for _, name := range []string{"errors.go", "lock_unix.go", "lock_other.go", "open_leaf_unix.go"} {
		data, err := sources.ReadFile(name)
		if err != nil {
			panic(err)
		}
		result[name] = data
	}
	return result
}

const ModulePath = "golang.org/x/sys"
const ModuleVersion = "v0.45.0"
