// Package packageextensionhost validates package-extension data and converts
// it into compiler-owned semantic declarations. Providers never receive the
// internal declaration representation through this boundary.
package packageextensionhost

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/packageextension"
	"github.com/type-rb/type-rb/internal/types"
)

func ExportDeclarationCatalog(provider string, catalog *declaration.Catalog) (packageextension.DeclarationCatalog, error) {
	result := packageextension.DeclarationCatalog{
		ProtocolVersion: packageextension.DeclarationProtocolVersion,
		Provider:        provider,
	}
	if catalog == nil {
		return result, fmt.Errorf("declaration catalog is empty")
	}
	for _, name := range sortedKeys(catalog.Types) {
		declared := catalog.Types[name]
		if declared == nil {
			return result, fmt.Errorf("declaration catalog type %s is empty", name)
		}
		if declared.Name != name {
			return result, fmt.Errorf("declaration catalog type key %s does not match declaration name %s", name, declared.Name)
		}
		converted := packageextension.DeclaredType{
			Name: name, TypeParameters: append([]string(nil), declared.TypeParameters...),
			Superclass: declared.Superclass, SourceModule: declared.SourceModule,
		}
		var err error
		converted.InstanceMembers, err = exportMembers(provider, declared.InstanceMembers)
		if err != nil {
			return result, fmt.Errorf("type %s instance members: %w", name, err)
		}
		converted.ClassMembers, err = exportMembers(provider, declared.ClassMembers)
		if err != nil {
			return result, fmt.Errorf("type %s class members: %w", name, err)
		}
		result.Types = append(result.Types, converted)
	}
	for _, name := range sortedKeys(catalog.Modules) {
		declared := catalog.Modules[name]
		if declared == nil {
			return result, fmt.Errorf("declaration catalog module %s is empty", name)
		}
		if declared.Name != name {
			return result, fmt.Errorf("declaration catalog module key %s does not match declaration name %s", name, declared.Name)
		}
		members, err := exportMembers(provider, declared.InstanceMembers)
		if err != nil {
			return result, fmt.Errorf("module %s members: %w", name, err)
		}
		result.Modules = append(result.Modules, packageextension.DeclaredModule{Name: name, InstanceMembers: members})
	}
	for _, rule := range catalog.FunctionBlockRules {
		result.FunctionBlockRules = append(result.FunctionBlockRules, packageextension.DeclaredFunctionBlockRule{
			Package: rule.Package, Function: rule.Function, EnclosingSuperclass: rule.EnclosingSuperclass,
			TypeArgument: rule.TypeArgument, ParameterTypeSuffix: rule.ParameterTypeSuffix,
		})
	}
	for _, rule := range catalog.FunctionArgumentReferenceRules {
		converted := packageextension.DeclaredFunctionArgumentReferenceRule{
			Package: rule.Package, Function: rule.Function, Argument: rule.Argument,
			Owner: exportReference(rule.Owner),
		}
		for _, target := range rule.Targets {
			converted.Targets = append(converted.Targets, exportReference(target))
		}
		result.FunctionArgumentReferenceRules = append(result.FunctionArgumentReferenceRules, converted)
	}
	for _, module := range sortedKeys(catalog.RuntimeTypesByModule) {
		converted := packageextension.DeclaredModuleRuntimeTypes{ModulePath: module}
		for _, typ := range catalog.RuntimeTypesByModule[module] {
			converted.Types = append(converted.Types, exportType(typ))
		}
		result.RuntimeTypes = append(result.RuntimeTypes, converted)
	}
	if err := packageextension.ValidateDeclarationCatalog(result); err != nil {
		return packageextension.DeclarationCatalog{}, err
	}
	return result, nil
}

