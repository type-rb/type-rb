package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

func init() {
	register(ormintegration.TypeProvider, loadORM)
}

func loadORM(programs []*ast.Program, context Context) (*declaration.Catalog, error) {
	return ormintegration.Declarations(programs, context.ProjectRoot, context.PackageOptions)
}
