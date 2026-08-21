// Package compiler owns the public compilation pipeline.
package compiler

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/lower"
	"github.com/type-rb/type-rb/internal/nativepackage"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/projectintegration"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/sourcemap"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/typeprovider"
)

type Artifact struct {
	Filename        string
	Mode            string
	AST             *ast.Program
	IR              *ir.Program
	Output          []byte
	SourceMap       sourcemap.Map
	CompilerOwned   bool
	Official        bool
	ExternalPackage bool
	// CompilerGeneratedStart is the byte offset at which virtual package
	// extension source begins. Interactive consumers use it to distinguish
	// authored submissions from helper declarations loaded as definitions.
	CompilerGeneratedStart int
	sourceUnit             SourceUnit
}

type SourceUnit struct {
	Filename        string
	Source          []byte
	ModulePath      string
	Package         string
	PackageAliases  map[string]string
	CompilerOwned   bool
	Official        bool
	ExternalPackage bool
	// TestRegistration moves top-level test suites into this generated
	// function. An empty value keeps ordinary source behavior.
	TestRegistration string
	// MainReplacement disables automatic application startup while preserving
	// the rest of a module for a test build.
	MainReplacement string
	// CompilerGeneratedSources are virtual TypeRB fragments returned by a
	// package extension. They are parsed, resolved, checked, and lowered with
	// the owning module but remain distinct from the authored source for
	// diagnostics and incremental cache identity.
	CompilerGeneratedSources []CompilerGeneratedSource
}

type CompilerGeneratedSource struct {
	ID     string
	Source []byte
	Origin token.Span
}

type CompileError struct {
	Filename    string
	Diagnostics []diagnostic.Diagnostic
}

func NewCompileError(filename string, fallback diagnostic.Code, items []diagnostic.Diagnostic) *CompileError {
	items = diagnostic.Normalize(items, filename, fallback)
	if filename == "" && len(items) > 0 {
		filename = items[0].Path
	}
	return &CompileError{Filename: filename, Diagnostics: items}
}

type Options struct {
	Mode               string
	Package            string
	ModulePath         string
	GoModule           string
	RubyLoader         string
	TypeScriptRuntime  string
	SourceRoot         string
	ProjectRoot        string
	PackageOptions     map[string][]byte
	PackageAliases     map[string]string
	JobsConfiguration  string
	AllowUnusedImports bool
	InteractiveModule  string
	NativePackages     *nativepackage.Catalog
}

const MainFunction = "main"

func (e *CompileError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "compilation failed"
	}
	first := e.Diagnostics[0]
	path := first.Path
	if path == "" {
		path = e.Filename
	}
	return fmt.Sprintf("%s:%d:%d: %s[%s]: %s", path, first.Span.Start.Line, first.Span.Start.Column, first.Severity, first.Code, first.Message)
}

func Compile(filename string, source []byte, mode string) (*Artifact, error) {
	options := Options{Mode: mode}
	if mode == "go" {
		options.Package = "main"
	}
	return CompileWithOptions(filename, source, options)
}

func CompileWithOptions(filename string, source []byte, options Options) (*Artifact, error) {
	return compileSourceUnit(SourceUnit{
		Filename: filename, Source: source, ModulePath: options.ModulePath, Package: options.Package,
		PackageAliases: options.PackageAliases,
	}, options)
}

