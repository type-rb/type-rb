package projectintegration

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/resolver"
)

func init() {
	register(ormintegration.ProjectProvider, analyzeORM)
}

func analyzeORM(context Context) (Contribution, []Issue) {
	programs := make([]*ast.Program, 0, len(context.Sources))
	for _, source := range context.Sources {
		programs = append(programs, source.Program)
	}
	manifest, err := ormintegration.Analyze(programs, context.ProjectRoot, context.PackageOptions, context.PackageAliasesByModule)
	if err != nil {
		return Contribution{}, []Issue{{Message: err.Error()}}
	}
	return Contribution{Extension: manifest, AllPrograms: true}, ormRuntimeBootstrapIssues(context, manifest)
}

type ormProjectImportEdge struct {
	from string
	to   string
	node *ast.ImportStatement
}

func ormRuntimeBootstrapIssues(context Context, manifest *ormintegration.Manifest) []Issue {
	if manifest == nil || context.EntrypointModule == "" {
		return nil
	}
	filenames := map[string]string{}
	for _, source := range context.Sources {
		filenames[source.ModulePath] = source.Filename
	}
	modelModules := map[string]bool{}
	for _, model := range manifest.Models {
		if model.ModulePath != context.EntrypointModule {
			modelModules[model.ModulePath] = true
		}
	}
	modules := make([]string, 0, len(modelModules))
	for modulePath := range modelModules {
		modules = append(modules, modulePath)
	}
	sort.Strings(modules)
	issues := []Issue{}
	for _, modulePath := range modules {
		path, found := ormProjectImportPath(modulePath, context.EntrypointModule, context.Resolutions, map[string]bool{})
		if !found || len(path) == 0 {
			continue
		}
		cycle := []string{context.EntrypointModule, modulePath}
		for _, edge := range path {
			cycle = append(cycle, edge.to)
		}
		issues = append(issues, Issue{
			Filename: filenames[path[0].from],
			Span:     path[0].node.Span(),
			Message: "ORM runtime bootstrap would create import cycle: " + strings.Join(cycle, " -> ") +
				"; ORM model modules must not import the runnable entrypoint directly or transitively; move shared declarations into a separate module",
		})
	}
	return issues
}

func ormProjectImportPath(current, target string, resolutions map[string]resolver.Result, visiting map[string]bool) ([]ormProjectImportEdge, bool) {
	if current == target {
		return nil, true
	}
	if visiting[current] {
		return nil, false
	}
	visiting[current] = true
	defer delete(visiting, current)
	edges := []ormProjectImportEdge{}
	for _, imported := range resolutions[current].Imports {
		if imported == nil || imported.Kind != resolver.ProjectImport || imported.Node == nil {
			continue
		}
		edges = append(edges, ormProjectImportEdge{from: current, to: imported.Path, node: imported.Node})
	}
	sort.Slice(edges, func(left, right int) bool {
		if edges[left].to != edges[right].to {
			return edges[left].to < edges[right].to
		}
		return edges[left].node.Span().Start.Offset < edges[right].node.Span().Start.Offset
	})
	for _, edge := range edges {
		rest, found := ormProjectImportPath(edge.to, target, resolutions, visiting)
		if found {
			return append([]ormProjectImportEdge{edge}, rest...), true
		}
	}
	return nil, false
}
