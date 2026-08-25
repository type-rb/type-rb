package nativepackage

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
)

const maxCacheNesting = 64

// cacheCatalog is the generated cache representation. It keeps the public
// in-memory catalog convenient for compiler consumers while interning repeated
// semantic types, record fields, and diagnostics on disk. Pool references are
// one-based so zero remains the omitted value for optional references.
type cacheCatalog struct {
	FormatVersion               int                    `json:"formatVersion"`
	TypeScriptVersion           string                 `json:"typescriptVersion,omitempty"`
	Dependencies                map[string]string      `json:"dependencies"`
	DeclarationAdapterChecksums map[string]string      `json:"declarationAdapterChecksums,omitempty"`
	Types                       []cacheType            `json:"types"`
	Fields                      []cacheField           `json:"fields,omitempty"`
	Diagnostics                 []cacheDiagnostic      `json:"diagnostics,omitempty"`
	Modules                     map[string]cacheModule `json:"modules"`
}

type cacheModule struct {
	Exports     map[string]cacheExport `json:"exports"`
	Records     map[string]cacheExport `json:"records,omitempty"`
	Unsupported []int                  `json:"unsupported,omitempty"`
}

type cacheExport struct {
	Kind              string                 `json:"kind"`
	Type              int                    `json:"type"`
	AliasTarget       int                    `json:"aliasTarget,omitempty"`
	Parameters        []int                  `json:"parameters,omitempty"`
	Required          int                    `json:"required,omitempty"`
	Variadic          bool                   `json:"variadic,omitempty"`
	TypeParameters    []string               `json:"typeParameters,omitempty"`
	Fields            []int                  `json:"fields,omitempty"`
	Members           map[string]cacheExport `json:"members,omitempty"`
	InstanceMembers   map[string]cacheExport `json:"instanceMembers,omitempty"`
	ClassMembers      map[string]cacheExport `json:"classMembers,omitempty"`
	ResultBridge      *cacheResultBridge     `json:"resultBridge,omitempty"`
	UnsupportedFields []int                  `json:"unsupportedFields,omitempty"`
}

type cacheType struct {
	Kind         string             `json:"kind"`
	Name         string             `json:"name,omitempty"`
	Args         []int              `json:"args,omitempty"`
	ResultBridge *cacheResultBridge `json:"resultBridge,omitempty"`
	Nullable     bool               `json:"nullable,omitempty"`
	Readonly     bool               `json:"readonly,omitempty"`
}

type cacheResultBridge struct {
	Kind  string `json:"kind"`
	Error int    `json:"error"`
}

type cacheField struct {
	Name     string `json:"name"`
	Type     int    `json:"type"`
	Optional bool   `json:"optional,omitempty"`
}

type cacheDiagnostic struct {
	Name    string `json:"name"`
	Message string `json:"message"`
}

type catalogEncoder struct {
	types         []cacheType
	typeIDs       map[string]int
	fields        []cacheField
	fieldIDs      map[cacheField]int
	diagnostics   []cacheDiagnostic
	diagnosticIDs map[cacheDiagnostic]int
}

func encodeCatalog(catalog *Catalog) ([]byte, error) {
	encoder := &catalogEncoder{
		typeIDs:       map[string]int{},
		fieldIDs:      map[cacheField]int{},
		diagnosticIDs: map[cacheDiagnostic]int{},
	}
	encoded := cacheCatalog{
		FormatVersion:               FormatVersion,
		TypeScriptVersion:           catalog.TypeScriptVersion,
		Dependencies:                cloneDependencies(catalog.Dependencies),
		DeclarationAdapterChecksums: cloneDependencies(catalog.DeclarationAdapterChecksums),
		Types:                       []cacheType{},
		Modules:                     map[string]cacheModule{},
	}
	for _, name := range sortedMapKeys(catalog.Modules) {
		module, err := encoder.encodeModule(catalog.Modules[name])
		if err != nil {
			return nil, fmt.Errorf("module %q: %w", name, err)
		}
		encoded.Modules[name] = module
	}
	encoded.Types = encoder.types
	encoded.Fields = encoder.fields
	encoded.Diagnostics = encoder.diagnostics
	return json.Marshal(encoded)
}

func (e *catalogEncoder) encodeModule(module Module) (cacheModule, error) {
	exports, err := e.encodeExports(module.Exports, 0)
	if err != nil {
		return cacheModule{}, err
	}
	records, err := e.encodeExports(module.Records, 0)
	if err != nil {
		return cacheModule{}, err
	}
	return cacheModule{
		Exports:     exports,
		Records:     records,
		Unsupported: e.encodeDiagnostics(module.Unsupported),
	}, nil
}

