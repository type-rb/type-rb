package typeprovider

import (
	"path/filepath"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	railsprovider "github.com/type-rb/type-rb/internal/typeprovider/rails"
)

const railsTypeProvider = "rails"

func init() {
	register(railsTypeProvider, loadRails, railsProviderInputs)
}

func loadRails(_ []*ast.Program, context Context) (*declaration.Catalog, error) {
	provided, err := loadRailsDeclarations(context)
	if err != nil {
		return nil, err
	}
	return packageextensionhost.ImportDeclarationCatalog(provided)
}

func loadRailsDeclarations(context Context) (packageextension.DeclarationCatalog, error) {
	catalog, err := railsprovider.Load(context.ProjectRoot)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	return packageextensionhost.ExportDeclarationCatalog(railsTypeProvider, catalog)
}

func railsProviderInputs(_ []*ast.Program, context Context) providerInputSnapshot {
	if context.ProjectRoot == "" {
		return providerInputSnapshot{reusable: true}
	}
	file, ok := captureProviderFile(filepath.Join(context.ProjectRoot, "db", "schema.rb"), true)
	return providerInputSnapshot{files: []providerFileSnapshot{file}, reusable: ok}
}
