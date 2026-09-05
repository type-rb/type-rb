package effectplan

import (
	"sort"
	"strings"

	"github.com/type-rb/type-rb/internal/diagnostic"
	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/stdlib"
	"github.com/type-rb/type-rb/internal/types"
)

// ValidateResources is a shared semantic check, not a backend effect policy.
// Its call graph includes imported/generated source, defaults and recursion.
// Only the checked language's lifetime guarantee is claimed, not purity,
// termination, or protection against handwritten target-language code.
func ValidateResources(programs []*ir.Program) []diagnostic.Diagnostic {
	plan := Analyze(programs, Options{
		ResourceSafety: true,
		Intrinsic:      func(name string) bool { return name != "" && !resourceSyncIntrinsics[name] },
		Runtime:        func(*ir.RuntimeBinding) bool { return true },
		Transform:      func(node *ir.Transform) bool { return node.Operation == "concurrent_map" },
	})
	paths := map[string]string{}
	for _, program := range programs {
		paths[program.ModulePath] = program.SourcePath
	}
	var diagnostics []diagnostic.Diagnostic
	for node, module := range plan.ResourceFailures {
		diagnostics = append(diagnostics, diagnostic.Diagnostic{
			Code: diagnostic.TypeError, Severity: diagnostic.Error, Path: paths[module], Span: node.SourceSpan(),
			Message: "scoped resource scope or borrow method reaches a suspending, native, or unverified operation; move it outside the resource scope or use a checked synchronous source helper",
		})
	}
	sort.Slice(diagnostics, func(i, j int) bool {
		if diagnostics[i].Path != diagnostics[j].Path {
			return diagnostics[i].Path < diagnostics[j].Path
		}
		return diagnostics[i].Span.Start.Offset < diagnostics[j].Span.Start.Offset
	})
	return diagnostics
}

func borrowsResource(method *ir.Method) bool {
	for _, parameter := range method.Parameters {
		if stdlib.IsScopedResourceType(parameter.Type) {
			return true
		}
	}
	return false
}

// A closed list of traversed semantic node shapes. Native syntax and new IR
// nodes are unsafe until this pass explicitly understands their evaluation.
func resourceOpaqueNode(node any) bool {
	switch value := node.(type) {
	case *ir.Comment, *ir.Import, *ir.TypeAlias, *ir.Newtype, *ir.Enum, *ir.EnumMember,
		*ir.Record, *ir.RecordField, *ir.Interface, *ir.Method, *ir.Class, *ir.Module,
		*ir.Field, *ir.Variable, *ir.Temporary, *ir.Assignment, *ir.Return, *ir.Break, *ir.Next,
		*ir.ExpressionStatement, *ir.If, *ir.Case, *ir.While, *ir.StructuredBlock,
		*ir.Identifier, *ir.Literal, *ir.Symbol, *ir.InterpolatedString, *ir.Array, *ir.Hash,
		*ir.Unary, *ir.Binary, *ir.Range, *ir.RecordConstruct,
		*ir.Call, *ir.EnumConstruct, *ir.EnumCall, *ir.TypeApply, *ir.Index, *ir.Block, *ir.Lambda:
		return false
	case *ir.Iterate:
		return !resourceSynchronousIteration(value.Operation)
	case *ir.Transform:
		return !resourceSynchronousIteration(value.Operation)
	case *ir.Member:
		return value.Reference != nil && value.Reference.Runtime != nil
	case *ir.Conversion:
		switch value.Kind {
		case ir.IntegerToFloatConversion, ir.UnionIntegerToFloatConversion,
			ir.NonNullableToNullableConversion, ir.NullableToNonNullableConversion,
			ir.RangeToIterableConversion, ir.NewtypeConstructionConversion, ir.NewtypeValueConversion:
			return false
		}
	}
	return true
}

func resourceSynchronousIteration(operation string) bool {
	switch operation {
	case "each", "each_slice", "map", "select", "reduce", "any?", "all?", "none?", "find", "find_index", "sort_by", "sort_by_descending":
		return true
	default:
		return false
	}
}

func (a *analyzer) resourceMemberUnsafe(member *ir.Member, context methodContext) bool {
	if intrinsic := referenceIntrinsic(member); intrinsic != "" {
		return !resourceSyncIntrinsics[intrinsic]
	}
	if referenceRuntime(member) != nil {
		return true
	}
	if member.Receiver == nil {
		return true
	}
	return a.resourceStorageUnsafe(member.Receiver.ExprType(), context)
}

func (a *analyzer) resourceStorageUnsafe(typ types.Type, context methodContext) bool {
	if typ.Kind == types.Any || typ.Kind == types.Function {
		return true
	}
	if typ.Kind == types.Union {
		for _, alternative := range typ.Args {
			if a.resourceStorageUnsafe(alternative, context) {
				return true
			}
		}
		return false
	}
	if typ.Kind != types.Named {
		return false
	}
	if typ.Declaration.Empty() {
		return true
	}
	if stdlib.IsFilesystemContractType(typ) {
		return false
	}
	owner := a.canonicalDeclarationIdentity(effectDeclarationIdentity(typ.Declaration, context.module, typ.Name))
	if class := a.classDefinitions[owner]; class != nil {
		return class.class.External
	}
	// Known source records/newtypes/modules expose checked storage or members;
	// missing/native/interface owners cannot certify property access.
	return !a.declarations[owner] || a.interfaceDefinitions[owner] != nil
}

