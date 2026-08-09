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
		if _, ok := model.PrimaryKey(); ok {
			declared.InstanceMembers["with"] = withDeclaration(model)
			declared.InstanceMembers["update"] = updateDeclaration(model)
			declared.InstanceMembers["delete"] = declaration.Member{
				Name: "delete", Kind: declaration.Method, Intrinsic: "trb.orm.delete",
				Return: dbResult(types.FromName("Boolean")), Provider: PackageName,
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
		declared.ClassMembers["not"] = notDeclaration(model, "trb.orm.not", true)
		declared.ClassMembers["find_by"] = findByDeclaration(model, "trb.orm.find_by", true)
		declared.ClassMembers["exists?"] = existsDeclaration(model, "trb.orm.exists", true)
		declared.ClassMembers["pluck"] = projectionDeclaration(model, "pluck", "trb.orm.pluck", true, false)
		declared.ClassMembers["pick"] = projectionDeclaration(model, "pick", "trb.orm.pick", true, true)
		for _, operation := range AggregateOperations() {
			if aggregate, ok := aggregateDeclaration(model, operation, "trb.orm."+operation, true); ok {
				declared.ClassMembers[operation] = aggregate
			}
		}
		if primaryKey, ok := model.PrimaryKey(); ok {
			declared.ClassMembers["find"] = findDeclaration(model, primaryKey)
			declared.ClassMembers["ids"] = idsDeclaration(model, "trb.orm.ids", true, primaryKey)
			declared.ClassMembers["build"] = buildDeclaration(model, schema.Adapter)
			declared.ClassMembers["create"] = createDeclaration(model, schema.Adapter)
			declared.ClassMembers["insert_all"] = declaration.Member{
				Name: "insert_all", Kind: declaration.Method, Intrinsic: "trb.orm.insert_all",
				Parameters: []declaration.Parameter{{Name: "drafts", Type: arrayOf(model.DraftType())}},
				Return:     dbResult(types.FromName("Integer")), Class: true, Provider: PackageName,
			}
			declared.ClassMembers["insert_if_absent"] = declaration.Member{
				Name: "insert_if_absent", Kind: declaration.Method, Intrinsic: "trb.orm.insert_if_absent",
				Parameters: []declaration.Parameter{
					{Name: "draft", Type: types.FromName(model.DraftType())}, uniqueByDeclarationParameter(model),
				},
				Return: dbResult(types.FromName("Boolean")), Class: true, Provider: PackageName,
			}
			declared.ClassMembers["upsert_all"] = declaration.Member{
				Name: "upsert_all", Kind: declaration.Method, Intrinsic: "trb.orm.upsert_all",
				Parameters: []declaration.Parameter{
					{Name: "drafts", Type: arrayOf(model.DraftType())},
					uniqueByDeclarationParameter(model), updateColumnsDeclarationParameter(model),
				},
				Return: dbResult(types.FromName("Integer")), Class: true, Provider: PackageName,
			}
			draft := declaration.NewType(model.DraftType(), "")
			draft.InstanceMembers["save"] = declaration.Member{
				Name: "save", Kind: declaration.Method, Intrinsic: "trb.orm.draft.save",
				Return: dbResult(types.FromName(model.Name)), Provider: PackageName,
			}
			draft.InstanceMembers["upsert"] = declaration.Member{
				Name: "upsert", Kind: declaration.Method, Intrinsic: "trb.orm.draft.upsert",
				Parameters: []declaration.Parameter{uniqueByDeclarationParameter(model), updateColumnsDeclarationParameter(model)},
				Return:     dbResult(types.FromName(model.Name)), Provider: PackageName,
			}
			catalog.Types[model.DraftType()] = draft
			changes := declaration.NewType(model.ChangesType(), "")
			changes.InstanceMembers["save"] = declaration.Member{
				Name: "save", Kind: declaration.Method, Intrinsic: "trb.orm.changes.save",
				Return: dbResult(types.FromName(model.Name)), Provider: PackageName,
			}
			catalog.Types[model.ChangesType()] = changes
		}
		if _, ok := model.BatchKey(); ok {
			declared.ClassMembers["find_each"] = batchDeclaration(model, "find_each", true, false)
			declared.ClassMembers["find_in_batches"] = batchDeclaration(model, "find_in_batches", true, true)
		}
		catalog.Types[model.Name] = declared
		query := declaration.NewType(model.QueryType, "")
		query.InstanceMembers["where"] = whereDeclaration(model, "trb.orm.query.where", false)
		query.InstanceMembers["not"] = notDeclaration(model, "trb.orm.query.not", false)
		query.InstanceMembers["or"] = declaration.Member{
			Name: "or", Kind: declaration.Method, Intrinsic: "trb.orm.query.or",
			Parameters: []declaration.Parameter{{Name: "other", Type: types.FromName(model.QueryType)}},
			Return:     types.FromName(model.QueryType), Provider: PackageName,
		}
		query.InstanceMembers["find_by"] = findByDeclaration(model, "trb.orm.query.find_by", false)
		query.InstanceMembers["exists?"] = declaration.Member{
			Name: "exists?", Kind: declaration.Method, Intrinsic: "trb.orm.query.exists",
			Return: dbResult(types.FromName("Boolean")), Provider: PackageName,
		}
		query.InstanceMembers["update_all"] = relationUpdateAllDeclaration(model)
		query.InstanceMembers["delete_all"] = declaration.Member{
			Name: "delete_all", Kind: declaration.Method, Intrinsic: "trb.orm.query.delete_all",
			Return: dbResult(types.FromName("Integer")), Provider: PackageName,
		}
		query.InstanceMembers["pluck"] = projectionDeclaration(model, "pluck", "trb.orm.query.pluck", false, false)
		query.InstanceMembers["pick"] = projectionDeclaration(model, "pick", "trb.orm.query.pick", false, true)
		for _, operation := range AggregateOperations() {
			if aggregate, ok := aggregateDeclaration(model, operation, "trb.orm.query."+operation, false); ok {
				query.InstanceMembers[operation] = aggregate
			}
		}
		if primaryKey, ok := model.PrimaryKey(); ok {
			query.InstanceMembers["ids"] = idsDeclaration(model, "trb.orm.query.ids", false, primaryKey)
		}
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

func uniqueByDeclarationParameter(model Model) declaration.Parameter {
	return declaration.Parameter{
		Name: "unique_by", Type: arrayOf("String"), Keyword: true,
		LiteralArrays: uniqueColumnSets(model),
	}
}

func updateColumnsDeclarationParameter(model Model) declaration.Parameter {
	var values []string
	for _, column := range model.Columns {
		if !column.PrimaryKey && !column.Generated {
			values = append(values, column.Name)
		}
	}
	return declaration.Parameter{
		Name: "update", Type: arrayOf("String"), Keyword: true, LiteralArrayElements: values,
	}
}

func uniqueColumnSets(model Model) [][]string {
	values := make([][]string, len(model.UniqueConstraints))
	for index, constraint := range model.UniqueConstraints {
		values[index] = append([]string(nil), constraint.Columns...)
	}
	return values
}

func updateDeclaration(model Model) declaration.Member {
	return declaration.Member{
		Name: "update", Kind: declaration.Method, Intrinsic: "trb.orm.update",
		Parameters: updateParameters(model), Return: dbResult(types.FromName(model.Name)), Provider: PackageName,
	}
}

func withDeclaration(model Model) declaration.Member {
	return declaration.Member{
		Name: "with", Kind: declaration.Method, Intrinsic: "trb.orm.with",
		Parameters: updateParameters(model), Return: types.FromName(model.ChangesType()), Provider: PackageName,
	}
}

func updateParameters(model Model) []declaration.Parameter {
	parameters := make([]declaration.Parameter, 0, len(model.Columns)-1)
	for _, column := range model.Columns {
		if column.PrimaryKey {
			continue
		}
		parameters = append(parameters, declaration.Parameter{
			Name: column.Name, Type: column.Type, Keyword: true, Optional: true,
		})
	}
	return parameters
}

func relationUpdateAllDeclaration(model Model) declaration.Member {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	for _, column := range model.Columns {
		if column.PrimaryKey || column.Generated {
			continue
		}
		parameters = append(parameters, declaration.Parameter{
			Name: column.Name, Type: column.Type, Keyword: true, Optional: true,
		})
	}
	return declaration.Member{
		Name: "update_all", Kind: declaration.Method, Intrinsic: "trb.orm.query.update_all",
		Parameters: parameters, MinimumArguments: 1,
		Return: dbResult(types.FromName("Integer")), Provider: PackageName,
	}
}

func buildDeclaration(model Model, adapter string) declaration.Member {
	return declaration.Member{
		Name: "build", Kind: declaration.Method, Intrinsic: "trb.orm.build",
		Parameters: writeParameters(model, adapter), Return: types.FromName(model.DraftType()),
		Class: true, Provider: PackageName,
	}
}

func createDeclaration(model Model, adapter string) declaration.Member {
	return declaration.Member{
		Name: "create", Kind: declaration.Method, Intrinsic: "trb.orm.create",
		Parameters: writeParameters(model, adapter), Return: dbResult(types.FromName(model.Name)),
		Class: true, Provider: PackageName,
	}
}

func writeParameters(model Model, adapter string) []declaration.Parameter {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	for _, column := range model.Columns {
		optional := column.Nullable || column.Generated || column.HasDefault
		if adapter == "mysql" && column.PrimaryKey && !column.Generated {
			optional = column.Nullable
		}
		parameters = append(parameters, declaration.Parameter{
			Name: column.Name, Type: column.Type, Keyword: true,
			Optional: optional,
		})
	}
	return parameters
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
		parameters = append(parameters, declaration.Parameter{Name: column.Name, Type: predicateValueType(column), Keyword: true, Optional: true})
		alternatives = append(alternatives, comparisonSignatures(column, model.QueryType)...)
	}
	return declaration.Member{
		Name: "where", Kind: declaration.Method, Intrinsic: intrinsic, Parameters: parameters,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName, Alternatives: alternatives,
	}
}

func notDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	return predicateDeclaration(model, "not", intrinsic, class, 1, 1)
}

