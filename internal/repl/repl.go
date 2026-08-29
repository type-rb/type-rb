//go:build !js || !wasm

// Package repl implements TypeRB's project-aware interactive evaluator. Every
// accepted input is compiled through the normal project pipeline before its
// typed IR is evaluated.
package repl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/languageservice"
	"github.com/type-rb/type-rb/internal/types"
)

type Compilation struct {
	Session            *compiler.Artifact
	Artifacts          []*compiler.Artifact
	Programs           []*ir.Program
	HiddenPreludeLines int
}

type CompileFunc func(source string) (*Compilation, error)

type Options struct {
	Mode        string
	ProjectName string
	Version     string
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool
	HistoryFile string
	ProjectRoot string
	Compile     CompileFunc
	Initial     *Compilation
	Candidates  languageservice.Context
	language    *languageservice.Service
}

func Run(options Options) error {
	if options.Compile == nil {
		return errors.New("REPL compiler is not configured")
	}
	compilation := options.Initial
	if compilation == nil {
		var err error
		compilation, err = options.Compile("")
		if err != nil {
			return err
		}
	}
	options.Initial = compilation
	evaluator := NewEvaluator(options.Stdout, options.Mode)
	defer func() { _ = evaluator.Close() }()
	if err := evaluator.LoadProject(compilation.Programs, compilation.Session.IR.ModulePath); err != nil {
		return err
	}
	options.language = languageservice.New(options.Mode)
	options.language.SetCandidates(options.Candidates)
	options.language.Update(compilation.Programs, compilation.Session.IR.ModulePath)

	if options.Interactive {
		fmt.Fprintf(options.Stdout, "%s %s\n%s %s\n%s %s\n\n",
			colorize(true, colorTitle, "TypeRB"), colorize(true, colorMuted, options.Version),
			colorize(true, colorMuted, "project:"), colorize(true, colorName, options.ProjectName),
			colorize(true, colorMuted, "mode:"), colorize(true, colorType, options.Mode))
	}

	reader, err := newSubmissionReader(options)
	if err != nil {
		return err
	}
	defer reader.Close()

	source := ""
	statementCount := 0
	for {
		snippet, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if errors.Is(readErr, errInputInterrupted) {
			continue
		}
		if errors.Is(readErr, errIncompleteInput) {
			printReplError(options.Stderr, options.Interactive, "incomplete input")
			return nil
		}
		if readErr != nil {
			return readErr
		}
		trimmed := strings.TrimSpace(snippet)
		if trimmed == "" {
			continue
		}

		if strings.HasPrefix(trimmed, ":") {
			quit, replacement, nextCompilation := handleCommand(trimmed, source, options)
			if quit {
				return nil
			}
			if nextCompilation != nil {
				nextEvaluator := NewEvaluator(options.Stdout, options.Mode)
				if err := nextEvaluator.LoadProject(nextCompilation.Programs, nextCompilation.Session.IR.ModulePath); err != nil {
					_ = nextEvaluator.Close()
					printEvaluationError(options.Stderr, err, options.Interactive, options.ProjectRoot, nextCompilation)
					continue
				}
				nextEvaluator.LoadDefinitions(nextCompilation.Session.IR)
				if _, err := evaluateInterruptibly(nextEvaluator, authoredStatements(nextCompilation.Session), nextCompilation.Session.IR.ModulePath); err != nil {
					_ = nextEvaluator.Close()
					if errors.Is(err, context.Canceled) {
						printEvaluationInterrupted(options.Stdout, options.Interactive)
						continue
					}
					printReplError(options.Stderr, options.Interactive, err.Error())
					continue
				}
				source = replacement
				compilation = nextCompilation
				statementCount = len(authoredStatements(compilation.Session))
				_ = evaluator.Close()
				evaluator = nextEvaluator
				options.language.Update(compilation.Programs, compilation.Session.IR.ModulePath)
			}
			continue
		}

		candidate := appendSource(source, snippet)
		next, compileErr := options.Compile(candidate)
		if compileErr != nil {
			printCompileError(options.Stderr, compileErr, options.Interactive, options.ProjectRoot, sessionFilename(options.Initial))
			continue
		}
		if next.Session == nil || len(authoredStatements(next.Session)) < statementCount {
			printReplError(options.Stderr, options.Interactive, "compiler returned an invalid session")
			continue
		}

		evaluator.updateProjectDefinitions(compilation.Programs, next.Programs, next.Session.IR.ModulePath)
		if err := evaluator.configureRuntimeProviders(next.Programs); err != nil {
			printReplError(options.Stderr, options.Interactive, err.Error())
			continue
		}
		evaluator.LoadDefinitions(next.Session.IR)
		authored := authoredStatements(next.Session)
		result, runtimeErr := evaluateInterruptibly(evaluator, authored[statementCount:], next.Session.IR.ModulePath)
		if runtimeErr != nil {
			if errors.Is(runtimeErr, context.Canceled) {
				printEvaluationInterrupted(options.Stdout, options.Interactive)
				continue
			}
			printEvaluationError(options.Stderr, runtimeErr, options.Interactive, options.ProjectRoot, next)
			continue
		}
		source = candidate
		statementCount = len(authored)
		compilation = next
		options.language.Update(compilation.Programs, compilation.Session.IR.ModulePath)
		if result.Display && result.Value.Type.Kind != types.Void {
			mutable := ""
			if result.MutableBinding {
				mutable = " " + colorize(options.Interactive, colorKeyword, "[mut]")
			}
			fmt.Fprintf(options.Stdout, "%s %s %s%s\n", colorize(options.Interactive, colorValue, Inspect(result.Value)), colorize(options.Interactive, colorMuted, ":"), colorize(options.Interactive, colorType, DisplayType(result.Value.Type)), mutable)
		}
	}
	return nil
}

