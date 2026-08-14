package repl

import (
	"context"
	stdbase64 "encoding/base64"
	stdjson "encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Value struct {
	Type types.Type
	Data any
}

type bytesValue []byte

type stringBuilderValue struct{ value strings.Builder }

type Result struct {
	Value   Value
	Display bool
}

type arrayValue struct{ Items []Value }
type rangeValue struct {
	Start     int64
	End       int64
	Exclusive bool
}
type hashEntry struct{ Key, Value Value }
type hashValue struct{ Entries []hashEntry }

type recordDefinition struct {
	Module string
	Node   *ir.Record
	Fields []*ir.RecordField
}

type enumDefinition struct {
	Module  string
	Node    *ir.Enum
	Members map[string]*ir.EnumMember
	Methods map[string]*ir.Method
}

type enumValue struct {
	Definition *enumDefinition
	Name       string
	Payload    map[string]Value
}

type classDefinition struct {
	Module     string
	Node       *ir.Class
	Fields     []*ir.Field
	Methods    map[string]*ir.Method
	Superclass *classDefinition
}

type functionDefinition struct {
	Module string
	Method *ir.Method
}

type recordInstance struct {
	Definition *recordDefinition
	Fields     map[string]Value
}

type objectInstance struct {
	Definition *classDefinition
	Fields     map[string]Value
}

type typeValue struct {
	Record *recordDefinition
	Class  *classDefinition
	Enum   *enumDefinition
}

type callable struct {
	Function  *functionDefinition
	Lambda    *lambdaClosure
	Adapted   *callable
	Success   types.Type
	Failure   types.Type
	Method    *ir.Method
	Receiver  Value
	Module    string
	Construct *typeValue
	Intrinsic string
}

type lambdaClosure struct {
	Node   *ir.Lambda
	Module string
	Scope  *scope
}

type scope struct {
	parent *scope
	values map[string]Value
}

func (s *scope) get(name string) (Value, bool) {
	for current := s; current != nil; current = current.parent {
		value, ok := current.values[name]
		if ok {
			return value, true
		}
	}
	return Value{}, false
}

func (s *scope) assign(name string, value Value) bool {
	for current := s; current != nil; current = current.parent {
		if _, ok := current.values[name]; ok {
			current.values[name] = value
			return true
		}
	}
	return false
}

type Evaluator struct {
	stdout           io.Writer
	mode             string
	context          context.Context
	global           *scope
	definitions      map[string]any
	moduleValue      map[string]Value
	runtimeProviders []runtimeProvider
}

func NewEvaluator(stdout io.Writer, mode string) *Evaluator {
	return &Evaluator{
		stdout:           stdout,
		mode:             mode,
		context:          context.Background(),
		global:           &scope{values: map[string]Value{}},
		definitions:      map[string]any{},
		moduleValue:      map[string]Value{},
		runtimeProviders: newRuntimeProviders(),
	}
}

func (e *Evaluator) LoadProject(programs []*ir.Program, sessionModule string) error {
	if err := e.configureRuntimeProviders(programs); err != nil {
		return err
	}
	for _, program := range programs {
		if program.ModulePath != sessionModule {
			e.LoadDefinitions(program)
		}
	}
	for _, program := range programs {
		if program.ModulePath == sessionModule {
			continue
		}
		if err := e.loadProjectValues(program.Statements, program.ModulePath); err != nil {
			return fmt.Errorf("load %s: %w", program.ModulePath, err)
		}
	}
	return nil
}

func (e *Evaluator) loadProjectValues(statements []ir.Statement, module string) error {
	projectScope := &scope{parent: e.global, values: map[string]Value{}}
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Variable:
			if _, err := e.statement(node, module, projectScope); err != nil {
				return err
			}
		case *ir.Class:
			if err := e.evaluateOwnedConstants(node.Body, module, projectScope); err != nil {
				return err
			}
		case *ir.Module:
			if err := e.loadProjectValues(node.Body, module); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Evaluator) evaluateOwnedConstants(statements []ir.Statement, module string, sc *scope) error {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Variable:
			if node.Constant {
				if _, err := e.statement(node, module, sc); err != nil {
					return err
				}
			}
		case *ir.Class:
			if err := e.evaluateOwnedConstants(node.Body, module, sc); err != nil {
				return err
			}
		case *ir.Module:
			if err := e.evaluateOwnedConstants(node.Body, module, sc); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Evaluator) LoadDefinitions(program *ir.Program) {
	e.loadDefinitions(program.Statements, program.ModulePath)
	e.linkSuperclasses()
}

func (e *Evaluator) loadDefinitions(statements []ir.Statement, module string) {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Record:
			definition := &recordDefinition{Module: module, Node: node}
			for _, member := range node.Body {
				if field, ok := member.(*ir.RecordField); ok {
					definition.Fields = append(definition.Fields, field)
				}
			}
			e.definitions[symbolKey(module, node.Name)] = definition
		case *ir.Enum:
			definition := &enumDefinition{Module: module, Node: node, Members: map[string]*ir.EnumMember{}, Methods: map[string]*ir.Method{}}
			for _, statement := range node.Body {
				switch member := statement.(type) {
				case *ir.EnumMember:
					definition.Members[member.Name] = member
				case *ir.Method:
					definition.Methods[member.Name] = member
				}
			}
			e.definitions[symbolKey(module, node.Name)] = definition
		case *ir.TypeAlias:
			if len(node.Variants) == 0 {
				continue
			}
			enumNode := &ir.Enum{Name: node.Name, TypeParameters: append([]string(nil), node.TypeParameters...)}
			definition := &enumDefinition{Module: module, Node: enumNode, Members: map[string]*ir.EnumMember{}, Methods: map[string]*ir.Method{}}
			for index := range node.Variants {
				member := node.Variants[index]
				enumNode.Body = append(enumNode.Body, &member)
				definition.Members[member.Name] = &member
			}
			e.definitions[symbolKey(module, node.Name)] = definition
		case *ir.Class:
			definition := &classDefinition{Module: module, Node: node, Methods: map[string]*ir.Method{}}
			for _, member := range node.Body {
				switch item := member.(type) {
				case *ir.Field:
					definition.Fields = append(definition.Fields, item)
				case *ir.Method:
					definition.Methods[item.Name] = item
				}
			}
			e.definitions[symbolKey(module, node.Name)] = definition
		case *ir.Method:
			e.definitions[symbolKey(module, node.Name)] = &functionDefinition{Module: module, Method: node}
		case *ir.Module:
			e.loadDefinitions(node.Body, module)
		}
	}
}

func (e *Evaluator) linkSuperclasses() {
	for _, item := range e.definitions {
		definition, ok := item.(*classDefinition)
		if !ok || definition.Node.Superclass == nil {
			continue
		}
		name := expressionName(definition.Node.Superclass)
		if parent, ok := e.definitions[symbolKey(definition.Module, name)].(*classDefinition); ok {
			definition.Superclass = parent
			continue
		}
		for _, candidate := range e.definitions {
			if parent, ok := candidate.(*classDefinition); ok && parent.Node.Name == name {
				definition.Superclass = parent
				break
			}
		}
	}
}

func (e *Evaluator) Evaluate(statements []ir.Statement, module string) (Result, error) {
	return e.EvaluateContext(context.Background(), statements, module)
}

func (e *Evaluator) EvaluateContext(ctx context.Context, statements []ir.Statement, module string) (Result, error) {
	previous := e.context
	e.context = ctx
	defer func() { e.context = previous }()
	result, err := e.evaluate(statements, module, e.global)
	return result.Result, err
}

func (e *Evaluator) checkContext() error {
	select {
	case <-e.context.Done():
		return e.context.Err()
	default:
		return nil
	}
}

type flowResult struct {
	Result
	Returned bool
	Loop     loopFlow
}

type controlTransfer struct {
	flow flowResult
}

func (*controlTransfer) Error() string {
	return "control transfer from a value-producing branch"
}

type unhandledEffect struct {
	typ   types.Type
	value Value
}

func (e *unhandledEffect) Error() string {
	return e.typ.String() + ": " + Inspect(e.value)
}

type loopFlow uint8

const (
	loopNone loopFlow = iota
	loopBreak
	loopNext
)

func (e *Evaluator) evaluate(statements []ir.Statement, module string, sc *scope) (flowResult, error) {
	last := flowResult{}
	for _, statement := range statements {
		if err := e.checkContext(); err != nil {
			return flowResult{}, err
		}
		result, err := e.statement(statement, module, sc)
		if err != nil {
			var transfer *controlTransfer
			if errors.As(err, &transfer) {
				return transfer.flow, nil
			}
			return flowResult{}, err
		}
		if result.Display || result.Returned {
			last = result
		}
		if result.Returned || result.Loop != loopNone {
			return result, nil
		}
	}
	return last, nil
}

