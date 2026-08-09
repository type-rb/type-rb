package orm

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/types"
)

func Declarations(programs []*ast.Program, projectRoot string, options map[string][]byte) (*declaration.Catalog, error) {
	schema, err := LoadSchema(projectRoot, options)
	if err != nil {
		return nil, err
	}
	models, err := discoverModels(programs, schema)
	if err != nil {
		return nil, err
	}
	catalog := declaration.NewCatalog()
	for _, model := range models {
		declared := declaration.NewType(model.Name, "Model")
		parameters := make([]declaration.Parameter, 0, len(model.Columns))
		for _, column := range model.Columns {
			declared.InstanceMembers[column.Name] = declaration.Member{
				Name: column.Name, Kind: declaration.Property, Intrinsic: "trb.orm.column", Return: column.Type, Provider: PackageName,
			}
			parameters = append(parameters, declaration.Parameter{
				Name: column.Name, Type: column.Type, Keyword: true, Optional: true,
			})
		}
		queryType := model.Name + "Query"
		declared.ClassMembers["where"] = declaration.Member{
			Name: "where", Kind: declaration.Method, Intrinsic: "trb.orm.where", Parameters: parameters,
			Return: types.FromName(queryType), Class: true, Provider: PackageName,
		}
		catalog.Types[model.Name] = declared
		query := declaration.NewType(queryType, "")
		query.InstanceMembers["all"] = declaration.Member{
			Name: "all", Kind: declaration.Method, Intrinsic: "trb.orm.query.all",
			Return: arrayOf(model.Name), Provider: PackageName,
		}
		catalog.Types[queryType] = query
	}
	return catalog, nil
}

func discoverModels(programs []*ast.Program, schema *Schema) ([]Model, error) {
	var models []Model
	seen := map[string]bool{}
	for _, program := range programs {
		for _, statement := range program.Statements {
			class, ok := statement.(*ast.ClassStatement)
			if !ok || expressionName(class.Superclass) != "Model" {
				continue
			}
			if program.Mode != "" && program.Mode != "go" {
				return nil, fmt.Errorf("trb/orm currently supports mode go; got mode %s", program.Mode)
			}
			if seen[class.Name] {
				return nil, fmt.Errorf("trb/orm model %s is declared more than once", class.Name)
			}
			seen[class.Name] = true
			tableName := TableName(class.Name)
			table, exists := schema.Table(tableName)
			if !exists {
				return nil, fmt.Errorf("trb/orm model %s expects table %s, but it was not found", class.Name, tableName)
			}
			models = append(models, Model{
				Name: class.Name, QueryType: class.Name + "Query", Table: table.Name,
				ModulePath: program.ModulePath, Columns: append([]Column(nil), table.Columns...),
			})
		}
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ModulePath != models[j].ModulePath {
			return models[i].ModulePath < models[j].ModulePath
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}

func expressionName(expression ast.Expression) string {
	if identifier, ok := expression.(*ast.Identifier); ok {
		return identifier.Name
	}
	return ""
}

func arrayOf(name string) types.Type {
	return types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{types.FromName(name)}}
}
