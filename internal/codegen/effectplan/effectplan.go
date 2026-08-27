// Package effectplan performs whole-project call-graph analysis for backend
// effects that remain intentionally absent from TypeRB source signatures.
package effectplan

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/runtimeoperation"
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
	// LambdaModules retains the source module that owns each first-class
	// function. Backend policy can use that identity without moving target-
	// specific suspension rules into shared TypeRB semantics.
	LambdaModules map[*ir.Lambda]string
	methodKeys    map[methodKey]bool
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
	Intrinsic       func(string) bool
	Runtime         func(*ir.RuntimeBinding) bool
	Conversion      func(ir.ConversionKind) bool
	Transform       func(*ir.Transform) bool
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
		LambdaModules:    map[*ir.Lambda]string{},
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
			suspends = a.expressionReaches(node.Target, context, record) || suspends
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.Return:
			suspends = a.expressionReaches(node.Value, context, record) || suspends
		case *ir.ExpressionStatement:
			suspends = a.expressionReaches(node.Expression, context, record) || suspends
		case *ir.If:
			branchSuspends := a.expressionReaches(node.Condition, context, record)
			branchSuspends = a.statementsReach(node.Then, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ThenResult, context, record) || branchSuspends
			for _, branch := range node.ElseIf {
				branchSuspends = a.expressionReaches(branch.Condition, context, record) || branchSuspends
				branchSuspends = a.statementsReach(branch.Body, context, record) || branchSuspends
				branchSuspends = a.expressionReaches(branch.Result, context, record) || branchSuspends
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.Case:
			branchSuspends := a.statementsReach(node.Leading, context, record)
			branchSuspends = a.expressionReaches(node.Value, context, record) || branchSuspends
			for _, branch := range node.Branches {
				branchSuspends = a.expressionReaches(branch.Value, context, record) || branchSuspends
				branchSuspends = a.statementsReach(branch.Body, context, record) || branchSuspends
				branchSuspends = a.expressionReaches(branch.Result, context, record) || branchSuspends
				for _, alternative := range branch.Alternatives {
					branchSuspends = a.expressionReaches(alternative, context, record) || branchSuspends
				}
			}
			branchSuspends = a.statementsReach(node.Else, context, record) || branchSuspends
			branchSuspends = a.expressionReaches(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.While:
			suspends = a.expressionReaches(node.Condition, context, record) || suspends
			suspends = a.statementsReach(node.Body, context, record) || suspends
		case *ir.Iterate:
			iterationSuspends := a.intrinsicReaches(node.Intrinsic)
			iterationSuspends = a.expressionReaches(node.Source, context, record) || iterationSuspends
			iterationSuspends = a.expressionReaches(node.SliceSize, context, record) || iterationSuspends
			iterationSuspends = a.statementsReach(node.Body, context, record) || iterationSuspends
			if record && iterationSuspends {
				a.plan.Iterations[node] = true
			}
			suspends = iterationSuspends || suspends
		case *ir.StructuredBlock:
			blockSuspends := a.intrinsicReaches(node.Intrinsic)
			blockSuspends = a.expressionReaches(node.Call, context, record) || blockSuspends
			blockSuspends = a.statementsReach(node.Body, context, record) || blockSuspends
			blockSuspends = a.expressionReaches(node.Value, context, record) || blockSuspends
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
		a.plan.LambdaModules[node] = context.module
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
			suspends = a.expressionReaches(entry.Key, context, record) || suspends
			suspends = a.expressionReaches(entry.Value, context, record) || suspends
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
		if a.options.Conversion != nil && a.options.Conversion(node.Kind) {
			suspends = true
		}
	case *ir.Binary:
		suspends = a.expressionReaches(node.Left, context, record)
		suspends = a.expressionReaches(node.Right, context, record) || suspends
	case *ir.Range:
		suspends = a.expressionReaches(node.Start, context, record)
		suspends = a.expressionReaches(node.End, context, record) || suspends
	case *ir.Transform:
		suspends = a.expressionReaches(node.Source, context, record)
		suspends = a.expressionReaches(node.Initial, context, record) || suspends
		suspends = a.expressionReaches(node.Limit, context, record) || suspends
		suspends = a.statementsReach(node.Body, context, record) || suspends
		suspends = a.expressionReaches(node.Result, context, record) || suspends
		if a.options.Transform != nil && a.options.Transform(node) {
			suspends = true
		}
	case *ir.Call:
		suspends = a.expressionReaches(node.Callee, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
		}
		if node.Block != nil {
			suspends = a.statementsReach(node.Block.Body, context, record) || suspends
		}
		callSuspends := a.intrinsicReaches(referenceIntrinsic(node.Callee)) || a.runtimeReaches(referenceRuntime(node.Callee)) || a.options.WebNext && isWebNextCall(node.Callee) || a.callTargetReaches(node.Callee, context)
		if record && callSuspends {
			a.plan.Calls[node] = true
		}
		suspends = callSuspends || suspends
	case *ir.EnumConstruct:
		for _, argument := range node.Arguments {
			suspends = a.expressionReaches(argument.Value, context, record) || suspends
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
		suspends = a.expressionReaches(node.Receiver, context, record)
		suspends = a.expressionReaches(node.Index, context, record) || suspends
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
					if parameter.Name == identifier.Name {
						// The checked callee type above is authoritative. A source
						// parameter can retain a transparent function alias in IR.
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

func referenceRuntime(expression ir.Expression) *ir.RuntimeBinding {
	switch node := expression.(type) {
	case *ir.Identifier:
		if node.Reference != nil {
			return node.Reference.Runtime
		}
	case *ir.Member:
		if node.Reference != nil {
			return node.Reference.Runtime
		}
	case *ir.TypeApply:
		return referenceRuntime(node.Receiver)
	}
	return nil
}

func (a *analyzer) intrinsicReaches(intrinsic string) bool {
	return a.options.Intrinsic != nil && a.options.Intrinsic(intrinsic)
}

func (a *analyzer) runtimeReaches(binding *ir.RuntimeBinding) bool {
	return binding != nil && a.options.Runtime != nil && a.options.Runtime(binding)
}

// ORMOperation identifies intrinsics that may execute database work. It is
// shared by TypeScript suspension and portable execution-scope propagation.
func ORMOperation(intrinsic string) bool {
	return runtimeoperation.ORMExecution(intrinsic)
}

// ExecutionScope computes the functions that must receive the compiler-owned
// cancellation/deadline scope. The list contains operations whose native
// implementations can observe cancellation, plus web runtime boundaries that
// forward a child scope.
func ExecutionScope(programs []*ir.Program) *Plan {
	return Analyze(programs, Options{
		WebNext: true, CaptureLambdas: true,
		Intrinsic: func(intrinsic string) bool {
			return runtimeoperation.Describe(intrinsic).PropagatesExecutionScope
		},
		Runtime: func(binding *ir.RuntimeBinding) bool {
			return binding.PropagatesExecutionScope
		},
		Transform: func(transform *ir.Transform) bool {
			return transform.Operation == "concurrent_map"
		},
	})
}

func isWebNextCall(callee ir.Expression) bool {
	member, ok := callee.(*ir.Member)
	return ok && member.Name == "call" && member.Receiver.ExprType().Name == "Next"
}