func compileSourceUnit(unit SourceUnit, options Options) (*Artifact, error) {
	program, parseDiagnostics := parser.Parse(sourceUnitContents(unit))
	configureProgram(program, options, options.ModulePath, options.Package)
	if options.Mode == "" {
		parseDiagnostics = append(parseDiagnostics, diagnostic.Diagnostic{
			Code:     diagnostic.ProjectError,
			Severity: diagnostic.Error,
			Message:  "project mode is missing from trbconfig.jsonc",
			Span:     token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 1, Column: 1}},
		})
	} else if options.Mode != "ruby" && options.Mode != "typescript" && options.Mode != "go" {
		parseDiagnostics = append(parseDiagnostics, diagnostic.Diagnostic{
			Code:     diagnostic.ProjectError,
			Severity: diagnostic.Error,
			Message:  fmt.Sprintf("unsupported mode %q", options.Mode),
			Span:     program.Span(),
		})
	}
	declarations, providerErr := typeprovider.Load([]*ast.Program{program}, typeprovider.Context{
		ProjectRoot:            projectRoot(options),
		PackageOptions:         options.PackageOptions,
		PackageAliasesByModule: map[string]map[string]string{program.ModulePath: options.PackageAliases},
	})
	if providerErr != nil {
		return nil, compileProviderError(providerErr, []SourceUnit{unit})
	}
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{
		Mode: options.Mode, SourceRoot: options.SourceRoot, Filename: unit.Filename, PackageAliases: options.PackageAliases,
		Declarations: declarations, NativePackages: options.NativePackages, CompilerGeneratedStart: compilerGeneratedStart(unit),
	})
	checked, checkDiagnostics := checker.CheckWithOptions(program, resolved, checker.Options{
		AllowUnusedImports:     options.AllowUnusedImports,
		InteractiveTopLevel:    options.InteractiveModule != "" && options.InteractiveModule == options.ModulePath,
		RunnableMain:           topLevelMethod(program, MainFunction),
		CompilerGeneratedStart: compilerGeneratedStart(unit),
	})
	checkedPrograms := map[string]checker.Result{program.ModulePath: checked}
	programs := map[string]*ast.Program{program.ModulePath: program}
	updated, specializationDiagnostics, specialized, err := applyCallSpecializations([]SourceUnit{unit}, programs, checkedPrograms)
	if err != nil {
		return nil, err
	}
	diagnostics := append([]diagnostic.Diagnostic(nil), specializationDiagnostics...)
	diagnostics = append(diagnostics, normalizeSourceDiagnostics(parseDiagnostics, unit, diagnostic.SyntaxError)...)
	diagnostics = append(diagnostics, normalizeSourceDiagnostics(resolveDiagnostics, unit, diagnostic.ResolutionError)...)
	diagnostics = append(diagnostics, normalizeSourceDiagnostics(checkDiagnostics, unit, diagnostic.TypeError)...)
	if specialized && len(specializationDiagnostics) == 0 {
		return compileSourceUnit(updated[0], options)
	}
	if hasErrors(diagnostics) {
		return nil, NewCompileError(unit.Filename, diagnostic.TypeError, diagnostics)
	}
	checked = checkedPrograms[program.ModulePath]
	lowered := lower.Program(checked)
	lowered.SourcePath = unit.Filename
	generated, err := codegen.Generate(lowered)
	if err != nil {
		return nil, err
	}
	return &Artifact{
		Filename: unit.Filename, Mode: options.Mode, AST: program, IR: lowered, Output: generated.Output,
		SourceMap: normalizeGeneratedSourceMap(generated.SourceMap, unit), CompilerGeneratedStart: compilerGeneratedStart(unit),
		sourceUnit: cloneSourceUnit(unit),
	}, nil
}

// CompileProject analyzes every source unit and then generates target-language
// output for the resulting typed IR. Editor integrations that do not consume
// generated output should use AnalyzeProject so diagnostics and semantic
// artifacts do not pay the backend generation cost.
func CompileProject(sources []SourceUnit, options Options) ([]*Artifact, error) {
	analysis, err := analyzeProjectFull(NewAnalyzer(), sources, options, false, sources)
	if err != nil {
		return nil, err
	}
	artifacts := analysis.artifacts
	programs := make([]*ir.Program, len(artifacts))
	for index, artifact := range artifacts {
		programs[index] = artifact.IR
	}
	outputs, err := codegen.GenerateProject(programs)
	if err != nil {
		return nil, err
	}
	for index, output := range outputs {
		artifacts[index].Output = output.Output
		artifacts[index].SourceMap = normalizeGeneratedSourceMap(output.SourceMap, artifacts[index].sourceUnit)
	}
	return artifacts, nil
}

