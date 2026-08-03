// Package project owns trbconfig.jsonc discovery, validation, and persistence.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ConfigName = "trbconfig.jsonc"

const (
	ManagedPackages  = "managed"
	ExternalPackages = "external"
)

type Config struct {
	Schema            string            `json:"$schema,omitempty"`
	Name              string            `json:"name"`
	Version           string            `json:"version,omitempty"`
	Mode              string            `json:"mode"`
	SourceDir         string            `json:"sourceDir,omitempty"`
	OutDir            string            `json:"outDir,omitempty"`
	EntryPoint        string            `json:"entrypoint,omitempty"`
	CopyFiles         *bool             `json:"copyFiles,omitempty"`
	PackageManagement string            `json:"packageManagement,omitempty"`
	Dependencies      map[string]string `json:"dependencies,omitempty"`
	DevDependencies   map[string]string `json:"devDependencies,omitempty"`
	LocalPackages     map[string]string `json:"localPackages,omitempty"`
	Ruby              *RubyConfig       `json:"ruby,omitempty"`
	Go                *GoConfig         `json:"go,omitempty"`
	TypeScript        *TypeScriptConfig `json:"typescript,omitempty"`
	Root              string            `json:"-"`
	Path              string            `json:"-"`
}

type RubyConfig struct {
	Source  string `json:"source,omitempty"`
	Version string `json:"version,omitempty"`
	Loader  string `json:"loader,omitempty"`
}

type GoConfig struct {
	Module               string            `json:"module,omitempty"`
	Version              string            `json:"version,omitempty"`
	RootPackage          string            `json:"rootPackage,omitempty"`
	IndirectDependencies map[string]string `json:"indirectDependencies,omitempty"`
	Sqldef               *SqldefConfig     `json:"sqldef,omitempty"`
}

type SqldefConfig struct {
	Command   string   `json:"command"`
	Arguments []string `json:"arguments,omitempty"`
	Database  string   `json:"database"`
	Schema    string   `json:"schema"`
}

type TypeScriptConfig struct {
	PackageManager string            `json:"packageManager,omitempty"`
	ModuleType     string            `json:"moduleType,omitempty"`
	Scripts        map[string]string `json:"scripts,omitempty"`
}

func Find(start string) (*Config, error) {
	absolute, err := filepath.Abs(start)
	if err != nil {
		return nil, err
	}
	if info, statErr := os.Stat(absolute); statErr == nil && !info.IsDir() {
		absolute = filepath.Dir(absolute)
	}
	for {
		candidate := filepath.Join(absolute, ConfigName)
		if _, err := os.Stat(candidate); err == nil {
			return Load(candidate)
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		parent := filepath.Dir(absolute)
		if parent == absolute {
			return nil, fmt.Errorf("%s not found from %s", ConfigName, start)
		}
		absolute = parent
	}
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(stripJSONC(data)))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	config.Path = absolute
	config.Root = filepath.Dir(absolute)
	config.applyDefaults()
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &config, nil
}

func New(root, mode string) *Config {
	absolute, _ := filepath.Abs(root)
	copyFiles := true
	config := &Config{
		Name:            filepath.Base(absolute),
		Version:         "0.1.0",
		Mode:            mode,
		SourceDir:       ".",
		OutDir:          "build",
		CopyFiles:       &copyFiles,
		Dependencies:    map[string]string{},
		DevDependencies: map[string]string{},
		LocalPackages:   map[string]string{},
		EntryPoint:      "main",
		Root:            absolute,
		Path:            filepath.Join(absolute, ConfigName),
	}
	config.applyDefaults()
	return config
}

func (c *Config) Validate() error {
	switch c.Mode {
	case "ruby", "go", "typescript":
	default:
		return fmt.Errorf("mode must be ruby, go, or typescript; got %q", c.Mode)
	}
	if strings.TrimSpace(c.Name) == "" {
		return errors.New("name is required")
	}
	if filepath.IsAbs(c.SourceDir) || filepath.IsAbs(c.OutDir) {
		return errors.New("sourceDir and outDir must be relative to the project root")
	}
	if c.PackageManagement != ManagedPackages && c.PackageManagement != ExternalPackages {
		return fmt.Errorf("packageManagement must be managed or external; got %q", c.PackageManagement)
	}
	for name, packagePath := range c.LocalPackages {
		clean := filepath.ToSlash(filepath.Clean(name))
		if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "trb/") || filepath.IsAbs(name) {
			return fmt.Errorf("invalid local package name %q", name)
		}
		if strings.TrimSpace(packagePath) == "" {
			return fmt.Errorf("local package %s requires a path", name)
		}
	}
	if escapesRoot(c.SourceDir) || escapesRoot(c.OutDir) {
		return errors.New("sourceDir and outDir cannot escape the project root")
	}
	if c.Mode == "go" && (c.Go == nil || strings.TrimSpace(c.Go.Module) == "") {
		return errors.New("go.module is required for mode go")
	}
	if c.Go != nil && c.Go.Sqldef != nil {
		definition := c.Go.Sqldef
		if strings.TrimSpace(definition.Command) == "" || strings.TrimSpace(definition.Database) == "" || strings.TrimSpace(definition.Schema) == "" {
			return errors.New("go.sqldef requires command, database, and schema")
		}
		if filepath.IsAbs(definition.Database) || filepath.IsAbs(definition.Schema) || escapesRoot(definition.Database) || escapesRoot(definition.Schema) {
			return errors.New("go.sqldef database and schema must stay below the project root")
		}
	}
	if c.Ruby != nil && c.Ruby.Loader != "" && c.Ruby.Loader != "require_relative" && c.Ruby.Loader != "zeitwerk" {
		return fmt.Errorf("ruby.loader must be require_relative or zeitwerk; got %q", c.Ruby.Loader)
	}
	return nil
}

