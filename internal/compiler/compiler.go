// Package compiler owns the public compilation pipeline.
package compiler

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/lower"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/typeprovider"
	"github.com/type-rb/type-rb/internal/web"
)

type Artifact struct {
	Filename      string
	Mode          string
	AST           *ast.Program
	IR            *ir.Program
	Output        []byte
	CompilerOwned bool
	Official      bool
}

type SourceUnit struct {
	Filename      string
	Source        []byte
	ModulePath    string
	Package       string
	CompilerOwned bool
	Official      bool
}

type CompileError struct {
	Filename    string
	Diagnostics []diagnostic.Diagnostic
}

type Options struct {
	Mode               string
	Package            string
	ModulePath         string
	GoModule           string
	RubyLoader         string
	SourceRoot         string
	ProjectRoot        string
	AllowUnusedImports bool
}

const MainFunction = "main"

func (e *CompileError) Error() string {
	if len(e.Diagnostics) == 0 {
		return "compilation failed"
	}
	first := e.Diagnostics[0]
	return fmt.Sprintf("%s:%d:%d: %s: %s", e.Filename, first.Span.Start.Line, first.Span.Start.Column, first.Severity, first.Message)
}

func Compile(filename string, source []byte, mode string) (*Artifact, error) {
	options := Options{Mode: mode}
	if mode == "go" {
		options.Package = "main"
	}
	return CompileWithOptions(filename, source, options)
}

func CompileWithOptions(filename string, source []byte, options Options) (*Artifact, error) {
	program, diagnostics := parser.Parse(source)
	configureProgram(program, options, options.ModulePath, options.Package)
	if options.Mode == "" {
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.Error,
			Message:  "project mode is missing from trbconfig.jsonc",
			Span:     token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 1, Column: 1}},
		})
	} else if options.Mode != "ruby" && options.Mode != "typescript" && options.Mode != "go" {
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Severity: diagnostic.Error,
			Message:  fmt.Sprintf("unsupported mode %q", options.Mode),
			Span:     program.Span(),
		})
	}
	declarations, providerErr := typeprovider.Load([]*ast.Program{program}, typeprovider.Context{ProjectRoot: projectRoot(options)})
	if providerErr != nil {
		return nil, providerErr
	}
	resolved, resolveDiagnostics := resolver.Resolve(program, resolver.Options{Mode: options.Mode, SourceRoot: options.SourceRoot, Filename: filename, Declarations: declarations})
	diagnostics = append(diagnostics, resolveDiagnostics...)
	checked, checkDiagnostics := checker.CheckWithOptions(program, resolved, checker.Options{AllowUnusedImports: options.AllowUnusedImports})
	diagnostics = append(diagnostics, checkDiagnostics...)
	if hasErrors(diagnostics) {
		return nil, &CompileError{Filename: filename, Diagnostics: diagnostics}
	}
	lowered := lower.Program(checked)
	output, err := codegen.Generate(lowered)
	if err != nil {
		return nil, err
	}
	return &Artifact{Filename: filename, Mode: options.Mode, AST: program, IR: lowered, Output: output}, nil
}

