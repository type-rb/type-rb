// Package official loads versioned TypeRB packages bundled with the compiler.
// The manifest boundary keeps these packages distinct from the standard
// library so they can move to an external package source without changing
// application imports.
package official

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/stdlib"
)

//go:embed packages
var packageFiles embed.FS

type manifest struct {
	Name               string                       `json:"name"`
	Version            string                       `json:"version"`
	Module             string                       `json:"module"`
	Source             string                       `json:"source"`
	NativeDependencies map[string]map[string]string `json:"nativeDependencies,omitempty"`
}

type Package struct {
	ManifestPath       string
	Name               string
	Version            string
	NativeDependencies map[string]map[string]string
	Definition         *stdlib.Package
}

var registry = load()

func Lookup(name string) (*Package, bool) {
	definition, ok := registry[name]
	return definition, ok
}

func OwnsModule(modulePath string) bool {
	for _, packageDefinition := range registry {
		if packageDefinition.Definition.ModulePath == modulePath {
			return true
		}
	}
	return false
}

func Names() []string {
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func load() map[string]*Package {
	result := map[string]*Package{}
	err := fs.WalkDir(packageFiles, "packages", func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "trbpackage.json" {
			return nil
		}
		data, err := packageFiles.ReadFile(filename)
		if err != nil {
			return err
		}
		var descriptor manifest
		decoder := json.NewDecoder(strings.NewReader(string(data)))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&descriptor); err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		if descriptor.Name == "" || descriptor.Version == "" || descriptor.Module == "" || descriptor.Source == "" {
			return fmt.Errorf("%s: name, version, module, and source are required", filename)
		}
		if _, exists := result[descriptor.Name]; exists {
			return fmt.Errorf("%s: package %s is already registered", filename, descriptor.Name)
		}
		sourcePath := path.Join(path.Dir(filename), descriptor.Source)
		source, err := packageFiles.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("%s: %w", filename, err)
		}
		result[descriptor.Name] = &Package{
			ManifestPath:       filename,
			Name:               descriptor.Name,
			Version:            descriptor.Version,
			NativeDependencies: descriptor.NativeDependencies,
			Definition: &stdlib.Package{
				Path:       descriptor.Name,
				ModulePath: descriptor.Module,
				Source:     string(source),
				Kind:       stdlib.Portable,
				Symbols:    map[string]stdlib.Symbol{},
			},
		}
		return nil
	})
	if err != nil {
		panic("load official TypeRB packages: " + err.Error())
	}
	return result
}
