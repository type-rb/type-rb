package bootstrapsnapshot

import (
	"fmt"
	"maps"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

// Direct Array iteration is control flow, not a closure invocation. All live
// values travel through block parameters, including the retained source and
// shadowed outer bindings; user changes to an index binding cannot move the loop.
func (l *v3FunctionLowerer) lowerArrayIteration(node *ir.Iterate) (bool, error) {
	arity := 1
	if node.WithIndex {
		arity++
	}
	if l.version != Version4 || node.Source == nil || node.Operation != "each" ||
		node.Intrinsic != "" || node.SliceSize != nil || node.Result != nil ||
		node.ResultBoundary || node.CaptureEffect || len(node.Bindings) != arity {
		return false, l.unsupported(node.SourceSpan(), "direct Array each iteration")
	}
	typeID, err := l.typeName(node.Source.ExprType(), node.SourceSpan())
	if err != nil {
		return false, err
	}
	definition, ok := l.registry.definition(typeID)
	if !ok || definition.Kind != "array" || definition.Element == nil {
		return false, l.unsupported(node.SourceSpan(), "iteration source "+node.Source.ExprType().String())
	}
	for index, binding := range node.Bindings {
		want := *definition.Element
		if index == 1 {
			want = "Integer"
		}
		got, err := l.typeName(binding.Type, node.SourceSpan())
		if err != nil || got != want || binding.Name == "" ||
			(index == 1 && binding.Name == node.Bindings[0].Name) {
			return false, l.unsupported(node.SourceSpan(), "Array iteration binding")
		}
	}
	receiver, err := l.lowerExpression(node.Source)
	if err != nil {
		return false, err
	}
	at := l.origin(node.SourceSpan())
	outerNames := append([]string(nil), l.locals...)
	loopNames := append([]string(nil), outerNames...)
	loopEnv := cloneV3Env(l.env)
	prefix := fmt.Sprintf("\x00iteration%d.", l.nextBlock)
	restored := map[string]string{}
	captures := l.captures
	l.captures = maps.Clone(captures)
	defer func() { l.captures = captures }()
	for _, binding := range node.Bindings {
		delete(l.captures, binding.Name)
		for index, name := range loopNames {
			if name == binding.Name {
				hidden := prefix + "outer." + name
				loopNames[index] = hidden
				loopEnv[hidden] = loopEnv[name]
				delete(loopEnv, name)
				restored[name] = hidden
			}
		}
	}
	receiverName, cursorName := prefix+"receiver", prefix+"cursor"
	cursor := v3ValueRef{id: l.newValue(), typ: types.FromName("Integer")}
	l.emit(IntegerLiteral{Op: "integer_literal", Result: cursor.id, Value: 0, Origin: at})
	loopEnv[receiverName], loopEnv[cursorName] = receiver, cursor
	loopNames = append(loopNames, receiverName, cursorName)
	previous := l.current
	header, headerEnv := l.newBlockWithLocals(node.SourceSpan(), loopNames, loopEnv)
	body, bodyEnv := l.newBlockWithLocals(node.SourceSpan(), loopNames, loopEnv)
	advance, advanceEnv := l.newBlockWithLocals(node.SourceSpan(), loopNames, loopEnv)
	done, doneEnv := l.newBlockWithLocals(node.SourceSpan(), loopNames, loopEnv)
	previous.Terminator = Jump{Op: "jump", Target: header.ID, Arguments: v3EnvironmentArguments(loopNames, loopEnv), Origin: at}

	l.current, l.env, l.locals = header, headerEnv, append([]string(nil), loopNames...)
	length := v3ValueRef{id: l.newValue(), typ: cursor.typ}
	l.emit(ArraySize{Op: "array_size", Result: length.id, Type: typeID, Array: l.env[receiverName].id, Origin: at})
	condition, err := l.emitBinary("<", l.env[cursorName], length, node.SourceSpan())
	if err != nil {
		return false, err
	}
	arguments := v3EnvironmentArguments(loopNames, l.env)
	l.current.Terminator = Branch{Op: "branch", Condition: condition.id,
		WhenTrue: body.ID, TrueArguments: arguments, WhenFalse: done.ID, FalseArguments: arguments, Origin: at}

	l.current, l.env, l.locals = body, bodyEnv, append([]string(nil), loopNames...)
	element := v3ValueRef{id: l.newValue(), typ: node.Bindings[0].Type}
	l.emit(ArrayGet{Op: "array_get", Result: element.id, Type: typeID,
		Array: l.env[receiverName].id, Index: l.env[cursorName].id, Origin: at})
	l.env[node.Bindings[0].Name] = element
	l.locals = append(l.locals, node.Bindings[0].Name)
	if node.WithIndex {
		l.env[node.Bindings[1].Name] = l.env[cursorName]
		l.locals = append(l.locals, node.Bindings[1].Name)
	}
	l.loops = append(l.loops, v3LoopTargets{header: advance.ID, exit: done.ID, locals: loopNames})
	terminated, err := l.lowerStatements(node.Body)
	l.loops = l.loops[:len(l.loops)-1]
	if err != nil {
		return false, err
	}
	if !terminated {
		l.current.Terminator = Jump{Op: "jump", Target: advance.ID, Arguments: v3EnvironmentArguments(loopNames, l.env), Origin: at}
	}
	l.current, l.env, l.locals = advance, advanceEnv, append([]string(nil), loopNames...)
	one := v3ValueRef{id: l.newValue(), typ: cursor.typ}
	l.emit(IntegerLiteral{Op: "integer_literal", Result: one.id, Value: 1, Origin: at})
	next, err := l.emitBinary("+", l.env[cursorName], one, node.SourceSpan())
	if err != nil {
		return false, err
	}
	l.env[cursorName] = next
	l.current.Terminator = Jump{Op: "jump", Target: header.ID, Arguments: v3EnvironmentArguments(loopNames, l.env), Origin: at}
	for name, hidden := range restored {
		doneEnv[name] = doneEnv[hidden]
		delete(doneEnv, hidden)
	}
	delete(doneEnv, receiverName)
	delete(doneEnv, cursorName)
	l.current, l.env, l.locals = done, doneEnv, outerNames
	return false, nil
}
