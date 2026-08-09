package orm

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/go-sql-driver/mysql"

	"github.com/type-rb/type-rb/internal/types"
)

type mysqlIntrospector struct{}

func (mysqlIntrospector) Inspect(config Config) (*Schema, error) {
	database, err := sql.Open("mysql", config.Database)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	if err := database.Ping(); err != nil {
		return nil, err
	}
	rows, err := database.Query(`
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
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
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	schema := &Schema{Adapter: config.Adapter, Database: config.Database}
	for _, name := range names {
		table, err := inspectMySQLTable(database, name)
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

func inspectMySQLTable(database *sql.DB, name string) (Table, error) {
	rows, err := database.Query(`
		SELECT ordinal_position, column_name, data_type, column_type, is_nullable, column_key
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
		ORDER BY ordinal_position`, name)
	if err != nil {
		return Table{}, err
	}
	table := Table{Name: name}
	for rows.Next() {
		var position int
		var columnName, dataType, databaseType, nullableText, columnKey string
		if err := rows.Scan(&position, &columnName, &dataType, &databaseType, &nullableText, &columnKey); err != nil {
			rows.Close()
			return Table{}, err
		}
		typ, err := mysqlColumnType(dataType, databaseType)
		if err != nil {
			rows.Close()
			return Table{}, fmt.Errorf("table %s column %s: %w", name, columnName, err)
		}
		primaryKey := columnKey == "PRI"
		nullable := nullableText == "YES" && !primaryKey
		typ.Nullable = nullable
		table.Columns = append(table.Columns, Column{
			Name: columnName, DatabaseType: databaseType, Type: typ, Nullable: nullable,
			PrimaryKey: primaryKey, Position: position - 1,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Table{}, err
	}
	if err := rows.Close(); err != nil {
		return Table{}, err
	}
	foreignKeys, err := inspectMySQLForeignKeys(database, name)
	if err != nil {
		return Table{}, err
	}
	table.ForeignKeys = foreignKeys
	return table, nil
}

func inspectMySQLForeignKeys(database *sql.DB, name string) ([]ForeignKey, error) {
	rows, err := database.Query(`
		SELECT constraint_name, ordinal_position, column_name,
		       referenced_table_name, referenced_column_name
		FROM information_schema.key_column_usage
		WHERE table_schema = DATABASE()
		  AND table_name = ?
		  AND referenced_table_name IS NOT NULL
		ORDER BY constraint_name, ordinal_position`, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := map[string]int{}
	var result []ForeignKey
	for rows.Next() {
		var constraint, column, referencedTable, referencedColumn string
		var position int
		if err := rows.Scan(&constraint, &position, &column, &referencedTable, &referencedColumn); err != nil {
			return nil, err
		}
		id, ok := ids[constraint]
		if !ok {
			id = len(ids)
			ids[constraint] = id
		}
		result = append(result, ForeignKey{
			ID: id, Sequence: position - 1, Column: column,
			ReferencedTable: referencedTable, ReferencedColumn: referencedColumn,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func mysqlColumnType(dataType, databaseType string) (types.Type, error) {
	normalizedType := strings.ToLower(strings.TrimSpace(dataType))
	normalizedDatabaseType := strings.ToLower(strings.TrimSpace(databaseType))
	switch normalizedType {
	case "tinyint":
		if strings.HasPrefix(normalizedDatabaseType, "tinyint(1)") {
			return types.FromName("Boolean"), nil
		}
		return types.FromName("Integer"), nil
	case "smallint", "mediumint", "int", "integer", "bigint":
		return types.FromName("Integer"), nil
	case "decimal", "numeric", "float", "double", "real":
		return types.FromName("Float"), nil
	case "char", "varchar", "tinytext", "text", "mediumtext", "longtext", "enum", "set", "json":
		return types.FromName("String"), nil
	case "binary", "varbinary", "tinyblob", "blob", "mediumblob", "longblob":
		return types.FromName("Bytes"), nil
	default:
		return types.Type{}, fmt.Errorf("unsupported MySQL column type %q", databaseType)
	}
}
