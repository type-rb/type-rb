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
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("lsp does not accept source arguments; it serves the configured project")
	}
	config, err := loadConfig(*configPath, ".")
	if err != nil {
		return err
	}
	files, err := collectTRB([]string{config.SourcePath()}, config.OutputPath())
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return errors.New("no .trb files found")
	}
	units, options, err := projectCompilation(config, files)
	if err != nil {
		return err
	}
	server := lsp.New(lsp.Options{
		Mode: config.Mode, Version: Version, Units: units, CompilerOptions: options,
		ExcludedRoots: []string{
			config.OutputPath(),
			filepath.Join(config.Root, ".git"),
			filepath.Join(config.Root, ".trb"),
			filepath.Join(config.Root, "node_modules"),
		},
		ResolveUnit: func(filename string, source []byte) (compiler.SourceUnit, error) {
			return sourceUnit(config, filename, source)
		},
		Input: c.Stdin, Output: c.Stdout,
	})
	return server.Run()
}
