package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormSelect(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(call.Arguments) != 1 {
		return "nil"
	}
	model, queryReceiver := g.orm.QueryModel(member.Receiver.ExprType().Name)
	if !queryReceiver {
		model, queryReceiver = g.orm.ScopeModel(member.Receiver.ExprType().Name)
	}
	query := ""
	if queryReceiver {
		query = g.expr(member.Receiver)
	} else {
		modelName := member.Receiver.ExprType().Name
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			modelName = identifier.Name
		}
		var exists bool
		model, exists = g.orm.Model(modelName)
		if !exists {
			return "nil"
		}
		qualifier := g.ormModelQualifier(model)
		query = qualifier + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	column := ormProjectionColumn(call.Arguments[0].Value)
	if _, exists := model.Column(column); !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMSelect(model, column) + "(" + query + ")"
}

func (g *generator) ormSubqueryRuntime(adapter ormintegration.Adapter, model ormintegration.Model, column ormintegration.Column) {
	queryType := goORMQueryType(model)
	elementType := column.Type
	elementType.Nullable = false
	g.line("func " + goORMSelect(model, column.Name) + "(query " + queryType + ") " + g.goType(subqueryType(elementType)) + " {")
	g.indent++
	g.line("if query.lock || len(query.preloads) > 0 { panic(\"ORM select subquery does not accept lock or preload\") }")
	g.line("return " + g.ormLifecycleAlias() + ".NewTrbOrmSubquery[" + g.goType(elementType) + "](query.transaction, func(arguments *[]any) string { return " + goORMStatementAppend(model) + "(query, " + strconv.Quote(adapter.QuoteIdentifier(column.Name)) + ", arguments) })")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func subqueryType(element types.Type) types.Type {
	element.Nullable = false
	return types.Type{Kind: types.Named, Name: "Subquery", Args: []types.Type{element}}
}

func goORMSelect(model ormintegration.Model, column string) string {
	return "TrbOrmSelect" + goIdentifier(model.Name, true) + goIdentifier(column, true)
}
