// Package runtimeadapterhost strictly loads declarative native runtime
// mappings contributed by TypeRB packages. It never executes package code.
package runtimeadapterhost

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
)

type Source struct {
	Package      string
	Mode         string
	Path         string
	Dependencies map[string]string
}

type Binding struct {
	Identity                 string
	Package                  string
	Mode                     string
	Dependency               string
	Module                   string
	Symbol                   string
	CallConvention           string
	MaySuspend               bool
	PropagatesExecutionScope bool
}

type key struct {
	Package  string
	Mode     string
	Identity string
}

type Catalog struct {
	bindings map[key]Binding
}

func Read(path string) (packageextension.NativeRuntimeAdapterCatalog, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return packageextension.NativeRuntimeAdapterCatalog{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var catalog packageextension.NativeRuntimeAdapterCatalog
	if err := decoder.Decode(&catalog); err != nil {
		return packageextension.NativeRuntimeAdapterCatalog{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return packageextension.NativeRuntimeAdapterCatalog{}, fmt.Errorf("trailing JSON content")
	}
	if err := packageextension.ValidateNativeRuntimeAdapterCatalog(catalog); err != nil {
		return packageextension.NativeRuntimeAdapterCatalog{}, err
	}
	return catalog, nil
}

func Load(sources []Source) (*Catalog, error) {
	result := &Catalog{bindings: map[key]Binding{}}
	sorted := append([]Source(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Package != sorted[j].Package {
			return sorted[i].Package < sorted[j].Package
		}
		if sorted[i].Mode != sorted[j].Mode {
			return sorted[i].Mode < sorted[j].Mode
		}
		return sorted[i].Path < sorted[j].Path
	})
	seenSources := map[string]bool{}
	for _, source := range sorted {
		if strings.TrimSpace(source.Package) == "" || strings.TrimSpace(source.Mode) == "" || strings.TrimSpace(source.Path) == "" {
			return nil, fmt.Errorf("native runtime adapter source requires a package, mode, and path")
		}
		if source.Mode != "go" && source.Mode != "ruby" && source.Mode != "typescript" {
			return nil, fmt.Errorf("native runtime adapter %s selects unsupported mode %q", source.Package, source.Mode)
		}
		sourceIdentity := source.Package + "#" + source.Mode
		if seenSources[sourceIdentity] {
			return nil, fmt.Errorf("native runtime adapter source %s is duplicated", sourceIdentity)
		}
		seenSources[sourceIdentity] = true
		provided, err := Read(source.Path)
		if err != nil {
			return nil, fmt.Errorf("native runtime adapter %s (%s): %w", source.Package, source.Path, err)
		}
		identities := make([]string, 0, len(provided.Bindings))
		for identity := range provided.Bindings {
			identities = append(identities, identity)
		}
		sort.Strings(identities)
		for _, identity := range identities {
			module, symbol, ok := strings.Cut(identity, "#")
			if !ok || module == "" || symbol == "" || strings.Contains(symbol, "#") {
				return nil, fmt.Errorf("native runtime adapter %s binding %q must use canonical-module#export identity", source.Package, identity)
			}
			if module != source.Package && !strings.HasPrefix(module, source.Package+"/") {
				return nil, fmt.Errorf("native runtime adapter %s binding %s is outside the package namespace", source.Package, identity)
			}
			target := provided.Bindings[identity]
			if _, exists := source.Dependencies[target.Dependency]; !exists {
				return nil, fmt.Errorf("native runtime adapter %s binding %s selects undeclared %s dependency %s", source.Package, identity, source.Mode, target.Dependency)
			}
			catalogKey := key{Package: source.Package, Mode: source.Mode, Identity: identity}
			result.bindings[catalogKey] = Binding{
				Identity: identity, Package: source.Package, Mode: source.Mode,
				Dependency: target.Dependency, Module: target.Module, Symbol: target.Symbol,
				CallConvention: target.CallConvention, MaySuspend: target.MaySuspend,
				PropagatesExecutionScope: target.PropagatesExecutionScope,
			}
		}
	}
	return result, nil
}

func (c *Catalog) Lookup(packageName, mode, identity string) (Binding, bool) {
	if c == nil {
		return Binding{}, false
	}
	binding, ok := c.bindings[key{Package: packageName, Mode: mode, Identity: identity}]
	return binding, ok
}

func (c *Catalog) Bindings(packageName, mode string) []Binding {
	if c == nil {
		return nil
	}
	result := []Binding{}
	for bindingKey, binding := range c.bindings {
		if bindingKey.Package == packageName && bindingKey.Mode == mode {
			result = append(result, binding)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Identity < result[j].Identity })
	return result
}
