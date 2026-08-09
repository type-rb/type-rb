package orm

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Model struct {
	Name              string
	QueryType         string
	Table             string
	ModulePath        string
	Columns           []Column
	Associations      []Association
	UniqueConstraints []UniqueConstraint
}

type AssociationKind string

const (
	BelongsTo AssociationKind = "belongs_to"
	HasMany   AssociationKind = "has_many"
	HasOne    AssociationKind = "has_one"
)

type Association struct {
	Name                string
	Kind                AssociationKind
	TargetModel         string
	TargetQuery         string
	SourceColumn        string
	TargetColumn        string
	Preloadable         bool
	CardinalityVerified bool
}

func (m Model) DraftType() string { return m.Name + "Draft" }

func (m Model) ChangesType() string { return m.Name + "Changes" }

func (m Model) ScopeType() string { return m.Name + "Scope" }

func (m Model) PrimaryKey() (Column, bool) {
	var result Column
	found := false
	for _, column := range m.Columns {
		if !column.PrimaryKey {
			continue
		}
		if found {
			return Column{}, false
		}
		result = column
		found = true
	}
	return result, found
}

func (m Model) BatchKey() (Column, bool) {
	primaryKey, ok := m.PrimaryKey()
	if !ok {
		return Column{}, false
	}
	switch primaryKey.Type.Kind {
	case types.Int, types.Float, types.String:
		primaryKey.Type.Nullable = false
		return primaryKey, true
	default:
		return Column{}, false
	}
}

func (m Model) Association(name string) (Association, bool) {
	for _, association := range m.Associations {
		if association.Name == name {
			return association, true
		}
	}
	return Association{}, false
}

func (m Model) Column(name string) (Column, bool) {
	for _, column := range m.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return Column{}, false
}

func AggregateResultType(operation string, column Column) (types.Type, bool) {
	result := column.Type
	switch operation {
	case "sum":
		if result.Kind != types.Int && result.Kind != types.Float {
			return types.Type{}, false
		}
		result.Nullable = false
	case "average":
		if result.Kind != types.Int && result.Kind != types.Float {
			return types.Type{}, false
		}
		result = types.FromName("Float")
		result.Nullable = true
	case "minimum", "maximum":
		switch result.Kind {
		case types.Int, types.Float, types.String:
			result.Nullable = true
		default:
			return types.Type{}, false
		}
	default:
		return types.Type{}, false
	}
	return result, true
}

func AggregateOperations() []string {
	return []string{"sum", "average", "minimum", "maximum"}
}

func associationValueType(association Association) types.Type {
	if association.Kind == HasMany {
		return arrayOf(association.TargetModel)
	}
	result := types.FromName(association.TargetModel)
	result.Nullable = true
	return result
}

type Manifest struct {
	Adapter             string
	Database            string
	DatabaseEnvironment string
	Models              []Model
}

func Analyze(programs []*ast.Program, projectRoot string, options map[string][]byte) (*Manifest, error) {
	schema, err := LoadSchema(projectRoot, options)
	if err != nil {
		return nil, err
	}
	models, err := discoverModels(programs, schema)
	if err != nil {
		return nil, err
	}
	return &Manifest{
		Adapter: schema.Adapter, Database: schema.Database, DatabaseEnvironment: schema.DatabaseEnvironment, Models: models,
	}, nil
}

func (*Manifest) ExtensionName() string { return ProjectProvider }

