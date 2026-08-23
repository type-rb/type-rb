package nativepackage

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/declarationadapterhost"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

// ApplyDeclarationAdapterFiles overlays package-owned semantic declarations
// on top of declarations inferred by the TypeScript adapter from .d.ts files.
// The extension host validates the shared data protocol; this adapter validates
// the remaining TypeScript-specific bridge kinds and dependency ownership.
func ApplyDeclarationAdapterFiles(catalog *Catalog, sources []declarationadapterhost.Source) error {
	if catalog == nil {
		return errors.New("native type catalog is nil")
	}
	sorted := append([]declarationadapterhost.Source(nil), sources...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Package != sorted[j].Package {
			return sorted[i].Package < sorted[j].Package
		}
		if sorted[i].Mode != sorted[j].Mode {
			return sorted[i].Mode < sorted[j].Mode
		}
		return sorted[i].Path < sorted[j].Path
	})
	type declarationName struct {
		module string
		name   string
	}
	type declarationOwner struct {
		category    string
		packageName string
	}
	owners := map[declarationName]declarationOwner{}
	checksums, err := declarationadapterhost.Checksums(sorted)
	if err != nil {
		return err
	}
	catalog.DeclarationAdapterChecksums = checksums
	for _, source := range sorted {
		if source.Mode != "typescript" {
			return fmt.Errorf("declaration adapter %s selects unsupported mode %q; this TypeRB version provides only the TypeScript declaration adapter", source.Package, source.Mode)
		}
		provided, err := declarationadapterhost.Read(source.Path)
		if err != nil {
			return fmt.Errorf("declaration adapter %s (%s): %w", source.Package, source.Path, err)
		}
		moduleNames := make([]string, 0, len(provided.Modules))
		for moduleName := range provided.Modules {
			moduleNames = append(moduleNames, moduleName)
		}
		sort.Strings(moduleNames)
		for _, moduleName := range moduleNames {
			if !catalog.Owns(moduleName) || !nativeDependencyOwns(source.Dependencies, moduleName) {
				return fmt.Errorf("declaration adapter %s declares %s without a matching TypeScript native dependency", source.Package, moduleName)
			}
			patch, err := importDeclarationAdapterModule(provided.Modules[moduleName])
			if err != nil {
				return fmt.Errorf("declaration adapter %s module %s: %w", source.Package, moduleName, err)
			}
			current := catalog.Modules[moduleName]
			if current.Exports == nil {
				current.Exports = map[string]Export{}
			}
			if current.Records == nil {
				current.Records = map[string]Export{}
			}
			for _, name := range sortedDeclarationAdapterKeys(patch.Exports) {
				exported := patch.Exports[name]
				key := declarationName{module: moduleName, name: name}
				if previous, exists := owners[key]; exists {
					return declarationAdapterConflict(previous.packageName, previous.category, source.Package, "export", moduleName, name)
				}
				owners[key] = declarationOwner{category: "export", packageName: source.Package}
				current.Exports[name] = exported
				delete(current.Unsupported, name)
			}
			for _, name := range sortedDeclarationAdapterKeys(patch.Records) {
				record := patch.Records[name]
				key := declarationName{module: moduleName, name: name}
				if previous, exists := owners[key]; exists {
					return declarationAdapterConflict(previous.packageName, previous.category, source.Package, "supporting record", moduleName, name)
				}
				owners[key] = declarationOwner{category: "supporting record", packageName: source.Package}
				current.Records[name] = record
				delete(current.Unsupported, name)
			}
			catalog.Modules[moduleName] = current
		}
	}
	return nil
}

func declarationAdapterConflict(firstPackage, firstCategory, secondPackage, secondCategory, moduleName, name string) error {
	if firstCategory == secondCategory {
		return fmt.Errorf("declaration adapters %s and %s both declare %s %s from %s", firstPackage, secondPackage, firstCategory, name, moduleName)
	}
	return fmt.Errorf("declaration adapters %s and %s declare %s from %s once as an export and once as a supporting record", firstPackage, secondPackage, name, moduleName)
}

func nativeDependencyOwns(dependencies map[string]string, moduleName string) bool {
	for dependency := range dependencies {
		if moduleName == dependency || strings.HasPrefix(moduleName, dependency+"/") {
			return true
		}
	}
	return false
}

func importDeclarationAdapterModule(source packageextension.DeclarationAdapterModule) (Module, error) {
	result := Module{Exports: map[string]Export{}, Records: map[string]Export{}}
	for _, name := range sortedDeclarationAdapterKeys(source.Exports) {
		exported := source.Exports[name]
		converted, err := importDeclarationAdapterExport(exported)
		if err != nil {
			return Module{}, fmt.Errorf("export %s: %w", name, err)
		}
		result.Exports[name] = converted
	}
	for _, name := range sortedDeclarationAdapterKeys(source.Records) {
		record := source.Records[name]
		converted, err := importDeclarationAdapterExport(record)
		if err != nil {
			return Module{}, fmt.Errorf("record %s: %w", name, err)
		}
		result.Records[name] = converted
	}
	return result, nil
}

