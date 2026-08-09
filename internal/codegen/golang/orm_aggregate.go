package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormAggregate(call *ir.Call, arguments []string, operation string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, queryReceiver := g.orm.QueryModel(member.Receiver.ExprType().Name)
	if !queryReceiver {
		model, queryReceiver = g.orm.ScopeModel(member.Receiver.ExprType().Name)
	}
	query := ""
	if queryReceiver {
		if len(arguments) == 0 {
			return "nil"
		}
		query = arguments[0]
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
		query = g.ormModelQualifier(model) + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	if len(call.Arguments) != 1 {
		return "nil"
	}
	columnName := ormProjectionColumn(call.Arguments[0].Value)
	column, exists := model.Column(columnName)
	if !exists {
		return "nil"
	}
	if _, supported := ormintegration.AggregateResultType(operation, column); !supported {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMAggregate(model, operation, columnName) + "(" + query + ")"
}

func (g *generator) ormAggregateRuntime(adapter ormintegration.Adapter, model ormintegration.Model, column ormintegration.Column, operation string, resultType types.Type) {
	queryType := goORMQueryType(model)
	quotedValue := adapter.QuoteIdentifier("trb_value")
	projection := adapter.QuoteIdentifier(column.Name) + " AS " + quotedValue
	function := strings.ToUpper(operation)
	switch operation {
	case "average":
		function = "AVG"
	case "minimum":
		function = "MIN"
	case "maximum":
		function = "MAX"
	}
	expression := function + "(" + quotedValue + ")"
	if operation == "sum" {
		expression = "COALESCE(" + expression + ", 0)"
	}

	g.line("func " + goORMAggregate(model, operation, column.Name) + "(query " + queryType + ") " + g.ormResultType(resultType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(resultType, "*databaseError") + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(projection) + ")")
	g.line("rows, err := database.Query(" + strconv.Quote("SELECT "+expression+" FROM (") + "+statement+" + strconv.Quote(") AS trb_aggregate") + ", arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database aggregate query failed\")") + " }")
	g.line("defer rows.Close()")
	g.line("if !rows.Next() {")
	g.indent++
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database aggregate query failed\")") + " }")
	g.line("return " + g.ormResultErr(resultType, g.ormErrorValue("InvalidData", "database aggregate result was missing")))
	g.indent--
	g.line("}")
	g.line("var value " + g.goType(resultType))
	g.line("if err := rows.Scan(&value); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database aggregate result was invalid\")") + " }")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database aggregate query failed\")") + " }")
	g.line("return " + g.ormResultOK(resultType, "value"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func goORMAggregate(model ormintegration.Model, operation, column string) string {
	return "TrbOrm" + goIdentifier(operation, true) + goIdentifier(model.Name, true) + goIdentifier(column, true)
}
