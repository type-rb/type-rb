package orm

import (
	"database/sql"
	"encoding/json"
	"net/url"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/type-rb/type-rb/internal/types"
)

func TestPostgreSQLLiveIntrospection(t *testing.T) {
	databaseURL := os.Getenv("TRB_TEST_POSTGRESQL_DATABASE")
	if databaseURL == "" {
		t.Skip("set TRB_TEST_POSTGRESQL_DATABASE to run the live adapter test")
	}
	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	schemaName := "trb_orm_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	adapter, err := AdapterFor("postgresql")
	if err != nil {
		t.Fatal(err)
	}
	database, err := sql.Open(adapter.DriverName, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("CREATE SCHEMA " + adapter.QuoteIdentifier(schemaName)); err != nil {
		t.Fatal(err)
	}
	defer database.Exec("DROP SCHEMA " + adapter.QuoteIdentifier(schemaName) + " CASCADE")
	query := parsed.Query()
	query.Set("search_path", schemaName)
	parsed.RawQuery = query.Encode()
	testDatabaseURL := parsed.String()
	testDatabase, err := sql.Open(adapter.DriverName, testDatabaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testDatabase.Exec(`
		CREATE TABLE categories (id BIGSERIAL PRIMARY KEY, name TEXT NOT NULL);
		CREATE TABLE products (
			id BIGSERIAL PRIMARY KEY,
			category_id BIGINT NOT NULL REFERENCES categories(id),
			name TEXT NOT NULL,
			price DOUBLE PRECISION,
			active BOOLEAN NOT NULL,
			payload BYTEA
		)`); err != nil {
		testDatabase.Close()
		t.Fatal(err)
	}
	if err := testDatabase.Close(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(Config{Adapter: "postgresql", Database: testDatabaseURL})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := LoadSchema(t.TempDir(), map[string][]byte{PackageName: encoded})
	if err != nil {
		t.Fatal(err)
	}
	products, ok := schema.Table("products")
	if !ok || len(products.Columns) != 6 {
		t.Fatalf("unexpected PostgreSQL products table: %#v", products)
	}
	assertColumn(t, products.Columns[0], "id", types.Int, false, true)
	assertColumn(t, products.Columns[3], "price", types.Float, true, false)
	assertColumn(t, products.Columns[4], "active", types.Bool, false, false)
	assertColumn(t, products.Columns[5], "payload", types.Bytes, true, false)
	if len(products.ForeignKeys) != 1 || products.ForeignKeys[0].Column != "category_id" || products.ForeignKeys[0].ReferencedTable != "categories" || products.ForeignKeys[0].ReferencedColumn != "id" {
		t.Fatalf("unexpected PostgreSQL foreign keys: %#v", products.ForeignKeys)
	}
}
