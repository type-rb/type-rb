package rails

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDerivesActiveRecordModelsFromSchema(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	schema := `ActiveRecord::Schema[8.1].define do
  create_table "insurers", force: :cascade do |t|
    t.string "code", null: false
    t.string "name", null: false
    t.boolean "enabled", null: false
    t.datetime "created_at", null: false
  end
end
`
	if err := os.WriteFile(filepath.Join(root, "db", "schema.rb"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}

	catalog, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	insurer, ok := catalog.Type("Insurer")
	if !ok {
		t.Fatal("schema did not produce Insurer declaration")
	}
	for name, want := range map[string]string{"id": "Integer", "code": "String", "name": "String", "enabled": "Boolean", "created_at": "DateTime"} {
		member, exists := insurer.InstanceMembers[name]
		if !exists || member.Return.String() != want {
			t.Fatalf("column %s: got %#v, want %s", name, member, want)
		}
	}
	all := insurer.ClassMembers["all"]
	if got := all.Return.String(); got != "ActiveRecord::Relation<Insurer>" {
		t.Fatalf("all() return = %s", got)
	}
	find := insurer.ClassMembers["find_by!"]
	if got := find.Return.String(); got != "Insurer" {
		t.Fatalf("find_by!() return = %s", got)
	}
}