func (e *Evaluator) statement(statement ir.Statement, module string, sc *scope) (flowResult, error) {
	if err := e.checkContext(); err != nil {
		return flowResult{}, err
	}
	switch node := statement.(type) {
	case *ir.Comment, *ir.Import, *ir.Record, *ir.Enum, *ir.EnumMember, *ir.TypeAlias, *ir.Interface, *ir.Field, *ir.RecordField, *ir.Method:
		return flowResult{}, nil
	case *ir.Class:
		if err := e.evaluateOwnedConstants(node.Body, module, sc); err != nil {
			return flowResult{}, err
		}
		return flowResult{}, nil
	case *ir.Module:
		return e.evaluate(node.Body, module, sc)
	case *ir.Variable:
		value, err := e.expression(node.Value, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		value.Type = node.Type
		sc.values[node.Name] = value
		e.moduleValue[symbolKey(module, ownedName(node.Owner, node.Name))] = value
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	case *ir.Assignment:
		value, err := e.expression(node.Value, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		current, err := e.expression(node.Target, module, sc)
		if err != nil && node.Operator != "=" {
			return flowResult{}, err
		}
		if node.Operator != "=" {
			value, err = e.binary(current, strings.TrimSuffix(node.Operator, "="), value, current.Type)
			if err != nil {
				return flowResult{}, err
			}
		}
		if err := e.assign(node.Target, value, module, sc); err != nil {
			return flowResult{}, err
		}
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	case *ir.ExpressionStatement:
		value, err := e.expression(node.Expression, module, sc)
		return flowResult{Result: Result{Value: value, Display: err == nil}}, err
	case *ir.Return:
		value := Value{Type: types.FromName("Void")}
		var err error
		if node.Value != nil {
			value, err = e.expression(node.Value, module, sc)
		}
		return flowResult{Result: Result{Value: value}, Returned: true}, err
	case *ir.Break:
		return flowResult{Loop: loopBreak}, nil
	case *ir.Next:
		return flowResult{Loop: loopNext}, nil
	case *ir.If:
		body, _, branchScope, err := e.selectIfBranch(node, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		return e.evaluate(body, module, branchScope)
	case *ir.Case:
		body, _, branchScope, err := e.selectCaseBranch(node, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		return e.evaluate(body, module, branchScope)
	case *ir.While:
		last := flowResult{}
		for iterations := 0; ; iterations++ {
			if iterations >= 1_000_000 {
				return flowResult{}, errors.New("while loop exceeded REPL iteration limit")
			}
			condition, err := e.expression(node.Condition, module, sc)
			if err != nil {
				return flowResult{}, err
			}
			if !truthy(condition) {
				return last, nil
			}
			result, err := e.evaluate(node.Body, module, &scope{parent: sc, values: map[string]Value{}})
			if err != nil || result.Returned {
				return result, err
			}
			switch result.Loop {
			case loopBreak:
				result.Loop = loopNone
				return result, nil
			case loopNext:
				continue
			}
			last = result
		}
	case *ir.Iterate:
		return e.iterate(node, module, sc)
	case *ir.StructuredBlock:
		return e.structuredBlock(node, module, sc)
	case *ir.Native, *ir.NativeBlock:
		return flowResult{}, fmt.Errorf("native %s syntax is not executable by the typed IR REPL", e.mode)
	default:
		return flowResult{}, fmt.Errorf("unsupported REPL statement %T", statement)
	}
}

func (e *Evaluator) structuredBlock(node *ir.StructuredBlock, module string, sc *scope) (flowResult, error) {
	reference := expressionReference(node.Call.Callee)
	if reference == nil || reference.Intrinsic == "" {
		return flowResult{}, errors.New("structured block is missing its runtime intrinsic")
	}
	arguments := make([]evaluatedArgument, 0, len(node.Call.Arguments)+1)
	if member, ok := node.Call.Callee.(*ir.Member); ok && (reference.ReceiverMethod || e.runtimeHandles(reference.Intrinsic)) {
		receiver, err := e.expression(member.Receiver, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		arguments = append(arguments, evaluatedArgument{Value: receiver})
	}
	for _, argument := range node.Call.Arguments {
		value, err := e.expression(argument.Value, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		arguments = append(arguments, evaluatedArgument{Name: argument.Name, Value: value})
	}
	evaluate := func(bindings []Value) (Value, error) {
		blockScope := &scope{parent: sc, values: map[string]Value{}}
		for index, binding := range node.Bindings {
			if binding.Name == "_" || index >= len(bindings) {
				continue
			}
			value := bindings[index]
			value.Type = binding.Type
			blockScope.values[binding.Name] = value
		}
		flow, err := e.evaluate(node.Body, module, blockScope)
		if err != nil {
			return Value{}, err
		}
		if flow.Loop != loopNone {
			return Value{}, errors.New("loop transfer escaped a structured block")
		}
		if flow.Returned {
			return flow.Result.Value, nil
		}
		return e.expression(node.Value, module, blockScope)
	}
	resultType := node.Call.ExprType()
	value, handled, err := e.runtimeBlock(runtimeBlockInvocation{
		Name: reference.Intrinsic, Arguments: arguments, Type: resultType,
		Block: node, Evaluate: evaluate,
	})
	if err != nil {
		return flowResult{}, err
	}
	if !handled {
		return flowResult{}, fmt.Errorf("runtime intrinsic %s is not executable in the REPL", reference.Intrinsic)
	}
	if node.Result == nil {
		return flowResult{}, nil
	}
	fallible := node.Fails.Kind != "" && node.Fails.Kind != types.Never
	if fallible && !node.CaptureEffect {
		if node.Result.Return {
			return flowResult{Result: Result{Value: value}, Returned: true}, nil
		}
		variant, ok := value.Data.(*enumValue)
		if !ok {
			return flowResult{}, errors.New("fallible structured block did not return Result")
		}
		if variant.Name == "Err" {
			if node.UnhandledEffect {
				errorValue, exists := variant.Payload["error"]
				if !exists {
					return flowResult{}, errors.New("fallible structured block returned Err without an error")
				}
				return flowResult{}, &unhandledEffect{typ: node.Fails, value: errorValue}
			}
			return flowResult{Result: Result{Value: value}, Returned: true}, nil
		}
		if variant.Name != "Ok" {
			return flowResult{}, fmt.Errorf("fallible structured block returned unknown Result variant %s", variant.Name)
		}
		var exists bool
		value, exists = variant.Payload["value"]
		if !exists {
			return flowResult{}, errors.New("fallible structured block returned Ok without a value")
		}
	}
	value.Type = node.Result.Type
	if node.Result.Variable != nil {
		variable := node.Result.Variable
		sc.values[variable.Name] = value
		e.moduleValue[symbolKey(module, ownedName(variable.Owner, variable.Name))] = value
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	}
	if node.Result.Target != nil {
		if err := e.assign(node.Result.Target, value, module, sc); err != nil {
			return flowResult{}, err
		}
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	}
	if node.Result.Return {
		return flowResult{Result: Result{Value: value}, Returned: true}, nil
	}
	return flowResult{}, nil
}

func (e *Evaluator) selectIfBranch(node *ir.If, module string, sc *scope) ([]ir.Statement, ir.Expression, *scope, error) {
	condition, err := e.expression(node.Condition, module, sc)
	if err != nil {
		return nil, nil, nil, err
	}
	if truthy(condition) {
		return node.Then, node.ThenResult, &scope{parent: sc, values: map[string]Value{}}, nil
	}
	for _, branch := range node.ElseIf {
		condition, err := e.expression(branch.Condition, module, sc)
		if err != nil {
			return nil, nil, nil, err
		}
		if truthy(condition) {
			return branch.Body, branch.Result, &scope{parent: sc, values: map[string]Value{}}, nil
		}
	}
	return node.Else, node.ElseResult, &scope{parent: sc, values: map[string]Value{}}, nil
}

func (e *Evaluator) selectCaseBranch(node *ir.Case, module string, sc *scope) ([]ir.Statement, ir.Expression, *scope, error) {
	value, err := e.expression(node.Value, module, sc)
	if err != nil {
		return nil, nil, nil, err
	}
	for _, branch := range node.Branches {
		branchScope := &scope{parent: sc, values: map[string]Value{}}
		if branch.TypePattern {
			if !matchesTypePattern(value, branch.MatchType) {
				continue
			}
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				narrowed := value
				narrowed.Type = binding.Type
				branchScope.values[binding.Name] = narrowed
			}
			return branch.Body, branch.Result, branchScope, nil
		}
		if branch.PayloadEnum {
			variant, ok := value.Data.(*enumValue)
			if !ok || variant.Name != branch.Member {
				continue
			}
			for _, binding := range branch.Bindings {
				if binding.Name == "_" {
					continue
				}
				branchScope.values[binding.Name] = variant.Payload[binding.Field]
			}
			return branch.Body, branch.Result, branchScope, nil
		}
		candidate, err := e.expression(branch.Value, module, sc)
		if err != nil {
			return nil, nil, nil, err
		}
		if equal(value, candidate) {
			return branch.Body, branch.Result, branchScope, nil
		}
	}
	if node.HasElse {
		return node.Else, node.ElseResult, &scope{parent: sc, values: map[string]Value{}}, nil
	}
	return nil, nil, nil, fmt.Errorf("unreachable exhaustive case")
}

func matchesTypePattern(value Value, typ types.Type) bool {
	switch typ.Kind {
	case types.Bool:
		_, ok := value.Data.(bool)
		return ok
	case types.Int:
		_, ok := value.Data.(int64)
		return ok
	case types.Float:
		_, ok := value.Data.(float64)
		return ok
	case types.String:
		_, ok := value.Data.(string)
		return ok
	default:
		return false
	}
}

func (e *Evaluator) expression(expression ir.Expression, module string, sc *scope) (Value, error) {
	if err := e.checkContext(); err != nil {
		return Value{}, err
	}
	if expression == nil {
		return Value{Type: types.FromName("Void")}, nil
	}
	switch node := expression.(type) {
	case *ir.Lambda:
		return Value{Type: node.ExprType(), Data: &callable{Lambda: &lambdaClosure{Node: node, Module: module, Scope: sc}}}, nil
	case *ir.If:
		body, result, branchScope, err := e.selectIfBranch(node, module, sc)
		if err != nil {
			return Value{}, err
		}
		flow, err := e.evaluate(body, module, branchScope)
		if err != nil {
			return Value{}, err
		}
		if flow.Returned || flow.Loop != loopNone {
			return Value{}, &controlTransfer{flow: flow}
		}
		if result == nil {
			return Value{}, fmt.Errorf("if expression branch has no result")
		}
		return e.expression(result, module, branchScope)
	case *ir.Case:
		body, result, branchScope, err := e.selectCaseBranch(node, module, sc)
		if err != nil {
			return Value{}, err
		}
		flow, err := e.evaluate(body, module, branchScope)
		if err != nil {
			return Value{}, err
		}
		if flow.Returned || flow.Loop != loopNone {
			return Value{}, &controlTransfer{flow: flow}
		}
		if result == nil {
			return Value{}, fmt.Errorf("case expression branch has no result")
		}
		return e.expression(result, module, branchScope)
	case *ir.Attempt:
		attemptScope := &scope{parent: sc, values: map[string]Value{}}
		flow, err := e.evaluate(node.Body, module, attemptScope)
		if err != nil {
			return Value{}, err
		}
		if flow.Returned {
			return flow.Value, nil
		}
		result := node.Value
		if result == nil {
			result = node.BodyResult
		}
		value, err := e.expression(result, module, attemptScope)
		if err != nil {
			var transfer *controlTransfer
			if errors.As(err, &transfer) && transfer.flow.Returned {
				return transfer.flow.Value, nil
			}
		}
		return value, err
	case *ir.UnhandledEffect:
		result, err := e.expression(node.Value, module, sc)
		if err != nil {
			return Value{}, err
		}
		variant, ok := result.Data.(*enumValue)
		if !ok {
			return Value{}, fmt.Errorf("fallible operation returned %s instead of Result", result.Type)
		}
		switch variant.Name {
		case "Ok":
			value, exists := variant.Payload["value"]
			if !exists {
				return Value{}, errors.New("fallible operation returned Ok without a value")
			}
			value.Type = node.ExprType()
			return value, nil
		case "Err":
			value, exists := variant.Payload["error"]
			if !exists {
				return Value{}, errors.New("fallible operation returned Err without an error")
			}
			return Value{}, &unhandledEffect{typ: node.Fails, value: value}
		default:
			return Value{}, fmt.Errorf("fallible operation returned unknown Result variant %s", variant.Name)
		}
	case *ir.Literal:
		return literal(node)
	case *ir.InterpolatedString:
		var output strings.Builder
		for _, part := range node.Parts {
			if part.Expression == nil {
				text, err := strconv.Unquote("\"" + part.Text + "\"")
				if err != nil {
					text = part.Text
				}
				output.WriteString(text)
				continue
			}
			value, err := e.expression(part.Expression, module, sc)
			if err != nil {
				return Value{}, err
			}
			output.WriteString(plain(value))
		}
		return Value{Type: node.ExprType(), Data: output.String()}, nil
	case *ir.Symbol:
		return Value{Type: node.ExprType(), Data: node.Name}, nil
	case *ir.Array:
		items := make([]Value, 0, len(node.Elements))
		for _, element := range node.Elements {
			value, err := e.expression(element, module, sc)
			if err != nil {
				return Value{}, err
			}
			items = append(items, value)
		}
		return Value{Type: node.ExprType(), Data: &arrayValue{Items: items}}, nil
	case *ir.Hash:
		value := &hashValue{}
		for _, entry := range node.Entries {
			key, err := e.expression(entry.Key, module, sc)
			if err != nil {
				return Value{}, err
			}
			item, err := e.expression(entry.Value, module, sc)
			if err != nil {
				return Value{}, err
			}
			value.Entries = append(value.Entries, hashEntry{Key: key, Value: item})
		}
		return Value{Type: node.ExprType(), Data: value}, nil
	case *ir.Identifier:
		if node.Name == "self" || strings.HasPrefix(node.Name, "@") {
			return e.selfValue(node.Name, sc)
		}
		if value, ok := sc.get(node.Name); ok {
			return value, nil
		}
		if node.Owner != "" {
			if value, ok := e.moduleValue[symbolKey(module, ownedName(node.Owner, node.Name))]; ok {
				return value, nil
			}
		}
		if node.Reference != nil {
			if node.Reference.Intrinsic != "" {
				return Value{Type: node.ExprType(), Data: &callable{Intrinsic: node.Reference.Intrinsic, Module: module}}, nil
			}
			if value, ok := e.symbol(node.Reference.Package, node.Reference.Symbol); ok {
				return value, nil
			}
		}
		if value, ok := e.moduleValue[symbolKey(module, node.Name)]; ok {
			return value, nil
		}
		if value, ok := e.symbol(module, node.Name); ok {
			return value, nil
		}
		return Value{}, fmt.Errorf("%s is not available in the REPL environment", node.Name)
	case *ir.Unary:
		value, err := e.expression(node.Operand, module, sc)
		if err != nil {
			return Value{}, err
		}
		switch node.Operator {
		case "!", "not":
			return Value{Type: node.ExprType(), Data: !truthy(value)}, nil
		case "-":
			switch number := value.Data.(type) {
			case int64:
				return Value{Type: node.ExprType(), Data: -number}, nil
			case float64:
				return Value{Type: node.ExprType(), Data: -number}, nil
			}
		case "+":
			return value, nil
		}
		return Value{}, fmt.Errorf("operator %s does not support %s", node.Operator, value.Type)
	case *ir.Conversion:
		value, err := e.expression(node.Value, module, sc)
		if err != nil {
			return Value{}, err
		}
		switch node.Kind {
		case ir.RangeToIterableConversion:
			value.Type = node.ExprType()
			return value, nil
		case ir.PureFunctionToFallibleConversion:
			function, ok := value.Data.(*callable)
			if !ok {
				return Value{}, fmt.Errorf("cannot adapt %s as a fallible function", value.Type)
			}
			_, success, valid := types.FunctionSignature(node.ExprType())
			if !valid {
				return Value{}, fmt.Errorf("invalid fallible function type %s", node.ExprType())
			}
			return Value{Type: node.ExprType(), Data: &callable{
				Adapted: function,
				Success: success,
				Failure: types.FunctionFailure(node.ExprType()),
			}}, nil
		case ir.IntegerToFloatConversion:
			integer, ok := value.Data.(int64)
			if !ok {
				return Value{}, fmt.Errorf("cannot convert %s to Float", value.Type)
			}
			return Value{Type: node.ExprType(), Data: float64(integer)}, nil
		case ir.UnionIntegerToFloatConversion:
			if integer, ok := value.Data.(int64); ok {
				value.Data = float64(integer)
			}
			value.Type = node.ExprType()
			return value, nil
		case ir.NonNullableToNullableConversion:
			if node.Value.ExprType().Kind == types.Int && node.ExprType().Kind == types.Float {
				integer, ok := value.Data.(int64)
				if !ok {
					return Value{}, fmt.Errorf("cannot convert %s to Float?", value.Type)
				}
				value.Data = float64(integer)
			}
			value.Type = node.ExprType()
			return value, nil
		case ir.NullableToNonNullableConversion:
			if value.Data == nil {
				return Value{}, fmt.Errorf("cannot use nil as %s", node.ExprType())
			}
			value.Type = node.ExprType()
			return value, nil
		default:
			return Value{}, fmt.Errorf("unknown conversion %s", node.Kind)
		}
	case *ir.Binary:
		left, err := e.expression(node.Left, module, sc)
		if err != nil {
			return Value{}, err
		}
		if node.Operator == "and" || node.Operator == "&&" {
			if !truthy(left) {
				return Value{Type: node.ExprType(), Data: false}, nil
			}
		}
		if node.Operator == "or" || node.Operator == "||" {
			if truthy(left) {
				return Value{Type: node.ExprType(), Data: true}, nil
			}
		}
		right, err := e.expression(node.Right, module, sc)
		if err != nil {
			return Value{}, err
		}
		return e.binary(left, node.Operator, right, node.ExprType())
	case *ir.Range:
		start, err := e.expression(node.Start, module, sc)
		if err != nil {
			return Value{}, err
		}
		end, err := e.expression(node.End, module, sc)
		if err != nil {
			return Value{}, err
		}
		startValue, startOK := start.Data.(int64)
		endValue, endOK := end.Data.(int64)
		if !startOK || !endOK {
			return Value{}, errors.New("range endpoints must be Integer")
		}
		return Value{Type: node.ExprType(), Data: &rangeValue{Start: startValue, End: endValue, Exclusive: node.Exclusive}}, nil
	case *ir.Member:
		if node.Reference != nil && node.Reference.Intrinsic != "" && node.Reference.ExportKind == "property" {
			receiver, err := e.expression(node.Receiver, module, sc)
			if err != nil {
				return Value{}, err
			}
			value, handled, err := e.runtimeCall(runtimeInvocation{
				Name: node.Reference.Intrinsic, Arguments: []evaluatedArgument{{Value: receiver}},
				Type: node.ExprType(), MemberName: node.Name,
			})
			if handled {
				return value, err
			}
		}
		if node.Reference != nil && node.Reference.Intrinsic != "" {
			return Value{Type: node.ExprType(), Data: &callable{Intrinsic: node.Reference.Intrinsic, Module: module}}, nil
		}
		if node.Reference != nil && node.Reference.Package != "" {
			if value, ok := e.symbol(node.Reference.Package, node.Reference.Symbol); ok {
				return value, nil
			}
		}
		receiver, err := e.expression(node.Receiver, module, sc)
		if err != nil {
			return Value{}, err
		}
		if node.Safe && receiver.Data == nil {
			return Value{Type: node.ExprType(), Data: nil}, nil
		}
		return e.member(receiver, node.Name, module)
	case *ir.Call:
		reference := expressionReference(node.Callee)
		arguments := make([]evaluatedArgument, 0, len(node.Arguments)+1)
		if reference != nil && reference.Intrinsic != "" && (reference.ReceiverMethod || e.runtimeHandles(reference.Intrinsic)) {
			var member *ir.Member
			switch callee := node.Callee.(type) {
			case *ir.Member:
				member = callee
			case *ir.TypeApply:
				member, _ = callee.Receiver.(*ir.Member)
			}
			if member != nil {
				receiver, err := e.expression(member.Receiver, module, sc)
				if err != nil {
					return Value{}, err
				}
				arguments = append(arguments, evaluatedArgument{Value: receiver})
			}
		}
		for _, argument := range node.Arguments {
			value, err := e.expression(argument.Value, module, sc)
			if err != nil {
				return Value{}, err
			}
			arguments = append(arguments, evaluatedArgument{Name: argument.Name, Value: value})
		}
		if reference != nil && reference.Intrinsic != "" {
			return e.intrinsicCall(reference.Intrinsic, arguments, node.ExprType(), node.Codec, node)
		}
		var callee Value
		var err error
		if member, ok := node.Callee.(*ir.Member); ok && member.Reference != nil && member.Reference.Package != "" {
			callee, err = e.expression(node.Callee, module, sc)
		} else if member, ok := node.Callee.(*ir.Member); ok {
			receiver, receiverErr := e.expression(member.Receiver, module, sc)
			if receiverErr != nil {
				return Value{}, receiverErr
			}
			callee, err = e.callMember(receiver, member.Name, module)
		} else {
			callee, err = e.expression(node.Callee, module, sc)
		}
		if err != nil {
			return Value{}, err
		}
		function, ok := callee.Data.(*callable)
		if !ok {
			return Value{}, fmt.Errorf("%s is not callable", Inspect(callee))
		}
		if function.Intrinsic != "" {
			return e.intrinsicCall(function.Intrinsic, arguments, node.ExprType(), nil, node)
		}
		return e.call(function, arguments)
	case *ir.EnumCall:
		return e.enumCall(node, module, sc)
	case *ir.EnumConstruct:
		definitionModule := module
		if node.Reference != nil && node.Reference.Package != "" {
			definitionModule = node.Reference.Package
		}
		symbol, ok := e.symbol(definitionModule, node.EnumName)
		if !ok {
			return Value{}, fmt.Errorf("enum %s is not available in the REPL environment", node.EnumName)
		}
		typeDefinition, ok := symbol.Data.(*typeValue)
		if !ok || typeDefinition.Enum == nil {
			return Value{}, fmt.Errorf("%s is not an enum", node.EnumName)
		}
		member := typeDefinition.Enum.Members[node.Member]
		if member == nil {
			return Value{}, fmt.Errorf("enum %s has no member %s", node.EnumName, node.Member)
		}
		payload := map[string]Value{}
		for index, field := range member.Fields {
			value, err := e.expression(node.Arguments[index], module, sc)
			if err != nil {
				return Value{}, err
			}
			payload[field.Name] = value
		}
		return Value{Type: node.ExprType(), Data: &enumValue{Definition: typeDefinition.Enum, Name: node.Member, Payload: payload}}, nil
	case *ir.TypeApply:
		return e.expression(node.Receiver, module, sc)
	case *ir.Index:
		receiver, err := e.expression(node.Receiver, module, sc)
		if err != nil {
			return Value{}, err
		}
		index, err := e.expression(node.Index, module, sc)
		if err != nil {
			return Value{}, err
		}
		return indexValue(receiver, index, node.ExprType())
	case *ir.Transform:
		return e.transform(node, module, sc)
	case *ir.Block:
		return Value{Type: node.ExprType(), Data: node}, nil
	case *ir.NativeExpression:
		return Value{}, fmt.Errorf("native %s expression is not executable by the typed IR REPL", e.mode)
	default:
		return Value{}, fmt.Errorf("unsupported REPL expression %T", expression)
	}
}

func (e *Evaluator) transform(node *ir.Transform, module string, sc *scope) (Value, error) {
	source, err := e.expression(node.Source, module, sc)
	if err != nil {
		return Value{}, err
	}
	items, err := iterableValues(source)
	if err != nil {
		return Value{}, err
	}
	if node.Operation == "reduce" {
		accumulator, err := e.expression(node.Initial, module, sc)
		if err != nil {
			return Value{}, err
		}
		for _, item := range items {
			if err := e.checkContext(); err != nil {
				return Value{}, err
			}
			iterationScope := &scope{parent: sc, values: map[string]Value{}}
			iterationScope.values[node.Accumulator] = accumulator
			iterationScope.values[node.Item] = item
			accumulator, err = e.expression(node.Result, module, iterationScope)
			if err != nil {
				return Value{}, err
			}
		}
		accumulator.Type = node.ExprType()
		return accumulator, nil
	}
	if node.Operation == "sort_by" || node.Operation == "sort_by_descending" {
		type decoratedValue struct {
			value Value
			key   Value
		}
		decorated := make([]decoratedValue, 0, len(items))
		for _, item := range items {
			if err := e.checkContext(); err != nil {
				return Value{}, err
			}
			iterationScope := &scope{parent: sc, values: map[string]Value{node.Item: item}}
			key, err := e.expression(node.Result, module, iterationScope)
			if err != nil {
				return Value{}, err
			}
			decorated = append(decorated, decoratedValue{value: item, key: key})
		}
		var compareErr error
		descending := node.Operation == "sort_by_descending"
		sort.SliceStable(decorated, func(left, right int) bool {
			compared, err := comparePortableValues(decorated[left].key, decorated[right].key)
			if err != nil {
				compareErr = err
				return false
			}
			if descending {
				return compared > 0
			}
			return compared < 0
		})
		if compareErr != nil {
			return Value{}, compareErr
		}
		result := &arrayValue{Items: make([]Value, len(decorated))}
		for index, item := range decorated {
			result.Items[index] = item.value
		}
		return Value{Type: node.ExprType(), Data: result}, nil
	}

	result := &arrayValue{}
	for index, item := range items {
		if err := e.checkContext(); err != nil {
			return Value{}, err
		}
		iterationScope := &scope{parent: sc, values: map[string]Value{node.Item: item}}
		if node.WithIndex {
			iterationScope.values[node.Index] = Value{Type: types.FromName("Integer"), Data: int64(index)}
		}
		value, err := e.expression(node.Result, module, iterationScope)
		if err != nil {
			return Value{}, err
		}
		if node.Operation == "select" {
			selected, ok := value.Data.(bool)
			if !ok {
				return Value{}, errors.New("select block result must be Boolean")
			}
			if selected {
				result.Items = append(result.Items, item)
			}
			continue
		}
		if node.Operation == "any?" || node.Operation == "all?" || node.Operation == "none?" {
			matched, ok := value.Data.(bool)
			if !ok {
				return Value{}, fmt.Errorf("%s block result must be Boolean", node.Operation)
			}
			if node.Operation == "any?" && matched || node.Operation == "all?" && !matched || node.Operation == "none?" && matched {
				answer := node.Operation == "any?"
				return Value{Type: node.ExprType(), Data: answer}, nil
			}
			continue
		}
		if node.Operation == "find" || node.Operation == "find_index" {
			matched, ok := value.Data.(bool)
			if !ok {
				return Value{}, fmt.Errorf("%s block result must be Boolean", node.Operation)
			}
			if matched {
				if node.Operation == "find_index" {
					return Value{Type: node.ExprType(), Data: int64(index)}, nil
				}
				item.Type = node.ExprType()
				return item, nil
			}
			continue
		}
		result.Items = append(result.Items, value)
	}
	if node.Operation == "any?" {
		return Value{Type: node.ExprType(), Data: false}, nil
	}
	if node.Operation == "all?" || node.Operation == "none?" {
		return Value{Type: node.ExprType(), Data: true}, nil
	}
	if node.Operation == "find" || node.Operation == "find_index" {
		return Value{Type: node.ExprType(), Data: nil}, nil
	}
	return Value{Type: node.ExprType(), Data: result}, nil
}

func (e *Evaluator) iterate(node *ir.Iterate, module string, sc *scope) (flowResult, error) {
	source, err := e.expression(node.Source, module, sc)
	if err != nil {
		return flowResult{}, err
	}
	if node.Intrinsic != "" && e.runtimeHandles(node.Intrinsic) {
		return e.runtimeIterate(node, source, module, sc)
	}
	if hash, ok := source.Data.(*hashValue); ok {
		entries := append([]hashEntry(nil), hash.Entries...)
		for _, entry := range entries {
			if err := e.checkContext(); err != nil {
				return flowResult{}, err
			}
			iterationScope := &scope{parent: sc, values: map[string]Value{}}
			values := []Value{entry.Key, entry.Value}
			for index, binding := range node.Bindings {
				if index >= len(values) {
					break
				}
				value := values[index]
				value.Type = binding.Type
				iterationScope.values[binding.Name] = value
			}
			result, err := e.evaluate(node.Body, module, iterationScope)
			if err != nil || result.Returned {
				return result, err
			}
			switch result.Loop {
			case loopBreak:
				result.Loop = loopNone
				return result, nil
			case loopNext:
				continue
			}
		}
		return flowResult{}, nil
	}
	items, err := iterableValues(source)
	if err != nil {
		return flowResult{}, err
	}
	size := 1
	if node.Operation == "each_slice" {
		value, err := e.expression(node.SliceSize, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		integer, ok := value.Data.(int64)
		if !ok || integer <= 0 {
			return flowResult{}, errors.New("each_slice size must be greater than zero")
		}
		size = int(integer)
	}
	iterationIndex := 0
	itemBinding := node.Bindings[0]
	for offset := 0; offset < len(items); offset += size {
		if err := e.checkContext(); err != nil {
			return flowResult{}, err
		}
		iterationScope := &scope{parent: sc, values: map[string]Value{}}
		if node.Operation == "each_slice" {
			end := offset + size
			if end > len(items) {
				end = len(items)
			}
			slice := append([]Value(nil), items[offset:end]...)
			iterationScope.values[itemBinding.Name] = Value{Type: itemBinding.Type, Data: &arrayValue{Items: slice}}
		} else {
			item := items[offset]
			item.Type = itemBinding.Type
			iterationScope.values[itemBinding.Name] = item
		}
		if node.WithIndex {
			iterationScope.values[node.Bindings[1].Name] = Value{Type: node.Bindings[1].Type, Data: int64(iterationIndex)}
		}
		result, err := e.evaluate(node.Body, module, iterationScope)
		if err != nil || result.Returned {
			return result, err
		}
		switch result.Loop {
		case loopBreak:
			result.Loop = loopNone
			return result, nil
		case loopNext:
			iterationIndex++
			continue
		}
		iterationIndex++
	}
	return flowResult{}, nil
}

func (e *Evaluator) runtimeIterate(node *ir.Iterate, source Value, module string, sc *scope) (flowResult, error) {
	batchSize := int64(1000)
	if node.SliceSize != nil {
		value, err := e.expression(node.SliceSize, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		var ok bool
		batchSize, ok = value.Data.(int64)
		if !ok {
			return flowResult{}, errors.New("structured iteration batch size must be an Integer")
		}
	}
	evaluate := func(value Value) (flowResult, error) {
		iterationScope := &scope{parent: sc, values: map[string]Value{}}
		if len(node.Bindings) > 0 && node.Bindings[0].Name != "_" {
			value.Type = node.Bindings[0].Type
			iterationScope.values[node.Bindings[0].Name] = value
		}
		return e.evaluate(node.Body, module, iterationScope)
	}
	successType := types.FromName("Integer")
	if node.EffectSuccess.Kind != "" {
		successType = node.EffectSuccess
	}
	rawType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{successType, node.Fails}}
	value, escaped, handled, err := e.runtimeIteration(runtimeIterationInvocation{
		Name: node.Intrinsic, Source: source, BatchSize: batchSize, Type: rawType, Iteration: node, Evaluate: evaluate,
	})
	if err != nil || !handled {
		if err == nil {
			err = fmt.Errorf("runtime intrinsic %s is not executable in the REPL", node.Intrinsic)
		}
		return flowResult{}, err
	}
	if escaped.Returned {
		return escaped, nil
	}
	if node.Result == nil {
		return flowResult{}, nil
	}
	if node.CaptureEffect {
		value.Type = node.Result.Type
	} else {
		variant, ok := value.Data.(*enumValue)
		if !ok {
			return flowResult{}, errors.New("fallible structured iteration did not return Result")
		}
		if variant.Name == "Err" {
			if node.UnhandledEffect {
				errorValue, exists := variant.Payload["error"]
				if !exists {
					return flowResult{}, errors.New("fallible structured iteration returned Err without an error")
				}
				return flowResult{}, &unhandledEffect{typ: node.Fails, value: errorValue}
			}
			return flowResult{Result: Result{Value: value}, Returned: true}, nil
		}
		if variant.Name != "Ok" {
			return flowResult{}, fmt.Errorf("fallible structured iteration returned unknown Result variant %s", variant.Name)
		}
		var exists bool
		value, exists = variant.Payload["value"]
		if !exists {
			return flowResult{}, errors.New("fallible structured iteration returned Ok without a value")
		}
		value.Type = node.Result.Type
	}
	if node.Result.Variable != nil {
		variable := node.Result.Variable
		sc.values[variable.Name] = value
		e.moduleValue[symbolKey(module, ownedName(variable.Owner, variable.Name))] = value
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	}
	if node.Result.Target != nil {
		if err := e.assign(node.Result.Target, value, module, sc); err != nil {
			return flowResult{}, err
		}
		return flowResult{Result: Result{Value: value, Display: true}}, nil
	}
	if node.Result.Return {
		return flowResult{Result: Result{Value: value}, Returned: true}, nil
	}
	return flowResult{}, nil
}

func iterableValues(value Value) ([]Value, error) {
	switch data := value.Data.(type) {
	case *arrayValue:
		return data.Items, nil
	case *rangeValue:
		items := []Value{}
		for current := data.Start; current < data.End; current++ {
			if len(items) >= 1_000_000 {
				return nil, errors.New("range exceeded REPL iteration limit")
			}
			items = append(items, Value{Type: types.FromName("Integer"), Data: current})
		}
		if !data.Exclusive && data.Start <= data.End {
			if len(items) >= 1_000_000 {
				return nil, errors.New("range exceeded REPL iteration limit")
			}
			items = append(items, Value{Type: types.FromName("Integer"), Data: data.End})
		}
		return items, nil
	default:
		return nil, fmt.Errorf("%s is not iterable", value.Type)
	}
}

func (e *Evaluator) callMember(receiver Value, name, module string) (Value, error) {
	switch value := receiver.Data.(type) {
	case *objectInstance:
		if method := classMethod(value.Definition, name, false); method != nil {
			return Value{Type: method.ReturnType, Data: &callable{Method: method, Receiver: receiver, Module: value.Definition.Module}}, nil
		}
	case *typeValue:
		if name == "new" && (value.Record != nil || value.Class != nil) {
			return Value{Type: receiver.Type, Data: &callable{Construct: value, Module: module}}, nil
		}
		if value.Class != nil {
			if method := classMethod(value.Class, name, true); method != nil {
				return Value{Type: method.ReturnType, Data: &callable{Method: method, Receiver: receiver, Module: value.Class.Module}}, nil
			}
		}
	}
	return e.member(receiver, name, module)
}

type evaluatedArgument struct {
	Name  string
	Value Value
}

func (e *Evaluator) symbol(module, name string) (Value, bool) {
	if value, ok := e.moduleValue[symbolKey(module, name)]; ok {
		return value, true
	}
	definition := e.definitions[symbolKey(module, name)]
	if definition == nil && strings.HasSuffix(module, "/index") {
		definition = e.definitions[symbolKey(strings.TrimSuffix(module, "/index"), name)]
	}
	switch item := definition.(type) {
	case *recordDefinition:
		return Value{Type: types.FromName("Class"), Data: &typeValue{Record: item}}, true
	case *classDefinition:
		return Value{Type: types.FromName("Class"), Data: &typeValue{Class: item}}, true
	case *enumDefinition:
		return Value{Type: types.FromName(item.Node.Name), Data: &typeValue{Enum: item}}, true
	case *functionDefinition:
		return Value{Type: item.Method.ReturnType, Data: &callable{Function: item, Module: item.Module}}, true
	}
	return Value{}, false
}

func (e *Evaluator) selfValue(name string, sc *scope) (Value, error) {
	self, ok := sc.get("self")
	if !ok {
		return Value{}, errors.New("self is not available here")
	}
	if name == "self" {
		return self, nil
	}
	object, ok := self.Data.(*objectInstance)
	if !ok {
		return Value{}, fmt.Errorf("%s is not an object field", name)
	}
	value, ok := object.Fields[name]
	if !ok {
		return Value{}, fmt.Errorf("field %s is not initialized", name)
	}
	return value, nil
}

func (e *Evaluator) member(receiver Value, name, module string) (Value, error) {
	switch value := receiver.Data.(type) {
	case *recordInstance:
		field, ok := value.Fields[name]
		if !ok {
			return Value{}, fmt.Errorf("record %s has no field %s", value.Definition.Node.Name, name)
		}
		return field, nil
	case *objectInstance:
		if field, ok := value.Fields["@"+name]; ok {
			return field, nil
		}
		method := classMethod(value.Definition, name, false)
		if method == nil {
			return Value{}, fmt.Errorf("class %s has no member %s", value.Definition.Node.Name, name)
		}
		return Value{Type: method.ReturnType, Data: &callable{Method: method, Receiver: receiver, Module: value.Definition.Module}}, nil
	case *typeValue:
		if name == "new" && (value.Record != nil || value.Class != nil) {
			return Value{Type: receiver.Type, Data: &callable{Construct: value, Module: module}}, nil
		}
		if value.Class != nil {
			method := classMethod(value.Class, name, true)
			if method != nil {
				return Value{Type: method.ReturnType, Data: &callable{Method: method, Receiver: receiver, Module: value.Class.Module}}, nil
			}
		}
		if value.Enum != nil {
			if member := value.Enum.Members[name]; member != nil && len(member.Fields) == 0 {
				return Value{Type: types.FromName(value.Enum.Node.Name), Data: &enumValue{Definition: value.Enum, Name: name, Payload: map[string]Value{}}}, nil
			}
		}
	}
	return Value{}, fmt.Errorf("%s has no member %s", receiver.Type, name)
}

func (e *Evaluator) call(function *callable, arguments []evaluatedArgument) (Value, error) {
	if function.Adapted != nil {
		value, err := e.call(function.Adapted, arguments)
		if err != nil {
			return Value{}, err
		}
		success := function.Success
		if success.Kind == types.Void {
			value, err = e.unitValue()
			if err != nil {
				return Value{}, err
			}
			success = value.Type
		}
		resultType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{success, function.Failure}}
		return e.resultOK(resultType, value)
	}
	if function.Lambda != nil {
		closure := function.Lambda
		callScope := &scope{parent: closure.Scope, values: map[string]Value{}}
		if err := e.bind(callScope, closure.Node.Parameters, arguments, closure.Module); err != nil {
			return Value{}, err
		}
		result, err := e.evaluate(closure.Node.Body, closure.Module, callScope)
		if err != nil {
			return Value{}, err
		}
		if result.Returned {
			return result.Value, nil
		}
		return Value{Type: closure.Node.ReturnType}, nil
	}
	if function.Construct != nil {
		if function.Construct.Record != nil {
			return e.constructRecord(function.Construct.Record, arguments)
		}
		return e.constructClass(function.Construct.Class, arguments)
	}
	method := function.Method
	module := function.Module
	if function.Function != nil {
		method = function.Function.Method
		module = function.Function.Module
	}
	if method == nil {
		return Value{}, errors.New("invalid callable")
	}
	callScope := &scope{parent: e.global, values: map[string]Value{}}
	if function.Receiver.Data != nil {
		callScope.values["self"] = function.Receiver
	}
	if err := e.bind(callScope, method.Parameters, arguments, module); err != nil {
		return Value{}, err
	}
	result, err := e.evaluate(method.Body, module, callScope)
	if err != nil {
		return Value{}, err
	}
	if result.Returned {
		return result.Value, nil
	}
	return Value{Type: method.ReturnType}, nil
}

func (e *Evaluator) enumCall(node *ir.EnumCall, module string, sc *scope) (Value, error) {
	arguments := make([]evaluatedArgument, len(node.Arguments))
	for index, argument := range node.Arguments {
		value, err := e.expression(argument.Value, module, sc)
		if err != nil {
			return Value{}, err
		}
		arguments[index] = evaluatedArgument{Name: argument.Name, Value: value}
	}
	if node.Method == "from_raw" {
		definitionModule := module
		if node.Reference != nil && node.Reference.Package != "" {
			definitionModule = node.Reference.Package
		}
		symbol, ok := e.symbol(definitionModule, node.EnumName)
		if !ok {
			return Value{}, fmt.Errorf("enum %s is not available in the REPL environment", node.EnumName)
		}
		typeValue, ok := symbol.Data.(*typeValue)
		if !ok || typeValue.Enum == nil {
			return Value{}, fmt.Errorf("%s is not an enum", node.EnumName)
		}
		for _, raw := range node.RawValues {
			member := typeValue.Enum.Members[raw.Member]
			if member == nil || member.RawValue == nil {
				continue
			}
			rawValue, err := e.expression(member.RawValue, definitionModule, e.global)
			if err != nil {
				return Value{}, err
			}
			if equal(arguments[0].Value, rawValue) {
				value := Value{Type: types.FromName(node.EnumName), Data: &enumValue{Definition: typeValue.Enum, Name: raw.Member, Payload: map[string]Value{}}}
				return e.resultOK(node.ExprType(), value)
			}
		}
		return e.structuredResultErr(node.ExprType(), "EnumValueError", map[string]Value{
			"value":   arguments[0].Value,
			"message": {Type: types.FromName("String"), Data: "unknown raw value for " + node.EnumName},
		})
	}
	receiver, err := e.expression(node.Receiver, module, sc)
	if err != nil {
		return Value{}, err
	}
	variant, ok := receiver.Data.(*enumValue)
	if !ok {
		return Value{}, fmt.Errorf("%s is not an enum value", receiver.Type)
	}
	if node.Method == "raw_value" {
		member := variant.Definition.Members[variant.Name]
		if member == nil || member.RawValue == nil {
			return Value{}, fmt.Errorf("enum %s has no raw value", variant.Definition.Node.Name)
		}
		return e.expression(member.RawValue, variant.Definition.Module, e.global)
	}
	method := variant.Definition.Methods[node.Method]
	if method == nil {
		return Value{}, fmt.Errorf("enum %s has no method %s", variant.Definition.Node.Name, node.Method)
	}
	return e.call(&callable{Method: method, Receiver: receiver, Module: variant.Definition.Module}, arguments)
}

func (e *Evaluator) bind(sc *scope, parameters []ir.Parameter, arguments []evaluatedArgument, module string) error {
	positional := 0
	used := map[int]bool{}
	for _, parameter := range parameters {
		found := -1
		if parameter.Keyword {
			for index, argument := range arguments {
				if argument.Name == parameter.Name {
					found = index
					break
				}
			}
		} else {
			for positional < len(arguments) && arguments[positional].Name != "" {
				positional++
			}
			if positional < len(arguments) {
				found = positional
				positional++
			}
		}
		if found >= 0 {
			used[found] = true
			sc.values[parameter.Name] = arguments[found].Value
			continue
		}
		if parameter.Default != nil {
			value, err := e.expression(parameter.Default, module, sc)
			if err != nil {
				return err
			}
			sc.values[parameter.Name] = value
			continue
		}
		if parameter.Rest || parameter.KeywordRest {
			values := []Value{}
			for index, argument := range arguments {
				if !used[index] {
					values = append(values, argument.Value)
					used[index] = true
				}
			}
			sc.values[parameter.Name] = Value{Type: parameter.Type, Data: &arrayValue{Items: values}}
			continue
		}
		return fmt.Errorf("missing argument %s", parameter.Name)
	}
	for index := range arguments {
		if !used[index] {
			return errors.New("too many arguments")
		}
	}
	return nil
}

func (e *Evaluator) constructRecord(definition *recordDefinition, arguments []evaluatedArgument) (Value, error) {
	fields := map[string]Value{}
	for _, field := range definition.Fields {
		found := false
		for _, argument := range arguments {
			if argument.Name == field.Name {
				value := argument.Value
				value.Type = field.Type
				fields[field.Name] = value
				found = true
				break
			}
		}
		if !found {
			return Value{}, fmt.Errorf("missing record field %s", field.Name)
		}
	}
	return Value{Type: types.FromName(definition.Node.Name), Data: &recordInstance{Definition: definition, Fields: fields}}, nil
}

func (e *Evaluator) constructClass(definition *classDefinition, arguments []evaluatedArgument) (Value, error) {
	if definition == nil {
		return Value{}, errors.New("invalid class constructor")
	}
	object := &objectInstance{Definition: definition, Fields: map[string]Value{}}
	value := Value{Type: types.FromName(definition.Node.Name), Data: object}
	classScope := &scope{parent: e.global, values: map[string]Value{"self": value}}
	for _, field := range allFields(definition) {
		fieldValue := Value{Type: field.Type}
		if field.Value != nil {
			var err error
			fieldValue, err = e.expression(field.Value, definition.Module, classScope)
			if err != nil {
				return Value{}, err
			}
		}
		object.Fields[field.Name] = fieldValue
	}
	if initialize := classMethod(definition, "initialize", false); initialize != nil {
		_, err := e.call(&callable{Method: initialize, Receiver: value, Module: definition.Module}, arguments)
		if err != nil {
			return Value{}, err
		}
	} else if len(arguments) > 0 {
		return Value{}, errors.New("constructor does not accept arguments")
	}
	return value, nil
}

func allFields(definition *classDefinition) []*ir.Field {
	if definition == nil {
		return nil
	}
	return append(allFields(definition.Superclass), definition.Fields...)
}

func classMethod(definition *classDefinition, name string, class bool) *ir.Method {
	for current := definition; current != nil; current = current.Superclass {
		if method := current.Methods[name]; method != nil && method.Class == class {
			return method
		}
	}
	return nil
}

func (e *Evaluator) assign(target ir.Expression, value Value, module string, sc *scope) error {
	switch node := target.(type) {
	case *ir.Identifier:
		if strings.HasPrefix(node.Name, "@") {
			self, ok := sc.get("self")
			if !ok {
				return errors.New("self is not available here")
			}
			object, ok := self.Data.(*objectInstance)
			if !ok {
				return fmt.Errorf("cannot assign %s on %s", node.Name, self.Type)
			}
			object.Fields[node.Name] = value
			return nil
		}
		if !sc.assign(node.Name, value) {
			sc.values[node.Name] = value
		}
		e.moduleValue[symbolKey(module, ownedName(node.Owner, node.Name))] = value
		return nil
	case *ir.Member:
		receiver, err := e.expression(node.Receiver, module, sc)
		if err != nil {
			return err
		}
		switch item := receiver.Data.(type) {
		case *recordInstance:
			item.Fields[node.Name] = value
			return nil
		case *objectInstance:
			item.Fields["@"+node.Name] = value
			return nil
		}
	case *ir.Index:
		receiver, err := e.expression(node.Receiver, module, sc)
		if err != nil {
			return err
		}
		index, err := e.expression(node.Index, module, sc)
		if err != nil {
			return err
		}
		return assignIndex(receiver, index, value)
	}
	return fmt.Errorf("cannot assign to %T", target)
}

func (e *Evaluator) binary(left Value, operator string, right Value, typ types.Type) (Value, error) {
	switch operator {
	case "==":
		return Value{Type: typ, Data: equal(left, right)}, nil
	case "!=":
		return Value{Type: typ, Data: !equal(left, right)}, nil
	case "and", "&&":
		return Value{Type: typ, Data: truthy(left) && truthy(right)}, nil
	case "or", "||":
		return Value{Type: typ, Data: truthy(left) || truthy(right)}, nil
	}
	if leftString, ok := left.Data.(string); ok {
		rightString, rightOK := right.Data.(string)
		if operator == "+" && rightOK {
			return Value{Type: typ, Data: leftString + rightString}, nil
		}
		if rightOK {
			return comparison(strings.Compare(leftString, rightString), operator, typ)
		}
	}
	leftNumber, leftFloat, leftOK := number(left)
	rightNumber, rightFloat, rightOK := number(right)
	if !leftOK || !rightOK {
		return Value{}, fmt.Errorf("operator %s does not support %s and %s", operator, left.Type, right.Type)
	}
	useFloat := leftFloat || rightFloat || typ.Kind == types.Float
	switch operator {
	case "+":
		return numericValue(leftNumber+rightNumber, useFloat, typ), nil
	case "-":
		return numericValue(leftNumber-rightNumber, useFloat, typ), nil
	case "*":
		return numericValue(leftNumber*rightNumber, useFloat, typ), nil
	case "/":
		if rightNumber == 0 {
			return Value{}, errors.New("division by zero")
		}
		if useFloat {
			return numericValue(leftNumber/rightNumber, true, typ), nil
		}
		return numericValue(math.Trunc(leftNumber/rightNumber), false, typ), nil
	case "%":
		if rightNumber == 0 {
			return Value{}, errors.New("division by zero")
		}
		return numericValue(math.Mod(leftNumber, rightNumber), useFloat, typ), nil
	case "**":
		if typ.Kind == types.Int && rightNumber < 0 {
			return Value{}, errors.New("negative Integer exponent")
		}
		return numericValue(math.Pow(leftNumber, rightNumber), useFloat, typ), nil
	case "<":
		return Value{Type: typ, Data: leftNumber < rightNumber}, nil
	case "<=":
		return Value{Type: typ, Data: leftNumber <= rightNumber}, nil
	case ">":
		return Value{Type: typ, Data: leftNumber > rightNumber}, nil
	case ">=":
		return Value{Type: typ, Data: leftNumber >= rightNumber}, nil
	}
	return Value{}, fmt.Errorf("unsupported operator %s", operator)
}

func comparison(value int, operator string, typ types.Type) (Value, error) {
	switch operator {
	case "<":
		return Value{Type: typ, Data: value < 0}, nil
	case "<=":
		return Value{Type: typ, Data: value <= 0}, nil
	case ">":
		return Value{Type: typ, Data: value > 0}, nil
	case ">=":
		return Value{Type: typ, Data: value >= 0}, nil
	}
	return Value{}, fmt.Errorf("unsupported string operator %s", operator)
}

func validUnicodeScalar(value int64) bool {
	return value >= 0 && value <= utf8.MaxRune && (value < 0xd800 || value > 0xdfff)
}

func (e *Evaluator) resultOK(resultType types.Type, value Value) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/result")
	}
	if len(resultType.Args) == 2 {
		value.Type = resultType.Args[0]
	}
	return Value{
		Type: resultType,
		Data: &enumValue{
			Definition: definition,
			Name:       "Ok",
			Payload:    map[string]Value{"value": value},
		},
	}, nil
}

func (e *Evaluator) filesystemOK(resultType types.Type, value Value) (Value, error) {
	return e.resultOK(resultType, value)
}

func (e *Evaluator) unitValue() (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/std/unit/index", "Unit")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/unit")
	}
	return Value{Type: types.FromName("Unit"), Data: &recordInstance{Definition: definition, Fields: map[string]Value{}}}, nil
}

func (e *Evaluator) structuredResultErr(resultType types.Type, name string, fields map[string]Value) (Value, error) {
	return e.structuredResultErrFrom(resultType, "trb/std/errors/index", name, fields)
}

func (e *Evaluator) structuredResultErrFrom(resultType types.Type, modulePath, name string, fields map[string]Value) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/result")
	}
	errorDefinition, ok := e.definitions[symbolKey(modulePath, name)].(*recordDefinition)
	if !ok {
		return Value{}, fmt.Errorf("operation requires %s", strings.TrimSuffix(modulePath, "/index"))
	}
	errorType := types.FromName(name)
	if len(resultType.Args) == 2 {
		errorType = resultType.Args[1]
	}
	errorValue := Value{Type: errorType, Data: &recordInstance{Definition: errorDefinition, Fields: fields}}
	return Value{Type: resultType, Data: &enumValue{Definition: definition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
}

func (e *Evaluator) numberParseResultErr(resultType types.Type, kind, input, message string) (Value, error) {
	kindDefinition, ok := e.definitions[symbolKey("trb/std/errors/index", "NumberParseErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/errors")
	}
	kindValue := Value{Type: types.FromName("NumberParseErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	return e.structuredResultErr(resultType, "NumberParseError", map[string]Value{
		"kind":    kindValue,
		"input":   {Type: types.FromName("String"), Data: input},
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func (e *Evaluator) hexDecodeResultErr(resultType types.Type, kind, input string, index int64, message string) (Value, error) {
	kindDefinition, ok := e.definitions[symbolKey("trb/std/encoding/hex/index", "HexDecodeErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/encoding/hex")
	}
	kindValue := Value{Type: types.FromName("HexDecodeErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	return e.structuredResultErrFrom(resultType, "trb/std/encoding/hex/index", "HexDecodeError", map[string]Value{
		"kind":    kindValue,
		"input":   {Type: types.FromName("String"), Data: input},
		"index":   {Type: types.FromName("Integer"), Data: index},
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func (e *Evaluator) base64DecodeResultErr(resultType types.Type, kind, input string, index int64, message string) (Value, error) {
	kindDefinition, ok := e.definitions[symbolKey("trb/std/encoding/base64/index", "Base64DecodeErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/encoding/base64")
	}
	kindValue := Value{Type: types.FromName("Base64DecodeErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	return e.structuredResultErrFrom(resultType, "trb/std/encoding/base64/index", "Base64DecodeError", map[string]Value{
		"kind":    kindValue,
		"input":   {Type: types.FromName("String"), Data: input},
		"index":   {Type: types.FromName("Integer"), Data: index},
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func (e *Evaluator) percentDecodeResultErr(resultType types.Type, kind, input, message string) (Value, error) {
	kindDefinition, ok := e.definitions[symbolKey("trb/std/url/index", "PercentDecodeErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/url")
	}
	kindValue := Value{Type: types.FromName("PercentDecodeErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	return e.structuredResultErrFrom(resultType, "trb/std/url/index", "PercentDecodeError", map[string]Value{
		"kind":    kindValue,
		"input":   {Type: types.FromName("String"), Data: input},
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func encodeURLComponent(value string) string {
	const hexadecimal = "0123456789ABCDEF"
	var builder strings.Builder
	for _, octet := range []byte(value) {
		if urlUnreserved(octet) {
			builder.WriteByte(octet)
			continue
		}
		builder.WriteByte('%')
		builder.WriteByte(hexadecimal[octet>>4])
		builder.WriteByte(hexadecimal[octet&15])
	}
	return builder.String()
}

func decodeURLComponent(input string) (string, string, string) {
	characters := []rune(input)
	value := make([]byte, 0, len(input))
	for index := 0; index < len(characters); index++ {
		character := characters[index]
		if character != '%' {
			value = append(value, []byte(string(character))...)
			continue
		}
		if index+2 >= len(characters) {
			return "", "InvalidEscape", "invalid percent escape in URL component"
		}
		high, highOK := hexadecimalValue(characters[index+1])
		low, lowOK := hexadecimalValue(characters[index+2])
		if !highOK || !lowOK {
			return "", "InvalidEscape", "invalid percent escape in URL component"
		}
		value = append(value, high<<4|low)
		index += 2
	}
	if !utf8.Valid(value) {
		return "", "InvalidUtf8", "decoded URL component is not valid UTF-8"
	}
	return string(value), "", ""
}

func urlUnreserved(value byte) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' || value == '.' || value == '_' || value == '~'
}

func hexadecimalValue(value rune) (byte, bool) {
	switch {
	case value >= '0' && value <= '9':
		return byte(value - '0'), true
	case value >= 'A' && value <= 'F':
		return byte(value - 'A' + 10), true
	case value >= 'a' && value <= 'f':
		return byte(value - 'a' + 10), true
	default:
		return 0, false
	}
}

func decodeBase64(input string, urlSafe bool) ([]byte, string, int64, string) {
	characters := []rune(input)
	if urlSafe {
		if len(characters)%4 == 1 {
			return nil, "InvalidLength", int64(len(characters)), "base64url input has invalid length"
		}
		for index, character := range characters {
			if character == '=' {
				return nil, "InvalidPadding", int64(index), "base64url input must not contain padding"
			}
			if !base64URLCharacter(character) {
				return nil, "InvalidCharacter", int64(index), "invalid base64url character"
			}
		}
		value, err := stdbase64.RawURLEncoding.Strict().DecodeString(input)
		if err != nil {
			return nil, "NonCanonical", int64(len(characters) - 1), "non-canonical base64url encoding"
		}
		return value, "", 0, ""
	}

	if len(characters)%4 != 0 {
		return nil, "InvalidLength", int64(len(characters)), "base64 input length must be a multiple of 4"
	}
	padding := 0
	for index, character := range characters {
		if character == '=' {
			padding++
			if index < len(characters)-2 || padding > 2 {
				return nil, "InvalidPadding", int64(index), "invalid base64 padding"
			}
			continue
		}
		if padding > 0 {
			return nil, "InvalidPadding", int64(index), "invalid base64 padding"
		}
		if !base64StandardCharacter(character) {
			return nil, "InvalidCharacter", int64(index), "invalid base64 character"
		}
	}
	value, err := stdbase64.StdEncoding.Strict().DecodeString(input)
	if err != nil {
		return nil, "NonCanonical", int64(len(characters) - padding - 1), "non-canonical base64 encoding"
	}
	return value, "", 0, ""
}

func base64StandardCharacter(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '+' || value == '/'
}

func base64URLCharacter(value rune) bool {
	return value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' || value == '_'
}

func hexadecimalCharacter(value rune) bool {
	return value >= '0' && value <= '9' || value >= 'A' && value <= 'F' || value >= 'a' && value <= 'f'
}

func (e *Evaluator) indexLookupResultErr(resultType types.Type, index, size int64, message string) (Value, error) {
	return e.structuredResultErr(resultType, "IndexLookupError", map[string]Value{
		"index":   {Type: types.FromName("Integer"), Data: index},
		"size":    {Type: types.FromName("Integer"), Data: size},
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func (e *Evaluator) sliceRangeResultErr(resultType types.Type, bounds *rangeValue, size int64, message string) (Value, error) {
	return e.structuredResultErr(resultType, "SliceRangeError", map[string]Value{
		"start":     {Type: types.FromName("Integer"), Data: bounds.Start},
		"finish":    {Type: types.FromName("Integer"), Data: bounds.End},
		"exclusive": {Type: types.FromName("Boolean"), Data: bounds.Exclusive},
		"size":      {Type: types.FromName("Integer"), Data: size},
		"message":   {Type: types.FromName("String"), Data: message},
	})
}

func (e *Evaluator) keyLookupResultErr(resultType types.Type, key Value, message string) (Value, error) {
	key.Type = types.UnionOf(types.FromName("String"), types.FromName("Integer"))
	return e.structuredResultErr(resultType, "KeyLookupError", map[string]Value{
		"key":     key,
		"message": {Type: types.FromName("String"), Data: message},
	})
}

func parsePortableInteger(input string) (int64, string) {
	if input == "" {
		return 0, "invalid Integer"
	}
	start := 0
	if input[0] == '+' || input[0] == '-' {
		start = 1
	}
	if start == len(input) {
		return 0, "invalid Integer"
	}
	for index := start; index < len(input); index++ {
		if input[index] < '0' || input[index] > '9' {
			return 0, "invalid Integer"
		}
	}
	value, err := strconv.ParseInt(input, 10, 64)
	if err != nil || value < -9007199254740991 || value > 9007199254740991 {
		return 0, "Integer is outside the portable range"
	}
	return value, ""
}

func parsePortableFloat(input string) (float64, string) {
	if !isPortableFloatText(input) {
		return 0, "invalid Float"
	}
	value, err := strconv.ParseFloat(input, 64)
	if math.IsInf(value, 0) || (err != nil && value != 0) {
		return 0, "Float is outside the portable range"
	}
	return value, ""
}

func isPortableFloatText(input string) bool {
	if input == "" {
		return false
	}
	index := 0
	if input[index] == '+' || input[index] == '-' {
		index++
	}
	wholeDigits := 0
	for index < len(input) && input[index] >= '0' && input[index] <= '9' {
		wholeDigits++
		index++
	}
	fractionDigits := 0
	if index < len(input) && input[index] == '.' {
		index++
		for index < len(input) && input[index] >= '0' && input[index] <= '9' {
			fractionDigits++
			index++
		}
	}
	if wholeDigits == 0 && fractionDigits == 0 {
		return false
	}
	if index < len(input) && (input[index] == 'e' || input[index] == 'E') {
		index++
		if index < len(input) && (input[index] == '+' || input[index] == '-') {
			index++
		}
		exponentDigits := 0
		for index < len(input) && input[index] >= '0' && input[index] <= '9' {
			exponentDigits++
			index++
		}
		if exponentDigits == 0 {
			return false
		}
	}
	return index == len(input)
}

func portableFloatText(value float64) string {
	text := strconv.FormatFloat(value, 'f', -1, 64)
	switch {
	case math.IsNaN(value):
		return "NaN"
	case math.IsInf(value, 1):
		return "Infinity"
	case math.IsInf(value, -1):
		return "-Infinity"
	case value == 0:
		return "0.0"
	case !strings.Contains(text, "."):
		return text + ".0"
	default:
		return text
	}
}

func (e *Evaluator) filesystemErr(resultType types.Type, operation, path string, cause error) (Value, error) {
	resultDefinition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("filesystem requires trb/std/result")
	}
	fileErrorDefinition, ok := e.definitions[symbolKey("trb/std/filesystem/index", "FileError")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("filesystem runtime is not loaded")
	}
	errorType := types.FromName("FileError")
	if len(resultType.Args) == 2 {
		errorType = resultType.Args[1]
	}
	errorValue := Value{
		Type: errorType,
		Data: &recordInstance{
			Definition: fileErrorDefinition,
			Fields: map[string]Value{
				"operation": {Type: types.FromName("String"), Data: operation},
				"path":      {Type: types.FromName("String"), Data: path},
				"message":   {Type: types.FromName("String"), Data: cause.Error()},
			},
		},
	}
	return Value{
		Type: resultType,
		Data: &enumValue{
			Definition: resultDefinition,
			Name:       "Err",
			Payload:    map[string]Value{"error": errorValue},
		},
	}, nil
}

func (e *Evaluator) processErr(resultType types.Type, operation, command string, cause error) (Value, error) {
	resultDefinition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("process operation requires trb/std/result")
	}
	errorDefinition, ok := e.definitions[symbolKey("trb/std/process/index", "ProcessError")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("process operation requires trb/std/process")
	}
	errorValue := Value{Type: types.FromName("ProcessError"), Data: &recordInstance{Definition: errorDefinition, Fields: map[string]Value{
		"operation": {Type: types.FromName("String"), Data: operation},
		"command":   {Type: types.FromName("String"), Data: command},
		"message":   {Type: types.FromName("String"), Data: cause.Error()},
	}}}
	return Value{Type: resultType, Data: &enumValue{Definition: resultDefinition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
}

func (e *Evaluator) parseJSON(resultType types.Type, source string) (Value, error) {
	decoder := stdjson.NewDecoder(strings.NewReader(source))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		line, column := jsonSourceLocation(source, err)
		return e.jsonError(resultType, "Syntax", err.Error(), "", line, column)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			err = errors.New("JSON source contains multiple values")
		}
		line, column := jsonSourceLocation(source, err)
		return e.jsonError(resultType, "Syntax", err.Error(), "", line, column)
	}
	value, conversionErr := e.jsonValue(raw, "")
	if conversionErr != nil {
		return e.jsonError(resultType, "Decode", conversionErr.message, conversionErr.path, nil, nil)
	}
	return e.jsonOK(resultType, value)
}

func (e *Evaluator) stringifyJSON(resultType types.Type, value Value) (Value, error) {
	raw, conversionErr := jsonRaw(value, "")
	if conversionErr != nil {
		return e.jsonError(resultType, "Encode", conversionErr.message, conversionErr.path, nil, nil)
	}
	encoded, err := stdjson.Marshal(raw)
	if err != nil {
		return e.jsonError(resultType, "Encode", err.Error(), "", nil, nil)
	}
	return e.jsonOK(resultType, Value{Type: types.FromName("String"), Data: string(encoded)})
}

func (e *Evaluator) decodeJSONCodec(resultType types.Type, source string, schema *ir.CodecSchema) (Value, error) {
	parseType := types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("JsonValue"), types.FromName("JsonError")}}
	parsed, err := e.parseJSON(parseType, source)
	if err != nil {
		return Value{}, err
	}
	result, ok := parsed.Data.(*enumValue)
	if !ok {
		return Value{}, errors.New("json.decode parser returned an invalid Result")
	}
	if result.Name == "Err" {
		return e.jsonCodecErr(resultType, result.Payload["error"])
	}
	decoded, conversionErr := e.decodeJSONCodecValue(schema, result.Payload["value"], "")
	if conversionErr != nil {
		return e.jsonError(resultType, "Decode", conversionErr.message, conversionErr.path, nil, nil)
	}
	return e.jsonOK(resultType, decoded)
}

func (e *Evaluator) encodeJSONCodec(resultType types.Type, value Value, schema *ir.CodecSchema) (Value, error) {
	raw, conversionErr := jsonCodecRaw(schema, value, "")
	if conversionErr != nil {
		return e.jsonError(resultType, "Encode", conversionErr.message, conversionErr.path, nil, nil)
	}
	jsonValue, conversionErr := e.jsonValue(raw, "")
	if conversionErr != nil {
		return e.jsonError(resultType, "Encode", conversionErr.message, conversionErr.path, nil, nil)
	}
	return e.stringifyJSON(resultType, jsonValue)
}

func (e *Evaluator) decodeJSONCodecValue(schema *ir.CodecSchema, value Value, path string) (Value, *jsonConversionError) {
	variant, ok := value.Data.(*enumValue)
	if !ok || variant.Definition.Node.Name != "JsonValue" {
		return Value{}, &jsonConversionError{path: path, message: "expected JSON value"}
	}
	if schema.Type.Nullable {
		if variant.Name == "Null" {
			return Value{Type: schema.Type}, nil
		}
		nonnull := *schema
		nonnull.Type.Nullable = false
		decoded, err := e.decodeJSONCodecValue(&nonnull, value, path)
		if err == nil {
			decoded.Type = schema.Type
		}
		return decoded, err
	}
	payload := variant.Payload["value"]
	mismatch := func(expected string) (Value, *jsonConversionError) {
		return Value{}, &jsonConversionError{path: path, message: "expected " + expected}
	}
	switch schema.Kind {
	case "boolean":
		if variant.Name != "Boolean" {
			return mismatch("Boolean")
		}
		return Value{Type: schema.Type, Data: payload.Data}, nil
	case "integer":
		if variant.Name != "Integer" {
			return mismatch("Integer")
		}
		return Value{Type: schema.Type, Data: payload.Data}, nil
	case "float":
		if variant.Name == "Integer" {
			return Value{Type: schema.Type, Data: float64(payload.Data.(int64))}, nil
		}
		if variant.Name != "Float" {
			return mismatch("Float")
		}
		return Value{Type: schema.Type, Data: payload.Data}, nil
	case "string":
		if variant.Name != "String" {
			return mismatch("String")
		}
		return Value{Type: schema.Type, Data: payload.Data}, nil
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		if variant.Name != "String" {
			return mismatch("String")
		}
		decoded, err := e.decodeTimeJSONCodec(schema.Kind, payload.Data.(string))
		if err != nil {
			return Value{}, &jsonConversionError{path: path, message: "invalid " + schema.Type.Name}
		}
		decoded.Type = schema.Type
		return decoded, nil
	case "raw_enum":
		expected := "String"
		if schema.RawType.Kind == types.Int {
			expected = "Integer"
		}
		if variant.Name != expected {
			return mismatch(expected)
		}
		definition, ok := e.definitions[symbolKey(schema.Module, schema.Type.Name)].(*enumDefinition)
		if !ok {
			return Value{}, &jsonConversionError{path: path, message: "enum " + schema.Type.Name + " is not loaded"}
		}
		for _, item := range schema.RawValues {
			member := definition.Members[item.Member]
			if member == nil || member.RawValue == nil {
				continue
			}
			raw, err := e.expression(member.RawValue, schema.Module, e.global)
			if err != nil {
				return Value{}, &jsonConversionError{path: path, message: err.Error()}
			}
			if equal(payload, raw) {
				return Value{Type: schema.Type, Data: &enumValue{Definition: definition, Name: item.Member, Payload: map[string]Value{}}}, nil
			}
		}
		return Value{}, &jsonConversionError{path: path, message: "unknown raw value for " + schema.Type.Name}
	case "array":
		if variant.Name != "Array" {
			return mismatch("Array")
		}
		array := payload.Data.(*arrayValue)
		items := make([]Value, len(array.Items))
		for index, item := range array.Items {
			decoded, err := e.decodeJSONCodecValue(schema.Element, item, path+"/"+strconv.Itoa(index))
			if err != nil {
				return Value{}, err
			}
			items[index] = decoded
		}
		return Value{Type: schema.Type, Data: &arrayValue{Items: items}}, nil
	case "hash":
		if variant.Name != "Object" {
			return mismatch("Object")
		}
		hash := payload.Data.(*hashValue)
		entries := make([]hashEntry, len(hash.Entries))
		for index, entry := range hash.Entries {
			key := entry.Key.Data.(string)
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			decoded, err := e.decodeJSONCodecValue(schema.Element, entry.Value, path+"/"+escaped)
			if err != nil {
				return Value{}, err
			}
			entries[index] = hashEntry{Key: Value{Type: types.FromName("String"), Data: key}, Value: decoded}
		}
		return Value{Type: schema.Type, Data: &hashValue{Entries: entries}}, nil
	case "record":
		if variant.Name != "Object" {
			return mismatch(schema.Type.Name)
		}
		definition, ok := e.definitions[symbolKey(schema.Module, schema.Type.Name)].(*recordDefinition)
		if !ok {
			return Value{}, &jsonConversionError{path: path, message: "record " + schema.Type.Name + " is not loaded"}
		}
		byName := map[string]Value{}
		for _, entry := range payload.Data.(*hashValue).Entries {
			byName[entry.Key.Data.(string)] = entry.Value
		}
		fields := map[string]Value{}
		for _, field := range schema.Fields {
			fieldPath := path + "/" + jsonPointerEscapeREPL(field.WireName)
			raw, exists := byName[field.WireName]
			if !exists {
				if field.Schema.Type.Nullable {
					fields[field.Name] = Value{Type: field.Schema.Type}
					continue
				}
				return Value{}, &jsonConversionError{path: fieldPath, message: "missing field " + field.WireName}
			}
			decoded, err := e.decodeJSONCodecValue(field.Schema, raw, fieldPath)
			if err != nil {
				return Value{}, err
			}
			fields[field.Name] = decoded
		}
		return Value{Type: schema.Type, Data: &recordInstance{Definition: definition, Fields: fields}}, nil
	}
	return Value{}, &jsonConversionError{path: path, message: "unsupported JSON codec type"}
}

func jsonCodecRaw(schema *ir.CodecSchema, value Value, path string) (any, *jsonConversionError) {
	if schema.Type.Nullable && value.Data == nil {
		return nil, nil
	}
	switch schema.Kind {
	case "boolean", "integer", "float", "string":
		return value.Data, nil
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		object, ok := value.Data.(*objectInstance)
		if !ok {
			return nil, &jsonConversionError{path: path, message: "expected " + schema.Type.Name}
		}
		switch schema.Kind {
		case "time_date":
			year, month, day := timeDateFields(object)
			return fmt.Sprintf("%04d-%02d-%02d", year, month, day), nil
		case "time_of_day":
			hour, minute, second, nanosecond := timeClockFields(object)
			return formatLocalTime(hour, minute, second, nanosecond), nil
		case "time_datetime":
			year, month, day := timeDateFields(object)
			hour, minute, second, nanosecond := timeClockFields(object)
			return fmt.Sprintf("%04d-%02d-%02dT%s", year, month, day, formatLocalTime(hour, minute, second, nanosecond)), nil
		case "time_instant":
			seconds, nanosecond := timeInstantFields(object)
			return time.Unix(seconds, nanosecond).UTC().Format(time.RFC3339Nano), nil
		case "time_duration":
			seconds, nanosecond := timeDurationFields(object)
			return formatDuration(seconds, nanosecond), nil
		case "time_zone":
			return timeStringField(object, "@_identifier"), nil
		}
	case "raw_enum":
		variant, ok := value.Data.(*enumValue)
		if !ok {
			return nil, &jsonConversionError{path: path, message: "expected " + schema.Type.Name}
		}
		member := variant.Definition.Members[variant.Name]
		if member == nil || member.RawValue == nil {
			return nil, &jsonConversionError{path: path, message: "enum has no raw value"}
		}
		switch raw := member.RawValue.(type) {
		case *ir.Literal:
			if raw.Kind == "string" {
				decoded, err := strconv.Unquote(raw.Raw)
				if err != nil {
					return nil, &jsonConversionError{path: path, message: err.Error()}
				}
				return decoded, nil
			}
			parsed, err := strconv.ParseInt(strings.ReplaceAll(raw.Raw, "_", ""), 10, 64)
			if err == nil {
				return parsed, nil
			}
		case *ir.Unary:
			if literal, ok := raw.Operand.(*ir.Literal); raw.Operator == "-" && ok {
				parsed, err := strconv.ParseInt(strings.ReplaceAll(literal.Raw, "_", ""), 10, 64)
				if err == nil {
					return -parsed, nil
				}
			}
		}
		return nil, &jsonConversionError{path: path, message: "invalid enum raw value"}
	case "array":
		array, ok := value.Data.(*arrayValue)
		if !ok {
			return nil, &jsonConversionError{path: path, message: "expected Array"}
		}
		result := make([]any, len(array.Items))
		for index, item := range array.Items {
			converted, err := jsonCodecRaw(schema.Element, item, path+"/"+strconv.Itoa(index))
			if err != nil {
				return nil, err
			}
			result[index] = converted
		}
		return result, nil
	case "hash":
		hash, ok := value.Data.(*hashValue)
		if !ok {
			return nil, &jsonConversionError{path: path, message: "expected Hash"}
		}
		result := map[string]any{}
		for _, entry := range hash.Entries {
			key := entry.Key.Data.(string)
			converted, err := jsonCodecRaw(schema.Element, entry.Value, path+"/"+jsonPointerEscapeREPL(key))
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	case "record":
		record, ok := value.Data.(*recordInstance)
		if !ok {
			return nil, &jsonConversionError{path: path, message: "expected " + schema.Type.Name}
		}
		result := map[string]any{}
		for _, field := range schema.Fields {
			converted, err := jsonCodecRaw(field.Schema, record.Fields[field.Name], path+"/"+jsonPointerEscapeREPL(field.WireName))
			if err != nil {
				return nil, err
			}
			result[field.WireName] = converted
		}
		return result, nil
	}
	return nil, &jsonConversionError{path: path, message: "unsupported JSON codec type"}
}

func (e *Evaluator) jsonCodecErr(resultType types.Type, errorValue Value) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("JSON requires trb/std/result")
	}
	return Value{Type: resultType, Data: &enumValue{Definition: definition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
}

func jsonPointerEscapeREPL(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "~", "~0"), "/", "~1")
}

type jsonConversionError struct {
	path    string
	message string
}

func (e *Evaluator) jsonValue(raw any, path string) (Value, *jsonConversionError) {
	definition, ok := e.definitions[symbolKey("trb/std/json/index", "JsonValue")].(*enumDefinition)
	if !ok {
		return Value{}, &jsonConversionError{path: path, message: "JSON runtime is not loaded"}
	}
	typ := types.FromName("JsonValue")
	construct := func(name string, payload map[string]Value) Value {
		return Value{Type: typ, Data: &enumValue{Definition: definition, Name: name, Payload: payload}}
	}
	switch value := raw.(type) {
	case nil:
		return construct("Null", map[string]Value{}), nil
	case bool:
		return construct("Boolean", map[string]Value{"value": {Type: types.FromName("Boolean"), Data: value}}), nil
	case stdjson.Number:
		number, err := strconv.ParseFloat(string(value), 64)
		if err != nil || math.IsInf(number, 0) || math.IsNaN(number) {
			return Value{}, &jsonConversionError{path: path, message: "JSON number is not finite"}
		}
		if math.Trunc(number) == number {
			if number < -9007199254740991 || number > 9007199254740991 {
				return Value{}, &jsonConversionError{path: path, message: "JSON integer is outside the portable range"}
			}
			return construct("Integer", map[string]Value{"value": {Type: types.FromName("Integer"), Data: int64(number)}}), nil
		}
		return construct("Float", map[string]Value{"value": {Type: types.FromName("Float"), Data: number}}), nil
	case int64:
		if value < -9007199254740991 || value > 9007199254740991 {
			return Value{}, &jsonConversionError{path: path, message: "JSON integer is outside the portable range"}
		}
		return construct("Integer", map[string]Value{"value": {Type: types.FromName("Integer"), Data: value}}), nil
	case float64:
		if math.IsInf(value, 0) || math.IsNaN(value) {
			return Value{}, &jsonConversionError{path: path, message: "JSON number is not finite"}
		}
		return construct("Float", map[string]Value{"value": {Type: types.FromName("Float"), Data: value}}), nil
	case string:
		return construct("String", map[string]Value{"value": {Type: types.FromName("String"), Data: value}}), nil
	case []any:
		items := make([]Value, len(value))
		for index, item := range value {
			converted, err := e.jsonValue(item, path+"/"+strconv.Itoa(index))
			if err != nil {
				return Value{}, err
			}
			items[index] = converted
		}
		arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{typ}}
		return construct("Array", map[string]Value{"value": {Type: arrayType, Data: &arrayValue{Items: items}}}), nil
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		entries := make([]hashEntry, 0, len(keys))
		for _, key := range keys {
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			converted, err := e.jsonValue(value[key], path+"/"+escaped)
			if err != nil {
				return Value{}, err
			}
			entries = append(entries, hashEntry{
				Key:   Value{Type: types.FromName("String"), Data: key},
				Value: converted,
			})
		}
		hashType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{types.FromName("String"), typ}}
		return construct("Object", map[string]Value{"value": {Type: hashType, Data: &hashValue{Entries: entries}}}), nil
	default:
		return Value{}, &jsonConversionError{path: path, message: "unsupported JSON value"}
	}
}

func jsonRaw(value Value, path string) (any, *jsonConversionError) {
	item, ok := value.Data.(*enumValue)
	if !ok || item.Definition.Node.Name != "JsonValue" {
		return nil, &jsonConversionError{path: path, message: "unsupported JSON value"}
	}
	payload := item.Payload["value"]
	switch item.Name {
	case "Null":
		return nil, nil
	case "Boolean":
		return payload.Data.(bool), nil
	case "Integer":
		integer := payload.Data.(int64)
		if integer < -9007199254740991 || integer > 9007199254740991 {
			return nil, &jsonConversionError{path: path, message: "JSON integer is outside the portable range"}
		}
		return integer, nil
	case "Float":
		floating := payload.Data.(float64)
		if math.IsInf(floating, 0) || math.IsNaN(floating) {
			return nil, &jsonConversionError{path: path, message: "JSON Float must be finite"}
		}
		return floating, nil
	case "String":
		return payload.Data.(string), nil
	case "Array":
		array := payload.Data.(*arrayValue)
		result := make([]any, len(array.Items))
		for index, child := range array.Items {
			converted, err := jsonRaw(child, path+"/"+strconv.Itoa(index))
			if err != nil {
				return nil, err
			}
			result[index] = converted
		}
		return result, nil
	case "Object":
		hash := payload.Data.(*hashValue)
		result := make(map[string]any, len(hash.Entries))
		for _, entry := range hash.Entries {
			key := entry.Key.Data.(string)
			escaped := strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
			converted, err := jsonRaw(entry.Value, path+"/"+escaped)
			if err != nil {
				return nil, err
			}
			result[key] = converted
		}
		return result, nil
	default:
		return nil, &jsonConversionError{path: path, message: "unsupported JSON value"}
	}
}

func (e *Evaluator) jsonOK(resultType types.Type, value Value) (Value, error) {
	return e.resultOK(resultType, value)
}

func (e *Evaluator) jsonError(resultType types.Type, kind, message, path string, line, column *int64) (Value, error) {
	resultDefinition, ok := e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("JSON requires trb/std/result")
	}
	errorDefinition, ok := e.definitions[symbolKey("trb/std/json/index", "JsonError")].(*recordDefinition)
	if !ok {
		return Value{}, errors.New("JSON runtime is not loaded")
	}
	kindDefinition, ok := e.definitions[symbolKey("trb/std/json/index", "JsonErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("JSON runtime is not loaded")
	}
	nullableInteger := types.FromName("Integer")
	nullableInteger.Nullable = true
	lineValue := Value{Type: nullableInteger}
	if line != nil {
		lineValue.Data = *line
	}
	columnValue := Value{Type: nullableInteger}
	if column != nil {
		columnValue.Data = *column
	}
	kindValue := Value{Type: types.FromName("JsonErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	errorType := types.FromName("JsonError")
	if len(resultType.Args) == 2 {
		errorType = resultType.Args[1]
	}
	errorValue := Value{
		Type: errorType,
		Data: &recordInstance{
			Definition: errorDefinition,
			Fields: map[string]Value{
				"kind":    kindValue,
				"message": {Type: types.FromName("String"), Data: message},
				"path":    {Type: types.FromName("String"), Data: path},
				"line":    lineValue,
				"column":  columnValue,
			},
		},
	}
	return Value{
		Type: resultType,
		Data: &enumValue{
			Definition: resultDefinition,
			Name:       "Err",
			Payload:    map[string]Value{"error": errorValue},
		},
	}, nil
}

func jsonSourceLocation(source string, parseErr error) (*int64, *int64) {
	syntax, ok := parseErr.(*stdjson.SyntaxError)
	if !ok {
		return nil, nil
	}
	offset := int(syntax.Offset) - 1
	if offset < 0 {
		offset = 0
	}
	if offset > len(source) {
		offset = len(source)
	}
	line, column := int64(1), int64(1)
	for _, value := range source[:offset] {
		if value == '\n' {
			line++
			column = 1
		} else {
			column++
		}
	}
	return &line, &column
}

func stripJSONC(source string) string {
	result := []byte(source)
	inString := false
	escaped := false
	for index := 0; index < len(result); index++ {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if result[index] == '\\' {
				escaped = true
			} else if result[index] == '"' {
				inString = false
			}
			continue
		}
		if result[index] == '"' {
			inString = true
			continue
		}
		if result[index] != '/' || index+1 >= len(result) {
			continue
		}
		if result[index+1] == '/' {
			result[index], result[index+1] = ' ', ' '
			index += 2
			for index < len(result) && result[index] != '\n' {
				if result[index] != '\r' {
					result[index] = ' '
				}
				index++
			}
			index--
		} else if result[index+1] == '*' {
			result[index], result[index+1] = ' ', ' '
			index += 2
			for index < len(result) {
				if index+1 < len(result) && result[index] == '*' && result[index+1] == '/' {
					result[index], result[index+1] = ' ', ' '
					index++
					break
				}
				if result[index] != '\n' && result[index] != '\r' {
					result[index] = ' '
				}
				index++
			}
		}
	}
	return string(result)
}

func literal(node *ir.Literal) (Value, error) {
	value := Value{Type: node.ExprType()}
	switch node.Kind {
	case "string":
		decoded, err := strconv.Unquote(node.Raw)
		if err != nil {
			return Value{}, err
		}
		value.Data = decoded
	case "integer":
		parsed, err := strconv.ParseInt(strings.ReplaceAll(node.Raw, "_", ""), 0, 64)
		if err != nil {
			return Value{}, err
		}
		value.Data = parsed
	case "float":
		parsed, err := strconv.ParseFloat(strings.ReplaceAll(node.Raw, "_", ""), 64)
		if err != nil {
			return Value{}, err
		}
		value.Data = parsed
	case "boolean":
		value.Data = node.Raw == "true"
	case "nil":
		value.Data = nil
	default:
		return Value{}, fmt.Errorf("unsupported literal %s", node.Kind)
	}
	return value, nil
}

func indexValue(receiver, index Value, typ types.Type) (Value, error) {
	switch value := receiver.Data.(type) {
	case *arrayValue:
		position, ok := index.Data.(int64)
		if !ok || position < 0 || position >= int64(len(value.Items)) {
			return Value{}, errors.New("array index is out of bounds")
		}
		result := value.Items[position]
		result.Type = typ
		return result, nil
	case *hashValue:
		for _, entry := range value.Entries {
			if equal(entry.Key, index) {
				result := entry.Value
				result.Type = typ
				return result, nil
			}
		}
		return Value{}, errors.New("Hash key is missing")
	case string:
		position, ok := index.Data.(int64)
		runes := []rune(value)
		if !ok || position < 0 || position >= int64(len(runes)) {
			return Value{}, errors.New("string index is out of bounds")
		}
		return Value{Type: typ, Data: string(runes[position])}, nil
	}
	return Value{}, fmt.Errorf("%s is not indexable", receiver.Type)
}

func assignIndex(receiver, index, value Value) error {
	switch target := receiver.Data.(type) {
	case *arrayValue:
		position, ok := index.Data.(int64)
		if !ok || position < 0 || position >= int64(len(target.Items)) {
			return errors.New("array index is out of bounds")
		}
		target.Items[position] = value
		return nil
	case *hashValue:
		for entryIndex, entry := range target.Entries {
			if equal(entry.Key, index) {
				target.Entries[entryIndex].Value = value
				return nil
			}
		}
		target.Entries = append(target.Entries, hashEntry{Key: index, Value: value})
		return nil
	}
	return fmt.Errorf("%s is not index-assignable", receiver.Type)
}

func number(value Value) (float64, bool, bool) {
	switch item := value.Data.(type) {
	case int64:
		return float64(item), false, true
	case float64:
		return item, true, true
	default:
		return 0, false, false
	}
}

func numericValue(number float64, floating bool, typ types.Type) Value {
	if floating {
		return Value{Type: typ, Data: number}
	}
	return Value{Type: typ, Data: int64(number)}
}

func truthy(value Value) bool {
	if value.Data == nil {
		return false
	}
	if boolean, ok := value.Data.(bool); ok {
		return boolean
	}
	return true
}

func equal(left, right Value) bool {
	return reflect.DeepEqual(left.Data, right.Data)
}

func comparePortableValues(left, right Value) (int, error) {
	if leftString, ok := left.Data.(string); ok {
		rightString, rightOK := right.Data.(string)
		if !rightOK {
			return 0, fmt.Errorf("cannot compare %s and %s", left.Type, right.Type)
		}
		return strings.Compare(leftString, rightString), nil
	}
	leftNumber, _, leftOK := number(left)
	rightNumber, _, rightOK := number(right)
	if !leftOK || !rightOK {
		return 0, fmt.Errorf("portable natural order is not defined for %s", left.Type)
	}
	if math.IsNaN(leftNumber) {
		if math.IsNaN(rightNumber) {
			return 0, nil
		}
		return 1, nil
	}
	if math.IsNaN(rightNumber) {
		return -1, nil
	}
	if leftNumber < rightNumber {
		return -1, nil
	}
	if leftNumber > rightNumber {
		return 1, nil
	}
	return 0, nil
}

func expressionReference(expression ir.Expression) *ir.Reference {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Reference
	case *ir.Member:
		return node.Reference
	case *ir.TypeApply:
		return expressionReference(node.Receiver)
	default:
		return nil
	}
}

func expressionName(expression ir.Expression) string {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Name
	case *ir.Member:
		prefix := expressionName(node.Receiver)
		if prefix == "" {
			return node.Name
		}
		return prefix + "::" + node.Name
	default:
		return ""
	}
}

func symbolKey(module, name string) string { return module + "\x00" + name }

func ownedName(owner, name string) string {
	if owner == "" {
		return name
	}
	return owner + "::" + name
}

func Inspect(value Value) string {
	switch item := value.Data.(type) {
	case nil:
		return "nil"
	case string:
		return strconv.Quote(item)
	case int64:
		return strconv.FormatInt(item, 10)
	case float64:
		return strconv.FormatFloat(item, 'g', -1, 64)
	case bool:
		return strconv.FormatBool(item)
	case bytesValue:
		parts := make([]string, len(item))
		for index, value := range item {
			parts[index] = strconv.Itoa(int(value))
		}
		return "Bytes[" + strings.Join(parts, ", ") + "]"
	case *stringBuilderValue:
		return "StringBuilder(" + strconv.Quote(item.value.String()) + ")"
	case *arrayValue:
		parts := make([]string, len(item.Items))
		for index, value := range item.Items {
			parts[index] = Inspect(value)
		}
		return "[" + strings.Join(parts, ", ") + "]"
	case *rangeValue:
		operator := ".."
		if item.Exclusive {
			operator = "..."
		}
		return strconv.FormatInt(item.Start, 10) + operator + strconv.FormatInt(item.End, 10)
	case *hashValue:
		parts := make([]string, len(item.Entries))
		for index, entry := range item.Entries {
			parts[index] = Inspect(entry.Key) + ": " + Inspect(entry.Value)
		}
		return "{" + strings.Join(parts, ", ") + "}"
	case *recordInstance:
		parts := make([]string, 0, len(item.Definition.Fields))
		for _, field := range item.Definition.Fields {
			parts = append(parts, field.Name+": "+Inspect(item.Fields[field.Name]))
		}
		return item.Definition.Node.Name + "(" + strings.Join(parts, ", ") + ")"
	case *objectInstance:
		names := make([]string, 0, len(item.Fields))
		for name := range item.Fields {
			if strings.HasPrefix(name, "@__trb_") {
				continue
			}
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, strings.TrimPrefix(name, "@")+": "+Inspect(item.Fields[name]))
		}
		return "#<" + item.Definition.Node.Name + " " + strings.Join(parts, ", ") + ">"
	case *ormQueryValue:
		return "#<" + item.model.QueryType + ">"
	case *ormSubqueryValue:
		return "#<" + value.Type.String() + ">"
	case *typeValue:
		if item.Record != nil {
			return item.Record.Node.Name
		}
		if item.Class != nil {
			return item.Class.Node.Name
		}
		if item.Enum != nil {
			return item.Enum.Node.Name
		}
	case *enumValue:
		if len(item.Payload) == 0 {
			return item.Definition.Node.Name + "::" + item.Name
		}
		member := item.Definition.Members[item.Name]
		parts := make([]string, 0, len(member.Fields))
		for _, field := range member.Fields {
			parts = append(parts, field.Name+": "+Inspect(item.Payload[field.Name]))
		}
		return item.Definition.Node.Name + "::" + item.Name + "(" + strings.Join(parts, ", ") + ")"
	case map[string]string:
		return "#<" + value.Type.String() + ">"
	case *callable:
		if item.Lambda != nil {
			return "#<fn>"
		}
		return "#<callable>"
	}
	return fmt.Sprint(value.Data)
}

func plain(value Value) string {
	if stringValue, ok := value.Data.(string); ok {
		return stringValue
	}
	return Inspect(value)
}
