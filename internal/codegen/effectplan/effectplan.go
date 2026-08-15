// Package effectplan performs whole-project call-graph analysis for backend
// effects that remain intentionally absent from TypeRB source signatures.
package effectplan

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Plan records the declarations and expressions that transitively reach one
// of the backend effects selected by Options.
type Plan struct {
	Methods          map[*ir.Method]bool
	Lambdas          map[*ir.Lambda]bool
	Calls            map[*ir.Call]bool
	EnumCalls        map[*ir.EnumCall]bool
	Expressions      map[ir.Expression]bool
	Iterations       map[*ir.Iterate]bool
	StructuredBlocks map[*ir.StructuredBlock]bool
	methodKeys       map[methodKey]bool
}

type methodKey struct {
	module string
	owner  string
	name   string
}

// Method reports whether a named project method transitively reaches an
// effect root. Integrations use this stable identity when dispatch code is
// generated outside the module that owns the method.
func (p *Plan) Method(module, owner, name string) bool {
	return p != nil && p.methodKeys[methodKey{module: module, owner: owner, name: name}]
}

type methodContext struct {
	module string
	owner  string
	method *ir.Method
}

type classContext struct {
	name       string
	implements []types.Type
}

type analyzer struct {
	programs         []*ir.Program
	plan             *Plan
	options          Options
	methods          []methodContext
	methodInfo       map[*ir.Method]methodContext
	topMethods       map[string][]*ir.Method
	memberMethods    map[string][]*ir.Method
	interfaceMethods map[string][]*ir.Method
	classes          []classContext
	lambdaBindings   map[functionBindingKey]*ir.Lambda
}

type functionBindingKey struct {
	module string
	method *ir.Method
	name   string
}

// Options chooses effect roots while retaining one call-graph model.
type Options struct {
	Intrinsic       func(string, types.Type) bool
	WebNext         bool
	CaptureLambdas  bool
	PassToFunctions bool
}

func Analyze(programs []*ir.Program, options Options) *Plan {
	plan := &Plan{
		Methods:          map[*ir.Method]bool{},
		Lambdas:          map[*ir.Lambda]bool{},
		Calls:            map[*ir.Call]bool{},
		EnumCalls:        map[*ir.EnumCall]bool{},
		Expressions:      map[ir.Expression]bool{},
		Iterations:       map[*ir.Iterate]bool{},
		StructuredBlocks: map[*ir.StructuredBlock]bool{},
		methodKeys:       map[methodKey]bool{},
	}
	analyzer := &analyzer{
		programs: programs, plan: plan, options: options, methodInfo: map[*ir.Method]methodContext{},
		topMethods: map[string][]*ir.Method{}, memberMethods: map[string][]*ir.Method{},
		interfaceMethods: map[string][]*ir.Method{},
		lambdaBindings:   map[functionBindingKey]*ir.Lambda{},
	}
	for _, program := range programs {
		analyzer.collect(program.ModulePath, "", program.Statements, false)
	}
	if options.WebNext {
		for _, method := range analyzer.methods {
			if method.owner == "Next" && method.method.Name == "call" {
				plan.Methods[method.method] = true
			}
		}
	}

	for changed := true; changed; {
		changed = false
		for _, method := range analyzer.methods {
			if plan.Methods[method.method] || !analyzer.statementsReach(method.method.Body, method, false) {
				continue
			}
			plan.Methods[method.method] = true
			changed = true
		}
		if analyzer.propagateInterfaces() {
			changed = true
		}
	}

	for _, method := range analyzer.methods {
		analyzer.statementsReach(method.method.Body, method, true)
		if plan.Methods[method.method] {
			plan.methodKeys[methodKey{module: method.module, owner: method.owner, name: method.method.Name}] = true
		}
	}
	for _, program := range programs {
		analyzer.statementsReach(program.Statements, methodContext{module: program.ModulePath}, true)
	}
	return plan
}

func (a *analyzer) collect(module, owner string, statements []ir.Statement, interfaceOwner bool) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			a.classes = append(a.classes, classContext{name: node.Name, implements: append([]types.Type(nil), node.Implements...)})
			a.collect(module, node.Name, node.Body, false)
		case *ir.Enum:
			a.collect(module, node.Name, node.Body, false)
		case *ir.Interface:
			for _, method := range node.Methods {
				a.addMethod(methodContext{module: module, owner: node.Name, method: method}, true)
			}
		case *ir.Module:
			a.collect(module, owner, node.Body, interfaceOwner)
		case *ir.Method:
			a.addMethod(methodContext{module: module, owner: owner, method: node}, interfaceOwner)
		}
	}
}

