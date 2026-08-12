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

// DefaultRubyVersion is the current Ruby toolchain supported by TypeRB.
const DefaultRubyVersion = "4.0.6"

// DefaultTypeScriptVersion follows the latest compatible TypeScript 6 patch.
const DefaultTypeScriptVersion = "^6.0.0"

// DefaultTypeScriptRuntime preserves the runtime used by TypeScript projects
// created before runtime selection became explicit.
const DefaultTypeScriptRuntime = TypeScriptRuntimeNode

// ErrConfigNotFound reports that discovery reached the filesystem root.
var ErrConfigNotFound = errors.New(ConfigName + " not found")

const (
	ManagedPackages  = "managed"
	ExternalPackages = "external"

	TypeScriptRuntimeBrowser = "browser"
	TypeScriptRuntimeBun     = "bun"
	TypeScriptRuntimeNode    = "node"
)

type Config struct {
	Schema            string                        `json:"$schema,omitempty"`
	Name              string                        `json:"name"`
	Version           string                        `json:"version,omitempty"`
	Mode              string                        `json:"mode"`
	SourceDir         string                        `json:"sourceDir,omitempty"`
	OutDir            string                        `json:"outDir,omitempty"`
	CopyFiles         *bool                         `json:"copyFiles,omitempty"`
	PackageManagement string                        `json:"packageManagement,omitempty"`
	Packages          map[string]PackageRequirement `json:"packages,omitempty"`
	Dependencies      map[string]string             `json:"dependencies,omitempty"`
	DevDependencies   map[string]string             `json:"devDependencies,omitempty"`
	LocalPackages     map[string]string             `json:"localPackages,omitempty"`
	PackageOptions    map[string]json.RawMessage    `json:"packageOptions,omitempty"`
	Ruby              *RubyConfig                   `json:"ruby,omitempty"`
	Go                *GoConfig                     `json:"go,omitempty"`
	TypeScript        *TypeScriptConfig             `json:"typescript,omitempty"`
	Database          *DatabaseConfig               `json:"db,omitempty"`
	Root              string                        `json:"-"`
	Path              string                        `json:"-"`
}

// PackageRequirement identifies one TypeRB package. A package name is an
// import alias in the current dependency graph; Source selects an explicit Git
// origin and Path selects an explicitly local development package.
type PackageRequirement struct {
	Source  string `json:"source,omitempty"`
	Version string `json:"version,omitempty"`
	Path    string `json:"path,omitempty"`
}

func (r *PackageRequirement) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return errors.New("package requirement cannot be null")
	}
	if trimmed[0] == '"' {
		return json.Unmarshal(trimmed, &r.Version)
	}
	type encoded PackageRequirement
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	return decoder.Decode((*encoded)(r))
}

func (r PackageRequirement) MarshalJSON() ([]byte, error) {
	if r.Source == "" && r.Path == "" {
		return json.Marshal(r.Version)
	}
	type encoded PackageRequirement
	return json.Marshal(encoded(r))
}

const DefaultSqldefVersion = "3.11.19"

type DatabaseConfig struct {
	Adapter  string          `json:"adapter"`
	Database *DatabaseSource `json:"database,omitempty"`
	Schema   string          `json:"schema,omitempty"`
	Lock     string          `json:"lock,omitempty"`
	Sqldef   *DBSqldefConfig `json:"sqldef,omitempty"`
}

type DatabaseSource struct {
	Value       string
	Environment string
}

func (s *DatabaseSource) UnmarshalJSON(data []byte) error {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil
	}
	if trimmed[0] == '"' {
		return json.Unmarshal(trimmed, &s.Value)
	}
	var encoded struct {
		Environment string `json:"environment"`
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&encoded); err != nil {
		return errors.New("must be a string or an environment source")
	}
	s.Environment = encoded.Environment
	return nil
}

func (s DatabaseSource) MarshalJSON() ([]byte, error) {
	if strings.TrimSpace(s.Environment) != "" {
		return json.Marshal(struct {
			Environment string `json:"environment"`
		}{Environment: s.Environment})
	}
	return json.Marshal(s.Value)
}

func (s DatabaseSource) Resolve(root, adapter string) (string, error) {
	value := strings.TrimSpace(s.Value)
	if environment := strings.TrimSpace(s.Environment); environment != "" {
		var found bool
		value, found = os.LookupEnv(environment)
		if !found || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("database environment %q is not set or empty", environment)
		}
		value = strings.TrimSpace(value)
	}
	if value == "" {
		return "", errors.New("db.database is required for this command")
	}
	if adapter == "sqlite" && !filepath.IsAbs(value) {
		value = filepath.Join(root, value)
	}
	return value, nil
}

