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
	Name                       string                                             `json:"name"`
	Version                    string                                             `json:"version"`
	Module                     string                                             `json:"module"`
	Source                     string                                             `json:"source"`
	SemanticProvider           string                                             `json:"semanticProvider,omitempty"`
	ProjectProvider            string                                             `json:"projectProvider,omitempty"`
	TypeProvider               string                                             `json:"typeProvider,omitempty"`
	Kind                       string                                             `json:"kind,omitempty"`
	Targets                    []string                                           `json:"targets,omitempty"`
	NativeDependencies         map[string]map[string]string                       `json:"nativeDependencies,omitempty"`
	NativeDependenciesByOption map[string]map[string]map[string]map[string]string `json:"nativeDependenciesByOption,omitempty"`
}

type Package struct {
	ManifestPath               string
	Name                       string
	Version                    string
	ProjectProvider            string
	NativeDependencies         map[string]map[string]string
	NativeDependenciesByOption map[string]map[string]map[string]map[string]string
	Definition                 *stdlib.Package
}

func (p *Package) NativeDependenciesFor(mode string, rawOptions json.RawMessage) (map[string]string, error) {
	result := map[string]string{}
	merge := func(required map[string]string) error {
		for name, version := range required {
			if existing, ok := result[name]; ok && existing != version {
				return fmt.Errorf("TypeRB package %s requires conflicting versions of %s: %s and %s", p.Name, name, existing, version)
			}
			result[name] = version
		}
		return nil
	}
	if err := merge(p.NativeDependencies[mode]); err != nil {
		return nil, err
	}
	conditional := p.NativeDependenciesByOption[mode]
	if len(conditional) == 0 {
		return result, nil
	}
	var options map[string]json.RawMessage
	if len(rawOptions) > 0 {
		if err := json.Unmarshal(rawOptions, &options); err != nil {
			return nil, fmt.Errorf("packageOptions.%q: %w", p.Name, err)
		}
	}
	optionNames := make([]string, 0, len(conditional))
	for name := range conditional {
		optionNames = append(optionNames, name)
	}
	sort.Strings(optionNames)
	for _, optionName := range optionNames {
		var value string
		if err := json.Unmarshal(options[optionName], &value); err != nil || value == "" {
			return nil, fmt.Errorf("packageOptions.%q.%s is required to select native dependencies", p.Name, optionName)
		}
		required, ok := conditional[optionName][value]
		if !ok {
			return nil, fmt.Errorf("packageOptions.%q.%s has unsupported value %q", p.Name, optionName, value)
		}
		if err := merge(required); err != nil {
			return nil, err
		}
	}
	return result, nil
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
		if descriptor.Kind != "" && descriptor.Kind != "portable" && descriptor.Kind != "platform" {
			return fmt.Errorf("%s: kind must be portable or platform", filename)
		}
		for _, target := range descriptor.Targets {
			if target != "go" && target != "ruby" && target != "typescript" {
				return fmt.Errorf("%s: unsupported target %q", filename, target)
			}
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
			ManifestPath:               filename,
			Name:                       descriptor.Name,
			Version:                    descriptor.Version,
			ProjectProvider:            descriptor.ProjectProvider,
			NativeDependencies:         descriptor.NativeDependencies,
			NativeDependenciesByOption: descriptor.NativeDependenciesByOption,
			Definition: &stdlib.Package{
				Path:         descriptor.Name,
				ModulePath:   descriptor.Module,
				Source:       string(source),
				Kind:         manifestKind(descriptor.Kind),
				Targets:      manifestTargets(descriptor.Targets),
				TypeProvider: descriptor.TypeProvider,
				Symbols:      semanticSymbols(descriptor.SemanticProvider),
			},
		}
		return nil
	})
	if err != nil {
		panic("load official TypeRB packages: " + err.Error())
	}
	return result
}

func manifestKind(kind string) stdlib.Kind {
	if kind == "platform" {
		return stdlib.Platform
	}
	return stdlib.Portable
}

func manifestTargets(targets []string) map[string]bool {
	if len(targets) == 0 {
		return nil
	}
	result := make(map[string]bool, len(targets))
	for _, target := range targets {
		result[target] = true
	}
	return result
}
