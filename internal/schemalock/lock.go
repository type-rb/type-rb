// Package schemalock owns the deterministic, backend-independent database
// schema artifact consumed by compiler integrations and command-line tools.
package schemalock

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const FormatVersion = 1

type Lock struct {
	FormatVersion int              `json:"formatVersion"`
	Adapter       string           `json:"adapter"`
	Tables        map[string]Table `json:"tables"`
}

type Table struct {
	Columns           map[string]Column           `json:"columns"`
	ForeignKeys       map[string]ForeignKey       `json:"foreignKeys,omitempty"`
	UniqueConstraints map[string]UniqueConstraint `json:"uniqueConstraints,omitempty"`
}

type Column struct {
	// DatabaseType is used while parsing SQL but is intentionally omitted from
	// the lock. The lock records the portable type contract, while sqldef owns
	// database-specific details such as varchar lengths and storage aliases.
	DatabaseType string `json:"-"`
	Type         string `json:"type"`
	Nullable     bool   `json:"nullable"`
	PrimaryKey   bool   `json:"primaryKey,omitempty"`
	HasDefault   bool   `json:"hasDefault,omitempty"`
	Generated    bool   `json:"generated,omitempty"`
}

type ForeignKey struct {
	Column           string `json:"column"`
	ReferencedTable  string `json:"referencedTable"`
	ReferencedColumn string `json:"referencedColumn"`
}

type UniqueConstraint struct {
	Columns []string `json:"columns"`
	Primary bool     `json:"primary,omitempty"`
}

func New(adapter string) *Lock {
	return &Lock{FormatVersion: FormatVersion, Adapter: strings.ToLower(strings.TrimSpace(adapter)), Tables: map[string]Table{}}
}

func Read(path string) (*Lock, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var lock Lock
	if err := decoder.Decode(&lock); err != nil {
		return nil, fmt.Errorf("decode schema lock: %w", err)
	}
	if err := lock.Validate(); err != nil {
		return nil, err
	}
	return &lock, nil
}

func (l *Lock) Validate() error {
	if l == nil {
		return errors.New("schema lock is empty")
	}
	if l.FormatVersion != FormatVersion {
		return fmt.Errorf("unsupported schema lock formatVersion %d; expected %d", l.FormatVersion, FormatVersion)
	}
	switch l.Adapter {
	case "sqlite", "postgresql", "mysql":
	default:
		return fmt.Errorf("unsupported schema lock adapter %q", l.Adapter)
	}
	if l.Tables == nil {
		return errors.New("schema lock tables are missing")
	}
	for tableName, table := range l.Tables {
		if strings.TrimSpace(tableName) == "" {
			return errors.New("schema lock contains an empty table name")
		}
		if len(table.Columns) == 0 {
			return fmt.Errorf("schema lock table %s has no columns", tableName)
		}
		for columnName, column := range table.Columns {
			if strings.TrimSpace(columnName) == "" || strings.TrimSpace(column.Type) == "" {
				return fmt.Errorf("schema lock table %s contains an incomplete column", tableName)
			}
			switch column.Type {
			case "Boolean", "Bytes", "Date", "DateTime", "Float", "Instant", "Integer", "String", "TimeOfDay":
			default:
				return fmt.Errorf("schema lock table %s column %s has unsupported portable type %q", tableName, columnName, column.Type)
			}
		}
		for key, foreignKey := range table.ForeignKeys {
			if key != ForeignKeyKey(foreignKey.Column, foreignKey.ReferencedTable, foreignKey.ReferencedColumn) {
				return fmt.Errorf("schema lock table %s has non-canonical foreign key %q", tableName, key)
			}
			if _, ok := table.Columns[foreignKey.Column]; !ok {
				return fmt.Errorf("schema lock table %s foreign key references unknown column %s", tableName, foreignKey.Column)
			}
			referenced, ok := l.Tables[foreignKey.ReferencedTable]
			if !ok {
				return fmt.Errorf("schema lock table %s foreign key references unknown table %s", tableName, foreignKey.ReferencedTable)
			}
			if _, ok := referenced.Columns[foreignKey.ReferencedColumn]; !ok {
				return fmt.Errorf("schema lock table %s foreign key references unknown column %s.%s", tableName, foreignKey.ReferencedTable, foreignKey.ReferencedColumn)
			}
		}
		for key, constraint := range table.UniqueConstraints {
			if len(constraint.Columns) == 0 {
				return fmt.Errorf("schema lock table %s contains an empty unique constraint", tableName)
			}
			if !sort.StringsAreSorted(constraint.Columns) || key != ConstraintKey(constraint.Primary, constraint.Columns) {
				return fmt.Errorf("schema lock table %s has non-canonical unique constraint %q", tableName, key)
			}
			for _, column := range constraint.Columns {
				if _, ok := table.Columns[column]; !ok {
					return fmt.Errorf("schema lock table %s unique constraint references unknown column %s", tableName, column)
				}
			}
		}
	}
	return nil
}

func (l *Lock) Bytes() ([]byte, error) {
	if err := l.Validate(); err != nil {
		return nil, err
	}
	var data bytes.Buffer
	encoder := json.NewEncoder(&data)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(l); err != nil {
		return nil, err
	}
	return data.Bytes(), nil
}

func (l *Lock) Write(path string) error {
	data, err := l.Bytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".schema-lock-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o644); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func Equal(left, right *Lock) (bool, error) {
	leftData, err := left.Bytes()
	if err != nil {
		return false, err
	}
	rightData, err := right.Bytes()
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftData, rightData), nil
}

func ConstraintKey(primary bool, columns []string) string {
	prefix := "unique:"
	if primary {
		prefix = "primary:"
	}
	canonical := append([]string(nil), columns...)
	sort.Strings(canonical)
	return prefix + strings.Join(canonical, ",")
}

func ForeignKeyKey(column, table, referencedColumn string) string {
	return column + "->" + table + "." + referencedColumn
}