type DBSqldefConfig struct {
	Command   string   `json:"command,omitempty"`
	Version   string   `json:"version,omitempty"`
	Arguments []string `json:"arguments,omitempty"`
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
	Runtime        string            `json:"runtime,omitempty"`
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
			return nil, fmt.Errorf("%w from %s", ErrConfigNotFound, start)
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
		Packages:        map[string]PackageRequirement{},
		Dependencies:    map[string]string{},
		DevDependencies: map[string]string{},
		LocalPackages:   map[string]string{},
		PackageOptions:  map[string]json.RawMessage{},
		Root:            absolute,
		Path:            filepath.Join(absolute, ConfigName),
	}
	if mode == "typescript" {
		config.DevDependencies["typescript"] = DefaultTypeScriptVersion
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
		if !validPackageName(name) {
			return fmt.Errorf("invalid local package name %q", name)
		}
		if strings.TrimSpace(packagePath) == "" {
			return fmt.Errorf("local package %s requires a path", name)
		}
	}
	for name, requirement := range c.Packages {
		if !validPackageName(name) {
			return fmt.Errorf("invalid TypeRB package name %q", name)
		}
		hasPath := strings.TrimSpace(requirement.Path) != ""
		hasSource := strings.TrimSpace(requirement.Source) != ""
		hasVersion := strings.TrimSpace(requirement.Version) != ""
		if hasPath {
			if hasSource || hasVersion {
				return fmt.Errorf("TypeRB package %s path cannot be combined with source or version", name)
			}
			continue
		}
		if !hasVersion {
			return fmt.Errorf("TypeRB package %s requires a version or path", name)
		}
	}
	if c.Mode == "typescript" {
		for name := range c.Dependencies {
			if _, exists := c.Packages[name]; exists {
				return fmt.Errorf("dependency %s is declared as both a TypeRB package and a native TypeScript package", name)
			}
			if _, exists := c.LocalPackages[name]; exists {
				return fmt.Errorf("dependency %s is declared as both a local TypeRB package and a native TypeScript package", name)
			}
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
	if c.Database != nil {
		database := c.Database
		database.Adapter = strings.ToLower(strings.TrimSpace(database.Adapter))
		switch database.Adapter {
		case "sqlite", "postgresql", "mysql":
		default:
			return fmt.Errorf("db.adapter must be sqlite, postgresql, or mysql; got %q", database.Adapter)
		}
		for name, path := range map[string]string{"schema": database.Schema, "lock": database.Lock} {
			if filepath.IsAbs(path) || escapesRoot(path) {
				return fmt.Errorf("db.%s must stay below the project root", name)
			}
		}
		if database.Database != nil && strings.TrimSpace(database.Database.Value) != "" && strings.TrimSpace(database.Database.Environment) != "" {
			return errors.New("db.database cannot contain both a value and environment")
		}
		if database.Database != nil && strings.TrimSpace(database.Database.Value) == "" && strings.TrimSpace(database.Database.Environment) == "" {
			return errors.New("db.database must be a non-empty string or environment source")
		}
	}
	if c.Ruby != nil && c.Ruby.Loader != "" && c.Ruby.Loader != "require_relative" && c.Ruby.Loader != "zeitwerk" {
		return fmt.Errorf("ruby.loader must be require_relative or zeitwerk; got %q", c.Ruby.Loader)
	}
	if c.TypeScript != nil {
		if c.TypeScript.Runtime != "" {
			switch c.TypeScript.Runtime {
			case TypeScriptRuntimeBrowser, TypeScriptRuntimeBun, TypeScriptRuntimeNode:
			default:
				return fmt.Errorf("typescript.runtime must be browser, bun, or node; got %q", c.TypeScript.Runtime)
			}
		}
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
	data = append([]byte("// TypeRB project configuration. Comments are allowed; trailing commas are not.\n"), data...)
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
	if c.Packages == nil {
		c.Packages = map[string]PackageRequirement{}
	}
	if c.DevDependencies == nil {
		c.DevDependencies = map[string]string{}
	}
	if c.LocalPackages == nil {
		c.LocalPackages = map[string]string{}
	}
	if c.PackageOptions == nil {
		c.PackageOptions = map[string]json.RawMessage{}
	}
	if c.Database != nil {
		c.Database.Adapter = strings.ToLower(strings.TrimSpace(c.Database.Adapter))
		if c.Database.Schema == "" {
			c.Database.Schema = "db/schema.sql"
		}
		if c.Database.Lock == "" {
			c.Database.Lock = "db/schema.lock.json"
		}
		if c.Database.Sqldef == nil {
			c.Database.Sqldef = &DBSqldefConfig{}
		}
		if c.Database.Sqldef.Command == "" {
			switch c.Database.Adapter {
			case "sqlite":
				c.Database.Sqldef.Command = "sqlite3def"
			case "postgresql":
				c.Database.Sqldef.Command = "psqldef"
			case "mysql":
				c.Database.Sqldef.Command = "mysqldef"
			}
		}
		if c.Database.Sqldef.Version == "" {
			c.Database.Sqldef.Version = DefaultSqldefVersion
		}
	}
	switch c.Mode {
	case "ruby":
		if c.Ruby == nil {
			c.Ruby = &RubyConfig{}
		}
		if c.Ruby.Source == "" {
			c.Ruby.Source = "https://rubygems.org"
		}
		if c.Ruby.Version == "" {
			c.Ruby.Version = DefaultRubyVersion
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
		if c.TypeScript.Runtime == "" {
			c.TypeScript.Runtime = DefaultTypeScriptRuntime
		}
		if c.TypeScript.PackageManager == "" {
			if c.TypeScript.Runtime == TypeScriptRuntimeBun {
				c.TypeScript.PackageManager = "bun"
			} else {
				c.TypeScript.PackageManager = "npm"
			}
		}
		if c.TypeScript.ModuleType == "" {
			c.TypeScript.ModuleType = "module"
		}
		if c.TypeScript.Scripts == nil {
			c.TypeScript.Scripts = map[string]string{}
		}
	}
}

func validPackageName(name string) bool {
	clean := filepath.ToSlash(filepath.Clean(name))
	return clean != "." && clean != ".." && clean == name && !strings.HasPrefix(clean, "../") &&
		!strings.HasPrefix(clean, "trb/") && !filepath.IsAbs(name) && !strings.ContainsAny(name, " \\@")
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
	return result
}
