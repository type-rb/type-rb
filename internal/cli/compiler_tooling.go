package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/compilerservice"
	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/toolingprotocol"
)

func (c *CLI) runCompiler(args []string) error {
	if len(args) == 0 {
		return errors.New("compiler requires a subcommand: inspect")
	}
	switch args[0] {
	case "inspect":
		return c.runCompilerInspect(args[1:])
	default:
		return fmt.Errorf("unknown compiler command %q", args[0])
	}
}

func (c *CLI) runCompilerInspect(args []string) error {
	flags := flag.NewFlagSet("compiler inspect", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	mode := flags.String("mode", "", "standalone mode: ruby, go, or typescript")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("compiler inspect accepts at most one standalone .trb file")
	}

	filename := ""
	configStart := "."
	if flags.NArg() == 1 {
		if filepath.Ext(flags.Arg(0)) != ".trb" {
			return errors.New("standalone compiler inspection source must be a .trb file")
		}
		var err error
		filename, err = filepath.Abs(flags.Arg(0))
		if err != nil {
			return err
		}
		configStart = filename
	}

	config, standalone, err := loadCommandConfig("compiler inspect", *configPath, configStart, filename, *mode, "")
	if err != nil {
		return c.reportCompilerInspectionError(*mode, diagnostic.ProjectError, err)
	}

	var (
		units   []compiler.SourceUnit
		options compiler.Options
	)
	if standalone {
		graph, graphErr := loadFileRootSourceGraph(filename, os.ReadFile)
		if graphErr != nil {
			return c.reportCompilerInspectionError(config.Mode, diagnostic.ProjectError, graphErr)
		}
		units, options, err = projectCompilationSources(config, graph.Sources)
	} else {
		files, collectErr := collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if collectErr != nil {
			return c.reportCompilerInspectionError(config.Mode, diagnostic.ProjectError, collectErr)
		}
		if len(files) == 0 {
			return c.reportCompilerInspectionError(config.Mode, diagnostic.ProjectError, errors.New("no .trb files found"))
		}
		units, options, err = projectCompilation(config, files)
	}
	if err != nil {
		return c.reportCompilerInspectionError(config.Mode, diagnostic.ProjectError, err)
	}

	snapshot := compilerservice.New(units, options).Analyze()
	report := toolingprotocol.Build(toolingprotocol.BuildOptions{CompilerVersion: Version, Mode: config.Mode}, units, snapshot)
	if err := c.writeCompilerInspection(report); err != nil {
		return err
	}
	if snapshot.HasErrors() {
		return &reportedError{cause: errors.New("compiler inspection found errors")}
	}
	return nil
}

func (c *CLI) reportCompilerInspectionError(mode string, fallback diagnostic.Code, err error) error {
	if mode != "go" && mode != "ruby" && mode != "typescript" {
		mode = ""
	}
	var items []diagnostic.Diagnostic
	var compilation *compiler.CompileError
	if errors.As(err, &compilation) {
		items = diagnostic.Normalize(append([]diagnostic.Diagnostic(nil), compilation.Diagnostics...), compilation.Filename, fallback)
	} else {
		items = []diagnostic.Diagnostic{{Code: fallback, Severity: diagnostic.Error, Message: err.Error()}}
	}
	report := toolingprotocol.Build(
		toolingprotocol.BuildOptions{CompilerVersion: Version, Mode: mode},
		nil,
		compilerservice.Snapshot{Diagnostics: items},
	)
	if writeErr := c.writeCompilerInspection(report); writeErr != nil {
		return writeErr
	}
	return &reportedError{cause: errors.New("compiler inspection failed")}
}

func (c *CLI) writeCompilerInspection(report toolingprotocol.Report) error {
	encoder := json.NewEncoder(c.Stdout)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	return encoder.Encode(report)
}
