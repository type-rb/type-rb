package cli

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen"
	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/project"
	"golang.org/x/mod/modfile"
)

func compiledSupportFiles(config *project.Config, compiled map[string]*compiler.Artifact) ([]codegen.SupportFile, error) {
	var files []codegen.SupportFile
	seen := map[string]bool{}
	for _, name := range sortedArtifactNames(compiled) {
		for _, file := range compiled[name].SupportFiles {
			if file.Path == "" || file.Path == "." || file.Path == ".." || path.IsAbs(file.Path) || path.Clean(file.Path) != file.Path || strings.ContainsAny(file.Path, "\\\x00:") || strings.HasPrefix(file.Path, "../") {
				return nil, fmt.Errorf("invalid compiler support path %q", file.Path)
			}
			if seen[file.Path] {
				return nil, fmt.Errorf("duplicate compiler support path %q", file.Path)
			}
			seen[file.Path] = true
			files = append(files, file)
		}
	}
	for _, file := range files {
		directory := path.Dir(file.Path)
		for _, name := range sortedArtifactNames(compiled) {
			relative, _ := generatedRelative(config, name, compiled[name])
			if path.Dir(filepath.ToSlash(relative)) == directory {
				return nil, fmt.Errorf("source %s collides with compiler support package %s", name, directory)
			}
		}
		// Native project files must not inject additional declarations into a
		// compiler-owned package when --copy mirrors the project tree.
		if _, err := os.Lstat(filepath.Join(config.Root, filepath.FromSlash(directory))); err == nil {
			return nil, fmt.Errorf("project path %s collides with compiler support package", directory)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func writeCompiledGoDependencies(config *project.Config, root string, compiled map[string]*compiler.Artifact) error {
	dependencies := map[string]string{}
	for _, artifact := range compiled {
		for name, version := range artifact.NativeDependencies {
			if old, found := dependencies[name]; found && old != version {
				return fmt.Errorf("conflicting compiler support dependency %s", name)
			}
			dependencies[name] = version
		}
	}
	if len(dependencies) == 0 {
		return nil
	}
	filename := filepath.Join(root, "go.mod")
	data, err := os.ReadFile(filename)
	if errors.Is(err, os.ErrNotExist) {
		if config.Go == nil || config.Go.Module == "" {
			return errors.New("compiler support requires a Go module")
		}
		data = []byte(fmt.Sprintf("module %s\n\ngo %s\n", config.Go.Module, project.DefaultGoVersion))
	} else if err != nil {
		return err
	}
	file, err := modfile.Parse(filename, data, nil)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(dependencies))
	for name := range dependencies {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		// The adapter is conformance-tested against this version. Do not
		// silently replace an application's different explicit requirement.
		for _, requirement := range file.Require {
			if requirement.Mod.Path == name && requirement.Mod.Version != dependencies[name] {
				return fmt.Errorf("compiler support requires %s %s; module requires %s", name, dependencies[name], requirement.Mod.Version)
			}
		}
		if err := file.AddRequire(name, dependencies[name]); err != nil {
			return err
		}
	}
	data, err = file.Format()
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0o644)
}
