package projectintegration

import (
	"github.com/type-rb/type-rb/internal/ast"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
)

func init() {
	register(jobsintegration.ProjectProvider, analyzeJobs)
}

func analyzeJobs(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	manifest, err := jobsintegration.Analyze(programs, context.PackageOptions[jobsintegration.PackageName])
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	return Contribution{Extension: manifest, AllPrograms: true}, nil
}
