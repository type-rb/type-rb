package orm

import (
	"fmt"
	"sort"
	"strings"

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
		for _, association := range model.Associations {
			if association.Preloadable {
				declared.InstanceMembers[association.Name] = declaration.Member{
					Name: association.Name, Kind: declaration.Method,
					Intrinsic: "trb.orm.association.loaded." + string(association.Kind),
					Return:    associationValueType(association), Provider: PackageName,
				}
			}
			declared.InstanceMembers[association.Name+"_query"] = declaration.Member{
				Name: association.Name + "_query", Kind: declaration.Method,
				Intrinsic: "trb.orm.association.query." + string(association.Kind),
				Return:    types.FromName(association.TargetQuery), Provider: PackageName,
			}
		}
		declared.ClassMembers["where"] = whereDeclaration(model, "trb.orm.where", true)
		if primaryKey, ok := model.PrimaryKey(); ok {
			declared.ClassMembers["find"] = findDeclaration(model, primaryKey)
		}
		if _, ok := model.BatchKey(); ok {
			declared.ClassMembers["find_each"] = batchDeclaration(model, "find_each", true, false)
			declared.ClassMembers["find_in_batches"] = batchDeclaration(model, "find_in_batches", true, true)
		}
		catalog.Types[model.Name] = declared
		query := declaration.NewType(model.QueryType, "")
		query.InstanceMembers["where"] = whereDeclaration(model, "trb.orm.query.where", false)
		query.InstanceMembers["order"] = orderDeclaration(model)
		query.InstanceMembers["limit"] = integerQueryDeclaration("limit", "trb.orm.query.limit", model.QueryType)
		query.InstanceMembers["offset"] = integerQueryDeclaration("offset", "trb.orm.query.offset", model.QueryType)
		query.InstanceMembers["all"] = declaration.Member{
			Name: "all", Kind: declaration.Method, Intrinsic: "trb.orm.query.all",
			Return: dbResult(arrayOf(model.Name)), Provider: PackageName,
		}
		firstType := types.FromName(model.Name)
		firstType.Nullable = true
		query.InstanceMembers["first"] = declaration.Member{
			Name: "first", Kind: declaration.Method, Intrinsic: "trb.orm.query.first", Return: dbResult(firstType), Provider: PackageName,
		}
		query.InstanceMembers["count"] = declaration.Member{
			Name: "count", Kind: declaration.Method, Intrinsic: "trb.orm.query.count", Return: dbResult(types.FromName("Integer")), Provider: PackageName,
		}
		query.InstanceMembers["to_sql"] = stringQueryDeclaration("to_sql", "trb.orm.query.to_sql")
		query.InstanceMembers["explain"] = resultQueryDeclaration("explain", "trb.orm.query.explain", types.FromName("String"))
		if preload := preloadDeclaration(model); preload.Name != "" {
			query.InstanceMembers["preload"] = preload
		}
		if _, ok := model.BatchKey(); ok {
			query.InstanceMembers["find_each"] = batchDeclaration(model, "find_each", false, false)
			query.InstanceMembers["find_in_batches"] = batchDeclaration(model, "find_in_batches", false, true)
		}
		catalog.Types[model.QueryType] = query
	}
	return catalog, nil
}

func preloadDeclaration(model Model) declaration.Member {
	var values []string
	for _, association := range model.Associations {
		if association.Preloadable {
			values = append(values, association.Name)
		}
	}
	if len(values) == 0 {
		return declaration.Member{}
	}
	return declaration.Member{
		Name: "preload", Kind: declaration.Method, Intrinsic: "trb.orm.query.preload",
		Parameters: []declaration.Parameter{{Name: "association", Type: types.FromName("String"), LiteralValues: values}},
		Return:     types.FromName(model.QueryType), Provider: PackageName,
	}
}

func findDeclaration(model Model, primaryKey Column) declaration.Member {
	result := types.FromName(model.Name)
	result.Nullable = true
	keyType := primaryKey.Type
	keyType.Nullable = false
	return declaration.Member{
		Name: "find", Kind: declaration.Method, Intrinsic: "trb.orm.find",
		Parameters: []declaration.Parameter{{Name: primaryKey.Name, Type: keyType}},
		Return:     dbResult(result), Class: true, Provider: PackageName,
	}
}

func batchDeclaration(model Model, name string, class, batches bool) declaration.Member {
	parameterType := types.FromName(model.Name)
	if batches {
		parameterType = arrayOf(model.Name)
	}
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: "trb.orm.query." + name,
		Parameters: []declaration.Parameter{{Name: "batch_size", Type: types.FromName("Integer"), Keyword: true, Optional: true}},
		Return:     dbResult(types.FromName("Integer")), Class: class, Provider: PackageName,
		Block: &declaration.Block{Parameters: []types.Type{parameterType}, Structured: true},
	}
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

func stringQueryDeclaration(name, intrinsic string) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Return: types.FromName("String"), Provider: PackageName,
	}
}

func resultQueryDeclaration(name, intrinsic string, result types.Type) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Return: dbResult(result), Provider: PackageName,
	}
}

