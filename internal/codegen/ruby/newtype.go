package ruby

import (
	"encoding/hex"
	"strings"

	"github.com/type-rb/type-rb/internal/identity"
	"github.com/type-rb/type-rb/internal/ir"
)

func newtypeNamespace(owner identity.Declaration) string {
	return "::TrbNewtype_" + hex.EncodeToString([]byte(owner.Module+"\x00"+owner.Name))
}

func (g *generator) newtypeMethods(value *ir.Newtype) {
	if len(value.Body) == 0 {
		return
	}
	g.line("module "+newtypeNamespace(value.Declaration), value.TrailingComment)
	g.indent++
	for _, statement := range value.Body {
		method, ok := statement.(*ir.Method)
		if !ok || method.External {
			continue
		}
		previousSelf, hadSelf := g.lexicalNames["self"]
		g.lexicalNames["self"] = "__trb_self"
		parameters := g.methodParameters(method)
		if !method.Class {
			if parameters != "" {
				parameters = ", " + parameters
			}
			parameters = "__trb_self" + parameters
		}
		g.line("def self."+method.Name+"("+parameters+")", method.TrailingComment)
		g.indent++
		previousExecution := g.executionActive
		g.executionActive = g.methodUsesExecutionScope(method)
		g.statements(method.Body)
		g.executionActive = previousExecution
		g.indent--
		g.line("end", "")
		if hadSelf {
			g.lexicalNames["self"] = previousSelf
		} else {
			delete(g.lexicalNames, "self")
		}
	}
	g.indent--
	g.line("end", "")
}

func (g *generator) newtypeMethodCall(call *ir.Call, arguments []string) string {
	method := call.NewtypeMethod
	owner := newtypeNamespace(method.Dispatch.Owner)
	arguments = g.executionArguments(call, arguments)
	if receiver := call.NewtypeReceiver(); receiver != nil {
		if member, ok := receiverMember(call.Callee); ok && member.Safe {
			arguments = append([]string{"__trb_safe_receiver"}, arguments...)
			body := owner + "." + method.Dispatch.Name + "(" + strings.Join(arguments, ", ") + ")"
			return "->(__trb_safe_receiver) { __trb_safe_receiver.nil? ? nil : " + body + " }.call(" + g.expr(receiver) + ")"
		}
		arguments = append([]string{g.expr(receiver)}, arguments...)
	}
	return owner + "." + method.Dispatch.Name + "(" + strings.Join(arguments, ", ") + ")"
}
