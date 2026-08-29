package compiler

import (
	"bytes"
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
)

func applyCallSpecializations(units []SourceUnit, programs map[string]*ast.Program, checkedPrograms map[string]checker.Result) ([]SourceUnit, []diagnostic.Diagnostic, bool, error) {
	updated := cloneSourceUnits(units)
	byModule := make(map[string]*SourceUnit, len(updated))
	for index := range updated {
		byModule[updated[index].ModulePath] = &updated[index]
	}
	var diagnostics []diagnostic.Diagnostic
	changed := false
	requiredImports := map[string]map[string]map[string]bool{}
	firstOrigins := map[string]ast.Expression{}
	for _, source := range units {
		checked := checkedPrograms[source.ModulePath]
		calls := make([]*ast.CallExpression, 0, len(checked.CallSpecializationRequests))
		for call := range checked.CallSpecializationRequests {
			calls = append(calls, call)
		}
		sort.Slice(calls, func(i, j int) bool { return calls[i].Span().Start.Offset < calls[j].Span().Start.Offset })
		for _, call := range calls {
			semantic := checked.CallSpecializationRequests[call]
			response, err := packageextension.SpecializeCall(semantic.Request)
			if err != nil {
				return nil, nil, false, err
			}
			for _, issue := range response.Issues {
				diagnostics = append(diagnostics, diagnostic.Diagnostic{
					Code: diagnostic.TypeError, Severity: diagnostic.Error, Message: issue.Message,
					Path: source.Filename, Span: call.Span(),
				})
			}
			if firstOrigins[source.ModulePath] == nil {
				firstOrigins[source.ModulePath] = call
			}
			if requiredImports[source.ModulePath] == nil {
				requiredImports[source.ModulePath] = map[string]map[string]bool{}
			}
			for _, imported := range response.RequiredImports {
				if requiredImports[source.ModulePath][imported.Path] == nil {
					requiredImports[source.ModulePath][imported.Path] = map[string]bool{}
				}
				for _, symbol := range imported.Symbols {
					requiredImports[source.ModulePath][imported.Path][symbol] = true
				}
			}
			if response.Replacement != nil {
				replacement := checker.CallSpecialization{Callee: response.Replacement.Callee}
				for _, argument := range response.Replacement.Arguments {
					switch argument {
					case packageextension.ReceiverValue:
						replacement.Arguments = append(replacement.Arguments, semantic.Receiver)
					default:
						return nil, nil, false, fmt.Errorf("package call specializer %q returned unsupported value source %q", semantic.Request.Provider, argument)
					}
				}
				checked.CallSpecializations[call] = replacement
			}
			if response.GeneratedSource == nil || response.GeneratedSource.Source == "" {
				continue
			}
			unit := byModule[source.ModulePath]
			if unit == nil {
				return nil, nil, false, fmt.Errorf("package call specialization module %q is missing", source.ModulePath)
			}
			found := false
			for _, existing := range unit.CompilerGeneratedSources {
				if existing.ID != response.GeneratedSource.ID {
					continue
				}
				found = true
				if !bytes.Equal(existing.Source, []byte(response.GeneratedSource.Source)) {
					return nil, nil, false, fmt.Errorf("package call specialization %q changed generated source during one analysis", existing.ID)
				}
				break
			}
			if !found {
				unit.CompilerGeneratedSources = append(unit.CompilerGeneratedSources, CompilerGeneratedSource{
					ID: response.GeneratedSource.ID, Source: []byte(response.GeneratedSource.Source), Origin: call.Span(),
				})
				changed = true
			}
		}
		checkedPrograms[source.ModulePath] = checked
	}
	for modulePath, imports := range requiredImports {
		unit := byModule[modulePath]
		if unit == nil {
			continue
		}
		source := generatedImportsSource(programs[modulePath], checkedPrograms[modulePath].Resolution, *unit, imports, func(id string) bool {
			return id == "packageextension.imports"
		})
		if source == "" {
			continue
		}
		origin := firstOrigins[modulePath].Span()
		found := false
		for index, existing := range unit.CompilerGeneratedSources {
			if existing.ID != "packageextension.imports" {
				continue
			}
			found = true
			if !bytes.Equal(existing.Source, []byte(source)) {
				unit.CompilerGeneratedSources[index].Source = []byte(source)
				unit.CompilerGeneratedSources[index].Origin = origin
				changed = true
			}
			break
		}
		if !found {
			unit.CompilerGeneratedSources = append([]CompilerGeneratedSource{{
				ID: "packageextension.imports", Source: []byte(source), Origin: origin,
			}}, unit.CompilerGeneratedSources...)
			changed = true
		}
	}
	return updated, diagnostics, changed, nil
}

func generatedImportsSource(_ *ast.Program, _ resolver.Result, unit SourceUnit, required map[string]map[string]bool, excluded func(string) bool) string {
	visible := map[string]map[string]bool{}
	for _, generated := range unit.CompilerGeneratedSources {
		if excluded != nil && excluded(generated.ID) {
			continue
		}
		fragment, diagnostics := parser.Parse(generated.Source)
		if len(diagnostics) != 0 {
			continue
		}
		for _, statement := range fragment.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if ok && imported.Alias == "" {
				addVisibleImport(visible, imported.Path, imported.Symbols)
			}
		}
	}
	paths := make([]string, 0, len(required))
	for path := range required {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	var lines []string
	for _, path := range paths {
		var symbols []string
		for symbol := range required[path] {
			if !visible[path][symbol] {
				symbols = append(symbols, symbol)
			}
		}
		if len(symbols) == 0 {
			continue
		}
		sort.Strings(symbols)
		lines = append(lines, "import { "+strings.Join(symbols, ", ")+" } from "+path)
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n") + "\n"
}

func addVisibleImport(visible map[string]map[string]bool, path string, symbols []string) {
	if visible[path] == nil {
		visible[path] = map[string]bool{}
	}
	for _, symbol := range symbols {
		visible[path][symbol] = true
	}
}
