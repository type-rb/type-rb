package typescript

import (
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func newtypeContractHasMethods(contract ir.TypeContract) bool {
	for name := range contract.Members {
		if name != "new" && name != "value" {
			return true
		}
	}
	return false
}

func (g *generator) newtypeMethods(value *ir.Newtype) {
	if len(value.Body) == 0 {
		return
	}
	g.line("export const " + value.Name + " = {")
	g.indent++
	for _, statement := range value.Body {
		method, ok := statement.(*ir.Method)
		if !ok || method.External {
			continue
		}
		popTypeParameters := g.pushTypeParameters(method.TypeParameters)
		parameters := g.methodParameters(method)
		if !method.Class {
			if parameters != "" {
				parameters = ", " + parameters
			}
			parameters = "self: " + value.Name + parameters
		}
		prefix, result := "", g.tsType(method.ReturnType)
		if g.suspension != nil && g.suspension.Methods[method] {
			prefix, result = "async ", "Promise<"+result+">"
		}
		g.line("__trb_" + tsCallableName(method.Name) + ": " + prefix + tsTypeParameterDeclarations(method.TypeParameters) + "(" + parameters + "): " + result + " => {")
		g.indent++
		popExactTypes := g.pushExactTypeScope(method.Parameters)
		previousReceiver, previousExecution := g.enumReceiver, g.executionActive
		g.enumReceiver, g.executionActive = "self", g.methodUsesExecutionScope(method)
		g.functionDepth++
		g.parameterDefaults(method)
		g.statements(method.Body)
		g.functionDepth--
		g.enumReceiver, g.executionActive = previousReceiver, previousExecution
		popExactTypes()
		g.indent--
		g.line("},")
		popTypeParameters()
	}
	g.indent--
	g.line("};")
}

func (g *generator) newtypeMethodCall(call *ir.Call, arguments []string) string {
	method := call.NewtypeMethod
	owner := g.enumCallOwner(&ir.EnumCall{EnumName: method.Dispatch.Owner.Name, Owner: method.Dispatch.Owner.Name, OwnerIdentity: method.Dispatch.Owner, Reference: method.Reference})
	arguments = g.sourceCallArguments(call.Arguments, call.CallSignature, arguments, g.suspension != nil && g.suspension.CallParameterDefaults[call])
	arguments = g.executionArguments(call, arguments)
	if receiver := call.NewtypeReceiver(); receiver != nil {
		arguments = append([]string{g.expr(receiver)}, arguments...)
	}
	typeArguments := ""
	if application, ok := call.Callee.(*ir.TypeApply); ok && len(application.Arguments) > 0 {
		items := make([]string, len(application.Arguments))
		for index, argument := range application.Arguments {
			items[index] = g.tsType(argument)
		}
		typeArguments = "<" + strings.Join(items, ", ") + ">"
	}
	return g.awaitCall(call, owner+".__trb_"+tsCallableName(method.Dispatch.Name)+typeArguments+"("+strings.Join(arguments, ", ")+")")
}
