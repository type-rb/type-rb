// Package orm owns the compiler-side integration for the official trb/orm
// package. The compiler pipeline transports its declarations and IR manifest
// without depending on database adapters or ORM semantics.
package orm

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/types"
)

const (
	PackageName     = "trb/orm"
	TypeProvider    = "trb.orm"
	ProjectProvider = "trb.orm.schema"
)

type Config struct {
	Adapter  string `json:"adapter"`
	Database string `json:"database"`
}

type Schema struct {
	Adapter  string
	Database string
	Tables   []Table
}

type Table struct {
	Name        string
	Columns     []Column
	ForeignKeys []ForeignKey
}

type ForeignKey struct {
	ID               int
	Sequence         int
	Column           string
	ReferencedTable  string
	ReferencedColumn string
}

type Column struct {
	Name         string
	DatabaseType string
	Type         types.Type
	Nullable     bool
	PrimaryKey   bool
	Position     int
}

type Introspector interface {
	Inspect(Config) (*Schema, error)
}

func LoadSchema(projectRoot string, options map[string][]byte) (*Schema, error) {
	raw := options[PackageName]
	if len(raw) == 0 {
		return nil, fmt.Errorf("%s requires packageOptions.%q", PackageName, PackageName)
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("packageOptions.%q: %w", PackageName, err)
	}
	config.Adapter = strings.ToLower(strings.TrimSpace(config.Adapter))
	config.Database = strings.TrimSpace(config.Database)
	if config.Adapter == "" {
		return nil, fmt.Errorf("packageOptions.%q.adapter is required", PackageName)
	}
	if config.Database == "" {
		return nil, fmt.Errorf("packageOptions.%q.database is required", PackageName)
	}
	if config.Adapter == "sqlite" && !filepath.IsAbs(config.Database) {
		config.Database = filepath.Join(projectRoot, config.Database)
	}

	var introspector Introspector
	switch config.Adapter {
	case "sqlite":
		introspector = sqliteIntrospector{}
	case "postgresql", "mysql":
		return nil, fmt.Errorf("trb/orm adapter %s is not implemented yet", config.Adapter)
	default:
		return nil, fmt.Errorf("unsupported trb/orm adapter %q", config.Adapter)
	}
	schema, err := introspector.Inspect(config)
	if err != nil {
		return nil, fmt.Errorf("inspect %s database: %w", config.Adapter, err)
	}
	sort.Slice(schema.Tables, func(i, j int) bool { return schema.Tables[i].Name < schema.Tables[j].Name })
	for index := range schema.Tables {
		sort.Slice(schema.Tables[index].Columns, func(i, j int) bool {
			return schema.Tables[index].Columns[i].Position < schema.Tables[index].Columns[j].Position
		})
	}
	return schema, nil
}

func (s *Schema) Table(name string) (Table, bool) {
	if s == nil {
		return Table{}, false
	}
	for _, table := range s.Tables {
		if table.Name == name {
			return table, true
		}
	}
	return Table{}, false
}

func TableName(model string) string {
	words := splitIdentifier(model)
	if len(words) == 0 {
		return ""
	}
	name := strings.Join(words, "_")
	if strings.HasSuffix(name, "s") || strings.HasSuffix(name, "x") || strings.HasSuffix(name, "z") ||
		strings.HasSuffix(name, "ch") || strings.HasSuffix(name, "sh") {
		return name + "es"
	}
	if strings.HasSuffix(name, "y") && len(name) > 1 && !strings.ContainsRune("aeiou", rune(name[len(name)-2])) {
		return strings.TrimSuffix(name, "y") + "ies"
	}
	return name + "s"
}

func splitIdentifier(value string) []string {
	var words []string
	start := 0
	for index, character := range value {
		if index > 0 && character >= 'A' && character <= 'Z' {
			words = append(words, strings.ToLower(value[start:index]))
			start = index
		}
	}
	if start < len(value) {
		words = append(words, strings.ToLower(value[start:]))
	}
	return words
}

func validateSchema(schema *Schema) error {
	if schema == nil {
		return errors.New("introspector returned no schema")
	}
	if len(schema.Tables) == 0 {
		return errors.New("database has no application tables")
	}
	return nil
}