func predicateDeclaration(model Model, name, intrinsic string, class bool, minimum, maximum int) declaration.Member {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	for _, column := range model.Columns {
		parameters = append(parameters, declaration.Parameter{Name: column.Name, Type: predicateValueType(column), Keyword: true, Optional: true})
	}
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic, Parameters: parameters,
		MinimumArguments: minimum, MaximumArguments: maximum,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName,
	}
}

func findByDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	member := predicateDeclaration(model, "find_by", intrinsic, class, 1, 0)
	result := types.FromName(model.Name)
	result.Nullable = true
	member.Return = dbResult(result)
	return member
}

func existsDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	member := predicateDeclaration(model, "exists?", intrinsic, class, 0, 0)
	member.Return = dbResult(types.FromName("Boolean"))
	return member
}

func projectionDeclaration(model Model, name, intrinsic string, class, pick bool) declaration.Member {
	values := make([]string, 0, len(model.Columns))
	alternatives := make([]declaration.Signature, 0, len(model.Columns))
	for _, column := range model.Columns {
		values = append(values, column.Name)
		result := column.Type
		if pick {
			result.Nullable = true
		} else {
			result = types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{result}}
		}
		alternatives = append(alternatives, declaration.Signature{
			Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: []string{column.Name}}},
			Return:     dbResult(result),
		})
	}
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: values}},
		Return:     alternatives[0].Return, Class: class, Provider: PackageName, Alternatives: alternatives,
	}
}

