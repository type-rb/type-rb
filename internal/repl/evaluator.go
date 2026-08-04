package repl

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

type Value struct {
	Type types.Type
	Data any
}

type bytesValue []byte

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
	Method    *ir.Method
	Receiver  Value
	Module    string
	Construct *typeValue
	Intrinsic string
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
	stdout      io.Writer
	mode        string
	context     context.Context
	global      *scope
	definitions map[string]any
	moduleValue map[string]Value
}

func NewEvaluator(stdout io.Writer, mode string) *Evaluator {
	return &Evaluator{
		stdout:      stdout,
		mode:        mode,
		context:     context.Background(),
		global:      &scope{values: map[string]Value{}},
		definitions: map[string]any{},
		moduleValue: map[string]Value{},
	}
}

func (e *Evaluator) LoadProject(programs []*ir.Program, sessionModule string) error {
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
			definition := &enumDefinition{Module: module, Node: node, Members: map[string]*ir.EnumMember{}}
			for _, statement := range node.Body {
				if member, ok := statement.(*ir.EnumMember); ok {
					definition.Members[member.Name] = member
				}
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
	case *ir.Comment, *ir.Import, *ir.Record, *ir.Enum, *ir.EnumMember, *ir.Interface, *ir.Field, *ir.RecordField, *ir.Method:
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
		condition, err := e.expression(node.Condition, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		if truthy(condition) {
			return e.evaluate(node.Then, module, &scope{parent: sc, values: map[string]Value{}})
		}
		for _, branch := range node.ElseIf {
			condition, err := e.expression(branch.Condition, module, sc)
			if err != nil {
				return flowResult{}, err
			}
			if truthy(condition) {
				return e.evaluate(branch.Body, module, &scope{parent: sc, values: map[string]Value{}})
			}
		}
		return e.evaluate(node.Else, module, &scope{parent: sc, values: map[string]Value{}})
	case *ir.Case:
		value, err := e.expression(node.Value, module, sc)
		if err != nil {
			return flowResult{}, err
		}
		for _, branch := range node.Branches {
			if branch.PayloadEnum {
				variant, ok := value.Data.(*enumValue)
				if !ok || variant.Name != branch.Member {
					continue
				}
				branchScope := &scope{parent: sc, values: map[string]Value{}}
				for _, binding := range branch.Bindings {
					branchScope.values[binding.Name] = variant.Payload[binding.Field]
				}
				return e.evaluate(branch.Body, module, branchScope)
			}
			candidate, err := e.expression(branch.Value, module, sc)
			if err != nil {
				return flowResult{}, err
			}
			if equal(value, candidate) {
				return e.evaluate(branch.Body, module, &scope{parent: sc, values: map[string]Value{}})
			}
		}
		return e.evaluate(node.Else, module, &scope{parent: sc, values: map[string]Value{}})
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
	case *ir.Native, *ir.NativeBlock:
		return flowResult{}, fmt.Errorf("native %s syntax is not executable by the typed IR REPL", e.mode)
	default:
		return flowResult{}, fmt.Errorf("unsupported REPL statement %T", statement)
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
		if reference != nil && reference.Intrinsic != "" && reference.ReceiverMethod {
			if member, ok := node.Callee.(*ir.Member); ok {
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
			return e.intrinsic(reference.Intrinsic, arguments, node.ExprType())
		}
		var callee Value
		var err error
		if member, ok := node.Callee.(*ir.Member); ok {
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
			return e.intrinsic(function.Intrinsic, arguments, node.ExprType())
		}
		return e.call(function, arguments)
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
	case *ir.Block:
		return Value{Type: node.ExprType(), Data: node}, nil
	case *ir.NativeExpression:
		return Value{}, fmt.Errorf("native %s expression is not executable by the typed IR REPL", e.mode)
	default:
		return Value{}, fmt.Errorf("unsupported REPL expression %T", expression)
	}
}

func (e *Evaluator) iterate(node *ir.Iterate, module string, sc *scope) (flowResult, error) {
	source, err := e.expression(node.Source, module, sc)
	if err != nil {
		return flowResult{}, err
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
			iterationScope.values[node.Item] = Value{Type: node.ItemType, Data: &arrayValue{Items: slice}}
		} else {
			iterationScope.values[node.Item] = items[offset]
		}
		if node.WithIndex {
			iterationScope.values[node.Index] = Value{Type: types.FromName("Integer"), Data: int64(iterationIndex)}
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

func (e *Evaluator) intrinsic(name string, arguments []evaluatedArgument, typ types.Type) (Value, error) {
	values := func() []Value {
		result := make([]Value, len(arguments))
		for index, argument := range arguments {
			result[index] = argument.Value
		}
		return result
	}()
	require := func(count int) error {
		if len(values) < count {
			return fmt.Errorf("intrinsic %s requires %d arguments", name, count)
		}
		return nil
	}
	switch name {
	case "trb.std.io.puts":
		if err := require(1); err != nil {
			return Value{}, err
		}
		fmt.Fprintln(e.stdout, plain(values[0]))
		return Value{Type: typ}, nil
	case "trb.std.strings.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("strings.length expects String")
		}
		return Value{Type: typ, Data: int64(utf8.RuneCountInString(value))}, nil
	case "trb.std.strings.uppercase", "trb.std.strings.lowercase":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("string intrinsic expects String")
		}
		if name == "trb.std.strings.uppercase" {
			value = strings.ToUpper(value)
		} else {
			value = strings.ToLower(value)
		}
		return Value{Type: typ, Data: value}, nil
	case "trb.std.strings.contains":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(string)
		right, rightOK := values[1].Data.(string)
		if !leftOK || !rightOK {
			return Value{}, errors.New("strings.contains expects String arguments")
		}
		return Value{Type: typ, Data: strings.Contains(left, right)}, nil
	case "trb.std.bytes.from_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("bytes.from_string expects String")
		}
		return Value{Type: typ, Data: bytesValue([]byte(value))}, nil
	case "trb.std.bytes.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.to_string expects Bytes")
		}
		return Value{Type: typ, Data: strings.ToValidUTF8(string(value), "�")}, nil
	case "trb.std.bytes.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.length expects Bytes")
		}
		return Value{Type: typ, Data: int64(len(value))}, nil
	case "trb.std.bytes.at":
		if err := require(2); err != nil {
			return Value{}, err
		}
		value, valueOK := values[0].Data.(bytesValue)
		index, indexOK := values[1].Data.(int64)
		if !valueOK || !indexOK {
			return Value{}, errors.New("bytes.at expects Bytes and Integer")
		}
		if index < 0 || index >= int64(len(value)) {
			return Value{}, errors.New("Bytes index is out of bounds")
		}
		return Value{Type: typ, Data: int64(value[index])}, nil
	case "trb.std.bytes.concat":
		if err := require(2); err != nil {
			return Value{}, err
		}
		left, leftOK := values[0].Data.(bytesValue)
		right, rightOK := values[1].Data.(bytesValue)
		if !leftOK || !rightOK {
			return Value{}, errors.New("bytes.concat expects Bytes arguments")
		}
		result := append(bytesValue(nil), left...)
		result = append(result, right...)
		return Value{Type: typ, Data: result}, nil
	case "trb.std.bytes.valid_utf8":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(bytesValue)
		if !ok {
			return Value{}, errors.New("bytes.valid_utf8 expects Bytes")
		}
		return Value{Type: typ, Data: utf8.Valid(value)}, nil
	case "trb.std.arrays.length":
		if err := require(1); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.length expects Array")
		}
		return Value{Type: typ, Data: int64(len(array.Items))}, nil
	case "trb.std.arrays.push":
		if err := require(2); err != nil {
			return Value{}, err
		}
		array, ok := values[0].Data.(*arrayValue)
		if !ok {
			return Value{}, errors.New("arrays.push expects Array")
		}
		array.Items = append(array.Items, values[1])
		return Value{Type: typ}, nil
	case "trb.std.numbers.to_string":
		if err := require(1); err != nil {
			return Value{}, err
		}
		return Value{Type: typ, Data: plain(values[0])}, nil
	case "trb.std.numbers.parse_integer":
		if err := require(1); err != nil {
			return Value{}, err
		}
		value, ok := values[0].Data.(string)
		if !ok {
			return Value{}, errors.New("numbers.parse_integer expects String")
		}
		parsed, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return Value{}, err
		}
		return Value{Type: typ, Data: parsed}, nil
	case "trb.platform.typescript.node.argv":
		return Value{Type: typ, Data: &arrayValue{}}, nil
	case "trb.platform.go.context.background", "trb.platform.go.context.todo":
		return Value{Type: typ, Data: map[string]string{"context": strings.TrimPrefix(name, "trb.platform.go.context.")}}, nil
	default:
		return Value{}, fmt.Errorf("intrinsic %s is type-checked for mode %s but has no REPL runtime adapter", name, e.mode)
	}
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

func expressionReference(expression ir.Expression) *ir.Reference {
	switch node := expression.(type) {
	case *ir.Identifier:
		return node.Reference
	case *ir.Member:
		return node.Reference
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
			names = append(names, name)
		}
		sort.Strings(names)
		parts := make([]string, 0, len(names))
		for _, name := range names {
			parts = append(parts, strings.TrimPrefix(name, "@")+": "+Inspect(item.Fields[name]))
		}
		return "#<" + item.Definition.Node.Name + " " + strings.Join(parts, ", ") + ">"
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
	}
	return fmt.Sprint(value.Data)
}

func plain(value Value) string {
	if stringValue, ok := value.Data.(string); ok {
		return stringValue
	}
	return Inspect(value)
}
