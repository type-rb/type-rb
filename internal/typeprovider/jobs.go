package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
)

func init() {
	register(jobsintegration.TypeProvider, loadJobs, jobsProviderInputs)
}

func loadJobs(programs []*ast.Program, _ Context) (*declaration.Catalog, error) {
	return jobsintegration.Declarations(programs)
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
