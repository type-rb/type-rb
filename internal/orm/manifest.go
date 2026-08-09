package orm

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Model struct {
	Name       string
	QueryType  string
	Table      string
	ModulePath string
	Columns    []Column
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
			if !existing["where"] {
				method := &ir.Method{Name: "where", External: true, Class: true, ReturnType: namedType(model.QueryType)}
				for _, column := range model.Columns {
					method.Parameters = append(method.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
				}
				class.Body = append(class.Body, method)
			}
			if primaryKey, ok := model.PrimaryKey(); ok {
				keyType := primaryKey.Type
				keyType.Nullable = false
				findType := namedType(model.Name)
				findType.Nullable = true
				if !existing["find"] {
					class.Body = append(class.Body, &ir.Method{
						Name: "find", External: true, Class: true,
						Parameters: []ir.Parameter{{Name: primaryKey.Name, Type: keyType}}, ReturnType: findType,
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
	where := &ir.Method{Name: "where", External: true, ReturnType: namedType(model.QueryType)}
	order := &ir.Method{Name: "order", External: true, ReturnType: namedType(model.QueryType)}
	for _, column := range model.Columns {
		where.Parameters = append(where.Parameters, ir.Parameter{Name: column.Name, Type: column.Type, Keyword: true})
		order.Parameters = append(order.Parameters, ir.Parameter{Name: column.Name, Type: types.FromName("String"), Keyword: true})
	}
	firstType := namedType(model.Name)
	firstType.Nullable = true
	methods := []ir.Statement{
		where,
		order,
		&ir.Method{Name: "limit", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "offset", External: true, Parameters: []ir.Parameter{{Name: "count", Type: types.FromName("Integer")}}, ReturnType: namedType(model.QueryType)},
		&ir.Method{Name: "all", External: true, ReturnType: arrayOf(model.Name)},
		&ir.Method{Name: "first", External: true, ReturnType: firstType},
		&ir.Method{Name: "count", External: true, ReturnType: types.FromName("Integer")},
		&ir.Method{Name: "to_sql", External: true, ReturnType: types.FromName("String")},
		&ir.Method{Name: "explain", External: true, ReturnType: types.FromName("String")},
	}
	if _, ok := model.BatchKey(); ok {
		methods = append(methods, batchIRMethod("find_each", false), batchIRMethod("find_in_batches", false))
	}
	return methods
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