// AnalyzeProject parses every unit before resolving or checking any body. It
// establishes the complete project catalog, import graph, exported signature
// environment, package integrations, typed IR, and backend-owned validation
// without generating backend source. Returned artifacts therefore have empty
// Output and SourceMap fields.
func AnalyzeProject(sources []SourceUnit, options Options) ([]*Artifact, error) {
	return NewAnalyzer().AnalyzeProject(sources, options)
}

type projectAnalysis struct {
	artifacts       []*Artifact
	requestedUnits  []SourceUnit
	units           []SourceUnit
	options         Options
	programs        map[string]*ast.Program
	resolutions     map[string]resolver.Result
	checkedPrograms map[string]checker.Result
	validateBackend bool
}

func analyzeProjectFull(analyzer *Analyzer, sources []SourceUnit, options Options, validateBackend bool, requestedUnits []SourceUnit) (*projectAnalysis, error) {
	units := append([]SourceUnit(nil), sources...)
	sort.Slice(units, func(i, j int) bool { return units[i].ModulePath < units[j].ModulePath })
	programs := make(map[string]*ast.Program, len(units))
	var parseDiagnostics []diagnostic.Diagnostic
	for _, source := range units {
		program, diagnostics := analyzer.parseUnit(source, options, true)
		parseDiagnostics = append(parseDiagnostics, diagnostics...)
		programs[source.ModulePath] = program
	}
	if hasErrors(parseDiagnostics) {
		return nil, NewCompileError("", diagnostic.SyntaxError, parseDiagnostics)
	}
	for {
		dependencies := dependencySourceUnits(programs, options)
		if len(dependencies) == 0 {
			break
		}
		var dependencyDiagnostics []diagnostic.Diagnostic
		for _, source := range dependencies {
			program, diagnostics := analyzer.parseUnit(source, options, false)
			dependencyDiagnostics = append(dependencyDiagnostics, diagnostics...)
			units = append(units, source)
			programs[source.ModulePath] = program
		}
		if hasErrors(dependencyDiagnostics) {
			return nil, NewCompileError("", diagnostic.SyntaxError, dependencyDiagnostics)
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ModulePath < units[j].ModulePath })

	modules := make([]resolver.Module, 0, len(units))
	for _, source := range units {
		modules = append(modules, resolver.Module{Path: source.ModulePath, Filename: source.Filename, Program: programs[source.ModulePath], CompilerOwned: source.CompilerOwned, Official: source.Official})
	}
	catalog, catalogDiagnostics := resolver.NewCatalog(modules)
	var catalogErrors []diagnostic.Diagnostic
	for _, source := range units {
		catalogErrors = append(catalogErrors, catalogDiagnostics[source.Filename]...)
	}
	if hasErrors(catalogErrors) {
		return nil, NewCompileError("", diagnostic.ResolutionError, catalogErrors)
	}
	providerPrograms := make([]*ast.Program, 0, len(units))
	for _, source := range units {
		providerPrograms = append(providerPrograms, programs[source.ModulePath])
	}
	packageAliasesByModule := sourcePackageAliases(units, options.PackageAliases)
	declarations, providerErr := typeprovider.Load(providerPrograms, typeprovider.Context{
		ProjectRoot:            projectRoot(options),
		PackageOptions:         options.PackageOptions,
		PackageAliasesByModule: packageAliasesByModule,
	})
	if providerErr != nil {
		return nil, compileProviderError(providerErr, units)
	}

	resolutions := make(map[string]resolver.Result, len(units))
	var resolveErrors []diagnostic.Diagnostic
	for _, source := range units {
		packageAliases := options.PackageAliases
		if source.PackageAliases != nil {
			packageAliases = source.PackageAliases
		}
		resolved, diagnostics := resolver.Resolve(programs[source.ModulePath], resolver.Options{
			Mode:                   options.Mode,
			SourceRoot:             options.SourceRoot,
			Filename:               source.Filename,
			PackageAliases:         packageAliases,
			CompilerOwned:          source.CompilerOwned,
			Official:               source.Official,
			Catalog:                catalog,
			Declarations:           declarations,
			NativePackages:         options.NativePackages,
			CompilerGeneratedStart: compilerGeneratedStart(source),
		})
		resolveErrors = append(resolveErrors, normalizeSourceDiagnostics(diagnostics, source, diagnostic.ResolutionError)...)
		resolutions[source.ModulePath] = resolved
	}
	if hasErrors(resolveErrors) {
		return nil, NewCompileError("", diagnostic.ResolutionError, resolveErrors)
	}
	graphDiagnostics := resolver.ValidateImportGraph(catalog, resolutions)
	var graphErrors []diagnostic.Diagnostic
	for _, source := range units {
		graphErrors = append(graphErrors, graphDiagnostics[source.Filename]...)
	}
	if hasErrors(graphErrors) {
		return nil, NewCompileError("", diagnostic.ResolutionError, graphErrors)
	}

	owner := ""
	ownerModule := ""
	for _, source := range units {
		if hasTopLevelMethod(programs[source.ModulePath], MainFunction) {
			if owner != "" {
				item := diagnostic.Diagnostic{
					Code: diagnostic.DuplicateBinding, Severity: diagnostic.Error, Message: "main is already declared", Path: source.Filename, Span: programs[source.ModulePath].Span(),
					Related: []diagnostic.RelatedInformation{{Message: "first declaration", Location: diagnostic.Location{Path: owner, Span: programs[ownerModule].Span()}}},
				}
				return nil, NewCompileError(source.Filename, diagnostic.TypeError, []diagnostic.Diagnostic{item})
			}
			owner = source.Filename
			ownerModule = source.ModulePath
		}
	}

	checkedPrograms := make(map[string]checker.Result, len(units))
	checkDiagnostics := make(map[string][]diagnostic.Diagnostic, len(units))
	for _, source := range units {
		program := programs[source.ModulePath]
		checked, diagnostics := analyzer.checkProgram(program, resolutions[source.ModulePath], checker.Options{
			AllowUnusedImports:     options.AllowUnusedImports,
			InteractiveTopLevel:    options.InteractiveModule != "" && options.InteractiveModule == source.ModulePath,
			RunnableMain:           topLevelMethod(program, MainFunction),
			CompilerGeneratedStart: compilerGeneratedStart(source),
		})
		checkedPrograms[source.ModulePath] = checked
		checkDiagnostics[source.ModulePath] = diagnostics
	}
	if runtimeUnits := compilerOwnedRuntimeSourceUnits(checkedPrograms, programs, options); len(runtimeUnits) > 0 {
		return analyzeProjectFull(analyzer, append(units, runtimeUnits...), options, validateBackend, requestedUnits)
	}
	specializedUnits, specializationDiagnostics, specialized, err := applyCallSpecializations(units, programs, checkedPrograms)
	if err != nil {
		return nil, err
	}
	typeErrors := append([]diagnostic.Diagnostic(nil), specializationDiagnostics...)
	for _, source := range units {
		typeErrors = append(typeErrors, normalizeSourceDiagnostics(checkDiagnostics[source.ModulePath], source, diagnostic.TypeError)...)
	}
	if specialized && len(specializationDiagnostics) == 0 {
		return analyzeProjectFull(analyzer, specializedUnits, options, validateBackend, requestedUnits)
	}
	if hasErrors(typeErrors) {
		return nil, NewCompileError("", diagnostic.TypeError, typeErrors)
	}
	integrationSources := make([]projectintegration.Source, 0, len(units))
	for _, source := range units {
		integrationSources = append(integrationSources, projectintegration.Source{
			Filename:   source.Filename,
			ModulePath: source.ModulePath,
			Program:    programs[source.ModulePath],
		})
	}
	integrations, integrationIssues, err := projectintegration.Analyze(projectintegration.Context{
		Sources:                integrationSources,
		Resolutions:            resolutions,
		EntrypointModule:       ownerModule,
		SourceRoot:             options.SourceRoot,
		ProjectRoot:            projectRoot(options),
		PackageOptions:         options.PackageOptions,
		PackageAliasesByModule: packageAliasesByModule,
		JobsConfiguration:      options.JobsConfiguration,
	})
	if err != nil {
		return nil, err
	}
	if len(integrationIssues) > 0 {
		items := make([]diagnostic.Diagnostic, 0, len(integrationIssues))
		for _, issue := range integrationIssues {
			items = append(items, diagnostic.Diagnostic{Code: diagnostic.ProjectIntegration, Severity: diagnostic.Error, Message: issue.Message, Path: issue.Filename, Span: issue.Span})
		}
		return nil, NewCompileError("", diagnostic.ProjectIntegration, items)
	}

	loweredPrograms := make([]*ir.Program, 0, len(units))
	for _, source := range units {
		checked := checkedPrograms[source.ModulePath]
		lowered := lower.Program(checked)
		lowered.SourcePath = source.Filename
		integrations.Apply(lowered, source.ModulePath == ownerModule)
		loweredPrograms = append(loweredPrograms, lowered)
	}
	if validateBackend {
		if err := codegen.ValidateProject(loweredPrograms); err != nil {
			return nil, err
		}
	}
	artifacts := make([]*Artifact, 0, len(units))
	for index, source := range units {
		program := programs[source.ModulePath]
		artifacts = append(artifacts, &Artifact{
			Filename: source.Filename, Mode: options.Mode, AST: program, IR: loweredPrograms[index],
			CompilerOwned: source.CompilerOwned, Official: source.Official, ExternalPackage: source.ExternalPackage,
			CompilerGeneratedStart: compilerGeneratedStart(source), sourceUnit: cloneSourceUnit(source),
		})
	}
	return &projectAnalysis{
		artifacts: artifacts, requestedUnits: cloneSourceUnits(requestedUnits), units: cloneSourceUnits(units), options: cloneOptions(options),
		programs: programs, resolutions: resolutions, checkedPrograms: checkedPrograms, validateBackend: validateBackend,
	}, nil
}

