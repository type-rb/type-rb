package compiler

import (
	pathpkg "path"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/resolver"
)

// validateGoRunnableEntrypointImports rejects project edges that would make a
// generated Go package import the runnable package. Same-directory TypeRB
// modules share one generated Go package and therefore remain valid.
func validateGoRunnableEntrypointImports(units []SourceUnit, programs map[string]*ast.Program, resolutions map[string]resolver.Result, ownerModule string, options Options) error {
	if options.Mode != "go" || ownerModule == "" {
		return nil
	}
	ownerProgram := programs[ownerModule]
	if ownerProgram == nil {
		return nil
	}
	ownerMethod := topLevelMethod(ownerProgram, MainFunction)
	if ownerMethod == nil {
		return nil
	}
	ownerFilename := ""
	for _, source := range units {
		if source.ModulePath == ownerModule {
			ownerFilename = source.Filename
			break
		}
	}

	ordered := append([]SourceUnit(nil), units...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].ModulePath < ordered[right].ModulePath })
	var items []diagnostic.Diagnostic
	for _, source := range ordered {
		if source.ModulePath == ownerModule || source.ModulePath == options.InteractiveModule || goModuleDirectory(source.ModulePath) == goModuleDirectory(ownerModule) {
			continue
		}
		program := programs[source.ModulePath]
		resolution := resolutions[source.ModulePath]
		if program == nil {
			continue
		}
		for _, statement := range program.Statements {
			importedNode, ok := statement.(*ast.ImportStatement)
			if !ok {
				continue
			}
			imported := resolution.Imports[importedNode]
			if imported == nil || imported.Kind != resolver.ProjectImport || imported.Path != ownerModule {
				continue
			}
			item := diagnostic.Diagnostic{
				Code: diagnostic.BackendError, Severity: diagnostic.Error,
				Message: "module " + source.ModulePath + " cannot import runnable entrypoint module " + ownerModule +
					" in Go mode; generated Go entrypoints cannot be imported from another package; move shared declarations into a separate module",
				Path: source.Filename, Span: importedNode.Span(),
				Related: []diagnostic.RelatedInformation{{
					Message:  "runnable entrypoint declared here",
					Location: diagnostic.Location{Path: ownerFilename, Span: ownerMethod.Span()},
				}},
			}
			items = append(items, normalizeSourceDiagnostics([]diagnostic.Diagnostic{item}, source, diagnostic.BackendError)...)
		}
	}
	if len(items) == 0 {
		return nil
	}
	return NewCompileError("", diagnostic.BackendError, items)
}

func goModuleDirectory(modulePath string) string {
	directory := pathpkg.Dir(modulePath)
	if directory == "." {
		return ""
	}
	return directory
}
