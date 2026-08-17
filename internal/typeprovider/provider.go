// Package typeprovider loads compiler-owned declaration graphs for explicitly
// imported platform packages. Providers are application-transparent: users
// import a runtime package and the compiler discovers its types automatically.
package typeprovider

import (
	"fmt"
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

var loaders = map[string]loader{}

func register(name string, implementation loader) {
	if name == "" || implementation == nil {
		panic("type provider requires a name and implementation")
	}
	if _, exists := loaders[name]; exists {
		panic("type provider is already registered: " + name)
	}
	loaders[name] = implementation
}

func Load(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	providers := map[string]bool{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			if imported, ok := statement.(*ast.ImportStatement); ok {
				if definition, exists := stdlib.Lookup(imported.Path); exists && definition.TypeProvider != "" {
					providers[definition.TypeProvider] = true
				} else if bundled, exists := official.Lookup(imported.Path); exists && bundled.Definition.TypeProvider != "" {
					providers[bundled.Definition.TypeProvider] = true
				}
			}
		}
	}
	result := declaration.NewCatalog()
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		implementation := loaders[name]
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
