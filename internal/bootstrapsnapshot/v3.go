package bootstrapsnapshot

import (
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

type aggregateDeclaration struct {
	declaration identity.Declaration
	record      *ir.Record
	enum        *ir.Enum
}

type aggregateRegistry struct {
	declarations map[string]aggregateDeclaration
	byName       map[string][]aggregateDeclaration
	definitions  map[string]TypeDefinition
	building     map[string]bool
}

// BuildV3 encodes the aggregate-capable version 3 bootstrap snapshot.
func BuildV3(artifacts []*compiler.Artifact, sourceRoot string) (SnapshotV3, error) {
	inputs := projectMethods(artifacts)
	if len(inputs) == 0 {
		return SnapshotV3{}, fmt.Errorf("bootstrap snapshot v3 found no project functions")
	}
	methodIDs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		id := functionID(input.program, input.method)
		if key := input.method.Declaration.Key(); key != "" {
			methodIDs[key] = id
		}
		methodIDs["name:"+input.program.ModulePath+"#"+input.method.Name] = id
	}
	entry := ""
	module := ""
	for _, input := range inputs {
		if input.method.Name != compiler.MainFunction {
			continue
		}
		if entry != "" {
			return SnapshotV3{}, fmt.Errorf("bootstrap snapshot v3 requires exactly one top-level main function")
		}
		entry = functionID(input.program, input.method)
		module = input.program.ModulePath
	}
	if entry == "" {
		return SnapshotV3{}, fmt.Errorf("bootstrap snapshot v3 requires one top-level def main()")
	}

	sources, sourceIDs := projectSources(inputs, sourceRoot)
	registry := newAggregateRegistry(artifacts)
	functions := make([]Function, 0, len(inputs))
	for _, input := range inputs {
		lowered, err := lowerV3Function(input.program, input.method, sourceIDs[input.program.SourcePath], methodIDs, registry)
		if err != nil {
			return SnapshotV3{}, err
		}
		functions = append(functions, lowered)
	}
	sort.Slice(functions, func(i, j int) bool { return functions[i].ID < functions[j].ID })
	definitions := make([]TypeDefinition, 0, len(registry.definitions))
	for _, definition := range registry.definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ID < definitions[j].ID })
	return SnapshotV3{
		Format: Format, Version: Version3, Module: module, EntryFunction: entry,
		Sources: sources, Types: definitions, Functions: functions,
	}, nil
}

func newAggregateRegistry(artifacts []*compiler.Artifact) *aggregateRegistry {
	result := &aggregateRegistry{
		declarations: map[string]aggregateDeclaration{},
		byName:       map[string][]aggregateDeclaration{},
		definitions:  map[string]TypeDefinition{},
		building:     map[string]bool{},
	}
	for _, artifact := range artifacts {
		if artifact == nil || artifact.IR == nil || artifact.ExternalPackage {
			continue
		}
		result.collectStatements(artifact.IR.Statements)
	}
	return result
}

func (r *aggregateRegistry) collectStatements(statements []ir.Statement) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Record:
			r.add(aggregateDeclaration{declaration: node.Declaration, record: node})
		case *ir.Enum:
			r.add(aggregateDeclaration{declaration: node.Declaration, enum: node})
		case *ir.Module:
			r.collectStatements(node.Body)
		}
	}
}

func (r *aggregateRegistry) add(item aggregateDeclaration) {
	if item.declaration.Empty() {
		return
	}
	r.declarations[item.declaration.Key()] = item
	r.byName[item.declaration.Name] = append(r.byName[item.declaration.Name], item)
	leaf := item.declaration.LeafName()
	if leaf != item.declaration.Name {
		r.byName[leaf] = append(r.byName[leaf], item)
	}
}

func (r *aggregateRegistry) typeName(program *ir.Program, typ types.Type, span token.Span) (string, error) {
	if name, ok := scalarTypeName(typ); ok {
		return name, nil
	}
	if typ.Nullable || typ.Kind != types.Named {
		return "", unsupportedV3(program, span, "value type "+typ.String())
	}
	declaration, ok := r.resolve(typ)
	if !ok {
		return "", unsupportedV3(program, span, "aggregate type "+typ.String()+" without a unique declaration")
	}
	return r.register(program, typ, declaration, span)
}

func (r *aggregateRegistry) typeNameWithDeclaration(program *ir.Program, typ types.Type, declaration identity.Declaration, span token.Span) (string, error) {
	if typ.Declaration.Empty() && !declaration.Empty() {
		typ.Declaration = declaration
	}
	return r.typeName(program, typ, span)
}

func (r *aggregateRegistry) resolve(typ types.Type) (aggregateDeclaration, bool) {
	if key := typ.Declaration.Key(); key != "" {
		item, ok := r.declarations[key]
		return item, ok
	}
	items := r.byName[typ.Name]
	if len(items) == 1 {
		return items[0], true
	}
	return aggregateDeclaration{}, false
}

