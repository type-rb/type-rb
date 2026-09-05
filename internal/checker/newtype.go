package checker

import (
	"fmt"
	"strings"

	"github.com/type-rb/type-rb/internal/ast"
	"github.com/type-rb/type-rb/internal/callsignature"
	"github.com/type-rb/type-rb/internal/declaration"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/resolver"
	"github.com/type-rb/type-rb/internal/token"
	"github.com/type-rb/type-rb/internal/types"
)

func (c *Checker) newtypeIsClosed(typ types.Type) bool {
	if typ.Kind != types.Named {
		return false
	}
	if binding, ok := c.resolution.ImportedTypeIdentity(typ.Declaration); ok && binding.Export != nil {
		return binding.Export.NewtypePrivateNew
	}
	if local := c.newtypes[typ.Name]; local != nil {
		return local.statement.PrivateNew
	}
	if _, binding, ok := c.newtypeDefinition(typ.Name); ok && binding != nil {
		return binding.Export.NewtypePrivateNew
	}
	if exported, ok := c.resolution.CompilerOwnedType(typ.Name); ok {
		return exported.NewtypePrivateNew
	}
	if binding, ok := c.resolution.ContractType(typ.Name); ok && binding.Export != nil {
		return binding.Export.NewtypePrivateNew
	}
	return false
}

// Closed construction needs immutable reachable storage, not the separate
// execution policy for concurrent captures. Recursively defined immutable
// records/enums are safe; a recursive generic that keeps changing its type
// arguments is conservatively rejected instead of expanding without a bound.
func (c *Checker) immutableNewtypeRepresentation(typ types.Type, visiting map[string]string) bool {
	typ = c.expandAlias(typ, map[string]bool{})
	typ.Nullable = false
	switch typ.Kind {
	case types.Nil, types.Bool, types.Int, types.IntLiteral, types.Float, types.String, types.StringLiteral, types.Bytes:
		return true
	case types.Range, types.Union:
		for _, argument := range typ.Args {
			if !c.immutableNewtypeRepresentation(argument, visiting) {
				return false
			}
		}
		return len(typ.Args) > 0
	case types.Named:
		key := typ.Declaration.Module + "#" + typ.Name
		if previous, exists := visiting[key]; exists {
			return previous == typ.String()
		}
		visiting[key] = typ.String()
		defer delete(visiting, key)
		if target, _, ok := c.newtypeDefinitionForType(typ); ok {
			return c.immutableNewtypeRepresentation(target, visiting)
		}
		if fields, ok := c.newtypeRecordFields(typ); ok {
			for _, field := range fields {
				if !c.immutableNewtypeRepresentation(field.Type, visiting) {
					return false
				}
			}
			return true
		}
		if variants, ok := c.enumVariants(typ); ok {
			for _, variant := range variants {
				for _, field := range variant.Fields {
					if !c.immutableNewtypeRepresentation(field.Type, visiting) {
						return false
					}
				}
			}
			return true
		}
	}
	return false
}

// Inbound plans inspect nominal nodes before representation erasure. This is
// a use-site restriction, not a ban on declaring or encoding closed values.
func (c *Checker) rejectClosedInbound(span token.Span, typ types.Type) bool {
	path := c.closedInboundPath(typ, map[string]bool{})
	return c.reportClosedInbound(span, path)
}

func (c *Checker) reportClosedInbound(span token.Span, path string) bool {
	if path == "" {
		return false
	}
	c.error(span, "generic inbound representation boundary cannot construct closed newtype: "+path+"; decode a representation DTO and call a public factory explicitly")
	return true
}