type providerDiagnosticError interface {
	Diagnostics() []diagnostic.Diagnostic
}

func compileProviderError(err error, sources []SourceUnit) error {
	var diagnosed providerDiagnosticError
	if !errors.As(err, &diagnosed) {
		return err
	}
	filenames := make(map[string]string, len(sources))
	for _, source := range sources {
		filenames[source.ModulePath] = source.Filename
	}
	items := diagnosed.Diagnostics()
	for index := range items {
		items[index].Related = append([]diagnostic.RelatedInformation(nil), items[index].Related...)
		if filename, ok := filenames[items[index].Path]; ok {
			items[index].Path = filename
		}
		for relatedIndex := range items[index].Related {
			location := &items[index].Related[relatedIndex].Location
			if filename, ok := filenames[location.Path]; ok {
				location.Path = filename
			}
		}
	}
	return NewCompileError("", diagnostic.ProjectIntegration, items)
}

func sourcePackageAliases(units []SourceUnit, defaults map[string]string) map[string]map[string]string {
	aliases := make(map[string]map[string]string, len(units))
	for _, source := range units {
		moduleAliases := defaults
		if source.PackageAliases != nil {
			moduleAliases = source.PackageAliases
		}
		aliases[source.ModulePath] = moduleAliases
	}
	return aliases
}

