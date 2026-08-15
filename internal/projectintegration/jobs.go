package projectintegration

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
)

func init() {
	register(jobsintegration.ProjectProvider, analyzeJobs)
	register(jobssql.ProjectProvider, analyzeSQLJobs)
}

func analyzeJobs(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	manifest, err := jobsintegration.Analyze(programs, context.Resolutions)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	if len(manifest.Jobs) > 0 {
		configuration := strings.TrimSpace(context.JobsConfiguration)
		if configuration == "" {
			return Contribution{}, []Issue{{Message: "trb/jobs requires jobs.configuration in trbconfig.jsonc"}}
		}
		found := false
		importsSQL := false
		for _, source := range context.Sources {
			if source.ModulePath != configuration {
				continue
			}
			found = true
			for _, statement := range source.Program.Statements {
				imported, ok := statement.(*ast.ImportStatement)
				if ok && imported.Path == jobssql.PackageName {
					importsSQL = true
					break
				}
			}
			break
		}
		if !found {
			return Contribution{}, []Issue{{Message: "jobs.configuration module " + strconv.Quote(configuration) + " was not found"}}
		}
		if !importsSQL {
			return Contribution{}, []Issue{{Message: "jobs.configuration must import a trb/jobs adapter; trb/jobs/sql is currently available"}}
		}
	}
	return Contribution{Extension: manifest, AllPrograms: true}, nil
}

func analyzeSQLJobs(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	manifest, err := jobssql.Analyze(programs, context.JobsConfiguration)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	return Contribution{Extension: manifest, AllPrograms: true}, nil
}