func (c *Checker) checkClosedNativeIngress(expression ast.Expression, typ types.Type) {
	var callee ast.Expression
	switch node := expression.(type) {
	case *ast.CallExpression:
		if _, sourceConstruction := c.result.RecordConstructions[node]; sourceConstruction {
			return
		}
		callee = node.Callee
		if generic, ok := callee.(*ast.GenericExpression); ok {
			callee = generic.Receiver
		}
	case *ast.MemberExpression:
		if binding, ok := c.result.References[node]; ok && binding.Member != nil && binding.Member.Kind == resolver.FunctionExport {
			return
		}
		if member, ok := c.external[node]; ok && member.Kind == declaration.Method {
			return
		}
		callee = node
	case *ast.Identifier:
		binding, ok := c.result.References[node]
		if !ok || binding.Export == nil || binding.Export.Kind != resolver.ValueExport {
			return
		}
		callee = node
	default:
		return
	}
	if !c.nativeRepresentationBoundary(callee) {
		return
	}
	c.rejectClosedInbound(expression.Span(), typ)
	if call, ok := expression.(*ast.CallExpression); ok {
		for _, argument := range call.Arguments {
			// An outgoing callback has incoming parameters. Follow nested
			// functions with their direction reversed rather than treating all
			// types in an outgoing value as ordinary projections.
			path := c.closedRepresentationEdge(c.result.Expressions[argument.Value], false, map[string]bool{})
			c.reportClosedInbound(argument.Value.Span(), path)
		}
	}
}

func (c *Checker) nativeRepresentationBoundary(callee ast.Expression) bool {
	if generic, ok := callee.(*ast.GenericExpression); ok {
		callee = generic.Receiver
	}
	if binding, ok := c.result.References[callee]; ok {
		if binding.Library != nil {
			// Compiler-owned generic collection operations preserve their input
			// values; they are not inbound representation edges. Codec operations
			// have an explicit directional gate in checkCodecApplication.
			return false
		}
		if binding.Export != nil {
			return !binding.Export.Source
		}
	}
	_, native := c.external[callee]
	return native
}

func (c *Checker) closedInboundPath(typ types.Type, visiting map[string]bool) string {
	return c.closedRepresentationEdge(typ, true, visiting)
}

func (c *Checker) closedRepresentationEdge(typ types.Type, inbound bool, visiting map[string]bool) string {
	typ = c.expandAlias(typ, map[string]bool{})
	typ.Nullable = false
	key := fmt.Sprintf("%t:%s#%s", inbound, typ.Declaration.Module, typ.String())
	if visiting[key] {
		return ""
	}
	visiting[key] = true
	defer delete(visiting, key)
	if c.newtypeIsClosed(typ) {
		if inbound {
			return typ.Name
		}
		return ""
	}
	for index, argument := range typ.Args {
		direction := inbound
		if typ.Kind == types.Function && index < len(typ.Args)-1 {
			direction = !inbound
		}
		if child := c.closedRepresentationEdge(argument, direction, visiting); child != "" {
			return typ.Name + " -> " + child
		}
	}
	if typ.Kind != types.Named {
		return ""
	}
	if target, _, ok := c.newtypeDefinitionForType(typ); ok {
		if child := c.closedRepresentationEdge(target, inbound, visiting); child != "" {
			return typ.Name + " -> " + child
		}
	}
	if fields, ok := c.newtypeRecordFields(typ); ok {
		for _, field := range fields {
			if child := c.closedRepresentationEdge(field.Type, inbound, visiting); child != "" {
				return typ.Name + "." + field.Name + " -> " + child
			}
		}
	}
	if variants, ok := c.enumVariants(typ); ok {
		for _, variant := range variants {
			for _, field := range variant.Fields {
				if child := c.closedRepresentationEdge(field.Type, inbound, visiting); child != "" {
					return typ.Name + "::" + variant.Name + "." + field.Name + " -> " + child
				}
			}
		}
	}
	return ""
}

func (c *Checker) newtypeRecordFields(typ types.Type) ([]resolver.RecordField, bool) {
	var fields []resolver.RecordField
	var parameters []string
	if binding, ok := c.resolution.ImportedTypeIdentity(typ.Declaration); ok && binding.Export != nil {
		if binding.Export.Kind != resolver.RecordExport {
			return nil, false
		}
		fields, parameters = binding.Export.Fields, binding.Export.TypeParameters
	} else {
		resolved, _, reference, ok := c.codecRecordResolved(typ.Name, true)
		if !ok {
			return nil, false
		}
		fields = resolved
		if reference != nil {
			parameters = reference.Export.TypeParameters
		} else if record := c.records[typ.Name]; record != nil {
			parameters = record.typeParameters
		}
	}
	result := append([]resolver.RecordField(nil), fields...)
	for index := range result {
		result[index].Type = substituteType(result[index].Type, typeSubstitutions(parameters, typ.Args))
	}
	return result, true
}

