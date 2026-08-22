package orm

import (
	"fmt"
	"strings"

	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

const DeclarationInputProtocolVersion = 1

// DeclarationInput is the complete data-only input consumed by the ORM
// declaration provider. Project source and schema metadata can cross a JSON
// boundary without exposing parser nodes, resolver state, database credentials,
// or filesystem access.
type DeclarationInput struct {
	ProtocolVersion int                                      `json:"protocolVersion"`
	Project         packageextension.ProjectDeclarationInput `json:"project"`
	Schema          DeclarationSchema                        `json:"schema"`
}

type DeclarationSchema struct {
	Adapter string             `json:"adapter"`
	Tables  []DeclarationTable `json:"tables,omitempty"`
}

type DeclarationTable struct {
	Name              string                        `json:"name"`
	Columns           []DeclarationColumn           `json:"columns,omitempty"`
	ForeignKeys       []DeclarationForeignKey       `json:"foreignKeys,omitempty"`
	UniqueConstraints []DeclarationUniqueConstraint `json:"uniqueConstraints,omitempty"`
}

type DeclarationColumn struct {
	Name       string                `json:"name"`
	Type       packageextension.Type `json:"type"`
	PrimaryKey bool                  `json:"primaryKey,omitempty"`
	HasDefault bool                  `json:"hasDefault,omitempty"`
	Generated  bool                  `json:"generated,omitempty"`
}

type DeclarationForeignKey struct {
	Column           string `json:"column"`
	ReferencedTable  string `json:"referencedTable"`
	ReferencedColumn string `json:"referencedColumn"`
}

type DeclarationUniqueConstraint struct {
	Columns []string `json:"columns"`
}

// ExportDeclarationInput copies compiler-owned schema state into the ORM
// provider boundary and combines it with the generic project declaration
// snapshot. Database connection details are deliberately omitted.
func ExportDeclarationInput(project packageextension.ProjectDeclarationInput, schema *Schema) (DeclarationInput, error) {
	result := DeclarationInput{ProtocolVersion: DeclarationInputProtocolVersion, Project: project}
	if schema == nil {
		return result, fmt.Errorf("trb/orm declaration input schema is missing")
	}
	result.Schema.Adapter = schema.Adapter
	for _, table := range schema.Tables {
		converted := DeclarationTable{Name: table.Name}
		for _, column := range table.Columns {
			converted.Columns = append(converted.Columns, DeclarationColumn{
				Name: column.Name, Type: exportDeclarationInputType(column.Type),
				PrimaryKey: column.PrimaryKey, HasDefault: column.HasDefault, Generated: column.Generated,
			})
		}
		for _, foreignKey := range table.ForeignKeys {
			converted.ForeignKeys = append(converted.ForeignKeys, DeclarationForeignKey{
				Column: foreignKey.Column, ReferencedTable: foreignKey.ReferencedTable, ReferencedColumn: foreignKey.ReferencedColumn,
			})
		}
		for _, constraint := range table.UniqueConstraints {
			converted.UniqueConstraints = append(converted.UniqueConstraints, DeclarationUniqueConstraint{
				Columns: append([]string(nil), constraint.Columns...),
			})
		}
		result.Schema.Tables = append(result.Schema.Tables, converted)
	}
	if err := ValidateDeclarationInput(result); err != nil {
		return DeclarationInput{}, err
	}
	return result, nil
}

func ValidateDeclarationInput(input DeclarationInput) error {
	if input.ProtocolVersion != DeclarationInputProtocolVersion {
		return fmt.Errorf("unsupported trb/orm declaration input protocol version %d", input.ProtocolVersion)
	}
	if err := packageextension.ValidateProjectDeclarationInput(input.Project); err != nil {
		return fmt.Errorf("trb/orm declaration project input: %w", err)
	}
	if input.Project.Provider != PackageName {
		return fmt.Errorf("trb/orm received project declaration input for provider %s", input.Project.Provider)
	}
	if _, err := AdapterFor(input.Schema.Adapter); err != nil {
		return fmt.Errorf("trb/orm declaration schema: %w", err)
	}
	seenTables := map[string]bool{}
	for _, table := range input.Schema.Tables {
		if strings.TrimSpace(table.Name) == "" || seenTables[table.Name] {
			return fmt.Errorf("trb/orm declaration schema contains an empty or duplicate table %q", table.Name)
		}
		seenTables[table.Name] = true
		seenColumns := map[string]bool{}
		for _, column := range table.Columns {
			if strings.TrimSpace(column.Name) == "" || seenColumns[column.Name] {
				return fmt.Errorf("trb/orm declaration schema table %s contains an empty or duplicate column %q", table.Name, column.Name)
			}
			seenColumns[column.Name] = true
			if err := validateDeclarationInputType(column.Type); err != nil {
				return fmt.Errorf("trb/orm declaration schema table %s column %s: %w", table.Name, column.Name, err)
			}
		}
		for _, foreignKey := range table.ForeignKeys {
			if strings.TrimSpace(foreignKey.Column) == "" || strings.TrimSpace(foreignKey.ReferencedTable) == "" || strings.TrimSpace(foreignKey.ReferencedColumn) == "" {
				return fmt.Errorf("trb/orm declaration schema table %s contains an incomplete foreign key", table.Name)
			}
		}
		for _, constraint := range table.UniqueConstraints {
			if len(constraint.Columns) == 0 {
				return fmt.Errorf("trb/orm declaration schema table %s contains an empty unique constraint", table.Name)
			}
			for _, column := range constraint.Columns {
				if strings.TrimSpace(column) == "" {
					return fmt.Errorf("trb/orm declaration schema table %s contains a unique constraint with an empty column", table.Name)
				}
			}
		}
	}
	return nil
}

func importDeclarationSchema(source DeclarationSchema) *Schema {
	result := &Schema{Adapter: source.Adapter}
	for _, table := range source.Tables {
		converted := Table{Name: table.Name}
		for position, column := range table.Columns {
			typ := importDeclarationInputType(column.Type)
			converted.Columns = append(converted.Columns, Column{
				Name: column.Name, Type: typ, Nullable: typ.Nullable, PrimaryKey: column.PrimaryKey,
				HasDefault: column.HasDefault, Generated: column.Generated, Position: position,
			})
		}
		for id, foreignKey := range table.ForeignKeys {
			converted.ForeignKeys = append(converted.ForeignKeys, ForeignKey{
				ID: id, Column: foreignKey.Column, ReferencedTable: foreignKey.ReferencedTable, ReferencedColumn: foreignKey.ReferencedColumn,
			})
		}
		for _, constraint := range table.UniqueConstraints {
			converted.UniqueConstraints = append(converted.UniqueConstraints, UniqueConstraint{Columns: append([]string(nil), constraint.Columns...)})
		}
		result.Tables = append(result.Tables, converted)
	}
	return result
}

func exportDeclarationInputType(source types.Type) packageextension.Type {
	result := packageextension.Type{Kind: string(source.Kind), Name: source.Name, Nullable: source.Nullable}
	for _, argument := range source.Args {
		result.Arguments = append(result.Arguments, exportDeclarationInputType(argument))
	}
	return result
}

func importDeclarationInputType(source packageextension.Type) types.Type {
	result := types.Type{Kind: types.Kind(source.Kind), Name: source.Name, Nullable: source.Nullable}
	for _, argument := range source.Arguments {
		result.Args = append(result.Args, importDeclarationInputType(argument))
	}
	return result
}

func validateDeclarationInputType(typ packageextension.Type) error {
	if typ.Kind == "" {
		return fmt.Errorf("type kind is missing")
	}
	switch typ.Kind {
	case "invalid", "never", "any", "void", "bool", "int", "int_literal", "float", "string", "string_literal", "bytes", "string_builder", "array", "range", "iterable", "hash", "function", "union", "named", "nil":
	default:
		return fmt.Errorf("unsupported type kind %q", typ.Kind)
	}
	if typ.Definition != nil || typ.Record != nil {
		return fmt.Errorf("source inspection metadata is not valid in an ORM declaration schema")
	}
	for _, argument := range typ.Arguments {
		if err := validateDeclarationInputType(argument); err != nil {
			return err
		}
	}
	return nil
}
