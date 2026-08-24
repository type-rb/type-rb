package golang

import (
	"go/parser"
	"go/token"
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
				`mark := "\""`, `database.Query("EXPLAIN "+statement`, `statement+" RETURNING \"id\""`,
				` ON CONFLICT (`, `excluded.`,
			},
		},
		{
			adapter: "mysql",
			driver:  `trbmysql "github.com/go-sql-driver/mysql"`,
			want: []string{
				`return "?"`, `mark := "` + "`" + `"`,
				`column := trbOrmQuoteIdentifier(condition.column)`,
				`column := trbOrmQuoteIdentifier(order.column)`,
				`database.Query("EXPLAIN FORMAT=JSON "+statement`,
				`written.LastInsertId()`,
				`LIMIT 1 FOR UPDATE`, `transaction.Commit()`,
			},
		},
	} {
		t.Run(test.adapter, func(t *testing.T) {
			manifest := &ormintegration.Manifest{
				Adapter: test.adapter, Database: "database",
				Models: []ormintegration.Model{{
					Name: "Product", QueryType: "ProductQuery", Table: "products", ModulePath: "main",
					Columns: []ormintegration.Column{{Name: "id", Type: types.FromName("Integer"), PrimaryKey: true, HasDefault: true, Generated: true}},
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
			if !strings.Contains(output, `import "database/sql"`) {
				t.Fatalf("generated %s ORM runtime does not import database/sql:\n%s", test.adapter, output)
			}
			for _, want := range test.want {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s ORM runtime is missing %q:\n%s", test.adapter, want, output)
				}
			}
			if !strings.Contains(output, `database Integer is outside the portable range`) {
				t.Fatalf("generated %s ORM runtime does not validate portable Integer ingress:\n%s", test.adapter, output)
			}
			if !strings.Contains(output, `statement += " FOR UPDATE"`) {
				t.Fatalf("generated %s ORM runtime does not append the row lock clause:\n%s", test.adapter, output)
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "main.go", output, parser.AllErrors); err != nil {
				t.Fatalf("generated invalid %s ORM Go: %v\n%s", test.adapter, err, output)
			}
			for _, want := range []string{
				`"example.com/orm/trb/orm"`, `orm.DbResult[[]*Product]`, `orm.NewDbResultErr[[]*Product]`,
				`trbOrmError(err, orm.DbErrorKindQuery, "database query failed")`, `database, err := orm.TrbOrmDatabase()`,
				`type ProductDraft struct {`,
				`func TrbOrmBuildProduct(columns []string, values []any) *ProductDraft`,
				`func TrbOrmSaveProductDraft(draft *ProductDraft) orm.DbResult[*Product]`,
				`return TrbOrmProductCreateScoped(draft.query, draft.columns, draft.values)`,
				`func TrbOrmCreateProduct(columns []string, values []any) orm.DbResult[*Product]`,
				`func TrbOrmInsertAllProduct(drafts []*ProductDraft) orm.DbResult[int]`,
				`bulk insert drafts must use the same attributes`,
				`written, err := database.Exec(statement, arguments...)`,
				`func TrbOrmInsertProductIfAbsent(draft *ProductDraft, uniqueColumns []string) orm.DbResult[bool]`,
				`func TrbOrmUpsertProduct(draft *ProductDraft, uniqueColumns []string, updateColumns []string) orm.DbResult[*Product]`,
				`func TrbOrmUpsertAllProduct(drafts []*ProductDraft, uniqueColumns []string, updateColumns []string) orm.DbResult[int]`,
				`unique_by must match a primary or unique constraint`,
				`type ProductChanges struct {`,
				`func TrbOrmWithProduct(value *Product, columns []string, values []any) *ProductChanges`,
				`func TrbOrmSaveProductChanges(changes *ProductChanges) orm.DbResult[*Product]`,
				`return TrbOrmUpdateProduct(changes.value, changes.columns, changes.values)`,
				`func TrbOrmUpdateProduct(value *Product, columns []string, values []any) orm.DbResult[*Product]`,
				`func TrbOrmDeleteProduct(value *Product) orm.DbResult[bool]`,
				`func TrbOrmSelectProductId(query TrbOrmProductQuery) *orm.TrbOrmSubquery[int]`,
				`func TrbOrmProductDistinct(query TrbOrmProductQuery) TrbOrmProductQuery`,
				`prefix += "DISTINCT "`,
				`func trbOrmProductStatementAppend(query TrbOrmProductQuery, projection string, arguments *[]any) string`,
				`func TrbOrmProductWhereExists(query TrbOrmProductQuery`,
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
				"func TrbOrmBeginNestedTransaction(parent *TrbOrmTransaction)", `"SAVEPOINT " + savepoint`,
				"type TrbOrmSubquery[T any] struct", "type TrbOrmSubqueryValue interface", "func NewTrbOrmSubquery[T any]",
				"type TrbOrmAssociationPredicate func", "type TrbOrmExistsPredicate func",
			} {
				if !strings.Contains(pool, want) {
					t.Fatalf("generated %s ORM pool runtime is missing %q:\n%s", test.adapter, want, pool)
				}
			}
			if _, err := parser.ParseFile(token.NewFileSet(), "orm.go", pool, parser.AllErrors); err != nil {
				t.Fatalf("generated invalid %s ORM pool Go: %v\n%s", test.adapter, err, pool)
			}
			if test.adapter == "mysql" {
				for _, want := range []string{
					`"net/url"`, `strings.HasPrefix(trbOrmSource, "mysql://")`, `url.Parse(trbOrmSource)`,
					`trbmysql.ParseDSN(trbOrmSource)`, `trbMySQLConfig.Params["allowPublicKeyRetrieval"]`,
					`delete(trbMySQLConfig.Params, "allowPublicKeyRetrieval")`, `trbOrmSource = trbMySQLConfig.FormatDSN()`,
					`errors.New("MySQL allowPublicKeyRetrieval must be true or false")`,
				} {
					if !strings.Contains(pool, want) {
						t.Fatalf("generated MySQL ORM pool is missing %q:\n%s", want, pool)
					}
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
		`os.LookupEnv("DATABASE_URL")`, `sql.Open("pgx", trbOrmSource)`,
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

func TestMySQLORMCreateUsesApplicationPrimaryKeyWhenNotGenerated(t *testing.T) {
	manifest := &ormintegration.Manifest{
		Adapter: "mysql", Database: "database",
		Models: []ormintegration.Model{{
			Name: "Product", QueryType: "ProductQuery", Table: "products", ModulePath: "main",
			Columns: []ormintegration.Column{{Name: "id", Type: types.FromName("String"), PrimaryKey: true}},
		}},
	}
	program := &ir.Program{
		Mode: "go", Package: "main", ModulePath: "main", GoModule: "example.com/orm",
		Extensions: []ir.Extension{manifest}, Statements: []ir.Statement{
			&ir.Import{Path: "trb/orm/index", Official: true, Runtime: true},
			&ir.Class{Name: "Product"},
		},
	}
	manifest.Augment(program)
	output := Generate(program)
	for _, want := range []string{
		`_, err := database.Exec(statement, values...)`, `if column == "id" {`, `primaryKeyValue = values[index]`,
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("generated MySQL application-key create is missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "written.LastInsertId") {
		t.Fatalf("generated MySQL application-key create requests a generated key:\n%s", output)
	}
	if strings.Contains(output, `_, err = database.Exec(statement, values...)`) {
		t.Fatalf("generated MySQL application-key create assigns to an undeclared error variable:\n%s", output)
	}
}
