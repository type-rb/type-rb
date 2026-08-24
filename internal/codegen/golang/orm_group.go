package golang

import (
	"strconv"
	"strings"

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
	query = g.ormExecutionQuery(model, query)
	return g.ormModelQualifier(model) + goORMGroup(model, column) + "(" + query + ")"
}

func (g *generator) ormGroupHaving(call *ir.Call, arguments []string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(arguments) < 4 || len(arguments) > 5 {
		return "nil"
	}
	model, column, ok := g.orm.GroupModel(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	expression, operatorIndex, valueIndex := "COUNT(*)", 2, 3
	if len(arguments) == 5 {
		operation := ormProjectionColumn(call.Arguments[0].Value)
		targetName := ormProjectionColumn(call.Arguments[1].Value)
		_, exists := model.Column(targetName)
		if !exists {
			return "nil"
		}
		expression, operatorIndex, valueIndex = groupedAggregateExpression(operation, "trb_value"), 3, 4
	}
	return g.ormModelQualifier(model) + goORMGroupHaving(model, column) + "(" + arguments[0] + ", " + strconv.Quote(expression) + ", " + arguments[operatorIndex] + ", " + arguments[valueIndex] + ")"
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
	grouped := g.ormModelQualifier(model) + goORMGroupExecutionScope(model, column) + "(" + arguments[0] + ", __trbScope)"
	return g.ormModelQualifier(model) + goORMGroupCount(model, column) + "(" + grouped + ")"
}

func (g *generator) ormGroupAggregate(call *ir.Call, arguments []string, operation string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(arguments) != 2 {
		return "nil"
	}
	model, groupColumn, ok := g.orm.GroupModel(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	targetName := ormProjectionColumn(call.Arguments[0].Value)
	target, ok := model.Column(targetName)
	if !ok {
		return "nil"
	}
	if _, ok := ormintegration.AggregateResultType(operation, target); !ok {
		return "nil"
	}
	grouped := g.ormModelQualifier(model) + goORMGroupExecutionScope(model, groupColumn) + "(" + arguments[0] + ", __trbScope)"
	return g.ormModelQualifier(model) + goORMGroupedAggregate(model, groupColumn, operation, target) + "(" + grouped + ")"
}

func (g *generator) ormGroupRuntime(adapter ormintegration.Adapter, model ormintegration.Model, groupColumn ormintegration.Column) {
	groupType := goORMGroupType(model, groupColumn)
	queryType := goORMQueryType(model)
	g.line("type " + groupType + " struct { query " + queryType + "; orders []" + goORMOrderType(model) + "; limit *int; offset *int; havingExpression string; havingOperator string; havingValue any }")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroup(model, groupColumn) + "(query " + queryType + ") " + groupType + " {")
	g.indent++
	g.line("if query.lock || query.distinct || len(query.preloads) > 0 { panic(\"ORM group does not accept distinct, lock, or preload\") }")
	g.line("for _, order := range query.orders { if order.column != " + strconv.Quote(groupColumn.Name) + " { panic(\"ORM grouped order must use the group key\") } }")
	g.line("grouped := " + groupType + "{query: query, orders: append([]" + goORMOrderType(model) + "(nil), query.orders...), limit: query.limit, offset: query.offset}")
	g.line("grouped.query.orders = nil; grouped.query.limit = nil; grouped.query.offset = nil; return grouped")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroupExecutionScope(model, groupColumn) + "(grouped " + groupType + ", scope trbcontext.Context) " + groupType + " { grouped.query.scope = scope; return grouped }")
	g.b.WriteByte('\n')
	g.line("func " + goORMGroupHaving(model, groupColumn) + "(grouped " + groupType + ", expression string, operator string, value any) " + groupType + " {")
	g.indent++
	g.line("switch operator { case \"=\", \"!=\", \"<\", \"<=\", \">\", \">=\": default: panic(\"unsupported ORM having operator\") }; grouped.havingExpression = expression; grouped.havingOperator = operator; grouped.havingValue = value; return grouped")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.ormGroupedAggregateRuntime(adapter, model, groupColumn, "count", ormintegration.Column{}, types.FromName("Integer"))
	for _, operation := range ormintegration.AggregateOperations() {
		for _, target := range model.Columns {
			if result, ok := ormintegration.AggregateResultType(operation, target); ok {
				g.ormGroupedAggregateRuntime(adapter, model, groupColumn, operation, target, result)
			}
		}
	}
}

