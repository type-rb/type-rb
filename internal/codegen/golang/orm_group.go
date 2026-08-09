package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormGroup(call *ir.Call) string {
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
		name := member.Receiver.ExprType().Name
		if id, ok := member.Receiver.(*ir.Identifier); ok {
			name = id.Name
		}
		var exists bool
		model, exists = g.orm.Model(name)
		if !exists {
			return "nil"
		}
		query = g.ormModelQualifier(model) + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	columnName := ormProjectionColumn(call.Arguments[0].Value)
	column, ok := model.Column(columnName)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMGroup(model, column) + "(" + query + ")"
}

func (g *generator) ormGroupHaving(call *ir.Call, arguments []string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(arguments) != 4 {
		return "nil"
	}
	model, column, ok := g.orm.GroupModel(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMGroupHaving(model, column) + "(" + arguments[0] + ", " + arguments[2] + ", " + arguments[3] + ")"
}

func (g *generator) ormGroupCount(call *ir.Call, arguments []string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(arguments) != 1 {
		return "nil"
	}
	model, column, ok := g.orm.GroupModel(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMGroupCount(model, column) + "(" + arguments[0] + ")"
}

func (g *generator) ormGroupRuntime(adapter ormintegration.Adapter, model ormintegration.Model, column ormintegration.Column) {
	groupType := goORMGroupType(model, column)
	queryType := goORMQueryType(model)
	keyType := column.Type
	resultType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{keyType, types.FromName("Integer")}}
	g.line("type " + groupType + " struct { query " + queryType + "; havingOperator string; havingValue *int }")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroup(model, column) + "(query " + queryType + ") " + groupType + " {")
	g.indent++
	g.line("if len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || len(query.preloads) > 0 { panic(\"ORM group currently accepts predicates and joins, but not order, limit, offset, lock, or preload\") }")
	g.line("return " + groupType + "{query: query}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroupHaving(model, column) + "(grouped " + groupType + ", operator string, value int) " + groupType + " {")
	g.indent++
	g.line("switch operator { case \"=\", \"!=\", \"<\", \"<=\", \">\", \">=\": default: panic(\"unsupported ORM having operator\") }; grouped.havingOperator = operator; grouped.havingValue = &value; return grouped")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroupCount(model, column) + "(grouped " + groupType + ") " + g.ormResultType(resultType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(grouped.query.transaction, false); if databaseError != nil { return " + g.ormResultErr(resultType, "*databaseError") + " }")
	g.line("projection := " + strconv.Quote(adapter.QuoteIdentifier(column.Name)+" AS "+adapter.QuoteIdentifier("trb_group")))
	g.line("statement, arguments := " + goORMStatement(model) + "(grouped.query, projection)")
	g.line("statement = \"SELECT \" + trbOrmQuoteIdentifier(\"trb_group\") + \", COUNT(*) FROM (\" + statement + \") AS trb_grouped GROUP BY \" + trbOrmQuoteIdentifier(\"trb_group\")")
	g.line("if grouped.havingValue != nil { statement += \" HAVING COUNT(*) \" + grouped.havingOperator + \" \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, *grouped.havingValue) }")
	g.line("rows, err := database.Query(statement, arguments...); if err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database grouped count query failed\")") + " }; defer rows.Close()")
	g.line("values := make(" + g.goType(resultType) + ")")
	g.line("for rows.Next() { var key " + g.goType(keyType) + "; var count int; if err := rows.Scan(&key, &count); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database grouped count row was invalid\")") + " }; values[key] = count }")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database grouped count query failed\")") + " }; return " + g.ormResultOK(resultType, "values"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func goORMGroupType(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrm" + goIdentifier(model.GroupType(column), true)
}
func goORMGroup(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmGroup" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
func goORMGroupHaving(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmHaving" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
func goORMGroupCount(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmCountGrouped" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