func (m *Manifest) Augment(program *ir.Program) {
	if m == nil || program == nil {
		return
	}
	ensureRuntimeTypes(program)
	if program.ModulePath == "trb/orm/index" {
		for _, statement := range program.Statements {
			class, ok := statement.(*ir.Class)
			if ok && (class.Name == "Database" || class.Name == "Transaction") {
				class.External = true
			}
		}
	}
	for _, model := range m.Models {
		if model.ModulePath != program.ModulePath {
			continue
		}
		for _, statement := range program.Statements {
			class, ok := statement.(*ir.Class)
			if !ok || class.Name != model.Name {
				continue
			}
			existing := map[string]bool{}
			for _, member := range class.Body {
				switch node := member.(type) {
				case *ir.Field:
					existing[strings.TrimPrefix(node.Name, "@")] = true
				case *ir.Method:
					existing[node.Name] = true
				}
			}
			class.Body = append(class.Body, &ir.Field{Name: ormQueryScopeField(), Type: types.FromName(model.QueryType)})
			for _, column := range model.Columns {
				if !existing[column.Name] {
					class.Body = append(class.Body, &ir.Field{Name: "@" + column.Name, Type: column.Type})
				}
			}
			if _, ok := model.PrimaryKey(); ok {
				if !existing["with"] {
					class.Body = append(class.Body, withIRMethod(model))
				}
				if !existing["update"] {
					class.Body = append(class.Body, updateIRMethod(model))
				}
				if !existing["delete"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "delete", External: true, ReturnType: dbResult(types.FromName("Boolean")),
					})
				}
			}
			for _, association := range model.Associations {
				if association.Preloadable && !existing[association.Name] {
					class.Body = append(class.Body,
						&ir.Field{Name: associationValueField(association.Name), Type: types.FromName("Any")},
						&ir.Field{Name: associationLoadedField(association.Name), Type: types.FromName("Boolean")},
					)
					class.Body = append(class.Body, &ir.Method{
						Name: association.Name, External: true,
						ReturnType: associationValueType(association),
					})
				}
				if !existing[association.Name+"_query"] {
					class.Body = append(class.Body, &ir.Method{Name: association.Name + "_query", External: true, ReturnType: types.FromName(association.TargetQuery)})
				}
			}
			if !existing["where"] {
				class.Body = append(class.Body, whereIRMethod(model, true))
			}
			if !existing["select"] {
				class.Body = append(class.Body, selectIRMethod(model, true))
			}
			if !existing["join"] {
				if join := joinIRMethod(model, "join", true); join != nil {
					class.Body = append(class.Body, join)
				}
			}
			if !existing["left_join"] {
				if join := joinIRMethod(model, "left_join", true); join != nil {
					class.Body = append(class.Body, join)
				}
			}
			if !existing["using"] {
				class.Body = append(class.Body, &ir.Method{
					Name: "using", External: true, Class: true,
					Parameters: []ir.Parameter{{Name: "transaction", Type: types.FromName("Transaction")}},
					ReturnType: types.FromName(model.ScopeType()),
				})
			}
			if !existing["not"] {
				class.Body = append(class.Body, notIRMethod(model, true))
			}
			if !existing["find_by"] {
				class.Body = append(class.Body, findByIRMethod(model, true))
			}
			if !existing["exists?"] {
				class.Body = append(class.Body, existsIRMethod(model, true))
			}
			if !existing["pluck"] {
				class.Body = append(class.Body, projectionIRMethod(model, "pluck", true, false))
			}
			if !existing["pick"] {
				class.Body = append(class.Body, projectionIRMethod(model, "pick", true, true))
			}
			for _, operation := range AggregateOperations() {
				if !existing[operation] {
					if aggregate := aggregateIRMethod(model, operation, true); aggregate != nil {
						class.Body = append(class.Body, aggregate)
					}
				}
			}
			if primaryKey, ok := model.PrimaryKey(); ok {
				keyType := primaryKey.Type
				keyType.Nullable = false
				findType := namedType(model.Name)
				findType.Nullable = true
				if !existing["find"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "find", External: true, Class: true,
						Parameters: []ir.Parameter{{Name: primaryKey.Name, Type: keyType}}, ReturnType: dbResult(findType),
					})
				}
				if !existing["ids"] {
					class.Body = append(class.Body, idsIRMethod(model, true, primaryKey))
				}
				if !existing["create"] {
					class.Body = append(class.Body, createIRMethod(model))
				}
				if !existing["build"] {
					class.Body = append(class.Body, buildIRMethod(model))
				}
				if !existing["insert_all"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "insert_all", External: true, Class: true,
						Parameters: []ir.Parameter{{Name: "drafts", Type: arrayOf(model.DraftType())}},
						ReturnType: dbResult(types.FromName("Integer")),
					})
				}
				if !existing["insert_if_absent"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "insert_if_absent", External: true, Class: true,
						Parameters: []ir.Parameter{
							{Name: "draft", Type: namedType(model.DraftType())}, uniqueByIRParameter(model),
						},
						ReturnType: dbResult(types.FromName("Boolean")),
					})
				}
				if !existing["upsert_all"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "upsert_all", External: true, Class: true,
						Parameters: []ir.Parameter{
							{Name: "drafts", Type: arrayOf(model.DraftType())},
							uniqueByIRParameter(model), updateColumnsIRParameter(model),
						},
						ReturnType: dbResult(types.FromName("Integer")),
					})
				}
			}
			if _, ok := model.BatchKey(); ok {
				for _, name := range []string{"find_each", "find_in_batches"} {
					if !existing[name] {
						class.Body = append(class.Body, batchIRMethod(name, true))
					}
				}
			}
		}
		program.Statements = append(program.Statements, &ir.Class{Name: model.QueryType, External: true, Body: queryIRMethods(model)})
		program.Statements = append(program.Statements, &ir.Class{Name: model.ScopeType(), External: true, Body: scopeIRMethods(model)})
		if _, ok := model.PrimaryKey(); ok {
			program.Statements = append(program.Statements, &ir.Class{
				Name: model.DraftType(), External: true,
				Body: []ir.Statement{
					&ir.Method{Name: "save", External: true, ReturnType: dbResult(namedType(model.Name))},
					&ir.Method{
						Name: "upsert", External: true,
						Parameters: []ir.Parameter{uniqueByIRParameter(model), updateColumnsIRParameter(model)},
						ReturnType: dbResult(namedType(model.Name)),
					},
				},
			})
			program.Statements = append(program.Statements, &ir.Class{
				Name: model.ChangesType(), External: true,
				Body: []ir.Statement{&ir.Method{Name: "save", External: true, ReturnType: dbResult(namedType(model.Name))}},
			})
		}
	}
}

