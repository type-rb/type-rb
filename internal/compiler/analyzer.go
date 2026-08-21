package compiler

import (
	"bytes"
	"sync"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/checker"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/official"
	"github.com/type-rb/type-rb/internal/parser"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/testsuite"
)

// Analyzer retains immutable compiler inputs that can be reused by repeated
// project analysis. AnalyzeProject remains the one-shot compatibility entry
// point; long-lived clients such as the compiler service and REPL should own
// one Analyzer for the lifetime of their project snapshot.
type Analyzer struct {
	analysisMu sync.Mutex
	mu         sync.Mutex
	cache      map[parseCacheIdentity]parseCacheEntry
	state      *projectAnalysis
	parse      func([]byte) (*ast.Program, []diagnostic.Diagnostic)
	check      func(*ast.Program, resolver.Result, checker.Options) (checker.Result, []diagnostic.Diagnostic)
}

type parseCacheIdentity struct {
	filename   string
	modulePath string
	initial    bool
}

type parseCacheEntry struct {
	unit        SourceUnit
	options     parseOptions
	program     *ast.Program
	diagnostics []diagnostic.Diagnostic
}

type parseOptions struct {
	mode              string
	goModule          string
	rubyLoader        string
	typeScriptRuntime string
}

func NewAnalyzer() *Analyzer {
	return &Analyzer{
		cache: map[parseCacheIdentity]parseCacheEntry{},
		parse: parser.Parse,
		check: checker.CheckWithOptions,
	}
}

// AnalyzeProject returns checked semantic artifacts without backend output,
// reusing prepared syntax trees and safe interactive analysis state.
func (a *Analyzer) AnalyzeProject(sources []SourceUnit, options Options) ([]*Artifact, error) {
	a.analysisMu.Lock()
	defer a.analysisMu.Unlock()

	if analysis, handled, err := analyzeInteractiveProject(a, a.state, sources, options, true); handled {
		if err == nil {
			a.state = analysis
			return analysis.artifacts, nil
		}
		return nil, err
	}
	analysis, err := analyzeProjectFull(a, sources, options, true, sources)
	if err != nil {
		return nil, err
	}
	a.state = analysis
	return analysis.artifacts, nil
}

func (a *Analyzer) checkProgram(program *ast.Program, resolution resolver.Result, options checker.Options) (checker.Result, []diagnostic.Diagnostic) {
	a.mu.Lock()
	if a.check == nil {
		a.check = checker.CheckWithOptions
	}
	check := a.check
	a.mu.Unlock()
	return check(program, resolution, options)
}

func (a *Analyzer) parseUnit(unit SourceUnit, options Options, initial bool) (*ast.Program, []diagnostic.Diagnostic) {
	identity := parseCacheIdentity{filename: unit.Filename, modulePath: unit.ModulePath, initial: initial}
	configured := parseOptions{
		mode: options.Mode, goModule: options.GoModule, rubyLoader: options.RubyLoader, typeScriptRuntime: options.TypeScriptRuntime,
	}

	a.mu.Lock()
	if a.cache == nil {
		a.cache = map[parseCacheIdentity]parseCacheEntry{}
	}
	if a.parse == nil {
		a.parse = parser.Parse
	}
	cached, exists := a.cache[identity]
	if exists && equalParsedUnit(cached.unit, unit) && cached.options == configured {
		program := cached.program
		diagnostics := cloneParseDiagnostics(cached.diagnostics)
		a.mu.Unlock()
		return program, diagnostics
	}
	parse := a.parse
	a.mu.Unlock()

	program, diagnostics := parse(unit.Source)
	configureProgram(program, options, unit.ModulePath, unit.Package)
	if initial {
		if unit.MainReplacement != "" {
			renameTopLevelMethod(program, MainFunction, unit.MainReplacement)
		}
		_, testDiagnostics := testsuite.Prepare(program, unit.Filename, unit.TestRegistration)
		diagnostics = append(diagnostics, testDiagnostics...)
		if official.OwnsModule(unit.ModulePath) && !unit.Official {
			diagnostics = append(diagnostics, diagnostic.Diagnostic{
				Code:     diagnostic.ProjectError,
				Severity: diagnostic.Error,
				Message:  "module path " + unit.ModulePath + " is reserved for TypeRB packages",
				Span:     program.Span(),
			})
		}
	}
	diagnostics = diagnostic.Normalize(append(diagnostics, modeDiagnostics(program, options.Mode)...), unit.Filename, diagnostic.SyntaxError)

	entry := parseCacheEntry{
		unit: cloneParsedUnit(unit), options: configured, program: program,
		diagnostics: cloneParseDiagnostics(diagnostics),
	}
	a.mu.Lock()
	a.cache[identity] = entry
	a.mu.Unlock()
	return program, cloneParseDiagnostics(diagnostics)
}

func equalParsedUnit(left, right SourceUnit) bool {
	return left.Filename == right.Filename && left.ModulePath == right.ModulePath && left.Package == right.Package &&
		left.CompilerOwned == right.CompilerOwned && left.Official == right.Official && left.ExternalPackage == right.ExternalPackage &&
		left.TestRegistration == right.TestRegistration && left.MainReplacement == right.MainReplacement && bytes.Equal(left.Source, right.Source)
}

func cloneParsedUnit(unit SourceUnit) SourceUnit {
	unit.Source = append([]byte(nil), unit.Source...)
	return unit
}

func cloneParseDiagnostics(items []diagnostic.Diagnostic) []diagnostic.Diagnostic {
	result := append([]diagnostic.Diagnostic(nil), items...)
	for index := range result {
		result[index].Related = append([]diagnostic.RelatedInformation(nil), result[index].Related...)
		result[index].Fixes = append([]diagnostic.Fix(nil), result[index].Fixes...)
		for fixIndex := range result[index].Fixes {
			result[index].Fixes[fixIndex].Edits = append([]diagnostic.TextEdit(nil), result[index].Fixes[fixIndex].Edits...)
		}
	}
	return result
}
