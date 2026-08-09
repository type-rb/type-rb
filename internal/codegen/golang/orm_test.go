package golang

import (
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func TestORMRuntimeUsesSelectedDatabaseDialect(t *testing.T) {
	for _, test := range []struct {
		adapter string
		driver  string
		want    []string
	}{
		{
			adapter: "postgresql",
			driver:  `_ "github.com/jackc/pgx/v5/stdlib"`,
			want: []string{
				`return "$" + strconv.Itoa(position)`,
				`mark := "\""`, `database.Query("EXPLAIN "+statement`,
			},
		},
		{
			adapter: "mysql",
			driver:  `_ "github.com/go-sql-driver/mysql"`,
			want: []string{
				`return "?"`, `mark := "` + "`" + `"`,
				`column := trbOrmQuoteIdentifier(condition.column)`,
				`column := trbOrmQuoteIdentifier(order.column)`,
				`database.Query("EXPLAIN FORMAT=JSON "+statement`,
			},
		},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			manifest := &ormintegration.Manifest{
				Adapter: test.adapter, Database: "database",
				Models: []ormintegration.Model{{
					Name: "Product", QueryType: "ProductQuery", Table: "products", ModulePath: "main",
					Columns: []ormintegration.Column{{Name: "id", Type: types.FromName("Integer"), PrimaryKey: true}},
				}},
			}
			program := &ir.Program{
				Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/orm",
				Extensions: []ir.Extension{manifest}, Statements: []ir.Statement{
					&ir.Import{
						Path: "trb/orm/index", Official: true, Runtime: true,
						SymbolKinds: map[string]string{"DbError": "record", "DbErrorKind": "enum", "DbResult": "enum_alias"},
					},
					&ir.Class{Name: "Product"},
				},
			}
			manifest.Augment(program)
			output := Generate(program)
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s ORM runtime is missing %q:\n%s", test.adapter, want, output)
				}
			}
			for _, want := range []string{
				`"example.com/orm/trb/orm"`, `orm.DbResult[[]*Product]`, `orm.NewDbResultErr[[]*Product]`,
				`trbOrmError(err, orm.DbErrorKindQuery, "database query failed")`, `database, err := orm.TrbOrmDatabase()`,
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s ORM Result runtime is missing %q:\n%s", test.adapter, want, output)
				}
			}
			if strings.Contains(output, "panic(err)") {
				t.Fatalf("generated %s ORM runtime still exposes database errors through panic:\n%s", test.adapter, output)
			}
			pool := Generate(&ir.Program{
				Mode: "go", Package: "orm", ModulePath: "trb/orm/index", GoModule: "example.com/orm",
				Extensions: []ir.Extension{manifest},
			})
			for _, want := range []string{
				test.driver, "var trbOrmDatabaseOnce sync.Once", "func TrbOrmDatabase() (*sql.DB, error)",
				`sql.Open(`, "trbOrmDatabase.Ping()", "func TrbOrmCloseDatabase() error",
			} {
				if !strings.Contains(pool, want) {
					t.Fatalf("generated %s ORM pool runtime is missing %q:\n%s", test.adapter, want, pool)
				}
			}
		})
	}
}

func TestORMPoolResolvesRuntimeDatabaseFromEnvironment(t *testing.T) {
	manifest := &ormintegration.Manifest{
		Adapter: "postgresql", Database: "compile-time-secret", DatabaseEnvironment: "DATABASE_URL",
	}
	output := Generate(&ir.Program{
		Mode: "go", Package: "orm", ModulePath: "trb/orm/index", GoModule: "example.com/orm",
		Extensions: []ir.Extension{manifest},
	})
	for _, want := range []string{
		`os.LookupEnv("DATABASE_URL")`, `sql.Open("pgx", databaseSource)`,
		`errors.New("database environment variable is not set or empty")`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated ORM environment pool is missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "compile-time-secret") {
		t.Fatalf("generated ORM pool exposes the compile-time database value:\n%s", output)
	}
}
