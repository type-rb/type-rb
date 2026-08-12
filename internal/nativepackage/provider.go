package nativepackage

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

type Provider struct {
	FormatVersion int               `json:"formatVersion"`
	Modules       map[string]Module `json:"modules"`
}

type ProviderSource struct {
	Package      string
	Path         string
	Dependencies map[string]string
}

func ReadProvider(path string) (*Provider, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var provider Provider
	if err := decoder.Decode(&provider); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON content")
	}
	if provider.FormatVersion != FormatVersion {
		return nil, fmt.Errorf("unsupported formatVersion %d; expected %d", provider.FormatVersion, FormatVersion)
	}
	if provider.Modules == nil {
		return nil, errors.New("modules is required")
	}
	return &provider, nil
}

// ApplyProviderFiles overlays declarative package-owned corrections on top of
// declarations inferred from .d.ts files. Two providers cannot redefine the
// same symbol, which keeps the result independent from package traversal order.
func ApplyProviderFiles(catalog *Catalog, sources []ProviderSource) error {
	if catalog == nil {
		return errors.New("native type catalog is nil")
	}
	sorted := append([]ProviderSource(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Package != sorted[j].Package {
			return sorted[i].Package < sorted[j].Package
		}
		return sorted[i].Path < sorted[j].Path
	})
	owners := map[string]string{}
	checksums, err := providerChecksums(sorted)
	if err != nil {
		return err
	}
	catalog.ProviderChecksums = checksums
	for _, source := range sorted {
		provider, err := ReadProvider(source.Path)
		if err != nil {
			return fmt.Errorf("native type provider %s (%s): %w", source.Package, source.Path, err)
		}
		moduleNames := make([]string, 0, len(provider.Modules))
		for moduleName := range provider.Modules {
			moduleNames = append(moduleNames, moduleName)
		}
		sort.Strings(moduleNames)
		for _, moduleName := range moduleNames {
			if !catalog.Owns(moduleName) || !nativeDependencyOwns(source.Dependencies, moduleName) {
				return fmt.Errorf("native type provider %s declares %s without a matching native dependency", source.Package, moduleName)
			}
			patch := provider.Modules[moduleName]
			if len(patch.Unsupported) != 0 {
				return fmt.Errorf("native type provider %s cannot declare unsupported entries for %s", source.Package, moduleName)
			}
			if err := validateProviderModule(moduleName, patch); err != nil {
				return fmt.Errorf("native type provider %s: %w", source.Package, err)
			}
			current := catalog.Modules[moduleName]
			if current.Exports == nil {
				current.Exports = map[string]Export{}
			}
			if current.Records == nil {
				current.Records = map[string]Export{}
			}
			for name, exported := range patch.Exports {
				key := moduleName + "#export#" + name
				if previous := owners[key]; previous != "" {
					return fmt.Errorf("native type providers %s and %s both declare export %s from %s", previous, source.Package, name, moduleName)
				}
				owners[key] = source.Package
				current.Exports[name] = exported
				delete(current.Unsupported, name)
			}
			for name, record := range patch.Records {
				key := moduleName + "#record#" + name
				if previous := owners[key]; previous != "" {
					return fmt.Errorf("native type providers %s and %s both declare record %s for %s", previous, source.Package, name, moduleName)
				}
				owners[key] = source.Package
				current.Records[name] = record
				delete(current.Unsupported, name)
			}
			catalog.Modules[moduleName] = current
		}
	}
	return nil
}

func nativeDependencyOwns(dependencies map[string]string, moduleName string) bool {
	for dependency := range dependencies {
		if moduleName == dependency || strings.HasPrefix(moduleName, dependency+"/") {
			return true
		}
	}
	return false
}

func providerChecksums(sources []ProviderSource) (map[string]string, error) {
	result := make(map[string]string, len(sources))
	for _, source := range sources {
		data, err := os.ReadFile(source.Path)
		if err != nil {
			return nil, fmt.Errorf("read native type provider %s (%s): %w", source.Package, source.Path, err)
		}
		sum := sha256.Sum256(data)
		result[source.Package] = "sha256:" + hex.EncodeToString(sum[:])
	}
	return result, nil
}

