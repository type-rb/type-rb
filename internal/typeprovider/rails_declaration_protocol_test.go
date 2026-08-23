package typeprovider

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/packageextensionhost"
	railsprovider "github.com/type-rb/type-rb/internal/typeprovider/rails"
)

func TestRailsDeclarationsCrossVersionedExtensionBoundary(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0o755); err != nil {
		t.Fatal(err)
	}
	const schema = `ActiveRecord::Schema[8.1].define do
  create_table "insurers", force: :cascade do |t|
    t.string "code", null: false
  end
end
`
	if err := os.WriteFile(filepath.Join(root, "db", "schema.rb"), []byte(schema), 0o644); err != nil {
		t.Fatal(err)
	}

	context := Context{ProjectRoot: root}
	provided, err := loadRailsDeclarations(context)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(provided)
	if err != nil {
		t.Fatal(err)
	}
	var decoded packageextension.DeclarationCatalog
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := packageextension.ValidateDeclarationCatalog(decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Provider != railsTypeProvider {
		t.Fatalf("provider=%q, want %q", decoded.Provider, railsTypeProvider)
	}

	insurer := declaredProtocolType(t, decoded, "Insurer")
	all := declaredProtocolMember(t, insurer.ClassMembers, "all")
	if got := protocolTypeString(all.Return); got != "ActiveRecord::Relation<Insurer>" {
		t.Fatalf("Insurer.all return=%s", got)
	}
	for _, name := range []string{"ApplicationController", "Api::ApplicationController", "Pagination", "PaginationHelper"} {
		if declarationCatalogContainsName(decoded, name) {
			t.Fatalf("Rails declaration protocol exposed application-owned %s", name)
		}
	}

	imported, err := packageextensionhost.ImportDeclarationCatalog(decoded)
	if err != nil {
		t.Fatal(err)
	}
	original, err := railsprovider.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, original) {
		t.Fatal("declaration protocol changed the Rails catalog semantics")
	}
	loaded, err := loadRails(nil, context)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(imported, loaded) {
		t.Fatal("Rails provider did not load through the declaration protocol")
	}
}

func declarationCatalogContainsName(catalog packageextension.DeclarationCatalog, name string) bool {
	for _, declared := range catalog.Types {
		if declared.Name == name {
			return true
		}
	}
	for _, declared := range catalog.Modules {
		if declared.Name == name {
			return true
		}
	}
	return false
}
