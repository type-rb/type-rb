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
	if !config.ManagesPackages() {
		return "", fmt.Errorf("package management is external; trb will not modify the project manifest")
	}
	var name string
	var data []byte
	switch config.Mode {
	case "ruby":
		name = "Gemfile"
		data = rubyManifest(config)
	case "go":
		name = "go.mod"
		var err error
		data, err = goManifest(config)
		if err != nil {
			return "", err
		}
	case "typescript":
		name = "package.json"
		var err error
		data, err = npmManifest(config)
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
	if !config.ManagesPackages() {
		return fmt.Errorf("package management is external; install dependencies with the host project")
	}
	if _, err := Sync(config); err != nil {
		return err
	}
	var command *exec.Cmd
	switch config.Mode {
	case "ruby":
		command = exec.Command("bundle", "install")
	case "go":
		command = exec.Command("go", "mod", "download", "all")
	case "typescript":
		if config.TypeScript.PackageManager != "npm" {
			return fmt.Errorf("v0.1 supports npm package management; got %q", config.TypeScript.PackageManager)
		}
		command = exec.Command("npm", "install")
	}
	command.Dir = config.Root
	command.Stdout = stdout
	command.Stderr = stderr
	command.Stdin = stdin
	return command.Run()
}

func rubyManifest(config *project.Config) []byte {
	var out strings.Builder
	out.WriteString("# Generated from trbconfig.jsonc by trb.\n")
	out.WriteString("source " + strconv.Quote(config.Ruby.Source) + "\n")
	if config.Ruby.Version != "" {
		out.WriteString("ruby " + strconv.Quote(config.Ruby.Version) + "\n")
	}
	for _, name := range sortedKeys(config.Dependencies) {
		out.WriteString(gemLine(name, config.Dependencies[name], ""))
	}
	if len(config.DevDependencies) > 0 {
		out.WriteString("\ngroup :development, :test do\n")
		for _, name := range sortedKeys(config.DevDependencies) {
			out.WriteString(gemLine(name, config.DevDependencies[name], "  "))
		}
		out.WriteString("end\n")
	}
	return []byte(out.String())
}

func gemLine(name, version, indent string) string {
	line := indent + "gem " + strconv.Quote(name)
	if version != "" && version != "latest" {
		line += ", " + strconv.Quote(version)
	}
	return line + "\n"
}

func goManifest(config *project.Config) ([]byte, error) {
	var out strings.Builder
	out.WriteString("// Generated from trbconfig.jsonc by trb.\n")
	out.WriteString("module " + config.Go.Module + "\n\n")
	out.WriteString("go " + config.Go.Version + "\n")
	all := map[string]string{}
	for name, version := range config.Dependencies {
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

func npmManifest(config *project.Config) ([]byte, error) {
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
		Dependencies:    config.Dependencies,
		DevDependencies: config.DevDependencies,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
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
