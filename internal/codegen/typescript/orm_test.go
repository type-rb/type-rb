package typescript

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func TestORMRuntimeKeepsBunSQLBehindTypeRBOwnedQueryBoundary(t *testing.T) {
	for _, adapter := range []string{"sqlite", "postgresql", "mysql"} {
		t.Run(adapter, func(t *testing.T) {
			manifest := typeScriptORMTestManifest(adapter)
			program := &ir.Program{
				Mode: "typescript", Package: "orm", ModulePath: "trb/orm/index", TypeScriptRuntime: "bun",
				Extensions: []ir.Extension{manifest}, Statements: []ir.Statement{
					&ir.Enum{Name: "DbErrorKind", Body: []ir.Statement{
						&ir.EnumMember{Name: "Connection"}, &ir.EnumMember{Name: "Constraint"},
						&ir.EnumMember{Name: "InvalidData"}, &ir.EnumMember{Name: "Query"},
						&ir.EnumMember{Name: "Timeout"}, &ir.EnumMember{Name: "Unknown"},
					}},
					&ir.Record{Name: "DbError", Body: []ir.Statement{
						&ir.RecordField{Name: "kind", Type: types.FromName("DbErrorKind")},
						&ir.RecordField{Name: "message", Type: types.FromName("String")},
					}},
				},
			}
			generated, err := GenerateProject([]*ir.Program{program})
			if err != nil {
				t.Fatal(err)
			}
			pool := generated[0]
			for _, expected := range []string{
				`import { SQL, type ReservedSQL, type TransactionSQL } from "bun";`, "type TrbOrmQuery =",
				"function predicateSQL", "function associationQuery", "async function destroyInTransaction",
				"export async function transaction", `database().begin("immediate", run)`, "result.affectedRows ?? result.count",
				`const __trbOrmAdapter: TrbOrmAdapter = "` + adapter + `";`,
			} {
				if !strings.Contains(pool, expected) {
					t.Fatalf("generated %s TypeScript ORM pool is missing %q:\n%s", adapter, expected, pool)
				}
			}
			if adapter == "mysql" {
				for _, expected := range []string{
					"reserved = await database().reserve", `await reserved.unsafe("SET SESSION time_zone = '+00:00'", [])`,
					`parent === null) await client.unsafe("SET SESSION time_zone = '+00:00'", [])`, "reserved?.release()",
				} {
					if !strings.Contains(pool, expected) {
						t.Fatalf("generated MySQL TypeScript ORM runtime is missing UTC session setup %q:\n%s", expected, pool)
					}
				}
			}
			assertTypeScriptSyntax(t, pool)

			modelProgram := &ir.Program{
				Mode: "typescript", Package: "main", ModulePath: "main", TypeScriptRuntime: "bun",
				Extensions: []ir.Extension{manifest}, Statements: []ir.Statement{
					&ir.Import{Path: "trb/orm/index", Official: true, Runtime: true},
					&ir.Class{Name: "Product"},
				},
			}
			manifest.Augment(modelProgram)
			generated, err = GenerateProject([]*ir.Program{modelProgram})
			if err != nil {
				t.Fatal(err)
			}
			model := generated[0]
			for _, expected := range []string{
				"type Subquery<T> = __trbOrm.TrbOrmSubquery<T>;", "__trbOrm.registerModel(",
				`table: "products"`, `name: "category"`, `unique: [["id"], ["name"]]`,
			} {
				if !strings.Contains(model, expected) {
					t.Fatalf("generated %s TypeScript model runtime is missing %q:\n%s", adapter, expected, model)
				}
			}
			if strings.Contains(model, `from "bun"`) || strings.Contains(model, "new SQL") {
				t.Fatalf("generated model module leaks the private Bun execution adapter:\n%s", model)
			}
			assertTypeScriptSyntax(t, model)
		})
	}
}

func typeScriptORMTestManifest(adapter string) *ormintegration.Manifest {
	return &ormintegration.Manifest{
		Adapter: adapter, Database: "database",
		Models: []ormintegration.Model{
			{
				Name: "Product", QueryType: "ProductQuery", Table: "products", ModulePath: "main",
				Columns: []ormintegration.Column{
					{Name: "id", Type: types.FromName("Integer"), PrimaryKey: true, Generated: true},
					{Name: "name", Type: types.FromName("String")},
					{Name: "category_id", Type: types.FromName("Integer"), Nullable: true},
				},
				UniqueConstraints: []ormintegration.UniqueConstraint{
					{Name: "products_pkey", Columns: []string{"id"}, Primary: true},
					{Name: "products_name_key", Columns: []string{"name"}},
				},
				Associations: []ormintegration.Association{
					{Name: "category", Kind: ormintegration.BelongsTo, TargetModel: "Category", TargetQuery: "CategoryQuery", SourceColumn: "category_id", TargetColumn: "id", Preloadable: true},
				},
			},
			{
				Name: "Category", QueryType: "CategoryQuery", Table: "categories", ModulePath: "models/category",
				Columns: []ormintegration.Column{{Name: "id", Type: types.FromName("Integer"), PrimaryKey: true, Generated: true}},
			},
		},
	}
}

func assertTypeScriptSyntax(t *testing.T, source string) {
	t.Helper()
	if _, err := exec.LookPath("bun"); err != nil {
		t.Skip("Bun is not installed")
	}
	command := exec.Command("bun", "-e", `new Bun.Transpiler({ loader: "ts" }).transformSync(await Bun.stdin.text())`)
	command.Stdin = strings.NewReader(source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated invalid TypeScript: %v\n%s\n%s", err, output, source)
	}
}
