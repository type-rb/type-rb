package checker

import (
	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

// A transparent class alias names the underlying constructor, not a new
// runtime class. Retain the class identity for lowering and argument checking.
func (c *Checker) aliasClassConstruction(typ types.Type) (ClassConstruction, bool) {
	if _, _, alias := c.aliasDefinition(typ); !alias {
		return ClassConstruction{}, false
	}
	target := c.canonicalType(c.expandAlias(typ, map[string]bool{}), c.activeTypeParameterSet())
	if target.Kind != types.Named || target.Nullable || target.Declaration.Kind != identity.Class {
		return ClassConstruction{}, false
	}
	if info := c.classes[target.Name]; info != nil && (target.Declaration.Empty() || info.declaration == target.Declaration) {
		target.Declaration = info.declaration
		return ClassConstruction{ResolvedType: target}, true
	}
	binding, found := c.resolution.ImportedTypeIdentity(target.Declaration)
	if !found || binding.Export == nil || binding.Export.Kind != resolver.ClassExport || binding.DeclarationIdentity() != target.Declaration {
		return ClassConstruction{}, false
	}
	c.markImportUsed(binding)
	return ClassConstruction{ResolvedType: target, TargetBinding: &binding}, true
}

func (c *Checker) checkAliasClassArguments(call *ast.CallExpression, construction ClassConstruction, actual []types.Type, sc *scope) {
	target := construction.ResolvedType
	if construction.TargetBinding != nil {
		binding := *construction.TargetBinding
		exported := *binding.Export
		exported.Parameters = append([]callsignature.Parameter(nil), exported.Parameters...)
		substitutions := typeSubstitutions(exported.TypeParameters, target.Args)
		for index := range exported.Parameters {
			exported.Parameters[index].Type = substituteType(exported.Parameters[index].Type, substitutions)
		}
		binding.Export = &exported
		c.checkImportedArguments(call, binding, actual, sc)
		return
	}
	info := c.classes[target.Name]
	if info == nil || info.declaration != target.Declaration {
		return
	}
	initialize := info.methods["initialize"]
	if initialize == nil || len(target.Args) == 0 {
		c.checkArguments(call, initialize, actual)
		return
	}
	signature := c.signatureFromMethod(initialize)
	substitutions := typeSubstitutions(info.typeParameters, target.Args)
	for index := range signature.parameters {
		signature.parameters[index].Type = substituteType(signature.parameters[index].Type, substitutions)
	}
	names := make([]string, len(initialize.Parameters))
	for index, parameter := range initialize.Parameters {
		names[index] = parameter.Name
	}
	c.checkCallSignature(call.Span(), target.Name+".new", signature.parameters, signature.variadic, call.Arguments, actual, names, nil)
	c.result.CallSignatures[call] = append([]callsignature.Parameter(nil), signature.parameters...)
}
