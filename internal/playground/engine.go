package playground

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/formatter"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/repl"
	"github.com/type-rb/type-rb/internal/types"
)

const evaluationLimit = 3 * time.Second

type Request struct {
	Source string `json:"source"`
	Mode   string `json:"mode"`
}

type Response struct {
	OK          bool         `json:"ok"`
	Output      string       `json:"output,omitempty"`
	Generated   string       `json:"generated,omitempty"`
	Formatted   string       `json:"formatted,omitempty"`
	Diagnostics []Diagnostic `json:"diagnostics,omitempty"`
	DurationMS  int64        `json:"durationMs"`
}

type Diagnostic struct {
	Severity  string `json:"severity"`
	Message   string `json:"message"`
	Line      int    `json:"line,omitempty"`
	Column    int    `json:"column,omitempty"`
	EndLine   int    `json:"endLine,omitempty"`
	EndColumn int    `json:"endColumn,omitempty"`
}

type Config struct {
	Transport string   `json:"transport"`
	Mode      string   `json:"mode"`
	Modes     []string `json:"modes"`
	Page      string   `json:"page,omitempty"`
	Version   string   `json:"version"`
}

func Run(parent context.Context, source, mode string) Response {
	started := time.Now()
	result := evaluate(parent, source, mode)
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func Transpile(source, mode string) Response {
	started := time.Now()
	result := transpile(source, mode)
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func Format(source string) Response {
	started := time.Now()
	result := format(source)
	result.DurationMS = time.Since(started).Milliseconds()
	return result
}

func ValidMode(mode string) bool {
	return mode == "go" || mode == "ruby" || mode == "typescript"
}

func transpile(source, mode string) Response {
	_, session, err := compile(source, mode)
	if err != nil {
		return errorResponse(err)
	}
	return Response{OK: true, Generated: string(session.Output)}
}

func evaluate(parent context.Context, source, mode string) Response {
	programs, session, err := compile(source, mode)
	if err != nil {
		return errorResponse(err)
	}
	if item := disallowedRuntimeImport(session.AST); item != "" {
		return Response{OK: false, Generated: string(session.Output), Diagnostics: []Diagnostic{{
			Severity: "error", Message: fmt.Sprintf("%s is not available in the browser playground", item),
		}}}
	}

	var output bytes.Buffer
	evaluator := repl.NewEvaluator(&output, mode)
	if err := evaluator.LoadProject(programs, session.IR.ModulePath); err != nil {
		return runtimeError(err, string(session.Output))
	}
	evaluator.LoadDefinitions(session.IR)
	ctx, cancel := context.WithTimeout(parent, evaluationLimit)
	defer cancel()
	result, err := evaluator.EvaluateContext(ctx, session.IR.Statements, session.IR.ModulePath)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			err = fmt.Errorf("evaluation exceeded %s", evaluationLimit)
		}
		return runtimeError(err, string(session.Output))
	}
	if result.Display && result.Value.Type.Kind != types.Void {
		fmt.Fprintf(&output, "%s : %s\n", repl.Inspect(result.Value), result.Value.Type.String())
	}
	return Response{OK: true, Output: output.String(), Generated: string(session.Output)}
}

func format(source string) Response {
	formatted, diagnostics := formatter.Format([]byte(source))
	if hasDiagnosticErrors(diagnostics) {
		return Response{OK: false, Diagnostics: diagnosticsFrom(diagnostics)}
	}
	return Response{OK: true, Formatted: string(formatted)}
}

func compile(source, mode string) ([]*ir.Program, *compiler.Artifact, error) {
	if !ValidMode(mode) {
		return nil, nil, fmt.Errorf("mode must be go, ruby, or typescript; got %q", mode)
	}
	packageName := ""
	if mode == "go" {
		packageName = "main"
	}
	const modulePath = "playground"
	artifacts, err := compiler.CompileProject([]compiler.SourceUnit{{
		Filename: "playground.trb", Source: []byte(source), ModulePath: modulePath, Package: packageName,
	}}, compiler.Options{
		Mode: mode, Package: packageName, ModulePath: modulePath, GoModule: "trb.local/playground", SourceRoot: ".", ProjectRoot: ".",
	})
	if err != nil {
		return nil, nil, err
	}
	programs := make([]*ir.Program, 0, len(artifacts))
	var session *compiler.Artifact
	for _, artifact := range artifacts {
		programs = append(programs, artifact.IR)
		if artifact.IR.ModulePath == modulePath {
			session = artifact
		}
	}
	if session == nil {
		return nil, nil, errors.New("compiler did not return the playground source")
	}
	return programs, session, nil
}

func disallowedRuntimeImport(program *ast.Program) string {
	for _, statement := range program.Statements {
		imported, ok := statement.(*ast.ImportStatement)
		if !ok {
			continue
		}
		if imported.Path == "trb/std/filesystem" || imported.Path == "trb/std/process" || strings.HasPrefix(imported.Path, "trb/platform/") {
			return imported.Path
		}
	}
	return ""
}

func errorResponse(err error) Response {
	var compilation *compiler.CompileError
	if errors.As(err, &compilation) {
		return Response{OK: false, Diagnostics: diagnosticsFrom(compilation.Diagnostics)}
	}
	return runtimeError(err, "")
}

func runtimeError(err error, generated string) Response {
	return Response{OK: false, Generated: generated, Diagnostics: []Diagnostic{{Severity: "error", Message: err.Error()}}}
}

func diagnosticsFrom(diagnostics []diagnostic.Diagnostic) []Diagnostic {
	result := make([]Diagnostic, 0, len(diagnostics))
	for _, item := range diagnostics {
		result = append(result, Diagnostic{
			Severity: string(item.Severity), Message: item.Message,
			Line: item.Span.Start.Line, Column: item.Span.Start.Column, EndLine: item.Span.End.Line, EndColumn: item.Span.End.Column,
		})
	}
	return result
}

func hasDiagnosticErrors(diagnostics []diagnostic.Diagnostic) bool {
	for _, item := range diagnostics {
		if item.Severity == diagnostic.Error {
			return true
		}
	}
	return false
}
