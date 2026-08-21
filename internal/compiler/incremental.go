package compiler

import (
	"bytes"
	"reflect"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/lower"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/projectintegration"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/typeprovider"
)

// analyzeChangedProject reuses resolution and checking results when exactly
// one ordinary source unit changes. It invalidates importers whose visible
// catalog changed and falls back when compiler-owned dependencies change.
func analyzeChangedProject(analyzer *Analyzer, previous *projectAnalysis, sources []SourceUnit, options Options, validateBackend bool) (*projectAnalysis, bool, error) {
	changed, ok := singleSourceChange(previous, sources, options, validateBackend)
	if !ok {
		return nil, false, nil
	}

	program, parseDiagnostics := analyzer.parseUnit(changed, options, true)
	if hasErrors(parseDiagnostics) {
		return nil, true, NewCompileError(changed.Filename, diagnostic.SyntaxError, parseDiagnostics)
	}

	units := cloneSourceUnits(previous.units)
	programs := make(map[string]*ast.Program, len(previous.programs))
	for modulePath, cached := range previous.programs {
		programs[modulePath] = cached
	}
	programs[changed.ModulePath] = program
	found := false
	for index := range units {
		if units[index].ModulePath == changed.ModulePath {
			units[index] = cloneSourceUnit(changed)
			found = true
			break
		}
	}
	if !found || len(dependencySourceUnits(programs, options)) > 0 {
		return nil, false, nil
	}
	if !equalCompilerOwnedImports(previous.programs[changed.ModulePath], program) {
		return nil, false, nil
	}

	modules := make([]resolver.Module, 0, len(units))
	for _, source := range units {
		modules = append(modules, resolver.Module{
			Path: source.ModulePath, Filename: source.Filename, Program: programs[source.ModulePath],
			CompilerOwned: source.CompilerOwned, Official: source.Official,
		})
	}
	catalog, catalogDiagnostics := resolver.NewCatalog(modules)
	var catalogErrors []diagnostic.Diagnostic
	for _, source := range units {
		catalogErrors = append(catalogErrors, catalogDiagnostics[source.Filename]...)
	}
	if hasErrors(catalogErrors) {
		return nil, true, NewCompileError("", diagnostic.ResolutionError, catalogErrors)
	}

	providerPrograms := make([]*ast.Program, 0, len(units))
	for _, source := range units {
		providerPrograms = append(providerPrograms, programs[source.ModulePath])
	}
	packageAliasesByModule := sourcePackageAliases(units, options.PackageAliases)
	declarations, providerErr := typeprovider.Load(providerPrograms, typeprovider.Context{
		ProjectRoot: projectRoot(options), PackageOptions: options.PackageOptions, PackageAliasesByModule: packageAliasesByModule,
	})
	if providerErr != nil {
		return nil, true, compileProviderError(providerErr, units)
	}

	affected := affectedProjectModules(previous, catalog, declarations, changed.ModulePath)
	resolutions := make(map[string]resolver.Result, len(previous.resolutions))
	for modulePath, cached := range previous.resolutions {
		resolutions[modulePath] = cached
	}
	var resolveErrors []diagnostic.Diagnostic
	for _, source := range units {
		if !affected[source.ModulePath] {
			continue
		}
		packageAliases := options.PackageAliases
		if source.PackageAliases != nil {
			packageAliases = source.PackageAliases
		}
		resolved, diagnostics := resolver.Resolve(programs[source.ModulePath], resolver.Options{
			Mode: options.Mode, SourceRoot: options.SourceRoot, Filename: source.Filename, PackageAliases: packageAliases,
			CompilerOwned: source.CompilerOwned, Official: source.Official, Catalog: catalog, Declarations: declarations,
			NativePackages: options.NativePackages,
		})
		resolveErrors = append(resolveErrors, diagnostic.Normalize(diagnostics, source.Filename, diagnostic.ResolutionError)...)
		resolutions[source.ModulePath] = resolved
	}
	if hasErrors(resolveErrors) {
		return nil, true, NewCompileError("", diagnostic.ResolutionError, resolveErrors)
	}
	graphDiagnostics := resolver.ValidateImportGraph(catalog, resolutions)
	var graphErrors []diagnostic.Diagnostic
	for _, source := range units {
		graphErrors = append(graphErrors, graphDiagnostics[source.Filename]...)
	}
	if hasErrors(graphErrors) {
		return nil, true, NewCompileError("", diagnostic.ResolutionError, graphErrors)
	}

	ownerModule, ownerErr := projectMainOwner(units, programs)
	if ownerErr != nil {
		return nil, true, ownerErr
	}
	checkedPrograms := make(map[string]checker.Result, len(previous.checkedPrograms))
	for modulePath, cached := range previous.checkedPrograms {
		checkedPrograms[modulePath] = cached
	}
	var typeErrors []diagnostic.Diagnostic
	for _, source := range units {
		if !affected[source.ModulePath] {
			continue
		}
		program := programs[source.ModulePath]
		checked, diagnostics := analyzer.checkProgram(program, resolutions[source.ModulePath], checker.Options{
			AllowUnusedImports:  options.AllowUnusedImports,
			InteractiveTopLevel: options.InteractiveModule != "" && options.InteractiveModule == source.ModulePath,
			RunnableMain:        topLevelMethod(program, MainFunction),
		})
		if !equalRuntimeDependencies(previous.checkedPrograms[source.ModulePath], checked) {
			return nil, false, nil
		}
		checkedPrograms[source.ModulePath] = checked
		typeErrors = append(typeErrors, diagnostic.Normalize(diagnostics, source.Filename, diagnostic.TypeError)...)
	}
	if hasErrors(typeErrors) {
		return nil, true, NewCompileError("", diagnostic.TypeError, typeErrors)
	}
	if len(compilerOwnedRuntimeSourceUnits(checkedPrograms, programs, options)) > 0 {
		return nil, false, nil
	}

	integrationSources := make([]projectintegration.Source, 0, len(units))
	for _, source := range units {
		integrationSources = append(integrationSources, projectintegration.Source{
			Filename: source.Filename, ModulePath: source.ModulePath, Program: programs[source.ModulePath],
		})
	}
	integrations, integrationIssues, err := projectintegration.Analyze(projectintegration.Context{
		Sources: integrationSources, Resolutions: resolutions, EntrypointModule: ownerModule,
		SourceRoot: options.SourceRoot, ProjectRoot: projectRoot(options), PackageOptions: options.PackageOptions,
		PackageAliasesByModule: packageAliasesByModule, JobsConfiguration: options.JobsConfiguration,
	})
	if err != nil {
		return nil, true, err
	}
	if len(integrationIssues) > 0 {
		items := make([]diagnostic.Diagnostic, 0, len(integrationIssues))
		for _, issue := range integrationIssues {
			items = append(items, diagnostic.Diagnostic{
				Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error, Message: issue.Message,
				Path: issue.Filename, Span: issue.Span,
			})
		}
		return nil, true, NewCompileError("", diagnostic.ProjectIntegration, items)
	}

	loweredPrograms := make([]*ir.Program, 0, len(units))
	for _, source := range units {
		lowered := lower.Program(checkedPrograms[source.ModulePath])
		lowered.SourcePath = source.Filename
		integrations.Apply(lowered, source.ModulePath == ownerModule)
		loweredPrograms = append(loweredPrograms, lowered)
	}
	if validateBackend {
		if err := codegen.ValidateProject(loweredPrograms); err != nil {
			return nil, true, err
		}
	}
	artifacts := make([]*Artifact, 0, len(units))
	for index, source := range units {
		artifacts = append(artifacts, &Artifact{
			Filename: source.Filename, Mode: options.Mode, AST: programs[source.ModulePath], IR: loweredPrograms[index],
			CompilerOwned: source.CompilerOwned, Official: source.Official, ExternalPackage: source.ExternalPackage,
		})
	}
	return &projectAnalysis{
		artifacts: artifacts, requestedUnits: cloneSourceUnits(sources), units: units, options: cloneOptions(options),
		programs: programs, catalog: catalog, declarations: declarations, resolutions: resolutions,
		checkedPrograms: checkedPrograms, validateBackend: validateBackend,
	}, true, nil
}

