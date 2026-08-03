// Package repl implements TypeRB's project-aware interactive evaluator. Every
// accepted input is compiled through the normal project pipeline before its
// typed IR is evaluated.
package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
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
		fmt.Fprintf(options.Stdout, "TypeRB %s\nproject: %s\nmode: %s\n\n", options.Version, options.ProjectName, options.Mode)
	}

	scanner := bufio.NewScanner(options.Stdin)
	scanner.Buffer(make([]byte, 1024), 1024*1024)
	source := ""
	statementCount := 0
	for {
		line, ok := readInput(scanner, options, "trb:"+options.Mode+"> ")
		if !ok {
			break
		}
		trimmed := strings.TrimSpace(line)
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
					fmt.Fprintln(options.Stderr, "trb: repl:", err)
					continue
				}
				nextEvaluator.LoadDefinitions(nextCompilation.Session.IR)
				if _, err := nextEvaluator.Evaluate(nextCompilation.Session.IR.Statements, nextCompilation.Session.IR.ModulePath); err != nil {
					fmt.Fprintln(options.Stderr, "trb: repl:", err)
					continue
				}
				source = replacement
				compilation = nextCompilation
				statementCount = len(compilation.Session.IR.Statements)
				evaluator = nextEvaluator
			}
			continue
		}

		snippet := line
		for !Complete(snippet) {
			next, available := readInput(scanner, options, "trb:"+options.Mode+"*  ")
			if !available {
				fmt.Fprintln(options.Stderr, "trb: repl: incomplete input")
				return nil
			}
			snippet += "\n" + next
		}
		candidate := appendSource(source, snippet)
		next, compileErr := options.Compile(candidate)
		if compileErr != nil {
			printCompileError(options.Stderr, compileErr)
			continue
		}
		if next.Session == nil || len(next.Session.IR.Statements) < statementCount {
			fmt.Fprintln(options.Stderr, "trb: repl: compiler returned an invalid session")
			continue
		}

		evaluator.LoadDefinitions(next.Session.IR)
		result, runtimeErr := evaluator.Evaluate(next.Session.IR.Statements[statementCount:], next.Session.IR.ModulePath)
		if runtimeErr != nil {
			fmt.Fprintln(options.Stderr, "trb: repl:", runtimeErr)
			continue
		}
		source = candidate
		statementCount = len(next.Session.IR.Statements)
		compilation = next
		if result.Display {
			fmt.Fprintln(options.Stdout, Inspect(result.Value)+" : "+result.Value.Type.String())
		}
	}
	return scanner.Err()
}

func readInput(scanner *bufio.Scanner, options Options, prompt string) (string, bool) {
	if options.Interactive {
		fmt.Fprint(options.Stdout, prompt)
	}
	if !scanner.Scan() {
		return "", false
	}
	return scanner.Text(), true
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
	case ":type":
		if strings.TrimSpace(argument) == "" {
			fmt.Fprintln(options.Stderr, "trb: repl: usage: :type EXPRESSION")
			break
		}
		candidate := appendSource(source, argument)
		compilation, err := options.Compile(candidate)
		if err != nil {
			printCompileError(options.Stderr, err)
			break
		}
		statements := compilation.Session.IR.Statements
		if len(statements) == 0 {
			fmt.Fprintln(options.Stderr, "trb: repl: expression has no type")
			break
		}
		switch statement := statements[len(statements)-1].(type) {
		case *ir.ExpressionStatement:
			fmt.Fprintln(options.Stdout, statement.Expression.ExprType().String())
		case *ir.Variable:
			fmt.Fprintln(options.Stdout, statement.Type.String())
		default:
			fmt.Fprintln(options.Stderr, "trb: repl: input is not an expression")
		}
	case ":load":
		filename := strings.TrimSpace(argument)
		if filename == "" {
			fmt.Fprintln(options.Stderr, "trb: repl: usage: :load FILE")
			break
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			fmt.Fprintln(options.Stderr, "trb: repl:", err)
			break
		}
		candidate := appendSource(source, string(data))
		compilation, err := options.Compile(candidate)
		if err != nil {
			printCompileError(options.Stderr, err)
			break
		}
		return false, candidate, compilation
	case ":reload":
		compilation, err := options.Compile(source)
		if err != nil {
			printCompileError(options.Stderr, err)
			break
		}
		fmt.Fprintln(options.Stdout, "reloaded")
		return false, source, compilation
	default:
		fmt.Fprintf(options.Stderr, "trb: repl: unknown command %s; use :help\n", name)
	}
	return false, source, nil
}

func appendSource(source, snippet string) string {
	if source == "" {
		return strings.TrimRight(snippet, "\n") + "\n"
	}
	return strings.TrimRight(source, "\n") + "\n" + strings.TrimRight(snippet, "\n") + "\n"
}

func printCompileError(output io.Writer, err error) {
	var compilation *compiler.CompileError
	if !errors.As(err, &compilation) {
		fmt.Fprintln(output, "trb: repl:", err)
		return
	}
	for _, item := range compilation.Diagnostics {
		fmt.Fprintf(output, "%s:%d:%d: %s: %s\n", compilation.Filename, item.Span.Start.Line, item.Span.Start.Column, item.Severity, item.Message)
	}
}
