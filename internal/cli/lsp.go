package cli

import (
	"errors"
	"flag"
	"path/filepath"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/lsp"
)

func (c *CLI) runLSP(args []string) error {
	flags := flag.NewFlagSet("lsp", flag.ContinueOnError)
	flags.SetOutput(c.Stderr)
	configPath := flags.String("config", "", "path to trbconfig.jsonc")
	mode := flags.String("mode", "", "standalone mode: ruby, go, or typescript")
	typeScriptRuntime := flags.String("runtime", "", "standalone TypeScript runtime: node or bun")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() > 1 {
		return errors.New("lsp accepts at most one standalone .trb file")
	}
	filename := ""
	configStart := "."
	if flags.NArg() == 1 {
		if filepath.Ext(flags.Arg(0)) != ".trb" {
			return errors.New("standalone LSP source must be a .trb file")
		}
		var err error
		filename, err = filepath.Abs(flags.Arg(0))
		if err != nil {
			return err
		}
		configStart = filename
	}
	config, standalone, err := loadCommandConfig("lsp", *configPath, configStart, filename, *mode, *typeScriptRuntime)
	if err != nil {
		return err
	}
	files := []string{filename}
	if standalone {
		files, err = standaloneSourceFiles(filename)
		if err != nil {
			return err
		}
	} else {
		files, err = collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if err != nil {
			return err
		}
	}
	if len(files) == 0 {
		return errors.New("no .trb files found")
	}
	units, options, err := projectCompilation(config, files)
	if err != nil {
		return err
	}
	var includedFiles []string
	if standalone {
		includedFiles = []string{filename}
		if err := markStandaloneSourceUnits(units, filename); err != nil {
			return err
		}
	}
	server := lsp.New(lsp.Options{
		Mode: config.Mode, Version: Version, Units: units, CompilerOptions: options,
		IncludedFiles: includedFiles,
		ExcludedRoots: []string{
			config.OutputPath(),
			filepath.Join(config.Root, ".git"),
			filepath.Join(config.Root, ".trb"),
			filepath.Join(config.Root, "node_modules"),
		},
		ResolveUnit: func(filename string, source []byte) (compiler.SourceUnit, error) {
			unit, err := sourceUnit(config, filename, source)
			if err == nil && standalone {
				candidate, _ := filepath.Abs(filename)
				entry, _ := filepath.Abs(includedFiles[0])
				unit.Standalone = true
				unit.ScriptEntry = candidate == entry
			}
			return unit, err
		},
		Input: c.Stdin, Output: c.Stdout,
	})
	return server.Run()
}
