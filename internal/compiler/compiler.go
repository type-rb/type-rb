// Package compiler owns the public compilation pipeline.
package compiler

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/lower"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/typeprovider"
)

type Artifact struct {
	Filename string
	Mode     string
	AST      *ast.Program
	IR       *ir.Program
	Output   []byte
}

type SourceUnit struct {
	Filename   string
	Source     []byte
	ModulePath string
	Package    string
}

type CompileError struct {
	Filename    string
	Diagnostics []diagnostic.Diagnostic
}

type Options struct {
	Mode        string
	Package     string
	ModulePath  string
	GoModule    string
	RubyLoader  string
	SourceRoot  string
	ProjectRoot string
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
	checked, checkDiagnostics := checker.Check(program, resolved)
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
		diagnostics = append(diagnostics, modeDiagnostics(program, options.Mode)...)
		if hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
		programs[source.ModulePath] = program
	}

	modules := make([]resolver.Module, 0, len(units))
	for _, source := range units {
		modules = append(modules, resolver.Module{Path: source.ModulePath, Filename: source.Filename, Program: programs[source.ModulePath]})
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
			Mode:         options.Mode,
			SourceRoot:   options.SourceRoot,
			Filename:     source.Filename,
			Catalog:      catalog,
			Declarations: declarations,
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
	for _, source := range units {
		if hasTopLevelMethod(programs[source.ModulePath], MainFunction) {
			if owner != "" {
				item := diagnostic.Diagnostic{Severity: diagnostic.Error, Message: fmt.Sprintf("main is already declared in %s", owner), Span: programs[source.ModulePath].Span()}
				return nil, &CompileError{Filename: source.Filename, Diagnostics: []diagnostic.Diagnostic{item}}
			}
			owner = source.Filename
		}
	}

	artifacts := make([]*Artifact, 0, len(units))
	for _, source := range units {
		program := programs[source.ModulePath]
		checked, diagnostics := checker.Check(program, resolutions[source.ModulePath])
		if hasErrors(diagnostics) {
			return nil, &CompileError{Filename: source.Filename, Diagnostics: diagnostics}
		}
		lowered := lower.Program(checked)
		output, err := codegen.Generate(lowered)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, &Artifact{Filename: source.Filename, Mode: options.Mode, AST: program, IR: lowered, Output: output})
	}
	return artifacts, nil
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
