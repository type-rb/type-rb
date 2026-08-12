package packages

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/project"
	"golang.org/x/mod/semver"
)

const TypeRBManifestName = "trbpackage.json"

// TypeRBManifest is the portable, repository-owned description of an external
// TypeRB package. Compiler extensions are recorded at this boundary but are
// deliberately rejected until the sandboxed extension protocol is available.
type TypeRBManifest struct {
	FormatVersion       int                                   `json:"formatVersion"`
	Name                string                                `json:"name"`
	Version             string                                `json:"version"`
	SourceDir           string                                `json:"sourceDir,omitempty"`
	Modes               []string                              `json:"modes,omitempty"`
	Packages            map[string]project.PackageRequirement `json:"packages,omitempty"`
	NativeDependencies  map[string]map[string]string          `json:"nativeDependencies,omitempty"`
	NativeTypeProviders map[string]string                     `json:"nativeTypeProviders,omitempty"`
	CompilerExtension   json.RawMessage                       `json:"compilerExtension,omitempty"`
}

func ReadTypeRBManifest(root string) (*TypeRBManifest, error) {
	manifestPath := filepath.Join(root, TypeRBManifestName)
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", manifestPath, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var manifest TypeRBManifest
	if err := decoder.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decode %s: %w", manifestPath, err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("decode %s: trailing JSON content", manifestPath)
	}
	manifest.applyDefaults()
	if err := manifest.Validate(root); err != nil {
		return nil, fmt.Errorf("%s: %w", manifestPath, err)
	}
	return &manifest, nil
}

func (m *TypeRBManifest) applyDefaults() {
	if m.SourceDir == "" {
		m.SourceDir = "src"
	}
	if len(m.Modes) == 0 {
		m.Modes = []string{"go", "ruby", "typescript"}
	}
	if m.Packages == nil {
		m.Packages = map[string]project.PackageRequirement{}
	}
	if m.NativeDependencies == nil {
		m.NativeDependencies = map[string]map[string]string{}
	}
	if m.NativeTypeProviders == nil {
		m.NativeTypeProviders = map[string]string{}
	}
}

func (m *TypeRBManifest) Validate(root string) error {
	if m.FormatVersion != 1 {
		return fmt.Errorf("unsupported formatVersion %d; expected 1", m.FormatVersion)
	}
	if !validExternalPackageName(m.Name) {
		return fmt.Errorf("invalid package name %q", m.Name)
	}
	if !validPackageVersion(m.Version) {
		return fmt.Errorf("version must be a semantic version; got %q", m.Version)
	}
	if filepath.IsAbs(m.SourceDir) || escapesDirectory(m.SourceDir) {
		return errors.New("sourceDir must stay below the package root")
	}
	sourceRoot := filepath.Join(root, m.SourceDir)
	info, err := os.Stat(sourceRoot)
	if err != nil {
		return fmt.Errorf("sourceDir: %w", err)
	}
	if !info.IsDir() {
		return errors.New("sourceDir must be a directory")
	}
	seenModes := map[string]bool{}
	for _, mode := range m.Modes {
		if mode != "go" && mode != "ruby" && mode != "typescript" {
			return fmt.Errorf("unsupported mode %q", mode)
		}
		if seenModes[mode] {
			return fmt.Errorf("duplicate mode %q", mode)
		}
		seenModes[mode] = true
	}
	for name, requirement := range m.Packages {
		if !validExternalPackageName(name) {
			return fmt.Errorf("invalid dependency name %q", name)
		}
		if err := validateManifestRequirement(name, requirement); err != nil {
			return err
		}
	}
	for mode, dependencies := range m.NativeDependencies {
		if mode != "go" && mode != "ruby" && mode != "typescript" {
			return fmt.Errorf("nativeDependencies has unsupported mode %q", mode)
		}
		for name, version := range dependencies {
			if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
				return fmt.Errorf("nativeDependencies.%s contains an empty name or version", mode)
			}
		}
	}
	for mode, provider := range m.NativeTypeProviders {
		if mode != "typescript" {
			return fmt.Errorf("nativeTypeProviders has unsupported mode %q", mode)
		}
		if filepath.IsAbs(provider) || escapesDirectory(provider) || strings.TrimSpace(provider) == "" {
			return fmt.Errorf("nativeTypeProviders.%s must stay below the package root", mode)
		}
		info, err := os.Stat(filepath.Join(root, provider))
		if err != nil {
			return fmt.Errorf("nativeTypeProviders.%s: %w", mode, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("nativeTypeProviders.%s must name a regular file", mode)
		}
	}
	if len(bytes.TrimSpace(m.CompilerExtension)) > 0 && string(bytes.TrimSpace(m.CompilerExtension)) != "null" {
		return errors.New("external compilerExtension is not supported until the sandboxed extension protocol is available")
	}
	return nil
}

func (m *TypeRBManifest) Supports(mode string) bool {
	for _, supported := range m.Modes {
		if supported == mode {
			return true
		}
	}
	return false
}

func (m *TypeRBManifest) NativeDependenciesFor(mode string) map[string]string {
	result := map[string]string{}
	for name, version := range m.NativeDependencies[mode] {
		result[name] = version
	}
	return result
}

func (m *TypeRBManifest) NativeTypeProviderFor(mode string) string {
	return m.NativeTypeProviders[mode]
}

func validateManifestRequirement(name string, requirement project.PackageRequirement) error {
	hasPath := strings.TrimSpace(requirement.Path) != ""
	hasSource := strings.TrimSpace(requirement.Source) != ""
	hasVersion := strings.TrimSpace(requirement.Version) != ""
	if hasPath {
		if filepath.IsAbs(requirement.Path) || escapesDirectory(requirement.Path) {
			return fmt.Errorf("dependency %s path must stay below the package root", name)
		}
		if hasSource || hasVersion {
			return fmt.Errorf("dependency %s path cannot be combined with source or version", name)
		}
		return nil
	}
	if !hasVersion {
		return fmt.Errorf("dependency %s requires a version or path", name)
	}
	if hasSource {
		if err := validatePackageSource(requirement.Source); err != nil {
			return fmt.Errorf("dependency %s source: %w", name, err)
		}
	}
	if requirement.Version != "latest" && !validPackageVersion(requirement.Version) && !isGitRevision(requirement.Version) {
		return fmt.Errorf("dependency %s has unsupported version %q", name, requirement.Version)
	}
	return nil
}

func validatePackageSource(source string) error {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	if strings.HasPrefix(source, "-") || strings.ContainsAny(source, "\r\n") {
		return errors.New("invalid Git source")
	}
	if !strings.Contains(source, "://") {
		return nil
	}
	parsed, err := url.Parse(source)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if parsed.User != nil {
		if _, containsPassword := parsed.User.Password(); containsPassword {
			return errors.New("Git source must not embed credentials; use the Git credential manager")
		}
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return errors.New("Git source must not contain a query or fragment")
	}
	return nil
}

func validExternalPackageName(name string) bool {
	clean := filepath.ToSlash(filepath.Clean(name))
	return clean == name && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") &&
		!strings.HasPrefix(clean, "trb/") && !filepath.IsAbs(name) && !strings.ContainsAny(name, " \\@")
}

func validPackageVersion(version string) bool {
	return semver.IsValid(canonicalVersion(version))
}

func canonicalVersion(version string) string {
	version = strings.TrimSpace(version)
	if version != "" && !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func escapesDirectory(name string) bool {
	clean := filepath.Clean(name)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func sortedManifestDependencies(values map[string]project.PackageRequirement) []string {
	result := make([]string, 0, len(values))
	for name := range values {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}