func (c *Checker) checkNewtypeConstructionAccess(span token.Span, typ types.Type) {
	if !c.newtypeIsClosed(typ) {
		return
	}
	if c.currentNewtype != nil {
		owner := c.result.Declarations[c.currentNewtype.statement]
		if typ.Declaration == owner || typ.Declaration.Empty() && typ.Name == c.currentNewtype.statement.Name {
			return
		}
	}
	c.error(span, "raw constructor for closed newtype "+typ.Name+" is private; use a public factory")
}

func (c *Checker) newtypeMethodBinding(typ types.Type, name string) (resolver.Binding, bool) {
	if binding, ok := c.resolution.TypeMemberIdentity(typ.Declaration, name); ok && binding.Member != nil {
		return binding, true
	}
	if _, owner, ok := c.newtypeDefinition(typ.Name); ok && owner != nil {
		if member, found := owner.Export.Members[name]; found {
			binding := *owner
			binding.Name, binding.Member = name, &member
			return binding, true
		}
	}
	return resolver.Binding{}, false
}

func (c *Checker) checkNewtypeMember(node *ast.MemberExpression, typ types.Type, class bool) types.Type {
	if strings.HasPrefix(node.Name, "_") && (c.currentNewtype == nil || c.result.Declarations[c.currentNewtype.statement] != typ.Declaration) {
		c.error(node.Span(), "private member "+node.Name+" cannot be accessed externally")
	}
	if member, ok := c.localNewtypeMember(typ, node.Name, class); ok {
		c.authoredMemberMethods[node] = member.method
		c.result.ExpressionDispatches[node] = c.result.MethodDispatches[member.method]
		return member.typ
	}
	if binding, ok := c.newtypeMethodBinding(typ, node.Name); ok && binding.Member.Class == class {
		c.recordReference(node, binding)
		c.markImportedDeclarationUsed(binding.DeclarationIdentity())
		return c.resolvedBindingType(binding)
	}
	kind := "instance"
	if class {
		kind = "class"
	}
	c.error(node.Span(), fmt.Sprintf("newtype %s has no %s member %s", typ.Name, kind, node.Name))
	return invalidType()
}

func (c *Checker) localNewtypeMember(typ types.Type, name string, class bool) (classMember, bool) {
	local := c.newtypes[typ.Name]
	if local == nil || !typ.Declaration.Empty() && typ.Declaration != c.result.Declarations[local.statement] {
		return classMember{}, false
	}
	return c.localMember(typ.Name, name, class, map[string]bool{})
}

func (c *Checker) checkNewtypeMethodCall(call *ast.CallExpression, typ types.Type, name string, class bool, arguments []types.Type) types.Type {
	if call.Block != nil {
		c.error(call.Block.Span(), "newtype methods do not accept call blocks")
	}
	if member, ok := c.localNewtypeMember(typ, name, class); ok && member.method != nil {
		if len(member.method.TypeParameters) > 0 {
			c.error(call.Span(), "generic method "+name+" requires explicit type arguments")
		}
		c.checkArguments(call, member.method, arguments)
		c.result.NewtypeMethodCalls[call] = NewtypeMethodCall{Dispatch: c.result.MethodDispatches[member.method]}
		return member.typ
	}
	if binding, ok := c.newtypeMethodBinding(typ, name); ok && binding.Member.Class == class {
		member := binding.Member
		if len(member.TypeParameters) > 0 {
			c.error(call.Span(), "generic method "+name+" requires explicit type arguments")
		}
		c.checkCallSignature(call.Span(), name, member.Parameters, member.Variadic, call.Arguments, arguments, nil, nil)
		c.result.CallSignatures[call] = append([]callsignature.Parameter(nil), member.Parameters...)
		c.result.NewtypeMethodCalls[call] = NewtypeMethodCall{Dispatch: identity.Dispatch{Owner: binding.DeclarationIdentity(), Name: name, Class: class}, Reference: &binding}
		return c.resolvedBindingType(binding)
	}
	return invalidType()
}
