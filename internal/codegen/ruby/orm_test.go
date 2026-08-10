package ruby

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func TestORMRuntimeKeepsSequelBehindTypeRBOwnedQueryBoundary(t *testing.T) {
	for _, adapter := range []string{"sqlite", "postgresql", "mysql"} {
		t.Run(adapter, func(t *testing.T) {
			manifest := rubyORMTestManifest(adapter)
			pool := Generate(&ir.Program{
				Mode: "ruby", Package: "orm", ModulePath: "trb/orm/index", RubyLoader: "require_relative",
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
			})
			for _, expected := range []string{
				`require "sequel"`, "class Query", "def select_sql", "def render_predicate",
				"def association_query", "def destroy_model_result", "def transaction_result",
				`TrbOrmRuntime.configure(adapter: "` + adapter + `"`,
			} {
				if !strings.Contains(pool, expected) {
					t.Fatalf("generated %s Ruby ORM pool is missing %q:\n%s", adapter, expected, pool)
				}
			}
			if strings.Contains(pool, "Sequel::Model") || strings.Contains(pool, ".where(") && strings.Contains(pool, "database[") {
				t.Fatalf("generated runtime exposes Sequel ORM semantics instead of the TypeRB query boundary:\n%s", pool)
			}
			assertRubySyntax(t, pool)

			modelProgram := &ir.Program{
				Mode: "ruby", Package: "main", ModulePath: "main", RubyLoader: "require_relative",
				Extensions: []ir.Extension{manifest}, Statements: []ir.Statement{
					&ir.Import{Path: "trb/orm/index", Official: true, Runtime: true},
					&ir.Class{Name: "Product"},
				},
			}
			manifest.Augment(modelProgram)
			model := Generate(modelProgram)
			for _, expected := range []string{
				"TrbOrmRuntime.register_model(", `table: "products"`, `name: "category"`,
				`unique_constraints: [["id"], ["name"]]`,
			} {
				if !strings.Contains(model, expected) {
					t.Fatalf("generated %s Ruby model runtime is missing %q:\n%s", adapter, expected, model)
				}
			}
			assertRubySyntax(t, model)
		})
	}
}

func rubyORMTestManifest(adapter string) *ormintegration.Manifest {
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

func assertRubySyntax(t *testing.T, source string) {
	t.Helper()
	if _, err := exec.LookPath("ruby"); err != nil {
		t.Skip("ruby is not installed")
	}
	command := exec.Command("ruby", "-c")
	command.Stdin = strings.NewReader(source)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("generated invalid Ruby: %v\n%s\n%s", err, output, source)
	}
}