func scopeIRMethods(model Model) []ir.Statement {
	methods := queryIRMethods(model)
	if primaryKey, ok := model.PrimaryKey(); ok {
		keyType := primaryKey.Type
		keyType.Nullable = false
		findType := namedType(model.Name)
		findType.Nullable = true
		build := writeIRMethod(model, "build", namedType(model.DraftType()))
		build.Class = false
		create := writeIRMethod(model, "create", dbResult(namedType(model.Name)))
		create.Class = false
		methods = append(methods,
			&ir.Method{
				Name: "find", External: true,
				Parameters: []ir.Parameter{{Name: primaryKey.Name, Type: keyType}},
				ReturnType: dbResult(findType),
			},
			build,
			create,
		)
	}
	return methods
}

func uniqueByIRParameter(model Model) ir.Parameter {
	return ir.Parameter{
		Name: "unique_by", Type: arrayOf("String"), Keyword: true,
		LiteralArrays: copyUniqueColumns(uniqueColumnSets(model)),
	}
}

func updateColumnsIRParameter(model Model) ir.Parameter {
	declared := updateColumnsDeclarationParameter(model)
	return ir.Parameter{
		Name: "update", Type: declared.Type, Keyword: true,
		LiteralArrayElements: append([]string(nil), declared.LiteralArrayElements...),
	}
}

func createIRMethod(model Model) *ir.Method {
	return writeIRMethod(model, "create", dbResult(namedType(model.Name)))
}

func buildIRMethod(model Model) *ir.Method {
	return writeIRMethod(model, "build", namedType(model.DraftType()))
}

func writeIRMethod(model Model, name string, returnType types.Type) *ir.Method {
	method := &ir.Method{Name: name, External: true, Class: true, ReturnType: returnType}
	for _, column := range model.Columns {
		method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
	}
	return method
}

func updateIRMethod(model Model) *ir.Method {
	return modelChangeIRMethod(model, "update", dbResult(namedType(model.Name)))
}

func withIRMethod(model Model) *ir.Method {
	return modelChangeIRMethod(model, "with", namedType(model.ChangesType()))
}