func (a *analyzer) addMethod(method methodContext, interfaceMethod bool) {
	a.methods = append(a.methods, method)
	a.methodInfo[method.method] = method
	if method.owner == "" {
		modules := []string{method.module}
		if strings.HasSuffix(method.module, "/index") {
			modules = append(modules, strings.TrimSuffix(method.module, "/index"))
		} else {
			modules = append(modules, strings.TrimSuffix(method.module, "/")+"/index")
		}
		for _, module := range modules {
			key := callableKey(module, method.method.Name)
			a.topMethods[key] = append(a.topMethods[key], method.method)
		}
		return
	}
	key := memberKey(method.owner, method.method.Name)
	a.memberMethods[key] = append(a.memberMethods[key], method.method)
	if interfaceMethod {
		a.interfaceMethods[key] = append(a.interfaceMethods[key], method.method)
	}
}

func (a *analyzer) propagateInterfaces() bool {
	changed := false
	for _, class := range a.classes {
		for _, implemented := range class.implements {
			interfaceName := implemented.Name
			for key, declarations := range a.interfaceMethods {
				prefix := interfaceName + "\x00"
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				name := strings.TrimPrefix(key, prefix)
				implementations := a.memberMethods[memberKey(class.name, name)]
				maySuspend := anyReached(a.plan.Methods, declarations) || anyReached(a.plan.Methods, implementations)
				if !maySuspend {
					continue
				}
				for _, method := range append(append([]*ir.Method(nil), declarations...), implementations...) {
					if !a.plan.Methods[method] {
						a.plan.Methods[method] = true
						changed = true
					}
				}
			}
		}
	}
	return changed
}

func anyReached(methods map[*ir.Method]bool, candidates []*ir.Method) bool {
	for _, method := range candidates {
		if methods[method] {
			return true
		}
	}
	return false
}

func callableKey(module, name string) string { return module + "\x00" + name }
func memberKey(owner, name string) string    { return owner + "\x00" + name }

func (a *analyzer) statementsReach(statements []ir.Statement, context methodContext, record bool) bool {
	suspends := false
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Field:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.Variable:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
			if lambda, ok := node.Value.(*ir.Lambda); ok {
				a.lambdaBindings[functionBindingKey{module: context.module, method: context.method, name: node.Name}] = lambda
			}
		case *ir.Assignment:
			suspends = a.expressionReaches(node.Target, context, record) || a.expressionReaches(node.Value, context, record) || suspends
		case *ir.Return:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.ExpressionStatement:
			suspends = a.expressionReaches(node.Expression, context, record) || suspends
		case *ir.If:
			branchSuspends := a.expressionReaches(node.Condition, context, record) || a.statementsReach(node.Then, context, record) || a.expressionReaches(node.ThenResult, context, record)
			for _, branch := range node.ElseIf {
				branchSuspends = a.expressionReaches(branch.Condition, context, record) || a.statementsReach(branch.Body, context, record) || a.expressionReaches(branch.Result, context, record) || branchSuspends
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.Case:
			branchSuspends := a.statementsReach(node.Leading, context, record) || a.expressionReaches(node.Value, context, record)
			for _, branch := range node.Branches {
				branchSuspends = a.expressionReaches(branch.Value, context, record) || a.statementsReach(branch.Body, context, record) || a.expressionReaches(branch.Result, context, record) || branchSuspends
				for _, alternative := range branch.Alternatives {
					branchSuspends = a.expressionReaches(alternative, context, record) || branchSuspends
				}
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.While:
			suspends = a.expressionReaches(node.Condition, context, record) || a.statementsReach(node.Body, context, record) || suspends
		case *ir.Iterate:
			iterationSuspends := a.intrinsicReaches(node.Intrinsic, node.Fails) || a.expressionReaches(node.Source, context, record) || a.expressionReaches(node.SliceSize, context, record) || a.statementsReach(node.Body, context, record)
			if record && iterationSuspends {
				a.plan.Iterations[node] = true
			}
			suspends = iterationSuspends || suspends
		case *ir.StructuredBlock:
			blockSuspends := a.intrinsicReaches(node.Intrinsic, node.Fails) || a.expressionReaches(node.Call, context, record) || a.statementsReach(node.Body, context, record) || a.expressionReaches(node.Value, context, record)
			if record && blockSuspends {
				a.plan.StructuredBlocks[node] = true
			}
			suspends = blockSuspends || suspends
		case *ir.NativeBlock:
			suspends = a.statementsReach(node.Body, context, record) || suspends
		}
	}
	return suspends
}

