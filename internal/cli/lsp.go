package cli

import (
	"errors"
	"flag"
	"os"
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
	var (
		units            []compiler.SourceUnit
		options          compiler.Options
		resolveWorkspace lsp.WorkspaceResolver
	)
	if standalone {
		resolveWorkspace = func(overlays map[string][]byte) ([]compiler.SourceUnit, error) {
			graph, err := loadFileRootSourceGraph(filename, func(path string) ([]byte, error) {
				path = filepath.Clean(path)
				if source, exists := overlays[path]; exists {
					return append([]byte(nil), source...), nil
				}
				return os.ReadFile(path)
			})
			if err != nil {
				// A standalone editor client can outlive an unsaved file after its
				// backing path is deleted. Keep the LSP transport available until
				// didOpen supplies that selected entry as an overlay. Other commands
				// continue to require a readable entry through the shared graph loader.
				if errors.Is(err, os.ErrNotExist) {
					return nil, nil
				}
				return nil, err
			}
			result := make([]compiler.SourceUnit, 0, len(graph.Sources))
			for _, snapshot := range graph.Sources {
				unit, err := sourceUnit(config, snapshot.Filename, snapshot.Source)
				if err != nil {
					return nil, err
				}
				result = append(result, unit)
			}
			return result, nil
		}
		units, err = resolveWorkspace(nil)
		if err != nil {
			return err
		}
		options, err = compilerOptions(config)
	} else {
		files, collectErr := collectTRB([]string{config.SourcePath()}, config.OutputPath())
		if collectErr != nil {
			return collectErr
		}
		if len(files) == 0 {
			return errors.New("no .trb files found")
		}
		units, options, err = projectCompilation(config, files)
	}
	if err != nil {
		return err
	}
	var includedFiles []string
	if standalone {
		includedFiles = []string{filename}
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
			return sourceUnit(config, filename, source)
		},
		ResolveWorkspace: resolveWorkspace,
		Input:            c.Stdin, Output: c.Stdout,
	})
	return server.Run()
}
