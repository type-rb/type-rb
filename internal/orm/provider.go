package orm

import (
	"fmt"
	"sort"
	"strconv"
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
	for _, function := range []string{string(BelongsTo), string(HasMany), string(HasOne)} {
		catalog.FunctionBlockRules = append(catalog.FunctionBlockRules, declaration.FunctionBlockRule{
			Package: PackageName, Function: function, EnclosingSuperclass: "Model",
			TypeArgument: 0, ParameterTypeSuffix: "Query",
		})
	}
	database := declaration.NewType("Database", "")
	database.ClassMembers["transaction"] = transactionDeclaration(true)
	catalog.Types["Database"] = database
	transaction := declaration.NewType("Transaction", "")
	transaction.InstanceMembers["transaction"] = transactionDeclaration(false)
	catalog.Types["Transaction"] = transaction
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
		declared.ClassMembers["distinct"] = distinctDeclaration(model, "trb.orm.distinct", true)
		declared.ClassMembers["select"] = selectDeclaration(model, "trb.orm.select", true)
		declared.ClassMembers["group"] = groupDeclaration(model, "trb.orm.group", true)
		if join := joinDeclaration(model, "join", "trb.orm.join", true); join.Name != "" {
			declared.ClassMembers["join"] = join
		}
		if join := joinDeclaration(model, "left_join", "trb.orm.left_join", true); join.Name != "" {
			declared.ClassMembers["left_join"] = join
		}
		if exists := joinDeclaration(model, "where_exists", "trb.orm.where_exists", true); exists.Name != "" {
			declared.ClassMembers["where_exists"] = exists
		}
		if exists := joinDeclaration(model, "where_not_exists", "trb.orm.where_not_exists", true); exists.Name != "" {
			declared.ClassMembers["where_not_exists"] = exists
		}
		declared.ClassMembers["using"] = declaration.Member{
			Name: "using", Kind: declaration.Method, Intrinsic: "trb.orm.using",
			Parameters: []declaration.Parameter{{Name: "transaction", Type: types.FromName("Transaction")}},
			Return:     types.FromName(model.ScopeType()), Class: true, Provider: PackageName,
		}
		declared.ClassMembers["not"] = notDeclaration(model, "trb.orm.not", true)
		declared.ClassMembers["order"] = orderDeclaration(model, "trb.orm.order", true)
		declared.ClassMembers["limit"] = integerQueryDeclaration("limit", "trb.orm.limit", model.QueryType, true)
		declared.ClassMembers["offset"] = integerQueryDeclaration("offset", "trb.orm.offset", model.QueryType, true)
		declared.ClassMembers["all"] = resultQueryDeclaration("all", "trb.orm.all", arrayOf(model.Name), true)
		firstType := types.FromName(model.Name)
		firstType.Nullable = true
		declared.ClassMembers["first"] = resultQueryDeclaration("first", "trb.orm.first", firstType, true)
		declared.ClassMembers["count"] = resultQueryDeclaration("count", "trb.orm.count", types.FromName("Integer"), true)
		declared.ClassMembers["to_sql"] = stringQueryDeclaration("to_sql", "trb.orm.to_sql", true)
		declared.ClassMembers["explain"] = resultQueryDeclaration("explain", "trb.orm.explain", types.FromName("String"), true)
		if preload := preloadDeclaration(model, "trb.orm.preload", true); preload.Name != "" {
			declared.ClassMembers["preload"] = preload
		}
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
		query.InstanceMembers["distinct"] = distinctDeclaration(model, "trb.orm.query.distinct", false)
		query.InstanceMembers["select"] = selectDeclaration(model, "trb.orm.query.select", false)
		query.InstanceMembers["group"] = groupDeclaration(model, "trb.orm.query.group", false)
		if join := joinDeclaration(model, "join", "trb.orm.query.join", false); join.Name != "" {
			query.InstanceMembers["join"] = join
		}
		if join := joinDeclaration(model, "left_join", "trb.orm.query.left_join", false); join.Name != "" {
			query.InstanceMembers["left_join"] = join
		}
		if exists := joinDeclaration(model, "where_exists", "trb.orm.query.where_exists", false); exists.Name != "" {
			query.InstanceMembers["where_exists"] = exists
		}
		if exists := joinDeclaration(model, "where_not_exists", "trb.orm.query.where_not_exists", false); exists.Name != "" {
			query.InstanceMembers["where_not_exists"] = exists
		}
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
		query.InstanceMembers["order"] = orderDeclaration(model, "trb.orm.query.order", false)
		query.InstanceMembers["limit"] = integerQueryDeclaration("limit", "trb.orm.query.limit", model.QueryType, false)
		query.InstanceMembers["offset"] = integerQueryDeclaration("offset", "trb.orm.query.offset", model.QueryType, false)
		query.InstanceMembers["lock"] = declaration.Member{
			Name: "lock", Kind: declaration.Method, Intrinsic: "trb.orm.query.lock",
			Return: types.FromName(model.QueryType), Provider: PackageName,
		}
		query.InstanceMembers["all"] = declaration.Member{
			Name: "all", Kind: declaration.Method, Intrinsic: "trb.orm.query.all",
			Return: dbResult(arrayOf(model.Name)), Provider: PackageName,
		}
		queryFirstType := types.FromName(model.Name)
		queryFirstType.Nullable = true
		query.InstanceMembers["first"] = declaration.Member{
			Name: "first", Kind: declaration.Method, Intrinsic: "trb.orm.query.first", Return: dbResult(queryFirstType), Provider: PackageName,
		}
		query.InstanceMembers["count"] = declaration.Member{
			Name: "count", Kind: declaration.Method, Intrinsic: "trb.orm.query.count", Return: dbResult(types.FromName("Integer")), Provider: PackageName,
		}
		query.InstanceMembers["to_sql"] = stringQueryDeclaration("to_sql", "trb.orm.query.to_sql", false)
		query.InstanceMembers["explain"] = resultQueryDeclaration("explain", "trb.orm.query.explain", types.FromName("String"), false)
		if preload := preloadDeclaration(model, "trb.orm.query.preload", false); preload.Name != "" {
			query.InstanceMembers["preload"] = preload
		}
		if _, ok := model.BatchKey(); ok {
			query.InstanceMembers["find_each"] = batchDeclaration(model, "find_each", false, false)
			query.InstanceMembers["find_in_batches"] = batchDeclaration(model, "find_in_batches", false, true)
		}
		catalog.Types[model.QueryType] = query
		for _, column := range model.Columns {
			grouped := declaration.NewType(model.GroupType(column), "")
			having := declaration.Member{Name: "having", Kind: declaration.Method, Intrinsic: "trb.orm.group.having", Return: types.FromName(model.GroupType(column)), Provider: PackageName}
			having.Alternatives = append(having.Alternatives, declaration.Signature{Parameters: []declaration.Parameter{{Name: "aggregate", Type: types.FromName("String"), LiteralValues: []string{"count"}}, {Name: "operator", Type: types.FromName("String"), LiteralValues: []string{"=", "!=", "<", "<=", ">", ">="}}, {Name: "value", Type: types.FromName("Integer")}}, Return: having.Return})
			grouped.InstanceMembers["count"] = declaration.Member{Name: "count", Kind: declaration.Method, Intrinsic: "trb.orm.group.count", Return: dbResult(types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{column.Type, types.FromName("Integer")}}), Provider: PackageName}
			for _, operation := range AggregateOperations() {
				aggregate, ok := aggregateDeclaration(model, operation, "trb.orm.group."+operation, false)
				if !ok {
					continue
				}
				for index := range aggregate.Alternatives {
					result := aggregate.Alternatives[index].Return.Args[0]
					aggregate.Alternatives[index].Return = dbResult(types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{column.Type, result}})
				}
				aggregate.Return = aggregate.Alternatives[0].Return
				grouped.InstanceMembers[operation] = aggregate
				for _, target := range model.Columns {
					result, ok := AggregateResultType(operation, target)
					if !ok {
						continue
					}
					result.Nullable = false
					having.Alternatives = append(having.Alternatives, declaration.Signature{Parameters: []declaration.Parameter{{Name: "aggregate", Type: types.FromName("String"), LiteralValues: []string{operation}}, {Name: "column", Type: types.FromName("String"), LiteralValues: []string{target.Name}}, {Name: "operator", Type: types.FromName("String"), LiteralValues: []string{"=", "!=", "<", "<=", ">", ">="}}, {Name: "value", Type: result}}, Return: having.Return})
				}
			}
			having.Parameters = having.Alternatives[0].Parameters
			grouped.InstanceMembers["having"] = having
			catalog.Types[model.GroupType(column)] = grouped
		}
		scope := declaration.NewType(model.ScopeType(), "")
		for name, member := range query.InstanceMembers {
			scope.InstanceMembers[name] = member
		}
		if primaryKey, ok := model.PrimaryKey(); ok {
			scope.InstanceMembers["find"] = scopeFindDeclaration(model, primaryKey)
			scope.InstanceMembers["build"] = scopeWriteDeclaration(model, "build", "trb.orm.scope.build", types.FromName(model.DraftType()), schema.Adapter)
			scope.InstanceMembers["create"] = scopeWriteDeclaration(model, "create", "trb.orm.scope.create", dbResult(types.FromName(model.Name)), schema.Adapter)
		}
		catalog.Types[model.ScopeType()] = scope
	}
	return catalog, nil
}

func groupDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	member := declaration.Member{Name: "group", Kind: declaration.Method, Intrinsic: intrinsic, Class: class, Provider: PackageName}
	for _, column := range model.Columns {
		member.Alternatives = append(member.Alternatives, declaration.Signature{Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: []string{column.Name}}}, Return: types.FromName(model.GroupType(column))})
	}
	member.Return = member.Alternatives[0].Return
	return member
}

func distinctDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	return declaration.Member{
		Name: "distinct", Kind: declaration.Method, Intrinsic: intrinsic,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName,
	}
}

func transactionDeclaration(class bool) declaration.Member {
	typeParameter := types.FromName("T")
	result := dbResult(typeParameter)
	return declaration.Member{
		Name: "transaction", Kind: declaration.Method, Intrinsic: "trb.orm.transaction",
		Return: result, Class: class, TypeParameters: []string{"T"}, Provider: PackageName,
		Block: &declaration.Block{
			Parameters: []types.Type{types.FromName("Transaction")},
			Return:     result, Structured: true,
		},
	}
}

func joinDeclaration(model Model, name, intrinsic string, class bool) declaration.Member {
	member := declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName,
	}
	for _, association := range model.Associations {
		if association.Through != "" && name != "join" && name != "left_join" {
			continue
		}
		associationParameter := declaration.Parameter{
			Name: "association", Type: types.FromName("String"), LiteralValues: []string{association.Name},
		}
		member.Alternatives = append(member.Alternatives,
			declaration.Signature{
				Parameters: []declaration.Parameter{associationParameter},
				Return:     types.FromName(model.QueryType),
			},
			declaration.Signature{
				Parameters: []declaration.Parameter{
					associationParameter,
					{Name: "query", Type: types.FromName(association.TargetQuery)},
				},
				Return: types.FromName(model.QueryType),
			},
		)
	}
	if len(member.Alternatives) == 0 {
		return declaration.Member{}
	}
	return member
}