func authoredStatements(artifact *compiler.Artifact) []ir.Statement {
	if artifact == nil || artifact.IR == nil || artifact.CompilerGeneratedStart <= 0 {
		if artifact == nil || artifact.IR == nil {
			return nil
		}
		return artifact.IR.Statements
	}
	result := make([]ir.Statement, 0, len(artifact.IR.Statements))
	for _, statement := range artifact.IR.Statements {
		if statement.SourceSpan().Start.Offset < artifact.CompilerGeneratedStart {
			result = append(result, statement)
		}
	}
	return result
}

func evaluateInterruptibly(evaluator *Evaluator, statements []ir.Statement, module string) (Result, error) {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return evaluator.EvaluateContext(ctx, statements, module)
}

func printEvaluationInterrupted(output io.Writer, colored bool) {
	fmt.Fprintln(output, colorize(colored, colorMuted, "interrupted"))
}

func handleCommand(command, source string, options Options) (bool, string, *Compilation) {
	name, argument, _ := strings.Cut(command, " ")
	switch name {
	case ":quit", ":exit", ":q":
		return true, source, nil
	case ":help":
		fmt.Fprintln(options.Stdout, ":type EXPRESSION  show the checked TypeRB type")
		fmt.Fprintln(options.Stdout, ":load FILE        compile and evaluate a TypeRB file")
		fmt.Fprintln(options.Stdout, ":reload           reload the project and replay the session")
		fmt.Fprintln(options.Stdout, ":quit             leave the REPL")
		if options.Interactive {
			fmt.Fprintln(options.Stdout, "keys: Enter submit/indent; Left/Right or Ctrl-B/F move; Ctrl-A/E line")
			fmt.Fprintln(options.Stdout, "      Alt-B/F word")
			fmt.Fprintln(options.Stdout, "      Up/Down or Ctrl-P/N line/history; Ctrl-R search; Tab complete")
			fmt.Fprintln(options.Stdout, "      Ctrl-K/U/W edit; Ctrl-L clear; Ctrl-C interrupt; Ctrl-D exit")
		}
	case ":type":
		if strings.TrimSpace(argument) == "" {
			printReplError(options.Stderr, options.Interactive, "usage: :type EXPRESSION")
			break
		}
		candidate := appendSource(source, argument)
		compilation, err := options.Compile(candidate)
		if err != nil {
			printCompileError(options.Stderr, err, options.Interactive, options.ProjectRoot, sessionFilename(options.Initial))
			break
		}
		statements := authoredStatements(compilation.Session)
		if len(statements) == 0 {
			printReplError(options.Stderr, options.Interactive, "expression has no type")
			break
		}
		switch statement := statements[len(statements)-1].(type) {
		case *ir.ExpressionStatement:
			fmt.Fprintln(options.Stdout, colorize(options.Interactive, colorType, DisplayType(statement.Expression.ExprType())))
		case *ir.Variable:
			fmt.Fprintln(options.Stdout, colorize(options.Interactive, colorType, DisplayType(statement.Type)))
		default:
			printReplError(options.Stderr, options.Interactive, "input is not an expression")
		}
	case ":load":
		filename := strings.TrimSpace(argument)
		if filename == "" {
			printReplError(options.Stderr, options.Interactive, "usage: :load FILE")
			break
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			printReplError(options.Stderr, options.Interactive, err.Error())
			break
		}
		candidate := appendSource(source, string(data))
		compilation, err := options.Compile(candidate)
		if err != nil {
			printCompileError(options.Stderr, err, options.Interactive, options.ProjectRoot, sessionFilename(options.Initial))
			break
		}
		return false, candidate, compilation
	case ":reload":
		compilation, err := options.Compile(source)
		if err != nil {
			printCompileError(options.Stderr, err, options.Interactive, options.ProjectRoot, sessionFilename(options.Initial))
			break
		}
		fmt.Fprintln(options.Stdout, colorize(options.Interactive, colorSuccess, "reloaded"))
		return false, source, compilation
	default:
		printReplError(options.Stderr, options.Interactive, fmt.Sprintf("unknown command %s; use :help", name))
	}
	return false, source, nil
}

