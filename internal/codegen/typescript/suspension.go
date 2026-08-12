package typescript

import (
	"fmt"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// SuspensionPlan is backend-owned analysis data. It deliberately does not
// make suspension part of TypeRB syntax, checking, or shared semantics.
// TypeScript generation uses it to preserve sequential TypeRB evaluation over
// Promise-based native APIs.
type SuspensionPlan struct {
	Methods          map[*ir.Method]bool
	Lambdas          map[*ir.Lambda]bool
	Calls            map[*ir.Call]bool
	Expressions      map[ir.Expression]bool
	Iterations       map[*ir.Iterate]bool
	StructuredBlocks map[*ir.StructuredBlock]bool
}

type suspensionMethod struct {
	module string
	owner  string
	method *ir.Method
}

type suspensionClass struct {
	name       string
	implements []string
}

type suspensionAnalyzer struct {
	programs         []*ir.Program
	plan             *SuspensionPlan
	methods          []suspensionMethod
	methodInfo       map[*ir.Method]suspensionMethod
	topMethods       map[string][]*ir.Method
	memberMethods    map[string][]*ir.Method
	interfaceMethods map[string][]*ir.Method
	classes          []suspensionClass
	lambdaBindings   map[functionBindingKey]*ir.Lambda
}

type functionBindingKey struct {
	module string
	method *ir.Method
	name   string
}

func AnalyzeSuspension(programs []*ir.Program) (*SuspensionPlan, error) {
	plan := &SuspensionPlan{
		Methods:          map[*ir.Method]bool{},
		Lambdas:          map[*ir.Lambda]bool{},
		Calls:            map[*ir.Call]bool{},
		Expressions:      map[ir.Expression]bool{},
		Iterations:       map[*ir.Iterate]bool{},
		StructuredBlocks: map[*ir.StructuredBlock]bool{},
	}
	analyzer := &suspensionAnalyzer{
		programs: programs, plan: plan, methodInfo: map[*ir.Method]suspensionMethod{},
		topMethods: map[string][]*ir.Method{}, memberMethods: map[string][]*ir.Method{},
		interfaceMethods: map[string][]*ir.Method{},
		lambdaBindings:   map[functionBindingKey]*ir.Lambda{},
	}
	for _, program := range programs {
		analyzer.collect(program.ModulePath, "", program.Statements, false)
	}

	for changed := true; changed; {
		changed = false
		for _, method := range analyzer.methods {
			if plan.Methods[method.method] || !analyzer.statementsSuspend(method.method.Body, method, false) {
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
		analyzer.statementsSuspend(method.method.Body, method, true)
	}
	for _, program := range programs {
		analyzer.statementsSuspend(program.Statements, suspensionMethod{module: program.ModulePath}, true)
	}
	for lambda, suspends := range plan.Lambdas {
		if suspends && lambda.SuccessType.Kind != types.Void && lambda.Fails.Kind == types.Never {
			return nil, fmt.Errorf("TypeScript function values that may suspend must omit their return type")
		}
	}
	return plan, nil
}

func (a *suspensionAnalyzer) collect(module, owner string, statements []ir.Statement, interfaceOwner bool) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Class:
			a.classes = append(a.classes, suspensionClass{name: node.Name, implements: append([]string(nil), node.Implements...)})
			a.collect(module, node.Name, node.Body, false)
		case *ir.Enum:
			a.collect(module, node.Name, node.Body, false)
		case *ir.Interface:
			for _, method := range node.Methods {
				a.addMethod(suspensionMethod{module: module, owner: node.Name, method: method}, true)
			}
		case *ir.Module:
			a.collect(module, owner, node.Body, interfaceOwner)
		case *ir.Method:
			a.addMethod(suspensionMethod{module: module, owner: owner, method: node}, interfaceOwner)
		}
	}
}

func (a *suspensionAnalyzer) addMethod(method suspensionMethod, interfaceMethod bool) {
	a.methods = append(a.methods, method)
	a.methodInfo[method.method] = method
	if method.owner == "" {
		key := callableKey(method.module, method.method.Name)
		a.topMethods[key] = append(a.topMethods[key], method.method)
		return
	}
	key := memberKey(method.owner, method.method.Name)
	a.memberMethods[key] = append(a.memberMethods[key], method.method)
	if interfaceMethod {
		a.interfaceMethods[key] = append(a.interfaceMethods[key], method.method)
	}
}

