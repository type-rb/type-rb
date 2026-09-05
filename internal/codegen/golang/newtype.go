package golang

import (
	"encoding/hex"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

func newtypeMethodName(dispatch identity.Dispatch) string {
	return "TrbNewtype_" + hex.EncodeToString([]byte(dispatch.Owner.Module+"\x00"+dispatch.Owner.Name+"\x00"+dispatch.Name))
}

func (g *generator) newtypeMethods(value *ir.Newtype) {
	for _, statement := range value.Body {
		method, ok := statement.(*ir.Method)
		if !ok || method.External {
			continue
		}
		parameters := g.methodParameters(method)
		if !method.Class {
			if parameters != "" {
				parameters = ", " + parameters
			}
			parameters = "self " + goDeclaredTypeName(value.Declaration.Name, value.Name) + parameters
		}
		g.line("func " + newtypeMethodName(method.Dispatch) + goTypeParameterDeclarations(method.TypeParameters) + "(" + parameters + ")" + g.goReturn(method.ReturnType) + " {")
		g.indent++
		previousExecution, previousReturn := g.executionActive, g.returnType
		g.executionActive, g.returnType = g.methodUsesExecutionScope(method), method.ReturnType
		g.functionDepth++
		g.parameterDefaults(method.Parameters)
		g.statements(method.Body)
		g.functionDepth--
		g.executionActive, g.returnType = previousExecution, previousReturn
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
}

func (g *generator) newtypeMethodCall(call *ir.Call, arguments []string) string {
	method := call.NewtypeMethod
	name := newtypeMethodName(method.Dispatch)
	if alias := g.referenceAlias(method.Reference); alias != "" {
		name = alias + "." + name
	}
	if application, ok := call.Callee.(*ir.TypeApply); ok && len(application.Arguments) > 0 {
		items := make([]string, len(application.Arguments))
		for index, argument := range application.Arguments {
			items[index] = g.goType(argument)
		}
		name += "[" + strings.Join(items, ", ") + "]"
	}
	arguments = g.sourceCallArguments(call.Arguments, call.CallSignature, arguments)
	arguments = g.executionArguments(call, arguments)
	if receiver := call.NewtypeReceiver(); receiver != nil {
		arguments = append([]string{g.expr(receiver)}, arguments...)
	}
	return name + "(" + strings.Join(arguments, ", ") + ")"
}
