package checker

import (
	"fmt"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/types"
)

func (c *Checker) checkInterfaceMember(expression *ast.MemberExpression, receiver types.Type, class bool) types.Type {
	if class {
		c.memberKindMismatch(expression.Span(), receiver.Name, expression.Name, true)
		return invalidType()
	}
	if member, found := c.localMember(receiver, expression.Name, false, map[string]bool{}); found {
		member = c.specializeLocalClassMember(receiver, member)
		c.result.ExpressionDispatches[expression] = c.result.MethodDispatches[member.method]
		return member.typ
	}
	if binding, found := c.resolution.TypeMemberIdentity(receiver.Declaration, expression.Name); found && binding.Member != nil && !binding.Member.Class {
		binding = specializeResolvedClassMember(receiver, binding)
		c.recordReference(expression, binding)
		return c.resolvedBindingType(binding)
	}
	if binding, found := c.resolvedInterface(receiver); found {
		if binding.Export.UnsupportedFields[expression.Name] != "" {
			c.error(expression.Span(), fmt.Sprintf("member %s from native type %s cannot be represented safely: %s; use a TypeRB provider for this package", expression.Name, receiver.Name, binding.Export.UnsupportedFields[expression.Name]))
		} else {
			c.error(expression.Span(), fmt.Sprintf("type %s imported from %s has no member %s", receiver.Name, binding.Import.Path, expression.Name))
		}
		return invalidType()
	}
	c.error(expression.Span(), fmt.Sprintf("interface %s has no member %s", receiver, expression.Name))
	return invalidType()
}

func (c *Checker) interfaceOwnerAccess(expression ast.Expression) bool {
	if generic, ok := expression.(*ast.GenericExpression); ok {
		return c.interfaceOwnerAccess(generic.Receiver)
	}
	declaration := c.result.ExpressionDeclarations[expression]
	return declaration.Kind == identity.Interface || declaration.Kind == identity.TypeAlias
}

func (c *Checker) localTypeDeclaration(name string) (typeDeclaration, bool) {
	declaration := c.authoredTypeIdentity(name, c.activeTypeOwner)
	if !declaration.Empty() {
		if local, ok := c.declaredTypes[declaration.Name]; ok && local.identity == declaration {
			return local, true
		}
	}
	local, ok := c.declaredTypes[name]
	return local, ok && (declaration.Empty() || local.identity == declaration)
}

func (c *Checker) localInterface(typ types.Type) *ast.InterfaceStatement {
	typ = c.canonicalType(typ, c.activeTypeParameterSet())
	if typ.Declaration.Kind != identity.Interface || typ.Declaration.Module != c.result.Program.ModulePath {
		return nil
	}
	return c.interfaces[typ.Declaration.Name]
}

func (c *Checker) isInterface(typ types.Type) bool {
	typ = c.canonicalType(typ, c.activeTypeParameterSet())
	return typ.Kind == types.Named && typ.Declaration.Kind == identity.Interface
}

func (c *Checker) resolvedInterface(typ types.Type) (resolver.Binding, bool) {
	typ = c.canonicalType(typ, c.activeTypeParameterSet())
	binding, ok := c.resolution.ImportedTypeIdentity(typ.Declaration)
	return binding, ok && binding.Export != nil && binding.Export.Kind == resolver.InterfaceExport
}

func (c *Checker) interfaceMethodSignature(owner *ast.InterfaceStatement, method *ast.MethodStatement) methodSignature {
	popOwner := c.pushActiveTypeOwner(c.result.Declarations[owner].Name)
	defer popOwner()
	popParameters := c.pushActiveTypeParameters(owner.TypeParameters)
	defer popParameters()
	return c.signatureFromMethod(method)
}

// Implementation edges are interpreted in the class's declaring scope, before
// substituting the caller's type arguments. Leaf names are not type identities.
func (c *Checker) classImplements(classType, interfaceType types.Type, seen map[string]bool) bool {
	classType = c.canonicalType(classType, c.activeTypeParameterSet())
	key := classType.Declaration.Key()
	if key == "" || classType.Declaration.Kind != identity.Class || seen[key] {
		return false
	}
	seen[key] = true
	if info := c.classes[classType.Name]; info != nil && info.declaration == classType.Declaration {
		parameters := map[string]bool{}
		for _, name := range info.typeParameters {
			parameters[name] = true
		}
		popOwner := c.pushActiveTypeOwner(info.declaration.Name)
		defer popOwner()
		substitutions := typeSubstitutions(info.typeParameters, classType.Args)
		for _, implemented := range info.interfaces {
			implemented = c.expandAlias(c.canonicalType(implemented, parameters), map[string]bool{})
			if c.typesEquivalent(substituteType(implemented, substitutions), interfaceType) {
				return true
			}
		}
		return c.classImplements(types.FromName(info.superclass), interfaceType, seen)
	}
	binding, ok := c.resolution.ImportedTypeIdentity(classType.Declaration)
	if !ok {
		binding, ok = c.resolution.ContractTypeIdentity(classType.Declaration)
	}
	if !ok || binding.Export == nil || binding.Export.Kind != resolver.ClassExport {
		return false
	}
	parameters := map[string]bool{}
	for _, name := range binding.Export.TypeParameters {
		parameters[name] = true
	}
	substitutions := typeSubstitutions(binding.Export.TypeParameters, classType.Args)
	for _, implemented := range binding.Export.Interfaces {
		implemented = c.canonicalContractType(implemented, parameters, binding.Import)
		if c.typesEquivalent(substituteType(implemented, substitutions), interfaceType) {
			return true
		}
	}
	superclass := c.canonicalContractType(types.FromName(binding.Export.Superclass), parameters, binding.Import)
	return c.classImplements(superclass, interfaceType, seen)
}