func modelChangeIRMethod(model Model, name string, returnType types.Type) *ir.Method {
	method := &ir.Method{Name: name, External: true, ReturnType: returnType}
	for _, column := range model.Columns {
		if !column.PrimaryKey {
			method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
		}
	}
	return method
}

func relationUpdateAllIRMethod(model Model) *ir.Method {
	method := &ir.Method{Name: "update_all", External: true, ReturnType: dbResult(types.FromName("Integer"))}
	for _, column := range model.Columns {
		if !column.PrimaryKey && !column.Generated {
			method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
		}
	}
	return method
}

func queryIRMethods(model Model) []ir.Statement {
	where := whereIRMethod(model, false)
	not := notIRMethod(model, false)
	order := &ir.Method{Name: "order", External: true, ReturnType: namedType(model.QueryType)}
	for _, column := range model.Columns {
		order.Parameters = append(order.Parameters, ir.Parameter{
			Name: column.Name, Type: types.FromName("String"), Keyword: true,
			LiteralValues: []string{"asc", "desc"},
		})
	}
	firstType := namedType(model.Name)
	firstType.Nullable = true
	methods := []ir.Statement{
		where,
		not,
		&ir.Method{Name: "or", External: true, Parameters: []ir.Parameter{{Name: "other", Type: namedType(model.QueryType)}}, ReturnType: namedType(model.QueryType)},
		findByIRMethod(model, false),
		&ir.Method{Name: "exists?", External: true, ReturnType: dbResult(types.FromName("Boolean"))},
		relationUpdateAllIRMethod(model),
		&ir.Method{Name: "delete_all", External: true, ReturnType: dbResult(types.FromName("Integer"))},
		projectionIRMethod(model, "pluck", false, false),
		projectionIRMethod(model, "pick", false, true),
		order,
		&ir.Method{Name: "limit", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "offset", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "lock", External: true, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "all", External: true, ReturnType: dbResult(arrayOf(model.Name))},
		&ir.Method{Name: "first", External: true, ReturnType: dbResult(firstType)},
		&ir.Method{Name: "count", External: true, ReturnType: dbResult(types.FromName("Integer"))},
		&ir.Method{Name: "to_sql", External: true, ReturnType: types.FromName("String")},
		&ir.Method{Name: "explain", External: true, ReturnType: dbResult(types.FromName("String"))},
	}
	methods = append(methods, selectIRMethod(model, false))
	if join := joinIRMethod(model, "join", false); join != nil {
		methods = append(methods, join)
	}
	if join := joinIRMethod(model, "left_join", false); join != nil {
		methods = append(methods, join)
	}
	for _, operation := range AggregateOperations() {
		if aggregate := aggregateIRMethod(model, operation, false); aggregate != nil {
			methods = append(methods, aggregate)
		}
	}
	if preload := preloadIRMethod(model); preload != nil {
		methods = append(methods, preload)
	}
	if primaryKey, ok := model.PrimaryKey(); ok {
		methods = append(methods, idsIRMethod(model, false, primaryKey))
	}
	if _, ok := model.BatchKey(); ok {
		methods = append(methods, batchIRMethod("find_each", false), batchIRMethod("find_in_batches", false))
	}
	return methods
}

func joinIRMethod(model Model, name string, class bool) *ir.Method {
	method := &ir.Method{Name: name, External: true, Class: class, ReturnType: namedType(model.QueryType)}
	for _, association := range model.Associations {
		associationParameter := ir.Parameter{
			Name: "association", Type: types.FromName("String"), LiteralValues: []string{association.Name},
		}
		method.Alternatives = append(method.Alternatives,
			ir.MethodSignature{
				Parameters: []ir.Parameter{associationParameter}, ReturnType: namedType(model.QueryType),
			},
			ir.MethodSignature{
				Parameters: []ir.Parameter{
					associationParameter,
					{Name: "query", Type: types.FromName(association.TargetQuery)},
				},
				ReturnType: namedType(model.QueryType),
			},
		)
	}
	if len(method.Alternatives) == 0 {
		return nil
	}
	return method
}

