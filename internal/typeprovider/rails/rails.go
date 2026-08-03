// Package rails supplies TypeRB declarations for Rails and derives
// ActiveRecord model types from db/schema.rb. No application-maintained shadow
// signatures are required.
package rails

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/typeprovider/rails/schema"
	"github.com/type-rb/type-rb/internal/types"
)

func Load(projectRoot string) (*declaration.Catalog, error) {
	catalog := builtins()
	if projectRoot == "" {
		return catalog, nil
	}
	path := filepath.Join(projectRoot, "db", "schema.rb")
	source, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return catalog, nil
	}
	if err != nil {
		return nil, err
	}
	parsed, err := schema.Parse(source)
	if err != nil {
		return nil, err
	}
	for _, table := range parsed.Tables {
		model := modelDeclaration(modelName(table.Name))
		if table.ID {
			model.InstanceMembers["id"] = property("id", types.FromName("Integer"))
		}
		for _, column := range table.Columns {
			addColumn(model, column.Name, column.DatabaseType, column.Nullable)
		}
		finishModel(catalog, model)
	}
	return catalog, nil
}

func builtins() *declaration.Catalog {
	catalog := declaration.NewCatalog()
	any := types.FromName("Any")
	void := types.FromName("Void")
	stringType := types.FromName("String")

	controller := declaration.NewType("ActionController::API", "")
	controller.InstanceMembers["params"] = property("params", types.FromName("ActionController::Parameters"))
	controller.InstanceMembers["render"] = declaration.Member{
		Name: "render", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "value", Type: any, Optional: true}}, Return: void, Variadic: true, Provider: "rails",
	}
	catalog.Types[controller.Name] = controller
	for _, name := range []string{"ApplicationController", "Api::ApplicationController"} {
		catalog.Types[name] = declaration.NewType(name, controller.Name)
	}

	parameters := declaration.NewType("ActionController::Parameters", "")
	parameters.InstanceMembers["[]"] = declaration.Member{Name: "[]", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "key", Type: stringType}}, Return: stringType, Provider: "rails"}
	catalog.Types[parameters.Name] = parameters

	activeRecord := declaration.NewType("ActiveRecord::Base", "")
	activeRecord.InstanceMembers["as_json"] = declaration.Member{Name: "as_json", Kind: declaration.Method, Return: types.FromName("JSON::Value"), Provider: "rails"}
	catalog.Types[activeRecord.Name] = activeRecord
	catalog.Types["ActiveRecord::Relation"] = declaration.NewType("ActiveRecord::Relation", "")
	catalog.Types["Pagination"] = declaration.NewType("Pagination", "")
	catalog.Types["JSON::Value"] = declaration.NewType("JSON::Value", "")

	pagination := declaration.NewModule("PaginationHelper")
	typeVariable := types.FromName("T")
	relation := types.FromName("ActiveRecord::Relation")
	relation.Args = []types.Type{typeVariable}
	array := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{typeVariable}}
	tuple := types.FromName("Tuple")
	tuple.Args = []types.Type{array, types.FromName("Pagination")}
	pagination.InstanceMembers["paginate_with_headers"] = declaration.Member{
		Name: "paginate_with_headers", Kind: declaration.Method, Parameters: []declaration.Parameter{{Name: "relation", Type: relation}}, Return: tuple, TypeParameters: []string{"T"}, Provider: "rails",
	}
	catalog.Modules[pagination.Name] = pagination
	return catalog
}

func modelDeclaration(name string) *declaration.Type {
	model := declaration.NewType(name, "ActiveRecord::Base")
	model.ClassMembers["all"] = declaration.Member{Name: "all", Kind: declaration.Method, Return: relationOf(name), Class: true, Provider: "rails"}
	return model
}

func finishModel(catalog *declaration.Catalog, model *declaration.Type) {
	if model == nil {
		return
	}
	parameters := make([]declaration.Parameter, 0, len(model.InstanceMembers))
	names := make([]string, 0, len(model.InstanceMembers))
	for name := range model.InstanceMembers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		member := model.InstanceMembers[name]
		parameters = append(parameters, declaration.Parameter{Name: name, Type: member.Return, Keyword: true, Optional: true})
	}
	model.ClassMembers["find_by!"] = declaration.Member{Name: "find_by!", Kind: declaration.Method, Parameters: parameters, Return: types.FromName(model.Name), Class: true, Provider: "rails"}
	catalog.Types[model.Name] = model
}

func addColumn(model *declaration.Type, name, databaseType string, nullable bool) {
	if model == nil {
		return
	}
	typ := columnType(databaseType)
	if nullable {
		typ.Nullable = true
	}
	model.InstanceMembers[name] = property(name, typ)
}

func property(name string, typ types.Type) declaration.Member {
	return declaration.Member{Name: name, Kind: declaration.Property, Return: typ, Provider: "rails"}
}

func relationOf(model string) types.Type {
	result := types.FromName("ActiveRecord::Relation")
	result.Args = []types.Type{types.FromName(model)}
	return result
}

func columnType(name string) types.Type {
	name = strings.ToLower(name)
	switch {
	case strings.Contains(name, "int"):
		return types.FromName("Integer")
	case name == "float" || name == "decimal" || name == "numeric":
		return types.FromName("Float")
	case name == "boolean":
		return types.FromName("Boolean")
	case name == "json" || name == "jsonb":
		return types.FromName("JSON::Value")
	case name == "date", name == "datetime", name == "timestamp", name == "time":
		return types.FromName("DateTime")
	default:
		return types.FromName("String")
	}
}

func modelName(table string) string {
	word := table
	switch {
	case strings.HasSuffix(word, "ies"):
		word = strings.TrimSuffix(word, "ies") + "y"
	case strings.HasSuffix(word, "sses"):
		word = strings.TrimSuffix(word, "es")
	case strings.HasSuffix(word, "s"):
		word = strings.TrimSuffix(word, "s")
	}
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '_' || r == '-' })
	for i, part := range parts {
		runes := []rune(part)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			parts[i] = string(runes)
		}
	}
	return strings.Join(parts, "")
}