func ImportDeclarationCatalog(source packageextension.DeclarationCatalog) (*declaration.Catalog, error) {
	if err := packageextension.ValidateDeclarationCatalog(source); err != nil {
		return nil, err
	}
	result := declaration.NewCatalog()
	for _, sourceType := range source.Types {
		declared := declaration.NewType(sourceType.Name, sourceType.Superclass)
		declared.TypeParameters = append([]string(nil), sourceType.TypeParameters...)
		declared.SourceModule = sourceType.SourceModule
		for _, member := range sourceType.InstanceMembers {
			declared.InstanceMembers[member.Name] = importMember(source.Provider, member, false)
		}
		for _, member := range sourceType.ClassMembers {
			declared.ClassMembers[member.Name] = importMember(source.Provider, member, true)
		}
		result.Types[sourceType.Name] = declared
	}
	for _, sourceModule := range source.Modules {
		declared := declaration.NewModule(sourceModule.Name)
		for _, member := range sourceModule.InstanceMembers {
			declared.InstanceMembers[member.Name] = importMember(source.Provider, member, false)
		}
		result.Modules[sourceModule.Name] = declared
	}
	for _, rule := range source.FunctionBlockRules {
		result.FunctionBlockRules = append(result.FunctionBlockRules, declaration.FunctionBlockRule{
			Package: rule.Package, Function: rule.Function, EnclosingSuperclass: rule.EnclosingSuperclass,
			TypeArgument: rule.TypeArgument, ParameterTypeSuffix: rule.ParameterTypeSuffix,
		})
	}
	for _, rule := range source.FunctionArgumentReferenceRules {
		converted := declaration.FunctionArgumentReferenceRule{
			Package: rule.Package, Function: rule.Function, Argument: rule.Argument,
			Owner: importReference(rule.Owner),
		}
		for _, target := range rule.Targets {
			converted.Targets = append(converted.Targets, importReference(target))
		}
		result.FunctionArgumentReferenceRules = append(result.FunctionArgumentReferenceRules, converted)
	}
	for _, runtime := range source.RuntimeTypes {
		for _, typ := range runtime.Types {
			result.RuntimeTypesByModule[runtime.ModulePath] = append(result.RuntimeTypesByModule[runtime.ModulePath], importType(typ))
		}
	}
	return result, nil
}

func exportMembers(provider string, members map[string]declaration.Member) ([]packageextension.DeclaredMember, error) {
	result := make([]packageextension.DeclaredMember, 0, len(members))
	for _, name := range sortedKeys(members) {
		member := members[name]
		if member.Name != name {
			return nil, fmt.Errorf("member key %s does not match declaration name %s", name, member.Name)
		}
		if member.Provider != provider {
			return nil, fmt.Errorf("member %s belongs to provider %s, expected %s", name, member.Provider, provider)
		}
		converted := packageextension.DeclaredMember{
			Name: name, Kind: string(member.Kind), RuntimeOperation: member.Intrinsic,
			CallSpecializer: member.Specializer, MinimumArguments: member.MinimumArguments,
			MaximumArguments: member.MaximumArguments, Return: exportType(member.Return),
			Variadic:       member.Variadic,
			TypeParameters: append([]string(nil), member.TypeParameters...),
		}
		for _, parameter := range member.Parameters {
			converted.Parameters = append(converted.Parameters, exportParameter(parameter))
		}
		for _, signature := range member.Alternatives {
			alternative := packageextension.DeclaredSignature{Return: exportType(signature.Return), Variadic: signature.Variadic}
			for _, parameter := range signature.Parameters {
				alternative.Parameters = append(alternative.Parameters, exportParameter(parameter))
			}
			converted.Alternatives = append(converted.Alternatives, alternative)
		}
		if member.Block != nil {
			converted.Block = &packageextension.DeclaredBlock{
				ControlBoundary: member.Block.ControlBoundary, Return: exportType(member.Block.Return),
				ResultBoundary: exportType(member.Block.ResultBoundary), Structured: member.Block.Structured,
			}
			for _, parameter := range member.Block.Parameters {
				converted.Block.Parameters = append(converted.Block.Parameters, exportType(parameter))
			}
		}
		result = append(result, converted)
	}
	return result, nil
}

