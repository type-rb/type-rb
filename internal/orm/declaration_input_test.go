package orm

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

func TestORMDeclarationInputIsVersionedSerializableAndDataOnly(t *testing.T) {
	project := packageextension.ProjectDeclarationInput{
		ProtocolVersion: packageextension.ProjectDeclarationInputProtocolVersion,
		Provider:        PackageName,
		Modules:         []packageextension.ProjectModule{{ModulePath: "models/product"}},
	}
	schema := &Schema{
		Adapter: "sqlite", Database: "/private/application.sqlite3", DatabaseEnvironment: "DATABASE_URL",
		Tables: []Table{{
			Name: "products",
			Columns: []Column{
				{Name: "id", Type: types.FromName("Integer"), PrimaryKey: true, Generated: true},
				{Name: "name", Type: types.FromName("String"), HasDefault: true},
			},
			ForeignKeys:       []ForeignKey{{Column: "owner_id", ReferencedTable: "owners", ReferencedColumn: "id"}},
			UniqueConstraints: []UniqueConstraint{{Columns: []string{"name"}}},
		}},
	}
	input, err := ExportDeclarationInput(project, schema)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), schema.Database) || strings.Contains(string(encoded), schema.DatabaseEnvironment) {
		t.Fatalf("ORM declaration input exposed database connection details: %s", encoded)
	}
	var decoded DeclarationInput
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDeclarationInput(decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, input) {
		t.Fatalf("ORM declaration input changed across its JSON boundary:\ninput: %#v\ndecoded: %#v", input, decoded)
	}
	imported := importDeclarationSchema(decoded.Schema)
	if imported.Database != "" || imported.DatabaseEnvironment != "" || imported.Tables[0].Columns[0].Type.Kind != types.Int {
		t.Fatalf("unexpected imported declaration schema: %#v", imported)
	}
}

func TestORMDeclarationInputRejectsInvalidBoundaryData(t *testing.T) {
	valid := func() DeclarationInput {
		return DeclarationInput{
			ProtocolVersion: DeclarationInputProtocolVersion,
			Project: packageextension.ProjectDeclarationInput{
				ProtocolVersion: packageextension.ProjectDeclarationInputProtocolVersion,
				Provider:        PackageName,
			},
			Schema: DeclarationSchema{Adapter: "sqlite"},
		}
	}
	tests := []struct {
		name    string
		mutate  func(*DeclarationInput)
		message string
	}{
		{name: "version", mutate: func(input *DeclarationInput) { input.ProtocolVersion++ }, message: "unsupported trb/orm declaration input protocol version"},
		{name: "provider", mutate: func(input *DeclarationInput) { input.Project.Provider = "trb/jobs" }, message: "received project declaration input"},
		{name: "adapter", mutate: func(input *DeclarationInput) { input.Schema.Adapter = "oracle" }, message: "unsupported trb/orm adapter"},
		{name: "duplicate table", mutate: func(input *DeclarationInput) {
			input.Schema.Tables = []DeclarationTable{{Name: "products"}, {Name: "products"}}
		}, message: "empty or duplicate table"},
		{name: "column type", mutate: func(input *DeclarationInput) {
			input.Schema.Tables = []DeclarationTable{{Name: "products", Columns: []DeclarationColumn{{Name: "id", Type: packageextension.Type{Kind: "mystery"}}}}}
		}, message: "unsupported type kind"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := valid()
			test.mutate(&input)
			err := ValidateDeclarationInput(input)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("expected %q, got %v", test.message, err)
			}
		})
	}
}
