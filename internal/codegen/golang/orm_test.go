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
		want    []string
	}{
		{
			adapter: "postgresql",
			want: []string{
				`_ "github.com/jackc/pgx/v5/stdlib"`, `return "$" + strconv.Itoa(position)`,
				`mark := "\""`, `database.Query("EXPLAIN "+statement`,
			},
		},
		{
			adapter: "mysql",
			want: []string{
				`_ "github.com/go-sql-driver/mysql"`, `return "?"`, `mark := "` + "`" + `"`,
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
				`trbOrmError(err, orm.DbErrorKindQuery, "database query failed")`,
			} {
				if !strings.Contains(output, want) {
					t.Fatalf("generated %s ORM Result runtime is missing %q:\n%s", test.adapter, want, output)
				}
			}
			if strings.Contains(output, "panic(err)") {
				t.Fatalf("generated %s ORM runtime still exposes database errors through panic:\n%s", test.adapter, output)
			}
		})
	}
}
