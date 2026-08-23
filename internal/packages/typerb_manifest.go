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
	FormatVersion             int                                   `json:"formatVersion"`
	Name                      string                                `json:"name"`
	Version                   string                                `json:"version"`
	SourceDir                 string                                `json:"sourceDir,omitempty"`
	Modes                     []string                              `json:"modes,omitempty"`
	Packages                  map[string]project.PackageRequirement `json:"packages,omitempty"`
	NativeDependencies        map[string]map[string]string          `json:"nativeDependencies,omitempty"`
	DeclarationAdapters       map[string]string                     `json:"declarationAdapters,omitempty"`
	RuntimeAdapters           map[string]string                     `json:"runtimeAdapters,omitempty"`
	AdapterTests              map[string]AdapterTest                `json:"adapterTests,omitempty"`
	LegacyNativeTypeProviders map[string]string                     `json:"nativeTypeProviders,omitempty"`
	CompilerExtension         json.RawMessage                       `json:"compilerExtension,omitempty"`
}

// AdapterTest declares one explicitly invoked conformance project for a
// package-owned declaration adapter. Command is an argv vector and is never
// interpreted by a shell.
type AdapterTest struct {
	Config  string   `json:"config"`
	Command []string `json:"command"`
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
	if m.DeclarationAdapters == nil {
		m.DeclarationAdapters = map[string]string{}
	}
	if m.RuntimeAdapters == nil {
		m.RuntimeAdapters = map[string]string{}
	}
	if m.AdapterTests == nil {
		m.AdapterTests = map[string]AdapterTest{}
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
	if len(m.LegacyNativeTypeProviders) != 0 {
		return errors.New("nativeTypeProviders has been replaced by declarationAdapters")
	}
	for mode, adapter := range m.DeclarationAdapters {
		if mode != "go" && mode != "ruby" && mode != "typescript" {
			return fmt.Errorf("declarationAdapters has unsupported mode %q", mode)
		}
		if filepath.IsAbs(adapter) || escapesDirectory(adapter) || strings.TrimSpace(adapter) == "" {
			return fmt.Errorf("declarationAdapters.%s must stay below the package root", mode)
		}
		info, err := os.Stat(filepath.Join(root, adapter))
		if err != nil {
			return fmt.Errorf("declarationAdapters.%s: %w", mode, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("declarationAdapters.%s must name a regular file", mode)
		}
	}
	for mode, adapter := range m.RuntimeAdapters {
		if mode != "go" && mode != "ruby" && mode != "typescript" {
			return fmt.Errorf("runtimeAdapters has unsupported mode %q", mode)
		}
		if m.DeclarationAdapterFor(mode) == "" {
			return fmt.Errorf("runtimeAdapters.%s requires declarationAdapters.%s", mode, mode)
		}
		if filepath.IsAbs(adapter) || escapesDirectory(adapter) || strings.TrimSpace(adapter) == "" {
			return fmt.Errorf("runtimeAdapters.%s must stay below the package root", mode)
		}
		info, err := os.Stat(filepath.Join(root, adapter))
		if err != nil {
			return fmt.Errorf("runtimeAdapters.%s: %w", mode, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("runtimeAdapters.%s must name a regular file", mode)
		}
	}
	for mode := range m.DeclarationAdapters {
		if mode != "typescript" && m.RuntimeAdapterFor(mode) == "" {
			return fmt.Errorf("declarationAdapters.%s requires runtimeAdapters.%s until direct %s declaration import is implemented", mode, mode, mode)
		}
	}
	testModes := make([]string, 0, len(m.AdapterTests))
	for mode := range m.AdapterTests {
		testModes = append(testModes, mode)
	}
	sort.Strings(testModes)
	for _, mode := range testModes {
		test := m.AdapterTests[mode]
		if !m.Supports(mode) {
			return fmt.Errorf("adapterTests has unsupported package mode %q", mode)
		}
		if m.DeclarationAdapterFor(mode) == "" {
			return fmt.Errorf("adapterTests.%s requires declarationAdapters.%s", mode, mode)
		}
		if filepath.IsAbs(test.Config) || escapesDirectory(test.Config) || strings.TrimSpace(test.Config) == "" {
			return fmt.Errorf("adapterTests.%s.config must stay below the package root", mode)
		}
		info, err := os.Stat(filepath.Join(root, test.Config))
		if err != nil {
			return fmt.Errorf("adapterTests.%s.config: %w", mode, err)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("adapterTests.%s.config must name a regular file", mode)
		}
		if len(test.Command) == 0 || strings.TrimSpace(test.Command[0]) == "" {
			return fmt.Errorf("adapterTests.%s.command must contain an executable", mode)
		}
		if filepath.IsAbs(test.Command[0]) || escapesDirectory(test.Command[0]) {
			return fmt.Errorf("adapterTests.%s.command executable must not be absolute or escape the conformance project", mode)
		}
		for _, argument := range test.Command {
			if strings.ContainsAny(argument, "\x00\r\n") {
				return fmt.Errorf("adapterTests.%s.command arguments must not contain NUL or newlines", mode)
			}
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

func (m *TypeRBManifest) DeclarationAdapterFor(mode string) string {
	return m.DeclarationAdapters[mode]
}

func (m *TypeRBManifest) RuntimeAdapterFor(mode string) string {
	return m.RuntimeAdapters[mode]
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