func (a *analyzer) expressionReaches(expression ir.Expression, context methodContext, record bool) bool {
	if expression == nil {
		return false
	}
	suspends := false
	switch node := expression.(type) {
	case *ir.Lambda:
		// Creating a lambda never suspends the enclosing function. Its body owns
		// a separate backend-only suspension boundary.
		lambdaSuspends := a.statementsReach(node.Body, context, record)
		a.plan.Lambdas[node] = lambdaSuspends
		return a.options.CaptureLambdas && lambdaSuspends
	case *ir.InterpolatedString:
		for _, part := range node.Parts {
			suspends = a.expressionReaches(part.Expression, context, record) || suspends
		}
	case *ir.Array:
		for _, element := range node.Elements {
			suspends = a.expressionReaches(element, context, record) || suspends
		}
	case *ir.Hash:
		for _, entry := range node.Entries {
			suspends = a.expressionReaches(entry.Key, context, record) || a.expressionReaches(entry.Value, context, record) || suspends
		}
	case *ir.JSXElement:
		suspends = a.expressionReaches(node.Component, context, record)
		for _, attribute := range node.Attributes {
			suspends = a.expressionReaches(attribute.Value, context, record) || suspends
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ir.JSXElement:
				suspends = a.expressionReaches(item, context, record) || suspends
			case *ir.JSXExpression:
				suspends = a.expressionReaches(item.Value, context, record) || suspends
			}
		}
	case *ir.Unary:
		suspends = a.expressionReaches(node.Operand, context, record)
	case *ir.Conversion:
		suspends = a.expressionReaches(node.Value, context, record)
	case *ir.Binary:
		suspends = a.expressionReaches(node.Left, context, record) || a.expressionReaches(node.Right, context, record)
	case *ir.Range:
		suspends = a.expressionReaches(node.Start, context, record) || a.expressionReaches(node.End, context, record)
	case *ir.Transform:
		suspends = a.expressionReaches(node.Source, context, record) || a.expressionReaches(node.Initial, context, record) || a.statementsReach(node.Body, context, record) || a.expressionReaches(node.Result, context, record)
	case *ir.Call:
		suspends = a.expressionReaches(node.Callee, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		if node.Block != nil {
			suspends = a.statementsReach(node.Block.Body, context, record) || suspends
		}
		callSuspends := a.intrinsicReaches(referenceIntrinsic(node.Callee), node.Fails) || a.options.WebNext && isWebNextCall(node.Callee) || a.callTargetReaches(node.Callee, context)
		if record && callSuspends {
			a.plan.Calls[node] = true
		}
		suspends = callSuspends || suspends
	case *ir.Attempt:
		suspends = a.expressionReaches(node.Value, context, record) || a.statementsReach(node.Body, context, record) || a.expressionReaches(node.BodyResult, context, record)
	case *ir.UnhandledEffect:
		suspends = a.expressionReaches(node.Value, context, record)
	case *ir.EnumConstruct:
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument, context, record) || suspends
		}
	case *ir.EnumCall:
		suspends = a.expressionReaches(node.Receiver, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		targetSuspends := anyReached(a.plan.Methods, a.memberMethods[memberKey(node.EnumName, node.Method)])
		if record && targetSuspends {
			a.plan.EnumCalls[node] = true
		}
		suspends = targetSuspends || suspends
	case *ir.TypeApply:
		suspends = a.expressionReaches(node.Receiver, context, record)
	case *ir.Member:
		suspends = a.expressionReaches(node.Receiver, context, record)
	case *ir.Index:
		suspends = a.expressionReaches(node.Receiver, context, record) || a.expressionReaches(node.Index, context, record)
	case *ir.Block:
		suspends = a.statementsReach(node.Body, context, record)
	case *ir.If:
		suspends = a.statementsReach([]ir.Statement{node}, context, record)
	case *ir.Case:
		suspends = a.statementsReach([]ir.Statement{node}, context, record)
	}
	if record && suspends {
		a.plan.Expressions[expression] = true
	}
	return suspends
}

func (a *analyzer) callTargetReaches(callee ir.Expression, context methodContext) bool {
	if a.options.PassToFunctions && callee != nil && callee.ExprType().Kind == types.Function {
		if lambda, ok := callee.(*ir.Lambda); ok {
			return a.plan.Lambdas[lambda]
		}
		if identifier, ok := callee.(*ir.Identifier); ok {
			if lambda := a.lambdaBindings[functionBindingKey{module: context.module, method: context.method, name: identifier.Name}]; lambda != nil {
				return a.plan.Lambdas[lambda]
			}
			if context.method != nil {
				for _, parameter := range context.method.Parameters {
					if parameter.Name == identifier.Name && parameter.Type.Kind == types.Function {
						// A higher-order function must accept both synchronous and
						// backend-suspending callbacks.
						return true
					}
				}
			}
		}
	}
	switch node := callee.(type) {
	case *ir.TypeApply:
		return a.callTargetReaches(node.Receiver, context)
	case *ir.Identifier:
		if node.Reference != nil && node.Reference.Package != "" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		if context.owner != "" && anyReached(a.plan.Methods, a.memberMethods[memberKey(context.owner, node.Name)]) {
			return true
		}
		return anyReached(a.plan.Methods, a.topMethods[callableKey(context.module, node.Name)])
	case *ir.Member:
		if node.Reference != nil && node.Reference.Intrinsic != "" {
			return false
		}
		owner := node.Receiver.ExprType().Name
		if candidates := a.memberMethods[memberKey(owner, node.Name)]; len(candidates) > 0 {
			return anyReached(a.plan.Methods, candidates)
		}
		if node.Reference != nil && node.Reference.Package != "" && node.Reference.ExportKind == "function" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		if member, ok := node.Receiver.(*ir.Identifier); ok && member.Reference != nil && member.Reference.Package != "" {
			return anyReached(a.plan.Methods, a.topMethods[callableKey(member.Reference.Package, node.Name)])
		}
		if owner == "" {
			return false
		}
		return anyReached(a.plan.Methods, a.memberMethods[memberKey(owner, node.Name)])
	default:
		return false
	}
}