func singleSourceChange(previous *projectAnalysis, sources []SourceUnit, options Options, validateBackend bool) (SourceUnit, bool) {
	if previous == nil || previous.validateBackend != validateBackend || !equalOptions(previous.options, options) {
		return SourceUnit{}, false
	}
	if len(previous.requestedUnits) != len(sources) {
		return SourceUnit{}, false
	}
	previousByModule := make(map[string]SourceUnit, len(previous.requestedUnits))
	for _, source := range previous.requestedUnits {
		if _, duplicate := previousByModule[source.ModulePath]; duplicate {
			return SourceUnit{}, false
		}
		previousByModule[source.ModulePath] = source
	}
	var changed SourceUnit
	changes := 0
	seen := make(map[string]bool, len(sources))
	for _, source := range sources {
		if seen[source.ModulePath] {
			return SourceUnit{}, false
		}
		seen[source.ModulePath] = true
		cached, exists := previousByModule[source.ModulePath]
		if !exists || !equalSourceUnitMetadata(cached, source) {
			return SourceUnit{}, false
		}
		if !bytes.Equal(cached.Source, source.Source) {
			changed = source
			changes++
		}
	}
	if changes != 1 || changed.CompilerOwned || changed.Official || changed.ExternalPackage {
		return SourceUnit{}, false
	}
	return changed, true
}

