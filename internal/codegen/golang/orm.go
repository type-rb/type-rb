package golang

import (
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
	conditions := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			continue
		}
		conditions = append(conditions, "{column: "+strconv.Quote(argument.Name)+", value: "+g.expr(argument.Value)+"}")
	}
	return goORMQueryType(model) + "{conditions: []trbOrmCondition{" + strings.Join(conditions, ", ") + "}}"
}

func (g *generator) ormAll(call *ir.Call, arguments []string) string {
	if len(arguments) == 0 {
		return "nil"
	}
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.QueryModel(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	return goORMLoader(model) + "(" + arguments[0] + ")"
}

func (g *generator) ormRuntime(manifest *ormintegration.Manifest) {
	models := manifest.ModelsForModule(g.modulePath)
	if len(models) == 0 {
		return
	}
	g.requireImport("database/sql", "sql")
	g.requireImport("modernc.org/sqlite", "_")
	g.requireImport("strings", "")
	g.line("type trbOrmCondition struct {")
	g.indent++
	g.line("column string")
	g.line("value any")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	for _, model := range models {
		g.ormModelRuntime(manifest, model)
	}
}

func (g *generator) ormModelRuntime(manifest *ormintegration.Manifest, model ormintegration.Model) {
	queryType := goORMQueryType(model)
	g.line("type " + queryType + " struct {")
	g.indent++
	g.line("conditions []trbOrmCondition")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	columns := make([]string, len(model.Columns))
	scanTargets := make([]string, len(model.Columns))
	for index, column := range model.Columns {
		columns[index] = quoteORMIdentifier(column.Name)
		scanTargets[index] = "&value." + goFieldName(column.Name)
	}
	statement := "SELECT " + strings.Join(columns, ", ") + " FROM " + quoteORMIdentifier(model.Table)
	g.line("func " + goORMLoader(model) + "(query " + queryType + ") []*" + goIdentifier(model.Name, true) + " {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(manifest.Adapter) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { panic(err) }")
	g.line("defer database.Close()")
	g.line("statement := " + strconv.Quote(statement))
	g.line("arguments := []any{}")
	g.line("if len(query.conditions) > 0 {")
	g.indent++
	g.line("clauses := make([]string, 0, len(query.conditions))")
	g.line("for _, condition := range query.conditions {")
	g.indent++
	g.line("clauses = append(clauses, \"\\\"\"+strings.ReplaceAll(condition.column, \"\\\"\", \"\\\"\\\"\")+\"\\\" = ?\")")
	g.line("arguments = append(arguments, condition.value)")
	g.indent--
	g.line("}")
	g.line("statement += \" WHERE \" + strings.Join(clauses, \" AND \")")
	g.indent--
	g.line("}")
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
}

func goORMQueryType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Query"
}

func goORMLoader(model ormintegration.Model) string {
	return "trbOrmLoad" + goIdentifier(model.Name, true)
}

func quoteORMIdentifier(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}