func ensureRuntimeTypes(program *ir.Program) {
	for _, statement := range program.Statements {
		imported, ok := statement.(*ir.Import)
		if !ok || imported.Path != "trb/orm/index" {
			continue
		}
		imported.RuntimeRequired = true
		for _, symbol := range []string{"DbError", "DbErrorKind", "DbResult"} {
			if !contains(imported.Symbols, symbol) {
				imported.Symbols = append(imported.Symbols, symbol)
			}
		}
		return
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func preloadIRMethod(model Model) *ir.Method {
	var values []string
	for _, association := range model.Associations {
		if association.Preloadable {
			values = append(values, association.Name)
		}
	}
	if len(values) == 0 {
		return nil
	}
	return &ir.Method{
		Name: "preload", External: true, ReturnType: namedType(model.QueryType),
		Parameters: []ir.Parameter{{
			Name: "association", Type: types.FromName("String"), LiteralValues: values,
		}},
	}
}

func associationValueField(name string) string {
	return "@__trb_association_" + name
}

func associationLoadedField(name string) string {
	return "@__trb_association_" + name + "_loaded"
}

func ormQueryScopeField() string {
	return "@__trb_orm_query_scope"
}

func whereIRMethod(model Model, class bool) *ir.Method {
	method := &ir.Method{Name: "where", External: true, Class: class, ReturnType: namedType(model.QueryType)}
	for _, column := range model.Columns {
		method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: predicateValueType(column), Keyword: true})
		for _, signature := range comparisonSignatures(column, model.QueryType) {
			alternative := ir.MethodSignature{ReturnType: signature.Return, Variadic: signature.Variadic}
			for _, parameter := range signature.Parameters {
				alternative.Parameters = append(alternative.Parameters, ir.Parameter{
					Name: parameter.Name, Type: parameter.Type, Keyword: parameter.Keyword,
					LiteralValues:        append([]string(nil), parameter.LiteralValues...),
					LiteralArrays:        copyUniqueColumns(parameter.LiteralArrays),
					LiteralArrayElements: append([]string(nil), parameter.LiteralArrayElements...),
				})
			}
			method.Alternatives = append(method.Alternatives, alternative)
		}
	}
	return method
}

func notIRMethod(model Model, class bool) *ir.Method {
	return predicateIRMethod(model, "not", class, namedType(model.QueryType))
}

func findByIRMethod(model Model, class bool) *ir.Method {
	result := namedType(model.Name)
	result.Nullable = true
	return predicateIRMethod(model, "find_by", class, dbResult(result))
}

func existsIRMethod(model Model, class bool) *ir.Method {
	return predicateIRMethod(model, "exists?", class, dbResult(types.FromName("Boolean")))
}

func predicateIRMethod(model Model, name string, class bool, result types.Type) *ir.Method {
	method := &ir.Method{Name: name, External: true, Class: class, ReturnType: result}
	for _, column := range model.Columns {
		method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: predicateValueType(column), Keyword: true})
	}
	return method
}

func projectionIRMethod(model Model, name string, class, pick bool) *ir.Method {
	declared := projectionDeclaration(model, name, "", class, pick)
	method := &ir.Method{Name: name, External: true, Class: class, ReturnType: declared.Return}
	for _, parameter := range declared.Parameters {
		method.Parameters = append(method.Parameters, ir.Parameter{
			Name: parameter.Name, Type: parameter.Type, Keyword: parameter.Keyword,
			LiteralValues: append([]string(nil), parameter.LiteralValues...),
		})
	}
	for _, signature := range declared.Alternatives {
		alternative := ir.MethodSignature{ReturnType: signature.Return}
		for _, parameter := range signature.Parameters {
			alternative.Parameters = append(alternative.Parameters, ir.Parameter{
				Name: parameter.Name, Type: parameter.Type, LiteralValues: append([]string(nil), parameter.LiteralValues...),
			})
		}
		method.Alternatives = append(method.Alternatives, alternative)
	}
	return method
}