func affectedProjectModules(previous *projectAnalysis, catalog *resolver.Catalog, declarations *declaration.Catalog, changedModule string) map[string]bool {
	affected := map[string]bool{changedModule: true}
	downstreamChanges := map[string]bool{}
	previousChanged := previous.catalog.Modules[changedModule]
	currentChanged := catalog.Modules[changedModule]
	if previousChanged == nil || currentChanged == nil || !reflect.DeepEqual(previousChanged.Exports, currentChanged.Exports) {
		for modulePath, module := range catalog.Modules {
			previousModule := previous.catalog.Modules[modulePath]
			if previousModule == nil || !reflect.DeepEqual(previousModule.Exports, module.Exports) {
				affected[modulePath] = true
				downstreamChanges[modulePath] = true
			}
		}
	}
	if !reflect.DeepEqual(previous.declarations, declarations) {
		for modulePath := range catalog.Modules {
			affected[modulePath] = true
		}
		return affected
	}
	for {
		changed := false
		for owner, resolution := range previous.resolutions {
			if downstreamChanges[owner] {
				continue
			}
			for _, imported := range resolution.Imports {
				if imported != nil && imported.Kind == resolver.ProjectImport && downstreamChanges[imported.Path] {
					affected[owner] = true
					downstreamChanges[owner] = true
					changed = true
					break
				}
			}
		}
		if !changed {
			return affected
		}
	}
}

func equalCompilerOwnedImports(left, right *ast.Program) bool {
	return equalStringSet(compilerOwnedImports(left), compilerOwnedImports(right))
}

func compilerOwnedImports(program *ast.Program) map[string]bool {
	result := map[string]bool{}
	if program == nil {
		return result
	}
	for _, statement := range program.Statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		if definition, exists := stdlib.Lookup(imported.Path); exists && definition.Source != "" && definition.ModulePath != "" {
			result[definition.ModulePath] = true
			continue
		}
		if bundled, exists := official.Lookup(imported.Path); exists && bundled.Definition.Source != "" && bundled.Definition.ModulePath != "" {
			result[bundled.Definition.ModulePath] = true
		}
	}
	return result
}

func equalRuntimeDependencies(left, right checker.Result) bool {
	leftSet := make(map[string]bool, len(left.RuntimeDependencies))
	for modulePath := range left.RuntimeDependencies {
		leftSet[modulePath] = true
	}
	rightSet := make(map[string]bool, len(right.RuntimeDependencies))
	for modulePath := range right.RuntimeDependencies {
		rightSet[modulePath] = true
	}
	return equalStringSet(leftSet, rightSet)
}

