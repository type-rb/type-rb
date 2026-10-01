package projectintegration

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/resolver"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func init() {
	register(webintegration.ProjectProvider, analyzeWeb)
}

func analyzeWeb(context Context) (Contribution, []Issue) {
	sources := make([]webintegration.Source, 0, len(context.Sources))
	programs := make([]*ast.Program, 0, len(context.Sources))
	modulePaths := make([]string, 0, len(context.Sources))
	filenames := map[string]string{}
	for _, source := range context.Sources {
		sources = append(sources, webintegration.Source{
			Filename:   source.Filename,
			ModulePath: source.ModulePath,
			Program:    source.Program,
		})
		filenames[source.ModulePath] = source.Filename
		modulePaths = append(modulePaths, source.ModulePath)
		if !source.CompilerOwned && !source.Official && !source.ExternalPackage {
			programs = append(programs, source.Program)
		}
	}
	manifest, webIssues := webintegration.Analyze(sources, context.Resolutions, context.SourceRoot)
	issues := make([]Issue, 0, len(webIssues))
	for _, issue := range webIssues {
		issues = append(issues, Issue{Filename: issue.Filename, Message: issue.Message, Span: issue.Span})
	}
	if len(issues) == 0 {
		declarationRoots := map[string]string{}
		for _, resolution := range context.Resolutions {
			for _, imported := range resolution.Imports {
				if root, ok := resolver.RootDeclaration(imported); ok {
					declarationRoots[imported.RuntimePath()] = root
				}
			}
		}
		input, err := packageextensionhost.ExportProjectDeclarationInput(webintegration.PackageName, programs, packageextensionhost.ProjectDeclarationInputOptions{
			PackageAliasesByModule: context.PackageAliasesByModule,
			KnownModulePaths:       modulePaths,
			KnownDeclarationRoots:  declarationRoots,
		})
		if err != nil {
			issues = append(issues, Issue{Filename: firstWebSourceFilename(context.Sources), Message: err.Error()})
		} else {
			catalog, contractIssues, contractErr := webintegration.BuildEndpointCatalog(input, manifest.Routes)
			if contractErr != nil {
				issues = append(issues, Issue{Filename: firstWebSourceFilename(context.Sources), Message: contractErr.Error()})
			} else {
				manifest.EndpointCatalog = catalog
				for _, issue := range contractIssues {
					issues = append(issues, Issue{
						Filename: filenames[issue.ModulePath], Message: issue.Message,
						Span: packageextensionhost.ImportSourceSpan(issue.Span),
					})
				}
			}
		}
	}
	return Contribution{Extension: manifest, MethodTargets: manifest.MethodTargets()}, issues
}

func firstWebSourceFilename(sources []Source) string {
	for _, source := range sources {
		if !source.CompilerOwned && !source.Official && !source.ExternalPackage {
			return source.Filename
		}
	}
	return ""
}
