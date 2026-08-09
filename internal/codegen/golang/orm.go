package golang

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

func (g *generator) ormWhere(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	modelName := member.Receiver.ExprType().Name
	if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
		modelName = identifier.Name
	}
	model, exists := g.orm.Model(modelName)
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMWhere(model) + "(" + g.ormPredicateArguments(call) + ")"
}

func (g *generator) ormPredicateArguments(call *ir.Call) string {
	predicates := ormintegration.Predicates(call)
	columns := make([]string, len(predicates))
	operators := make([]string, len(predicates))
	values := make([]string, len(predicates))
	for index, predicate := range predicates {
		columns[index] = strconv.Quote(predicate.Column)
		operators[index] = strconv.Quote(string(predicate.Operator))
		values[index] = g.expr(predicate.Value)
	}
	return "[]string{" + strings.Join(columns, ", ") + "}, []string{" + strings.Join(operators, ", ") + "}, []any{" + strings.Join(values, ", ") + "}"
}

func (g *generator) ormQueryModel(call *ir.Call, arguments []string) (ormintegration.Model, string, bool) {
	if len(arguments) == 0 {
		return ormintegration.Model{}, "", false
	}
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ormintegration.Model{}, "", false
	}
	model, exists := g.orm.QueryModel(member.Receiver.ExprType().Name)
	if !exists {
		return ormintegration.Model{}, "", false
	}
	return model, arguments[0], true
}

func (g *generator) ormQueryWhere(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMQueryWhere(model) + "(" + query + ", " + g.ormPredicateArguments(call) + ")"
}

func (g *generator) ormOrder(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	columns := make([]string, 0, len(call.Arguments))
	directions := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			continue
		}
		columns = append(columns, strconv.Quote(argument.Name))
		directions = append(directions, g.expr(argument.Value))
	}
	return g.ormModelQualifier(model) + goORMOrder(model) + "(" + query + ", []string{" + strings.Join(columns, ", ") + "}, []string{" + strings.Join(directions, ", ") + "})"
}

func (g *generator) ormQueryInteger(call *ir.Call, arguments []string, operation func(ormintegration.Model) string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok || len(arguments) < 2 {
		return "nil"
	}
	return g.ormModelQualifier(model) + operation(model) + "(" + query + ", " + arguments[1] + ")"
}

func (g *generator) ormQueryTerminal(call *ir.Call, arguments []string, operation func(ormintegration.Model) string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + operation(model) + "(" + query + ")"
}

func (g *generator) ormRuntime(manifest *ormintegration.Manifest) {
	models := manifest.ModelsForModule(g.modulePath)
	if len(models) == 0 {
		return
	}
	g.requireImport("database/sql", "sql")
	g.requireImport("modernc.org/sqlite", "_")
	g.requireImport("reflect", "")
	g.requireImport("strings", "")
	for _, model := range models {
		g.ormModelRuntime(manifest, model)
	}
}

