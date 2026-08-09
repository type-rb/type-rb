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
	defer rows.Close()
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
		typ.Nullable = nullable
		table.Columns = append(table.Columns, Column{
			Name:         columnName,
			DatabaseType: databaseType,
			Type:         typ,
			Nullable:     nullable,
			PrimaryKey:   primaryKey != 0,
			Position:     position,
		})
	}
	if err := rows.Err(); err != nil {
		return Table{}, err
	}
	return table, nil
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