func referenceIntrinsic(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Reference != nil {
			return node.Reference.Intrinsic
		}
	case *ir.Member:
		if node.Reference != nil {
			return node.Reference.Intrinsic
		}
	case *ir.TypeApply:
		return referenceIntrinsic(node.Receiver)
	}
	return ""
}

func (a *analyzer) intrinsicReaches(intrinsic string, fails types.Type) bool {
	return a.options.Intrinsic != nil && a.options.Intrinsic(intrinsic, fails)
}

// ORMOperation identifies intrinsics that may execute database work. It is
// shared by TypeScript suspension and portable execution-scope propagation.
func ORMOperation(intrinsic string, fails types.Type) bool {
	if !strings.HasPrefix(intrinsic, "trb.orm.") {
		return false
	}
	if fails.Kind != "" && fails.Kind != types.Never {
		return true
	}
	switch intrinsic {
	case "trb.orm.all", "trb.orm.first", "trb.orm.count", "trb.orm.explain", "trb.orm.find_by", "trb.orm.exists",
		"trb.orm.pluck", "trb.orm.pick", "trb.orm.ids", "trb.orm.sum", "trb.orm.average", "trb.orm.minimum", "trb.orm.maximum",
		"trb.orm.find", "trb.orm.create", "trb.orm.scope.find", "trb.orm.scope.create", "trb.orm.draft.save",
		"trb.orm.insert_all", "trb.orm.insert_if_absent", "trb.orm.draft.upsert", "trb.orm.upsert_all", "trb.orm.update",
		"trb.orm.changes.save", "trb.orm.delete", "trb.orm.destroy", "trb.orm.destroy_all", "trb.orm.update_all", "trb.orm.delete_all",
		"trb.orm.query.find_by", "trb.orm.query.exists", "trb.orm.query.update_all", "trb.orm.query.delete_all", "trb.orm.query.destroy_all",
		"trb.orm.query.pluck", "trb.orm.query.pick", "trb.orm.query.ids", "trb.orm.query.sum", "trb.orm.query.average", "trb.orm.query.minimum", "trb.orm.query.maximum",
		"trb.orm.query.all", "trb.orm.query.first", "trb.orm.query.count", "trb.orm.query.explain",
		"trb.orm.group.count", "trb.orm.group.sum", "trb.orm.group.average", "trb.orm.group.minimum", "trb.orm.group.maximum",
		"trb.orm.association.value.belongs_to", "trb.orm.association.value.has_many", "trb.orm.association.value.has_one",
		"trb.orm.association.load.belongs_to", "trb.orm.association.load.has_many", "trb.orm.association.load.has_one",
		"trb.orm.association.reload.belongs_to", "trb.orm.association.reload.has_many", "trb.orm.association.reload.has_one":
		return true
	default:
		return false
	}
}

// ExecutionScope computes the functions that must receive the compiler-owned
// cancellation/deadline scope. The list contains operations whose native
// implementations can observe cancellation, plus web runtime boundaries that
// forward a child scope.
func ExecutionScope(programs []*ir.Program) *Plan {
	return Analyze(programs, Options{
		WebNext: true, CaptureLambdas: true,
		Intrinsic: func(intrinsic string, fails types.Type) bool {
			return strings.HasPrefix(intrinsic, "trb.orm.") ||
				strings.HasPrefix(intrinsic, "trb.jobs.") ||
				intrinsic == "trb.platform.typescript.browser.request" ||
				intrinsic == "trb.web.middleware.logger.call" ||
				intrinsic == "trb.web.middleware.timeout.call" ||
				intrinsic == "trb.web.testing.dispatch"
		},
	})
}

func isWebNextCall(callee ir.Expression) bool {
	member, ok := callee.(*ir.Member)
	return ok && member.Name == "call" && member.Receiver.ExprType().Name == "Next"
}
