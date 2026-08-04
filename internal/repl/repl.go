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
	"strings"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Compilation struct {
	Session  *compiler.Artifact
	Programs []*ir.Program
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
	Compile     CompileFunc
}

func Run(options Options) error {
	if options.Compile == nil {
		return errors.New("REPL compiler is not configured")
	}
	compilation, err := options.Compile("")
	if err != nil {
		return err
	}
	evaluator := NewEvaluator(options.Stdout, options.Mode)
	if err := evaluator.LoadProject(compilation.Programs, compilation.Session.IR.ModulePath); err != nil {
		return err
	}

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
					printReplError(options.Stderr, options.Interactive, err.Error())
					continue
				}
				nextEvaluator.LoadDefinitions(nextCompilation.Session.IR)
				if _, err := evaluateInterruptibly(nextEvaluator, nextCompilation.Session.IR.Statements, nextCompilation.Session.IR.ModulePath); err != nil {
					if errors.Is(err, context.Canceled) {
						printEvaluationInterrupted(options.Stdout, options.Interactive)
						continue
					}
					printReplError(options.Stderr, options.Interactive, err.Error())
					continue
				}
				source = replacement
				compilation = nextCompilation
				statementCount = len(compilation.Session.IR.Statements)
				evaluator = nextEvaluator
			}
			continue
		}

		candidate := appendSource(source, snippet)
		next, compileErr := options.Compile(candidate)
		if compileErr != nil {
			printCompileError(options.Stderr, compileErr, options.Interactive)
			continue
		}
		if next.Session == nil || len(next.Session.IR.Statements) < statementCount {
			printReplError(options.Stderr, options.Interactive, "compiler returned an invalid session")
			continue
		}

		for _, program := range next.Programs {
			if program.ModulePath != next.Session.IR.ModulePath {
				evaluator.LoadDefinitions(program)
			}
		}
		evaluator.LoadDefinitions(next.Session.IR)
		result, runtimeErr := evaluateInterruptibly(evaluator, next.Session.IR.Statements[statementCount:], next.Session.IR.ModulePath)
		if runtimeErr != nil {
			if errors.Is(runtimeErr, context.Canceled) {
				printEvaluationInterrupted(options.Stdout, options.Interactive)
				continue
			}
			printReplError(options.Stderr, options.Interactive, runtimeErr.Error())
			continue
		}
		source = candidate
		statementCount = len(next.Session.IR.Statements)
		compilation = next
		if result.Display && result.Value.Type.Kind != types.Void {
			fmt.Fprintf(options.Stdout, "%s %s %s\n", colorize(options.Interactive, colorValue, Inspect(result.Value)), colorize(options.Interactive, colorMuted, ":"), colorize(options.Interactive, colorType, result.Value.Type.String()))
		}
	}
	return nil
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
			fmt.Fprintln(options.Stdout, "keys: Left/Right or Ctrl-B/F move; Ctrl-A/E line; Alt-B/F word")
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
			printCompileError(options.Stderr, err, options.Interactive)
			break
		}
		statements := compilation.Session.IR.Statements
		if len(statements) == 0 {
			printReplError(options.Stderr, options.Interactive, "expression has no type")
			break
		}
		switch statement := statements[len(statements)-1].(type) {
		case *ir.ExpressionStatement:
			fmt.Fprintln(options.Stdout, colorize(options.Interactive, colorType, statement.Expression.ExprType().String()))
		case *ir.Variable:
			fmt.Fprintln(options.Stdout, colorize(options.Interactive, colorType, statement.Type.String()))
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
			printCompileError(options.Stderr, err, options.Interactive)
			break
		}
		return false, candidate, compilation
	case ":reload":
		compilation, err := options.Compile(source)
		if err != nil {
			printCompileError(options.Stderr, err, options.Interactive)
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

func printCompileError(output io.Writer, err error, colored bool) {
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
		fmt.Fprintf(output, "%s:%d:%d: %s: %s\n", compilation.Filename, item.Span.Start.Line, item.Span.Start.Column, severity, item.Message)
	}
}

func printReplError(output io.Writer, colored bool, message string) {
	fmt.Fprintln(output, colorize(colored, colorError, "trb: repl:"), message)
}