func (g *generator) ormModelRuntime(manifest *ormintegration.Manifest, model ormintegration.Model) {
	conditionType := goORMConditionType(model)
	orderType := goORMOrderType(model)
	queryType := goORMQueryType(model)
	g.line("type " + conditionType + " struct {")
	g.indent++
	g.line("column string")
	g.line("operator string")
	g.line("value any")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type " + orderType + " struct {")
	g.indent++
	g.line("column string")
	g.line("direction string")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("type " + queryType + " struct {")
	g.indent++
	g.line("conditions []" + conditionType)
	g.line("orders []" + orderType)
	g.line("limit *int")
	g.line("offset *int")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMWhere(model) + "(columns []string, operators []string, values []any) " + queryType + " {")
	g.indent++
	g.line("return " + goORMQueryWhere(model) + "(" + queryType + "{}, columns, operators, values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMQueryWhere(model) + "(query " + queryType + ", columns []string, operators []string, values []any) " + queryType + " {")
	g.indent++
	g.line("result := query")
	g.line("result.conditions = append([]" + conditionType + "(nil), query.conditions...)")
	g.line("for index, column := range columns {")
	g.indent++
	g.line("result.conditions = append(result.conditions, " + conditionType + "{column: column, operator: operators[index], value: values[index]})")
	g.indent--
	g.line("}")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMOrder(model) + "(query " + queryType + ", columns []string, directions []string) " + queryType + " {")
	g.indent++
	g.line("result := query")
	g.line("result.orders = append([]" + orderType + "(nil), query.orders...)")
	g.line("for index, column := range columns {")
	g.indent++
	g.line("result.orders = append(result.orders, " + orderType + "{column: column, direction: directions[index]})")
	g.indent--
	g.line("}")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMLimit(model) + "(query " + queryType + ", count int) " + queryType + " {")
	g.indent++
	g.line("if count < 0 { panic(\"ORM limit must be non-negative\") }")
	g.line("query.limit = &count")
	g.line("return query")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMOffset(model) + "(query " + queryType + ", count int) " + queryType + " {")
	g.indent++
	g.line("if count < 0 { panic(\"ORM offset must be non-negative\") }")
	g.line("query.offset = &count")
	g.line("return query")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	columns := make([]string, len(model.Columns))
	scanTargets := make([]string, len(model.Columns))
	for index, column := range model.Columns {
		columns[index] = quoteORMIdentifier(column.Name)
		scanTargets[index] = "&value." + goFieldName(column.Name)
		g.line("func (self *" + goIdentifier(model.Name, true) + ") " + goORMColumnGetter(column.Name) + "() " + g.goType(column.Type) + " {")
		g.indent++
		g.line("return self." + goFieldName(column.Name))
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
	statement := " FROM " + quoteORMIdentifier(model.Table)
	g.line("func " + goORMStatement(model) + "(query " + queryType + ", projection string) (string, []any) {")
	g.indent++
	g.line("statement := \"SELECT \" + projection + " + strconv.Quote(statement))
	g.line("arguments := []any{}")
	g.line("if len(query.conditions) > 0 {")
	g.indent++
	g.line("clauses := make([]string, 0, len(query.conditions))")
	g.line("for _, condition := range query.conditions {")
	g.indent++
	g.line("switch condition.operator { case \"=\", \"!=\", \"<\", \"<=\", \">\", \">=\": default: panic(\"unsupported ORM comparison operator\") }")
	g.line("column := \"\\\"\" + strings.ReplaceAll(condition.column, \"\\\"\", \"\\\"\\\"\") + \"\\\"\"")
	g.line("nilValue := condition.value == nil")
	g.line("if !nilValue { reflected := reflect.ValueOf(condition.value); nilValue = reflected.Kind() == reflect.Ptr && reflected.IsNil() }")
	g.line("if nilValue && condition.operator == \"=\" {")
	g.indent++
	g.line("clauses = append(clauses, column+\" IS NULL\")")
	g.indent--
	g.line("} else if nilValue && condition.operator == \"!=\" {")
	g.indent++
	g.line("clauses = append(clauses, column+\" IS NOT NULL\")")
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("clauses = append(clauses, column+\" \"+condition.operator+\" ?\")")
	g.line("arguments = append(arguments, condition.value)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("statement += \" WHERE \" + strings.Join(clauses, \" AND \")")
	g.indent--
	g.line("}")
	g.line("if len(query.orders) > 0 {")
	g.indent++
	g.line("orders := make([]string, 0, len(query.orders))")
	g.line("for _, order := range query.orders {")
	g.indent++
	g.line("switch order.direction { case \"asc\", \"desc\": default: panic(\"unsupported ORM order direction\") }")
	g.line("column := \"\\\"\" + strings.ReplaceAll(order.column, \"\\\"\", \"\\\"\\\"\") + \"\\\"\"")
	g.line("orders = append(orders, column+\" \"+strings.ToUpper(order.direction))")
	g.indent--
	g.line("}")
	g.line("statement += \" ORDER BY \" + strings.Join(orders, \", \")")
	g.indent--
	g.line("}")
	g.line("if query.limit != nil { statement += \" LIMIT ?\"; arguments = append(arguments, *query.limit) } else if query.offset != nil { statement += \" LIMIT -1\" }")
	g.line("if query.offset != nil { statement += \" OFFSET ?\"; arguments = append(arguments, *query.offset) }")
	g.line("return statement, arguments")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + goORMLoader(model) + "(query " + queryType + ") []*" + goIdentifier(model.Name, true) + " {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(manifest.Adapter) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { panic(err) }")
	g.line("defer database.Close()")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(strings.Join(columns, ", ")) + ")")
	g.line("rows, err := database.Query(statement, arguments...)")
	g.line("if err != nil { panic(err) }")
	g.line("defer rows.Close()")
	g.line("result := []*" + goIdentifier(model.Name, true) + "{}")
	g.line("for rows.Next() {")
	g.indent++
	g.line("value := &" + goIdentifier(model.Name, true) + "{}")
	g.line("if err := rows.Scan(" + strings.Join(scanTargets, ", ") + "); err != nil { panic(err) }")
	g.line("result = append(result, value)")
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { panic(err) }")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + goORMFirst(model) + "(query " + queryType + ") *" + goIdentifier(model.Name, true) + " {")
	g.indent++
	g.line("if query.limit == nil || *query.limit > 1 { count := 1; query.limit = &count }")
	g.line("values := " + goORMLoader(model) + "(query)")
	g.line("if len(values) == 0 { return nil }")
	g.line("return values[0]")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + goORMCount(model) + "(query " + queryType + ") int {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(manifest.Adapter) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { panic(err) }")
	g.line("defer database.Close()")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, \"1\")")
	g.line("row := database.QueryRow(\"SELECT COUNT(*) FROM (\"+statement+\") AS trb_count\", arguments...)")
	g.line("var count int")
	g.line("if err := row.Scan(&count); err != nil { panic(err) }")
	g.line("return count")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func goORMQueryType(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Query"
}

func goORMConditionType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Condition"
}

func goORMOrderType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Order"
}

func goORMWhere(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Where"
}

func goORMQueryWhere(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "QueryWhere"
}

func goORMOrder(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Order"
}

func goORMLimit(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Limit"
}

func goORMOffset(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Offset"
}

func goORMLoader(model ormintegration.Model) string {
	return "TrbOrmLoad" + goIdentifier(model.Name, true)
}

func goORMFirst(model ormintegration.Model) string {
	return "TrbOrmFirst" + goIdentifier(model.Name, true)
}

func goORMCount(model ormintegration.Model) string {
	return "TrbOrmCount" + goIdentifier(model.Name, true)
}

func goORMStatement(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Statement"
}

func goORMColumnGetter(column string) string {
	return "TrbOrmColumn" + goIdentifier(column, true)
}

func (g *generator) ormModelQualifier(model ormintegration.Model) string {
	directory := pathpkg.Dir(model.ModulePath)
	if directory == "." {
		directory = ""
	}
	if directory == g.currentDirectory() {
		return ""
	}
	importPath := directory
	if g.goModule != "" {
		importPath = pathpkg.Join(g.goModule, directory)
	}
	if importPath == "" {
		return ""
	}
	alias, imported := g.imports[importPath]
	if !imported {
		alias = pathpkg.Base(directory)
		g.requireImport(importPath, alias)
	}
	return goImportAlias(alias) + "."
}

func quoteORMIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
