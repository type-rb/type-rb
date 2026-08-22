package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
)

func init() {
	register(jobsintegration.TypeProvider, loadJobs, jobsProviderInputs)
}

func loadJobs(programs []*ast.Program, _ Context) (*declaration.Catalog, error) {
	provided, err := loadJobDeclarations(programs)
	if err != nil {
		return nil, err
	}
	return packageextensionhost.ImportDeclarationCatalog(provided)
}

func loadJobDeclarations(programs []*ast.Program) (packageextension.DeclarationCatalog, error) {
	catalog, err := jobsintegration.Declarations(programs)
	if err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	return packageextensionhost.ExportDeclarationCatalog(jobsintegration.PackageName, catalog)
}

func jobsProviderInputs(programs []*ast.Program, _ Context) providerInputSnapshot {
	return providerInputSnapshot{programs: providerPrograms(programs, jobsProviderProgram), reusable: true}
}

func jobsProviderProgram(program *ast.Program) bool {
	for _, statement := range program.Statements {
		switch node := statement.(type) {
		case *ast.TypeAliasStatement:
			return true
		case *ast.ClassStatement:
			if identifier, ok := node.Superclass.(*ast.Identifier); ok && identifier.Name == "Job" {
				return true
			}
		}
	}
	return false
}
