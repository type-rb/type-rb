//go:build !js || !wasm

package orm

import (
	"database/sql"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"

	"github.com/type-rb/type-rb/internal/types"
)

type sqliteIntrospector struct{}

func (sqliteIntrospector) Inspect(config Config) (*Schema, error) {
	database, err := sql.Open("sqlite", config.Database)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	rows, err := database.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return nil, err
		}
		names = append(names, name)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	schema := &Schema{Adapter: config.Adapter, Database: config.Database}
	for _, name := range names {
		table, err := inspectSQLiteTable(database, name)
		if err != nil {
			return nil, err
		}
		schema.Tables = append(schema.Tables, table)
	}
	if err := validateSchema(schema); err != nil {
		return nil, err
	}
	return schema, nil
}

func inspectSQLiteTable(database *sql.DB, name string) (Table, error) {
	rows, err := database.Query("PRAGMA table_info(" + quoteSQLiteIdentifier(name) + ")")
	if err != nil {
		return Table{}, err
	}
	table := Table{Name: name}
	for rows.Next() {
		var position int
		var columnName, databaseType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&position, &columnName, &databaseType, &notNull, &defaultValue, &primaryKey); err != nil {
			return Table{}, err
		}
		typ, err := sqliteColumnType(databaseType)
		if err != nil {
			return Table{}, fmt.Errorf("table %s column %s: %w", name, columnName, err)
		}
		nullable := notNull == 0 && primaryKey == 0
		generated := primaryKey != 0 && strings.EqualFold(strings.TrimSpace(databaseType), "integer")
		typ.Nullable = nullable
		table.Columns = append(table.Columns, Column{
			Name:         columnName,
			DatabaseType: databaseType,
			Type:         typ,
			Nullable:     nullable,
			PrimaryKey:   primaryKey != 0,
			HasDefault:   defaultValue != nil || generated,
			Generated:    generated,
			Position:     position,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Table{}, err
	}
	if err := rows.Close(); err != nil {
		return Table{}, err
	}
	foreignKeys, err := inspectSQLiteForeignKeys(database, name)
	if err != nil {
		return Table{}, err
	}
	table.ForeignKeys = foreignKeys
	uniqueConstraints, err := inspectSQLiteUniqueConstraints(database, name)
	if err != nil {
		return Table{}, err
	}
	table.UniqueConstraints = uniqueConstraints
	completeUniqueConstraints(&table)
	return table, nil
}

func inspectSQLiteUniqueConstraints(database *sql.DB, name string) ([]UniqueConstraint, error) {
	rows, err := database.Query("PRAGMA index_list(" + quoteSQLiteIdentifier(name) + ")")
	if err != nil {
		return nil, err
	}
	type index struct {
		name    string
		primary bool
	}
	var indexes []index
	for rows.Next() {
		var sequence, unique, partial int
		var indexName, origin string
		if err := rows.Scan(&sequence, &indexName, &unique, &origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		if unique != 0 && partial == 0 {
			indexes = append(indexes, index{name: indexName, primary: origin == "pk"})
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	var result []UniqueConstraint
	for _, current := range indexes {
		columns, supported, err := inspectSQLiteIndexColumns(database, current.name)
		if err != nil {
			return nil, err
		}
		if supported {
			result = append(result, UniqueConstraint{Name: current.name, Columns: columns, Primary: current.primary})
		}
	}
	return result, nil
}

func inspectSQLiteIndexColumns(database *sql.DB, name string) ([]string, bool, error) {
	rows, err := database.Query("PRAGMA index_info(" + quoteSQLiteIdentifier(name) + ")")
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	var columns []string
	for rows.Next() {
		var sequence, columnID int
		var columnName sql.NullString
		if err := rows.Scan(&sequence, &columnID, &columnName); err != nil {
			return nil, false, err
		}
		if columnID < 0 || !columnName.Valid {
			return nil, false, nil
		}
		columns = append(columns, columnName.String)
	}
	if err := rows.Err(); err != nil {
		return nil, false, err
	}
	return columns, len(columns) > 0, nil
}

func inspectSQLiteForeignKeys(database *sql.DB, name string) ([]ForeignKey, error) {
	rows, err := database.Query("PRAGMA foreign_key_list(" + quoteSQLiteIdentifier(name) + ")")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ForeignKey
	for rows.Next() {
		var foreignKey ForeignKey
		var referencedColumn sql.NullString
		var onUpdate, onDelete, match string
		if err := rows.Scan(
			&foreignKey.ID, &foreignKey.Sequence, &foreignKey.ReferencedTable,
			&foreignKey.Column, &referencedColumn, &onUpdate, &onDelete, &match,
		); err != nil {
			return nil, err
		}
		foreignKey.ReferencedColumn = referencedColumn.String
		result = append(result, foreignKey)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func sqliteColumnType(databaseType string) (types.Type, error) {
	normalized := strings.ToLower(strings.TrimSpace(databaseType))
	switch {
	case strings.Contains(normalized, "int"):
		return types.FromName("Integer"), nil
	case strings.Contains(normalized, "char"), strings.Contains(normalized, "clob"), strings.Contains(normalized, "text"):
		return types.FromName("String"), nil
	case strings.Contains(normalized, "real"), strings.Contains(normalized, "floa"), strings.Contains(normalized, "doub"):
		return types.FromName("Float"), nil
	case strings.Contains(normalized, "bool"):
		return types.FromName("Boolean"), nil
	case strings.Contains(normalized, "blob"):
		return types.FromName("Bytes"), nil
	default:
		return types.Type{}, fmt.Errorf("unsupported SQLite column type %q", databaseType)
	}
}

func quoteSQLiteIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
