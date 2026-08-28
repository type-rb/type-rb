package bootstrapsnapshot

import (
	"fmt"
	"sort"

	"github.com/type-rb/type-rb/internal/compiler"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/token"
)

type v4Builder struct {
	methodIDs    map[string]string
	registry     *aggregateRegistry
	functions    []FunctionV4
	lambdaCounts map[string]int
}

// BuildV4 encodes managed values and first-class functions in bootstrap snapshot version 4.
func BuildV4(artifacts []*compiler.Artifact, sourceRoot string) (SnapshotV4, error) {
	inputs := projectMethods(artifacts)
	if len(inputs) == 0 {
		return SnapshotV4{}, fmt.Errorf("bootstrap snapshot v4 found no project functions")
	}
	methodIDs := make(map[string]string, len(inputs))
	for _, input := range inputs {
		id := functionID(input.program, input.method)
		if key := input.method.Declaration.Key(); key != "" {
			methodIDs[key] = id
		}
		methodIDs["name:"+input.program.ModulePath+"#"+input.method.Name] = id
	}
	entry := ""
	module := ""
	for _, input := range inputs {
		if input.method.Name != compiler.MainFunction {
			continue
		}
		if entry != "" {
			return SnapshotV4{}, fmt.Errorf("bootstrap snapshot v4 requires exactly one top-level main function")
		}
		entry = functionID(input.program, input.method)
		module = input.program.ModulePath
	}
	if entry == "" {
		return SnapshotV4{}, fmt.Errorf("bootstrap snapshot v4 requires one top-level def main()")
	}

	sources, sourceIDs := projectSources(inputs, sourceRoot)
	builder := &v4Builder{
		methodIDs: methodIDs, registry: newAggregateRegistry(artifacts, Version4),
		lambdaCounts: map[string]int{},
	}
	for _, input := range inputs {
		id := functionID(input.program, input.method)
		lowerer := &v3FunctionLowerer{
			version: Version4, program: input.program, method: input.method,
			sourceID: sourceIDs[input.program.SourcePath], methodIDs: methodIDs,
			registry: builder.registry, builder: builder, ownerID: id,
			env: map[string]v3ValueRef{}, captures: map[string]bool{},
		}
		function, captures, err := lowerer.lowerFunction(id, nil)
		if err != nil {
			return SnapshotV4{}, err
		}
		builder.functions = append(builder.functions, v4Function(function, captures))
	}
	sort.Slice(builder.functions, func(i, j int) bool { return builder.functions[i].ID < builder.functions[j].ID })
	definitions := make([]TypeDefinition, 0, len(builder.registry.definitions))
	for _, definition := range builder.registry.definitions {
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ID < definitions[j].ID })
	return SnapshotV4{
		Format: Format, Version: Version4, Module: module, EntryFunction: entry,
		Sources: sources, Types: definitions, Functions: builder.functions,
	}, nil
}

func v4Function(function Function, captures []Parameter) FunctionV4 {
	if captures == nil {
		captures = []Parameter{}
	}
	return FunctionV4{
		ID: function.ID, Name: function.Name, Captures: captures,
		Parameters: function.Parameters, Result: function.Result, Entry: function.Entry,
		Origin: function.Origin, Blocks: function.Blocks,
	}
}

func (b *v4Builder) nextLambda(ownerID string) (string, string) {
	index := b.lambdaCounts[ownerID]
	b.lambdaCounts[ownerID] = index + 1
	name := fmt.Sprintf("lambda%d", index)
	return ownerID + "$" + name, name
}