func (a *suspensionAnalyzer) propagateInterfaces() bool {
	changed := false
	for _, class := range a.classes {
		for _, interfaceName := range class.implements {
			for key, declarations := range a.interfaceMethods {
				prefix := interfaceName + "\x00"
				if !strings.HasPrefix(key, prefix) {
					continue
				}
				name := strings.TrimPrefix(key, prefix)
				implementations := a.memberMethods[memberKey(class.name, name)]
				maySuspend := anySuspending(a.plan.Methods, declarations) || anySuspending(a.plan.Methods, implementations)
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

func anySuspending(methods map[*ir.Method]bool, candidates []*ir.Method) bool {
	for _, method := range candidates {
		if methods[method] {
			return true
		}
	}
	return false
}

func callableKey(module, name string) string { return module + "\x00" + name }
func memberKey(owner, name string) string    { return owner + "\x00" + name }

func (a *suspensionAnalyzer) statementsSuspend(statements []ir.Statement, context suspensionMethod, record bool) bool {
	suspends := false
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Field:
			suspends = a.expressionSuspends(node.Value, context, record) || suspends
		case *ir.Variable:
			suspends = a.expressionSuspends(node.Value, context, record) || suspends
			if lambda, ok := node.Value.(*ir.Lambda); ok {
				a.lambdaBindings[functionBindingKey{module: context.module, method: context.method, name: node.Name}] = lambda
			}
		case *ir.Assignment:
			suspends = a.expressionSuspends(node.Target, context, record) || a.expressionSuspends(node.Value, context, record) || suspends
		case *ir.Return:
			suspends = a.expressionSuspends(node.Value, context, record) || suspends
		case *ir.ExpressionStatement:
			suspends = a.expressionSuspends(node.Expression, context, record) || suspends
		case *ir.If:
			branchSuspends := a.expressionSuspends(node.Condition, context, record) || a.statementsSuspend(node.Then, context, record) || a.expressionSuspends(node.ThenResult, context, record)
			for _, branch := range node.ElseIf {
				branchSuspends = a.expressionSuspends(branch.Condition, context, record) || a.statementsSuspend(branch.Body, context, record) || a.expressionSuspends(branch.Result, context, record) || branchSuspends
			}
			branchSuspends = a.statementsSuspend(node.Else, context, record) || a.expressionSuspends(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.Case:
			branchSuspends := a.statementsSuspend(node.Leading, context, record) || a.expressionSuspends(node.Value, context, record)
			for _, branch := range node.Branches {
				branchSuspends = a.expressionSuspends(branch.Value, context, record) || a.statementsSuspend(branch.Body, context, record) || a.expressionSuspends(branch.Result, context, record) || branchSuspends
			}
			branchSuspends = a.statementsSuspend(node.Else, context, record) || a.expressionSuspends(node.ElseResult, context, record) || branchSuspends
			if record && branchSuspends {
				a.plan.Expressions[node] = true
			}
			suspends = branchSuspends || suspends
		case *ir.While:
			suspends = a.expressionSuspends(node.Condition, context, record) || a.statementsSuspend(node.Body, context, record) || suspends
		case *ir.Iterate:
			iterationSuspends := isSuspendingORM(node.Intrinsic, node.Fails) || a.expressionSuspends(node.Source, context, record) || a.expressionSuspends(node.SliceSize, context, record) || a.statementsSuspend(node.Body, context, record)
			if record && iterationSuspends {
				a.plan.Iterations[node] = true
			}
			suspends = iterationSuspends || suspends
		case *ir.StructuredBlock:
			blockSuspends := isSuspendingORM(node.Intrinsic, node.Fails) || a.expressionSuspends(node.Call, context, record) || a.statementsSuspend(node.Body, context, record) || a.expressionSuspends(node.Value, context, record)
			if record && blockSuspends {
				a.plan.StructuredBlocks[node] = true
			}
			suspends = blockSuspends || suspends
		case *ir.NativeBlock:
			suspends = a.statementsSuspend(node.Body, context, record) || suspends
		}
	}
	return suspends
}

func (a *suspensionAnalyzer) expressionSuspends(expression ir.Expression, context suspensionMethod, record bool) bool {
	if expression == nil {
		return false
	}
	suspends := false
	switch node := expression.(type) {
	case *ir.Lambda:
		// Creating a lambda never suspends the enclosing function. Its body owns
		// a separate backend-only suspension boundary.
		lambdaSuspends := a.statementsSuspend(node.Body, context, record)
		a.plan.Lambdas[node] = lambdaSuspends
		return false
	case *ir.InterpolatedString:
		for _, part := range node.Parts {
			suspends = a.expressionSuspends(part.Expression, context, record) || suspends
		}
	case *ir.Array:
		for _, element := range node.Elements {
			suspends = a.expressionSuspends(element, context, record) || suspends
		}
	case *ir.Hash:
		for _, entry := range node.Entries {
			suspends = a.expressionSuspends(entry.Key, context, record) || a.expressionSuspends(entry.Value, context, record) || suspends
		}
	case *ir.JSXElement:
		suspends = a.expressionSuspends(node.Component, context, record)
		for _, attribute := range node.Attributes {
			suspends = a.expressionSuspends(attribute.Value, context, record) || suspends
		}
		for _, child := range node.Children {
			switch item := child.(type) {
			case *ir.JSXElement:
				suspends = a.expressionSuspends(item, context, record) || suspends
			case *ir.JSXExpression:
				suspends = a.expressionSuspends(item.Value, context, record) || suspends
			}
		}
	case *ir.Unary:
		suspends = a.expressionSuspends(node.Operand, context, record)
	case *ir.Conversion:
		suspends = a.expressionSuspends(node.Value, context, record)
	case *ir.Binary:
		suspends = a.expressionSuspends(node.Left, context, record) || a.expressionSuspends(node.Right, context, record)
	case *ir.Range:
		suspends = a.expressionSuspends(node.Start, context, record) || a.expressionSuspends(node.End, context, record)
	case *ir.Transform:
		suspends = a.expressionSuspends(node.Source, context, record) || a.expressionSuspends(node.Initial, context, record) || a.statementsSuspend(node.Body, context, record) || a.expressionSuspends(node.Result, context, record)
	case *ir.Call:
		suspends = a.expressionSuspends(node.Callee, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionSuspends(argument.Value, context, record) || suspends
		}
		if node.Block != nil {
			suspends = a.statementsSuspend(node.Block.Body, context, record) || suspends
		}
		callSuspends := isSuspendingIntrinsic(referenceIntrinsic(node.Callee), node.Fails) || isWebNextCall(node.Callee) || a.callTargetSuspends(node.Callee, context)
		if record && callSuspends {
			a.plan.Calls[node] = true
		}
		suspends = callSuspends || suspends
	case *ir.Attempt:
		suspends = a.expressionSuspends(node.Value, context, record) || a.statementsSuspend(node.Body, context, record) || a.expressionSuspends(node.BodyResult, context, record)
	case *ir.UnhandledEffect:
		suspends = a.expressionSuspends(node.Value, context, record)
	case *ir.EnumConstruct:
		for _, argument := range node.Arguments {
			suspends = a.expressionSuspends(argument, context, record) || suspends
		}
	case *ir.EnumCall:
		suspends = a.expressionSuspends(node.Receiver, context, record)
		for _, argument := range node.Arguments {
			suspends = a.expressionSuspends(argument.Value, context, record) || suspends
		}
		targetSuspends := anySuspending(a.plan.Methods, a.memberMethods[memberKey(node.EnumName, node.Method)])
		suspends = targetSuspends || suspends
	case *ir.TypeApply:
		suspends = a.expressionSuspends(node.Receiver, context, record)
	case *ir.Member:
		suspends = a.expressionSuspends(node.Receiver, context, record)
	case *ir.Index:
		suspends = a.expressionSuspends(node.Receiver, context, record) || a.expressionSuspends(node.Index, context, record)
	case *ir.Block:
		suspends = a.statementsSuspend(node.Body, context, record)
	case *ir.If:
		suspends = a.statementsSuspend([]ir.Statement{node}, context, record)
	case *ir.Case:
		suspends = a.statementsSuspend([]ir.Statement{node}, context, record)
	}
	if record && suspends {
		a.plan.Expressions[expression] = true
	}
	return suspends
}

func (a *suspensionAnalyzer) callTargetSuspends(callee ir.Expression, context suspensionMethod) bool {
	if callee != nil && callee.ExprType().Kind == types.Function {
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
		return a.callTargetSuspends(node.Receiver, context)
	case *ir.Identifier:
		if node.Reference != nil && node.Reference.Package != "" {
			return anySuspending(a.plan.Methods, a.topMethods[callableKey(node.Reference.Package, node.Reference.Symbol)])
		}
		if context.owner != "" && anySuspending(a.plan.Methods, a.memberMethods[memberKey(context.owner, node.Name)]) {
			return true
		}
		return anySuspending(a.plan.Methods, a.topMethods[callableKey(context.module, node.Name)])
	case *ir.Member:
		if node.Reference != nil && node.Reference.Intrinsic != "" {
			return false
		}
		if member, ok := node.Receiver.(*ir.Identifier); ok && member.Reference != nil && member.Reference.Package != "" && node.Reference != nil && node.Reference.ExportKind == "function" {
			return anySuspending(a.plan.Methods, a.topMethods[callableKey(member.Reference.Package, node.Name)])
		}
		owner := node.Receiver.ExprType().Name
		if owner == "" {
			return false
		}
		return anySuspending(a.plan.Methods, a.memberMethods[memberKey(owner, node.Name)])
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

func isSuspendingORM(intrinsic string, fails types.Type) bool {
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

func isSuspendingIntrinsic(intrinsic string, fails types.Type) bool {
	return isSuspendingORM(intrinsic, fails) || intrinsic == "trb.web.testing.dispatch" || intrinsic == "trb.web.middleware.logger.call" || intrinsic == "trb.platform.typescript.browser.request"
}

func isWebNextCall(callee ir.Expression) bool {
	member, ok := callee.(*ir.Member)
	return ok && member.Name == "call" && member.Receiver.ExprType().Name == "Next"
}