func equalStringSet(left, right map[string]bool) bool {
	if len(left) != len(right) {
		return false
	}
	for item := range left {
		if !right[item] {
			return false
		}
	}
	return true
}

func projectMainOwner(units []SourceUnit, programs map[string]*ast.Program) (string, error) {
	owner := ""
	ownerModule := ""
	for _, source := range units {
		if !hasTopLevelMethod(programs[source.ModulePath], MainFunction) {
			continue
		}
		if owner != "" {
			item := diagnostic.Diagnostic{
				Code: diagnostic.DuplicateBinding, Severity: diagnostic.Error, Message: "main is already declared",
				Path: source.Filename, Span: programs[source.ModulePath].Span(),
				Related: []diagnostic.RelatedInformation{{Message: "first declaration", Location: diagnostic.Location{Path: owner, Span: programs[ownerModule].Span()}}},
			}
			return "", NewCompileError(source.Filename, diagnostic.TypeError, []diagnostic.Diagnostic{item})
		}
		owner = source.Filename
		ownerModule = source.ModulePath
	}
	return ownerModule, nil
}

func cloneSourceUnits(units []SourceUnit) []SourceUnit {
	result := make([]SourceUnit, len(units))
	for index, unit := range units {
		result[index] = cloneSourceUnit(unit)
	}
	return result
}

func cloneSourceUnit(unit SourceUnit) SourceUnit {
	unit.Source = append([]byte(nil), unit.Source...)
	unit.PackageAliases = cloneStringMap(unit.PackageAliases)
	return unit
}

func equalSourceUnitMetadata(left, right SourceUnit) bool {
	return left.Filename == right.Filename && left.ModulePath == right.ModulePath && left.Package == right.Package &&
		left.CompilerOwned == right.CompilerOwned && left.Official == right.Official && left.ExternalPackage == right.ExternalPackage &&
		left.TestRegistration == right.TestRegistration && left.MainReplacement == right.MainReplacement &&
		equalStringMap(left.PackageAliases, right.PackageAliases)
}

func cloneOptions(options Options) Options {
	options.PackageOptions = cloneBytesMap(options.PackageOptions)
	options.PackageAliases = cloneStringMap(options.PackageAliases)
	return options
}

func equalOptions(left, right Options) bool {
	return left.Mode == right.Mode && left.Package == right.Package && left.ModulePath == right.ModulePath &&
		left.GoModule == right.GoModule && left.RubyLoader == right.RubyLoader && left.TypeScriptRuntime == right.TypeScriptRuntime &&
		left.SourceRoot == right.SourceRoot && left.ProjectRoot == right.ProjectRoot && left.JobsConfiguration == right.JobsConfiguration &&
		left.AllowUnusedImports == right.AllowUnusedImports && left.InteractiveModule == right.InteractiveModule &&
		left.NativePackages == right.NativePackages && equalBytesMap(left.PackageOptions, right.PackageOptions) &&
		equalStringMap(left.PackageAliases, right.PackageAliases)
}

func cloneBytesMap(items map[string][]byte) map[string][]byte {
	if items == nil {
		return nil
	}
	result := make(map[string][]byte, len(items))
	for key, value := range items {
		result[key] = append([]byte(nil), value...)
	}
	return result
}

func equalBytesMap(left, right map[string][]byte) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if other, exists := right[key]; !exists || !bytes.Equal(value, other) {
			return false
		}
	}
	return true
}

func cloneStringMap(items map[string]string) map[string]string {
	if items == nil {
		return nil
	}
	result := make(map[string]string, len(items))
	for key, value := range items {
		result[key] = value
	}
	return result
}

func equalStringMap(left, right map[string]string) bool {
	if len(left) != len(right) {
		return false
	}
	for key, value := range left {
		if other, exists := right[key]; !exists || other != value {
			return false
		}
	}
	return true
}
