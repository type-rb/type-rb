package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	railsprovider "github.com/type-rb/type-rb/internal/typeprovider/rails"
)

func init() {
	register("rails", loadRails)
}

func loadRails(_ []*ast.Program, context Context) (*declaration.Catalog, error) {
	return railsprovider.Load(context.ProjectRoot)
}