func (e *catalogEncoder) encodeExports(exports map[string]Export, depth int) (map[string]cacheExport, error) {
	if exports == nil {
		return nil, nil
	}
	result := make(map[string]cacheExport, len(exports))
	for _, name := range sortedMapKeys(exports) {
		exported, err := e.encodeExport(exports[name], depth+1)
		if err != nil {
			return nil, fmt.Errorf("export %q: %w", name, err)
		}
		result[name] = exported
	}
	return result, nil
}

func (e *catalogEncoder) encodeExport(exported Export, depth int) (cacheExport, error) {
	if depth > maxCacheNesting {
		return cacheExport{}, fmt.Errorf("export nesting exceeds %d levels", maxCacheNesting)
	}
	typeID, err := e.encodeType(exported.Type, 0)
	if err != nil {
		return cacheExport{}, err
	}
	result := cacheExport{
		Kind:              exported.Kind,
		Type:              typeID,
		Required:          exported.Required,
		Variadic:          exported.Variadic,
		TypeParameters:    append([]string(nil), exported.TypeParameters...),
		UnsupportedFields: e.encodeDiagnostics(exported.UnsupportedFields),
	}
	if exported.AliasTarget != nil {
		result.AliasTarget, err = e.encodeType(*exported.AliasTarget, 0)
		if err != nil {
			return cacheExport{}, err
		}
	}
	for _, parameter := range exported.Parameters {
		parameterID, encodeErr := e.encodeType(parameter, 0)
		if encodeErr != nil {
			return cacheExport{}, encodeErr
		}
		result.Parameters = append(result.Parameters, parameterID)
	}
	for _, field := range exported.Fields {
		fieldID, encodeErr := e.encodeField(field)
		if encodeErr != nil {
			return cacheExport{}, encodeErr
		}
		result.Fields = append(result.Fields, fieldID)
	}
	result.Members, err = e.encodeExports(exported.Members, depth)
	if err != nil {
		return cacheExport{}, err
	}
	result.InstanceMembers, err = e.encodeExports(exported.InstanceMembers, depth)
	if err != nil {
		return cacheExport{}, err
	}
	result.ClassMembers, err = e.encodeExports(exported.ClassMembers, depth)
	if err != nil {
		return cacheExport{}, err
	}
	if exported.ResultBridge != nil {
		result.ResultBridge, err = e.encodeResultBridge(*exported.ResultBridge, 0)
		if err != nil {
			return cacheExport{}, err
		}
	}
	return result, nil
}

func (e *catalogEncoder) encodeType(typ Type, depth int) (int, error) {
	if depth > maxCacheNesting {
		return 0, fmt.Errorf("type nesting exceeds %d levels", maxCacheNesting)
	}
	encoded := cacheType{Kind: typ.Kind, Name: typ.Name, Nullable: typ.Nullable, Readonly: typ.Readonly}
	for _, argument := range typ.Args {
		argumentID, err := e.encodeType(argument, depth+1)
		if err != nil {
			return 0, err
		}
		encoded.Args = append(encoded.Args, argumentID)
	}
	if typ.ResultBridge != nil {
		bridge, err := e.encodeResultBridge(*typ.ResultBridge, depth)
		if err != nil {
			return 0, err
		}
		encoded.ResultBridge = bridge
	}
	key, err := json.Marshal(encoded)
	if err != nil {
		return 0, err
	}
	if existing := e.typeIDs[string(key)]; existing != 0 {
		return existing, nil
	}
	e.types = append(e.types, encoded)
	id := len(e.types)
	e.typeIDs[string(key)] = id
	return id, nil
}

func (e *catalogEncoder) encodeResultBridge(bridge ResultBridge, depth int) (*cacheResultBridge, error) {
	errorID, err := e.encodeType(bridge.Error, depth+1)
	if err != nil {
		return nil, err
	}
	return &cacheResultBridge{Kind: bridge.Kind, Error: errorID}, nil
}

func (e *catalogEncoder) encodeField(field Field) (int, error) {
	typeID, err := e.encodeType(field.Type, 0)
	if err != nil {
		return 0, err
	}
	encoded := cacheField{Name: field.Name, Type: typeID, Optional: field.Optional}
	if existing := e.fieldIDs[encoded]; existing != 0 {
		return existing, nil
	}
	e.fields = append(e.fields, encoded)
	id := len(e.fields)
	e.fieldIDs[encoded] = id
	return id, nil
}

