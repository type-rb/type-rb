// Package typeprovider loads compiler-owned declaration graphs for explicitly
// imported platform packages. Providers are application-transparent: users
// import a runtime package and the compiler discovers its types automatically.
package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/stdlib"
	railsprovider "github.com/type-rb/type-rb/internal/typeprovider/rails"
)

type Context struct {
	ProjectRoot string
}

func Load(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	providers := map[string]bool{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			if imported, ok := statement.(*ast.ImportStatement); ok {
				if definition, exists := stdlib.Lookup(imported.Path); exists && definition.TypeProvider != "" {
					providers[definition.TypeProvider] = true
				}
			}
		}
	}
	result := declaration.NewCatalog()
	if providers["rails"] {
		catalog, err := railsprovider.Load(context.ProjectRoot)
		if err != nil {
			return nil, err
		}
		result.Merge(catalog)
	}
	return result, nil
}