func (l *v3FunctionLowerer) lowerArray(node *ir.Array) (v3ValueRef, error) {
	typeID, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "array" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "Array literal type "+node.ExprType().String())
	}
	arguments := make([]string, 0, len(node.Elements))
	for _, element := range node.Elements {
		value, elementErr := l.lowerExpression(element)
		if elementErr != nil {
			return v3ValueRef{}, elementErr
		}
		arguments = append(arguments, value.id)
	}
	id := l.newValue()
	l.emit(ArrayConstruct{
		Op: "array_construct", Result: id, Type: typeID, Arguments: arguments,
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerIndex(node *ir.Index) (v3ValueRef, error) {
	receiver, err := l.lowerExpression(node.Receiver)
	if err != nil {
		return v3ValueRef{}, err
	}
	index, err := l.lowerExpression(node.Index)
	if err != nil {
		return v3ValueRef{}, err
	}
	typeID, err := l.typeName(receiver.typ, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	id := l.newValue()
	if typeID == "String" {
		l.emit(StringIndex{
			Op: "string_index", Result: id, Value: receiver.id, Index: index.id,
			Origin: l.origin(node.SourceSpan()),
		})
		return v3ValueRef{id: id, typ: node.ExprType()}, nil
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "array" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "index receiver type "+receiver.typ.String())
	}
	l.emit(ArrayGet{
		Op: "array_get", Result: id, Type: typeID, Array: receiver.id, Index: index.id,
		Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerArraySet(target *ir.Index, expression ir.Expression, span token.Span) error {
	receiver, err := l.lowerExpression(target.Receiver)
	if err != nil {
		return err
	}
	index, err := l.lowerExpression(target.Index)
	if err != nil {
		return err
	}
	value, err := l.lowerExpression(expression)
	if err != nil {
		return err
	}
	typeID, err := l.typeName(receiver.typ, span)
	if err != nil {
		return err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "array" {
		return l.unsupported(span, "indexed assignment receiver type "+receiver.typ.String())
	}
	l.emit(ArraySet{
		Op: "array_set", Type: typeID, Array: receiver.id, Index: index.id, Value: value.id,
		Origin: l.origin(span),
	})
	return nil
}

func (l *v3FunctionLowerer) lowerFunctionValue(node *ir.Identifier) (v3ValueRef, error) {
	function := l.callFunctionID(node)
	if function == "" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "function value "+node.Name)
	}
	typeID, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	id := l.newValue()
	l.emit(ClosureConstruct{
		Op: "closure_construct", Result: id, Type: typeID, Function: function,
		Captures: []string{}, Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerLambda(node *ir.Lambda) (v3ValueRef, error) {
	if l.builder == nil {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "lambda without a version 4 builder")
	}
	names := v4LambdaCaptureNames(node, l.env)
	inputs := make([]v4CaptureInput, 0, len(names))
	values := make([]string, 0, len(names))
	for _, name := range names {
		value := l.env[name]
		inputs = append(inputs, v4CaptureInput{name: name, typ: value.typ, span: node.SourceSpan()})
		values = append(values, value.id)
	}
	functionID, functionName := l.builder.nextLambda(l.ownerID)
	method := &ir.Method{
		Base: node.ExprBase.Base, Name: functionName, Parameters: node.Parameters,
		ReturnType: node.ReturnType, Body: node.Body,
	}
	nested := &v3FunctionLowerer{
		version: Version4, program: l.program, method: method, sourceID: l.sourceID,
		methodIDs: l.methodIDs, registry: l.registry, builder: l.builder, ownerID: functionID,
		env: map[string]v3ValueRef{}, captures: map[string]bool{},
	}
	function, captures, err := nested.lowerFunction(functionID, inputs)
	if err != nil {
		return v3ValueRef{}, err
	}
	l.builder.functions = append(l.builder.functions, v4Function(function, captures))
	typeID, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	id := l.newValue()
	l.emit(ClosureConstruct{
		Op: "closure_construct", Result: id, Type: typeID, Function: functionID,
		Captures: values, Origin: l.origin(node.SourceSpan()),
	})
	return v3ValueRef{id: id, typ: node.ExprType()}, nil
}

func (l *v3FunctionLowerer) lowerClosureCall(node *ir.Call) (v3ValueRef, error) {
	closure, err := l.lowerExpression(node.Callee)
	if err != nil {
		return v3ValueRef{}, err
	}
	typeID, err := l.typeName(closure.typ, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "function" {
		return v3ValueRef{}, l.unsupported(node.SourceSpan(), "indirect call through "+closure.typ.String())
	}
	arguments := make([]string, 0, len(node.Arguments))
	for _, argument := range node.Arguments {
		value, argumentErr := l.lowerExpression(argument.Value)
		if argumentErr != nil {
			return v3ValueRef{}, argumentErr
		}
		arguments = append(arguments, value.id)
	}
	resultType, err := l.typeName(node.ExprType(), node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, err
	}
	var result *string
	ref := v3ValueRef{typ: node.ExprType()}
	if resultType != "Void" {
		ref.id = l.newValue()
		result = &ref.id
	}
	l.emit(ClosureCall{
		Op: "closure_call", Result: result, Type: typeID, Closure: closure.id,
		Arguments: arguments, Origin: l.origin(node.SourceSpan()),
	})
	return ref, nil
}

func (l *v3FunctionLowerer) lowerV4IntrinsicCall(node *ir.Call) (v3ValueRef, bool, error) {
	member, ok := node.Callee.(*ir.Member)
	if !ok || member.Reference == nil || member.Receiver == nil {
		return v3ValueRef{}, false, nil
	}
	intrinsic := member.Reference.Intrinsic
	if intrinsic != "trb.std.strings.length" && intrinsic != "trb.std.arrays.length" && intrinsic != "trb.std.arrays.push" {
		return v3ValueRef{}, false, nil
	}
	receiver, err := l.lowerExpression(member.Receiver)
	if err != nil {
		return v3ValueRef{}, true, err
	}
	typeID, err := l.typeName(receiver.typ, node.SourceSpan())
	if err != nil {
		return v3ValueRef{}, true, err
	}
	at := l.origin(node.SourceSpan())
	switch intrinsic {
	case "trb.std.strings.length":
		if len(node.Arguments) != 0 || typeID != "String" {
			return v3ValueRef{}, true, l.unsupported(node.SourceSpan(), "String size() call")
		}
		id := l.newValue()
		l.emit(StringSize{Op: "string_size", Result: id, Value: receiver.id, Origin: at})
		return v3ValueRef{id: id, typ: node.ExprType()}, true, nil
	case "trb.std.arrays.length":
		if len(node.Arguments) != 0 {
			return v3ValueRef{}, true, l.unsupported(node.SourceSpan(), "Array size() arity")
		}
		definition, defined := l.registry.definition(typeID)
		if !defined || definition.Kind != "array" {
			return v3ValueRef{}, true, l.unsupported(node.SourceSpan(), "Array size() receiver type")
		}
		id := l.newValue()
		l.emit(ArraySize{Op: "array_size", Result: id, Type: typeID, Array: receiver.id, Origin: at})
		return v3ValueRef{id: id, typ: node.ExprType()}, true, nil
	case "trb.std.arrays.push":
		if len(node.Arguments) != 1 {
			return v3ValueRef{}, true, l.unsupported(node.SourceSpan(), "Array push() arity")
		}
		definition, defined := l.registry.definition(typeID)
		if !defined || definition.Kind != "array" {
			return v3ValueRef{}, true, l.unsupported(node.SourceSpan(), "Array push() receiver type")
		}
		value, valueErr := l.lowerExpression(node.Arguments[0].Value)
		if valueErr != nil {
			return v3ValueRef{}, true, valueErr
		}
		l.emit(ArrayPush{Op: "array_push", Type: typeID, Array: receiver.id, Value: value.id, Origin: at})
		return v3ValueRef{typ: node.ExprType()}, true, nil
	}
	return v3ValueRef{}, false, nil
}

type v4CaptureCollector struct {
	outer map[string]v3ValueRef
	seen  map[string]bool
	names []string
}

func v4LambdaCaptureNames(lambda *ir.Lambda, outer map[string]v3ValueRef) []string {
	collector := &v4CaptureCollector{outer: outer, seen: map[string]bool{}}
	locals := map[string]bool{}
	for _, parameter := range lambda.Parameters {
		locals[parameter.Name] = true
	}
	collector.statements(lambda.Body, locals)
	return collector.names
}

func (c *v4CaptureCollector) identifier(node *ir.Identifier, locals map[string]bool) {
	if node == nil || !node.Lexical || locals[node.Name] || c.seen[node.Name] {
		return
	}
	if _, ok := c.outer[node.Name]; !ok {
		return
	}
	c.seen[node.Name] = true
	c.names = append(c.names, node.Name)
}

func (c *v4CaptureCollector) statements(statements []ir.Statement, locals map[string]bool) {
	for _, statement := range statements {
		c.statement(statement, locals)
	}
}

func (c *v4CaptureCollector) statement(statement ir.Statement, locals map[string]bool) {
	switch node := statement.(type) {
	case *ir.Variable:
		c.expression(node.Value, locals)
		locals[node.Name] = true
	case *ir.Assignment:
		c.expression(node.Target, locals)
		c.expression(node.Value, locals)
	case *ir.ExpressionStatement:
		c.expression(node.Expression, locals)
	case *ir.Return:
		c.expression(node.Value, locals)
	case *ir.If:
		c.ifNode(node, locals)
	case *ir.While:
		c.expression(node.Condition, locals)
		c.statements(node.Body, cloneV4Locals(locals))
	case *ir.Case:
		c.caseNode(node, locals)
	}
}

func (c *v4CaptureCollector) ifNode(node *ir.If, locals map[string]bool) {
	c.expression(node.Condition, locals)
	c.statements(node.Then, cloneV4Locals(locals))
	c.expression(node.ThenResult, locals)
	for _, branch := range node.ElseIf {
		c.expression(branch.Condition, locals)
		c.statements(branch.Body, cloneV4Locals(locals))
		c.expression(branch.Result, locals)
	}
	c.statements(node.Else, cloneV4Locals(locals))
	c.expression(node.ElseResult, locals)
}

func (c *v4CaptureCollector) caseNode(node *ir.Case, locals map[string]bool) {
	c.statements(node.Leading, locals)
	c.expression(node.Value, locals)
	for _, branch := range node.Branches {
		branchLocals := cloneV4Locals(locals)
		for _, binding := range branch.Bindings {
			branchLocals[binding.Name] = true
		}
		c.expression(branch.Value, branchLocals)
		for _, alternative := range branch.Alternatives {
			c.expression(alternative, branchLocals)
		}
		c.statements(branch.Body, branchLocals)
		c.expression(branch.Result, branchLocals)
	}
	c.statements(node.Else, cloneV4Locals(locals))
	c.expression(node.ElseResult, locals)
}

func (c *v4CaptureCollector) expression(expression ir.Expression, locals map[string]bool) {
	if expression == nil {
		return
	}
	switch node := expression.(type) {
	case *ir.Identifier:
		c.identifier(node, locals)
	case *ir.InterpolatedString:
		for _, part := range node.Parts {
			c.expression(part.Expression, locals)
		}
	case *ir.Array:
		for _, element := range node.Elements {
			c.expression(element, locals)
		}
	case *ir.Hash:
		for _, entry := range node.Entries {
			c.expression(entry.Key, locals)
			c.expression(entry.Value, locals)
		}
	case *ir.Unary:
		c.expression(node.Operand, locals)
	case *ir.Binary:
		c.expression(node.Left, locals)
		c.expression(node.Right, locals)
	case *ir.Range:
		c.expression(node.Start, locals)
		c.expression(node.End, locals)
	case *ir.Call:
		c.expression(node.Callee, locals)
		for _, argument := range node.Arguments {
			c.expression(argument.Value, locals)
		}
		if node.Block != nil {
			c.expression(node.Block, locals)
		}
	case *ir.RecordConstruct:
		c.expression(node.Target, locals)
		for _, argument := range node.Arguments {
			c.expression(argument.Value, locals)
		}
	case *ir.EnumConstruct:
		for _, argument := range node.Arguments {
			c.expression(argument.Value, locals)
		}
	case *ir.EnumCall:
		c.expression(node.Receiver, locals)
		for _, argument := range node.Arguments {
			c.expression(argument.Value, locals)
		}
	case *ir.Member:
		c.expression(node.Receiver, locals)
	case *ir.Index:
		c.expression(node.Receiver, locals)
		c.expression(node.Index, locals)
	case *ir.Conversion:
		c.expression(node.Value, locals)
	case *ir.TypeApply:
		c.expression(node.Receiver, locals)
	case *ir.If:
		c.ifNode(node, locals)
	case *ir.Case:
		c.caseNode(node, locals)
	case *ir.Lambda:
		nestedLocals := cloneV4Locals(locals)
		for _, parameter := range node.Parameters {
			nestedLocals[parameter.Name] = true
		}
		c.statements(node.Body, nestedLocals)
	case *ir.Block:
		blockLocals := cloneV4Locals(locals)
		for _, parameter := range node.Parameters {
			blockLocals[parameter] = true
		}
		c.statements(node.Body, blockLocals)
	}
}

func cloneV4Locals(source map[string]bool) map[string]bool {
	result := make(map[string]bool, len(source))
	for name, local := range source {
		result[name] = local
	}
	return result
}