func importMember(provider string, source packageextension.DeclaredMember, class bool) declaration.Member {
	result := declaration.Member{
		Name: source.Name, Kind: declaration.MemberKind(source.Kind), Intrinsic: source.RuntimeOperation,
		Specializer: source.CallSpecializer, MinimumArguments: source.MinimumArguments,
		MaximumArguments: source.MaximumArguments, Return: importType(source.Return),
		Variadic: source.Variadic, Class: class,
		TypeParameters: append([]string(nil), source.TypeParameters...), Provider: provider,
	}
	for _, parameter := range source.Parameters {
		result.Parameters = append(result.Parameters, importParameter(parameter))
	}
	for _, signature := range source.Alternatives {
		alternative := declaration.Signature{Return: importType(signature.Return), Variadic: signature.Variadic}
		for _, parameter := range signature.Parameters {
			alternative.Parameters = append(alternative.Parameters, importParameter(parameter))
		}
		result.Alternatives = append(result.Alternatives, alternative)
	}
	if source.Block != nil {
		result.Block = &declaration.Block{
			ControlBoundary: source.Block.ControlBoundary, Return: importType(source.Block.Return),
			ResultBoundary: importType(source.Block.ResultBoundary), Structured: source.Block.Structured,
		}
		for _, parameter := range source.Block.Parameters {
			result.Block.Parameters = append(result.Block.Parameters, importType(parameter))
		}
	}
	return result
}

func exportParameter(source declaration.Parameter) packageextension.DeclaredParameter {
	result := packageextension.DeclaredParameter{
		Name: source.Name, Type: exportType(source.Type), Keyword: source.Keyword, Optional: source.Optional,
		LiteralValues:        append([]string(nil), source.LiteralValues...),
		LiteralArrayElements: append([]string(nil), source.LiteralArrayElements...),
	}
	for _, values := range source.LiteralArrays {
		result.LiteralArrays = append(result.LiteralArrays, append([]string(nil), values...))
	}
	return result
}

func importParameter(source packageextension.DeclaredParameter) declaration.Parameter {
	result := declaration.Parameter{
		Name: source.Name, Type: importType(source.Type), Keyword: source.Keyword, Optional: source.Optional,
		LiteralValues:        append([]string(nil), source.LiteralValues...),
		LiteralArrayElements: append([]string(nil), source.LiteralArrayElements...),
	}
	for _, values := range source.LiteralArrays {
		result.LiteralArrays = append(result.LiteralArrays, append([]string(nil), values...))
	}
	return result
}

func exportType(source types.Type) packageextension.Type {
	if source.Kind == "" {
		return packageextension.Type{}
	}
	result := packageextension.Type{
		Kind: string(source.Kind), Name: source.Name, Nullable: source.Nullable,
	}
	for _, argument := range source.Args {
		result.Arguments = append(result.Arguments, exportType(argument))
	}
	return result
}

func importType(source packageextension.Type) types.Type {
	if source.Kind == "" {
		return types.Type{}
	}
	result := types.Type{
		Kind: types.Kind(source.Kind), Name: source.Name, Nullable: source.Nullable,
	}
	for _, argument := range source.Arguments {
		result.Args = append(result.Args, importType(argument))
	}
	return result
}

func exportReference(source declaration.DeclarationReference) packageextension.DeclaredReference {
	return packageextension.DeclaredReference{ModulePath: source.ModulePath, Name: source.Name}
}

func importReference(source packageextension.DeclaredReference) declaration.DeclarationReference {
	return declaration.DeclarationReference{ModulePath: source.ModulePath, Name: source.Name}
}

func sortedKeys[T any](items map[string]T) []string {
	keys := make([]string, 0, len(items))
	for key := range items {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
