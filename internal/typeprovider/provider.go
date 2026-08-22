// Package typeprovider loads compiler-owned declaration graphs for explicitly
// imported platform packages. Providers are application-transparent: users
// import a runtime package and the compiler discovers its types automatically.
package typeprovider

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/stdlib"
)

type Context struct {
	ProjectRoot            string
	PackageOptions         map[string][]byte
	PackageAliasesByModule map[string]map[string]string
}

type loader func([]*ast.Program, Context) (*declaration.Catalog, error)
type inputSnapshotter func([]*ast.Program, Context) providerInputSnapshot

type providerDefinition struct {
	load   loader
	inputs inputSnapshotter
}

// InputSnapshot records the syntax and external files that determine the
// declarations produced by the active type providers.
type InputSnapshot struct {
	providers []providerInputSnapshot
	reusable  bool
}

type providerInputSnapshot struct {
	name     string
	programs []*ast.Program
	files    []providerFileSnapshot
	reusable bool
}

type providerFileSnapshot struct {
	path    string
	missing bool
	digest  [sha256.Size]byte
}

var providers = map[string]providerDefinition{}

func register(name string, implementation loader, inputs inputSnapshotter) {
	if name == "" || implementation == nil || inputs == nil {
		panic("type provider requires a name, implementation, and input snapshotter")
	}
	if _, exists := providers[name]; exists {
		panic("type provider is already registered: " + name)
	}
	providers[name] = providerDefinition{load: implementation, inputs: inputs}
}

func Load(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	names := activeProviderNames(programs)
	result := declaration.NewCatalog()
	for _, name := range names {
		implementation := providers[name].load
		if implementation == nil {
			return nil, fmt.Errorf("unknown type provider %s", name)
		}
		catalog, err := implementation(programs, context)
		if err != nil {
			return nil, err
		}
		result.Merge(catalog)
	}
	return result, nil
}

// CaptureInputs creates a lightweight provider dependency snapshot. Providers
// backed by live resources that cannot be fingerprinted remain non-reusable.
func CaptureInputs(programs []*ast.Program, context Context) InputSnapshot {
	names := activeProviderNames(programs)
	result := InputSnapshot{providers: make([]providerInputSnapshot, 0, len(names)), reusable: true}
	for _, name := range names {
		definition, exists := providers[name]
		if !exists || definition.inputs == nil {
			result.reusable = false
			continue
		}
		inputs := definition.inputs(programs, context)
		inputs.name = name
		if !inputs.reusable {
			result.reusable = false
		}
		result.providers = append(result.providers, inputs)
	}
	return result
}

// CanReuse reports whether declarations produced for previous remain valid for
// the receiver's current provider inputs.
func (s InputSnapshot) CanReuse(previous InputSnapshot) bool {
	if !s.reusable || !previous.reusable || len(s.providers) != len(previous.providers) {
		return false
	}
	for index, left := range s.providers {
		right := previous.providers[index]
		if left.name != right.name || len(left.programs) != len(right.programs) || len(left.files) != len(right.files) {
			return false
		}
		for programIndex, program := range left.programs {
			if program != right.programs[programIndex] {
				return false
			}
		}
		for fileIndex, file := range left.files {
			if file != right.files[fileIndex] {
				return false
			}
		}
	}
	return true
}

func activeProviderNames(programs []*ast.Program) []string {
	active := map[string]bool{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			if imported, ok := statement.(*ast.ImportStatement); ok {
				if definition, exists := stdlib.Lookup(imported.Path); exists && definition.TypeProvider != "" {
					active[definition.TypeProvider] = true
				} else if bundled, exists := official.Lookup(imported.Path); exists && bundled.Definition.TypeProvider != "" {
					active[bundled.Definition.TypeProvider] = true
				}
			}
		}
	}
	names := make([]string, 0, len(active))
	for name := range active {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func staticProviderInputs(_ []*ast.Program, _ Context) providerInputSnapshot {
	return providerInputSnapshot{reusable: true}
}

func providerPrograms(programs []*ast.Program, relevant func(*ast.Program) bool) []*ast.Program {
	result := make([]*ast.Program, 0, len(programs))
	for _, program := range programs {
		if relevant(program) {
			result = append(result, program)
		}
	}
	return result
}

func captureProviderFile(path string, allowMissing bool) (providerFileSnapshot, bool) {
	contents, err := os.ReadFile(path)
	if err == nil {
		return providerFileSnapshot{path: path, digest: sha256.Sum256(contents)}, true
	}
	if allowMissing && errors.Is(err, os.ErrNotExist) {
		return providerFileSnapshot{path: path, missing: true}, true
	}
	return providerFileSnapshot{}, false
}