func importDeclarationAdapterExport(source packageextension.DeclarationAdapterExport) (Export, error) {
	if err := validateTypeScriptResultBridges(importDeclarationAdapterType(source.Type)); err != nil {
		return Export{}, err
	}
	result := Export{
		Kind: source.Kind, Type: importDeclarationAdapterType(source.Type), Required: source.Required,
		Variadic: source.Variadic, TypeParameters: append([]string(nil), source.TypeParameters...),
		Members: map[string]Export{}, InstanceMembers: map[string]Export{}, ClassMembers: map[string]Export{},
		UnsupportedFields: cloneStringMap(source.UnsupportedFields),
	}
	if source.ResultBridge != nil {
		result.ResultBridge = &ResultBridge{Kind: source.ResultBridge.Kind, Error: importDeclarationAdapterType(source.ResultBridge.Error)}
		if err := validateTypeScriptCallResultBridge(result); err != nil {
			return Export{}, err
		}
	}
	if source.AliasTarget != nil {
		converted := importDeclarationAdapterType(*source.AliasTarget)
		if err := validateTypeScriptResultBridges(converted); err != nil {
			return Export{}, err
		}
		result.AliasTarget = &converted
	}
	for _, parameter := range source.Parameters {
		converted := importDeclarationAdapterType(parameter)
		if err := validateTypeScriptResultBridges(converted); err != nil {
			return Export{}, err
		}
		result.Parameters = append(result.Parameters, converted)
	}
	for _, field := range source.Fields {
		converted := importDeclarationAdapterType(field.Type)
		if err := validateTypeScriptResultBridges(converted); err != nil {
			return Export{}, err
		}
		result.Fields = append(result.Fields, Field{Name: field.Name, Type: converted, Optional: field.Optional})
	}
	for _, name := range sortedDeclarationAdapterKeys(source.Members) {
		member := source.Members[name]
		converted, err := importDeclarationAdapterExport(member)
		if err != nil {
			return Export{}, fmt.Errorf("member %s: %w", name, err)
		}
		result.Members[name] = converted
	}
	for _, name := range sortedDeclarationAdapterKeys(source.InstanceMembers) {
		member := source.InstanceMembers[name]
		converted, err := importDeclarationAdapterExport(member)
		if err != nil {
			return Export{}, fmt.Errorf("instance member %s: %w", name, err)
		}
		result.InstanceMembers[name] = converted
	}
	for _, name := range sortedDeclarationAdapterKeys(source.ClassMembers) {
		member := source.ClassMembers[name]
		converted, err := importDeclarationAdapterExport(member)
		if err != nil {
			return Export{}, fmt.Errorf("class member %s: %w", name, err)
		}
		result.ClassMembers[name] = converted
	}
	return result, nil
}

func importDeclarationAdapterType(source packageextension.DeclarationAdapterType) Type {
	result := Type{Kind: source.Kind, Name: source.Name, Nullable: source.Nullable, Readonly: source.Readonly}
	for _, argument := range source.Arguments {
		result.Args = append(result.Args, importDeclarationAdapterType(argument))
	}
	if source.ResultBridge != nil {
		result.ResultBridge = &ResultBridge{Kind: source.ResultBridge.Kind, Error: importDeclarationAdapterType(source.ResultBridge.Error)}
	}
	return result
}

func cloneStringMap(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func validateTypeScriptResultBridges(typ Type) error {
	if typ.ResultBridge != nil && typ.ResultBridge.Kind != "result_to_promise_rejection" {
		return fmt.Errorf("unsupported TypeScript resultBridge kind %q", typ.ResultBridge.Kind)
	}
	for _, argument := range typ.Args {
		if err := validateTypeScriptResultBridges(argument); err != nil {
			return err
		}
	}
	return nil
}

func validateTypeScriptCallResultBridge(exported Export) error {
	if exported.ResultBridge == nil {
		return nil
	}
	if exported.ResultBridge.Kind != "promise_rejection_to_result" {
		return fmt.Errorf("unsupported TypeScript call resultBridge kind %q", exported.ResultBridge.Kind)
	}
	if exported.Kind != "function" {
		return fmt.Errorf("TypeScript call resultBridge is only valid on functions")
	}
	if exported.Type.Nullable || exported.Type.Kind != "named" || exported.Type.Name != "Result" || len(exported.Type.Args) != 2 {
		return fmt.Errorf("TypeScript promise_rejection_to_result bridge requires a Result<T, E> return type")
	}
	if exported.Type.Args[0].Kind == "void" {
		return fmt.Errorf("TypeScript promise_rejection_to_result bridge represents Promise<void> as Result<Unit, E>, not Result<Void, E>")
	}
	if !types.Equivalent(exported.Type.Args[1].Semantic(), exported.ResultBridge.Error.Semantic()) {
		return fmt.Errorf("TypeScript promise_rejection_to_result bridge error %s does not match Result error %s", exported.ResultBridge.Error.Semantic(), exported.Type.Args[1].Semantic())
	}
	errorType := exported.ResultBridge.Error
	if errorType.Nullable || errorType.Kind != "string" || errorType.Name != "String" || len(errorType.Args) != 0 {
		return fmt.Errorf("TypeScript promise_rejection_to_result currently requires String as its error type")
	}
	return nil
}

func sortedDeclarationAdapterKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
