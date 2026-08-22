package compiler

import (
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	"github.com/type-rb/type-rb/internal/projectintegration"
)

const projectGeneratedSourcePrefix = "packageextension.project:"

func applyProjectGeneratedSources(units []SourceUnit, programs map[string]*ast.Program, analysis projectintegration.Analysis) ([]SourceUnit, bool, error) {
	updated := cloneSourceUnits(units)
	byModule := make(map[string]*SourceUnit, len(updated))
	originalByModule := make(map[string]SourceUnit, len(units))
	for index := range updated {
		byModule[updated[index].ModulePath] = &updated[index]
		originalByModule[updated[index].ModulePath] = units[index]
	}

	desired := map[string][]CompilerGeneratedSource{}
	required := map[string]map[string]map[string]bool{}
	for _, generated := range analysis.GeneratedSources() {
		unit := byModule[generated.Source.ModulePath]
		if unit == nil {
			return nil, false, fmt.Errorf("project provider %s generated source %s for unknown module %s", generated.Provider, generated.Source.ID, generated.Source.ModulePath)
		}
		if unit.CompilerOwned || unit.Official || unit.ExternalPackage {
			return nil, false, fmt.Errorf("project provider %s cannot generate source in compiler-owned or external module %s", generated.Provider, generated.Source.ModulePath)
		}
		id := projectGeneratedSourcePrefix + generated.Provider + ":" + generated.Source.ID
		desired[generated.Source.ModulePath] = append(desired[generated.Source.ModulePath], CompilerGeneratedSource{
			ID: id, Source: []byte(generated.Source.Source), Origin: packageextensionhost.ImportSourceSpan(generated.Source.Origin),
		})
		if required[generated.Source.ModulePath] == nil {
			required[generated.Source.ModulePath] = map[string]map[string]bool{}
		}
		for _, imported := range generated.Source.RequiredImports {
			if required[generated.Source.ModulePath][imported.Path] == nil {
				required[generated.Source.ModulePath][imported.Path] = map[string]bool{}
			}
			for _, symbol := range imported.Symbols {
				required[generated.Source.ModulePath][imported.Path][symbol] = true
			}
		}
	}

	for modulePath := range required {
		original := originalByModule[modulePath]
		imports := generatedImportsSource(programs[modulePath], original, required[modulePath], func(id string) bool {
			return strings.HasPrefix(id, projectGeneratedSourcePrefix)
		})
		if imports == "" {
			continue
		}
		origin := desired[modulePath][0].Origin
		desired[modulePath] = append([]CompilerGeneratedSource{{
			ID: projectGeneratedSourcePrefix + "imports", Source: []byte(imports), Origin: origin,
		}}, desired[modulePath]...)
	}

	changed := false
	for index := range updated {
		unit := &updated[index]
		base := make([]CompilerGeneratedSource, 0, len(unit.CompilerGeneratedSources))
		for _, generated := range unit.CompilerGeneratedSources {
			if !strings.HasPrefix(generated.ID, projectGeneratedSourcePrefix) {
				base = append(base, generated)
			}
		}
		projectSources := desired[unit.ModulePath]
		if len(projectSources) > 1 {
			imports := projectSources[0].ID == projectGeneratedSourcePrefix+"imports"
			start := 0
			if imports {
				start = 1
			}
			sort.Slice(projectSources[start:], func(i, j int) bool {
				return projectSources[start+i].ID < projectSources[start+j].ID
			})
		}
		next := append(base, projectSources...)
		if !equalCompilerGeneratedSources(unit.CompilerGeneratedSources, next) {
			unit.CompilerGeneratedSources = next
			changed = true
		}
	}
	return updated, changed, nil
}

func hasProjectGeneratedSources(units []SourceUnit) bool {
	for _, unit := range units {
		for _, generated := range unit.CompilerGeneratedSources {
			if strings.HasPrefix(generated.ID, projectGeneratedSourcePrefix) {
				return true
			}
		}
	}
	return false
}
