package projectintegration

import webintegration "github.com/type-rb/type-rb/internal/web"

func init() {
	register(webintegration.ProjectProvider, analyzeWeb)
}

func analyzeWeb(context Context) (Contribution, []Issue) {
	sources := make([]webintegration.Source, 0, len(context.Sources))
	for _, source := range context.Sources {
		sources = append(sources, webintegration.Source{
			Filename:   source.Filename,
			ModulePath: source.ModulePath,
			Program:    source.Program,
		})
	}
	manifest, webIssues := webintegration.Analyze(sources, context.Resolutions, context.SourceRoot)
	issues := make([]Issue, 0, len(webIssues))
	for _, issue := range webIssues {
		issues = append(issues, Issue{Filename: issue.Filename, Message: issue.Message, Span: issue.Span})
	}
	return Contribution{Extension: manifest, MethodTargets: manifest.MethodTargets()}, issues
}