func (c *Config) Save() error {
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	data = append([]byte("// TypeRB project configuration. Comments and trailing commas are allowed.\n"), data...)
	data = append(data, '\n')
	return atomicWrite(c.Path, data, 0o644)
}

func (c *Config) SourcePath() string { return filepath.Join(c.Root, c.SourceDir) }
func (c *Config) OutputPath() string { return filepath.Join(c.Root, c.OutDir) }
func (c *Config) ShouldCopyFiles() bool {
	return c.CopyFiles == nil || *c.CopyFiles
}
func (c *Config) ManagesPackages() bool { return c.PackageManagement == ManagedPackages }

func (c *Config) applyDefaults() {
	if c.Version == "" {
		c.Version = "0.1.0"
	}
	if c.SourceDir == "" {
		c.SourceDir = "."
	}
	if c.OutDir == "" {
		c.OutDir = "build"
	}
	if c.PackageManagement == "" {
		c.PackageManagement = ManagedPackages
	}
	if c.Dependencies == nil {
		c.Dependencies = map[string]string{}
	}
	if c.DevDependencies == nil {
		c.DevDependencies = map[string]string{}
	}
	if c.LocalPackages == nil {
		c.LocalPackages = map[string]string{}
	}
	switch c.Mode {
	case "ruby":
		if c.Ruby == nil {
			c.Ruby = &RubyConfig{}
		}
		if c.Ruby.Source == "" {
			c.Ruby.Source = "https://rubygems.org"
		}
		if c.Ruby.Loader == "" {
			c.Ruby.Loader = "require_relative"
		}
	case "go":
		if c.Go == nil {
			c.Go = &GoConfig{}
		}
		if c.Go.Version == "" {
			c.Go.Version = "1.26"
		}
		if c.Go.RootPackage == "" {
			c.Go.RootPackage = "main"
		}
		if c.Go.IndirectDependencies == nil {
			c.Go.IndirectDependencies = map[string]string{}
		}
	case "typescript":
		if c.TypeScript == nil {
			c.TypeScript = &TypeScriptConfig{}
		}
		if c.TypeScript.PackageManager == "" {
			c.TypeScript.PackageManager = "npm"
		}
		if c.TypeScript.ModuleType == "" {
			c.TypeScript.ModuleType = "module"
		}
		if c.TypeScript.Scripts == nil {
			c.TypeScript.Scripts = map[string]string{}
		}
	}
}

func escapesRoot(path string) bool {
	clean := filepath.Clean(path)
	return clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator))
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".trbconfig-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func stripJSONC(source []byte) []byte {
	result := append([]byte(nil), source...)
	inString := false
	escaped := false
	for i := 0; i < len(result); i++ {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if result[i] == '\\' {
				escaped = true
			} else if result[i] == '"' {
				inString = false
			}
			continue
		}
		if result[i] == '"' {
			inString = true
			continue
		}
		if result[i] != '/' || i+1 >= len(result) {
			continue
		}
		if result[i+1] == '/' {
			result[i], result[i+1] = ' ', ' '
			i += 2
			for i < len(result) && result[i] != '\n' {
				result[i] = ' '
				i++
			}
			i--
		} else if result[i+1] == '*' {
			result[i], result[i+1] = ' ', ' '
			i += 2
			for i < len(result) {
				if i+1 < len(result) && result[i] == '*' && result[i+1] == '/' {
					result[i], result[i+1] = ' ', ' '
					i++
					break
				}
				if result[i] != '\n' && result[i] != '\r' {
					result[i] = ' '
				}
				i++
			}
		}
	}
	// encoding/json does not accept JSONC trailing commas. Replacing them with
	// spaces preserves line/column locations in diagnostics.
	inString, escaped = false, false
	for i := 0; i < len(result); i++ {
		if inString {
			if escaped {
				escaped = false
			} else if result[i] == '\\' {
				escaped = true
			} else if result[i] == '"' {
				inString = false
			}
			continue
		}
		if result[i] == '"' {
			inString = true
			continue
		}
		if result[i] != ',' {
			continue
		}
		j := i + 1
		for j < len(result) && (result[j] == ' ' || result[j] == '\t' || result[j] == '\n' || result[j] == '\r') {
			j++
		}
		if j < len(result) && (result[j] == '}' || result[j] == ']') {
			result[i] = ' '
		}
	}
	return result
}