func (r *aggregateRegistry) register(program *ir.Program, typ types.Type, item aggregateDeclaration, span token.Span) (string, error) {
	id, err := r.aggregateTypeID(typ, item)
	if err != nil {
		return "", unsupportedV3(program, span, err.Error())
	}
	if len(id) > 256 {
		return "", unsupportedV3(program, span, "aggregate type identifier longer than 256 bytes")
	}
	if _, ok := r.definitions[id]; ok || r.building[id] {
		return id, nil
	}
	r.building[id] = true
	defer delete(r.building, id)

	if item.record != nil {
		if len(item.record.TypeParameters) != len(typ.Args) {
			return "", unsupportedV3(program, span, "aggregate type argument arity for "+typ.String())
		}
		substitutions := v3TypeSubstitutions(item.record.TypeParameters, typ.Args)
		fields := []Field{}
		definition := TypeDefinition{Kind: "record", ID: id, Fields: &fields}
		for _, statement := range item.record.Body {
			field, ok := statement.(*ir.RecordField)
			if !ok {
				continue
			}
			fieldType := v3SubstituteType(field.Type, substitutions)
			name, err := r.typeName(program, fieldType, field.SourceSpan())
			if err != nil {
				return "", err
			}
			if name == "Void" {
				return "", unsupportedV3(program, field.SourceSpan(), "Void record field "+field.Name)
			}
			fields = append(fields, Field{Name: field.Name, Type: name})
		}
		r.definitions[id] = definition
		return id, nil
	}
	if item.enum != nil {
		if item.enum.RawType.Kind != "" && item.enum.RawType.Kind != types.Invalid {
			return "", unsupportedV3(program, span, "raw enum "+typ.String())
		}
		if len(item.enum.TypeParameters) != len(typ.Args) {
			return "", unsupportedV3(program, span, "aggregate type argument arity for "+typ.String())
		}
		substitutions := v3TypeSubstitutions(item.enum.TypeParameters, typ.Args)
		variants := []Variant{}
		definition := TypeDefinition{Kind: "tagged", ID: id, Variants: &variants}
		for _, statement := range item.enum.Body {
			member, ok := statement.(*ir.EnumMember)
			if !ok {
				continue
			}
			variant := Variant{Name: member.Name, Fields: []Field{}}
			for _, field := range member.Fields {
				fieldType := v3SubstituteType(field.Type, substitutions)
				name, err := r.typeName(program, fieldType, member.SourceSpan())
				if err != nil {
					return "", err
				}
				if name == "Void" {
					return "", unsupportedV3(program, member.SourceSpan(), "Void enum field "+field.Name)
				}
				variant.Fields = append(variant.Fields, Field{Name: field.Name, Type: name})
			}
			variants = append(variants, variant)
		}
		r.definitions[id] = definition
		return id, nil
	}
	return "", unsupportedV3(program, span, "aggregate declaration "+typ.String())
}

func (r *aggregateRegistry) definition(id string) (TypeDefinition, bool) {
	definition, ok := r.definitions[id]
	return definition, ok
}

func (r *aggregateRegistry) aggregateTypeID(typ types.Type, declaration aggregateDeclaration) (string, error) {
	id := declaration.declaration.Module + "#" + declaration.declaration.Name
	if len(typ.Args) == 0 {
		return id, nil
	}
	items := make([]string, len(typ.Args))
	for index, argument := range typ.Args {
		if scalar, ok := scalarTypeName(argument); ok {
			items[index] = scalar
			continue
		}
		item, ok := r.resolve(argument)
		if !ok {
			return "", fmt.Errorf("aggregate type argument %s without a unique declaration", argument.String())
		}
		name, err := r.aggregateTypeID(argument, item)
		if err != nil {
			return "", err
		}
		items[index] = name
	}
	return id + "<" + strings.Join(items, ",") + ">", nil
}

func v3TypeSubstitutions(parameters []string, arguments []types.Type) map[string]types.Type {
	result := map[string]types.Type{}
	for index, parameter := range parameters {
		if index < len(arguments) {
			result[parameter] = arguments[index]
		}
	}
	return result
}

func v3SubstituteType(typ types.Type, substitutions map[string]types.Type) types.Type {
	if replacement, ok := substitutions[typ.Name]; ok && typ.Kind == types.Named && len(typ.Args) == 0 {
		replacement.Nullable = replacement.Nullable || typ.Nullable
		replacement.Readonly = replacement.Readonly || typ.Readonly
		return replacement
	}
	result := typ
	result.Args = make([]types.Type, len(typ.Args))
	for index, argument := range typ.Args {
		result.Args[index] = v3SubstituteType(argument, substitutions)
	}
	return result
}

func unsupportedV3(program *ir.Program, span token.Span, feature string) error {
	return &UnsupportedError{Path: program.SourcePath, Span: span, Feature: feature, Version: Version3}
}