func scopeFindDeclaration(model Model, primaryKey Column) declaration.Member {
	member := findDeclaration(model, primaryKey)
	member.Class = false
	member.Intrinsic = "trb.orm.scope.find"
	return member
}

func scopeWriteDeclaration(model Model, name, intrinsic string, result types.Type, adapter string) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Parameters: writeParameters(model, adapter), Return: result, Provider: PackageName,
	}
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

func preloadDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	member := declaration.Member{
		Name: "preload", Kind: declaration.Method, Intrinsic: intrinsic,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName,
	}
	for _, association := range model.Associations {
		if !association.Preloadable {
			continue
		}
		associationParameter := declaration.Parameter{
			Name: "association", Type: types.FromName("String"), LiteralValues: []string{association.Name},
		}
		member.Alternatives = append(member.Alternatives,
			declaration.Signature{
				Parameters: []declaration.Parameter{associationParameter},
				Return:     types.FromName(model.QueryType),
			},
			declaration.Signature{
				Parameters: []declaration.Parameter{
					associationParameter,
					{Name: "query", Type: types.FromName(association.TargetQuery)},
				},
				Return: types.FromName(model.QueryType),
			},
		)
	}
	if len(member.Alternatives) == 0 {
		return declaration.Member{}
	}
	return member
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

func selectDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	values := make([]string, 0, len(model.Columns))
	alternatives := make([]declaration.Signature, 0, len(model.Columns))
	for _, column := range model.Columns {
		values = append(values, column.Name)
		alternatives = append(alternatives, declaration.Signature{
			Parameters: []declaration.Parameter{{Name: "column", Type: types.FromName("String"), LiteralValues: []string{column.Name}}},
			Return:     subqueryOf(column.Type),
		})
	}
	return declaration.Member{
		Name: "select", Kind: declaration.Method, Intrinsic: intrinsic,
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
	alternatives := []types.Type{column.Type, {Kind: types.Array, Name: "Array", Args: []types.Type{element}}, subqueryOf(element)}
	if element.Kind == types.Int {
		alternatives = append(alternatives, types.Type{Kind: types.Range, Name: "Range", Args: []types.Type{element}})
	}
	return types.UnionOf(alternatives...)
}

func subqueryOf(element types.Type) types.Type {
	element.Nullable = false
	return types.Type{Kind: types.Named, Name: "Subquery", Args: []types.Type{element}}
}

func orderDeclaration(model Model, intrinsic string, class bool) declaration.Member {
	parameters := make([]declaration.Parameter, 0, len(model.Columns))
	for _, column := range model.Columns {
		parameters = append(parameters, declaration.Parameter{
			Name: column.Name, Type: types.FromName("String"), Keyword: true, Optional: true, LiteralValues: []string{"asc", "desc"},
		})
	}
	return declaration.Member{
		Name: "order", Kind: declaration.Method, Intrinsic: intrinsic, Parameters: parameters,
		Return: types.FromName(model.QueryType), Class: class, Provider: PackageName,
	}
}

func integerQueryDeclaration(name, intrinsic, queryType string, class bool) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Parameters: []declaration.Parameter{{Name: "count", Type: types.FromName("Integer")}},
		Return:     types.FromName(queryType), Class: class, Provider: PackageName,
	}
}

func stringQueryDeclaration(name, intrinsic string, class bool) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Return: types.FromName("String"), Class: class, Provider: PackageName,
	}
}