func dbResult(value types.Type) types.Type {
	return types.Type{Kind: types.Named, Name: "DbResult", Args: []types.Type{value}}
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
	classes := map[string]*ast.ClassStatement{}
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
			classes[class.Name] = class
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
	byName := map[string]*Model{}
	for index := range models {
		byName[models[index].Name] = &models[index]
	}
	for index := range models {
		associations, err := discoverAssociations(models[index], classes[models[index].Name], byName, schema)
		if err != nil {
			return nil, err
		}
		models[index].Associations = associations
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ModulePath != models[j].ModulePath {
			return models[i].ModulePath < models[j].ModulePath
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}

func discoverAssociations(source Model, class *ast.ClassStatement, models map[string]*Model, schema *Schema) ([]Association, error) {
	if class == nil {
		return nil, nil
	}
	var result []Association
	seen := map[string]bool{}
	for _, statement := range class.Body {
		expression, ok := statement.(*ast.ExpressionStatement)
		if !ok {
			continue
		}
		call, ok := expression.Expression.(*ast.CallExpression)
		if !ok {
			continue
		}
		callee, ok := call.Callee.(*ast.Identifier)
		if !ok || callee.Name != string(BelongsTo) && callee.Name != string(HasMany) {
			continue
		}
		if len(call.Arguments) != 1 || call.Arguments[0].Name != "" {
			return nil, fmt.Errorf("trb/orm %s.%s expects exactly one model type", source.Name, callee.Name)
		}
		targetName := expressionName(call.Arguments[0].Value)
		target := models[targetName]
		if target == nil {
			return nil, fmt.Errorf("trb/orm %s.%s references unknown model %s", source.Name, callee.Name, targetName)
		}
		association, err := buildAssociation(source, *target, AssociationKind(callee.Name), schema)
		if err != nil {
			return nil, err
		}
		if seen[association.Name] {
			return nil, fmt.Errorf("trb/orm model %s declares association %s more than once", source.Name, association.Name)
		}
		seen[association.Name] = true
		result = append(result, association)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func buildAssociation(source, target Model, kind AssociationKind, schema *Schema) (Association, error) {
	sourceTable, _ := schema.Table(source.Table)
	targetTable, _ := schema.Table(target.Table)
	sourceKey, sourceKeyOK := source.PrimaryKey()
	targetKey, targetKeyOK := target.PrimaryKey()
	if !sourceKeyOK || !targetKeyOK {
		return Association{}, fmt.Errorf("trb/orm association %s to %s requires one primary key on each model", source.Name, target.Name)
	}
	association := Association{Kind: kind, TargetModel: target.Name, TargetQuery: target.QueryType}
	var foreignKeys []ForeignKey
	foreignTable, foreignColumn := "", ""
	referencedTable, referencedColumn := "", ""
	switch kind {
	case BelongsTo:
		association.Name = modelBaseName(target.Name)
		association.SourceColumn = modelBaseName(target.Name) + "_id"
		association.TargetColumn = targetKey.Name
		foreignKeys = sourceTable.ForeignKeys
		foreignTable, foreignColumn = source.Table, association.SourceColumn
		referencedTable, referencedColumn = target.Table, association.TargetColumn
	case HasMany:
		association.Name = target.Table
		association.SourceColumn = sourceKey.Name
		association.TargetColumn = modelBaseName(source.Name) + "_id"
		foreignKeys = targetTable.ForeignKeys
		foreignTable, foreignColumn = target.Table, association.TargetColumn
		referencedTable, referencedColumn = source.Table, association.SourceColumn
	default:
		return Association{}, fmt.Errorf("unsupported trb/orm association %q", kind)
	}
	for _, foreignKey := range foreignKeys {
		referencedColumn := foreignKey.ReferencedColumn
		if referencedColumn == "" {
			referencedColumn = targetKey.Name
		}
		if kind == BelongsTo && foreignKey.Column == association.SourceColumn && foreignKey.ReferencedTable == target.Table && referencedColumn == association.TargetColumn {
			association.Preloadable = source.ModulePath == target.ModulePath && preloadKeyCompatible(source, target, association)
			return association, nil
		}
		if kind == HasMany && foreignKey.Column == association.TargetColumn && foreignKey.ReferencedTable == source.Table && referencedColumn == association.SourceColumn {
			association.Preloadable = source.ModulePath == target.ModulePath && preloadKeyCompatible(source, target, association)
			return association, nil
		}
	}
	return Association{}, fmt.Errorf(
		"trb/orm association %s.%s requires foreign key %s.%s -> %s.%s",
		source.Name, association.Name,
		foreignTable, foreignColumn, referencedTable, referencedColumn,
	)
}

func preloadKeyCompatible(source, target Model, association Association) bool {
	sourceColumn, sourceOK := source.Column(association.SourceColumn)
	targetColumn, targetOK := target.Column(association.TargetColumn)
	if !sourceOK || !targetOK {
		return false
	}
	switch sourceColumn.Type.Kind {
	case types.Bool, types.Int, types.Float, types.String:
	default:
		return false
	}
	return sourceColumn.Type.Kind == targetColumn.Type.Kind
}

func modelBaseName(model string) string {
	return strings.Join(splitIdentifier(model), "_")
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