func appendSource(source, snippet string) string {
	if source == "" {
		return strings.TrimRight(snippet, "\n") + "\n"
	}
	return strings.TrimRight(source, "\n") + "\n" + strings.TrimRight(snippet, "\n") + "\n"
}

func printCompileError(output io.Writer, err error, colored bool, projectRoot, sessionPath string) {
	var compilation *compiler.CompileError
	if !errors.As(err, &compilation) {
		printReplError(output, colored, err.Error())
		return
	}
	for _, item := range compilation.Diagnostics {
		severity := string(item.Severity)
		if colored {
			severity = colorize(true, colorError, severity)
		}
		path := item.Path
		if path == "" {
			path = compilation.Filename
		}
		path = displayReplPath(path, projectRoot, sessionPath)
		fmt.Fprintf(output, "%s:%d:%d: %s[%s]: %s\n", path, item.Span.Start.Line, item.Span.Start.Column, severity, item.Code, item.Message)
		for _, related := range item.Related {
			relatedPath := related.Location.Path
			if relatedPath == "" {
				relatedPath = path
			} else {
				relatedPath = displayReplPath(relatedPath, projectRoot, sessionPath)
			}
			fmt.Fprintf(output, "  %s:%d:%d: note: %s\n", relatedPath, related.Location.Span.Start.Line, related.Location.Span.Start.Column, related.Message)
		}
		for _, fix := range item.Fixes {
			fmt.Fprintf(output, "  help: %s\n", fix.Message)
		}
	}
}

func printEvaluationError(output io.Writer, err error, colored bool, projectRoot string, compilation *Compilation) {
	var located *evaluationError
	if !errors.As(err, &located) {
		printReplError(output, colored, err.Error())
		return
	}
	path := ""
	for _, program := range compilation.Programs {
		if program.ModulePath == located.Module {
			path = program.SourcePath
			break
		}
	}
	path = displayReplPath(path, projectRoot, sessionFilename(compilation))
	if path == "" {
		printReplError(output, colored, err.Error())
		return
	}
	severity := "error"
	if colored {
		severity = colorize(true, colorError, severity)
	}
	line := located.Span.Start.Line
	if compilation.Session != nil && located.Module == compilation.Session.IR.ModulePath && line > compilation.HiddenPreludeLines {
		line -= compilation.HiddenPreludeLines
	}
	fmt.Fprintf(output, "%s:%d:%d: %s: %s\n", path, line, located.Span.Start.Column, severity, err)
}

func sessionFilename(compilation *Compilation) string {
	if compilation == nil || compilation.Session == nil {
		return ""
	}
	return compilation.Session.Filename
}

func displayReplPath(path, projectRoot, sessionPath string) string {
	if path == "" {
		return ""
	}
	if sessionPath != "" && filepath.Clean(path) == filepath.Clean(sessionPath) {
		return "(trb)"
	}
	if projectRoot == "" || !filepath.IsAbs(path) {
		return path
	}
	relative, err := filepath.Rel(projectRoot, path)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return relative
}

func printReplError(output io.Writer, colored bool, message string) {
	fmt.Fprintln(output, colorize(colored, colorError, "trb: repl:"), message)
}