func (e *catalogEncoder) encodeDiagnostics(input map[string]string) []int {
	var result []int
	for _, name := range sortedMapKeys(input) {
		encoded := cacheDiagnostic{Name: name, Message: input[name]}
		id := e.diagnosticIDs[encoded]
		if id == 0 {
			e.diagnostics = append(e.diagnostics, encoded)
			id = len(e.diagnostics)
			e.diagnosticIDs[encoded] = id
		}
		result = append(result, id)
	}
	return result
}

type catalogDecoder struct {
	catalog    cacheCatalog
	types      []Type
	typeStates []uint8
	fields     []Field
	fieldDone  []bool
}

func decodeCatalog(data []byte) (*Catalog, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var encoded cacheCatalog
	if err := decoder.Decode(&encoded); err != nil {
		return nil, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return nil, errors.New("trailing JSON content")
	}
	state := &catalogDecoder{
		catalog:    encoded,
		types:      make([]Type, len(encoded.Types)),
		typeStates: make([]uint8, len(encoded.Types)),
		fields:     make([]Field, len(encoded.Fields)),
		fieldDone:  make([]bool, len(encoded.Fields)),
	}
	for id := range encoded.Types {
		if _, err := state.decodeType(id+1, fmt.Sprintf("types[%d]", id)); err != nil {
			return nil, err
		}
	}
	for id := range encoded.Fields {
		if _, err := state.decodeField(id+1, fmt.Sprintf("fields[%d]", id)); err != nil {
			return nil, err
		}
	}
	result := &Catalog{
		FormatVersion:               encoded.FormatVersion,
		TypeScriptVersion:           encoded.TypeScriptVersion,
		Dependencies:                cloneDependencies(encoded.Dependencies),
		DeclarationAdapterChecksums: cloneDependencies(encoded.DeclarationAdapterChecksums),
		Modules:                     map[string]Module{},
	}
	for _, name := range sortedMapKeys(encoded.Modules) {
		module, err := state.decodeModule(encoded.Modules[name], "modules["+name+"]")
		if err != nil {
			return nil, err
		}
		result.Modules[name] = module
	}
	return result, nil
}

func (d *catalogDecoder) decodeModule(module cacheModule, context string) (Module, error) {
	exports, err := d.decodeExports(module.Exports, context+".exports", 0)
	if err != nil {
		return Module{}, err
	}
	records, err := d.decodeExports(module.Records, context+".records", 0)
	if err != nil {
		return Module{}, err
	}
	unsupported, err := d.decodeDiagnostics(module.Unsupported, context+".unsupported")
	if err != nil {
		return Module{}, err
	}
	return Module{Exports: exports, Records: records, Unsupported: unsupported}, nil
}

func (d *catalogDecoder) decodeExports(exports map[string]cacheExport, context string, depth int) (map[string]Export, error) {
	if exports == nil {
		return nil, nil
	}
	result := make(map[string]Export, len(exports))
	for _, name := range sortedMapKeys(exports) {
		exported, err := d.decodeExport(exports[name], context+"["+name+"]", depth+1)
		if err != nil {
			return nil, err
		}
		result[name] = exported
	}
	return result, nil
}

func (d *catalogDecoder) decodeExport(exported cacheExport, context string, depth int) (Export, error) {
	if depth > maxCacheNesting {
		return Export{}, fmt.Errorf("%s: export nesting exceeds %d levels", context, maxCacheNesting)
	}
	typ, err := d.decodeType(exported.Type, context+".type")
	if err != nil {
		return Export{}, err
	}
	result := Export{
		Kind:           exported.Kind,
		Type:           typ,
		Required:       exported.Required,
		Variadic:       exported.Variadic,
		TypeParameters: append([]string(nil), exported.TypeParameters...),
	}
	if exported.AliasTarget != 0 {
		alias, decodeErr := d.decodeType(exported.AliasTarget, context+".aliasTarget")
		if decodeErr != nil {
			return Export{}, decodeErr
		}
		result.AliasTarget = &alias
	}
	for index, parameterID := range exported.Parameters {
		parameter, decodeErr := d.decodeType(parameterID, fmt.Sprintf("%s.parameters[%d]", context, index))
		if decodeErr != nil {
			return Export{}, decodeErr
		}
		result.Parameters = append(result.Parameters, parameter)
	}
	for index, fieldID := range exported.Fields {
		field, decodeErr := d.decodeField(fieldID, fmt.Sprintf("%s.fields[%d]", context, index))
		if decodeErr != nil {
			return Export{}, decodeErr
		}
		result.Fields = append(result.Fields, field)
	}
	result.Members, err = d.decodeExports(exported.Members, context+".members", depth)
	if err != nil {
		return Export{}, err
	}
	result.InstanceMembers, err = d.decodeExports(exported.InstanceMembers, context+".instanceMembers", depth)
	if err != nil {
		return Export{}, err
	}
	result.ClassMembers, err = d.decodeExports(exported.ClassMembers, context+".classMembers", depth)
	if err != nil {
		return Export{}, err
	}
	if exported.ResultBridge != nil {
		result.ResultBridge, err = d.decodeResultBridge(*exported.ResultBridge, context+".resultBridge")
		if err != nil {
			return Export{}, err
		}
	}
	result.UnsupportedFields, err = d.decodeDiagnostics(exported.UnsupportedFields, context+".unsupportedFields")
	if err != nil {
		return Export{}, err
	}
	return result, nil
}

