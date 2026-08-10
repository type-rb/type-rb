package orm

import (
	"database/sql"
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/type-rb/type-rb/internal/types"
)

func TestMySQLLiveIntrospection(t *testing.T) {
	databaseDSN := os.Getenv("TRB_TEST_MYSQL_DATABASE")
	if databaseDSN == "" {
		t.Skip("set TRB_TEST_MYSQL_DATABASE to run the live adapter test")
	}
	baseConfig, err := mysqldriver.ParseDSN(databaseDSN)
	if err != nil {
		t.Fatal(err)
	}
	databaseName := "trb_orm_test_" + strconv.FormatInt(time.Now().UnixNano(), 10)
	adapter, err := AdapterFor("mysql")
	if err != nil {
		t.Fatal(err)
	}
	adminConfig := *baseConfig
	adminConfig.DBName = ""
	admin, err := sql.Open(adapter.DriverName, adminConfig.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err := admin.Exec("CREATE DATABASE " + adapter.QuoteIdentifier(databaseName)); err != nil {
		t.Fatal(err)
	}
	defer admin.Exec("DROP DATABASE " + adapter.QuoteIdentifier(databaseName))
	testConfig := *baseConfig
	testConfig.DBName = databaseName
	testDSN := testConfig.FormatDSN()
	database, err := sql.Open(adapter.DriverName, testDSN)
	if err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`CREATE TABLE categories (id BIGINT AUTO_INCREMENT PRIMARY KEY, name TEXT NOT NULL)`,
		`CREATE TABLE products (
			id BIGINT AUTO_INCREMENT PRIMARY KEY,
			category_id BIGINT NOT NULL,
			name VARCHAR(255) NOT NULL,
			price DOUBLE,
			active BOOLEAN NOT NULL,
			payload BLOB,
			CONSTRAINT products_category_fk FOREIGN KEY (category_id) REFERENCES categories(id)
		)`,
	} {
		if _, err := database.Exec(statement); err != nil {
			database.Close()
			t.Fatal(err)
		}
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(Config{Adapter: "mysql", Database: testDSN})
	if err != nil {
		t.Fatal(err)
	}
	schema, err := LoadSchema(t.TempDir(), map[string][]byte{PackageName: encoded})
	if err != nil {
		t.Fatal(err)
	}
	products, ok := schema.Table("products")
	if !ok || len(products.Columns) != 6 {
		t.Fatalf("unexpected MySQL products table: %#v", products)
	}
	assertColumn(t, products.Columns[0], "id", types.Int, false, true)
	assertColumn(t, products.Columns[3], "price", types.Float, true, false)
	assertColumn(t, products.Columns[4], "active", types.Bool, false, false)
	assertColumn(t, products.Columns[5], "payload", types.Bytes, true, false)
	if len(products.ForeignKeys) != 1 || products.ForeignKeys[0].Column != "category_id" || products.ForeignKeys[0].ReferencedTable != "categories" || products.ForeignKeys[0].ReferencedColumn != "id" {
		t.Fatalf("unexpected MySQL foreign keys: %#v", products.ForeignKeys)
	}
}
