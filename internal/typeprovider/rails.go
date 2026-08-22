package typeprovider

import (
	"path/filepath"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	railsprovider "github.com/type-rb/type-rb/internal/typeprovider/rails"
)

func init() {
	register("rails", loadRails, railsProviderInputs)
}

func loadRails(_ []*ast.Program, context Context) (*declaration.Catalog, error) {
	return railsprovider.Load(context.ProjectRoot)
}

func railsProviderInputs(_ []*ast.Program, context Context) providerInputSnapshot {
	if context.ProjectRoot == "" {
		return providerInputSnapshot{reusable: true}
	}
	file, ok := captureProviderFile(filepath.Join(context.ProjectRoot, "db", "schema.rb"), true)
	return providerInputSnapshot{files: []providerFileSnapshot{file}, reusable: ok}
}
