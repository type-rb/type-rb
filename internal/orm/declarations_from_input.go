package orm

import (
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func discoverDeclarationModels(input packageextension.ProjectDeclarationInput, schema *Schema) ([]Model, error) {
	var models []Model
	seen := map[string]bool{}
	classes := map[string]packageextension.ProjectClass{}
	enums, err := discoverDeclarationORMEnums(input)
	if err != nil {
		return nil, err
	}
	for _, module := range input.Modules {
		for _, class := range module.Classes {
			if class.Superclass == nil || class.Superclass.Authored.Kind != "named" || class.Superclass.Authored.Name != "Model" {
				continue
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
			model := Model{
				Name: class.Name, QueryType: class.Name + "Query", Table: table.Name,
				ModulePath:        module.ModulePath,
				Columns:           append([]Column(nil), table.Columns...),
				UniqueConstraints: append([]UniqueConstraint(nil), table.UniqueConstraints...),
			}
			if err := applyDeclarationEnumColumns(&model, class, module, enums); err != nil {
				return nil, err
			}
			models = append(models, model)
		}
	}
	byName := map[string]*Model{}
	for index := range models {
		byName[models[index].Name] = &models[index]
	}
	specs := map[string][]associationSpec{}
	for index := range models {
		associations, err := discoverDeclarationAssociationSpecs(models[index], classes[models[index].Name], byName, classes)
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

type declarationORMEnumDefinition struct {
	module  string
	enum    packageextension.ProjectEnum
	mapping *EnumColumn
}

func discoverDeclarationORMEnums(input packageextension.ProjectDeclarationInput) (map[string]map[string]*declarationORMEnumDefinition, error) {
	result := map[string]map[string]*declarationORMEnumDefinition{}
	for _, source := range input.Modules {
		module := result[source.ModulePath]
		if module == nil {
			module = map[string]*declarationORMEnumDefinition{}
			result[source.ModulePath] = module
		}
		for _, enum := range source.Enums {
			if module[enum.Name] != nil {
				return nil, fmt.Errorf("trb/orm enum %s is declared more than once in %s", enum.Name, source.ModulePath)
			}
			module[enum.Name] = &declarationORMEnumDefinition{module: source.ModulePath, enum: enum}
		}
	}
	return result, nil
}

func applyDeclarationEnumColumns(model *Model, class packageextension.ProjectClass, module packageextension.ProjectModule, enums map[string]map[string]*declarationORMEnumDefinition) error {
	seen := map[string]bool{}
	for _, directive := range class.Directives {
		if directive.Name != "enum_column" || directive.Block != nil {
			continue
		}
		if len(directive.Arguments) != 2 || directive.Arguments[0].Name != "" || directive.Arguments[1].Name != "" {
			return fmt.Errorf("trb/orm %s.enum_column expects a column name and enum type", model.Name)
		}
		columnName, ok := declarationOptionValue(directive.Arguments[0].Value)
		if !ok {
			return fmt.Errorf("trb/orm %s.enum_column column must be a symbol or string literal", model.Name)
		}
		if seen[columnName] {
			return fmt.Errorf("trb/orm model %s maps enum column %s more than once", model.Name, columnName)
		}
		seen[columnName] = true
		columnIndex := -1
		for index := range model.Columns {
			if model.Columns[index].Name == columnName {
				columnIndex = index
				break
			}
		}
		if columnIndex < 0 {
			return fmt.Errorf("trb/orm model %s has no column %s for enum_column", model.Name, columnName)
		}
		enumName := declarationReferenceName(directive.Arguments[1].Value)
		if enumName == "" {
			return fmt.Errorf("trb/orm %s.enum_column enum type must be an enum name", model.Name)
		}
		definition := resolveDeclarationORMEnum(module, enumName, enums)
		if definition == nil {
			return fmt.Errorf("trb/orm %s.enum_column references unknown enum %s", model.Name, enumName)
		}
		if definition.mapping == nil {
			mapping, err := buildDeclarationORMEnumMapping(definition.module, definition.enum)
			if err != nil {
				return fmt.Errorf("trb/orm %s.enum_column %s: %w", model.Name, columnName, err)
			}
			definition.mapping = mapping
		}
		column := &model.Columns[columnIndex]
		if column.Type.Kind != definition.mapping.StorageType.Kind {
			return fmt.Errorf("trb/orm %s.enum_column %s uses %s storage, but the schema column is %s", model.Name, columnName, definition.mapping.StorageType, column.Type)
		}
		copy := *definition.mapping
		copy.StorageType.Nullable = column.Nullable
		column.Enum = &copy
		column.Type = types.FromName(definition.enum.Name)
		column.Type.Nullable = column.Nullable
	}
	return nil
}

func resolveDeclarationORMEnum(module packageextension.ProjectModule, name string, enums map[string]map[string]*declarationORMEnumDefinition) *declarationORMEnumDefinition {
	if local := enums[module.ModulePath][name]; local != nil {
		return local
	}
	for _, imported := range module.Imports {
		for _, symbol := range imported.Symbols {
			if symbol != name {
				continue
			}
			if definition := enums[imported.ModulePath][name]; definition != nil {
				return definition
			}
			return enums[path.Join(imported.ModulePath, "index")][name]
		}
	}
	return nil
}

func buildDeclarationORMEnumMapping(module string, enum packageextension.ProjectEnum) (*EnumColumn, error) {
	if len(enum.TypeParameters) > 0 {
		return nil, fmt.Errorf("enum %s must not be generic", enum.Name)
	}
	raw := false
	for _, member := range enum.Members {
		if len(member.Parameters) > 0 {
			return nil, fmt.Errorf("enum %s must contain only payloadless members", enum.Name)
		}
		raw = raw || member.RawValue != nil
	}
	if len(enum.Members) == 0 {
		return nil, fmt.Errorf("enum %s has no members", enum.Name)
	}
	mapping := &EnumColumn{Name: enum.Name, ModulePath: module, StorageType: types.FromName("String")}
	seen := map[string]string{}
	for _, member := range enum.Members {
		value := EnumColumnValue{Name: member.Name, StringValue: EnumMemberStorageName(member.Name)}
		key := "string:" + value.StringValue
		if raw {
			if member.RawValue == nil {
				return nil, fmt.Errorf("raw-value enum %s requires a raw value for every member", enum.Name)
			}
			storageType, stringValue, integerValue, ok := declarationORMEnumRawValue(*member.RawValue)
			if !ok {
				return nil, fmt.Errorf("raw-value enum %s member %s must use a String or Integer literal", enum.Name, member.Name)
			}
			if len(mapping.Values) == 0 {
				mapping.StorageType = storageType
			} else if mapping.StorageType.Kind != storageType.Kind {
				return nil, fmt.Errorf("raw-value enum %s must use one storage type", enum.Name)
			}
			value.StringValue = stringValue
			value.IntegerValue = integerValue
			if storageType.Kind == types.Int {
				key = "integer:" + strconv.FormatInt(integerValue, 10)
			} else {
				key = "string:" + stringValue
			}
		}
		if previous := seen[key]; previous != "" {
			return nil, fmt.Errorf("enum %s members %s and %s use the same storage value", enum.Name, previous, member.Name)
		}
		seen[key] = member.Name
		mapping.Values = append(mapping.Values, value)
	}
	return mapping, nil
}

func declarationORMEnumRawValue(value packageextension.ProjectValue) (types.Type, string, int64, bool) {
	switch value.Kind {
	case "string":
		decoded, err := strconv.Unquote(value.Raw)
		return types.FromName("String"), decoded, 0, err == nil
	case "integer":
		parsed, err := strconv.ParseInt(strings.ReplaceAll(value.Raw, "_", ""), 10, 64)
		return types.FromName("Integer"), "", parsed, err == nil
	default:
		return types.Type{}, "", 0, false
	}
}

func discoverDeclarationAssociationSpecs(source Model, class packageextension.ProjectClass, models map[string]*Model, classes map[string]packageextension.ProjectClass) ([]associationSpec, error) {
	var result []associationSpec
	seen := map[string]bool{}
	for _, directive := range class.Directives {
		if directive.Name != string(BelongsTo) && directive.Name != string(HasMany) && directive.Name != string(HasOne) {
			continue
		}
		if len(directive.Arguments) == 0 || directive.Arguments[0].Name != "" {
			return nil, fmt.Errorf("trb/orm %s.%s expects a model type as its first argument", source.Name, directive.Name)
		}
		targetName := declarationReferenceName(directive.Arguments[0].Value)
		target := models[targetName]
		if target == nil {
			return nil, fmt.Errorf("trb/orm %s.%s references unknown model %s", source.Name, directive.Name, targetName)
		}
		if !sameModelGroup(source, *target) {
			targetClass := classes[targetName]
			targetSpan := importProjectSourceSpan(targetClass.Span)
			return nil, associationModelGroupError(source, *target, &targetSpan, directive.Name, importProjectSourceSpan(directive.Arguments[0].Span))
		}
		spec := associationSpec{Kind: AssociationKind(directive.Name), TargetModel: targetName}
		if directive.Block != nil {
			if len(directive.Block.Parameters) != 1 || directive.Block.StatementCount != 1 {
				return nil, fmt.Errorf("trb/orm %s.%s scope must have one query parameter and one result expression", source.Name, directive.Name)
			}
			if !directive.Block.ResultExpression {
				return nil, fmt.Errorf("trb/orm %s.%s scope must return one query expression", source.Name, directive.Name)
			}
			spec.Scoped = true
		}
		options := map[string]string{}
		for _, argument := range directive.Arguments[1:] {
			if argument.Name == "" {
				return nil, fmt.Errorf("trb/orm %s.%s accepts only one positional model type", source.Name, directive.Name)
			}
			if _, duplicate := options[argument.Name]; duplicate {
				return nil, fmt.Errorf("trb/orm %s.%s option %s is specified more than once", source.Name, directive.Name, argument.Name)
			}
			value, ok := declarationOptionValue(argument.Value)
			if !ok {
				return nil, fmt.Errorf("trb/orm %s.%s option %s must be a symbol or string literal", source.Name, directive.Name, argument.Name)
			}
			options[argument.Name] = value
		}
		for name := range options {
			if !associationOptionAllowed(spec.Kind, name) {
				return nil, fmt.Errorf("trb/orm %s.%s does not accept option %s", source.Name, directive.Name, name)
			}
		}
		spec.Name = options["name"]
		spec.ForeignKey = options["foreign_key"]
		spec.References = options["references"]
		spec.Inverse = options["inverse"]
		spec.Through = options["through"]
		spec.Source = options["source"]
		if spec.Source != "" && spec.Through == "" {
			return nil, fmt.Errorf("trb/orm %s.%s option source requires through", source.Name, directive.Name)
		}
		if dependent := options["dependent"]; dependent != "" {
			spec.Dependent = DependentAction(dependent)
			switch spec.Dependent {
			case DependentDestroy, DependentDelete, DependentNullify, DependentRestrict:
			default:
				return nil, fmt.Errorf("trb/orm %s.%s dependent must be destroy, delete, nullify, or restrict", source.Name, directive.Name)
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

func declarationOptionValue(value packageextension.ProjectValue) (string, bool) {
	switch value.Kind {
	case "symbol":
		return value.Name, true
	case "string":
		decoded, err := strconv.Unquote(value.Raw)
		return decoded, err == nil
	default:
		return "", false
	}
}

func declarationReferenceName(value packageextension.ProjectValue) string {
	if value.Kind == "reference" {
		return value.Name
	}
	return ""
}

func importProjectSourceSpan(source packageextension.SourceSpan) token.Span {
	return token.Span{
		Start: token.Position{Offset: source.Start.Offset, Line: source.Start.Line, Column: source.Start.Column},
		End:   token.Position{Offset: source.End.Offset, Line: source.End.Line, Column: source.End.Column},
	}
}
