// Package orm owns the compiler-side integration for the official trb/orm
// package. The compiler pipeline transports its declarations and IR manifest
// without depending on database adapters or ORM semantics.
package orm

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
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
	Adapter             string
	Database            string
	DatabaseEnvironment string
	Tables              []Table
}

type Table struct {
	Name              string
	Columns           []Column
	ForeignKeys       []ForeignKey
	UniqueConstraints []UniqueConstraint
}

type UniqueConstraint struct {
	Name    string
	Columns []string
	Primary bool
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
	HasDefault   bool
	Generated    bool
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
	var encoded struct {
		Adapter  string          `json:"adapter"`
		Database json.RawMessage `json:"database"`
	}
	if err := decoder.Decode(&encoded); err != nil {
		return nil, fmt.Errorf("packageOptions.%q: %w", PackageName, err)
	}
	config := Config{Adapter: encoded.Adapter}
	databaseEnvironment, err := decodeDatabaseSource(encoded.Database)
	if err != nil {
		return nil, fmt.Errorf("packageOptions.%q.database: %w", PackageName, err)
	}
	if databaseEnvironment == "" {
		if err := json.Unmarshal(encoded.Database, &config.Database); err != nil {
			return nil, fmt.Errorf("packageOptions.%q.database: must be a string or environment source", PackageName)
		}
	} else {
		value, found := os.LookupEnv(databaseEnvironment)
		if !found || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("packageOptions.%q.database.environment %q is not set or empty", PackageName, databaseEnvironment)
		}
		config.Database = value
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
		if databaseEnvironment != "" {
			return nil, fmt.Errorf("packageOptions.%q.database.environment %q must contain an absolute SQLite path", PackageName, databaseEnvironment)
		}
		config.Database = filepath.Join(projectRoot, config.Database)
	}

	definition, ok := adapterDefinitionFor(config.Adapter)
	if !ok {
		return nil, fmt.Errorf("unsupported trb/orm adapter %q", config.Adapter)
	}
	schema, err := definition.Introspector.Inspect(config)
	if err != nil {
		return nil, fmt.Errorf("inspect %s database: %w", config.Adapter, err)
	}
	schema.DatabaseEnvironment = databaseEnvironment
	sort.Slice(schema.Tables, func(i, j int) bool { return schema.Tables[i].Name < schema.Tables[j].Name })
	for index := range schema.Tables {
		sort.Slice(schema.Tables[index].Columns, func(i, j int) bool {
			return schema.Tables[index].Columns[i].Position < schema.Tables[index].Columns[j].Position
		})
		sort.Slice(schema.Tables[index].UniqueConstraints, func(i, j int) bool {
			left, right := schema.Tables[index].UniqueConstraints[i], schema.Tables[index].UniqueConstraints[j]
			if left.Primary != right.Primary {
				return left.Primary
			}
			return left.Name < right.Name
		})
	}
	return schema, nil
}

func decodeDatabaseSource(raw json.RawMessage) (string, error) {
	value := strings.TrimSpace(string(raw))
	if value == "" || value == "null" {
		return "", errors.New("is required")
	}
	if strings.HasPrefix(value, `"`) {
		return "", nil
	}
	if !strings.HasPrefix(value, "{") {
		return "", errors.New("must be a string or environment source")
	}
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.DisallowUnknownFields()
	var source struct {
		Environment string `json:"environment"`
	}
	if err := decoder.Decode(&source); err != nil {
		return "", err
	}
	source.Environment = strings.TrimSpace(source.Environment)
	if source.Environment == "" {
		return "", errors.New("environment is required")
	}
	return source.Environment, nil
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

func completeUniqueConstraints(table *Table) {
	if table == nil {
		return
	}
	var primaryColumns []string
	for _, column := range table.Columns {
		if column.PrimaryKey {
			primaryColumns = append(primaryColumns, column.Name)
		}
	}
	if len(primaryColumns) > 0 {
		found := false
		for index := range table.UniqueConstraints {
			if table.UniqueConstraints[index].Primary {
				found = true
				break
			}
		}
		if !found {
			table.UniqueConstraints = append(table.UniqueConstraints, UniqueConstraint{
				Name: "primary", Columns: primaryColumns, Primary: true,
			})
		}
	}
	sort.SliceStable(table.UniqueConstraints, func(i, j int) bool {
		return table.UniqueConstraints[i].Primary && !table.UniqueConstraints[j].Primary
	})
	seen := map[string]bool{}
	result := make([]UniqueConstraint, 0, len(table.UniqueConstraints))
	for _, constraint := range table.UniqueConstraints {
		if len(constraint.Columns) == 0 {
			continue
		}
		key := strings.Join(constraint.Columns, "\x00")
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, constraint)
	}
	table.UniqueConstraints = result
}