func aggregateDeclaration(model Model, operation, intrinsic string, class bool) (declaration.Member, bool) {
	values := make([]string, 0, len(model.Columns))
	alternatives := make([]declaration.Signature, 0, len(model.Columns))
	for _, column := range model.Columns {
		result, ok := AggregateResultType(operation, column)
		if !ok {
			continue
		}
		values = append(values, column.Name)
		alternatives = append(alternatives, declaration.Signature{
			Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: []string{column.Name}}},
			Return:     dbResult(result),
		})
	}
	if len(alternatives) == 0 {
		return declaration.Member{}, false
	}
	return declaration.Member{
		Name: operation, Kind: declaration.Method, Intrinsic: intrinsic,
		Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: values}},
		Return:     alternatives[0].Return, Class: class, Provider: PackageName, Alternatives: alternatives,
	}, true
}

func idsDeclaration(model Model, intrinsic string, class bool, primaryKey Column) declaration.Member {
	keyType := primaryKey.Type
	keyType.Nullable = false
	return declaration.Member{
		Name: "ids", Kind: declaration.Method, Intrinsic: intrinsic,
		Return: dbResult(types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{keyType}}),
		Class:  class, Provider: PackageName,
	}
}

func predicateValueType(column Column) types.Type {
	element := column.Type
	element.Nullable = false
	alternatives := []types.Type{column.Type, {Kind: types.Array, Name: "Array", Args: []types.Type{element}}}
	if element.Kind == types.Int {
		alternatives = append(alternatives, types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{element}})
	}
	return types.UnionOf(alternatives...)
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
				UniqueConstraints: append([]UniqueConstraint(nil), table.UniqueConstraints...),
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
		if !ok || callee.Name != string(BelongsTo) && callee.Name != string(HasMany) && callee.Name != string(HasOne) {
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
	case HasOne:
		association.Name = modelBaseName(target.Name)
		association.SourceColumn = sourceKey.Name
		association.TargetColumn = modelBaseName(source.Name) + "_id"
		foreignKeys = targetTable.ForeignKeys
		foreignTable, foreignColumn = target.Table, association.TargetColumn
		referencedTable, referencedColumn = source.Table, association.SourceColumn
		association.CardinalityVerified = hasExactUniqueConstraint(targetTable, association.TargetColumn)
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
		if (kind == HasMany || kind == HasOne) && foreignKey.Column == association.TargetColumn && foreignKey.ReferencedTable == source.Table && referencedColumn == association.SourceColumn {
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

func hasExactUniqueConstraint(table Table, column string) bool {
	for _, constraint := range table.UniqueConstraints {
		if len(constraint.Columns) == 1 && constraint.Columns[0] == column {
			return true
		}
	}
	return false
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