func (g *generator) ormGroupedAggregateRuntime(adapter ormintegration.Adapter, model ormintegration.Model, groupColumn ormintegration.Column, operation string, target ormintegration.Column, valueType types.Type) {
	groupType := goORMGroupType(model, groupColumn)
	resultType := types.Type{Kind: types.Hash, Name: "Hash", Args: []types.Type{groupColumn.Type, valueType}}
	functionName, expression, label := goORMGroupCount(model, groupColumn), "COUNT(*)", "count"
	projection := adapter.QuoteIdentifier(groupColumn.Name) + " AS " + adapter.QuoteIdentifier("trb_group")
	if operation != "count" {
		functionName = goORMGroupedAggregate(model, groupColumn, operation, target)
		expression, label = groupedAggregateExpression(operation, "trb_value"), operation
		projection += ", " + adapter.QuoteIdentifier(target.Name) + " AS " + adapter.QuoteIdentifier("trb_value")
	}
	g.line("func " + functionName + "(grouped " + groupType + ") " + g.ormResultType(resultType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(grouped.query.scope, grouped.query.transaction, false); if databaseError != nil { return " + g.ormResultErr(resultType, "*databaseError") + " }")
	g.line("projection := " + strconv.Quote(projection))
	g.line("statement, arguments := " + goORMStatement(model) + "(grouped.query, projection)")
	g.line("statement = \"SELECT \" + trbOrmQuoteIdentifier(\"trb_group\") + \", " + expression + " FROM (\" + statement + \") AS trb_grouped GROUP BY \" + trbOrmQuoteIdentifier(\"trb_group\")")
	g.line("if grouped.havingExpression != \"\" { statement += \" HAVING \" + grouped.havingExpression + \" \" + grouped.havingOperator + \" \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, grouped.havingValue) }")
	g.line("if len(grouped.orders) > 0 { orders := make([]string, len(grouped.orders)); for index, order := range grouped.orders { orders[index] = trbOrmQuoteIdentifier(\"trb_group\") + \" \" + strings.ToUpper(order.direction) }; statement += \" ORDER BY \" + strings.Join(orders, \", \") }")
	g.line("if grouped.limit != nil { statement += \" LIMIT \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, *grouped.limit) } else if grouped.offset != nil { statement += " + strconv.Quote(adapter.OffsetNoLimit) + " }")
	g.line("if grouped.offset != nil { statement += \" OFFSET \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, *grouped.offset) }")
	g.line("rows, err := database.Query(statement, arguments...); if err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database grouped "+label+" query failed\")") + " }; defer rows.Close()")
	g.line("values := make(" + g.goType(resultType) + ")")
	if ormintegration.IsPortableTimeType(valueType) {
		g.line("for rows.Next() {")
		g.indent++
		g.line("var key " + g.goType(groupColumn.Type) + "; var raw any")
		g.line("if err := rows.Scan(&key, &raw); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database grouped "+label+" row was invalid\")") + " }")
		if groupColumn.Type.Kind == types.Int {
			g.line("if " + goORMIntegerOutside("key", groupColumn.Type.Nullable) + " { return " + g.ormResultErr(resultType, g.ormErrorValue("InvalidData", "database Integer is outside the portable range")) + " }")
		}
		g.line("value, conversionError := " + goORMTemporalScan(valueType.Name) + "(raw)")
		g.line("if conversionError != nil { return " + g.ormResultErr(resultType, "trbOrmError(conversionError, "+g.ormErrorKind("InvalidData")+", \"database grouped "+label+" row was invalid\")") + " }")
		g.line("values[key] = value")
		g.indent--
		g.line("}")
	} else {
		g.line("for rows.Next() {")
		g.indent++
		g.line("var key " + g.goType(groupColumn.Type) + "; var value " + g.goType(valueType))
		g.line("if err := rows.Scan(&key, &value); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database grouped "+label+" row was invalid\")") + " }")
		if groupColumn.Type.Kind == types.Int {
			g.line("if " + goORMIntegerOutside("key", groupColumn.Type.Nullable) + " { return " + g.ormResultErr(resultType, g.ormErrorValue("InvalidData", "database Integer is outside the portable range")) + " }")
		}
		if valueType.Kind == types.Int {
			g.line("if " + goORMIntegerOutside("value", valueType.Nullable) + " { return " + g.ormResultErr(resultType, g.ormErrorValue("InvalidData", "database Integer is outside the portable range")) + " }")
		}
		g.line("values[key] = value")
		g.indent--
		g.line("}")
	}
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(resultType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database grouped "+label+" query failed\")") + " }; return " + g.ormResultOK(resultType, "values"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func groupedAggregateExpression(operation, column string) string {
	function := strings.ToUpper(operation)
	switch operation {
	case "average":
		function = "AVG"
	case "minimum":
		function = "MIN"
	case "maximum":
		function = "MAX"
	}
	expression := function + "(" + column + ")"
	if operation == "sum" {
		expression = "COALESCE(" + expression + ", 0)"
	}
	return expression
}

func goORMGroupType(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrm" + goIdentifier(model.GroupType(column), true)
}
func goORMGroup(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmGroup" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
func goORMGroupExecutionScope(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrm" + goIdentifier(model.GroupType(column), true) + "ExecutionScope"
}
func goORMGroupHaving(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmHaving" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
func goORMGroupCount(model ormintegration.Model, column ormintegration.Column) string {
	return "TrbOrmCountGrouped" + goIdentifier(model.Name, true) + goIdentifier(column.Name, true)
}
func goORMGroupedAggregate(model ormintegration.Model, groupColumn ormintegration.Column, operation string, target ormintegration.Column) string {
	return "TrbOrm" + goIdentifier(operation, true) + "Grouped" + goIdentifier(model.Name, true) + goIdentifier(groupColumn.Name, true) + goIdentifier(target.Name, true)
}
