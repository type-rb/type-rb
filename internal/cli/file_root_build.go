package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/type-rb/type-rb/internal/project"
)

func (c *CLI) buildGoFileRootExecutable(config *project.Config, graph *fileRootSourceGraph, outfile string, debug bool) error {
	compiled, err := compileProjectSources(config, graph.Sources)
	if err != nil {
		return err
	}
	if !artifactHasMain(compiled[graph.Entry]) {
		return errors.New("standalone file has no top-level main(); define def main()")
	}
	output, err := fileRootExecutableOutputPath(config, graph.Entry, outfile)
	if err != nil {
		return err
	}
	buildRoot, err := os.MkdirTemp("", "trb-file-root-build-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildRoot)
	generated, err := writeCompiledTree(config, compiled, buildRoot, debug)
	if err != nil {
		return err
	}
	if err := writeStandaloneGoModule(config, buildRoot, compiled); err != nil {
		return err
	}
	target := generated[graph.Entry]
	if target == "" {
		return errors.New("compiler did not produce the standalone file artifact")
	}
	return c.buildGoTarget(target, output, debug)
}

func fileRootExecutableOutputPath(config *project.Config, entry, outfile string) (string, error) {
	if outfile == "" {
		stem := strings.TrimSuffix(filepath.Base(entry), filepath.Ext(entry))
		if stem == "" || stem == "." || stem == ".." {
			return "", fmt.Errorf("standalone filename %q cannot be used as an executable filename; pass --outfile", filepath.Base(entry))
		}
		outfile = filepath.Join("bin", stem)
	}
	return executableOutputPath(config, outfile)
}

func (c *CLI) buildGoTarget(target, output string, debug bool) error {
	if info, statErr := os.Stat(output); statErr == nil && info.IsDir() {
		return fmt.Errorf("--outfile must name a file; %s is a directory", output)
	} else if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return statErr
	}
	if err := os.MkdirAll(filepath.Dir(output), 0o755); err != nil {
		return err
	}
	arguments := []string{"build", "-mod=mod"}
	if debug {
		arguments = append(arguments, "-gcflags=all=-N -l")
	}
	arguments = append(arguments, "-o", output, ".")
	command := exec.Command("go", arguments...)
	command.Dir = filepath.Dir(target)
	command.Stdout = c.Stdout
	command.Stderr = c.Stderr
	relay := newCommandSignalRelay()
	defer relay.Close()
	if err := relay.Run(command); err != nil {
		return fmt.Errorf("go build: %w", err)
	}
	fmt.Fprintf(c.Stdout, "executable -> %s\n", output)
	return nil
}
