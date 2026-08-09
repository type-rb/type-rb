//go:build !js || !wasm

package orm

import (
	"database/sql"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/type-rb/type-rb/internal/types"
)

type postgresqlIntrospector struct{}

func (postgresqlIntrospector) Inspect(config Config) (*Schema, error) {
	database, err := sql.Open("pgx", config.Database)
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
		WHERE table_schema = current_schema() AND table_type = 'BASE TABLE'
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
		table, err := inspectPostgreSQLTable(database, name)
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

func inspectPostgreSQLTable(database *sql.DB, name string) (Table, error) {
	rows, err := database.Query(`
		SELECT c.ordinal_position, c.column_name, c.data_type, c.udt_name, c.is_nullable,
		       c.column_default, c.is_identity,
		       EXISTS (
		         SELECT 1
		         FROM information_schema.table_constraints tc
		         JOIN information_schema.key_column_usage kcu
		           ON kcu.constraint_catalog = tc.constraint_catalog
		          AND kcu.constraint_schema = tc.constraint_schema
		          AND kcu.constraint_name = tc.constraint_name
		         WHERE tc.table_schema = c.table_schema
		           AND tc.table_name = c.table_name
		           AND tc.constraint_type = 'PRIMARY KEY'
		           AND kcu.column_name = c.column_name
		       )
		FROM information_schema.columns c
		WHERE c.table_schema = current_schema() AND c.table_name = $1
		ORDER BY c.ordinal_position`, name)
	if err != nil {
		return Table{}, err
	}
	table := Table{Name: name}
	for rows.Next() {
		var position int
		var columnName, dataType, databaseType, nullableText, identityText string
		var defaultValue sql.NullString
		var primaryKey bool
		if err := rows.Scan(&position, &columnName, &dataType, &databaseType, &nullableText, &defaultValue, &identityText, &primaryKey); err != nil {
			rows.Close()
			return Table{}, err
		}
		typ, err := postgresqlColumnType(dataType, databaseType)
		if err != nil {
			rows.Close()
			return Table{}, fmt.Errorf("table %s column %s: %w", name, columnName, err)
		}
		nullable := nullableText == "YES" && !primaryKey
		generated := identityText == "YES" || defaultValue.Valid && strings.HasPrefix(strings.ToLower(defaultValue.String), "nextval(")
		typ.Nullable = nullable
		table.Columns = append(table.Columns, Column{
			Name: columnName, DatabaseType: databaseType, Type: typ, Nullable: nullable,
			PrimaryKey: primaryKey, HasDefault: defaultValue.Valid || generated, Generated: generated, Position: position - 1,
		})
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return Table{}, err
	}
	if err := rows.Close(); err != nil {
		return Table{}, err
	}
	foreignKeys, err := inspectPostgreSQLForeignKeys(database, name)
	if err != nil {
		return Table{}, err
	}
	table.ForeignKeys = foreignKeys
	return table, nil
}

func inspectPostgreSQLForeignKeys(database *sql.DB, name string) ([]ForeignKey, error) {
	rows, err := database.Query(`
		SELECT kcu.constraint_name, kcu.ordinal_position, kcu.column_name,
		       ccu.table_name, ccu.column_name
		FROM information_schema.table_constraints tc
		JOIN information_schema.key_column_usage kcu
		  ON kcu.constraint_catalog = tc.constraint_catalog
		 AND kcu.constraint_schema = tc.constraint_schema
		 AND kcu.constraint_name = tc.constraint_name
		JOIN information_schema.referential_constraints rc
		  ON rc.constraint_catalog = tc.constraint_catalog
		 AND rc.constraint_schema = tc.constraint_schema
		 AND rc.constraint_name = tc.constraint_name
		JOIN information_schema.key_column_usage ccu
		  ON ccu.constraint_catalog = rc.unique_constraint_catalog
		 AND ccu.constraint_schema = rc.unique_constraint_schema
		 AND ccu.constraint_name = rc.unique_constraint_name
		 AND ccu.ordinal_position = kcu.position_in_unique_constraint
		WHERE tc.table_schema = current_schema()
		  AND tc.table_name = $1
		  AND tc.constraint_type = 'FOREIGN KEY'
		ORDER BY kcu.constraint_name, kcu.ordinal_position`, name)
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

func postgresqlColumnType(dataType, databaseType string) (types.Type, error) {
	normalized := strings.ToLower(strings.TrimSpace(databaseType))
	switch normalized {
	case "int2", "int4", "int8", "serial2", "serial4", "serial8":
		return types.FromName("Integer"), nil
	case "float4", "float8", "numeric", "decimal":
		return types.FromName("Float"), nil
	case "varchar", "bpchar", "text", "name", "uuid", "json", "jsonb":
		return types.FromName("String"), nil
	case "bool":
		return types.FromName("Boolean"), nil
	case "bytea":
		return types.FromName("Bytes"), nil
	default:
		return types.Type{}, fmt.Errorf("unsupported PostgreSQL column type %q (%s)", databaseType, dataType)
	}
}
