package projectintegration

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	jobssql "github.com/type-rb/type-rb/internal/jobs/sqladapter"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
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
	projectInput, err := packageextensionhost.ExportProjectDeclarationInput(
		jobsintegration.PackageName,
		programs,
		packageextensionhost.ProjectDeclarationInputOptions{PackageAliasesByModule: context.PackageAliasesByModule},
	)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	origin := packageextension.SourceSpan{}
	generationEntrypoint := context.EntrypointModule
	for _, source := range context.Sources {
		if source.ModulePath != context.EntrypointModule {
			continue
		}
		if source.CompilerOwned || source.Official || source.ExternalPackage {
			generationEntrypoint = ""
			break
		}
		for _, statement := range source.Program.Statements {
			method, ok := statement.(*ast.MethodStatement)
			if ok && method.Name == "main" {
				origin = packageextensionhost.ExportSourceSpan(method.Span())
				break
			}
		}
		break
	}
	generation, err := jobsintegration.GenerateProject(projectInput, generationEntrypoint, context.JobsConfiguration, origin)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	return Contribution{Extension: manifest, AllPrograms: true, Generation: &generation}, nil
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
