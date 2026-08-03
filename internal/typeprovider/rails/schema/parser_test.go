package schema

import "testing"

func TestParseBuildsSchemaAST(t *testing.T) {
	source := []byte(`ActiveRecord::Schema[8.1].define(version: 1) do
  create_table "insurers", id: false, force: :cascade do |t|
    t.string "code", null: false
    t.column "status", "enum('ACTIVE','INACTIVE')", null: false
    t.timestamps null: false
    t.index ["code"], unique: true
  end
end
`)
	parsed, err := Parse(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.Tables) != 1 {
		t.Fatalf("tables=%#v", parsed.Tables)
	}
	table := parsed.Tables[0]
	if table.Name != "insurers" || table.ID {
		t.Fatalf("unexpected table: %#v", table)
	}
	if len(table.Columns) != 4 {
		t.Fatalf("columns=%#v", table.Columns)
	}
	for index, want := range []struct{ name, databaseType string }{{"code", "string"}, {"status", "enum('ACTIVE','INACTIVE')"}, {"created_at", "datetime"}, {"updated_at", "datetime"}} {
		column := table.Columns[index]
		if column.Name != want.name || column.DatabaseType != want.databaseType || column.Nullable {
			t.Fatalf("column %d = %#v, want %#v", index, column, want)
		}
	}
}
