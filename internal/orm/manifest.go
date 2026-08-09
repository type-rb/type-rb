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
		}
		program.Statements = append(program.Statements, &ir.Class{
			Name: model.QueryType, External: true,
			Body: []ir.Statement{&ir.Method{Name: "all", External: true, ReturnType: arrayOf(model.Name)}},
		})
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