func (d *catalogDecoder) decodeType(id int, context string) (Type, error) {
	return d.decodeTypeAt(id, context, 0)
}

func (d *catalogDecoder) decodeTypeAt(id int, context string, depth int) (Type, error) {
	if depth > maxCacheNesting {
		return Type{}, fmt.Errorf("%s: type nesting exceeds %d levels", context, maxCacheNesting)
	}
	if id < 1 || id > len(d.catalog.Types) {
		return Type{}, fmt.Errorf("%s references unknown type %d", context, id)
	}
	index := id - 1
	switch d.typeStates[index] {
	case 1:
		return Type{}, fmt.Errorf("%s contains a cyclic type reference through %d", context, id)
	case 2:
		return d.types[index], nil
	}
	d.typeStates[index] = 1
	encoded := d.catalog.Types[index]
	result := Type{Kind: encoded.Kind, Name: encoded.Name, Nullable: encoded.Nullable, Readonly: encoded.Readonly}
	for argumentIndex, argumentID := range encoded.Args {
		argument, err := d.decodeTypeAt(argumentID, fmt.Sprintf("types[%d].args[%d]", index, argumentIndex), depth+1)
		if err != nil {
			return Type{}, err
		}
		result.Args = append(result.Args, argument)
	}
	if encoded.ResultBridge != nil {
		bridge, err := d.decodeResultBridgeAt(*encoded.ResultBridge, fmt.Sprintf("types[%d].resultBridge", index), depth+1)
		if err != nil {
			return Type{}, err
		}
		result.ResultBridge = bridge
	}
	d.types[index] = result
	d.typeStates[index] = 2
	return result, nil
}

func (d *catalogDecoder) decodeResultBridge(bridge cacheResultBridge, context string) (*ResultBridge, error) {
	return d.decodeResultBridgeAt(bridge, context, 0)
}

func (d *catalogDecoder) decodeResultBridgeAt(bridge cacheResultBridge, context string, depth int) (*ResultBridge, error) {
	errorType, err := d.decodeTypeAt(bridge.Error, context+".error", depth+1)
	if err != nil {
		return nil, err
	}
	return &ResultBridge{Kind: bridge.Kind, Error: errorType}, nil
}

func (d *catalogDecoder) decodeField(id int, context string) (Field, error) {
	if id < 1 || id > len(d.catalog.Fields) {
		return Field{}, fmt.Errorf("%s references unknown field %d", context, id)
	}
	index := id - 1
	if d.fieldDone[index] {
		return d.fields[index], nil
	}
	encoded := d.catalog.Fields[index]
	typ, err := d.decodeType(encoded.Type, fmt.Sprintf("fields[%d].type", index))
	if err != nil {
		return Field{}, err
	}
	result := Field{Name: encoded.Name, Type: typ, Optional: encoded.Optional}
	d.fields[index] = result
	d.fieldDone[index] = true
	return result, nil
}

func (d *catalogDecoder) decodeDiagnostics(ids []int, context string) (map[string]string, error) {
	if ids == nil {
		return nil, nil
	}
	result := make(map[string]string, len(ids))
	for index, id := range ids {
		if id < 1 || id > len(d.catalog.Diagnostics) {
			return nil, fmt.Errorf("%s[%d] references unknown diagnostic %d", context, index, id)
		}
		diagnostic := d.catalog.Diagnostics[id-1]
		if _, duplicate := result[diagnostic.Name]; duplicate {
			return nil, fmt.Errorf("%s contains duplicate diagnostic name %q", context, diagnostic.Name)
		}
		result[diagnostic.Name] = diagnostic.Message
	}
	return result, nil
}

func sortedMapKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