func (a *analyzer) resourceCallReaches(call *ir.Call, callee ir.Expression, context methodContext, record bool) bool {
	if intrinsic := referenceIntrinsic(callee); intrinsic != "" {
		return !resourceSyncIntrinsics[intrinsic]
	}
	if referenceRuntime(callee) != nil {
		return true
	}
	if application, ok := callee.(*ir.TypeApply); ok {
		return a.resourceCallReaches(call, application.Receiver, context, record)
	}
	if owner, ok := a.constructorIdentity(callee, context); ok {
		if class := a.classDefinitions[owner]; class != nil && !class.class.External {
			return a.plan.ClassConstructors[class.class]
		}
		return true
	}
	var targets []*ir.Method
	if call.NewtypeMethod != nil {
		dispatch := call.NewtypeMethod.Dispatch
		targets = a.memberMethodsFor(effectDeclarationIdentity(dispatch.Owner, context.module, ""), dispatch.Name, dispatch.Class)
	} else {
		switch node := callee.(type) {
		case *ir.Identifier:
			switch {
			case !node.Dispatch.Empty():
				targets = a.memberMethodsFor(effectDeclarationIdentity(node.Dispatch.Owner, context.module, ""), node.Dispatch.Name, node.Dispatch.Class)
			case node.Declaration.Kind == identity.Function:
				targets = a.topMethods[callableKey(node.Declaration.Module, node.Declaration.Name)]
			case node.Reference != nil && node.Reference.Declaration.Kind == identity.Function:
				targets = a.topMethods[callableKey(node.Reference.Declaration.Module, node.Reference.Declaration.Name)]
			case node.Reference != nil && node.Reference.Package != "":
				targets = a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)]
			case !node.Lexical:
				if !context.owner.empty() {
					targets = a.memberMethodsFor(context.owner, node.Name, context.method != nil && context.method.Class)
				}
				if len(targets) == 0 {
					targets = a.topMethods[callableKey(context.module, node.Name)]
				}
			}
		case *ir.Member:
			if owner, class, ok := a.memberTargetIdentity(node, context); ok {
				if owner.kind == identity.Interface {
					return true
				}
				targets = a.memberMethodsFor(owner, node.Name, class)
			}
			if len(targets) == 0 && node.Reference != nil && node.Reference.ExportKind == "function" {
				targets = a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)]
			}
		}
	}
	if len(targets) == 0 {
		return true
	}
	for _, target := range targets {
		if target.External || a.plan.Methods[target] {
			return true
		}
		// Do not erase a resource through a generic or Any parameter. Authored
		// bodies are checked with the exact File parameter as a borrow origin.
		position := 0
		for _, argument := range call.Arguments {
			if stdlib.IsScopedResourceType(argument.Value.ExprType()) {
				found := false
				for index, parameter := range target.Parameters {
					matches := argument.Name == "" && !parameter.NamedOnly && index == position || argument.Name != "" && parameter.NamedOnly && parameter.Name == argument.Name
					if matches && stdlib.IsScopedResourceType(parameter.Type) && !parameter.Type.Nullable {
						found = true
					}
				}
				if !found {
					return true
				}
			}
			if argument.Name == "" {
				position++
			}
		}
	}
	return false
}

// Positive, compiler-owned non-suspending operation identities. This is not a
// deterministic/pure allowlist: synchronous I/O and local mutation are valid.
// Neither package prefixes nor provider/adapter claims grant admission.
var resourceSyncIntrinsics = func() map[string]bool {
	result := map[string]bool{}
	groups := map[string]string{
		"trb.cli":                "fail",
		"trb.internal.runtime":   "fail",
		"trb.std.file":           "open read read_text write write_text",
		"trb.std.dir":            "children create_all open open_file root_children root_create_all",
		"trb.std.path":           "join",
		"trb.std.io":             "puts",
		"trb.std.strings":        "length empty strip lstrip rstrip uppercase lowercase starts_with ends_with split codepoints characters reverse try_fetch slice try_slice index rindex contains replace_all",
		"trb.std.bytes":          "from_string to_string length at concat valid_utf8",
		"trb.std.arrays":         "length empty try_fetch slice try_slice first last copy contains index count uniq concat join pop shift push unshift reverse sort sort_descending",
		"trb.std.hashes":         "length empty fetch try_fetch contains_key keys values copy delete merge update",
		"trb.std.ranges":         "to_array",
		"trb.std.numbers":        "to_string integer_to_float integer_absolute integer_min integer_max integer_clamp integer_zero integer_positive integer_negative integer_even integer_odd float_to_string float_to_integer float_absolute float_finite float_infinite float_nan float_floor float_ceil float_round parse_integer try_parse_integer parse_float try_parse_float",
		"trb.std.booleans":       "to_string",
		"trb.std.string_builder": "new from_string append append_codepoint clear to_string length",
		"trb.internal.json":      "decode encode stringify parse_jsonc",
		"trb.internal.process":   "arguments environment working_directory run",
	}
	for prefix, names := range groups {
		for _, name := range strings.Fields(names) {
			result[prefix+"."+name] = true
		}
	}
	return result
}()
