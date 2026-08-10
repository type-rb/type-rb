// Package packages generates target package-manager manifests from
// trbconfig.jsonc. The config is the source of truth.
package packages

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/project"
)

func Sync(config *project.Config) (string, error) {
	return SyncWithDependencies(config, nil)
}

// SyncWithDependencies adds target dependencies required by imported TypeRB
// packages without copying those implementation details into trbconfig.jsonc.
func SyncWithDependencies(config *project.Config, packageDependencies map[string]string) (string, error) {
	if !config.ManagesPackages() {
		return "", fmt.Errorf("package management is external; trb will not modify the project manifest")
	}
	var name string
	var data []byte
	switch config.Mode {
	case "ruby":
		name = "Gemfile"
		var err error
		data, err = rubyManifest(config, packageDependencies)
		if err != nil {
			return "", err
		}
	case "go":
		name = "go.mod"
		var err error
		data, err = goManifest(config, packageDependencies)
		if err != nil {
			return "", err
		}
	case "typescript":
		name = "package.json"
		var err error
		data, err = npmManifest(config, packageDependencies)
		if err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported mode %q", config.Mode)
	}
	path := filepath.Join(config.Root, name)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	if config.Mode == "ruby" {
		versionPath := filepath.Join(config.Root, ".ruby-version")
		if err := os.WriteFile(versionPath, []byte(config.Ruby.Version+"\n"), 0o644); err != nil {
			return "", err
		}
	}
	return path, nil
}

func Install(config *project.Config, stdin io.Reader, stdout, stderr io.Writer) error {
	return InstallWithDependencies(config, nil, stdin, stdout, stderr)
}

func InstallWithDependencies(config *project.Config, packageDependencies map[string]string, stdin io.Reader, stdout, stderr io.Writer) error {
	if !config.ManagesPackages() {
		return fmt.Errorf("package management is external; install dependencies with the host project")
	}
	if _, err := SyncWithDependencies(config, packageDependencies); err != nil {
		return err
	}
	command, err := installCommand(config)
	if err != nil {
		return err
	}
	command.Dir = config.Root
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin
	return command.Run()
}

func installCommand(config *project.Config) (*exec.Cmd, error) {
	switch config.Mode {
	case "ruby":
		return exec.Command("ruby", "-e", `load Gem.bin_path("bundler", "bundle")`, "--", "install"), nil
	case "go":
		return exec.Command("go", "mod", "download", "all"), nil
	case "typescript":
		switch config.TypeScript.PackageManager {
		case "bun":
			return exec.Command("bun", "install"), nil
		case "npm":
			return exec.Command("npm", "install"), nil
		default:
			return nil, fmt.Errorf("unsupported TypeScript package manager %q", config.TypeScript.PackageManager)
		}
	default:
		return nil, fmt.Errorf("unsupported mode %q", config.Mode)
	}
}

func rubyManifest(config *project.Config, packageDependencies map[string]string) ([]byte, error) {
	dependencies, err := mergeDependencies(config.Dependencies, packageDependencies)
	if err != nil {
		return nil, err
	}
	var out strings.Builder
	out.WriteString("# Generated from trbconfig.jsonc by trb.\n")
	out.WriteString("source " + strconv.Quote(config.Ruby.Source) + "\n")
	if config.Ruby.Version != "" {
		out.WriteString("ruby " + strconv.Quote(config.Ruby.Version) + "\n")
	}
	for _, name := range sortedKeys(dependencies) {
		out.WriteString(gemLine(name, dependencies[name], ""))
	}
	if len(config.DevDependencies) > 0 {
		out.WriteString("\ngroup :development, :test do\n")
		for _, name := range sortedKeys(config.DevDependencies) {
			out.WriteString(gemLine(name, config.DevDependencies[name], "  "))
		}
		out.WriteString("end\n")
	}
	return []byte(out.String()), nil
}

func gemLine(name, version, indent string) string {
	line := indent + "gem " + strconv.Quote(name)
	if version != "" && version != "latest" {
		line += ", " + strconv.Quote(version)
	}
	return line + "\n"
}

func goManifest(config *project.Config, packageDependencies map[string]string) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Generated from trbconfig.jsonc by trb.\n")
	out.WriteString("module " + config.Go.Module + "\n\n")
	out.WriteString("go " + config.Go.Version + "\n")
	dependencies, err := mergeDependencies(config.Dependencies, packageDependencies)
	if err != nil {
		return nil, err
	}
	all := map[string]string{}
	for name, version := range dependencies {
		all[name] = version
	}
	for name, version := range config.DevDependencies {
		all[name] = version
	}
	if len(all) > 0 {
		out.WriteString("\nrequire (\n")
		for _, name := range sortedKeys(all) {
			version := all[name]
			if version == "" || version == "latest" {
				return nil, fmt.Errorf("Go dependency %s requires an explicit module version", name)
			}
			out.WriteString("\t" + name + " " + version + "\n")
		}
		out.WriteString(")\n")
	}
	if len(config.Go.IndirectDependencies) > 0 {
		out.WriteString("\nrequire (\n")
		for _, name := range sortedKeys(config.Go.IndirectDependencies) {
			version := config.Go.IndirectDependencies[name]
			if version == "" || version == "latest" {
				return nil, fmt.Errorf("Go indirect dependency %s requires an explicit module version", name)
			}
			out.WriteString("\t" + name + " " + version + " // indirect\n")
		}
		out.WriteString(")\n")
	}
	return []byte(out.String()), nil
}

func npmManifest(config *project.Config, packageDependencies map[string]string) ([]byte, error) {
	dependencies, err := mergeDependencies(config.Dependencies, packageDependencies)
	if err != nil {
		return nil, err
	}
	manifest := struct {
		Name            string            `json:"name"`
		Version         string            `json:"version"`
		Private         bool              `json:"private"`
		Type            string            `json:"type,omitempty"`
		PackageManager  string            `json:"packageManager,omitempty"`
		Scripts         map[string]string `json:"scripts,omitempty"`
		Dependencies    map[string]string `json:"dependencies,omitempty"`
		DevDependencies map[string]string `json:"devDependencies,omitempty"`
	}{
		Name:            config.Name,
		Version:         config.Version,
		Private:         true,
		Type:            config.TypeScript.ModuleType,
		PackageManager:  config.TypeScript.PackageManager,
		Scripts:         config.TypeScript.Scripts,
		Dependencies:    dependencies,
		DevDependencies: config.DevDependencies,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func mergeDependencies(configured, required map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(configured)+len(required))
	for name, version := range configured {
		result[name] = version
	}
	for name, version := range required {
		if configuredVersion, exists := result[name]; exists && configuredVersion != version {
			return nil, fmt.Errorf("dependency %s is configured as %s but an imported TypeRB package requires %s", name, configuredVersion, version)
		}
		result[name] = version
	}
	return result, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// EqualManifest is used by checks without modifying a project.
func EqualManifest(path string, expected []byte) (bool, error) {
	actual, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return bytes.Equal(actual, expected), nil
}
