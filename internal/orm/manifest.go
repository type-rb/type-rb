package orm

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Model struct {
	Name         string
	QueryType    string
	Table        string
	ModulePath   string
	Columns      []Column
	Associations []Association
}

type AssociationKind string

const (
	BelongsTo AssociationKind = "belongs_to"
	HasMany   AssociationKind = "has_many"
)

type Association struct {
	Name         string
	Kind         AssociationKind
	TargetModel  string
	TargetQuery  string
	SourceColumn string
	TargetColumn string
	Preloadable  bool
}

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

func associationValueType(association Association) types.Type {
	if association.Kind == HasMany {
		return arrayOf(association.TargetModel)
	}
	result := types.FromName(association.TargetModel)
	result.Nullable = true
	return result
}

type Manifest struct {
	Adapter  string
	Database string
	Models   []Model
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
	return &Manifest{Adapter: schema.Adapter, Database: schema.Database, Models: models}, nil
}

func (*Manifest) ExtensionName() string { return ProjectProvider }

func (m *Manifest) Augment(program *ir.Program) {
	if m == nil || program == nil {
		return
	}
	ensureRuntimeTypes(program)
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
			for _, column := range model.Columns {
				if !existing[column.Name] {
					class.Body = append(class.Body, &ir.Field{Name: "@" + column.Name, Type: column.Type})
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
	}
}

func queryIRMethods(model Model) []ir.Statement {
	where := whereIRMethod(model, false)
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
		order,
		&ir.Method{Name: "limit", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "offset", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "all", External: true, ReturnType: dbResult(arrayOf(model.Name))},
		&ir.Method{Name: "first", External: true, ReturnType: dbResult(firstType)},
		&ir.Method{Name: "count", External: true, ReturnType: dbResult(types.FromName("Integer"))},
		&ir.Method{Name: "to_sql", External: true, ReturnType: types.FromName("String")},
		&ir.Method{Name: "explain", External: true, ReturnType: dbResult(types.FromName("String"))},
	}
	if preload := preloadIRMethod(model); preload != nil {
		methods = append(methods, preload)
	}
	if _, ok := model.BatchKey(); ok {
		methods = append(methods, batchIRMethod("find_each", false), batchIRMethod("find_in_batches", false))
	}
	return methods
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

func whereIRMethod(model Model, class bool) *ir.Method {
	method := &ir.Method{Name: "where", External: true, Class: class, ReturnType: namedType(model.QueryType)}
	for _, column := range model.Columns {
		method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
		for _, signature := range comparisonSignatures(column, model.QueryType) {
			alternative := ir.MethodSignature{ReturnType: signature.Return, Variadic: signature.Variadic}
			for _, parameter := range signature.Parameters {
				alternative.Parameters = append(alternative.Parameters, ir.Parameter{
					Name: parameter.Name, Type: parameter.Type, Keyword: parameter.Keyword,
					LiteralValues: append([]string(nil), parameter.LiteralValues...),
				})
			}
			method.Alternatives = append(method.Alternatives, alternative)
		}
	}
	return method
}

func batchIRMethod(name string, class bool) *ir.Method {
	return &ir.Method{
		Name: name, External: true, Class: class,
		Parameters: []ir.Parameter{{Name: "batch_size", Type: types.FromName("Integer"), Keyword: true}},
		ReturnType: types.FromName("Void"),
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