func resultQueryDeclaration(name, intrinsic string, result types.Type, class bool) declaration.Member {
	return declaration.Member{
		Name: name, Kind: declaration.Method, Intrinsic: intrinsic,
		Return: dbResult(result), Class: class, Provider: PackageName,
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
	result := []declaration.Signature{
		signature([]string{"=", "!="}, types.UnionOf(column.Type, subqueryOf(column.Type))),
	}
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
	specs := map[string][]associationSpec{}
	for index := range models {
		associations, err := discoverAssociationSpecs(models[index], classes[models[index].Name], byName)
		if err != nil {
			return nil, err
		}
		specs[models[index].Name] = associations
		for _, spec := range associations {
			if spec.Through != "" {
				continue
			}
			target := byName[spec.TargetModel]
			association, buildErr := buildAssociation(models[index], *target, spec, schema)
			if buildErr != nil {
				return nil, buildErr
			}
			models[index].Associations = append(models[index].Associations, association)
		}
	}
	for index := range models {
		for _, spec := range specs[models[index].Name] {
			if spec.Through == "" {
				continue
			}
			association, err := buildThroughAssociation(models[index], spec, byName)
			if err != nil {
				return nil, err
			}
			models[index].Associations = append(models[index].Associations, association)
		}
		if err := resolveAssociationInverses(&models[index], byName); err != nil {
			return nil, err
		}
		sort.Slice(models[index].Associations, func(left, right int) bool {
			return models[index].Associations[left].Name < models[index].Associations[right].Name
		})
	}
	sort.Slice(models, func(i, j int) bool {
		if models[i].ModulePath != models[j].ModulePath {
			return models[i].ModulePath < models[j].ModulePath
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}

type associationSpec struct {
	Kind        AssociationKind
	TargetModel string
	Name        string
	ForeignKey  string
	References  string
	Inverse     string
	Through     string
	Source      string
	Dependent   DependentAction
	Scoped      bool
}

func discoverAssociationSpecs(source Model, class *ast.ClassStatement, models map[string]*Model) ([]associationSpec, error) {
	if class == nil {
		return nil, nil
	}
	var result []associationSpec
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
		if len(call.Arguments) == 0 || call.Arguments[0].Name != "" {
			return nil, fmt.Errorf("trb/orm %s.%s expects a model type as its first argument", source.Name, callee.Name)
		}
		targetName := expressionName(call.Arguments[0].Value)
		if models[targetName] == nil {
			return nil, fmt.Errorf("trb/orm %s.%s references unknown model %s", source.Name, callee.Name, targetName)
		}
		spec := associationSpec{Kind: AssociationKind(callee.Name), TargetModel: targetName}
		if call.Block != nil {
			if len(call.Block.Parameters) != 1 || len(call.Block.Body) != 1 {
				return nil, fmt.Errorf("trb/orm %s.%s scope must have one query parameter and one result expression", source.Name, callee.Name)
			}
			if _, ok := call.Block.Body[0].(*ast.ExpressionStatement); !ok {
				return nil, fmt.Errorf("trb/orm %s.%s scope must return one query expression", source.Name, callee.Name)
			}
			spec.Scoped = true
		}
		options := map[string]string{}
		for _, argument := range call.Arguments[1:] {
			if argument.Name == "" {
				return nil, fmt.Errorf("trb/orm %s.%s accepts only one positional model type", source.Name, callee.Name)
			}
			if _, duplicate := options[argument.Name]; duplicate {
				return nil, fmt.Errorf("trb/orm %s.%s option %s is specified more than once", source.Name, callee.Name, argument.Name)
			}
			value, ok := associationOptionValue(argument.Value)
			if !ok {
				return nil, fmt.Errorf("trb/orm %s.%s option %s must be a symbol or string literal", source.Name, callee.Name, argument.Name)
			}
			options[argument.Name] = value
		}
		for name := range options {
			if !associationOptionAllowed(spec.Kind, name) {
				return nil, fmt.Errorf("trb/orm %s.%s does not accept option %s", source.Name, callee.Name, name)
			}
		}
		spec.Name = options["name"]
		spec.ForeignKey = options["foreign_key"]
		spec.References = options["references"]
		spec.Inverse = options["inverse"]
		spec.Through = options["through"]
		spec.Source = options["source"]
		if spec.Source != "" && spec.Through == "" {
			return nil, fmt.Errorf("trb/orm %s.%s option source requires through", source.Name, callee.Name)
		}
		if dependent := options["dependent"]; dependent != "" {
			spec.Dependent = DependentAction(dependent)
			switch spec.Dependent {
			case DependentDestroy, DependentDelete, DependentNullify, DependentRestrict:
			default:
				return nil, fmt.Errorf("trb/orm %s.%s dependent must be destroy, delete, nullify, or restrict", source.Name, callee.Name)
			}
		}
		associationName := spec.Name
		if associationName == "" {
			if spec.Kind == HasMany {
				associationName = models[targetName].Table
			} else {
				associationName = modelBaseName(targetName)
			}
		}
		if seen[associationName] {
			return nil, fmt.Errorf("trb/orm model %s declares association %s more than once", source.Name, associationName)
		}
		seen[associationName] = true
		result = append(result, spec)
	}
	return result, nil
}

func associationOptionAllowed(kind AssociationKind, name string) bool {
	switch name {
	case "name", "foreign_key", "references", "inverse", "dependent":
		return true
	case "through", "source":
		return kind == HasMany || kind == HasOne
	default:
		return false
	}
}

func associationOptionValue(expression ast.Expression) (string, bool) {
	switch value := expression.(type) {
	case *ast.SymbolLiteral:
		return value.Name, true
	case *ast.Literal:
		if value.Kind != ast.StringLiteral {
			return "", false
		}
		decoded, err := strconv.Unquote(value.Raw)
		return decoded, err == nil
	default:
		return "", false
	}
}

func buildAssociation(source, target Model, spec associationSpec, schema *Schema) (Association, error) {
	sourceTable, _ := schema.Table(source.Table)
	targetTable, _ := schema.Table(target.Table)
	sourceKey, sourceKeyOK := source.PrimaryKey()
	targetKey, targetKeyOK := target.PrimaryKey()
	if (!sourceKeyOK && spec.Kind != BelongsTo) || (!targetKeyOK && spec.Kind == BelongsTo) {
		return Association{}, fmt.Errorf("trb/orm association %s to %s requires one primary key on each model", source.Name, target.Name)
	}
	association := Association{
		Name: spec.Name, Kind: spec.Kind, TargetModel: target.Name, TargetQuery: target.QueryType,
		Inverse: spec.Inverse, Dependent: spec.Dependent, Scoped: spec.Scoped,
	}
	var foreignKeys []ForeignKey
	foreignTable, foreignColumn := "", ""
	referencedTable, referencedColumn := "", ""
	switch spec.Kind {
	case BelongsTo:
		if association.Name == "" {
			association.Name = modelBaseName(target.Name)
		}
		association.SourceColumn = spec.ForeignKey
		if association.SourceColumn == "" {
			association.SourceColumn = association.Name + "_id"
		}
		association.TargetColumn = spec.References
		if association.TargetColumn == "" {
			association.TargetColumn = targetKey.Name
		}
		foreignKeys = sourceTable.ForeignKeys
		foreignTable, foreignColumn = source.Table, association.SourceColumn
		referencedTable, referencedColumn = target.Table, association.TargetColumn
	case HasMany:
		if association.Name == "" {
			association.Name = target.Table
		}
		association.SourceColumn = spec.References
		if association.SourceColumn == "" {
			association.SourceColumn = sourceKey.Name
		}
		association.TargetColumn = spec.ForeignKey
		if association.TargetColumn == "" {
			association.TargetColumn = modelBaseName(source.Name) + "_id"
		}
		foreignKeys = targetTable.ForeignKeys
		foreignTable, foreignColumn = target.Table, association.TargetColumn
		referencedTable, referencedColumn = source.Table, association.SourceColumn
	case HasOne:
		if association.Name == "" {
			association.Name = modelBaseName(target.Name)
		}
		association.SourceColumn = spec.References
		if association.SourceColumn == "" {
			association.SourceColumn = sourceKey.Name
		}
		association.TargetColumn = spec.ForeignKey
		if association.TargetColumn == "" {
			association.TargetColumn = modelBaseName(source.Name) + "_id"
		}
		foreignKeys = targetTable.ForeignKeys
		foreignTable, foreignColumn = target.Table, association.TargetColumn
		referencedTable, referencedColumn = source.Table, association.SourceColumn
		association.CardinalityVerified = hasExactUniqueConstraint(targetTable, association.TargetColumn)
	default:
		return Association{}, fmt.Errorf("unsupported trb/orm association %q", spec.Kind)
	}
	for _, foreignKey := range foreignKeys {
		referencedColumn := foreignKey.ReferencedColumn
		if referencedColumn == "" {
			referencedColumn = association.SourceColumn
			if spec.Kind == BelongsTo {
				referencedColumn = association.TargetColumn
			}
		}
		if spec.Kind == BelongsTo && foreignKey.Column == association.SourceColumn && foreignKey.ReferencedTable == target.Table && referencedColumn == association.TargetColumn {
			association.Preloadable = source.ModulePath == target.ModulePath && preloadKeyCompatible(source, target, association)
			return association, nil
		}
		if (spec.Kind == HasMany || spec.Kind == HasOne) && foreignKey.Column == association.TargetColumn && foreignKey.ReferencedTable == source.Table && referencedColumn == association.SourceColumn {
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

func buildThroughAssociation(source Model, spec associationSpec, models map[string]*Model) (Association, error) {
	through, ok := source.Association(spec.Through)
	if !ok {
		return Association{}, fmt.Errorf("trb/orm association %s through references unknown association %s", source.Name, spec.Through)
	}
	middle := models[through.TargetModel]
	if middle == nil {
		return Association{}, fmt.Errorf("trb/orm association %s through target %s is unavailable", source.Name, through.TargetModel)
	}
	sourceName := spec.Source
	if sourceName == "" {
		for _, candidate := range middle.Associations {
			if candidate.TargetModel != spec.TargetModel {
				continue
			}
			if sourceName != "" {
				return Association{}, fmt.Errorf("trb/orm association %s through %s is ambiguous; specify source", source.Name, spec.Through)
			}
			sourceName = candidate.Name
		}
	}
	via, ok := middle.Association(sourceName)
	if !ok || via.TargetModel != spec.TargetModel {
		return Association{}, fmt.Errorf("trb/orm association %s through %s has no source %s to %s", source.Name, spec.Through, sourceName, spec.TargetModel)
	}
	name := spec.Name
	if name == "" {
		if spec.Kind == HasMany {
			name = models[spec.TargetModel].Table
		} else {
			name = modelBaseName(spec.TargetModel)
		}
	}
	return Association{
		Name: name, Kind: spec.Kind, TargetModel: spec.TargetModel, TargetQuery: models[spec.TargetModel].QueryType,
		Inverse: spec.Inverse, Through: spec.Through, Source: sourceName, Dependent: spec.Dependent, Scoped: spec.Scoped,
		Preloadable: source.ModulePath == middle.ModulePath && middle.ModulePath == models[spec.TargetModel].ModulePath && through.Preloadable && via.Preloadable,
	}, nil
}

func resolveAssociationInverses(source *Model, models map[string]*Model) error {
	for index := range source.Associations {
		association := &source.Associations[index]
		target := models[association.TargetModel]
		if target == nil || association.Through != "" {
			continue
		}
		if association.Inverse != "" {
			inverse, ok := target.Association(association.Inverse)
			if !ok || inverse.TargetModel != source.Name || inverse.Through != "" {
				return fmt.Errorf("trb/orm association %s.%s inverse %s does not reference %s", source.Name, association.Name, association.Inverse, source.Name)
			}
			continue
		}
		inferred := ""
		for _, candidate := range target.Associations {
			if candidate.TargetModel != source.Name || candidate.Through != "" || candidate.SourceColumn != association.TargetColumn || candidate.TargetColumn != association.SourceColumn {
				continue
			}
			if inferred != "" {
				inferred = ""
				break
			}
			inferred = candidate.Name
		}
		association.Inverse = inferred
	}
	return nil
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