// CompileProject parses every unit before resolving or checking any body. This
// establishes a stable project catalog, import graph, and exported signature
// environment shared by all files.
func CompileProject(sources []SourceUnit, options Options) ([]*Artifact, error) {
	units := append([]SourceUnit(nil), sources...)
	sort.Slice(units, func(i, j int) bool { return units[i].ModulePath < units[j].ModulePath })
	programs := make(map[string]*ast.Program, len(units))
	for _, source := range units {
		program, diagnostics := parser.Parse(source.Source)
		configureProgram(program, options, source.ModulePath, source.Package)
		if official.OwnsModule(source.ModulePath) && !source.Official {
			diagnostics = append(diagnostics, diagnostic.Diagnostic{
				Severity: diagnostic.Error,
				Message:  fmt.Sprintf("module path %s is reserved for TypeRB packages", source.ModulePath),
				Span:     program.Span(),
			})
		}
		diagnostics = append(diagnostics, modeDiagnostics(program, options.Mode)...)
		if hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
		programs[source.ModulePath] = program
	}
	for {
		dependencies := dependencySourceUnits(programs, options)
		if len(dependencies) == 0 {
			break
		}
		for _, source := range dependencies {
			program, diagnostics := parser.Parse(source.Source)
			configureProgram(program, options, source.ModulePath, source.Package)
			diagnostics = append(diagnostics, modeDiagnostics(program, options.Mode)...)
			if hasErrors(diagnostics) {
				return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
			}
			units = append(units, source)
			programs[source.ModulePath] = program
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].ModulePath < units[j].ModulePath })

	modules := make([]resolver.Module, 0, len(units))
	for _, source := range units {
		modules = append(modules, resolver.Module{Path: source.ModulePath, Filename: source.Filename, Program: programs[source.ModulePath], CompilerOwned: source.CompilerOwned})
	}
	catalog, catalogDiagnostics := resolver.NewCatalog(modules)
	for _, source := range units {
		if diagnostics := catalogDiagnostics[source.Filename]; hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
	}
	providerPrograms := make([]*ast.Program, 0, len(units))
	for _, source := range units {
		providerPrograms = append(providerPrograms, programs[source.ModulePath])
	}
	declarations, providerErr := typeprovider.Load(providerPrograms, typeprovider.Context{ProjectRoot: projectRoot(options)})
	if providerErr != nil {
		return nil, providerErr
	}

	resolutions := make(map[string]resolver.Result, len(units))
	for _, source := range units {
		resolved, diagnostics := resolver.Resolve(programs[source.ModulePath], resolver.Options{
			Mode:          options.Mode,
			SourceRoot:    options.SourceRoot,
			Filename:      source.Filename,
			CompilerOwned: source.CompilerOwned,
			Official:      source.Official,
			Catalog:       catalog,
			Declarations:  declarations,
		})
		if hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
		resolutions[source.ModulePath] = resolved
	}
	graphDiagnostics := resolver.ValidateImportGraph(catalog, resolutions)
	for _, source := range units {
		if diagnostics := graphDiagnostics[source.Filename]; hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
	}

	owner := ""
	ownerModule := ""
	for _, source := range units {
		if hasTopLevelMethod(programs[source.ModulePath], MainFunction) {
			if owner != "" {
				item := diagnostic.Diagnostic{Severity: diagnostic.Error, Message: fmt.Sprintf("main is already declared in %s", owner), Span: programs[source.ModulePath].Span()}
				return nil, &CompileError{Filename: source.Filename, Diagnostics: []diagnostic.Diagnostic{item}}
			}
			owner = source.Filename
			ownerModule = source.ModulePath
		}
	}

	checkedPrograms := make(map[string]checker.Result, len(units))
	checkDiagnostics := make(map[string][]diagnostic.Diagnostic, len(units))
	for _, source := range units {
		program := programs[source.ModulePath]
		checked, diagnostics := checker.CheckWithOptions(program, resolutions[source.ModulePath], checker.Options{AllowUnusedImports: options.AllowUnusedImports})
		checkedPrograms[source.ModulePath] = checked
		checkDiagnostics[source.ModulePath] = diagnostics
	}
	if runtimeUnits := compilerOwnedRuntimeSourceUnits(checkedPrograms, programs, options); len(runtimeUnits) > 0 {
		return CompileProject(append(units, runtimeUnits...), options)
	}
	for _, source := range units {
		if diagnostics := checkDiagnostics[source.ModulePath]; hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
	}
	webRoutes, err := projectWebRoutes(units, programs, options)
	if err != nil {
		return nil, err
	}

	artifacts := make([]*Artifact, 0, len(units))
	for _, source := range units {
		program := programs[source.ModulePath]
		checked := checkedPrograms[source.ModulePath]
		lowered := lower.Program(checked)
		if source.ModulePath == ownerModule {
			lowered.WebRoutes = webRoutes
		}
		output, err := codegen.Generate(lowered)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, &Artifact{Filename: source.Filename, Mode: options.Mode, AST: program, IR: lowered, Output: output, CompilerOwned: source.CompilerOwned, Official: source.Official})
	}
	return artifacts, nil
}

func projectWebRoutes(units []SourceUnit, programs map[string]*ast.Program, options Options) ([]ir.WebRoute, error) {
	if _, active := programs["trb/web/index"]; !active {
		return nil, nil
	}
	sources := make([]web.Source, 0, len(units))
	for _, source := range units {
		sources = append(sources, web.Source{Filename: source.Filename, ModulePath: source.ModulePath, Program: programs[source.ModulePath]})
	}
	routes, issues := web.Discover(sources, options.SourceRoot)
	if len(issues) > 0 {
		issue := issues[0]
		return nil, &CompileError{Filename: issue.Filename, Diagnostics: []diagnostic.Diagnostic{{Severity: diagnostic.Error, Message: issue.Message, Span: issue.Span}}}
	}
	result := make([]ir.WebRoute, 0, len(routes))
	for _, route := range routes {
		result = append(result, ir.WebRoute{
			Method:         route.Method,
			Path:           route.Path,
			ModulePath:     route.ModulePath,
			Handler:        route.Handler,
			PathParameters: append([]string(nil), route.PathParameters...),
		})
	}
	return result, nil
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
}

func modeDiagnostics(program *ast.Program, mode string) []diagnostic.Diagnostic {
	if mode == "" {
		return []diagnostic.Diagnostic{{
			Severity: diagnostic.Error,
			Message:  "project mode is missing from trbconfig.jsonc",
			Span:     token.Span{Start: token.Position{Line: 1, Column: 1}, End: token.Position{Line: 1, Column: 1}},
		}}
	}
	if mode != "ruby" && mode != "typescript" && mode != "go" {
		return []diagnostic.Diagnostic{{Severity: diagnostic.Error, Message: fmt.Sprintf("unsupported mode %q", mode), Span: program.Span()}}
	}
	return nil
}

func hasTopLevelMethod(program *ast.Program, name string) bool {
	for _, statement := range program.Statements {
		if method, ok := statement.(*ast.MethodStatement); ok && method.Name == name {
			return true
		}
	}
	return false
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