func selectIRMethod(model Model, class bool) *ir.Method {
	declared := selectDeclaration(model, "", class)
	method := &ir.Method{Name: "select", External: true, Class: class, ReturnType: declared.Return}
	for _, signature := range declared.Alternatives {
		alternative := ir.MethodSignature{ReturnType: signature.Return}
		for _, parameter := range signature.Parameters {
			alternative.Parameters = append(alternative.Parameters, ir.Parameter{
				Name: parameter.Name, Type: parameter.Type, LiteralValues: append([]string(nil), parameter.LiteralValues...),
			})
		}
		method.Alternatives = append(method.Alternatives, alternative)
	}
	return method
}

func aggregateIRMethod(model Model, operation string, class bool) *ir.Method {
	declared, ok := aggregateDeclaration(model, operation, "", class)
	if !ok {
		return nil
	}
	method := &ir.Method{Name: operation, External: true, Class: class, ReturnType: declared.Return}
	for _, parameter := range declared.Parameters {
		method.Parameters = append(method.Parameters, ir.Parameter{
			Name: parameter.Name, Type: parameter.Type,
			LiteralValues: append([]string(nil), parameter.LiteralValues...),
		})
	}
	for _, signature := range declared.Alternatives {
		alternative := ir.MethodSignature{ReturnType: signature.Return}
		for _, parameter := range signature.Parameters {
			alternative.Parameters = append(alternative.Parameters, ir.Parameter{
				Name: parameter.Name, Type: parameter.Type, LiteralValues: append([]string(nil), parameter.LiteralValues...),
			})
		}
		method.Alternatives = append(method.Alternatives, alternative)
	}
	return method
}

func idsIRMethod(model Model, class bool, primaryKey Column) *ir.Method {
	declared := idsDeclaration(model, "", class, primaryKey)
	return &ir.Method{Name: "ids", External: true, Class: class, ReturnType: declared.Return}
}

func copyUniqueColumns(values [][]string) [][]string {
	result := make([][]string, len(values))
	for index, value := range values {
		result[index] = append([]string(nil), value...)
	}
	return result
}

func batchIRMethod(name string, class bool) *ir.Method {
	return &ir.Method{
		Name: name, External: true, Class: class,
		Parameters: []ir.Parameter{{Name: "batch_size", Type: types.FromName("Integer"), Keyword: true}},
		ReturnType: dbResult(types.FromName("Integer")),
	}
}

func ManifestFrom(extensions []ir.Extension) *Manifest {
	for _, extension := range extensions {
		if manifest, ok := extension.(*Manifest); ok {
			return manifest
		}
	}
	return nil
}

func (m *Manifest) Model(name string) (Model, bool) {
	if m == nil {
		return Model{}, false
	}
	for _, model := range m.Models {
		if model.Name == name {
			return model, true
		}
	}
	return Model{}, false
}

func (m *Manifest) QueryModel(name string) (Model, bool) {
	if m == nil {
		return Model{}, false
	}
	for _, model := range m.Models {
		if model.QueryType == name {
			return model, true
		}
	}
	return Model{}, false
}

func (m *Manifest) DraftModel(name string) (Model, bool) {
	if m == nil {
		return Model{}, false
	}
	for _, model := range m.Models {
		if model.DraftType() == name {
			return model, true
		}
	}
	return Model{}, false
}

func (m *Manifest) ScopeModel(name string) (Model, bool) {
	if m == nil {
		return Model{}, false
	}
	for _, model := range m.Models {
		if model.ScopeType() == name {
			return model, true
		}
	}
	return Model{}, false
}

func (m *Manifest) ChangesModel(name string) (Model, bool) {
	if m == nil {
		return Model{}, false
	}
	for _, model := range m.Models {
		if model.ChangesType() == name {
			return model, true
		}
	}
	return Model{}, false
}

func (m *Manifest) ModelsForModule(modulePath string) []Model {
	var models []Model
	if m != nil {
		for _, model := range m.Models {
			if model.ModulePath == modulePath {
				models = append(models, model)
			}
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })
	return models
}

func namedType(name string) types.Type { return types.FromName(name) }
