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
		for _, column := range model.Columns {
			declared.InstanceMembers[column.Name] = declaration.Member{
				Name: column.Name, Kind: declaration.Property, Intrinsic: "trb.orm.column", Return: column.Type, Provider: PackageName,
			}
		}
		declared.ClassMembers["where"] = whereDeclaration(model, "trb.orm.where", true)
		catalog.Types[model.Name] = declared
		query := declaration.NewType(model.QueryType, "")
		query.InstanceMembers["where"] = whereDeclaration(model, "trb.orm.query.where", false)
		query.InstanceMembers["order"] = orderDeclaration(model)
		query.InstanceMembers["limit"] = integerQueryDeclaration("limit", "trb.orm.query.limit", model.QueryType)
		query.InstanceMembers["offset"] = integerQueryDeclaration("offset", "trb.orm.query.offset", model.QueryType)
		query.InstanceMembers["all"] = declaration.Member{
			Name: "all", Kind: declaration.Method, Intrinsic: "trb.orm.query.all",
			Return: arrayOf(model.Name), Provider: PackageName,
		}
		firstType := types.FromName(model.Name)
		firstType.Nullable = true
		query.InstanceMembers["first"] = declaration.Member{
			Name: "first", Kind: declaration.Method, Intrinsic: "trb.orm.query.first", Return: firstType, Provider: PackageName,
		}
		query.InstanceMembers["count"] = declaration.Member{
			Name: "count", Kind: declaration.Method, Intrinsic: "trb.orm.query.count", Return: types.FromName("Integer"), Provider: PackageName,
		}
		catalog.Types[model.QueryType] = query
	}
	return catalog, nil
}

func whereDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	var alternatives []declaration.Signature
	for _, column := range model.Columns {
		parameters = append(parameters, declaration.Parameter{Name: column.Name, Type: column.Type, Keyword: true, Optional: true})
		alternatives = append(alternatives, comparisonSignatures(column, model.QueryType)...)
	}
	return declaration.Member{
		Name: "where", Kind: declaration.Method, Intrinsic: intrinsic, Parameters: parameters,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName, Alternatives: alternatives,
	}
}

func orderDeclaration(model Model) declaration.Member {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	for _, column := range model.Columns {
		parameters = append(parameters, declaration.Parameter{
			Name: column.Name, Type: types.FromName("String"), Keyword: true, Optional: true, LiteralValues: []string{"asc", "desc"},
		})
	}
	return declaration.Member{
		Name: "order", Kind: declaration.Method, Intrinsic: "trb.orm.query.order", Parameters: parameters,
		Return: types.FromName(model.QueryType), Provider: PackageName,
	}
}

func integerQueryDeclaration(name, intrinsic, queryType string) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Parameters: []declaration.Parameter{{Name: "count", Type: types.FromName("Integer")}},
		Return:     types.FromName(queryType), Provider: PackageName,
	}
}

func comparisonSignatures(column Column, queryType string) []declaration.Signature {
	signature := func(operators []string, valueType types.Type) declaration.Signature {
		return declaration.Signature{
			Parameters: []declaration.Parameter{
				{Name: "column", Type: types.FromName("String"), LiteralValues: []string{column.Name}},
				{Name: "operator", Type: types.FromName("String"), LiteralValues: operators},
				{Name: "value", Type: valueType},
			},
			Return: types.FromName(queryType),
		}
	}
	result := []declaration.Signature{signature([]string{"=", "!="}, column.Type)}
	switch column.Type.Kind {
	case types.Int, types.Float, types.String:
		orderedType := column.Type
		orderedType.Nullable = false
		result = append(result, signature([]string{"<", "<=", ">", ">="}, orderedType))
	}
	return result
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