func validateProviderModule(moduleName string, module Module) error {
	if strings.TrimSpace(moduleName) == "" {
		return errors.New("native module name is empty")
	}
	for name, exported := range module.Exports {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("%s contains an empty export name", moduleName)
		}
		if exported.Kind != "component" && exported.Kind != "function" && exported.Kind != "class" && exported.Kind != "record" && exported.Kind != "type_alias" {
			return fmt.Errorf("export %s from %s has unsupported kind %q", name, moduleName, exported.Kind)
		}
		if err := validateProviderTypeParameters(exported.TypeParameters); err != nil {
			return fmt.Errorf("export %s from %s: %w", name, moduleName, err)
		}
		if err := validateProviderType(exported.Type); err != nil {
			return fmt.Errorf("export %s from %s: %w", name, moduleName, err)
		}
		if exported.Kind == "type_alias" {
			if exported.AliasTarget == nil || exported.AliasTarget.Kind == "" {
				return fmt.Errorf("export %s from %s: type alias requires aliasTarget", name, moduleName)
			}
			if err := validateProviderType(*exported.AliasTarget); err != nil {
				return fmt.Errorf("export %s from %s: %w", name, moduleName, err)
			}
		} else if exported.AliasTarget != nil {
			return fmt.Errorf("export %s from %s: aliasTarget is only valid for type aliases", name, moduleName)
		}
		for _, parameter := range exported.Parameters {
			if err := validateProviderType(parameter); err != nil {
				return fmt.Errorf("export %s from %s: %w", name, moduleName, err)
			}
		}
		if exported.Required < 0 || exported.Required > len(exported.Parameters) {
			return fmt.Errorf("export %s from %s has invalid required parameter count", name, moduleName)
		}
		for _, field := range exported.Fields {
			if strings.TrimSpace(field.Name) == "" {
				return fmt.Errorf("export %s from %s contains an empty field name", name, moduleName)
			}
			if err := validateProviderType(field.Type); err != nil {
				return fmt.Errorf("export %s.%s from %s: %w", name, field.Name, moduleName, err)
			}
		}
	}
	for name, record := range module.Records {
		if strings.TrimSpace(name) == "" || record.Kind != "record" {
			return fmt.Errorf("record %s from %s must use kind record", name, moduleName)
		}
		if err := validateProviderType(record.Type); err != nil {
			return fmt.Errorf("record %s from %s: %w", name, moduleName, err)
		}
		if err := validateProviderTypeParameters(record.TypeParameters); err != nil {
			return fmt.Errorf("record %s from %s: %w", name, moduleName, err)
		}
		for _, field := range record.Fields {
			if strings.TrimSpace(field.Name) == "" {
				return fmt.Errorf("record %s from %s contains an empty field name", name, moduleName)
			}
			if err := validateProviderType(field.Type); err != nil {
				return fmt.Errorf("record %s.%s from %s: %w", name, field.Name, moduleName, err)
			}
		}
	}
	return nil
}

func validateProviderType(typ Type) error {
	switch typ.Kind {
	case "array", "bool", "bytes", "float", "function", "hash", "int", "int_literal", "named", "never", "nil", "range", "string", "string_literal", "union", "void":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if (typ.Kind == "named" || typ.Kind == "function") && strings.TrimSpace(typ.Name) == "" {
		return fmt.Errorf("type kind %s requires a name", typ.Kind)
	}
	for _, argument := range typ.Args {
		if err := validateProviderType(argument); err != nil {
			return err
		}
	}
	return nil
}

func validateProviderTypeParameters(parameters []string) error {
	seen := map[string]bool{}
	for _, parameter := range parameters {
		if strings.TrimSpace(parameter) == "" {
			return errors.New("type parameter name is empty")
		}
		if seen[parameter] {
			return fmt.Errorf("type parameter %s is duplicated", parameter)
		}
		seen[parameter] = true
	}
	return nil
}
