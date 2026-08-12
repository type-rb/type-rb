package typeprovider

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
)

func init() {
	register(jobsintegration.TypeProvider, loadJobs)
}

func loadJobs(programs []*ast.Program, _ Context) (*declaration.Catalog, error) {
	return jobsintegration.Declarations(programs)
}
