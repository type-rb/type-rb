package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormProjection(call *ir.Call, arguments []string, operation string) string {
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
		qualifier := g.ormModelQualifier(model)
		query = qualifier + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	column := ""
	if operation == "ids" {
		primaryKey, exists := model.PrimaryKey()
		if !exists {
			return "nil"
		}
		column = primaryKey.Name
	} else if len(call.Arguments) == 1 {
		column = ormProjectionColumn(call.Arguments[0].Value)
	}
	if _, exists := model.Column(column); !exists {
		return "nil"
	}
	query = g.ormExecutionQuery(model, query)
	helper := goORMPluck(model, column)
	if operation == "pick" {
		helper = goORMPick(model, column)
	}
	return g.ormModelQualifier(model) + helper + "(" + query + ")"
}

func ormProjectionColumn(expression ir.Expression) string {
	switch value := expression.(type) {
	case *ir.Symbol:
		return value.Name
	case *ir.Literal:
		if decoded, err := strconv.Unquote(value.Raw); err == nil {
			return decoded
		}
		return strings.Trim(value.Raw, "'\"")
	default:
		return ""
	}
}

func (g *generator) ormProjectionRuntime(adapter ormintegration.Adapter, model ormintegration.Model, column ormintegration.Column) {
	queryType := goORMQueryType(model)
	elementType := column.Type
	arrayType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{elementType}}
	g.line("func " + goORMPluck(model, column.Name) + "(query " + queryType + ") " + g.ormResultType(arrayType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(query.scope, query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(arrayType, "*databaseError") + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(adapter.QuoteIdentifier(column.Name)) + ")")
	g.line("rows, err := database.Query(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(arrayType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database projection query failed\")") + " }")
	g.line("defer rows.Close()")
	g.line("result := []" + g.goType(elementType) + "{}")
	g.line("for rows.Next() {")
	g.indent++
	if ormintegration.IsPortableTimeType(elementType) {
		g.line("var raw any")
		g.line("if err := rows.Scan(&raw); err != nil { return " + g.ormResultErr(arrayType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database projection row was invalid\")") + " }")
		g.line("value, conversionError := " + goORMTemporalScan(elementType.Name) + "(raw); if conversionError != nil { return " + g.ormResultErr(arrayType, "trbOrmError(conversionError, "+g.ormErrorKind("InvalidData")+", \"database projection row was invalid\")") + " }")
		if !elementType.Nullable {
			g.line("if value == nil { return " + g.ormResultErr(arrayType, g.ormErrorValue("InvalidData", "database projection value must not be null")) + " }")
		}
	} else {
		g.line("var value " + g.goType(elementType))
		g.line("if err := rows.Scan(&value); err != nil { return " + g.ormResultErr(arrayType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database projection row was invalid\")") + " }")
	}
	g.line("result = append(result, value)")
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(arrayType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database projection query failed\")") + " }")
	g.line("return " + g.ormResultOK(arrayType, "result"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	pickType := column.Type
	pickType.Nullable = true
	g.line("func " + goORMPick(model, column.Name) + "(query " + queryType + ") " + g.ormResultType(pickType) + " {")
	g.indent++
	g.line("if query.limit == nil || *query.limit > 1 { count := 1; query.limit = &count }")
	g.line("database, databaseError := trbOrmExecutorForQuery(query.scope, query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(pickType, "*databaseError") + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(adapter.QuoteIdentifier(column.Name)) + ")")
	g.line("row := database.QueryRow(statement, arguments...)")
	if ormintegration.IsPortableTimeType(elementType) {
		g.line("var raw any")
		g.line("if err := row.Scan(&raw); err != nil {")
	} else {
		g.line("var value " + g.goType(elementType))
		g.line("if err := row.Scan(&value); err != nil {")
	}
	g.indent++
	g.line("if errors.Is(err, sql.ErrNoRows) { return " + g.ormResultOK(pickType, "nil") + " }")
	g.line("return " + g.ormResultErr(pickType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database projection query failed\")"))
	g.indent--
	g.line("}")
	value := "&value"
	if ormintegration.IsPortableTimeType(elementType) {
		g.line("value, conversionError := " + goORMTemporalScan(elementType.Name) + "(raw); if conversionError != nil { return " + g.ormResultErr(pickType, "trbOrmError(conversionError, "+g.ormErrorKind("InvalidData")+", \"database projection value was invalid\")") + " }")
		value = "value"
	} else if elementType.Nullable {
		value = "value"
	}
	g.line("return " + g.ormResultOK(pickType, value))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func goORMPluck(model ormintegration.Model, column string) string {
	return "TrbOrmPluck" + goIdentifier(model.Name, true) + goIdentifier(column, true)
}

func goORMPick(model ormintegration.Model, column string) string {
	return "TrbOrmPick" + goIdentifier(model.Name, true) + goIdentifier(column, true)
}