func dependencySourceUnits(programs map[string]*ast.Program, options Options) []SourceUnit {
	definitions := map[string]*stdlib.Package{}
	officialDefinitions := map[string]*official.Package{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			imported, ok := statement.(*ast.ImportStatement)
			if !ok {
				continue
			}
			if definition, exists := stdlib.Lookup(imported.Path); exists && definition.Source != "" && definition.ModulePath != "" {
				if _, alreadyLoaded := programs[definition.ModulePath]; !alreadyLoaded {
					definitions[definition.ModulePath] = definition
				}
				continue
			}
			bundled, exists := official.Lookup(imported.Path)
			if !exists || bundled.Definition.Source == "" || bundled.Definition.ModulePath == "" {
				continue
			}
			if _, alreadyLoaded := programs[bundled.Definition.ModulePath]; !alreadyLoaded {
				officialDefinitions[bundled.Definition.ModulePath] = bundled
			}
		}
	}
	result := compilerOwnedPackageSourceUnits(definitions, options)
	result = append(result, officialPackageSourceUnits(officialDefinitions, options)...)
	return result
}

func officialPackageSourceUnits(definitions map[string]*official.Package, options Options) []SourceUnit {
	modulePaths := make([]string, 0, len(definitions))
	for modulePath := range definitions {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	root := projectRoot(options)
	if root == "" {
		root = "."
	}
	result := make([]SourceUnit, 0, len(modulePaths))
	for _, modulePath := range modulePaths {
		bundled := definitions[modulePath]
		result = append(result, SourceUnit{
			Filename:   filepath.Join(root, ".trb", "packages", filepath.FromSlash(modulePath)+".trb"),
			Source:     []byte(bundled.Definition.Source),
			ModulePath: modulePath,
			Package:    filepath.Base(filepath.Dir(filepath.FromSlash(modulePath))),
			Official:   true,
		})
	}
	return result
}

func compilerOwnedRuntimeSourceUnits(checkedPrograms map[string]checker.Result, programs map[string]*ast.Program, options Options) []SourceUnit {
	definitions := map[string]*stdlib.Package{}
	for _, checked := range checkedPrograms {
		for _, definition := range checked.RuntimeDependencies {
			if definition == nil || definition.Source == "" || definition.ModulePath == "" {
				continue
			}
			if _, alreadyLoaded := programs[definition.ModulePath]; alreadyLoaded {
				continue
			}
			definitions[definition.ModulePath] = definition
		}
	}
	return compilerOwnedPackageSourceUnits(definitions, options)
}

func compilerOwnedPackageSourceUnits(definitions map[string]*stdlib.Package, options Options) []SourceUnit {
	modulePaths := make([]string, 0, len(definitions))
	for modulePath := range definitions {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	root := projectRoot(options)
	if root == "" {
		root = "."
	}
	result := make([]SourceUnit, 0, len(modulePaths))
	for _, modulePath := range modulePaths {
		definition := definitions[modulePath]
		result = append(result, SourceUnit{
			Filename:      filepath.Join(root, ".trb", "stdlib", filepath.FromSlash(modulePath)+".trb"),
			Source:        []byte(definition.Source),
			ModulePath:    modulePath,
			Package:       filepath.Base(filepath.Dir(filepath.FromSlash(modulePath))),
			CompilerOwned: true,
		})
	}
	return result
}

func configureProgram(program *ast.Program, options Options, modulePath, packageName string) {
	program.Mode = options.Mode
	program.Package = packageName
	program.ModulePath = modulePath
	program.GoModule = options.GoModule
	program.RubyLoader = options.RubyLoader
	program.TypeScriptRuntime = options.TypeScriptRuntime
}

func modeDiagnostics(program *ast.Program, mode string) []diagnostic.Diagnostic {
	if mode == "" {
		return []diagnostic.Diagnostic{{
			Code:     diagnostic.ProjectError,
			Severity: diagnostic.Error,
			Message:  "project mode is missing from trbconfig.jsonc",
			Span:     token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 1, Column: 1}},
		}}
	}
	if mode != "ruby" && mode != "typescript" && mode != "go" {
		return []diagnostic.Diagnostic{{Code: diagnostic.ProjectError, Severity: diagnostic.Error, Message: fmt.Sprintf("unsupported mode %q", mode), Span: program.Span()}}
	}
	return nil
}

func hasTopLevelMethod(program *ast.Program, name string) bool {
	return topLevelMethod(program, name) != nil
}

func topLevelMethod(program *ast.Program, name string) *ast.MethodStatement {
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok && method.Name == name {
			return method
		}
	}
	return nil
}

func renameTopLevelMethod(program *ast.Program, name, replacement string) {
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok && method.Name == name {
			method.Name = replacement
		}
	}
}

func projectRoot(options Options) string {
	if options.ProjectRoot != "" {
		return options.ProjectRoot
	}
	return options.SourceRoot
}

func hasErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}
