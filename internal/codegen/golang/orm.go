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
	predicates := ormintegration.Predicates(call)
	columns := make([]string, len(predicates))
	operators := make([]string, len(predicates))
	values := make([]string, len(predicates))
	for index, predicate := range predicates {
		columns[index] = strconv.Quote(predicate.Column)
		operators[index] = strconv.Quote(string(predicate.Operator))
		values[index] = g.expr(predicate.Value)
	}
	return g.ormModelQualifier(model) + goORMWhere(model) + "([]string{" + strings.Join(columns, ", ") + "}, []string{" + strings.Join(operators, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
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
	return g.ormModelQualifier(model) + goORMLoader(model) + "(" + arguments[0] + ")"
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
	queryType := goORMQueryType(model)
	g.line("type " + conditionType + " struct {")
	g.indent++
	g.line("column string")
	g.line("operator string")
	g.line("value any")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("type " + queryType + " struct {")
	g.indent++
	g.line("conditions []" + conditionType)
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMWhere(model) + "(columns []string, operators []string, values []any) " + queryType + " {")
	g.indent++
	g.line("query := " + queryType + "{conditions: make([]" + conditionType + ", 0, len(columns))}")
	g.line("for index, column := range columns {")
	g.indent++
	g.line("query.conditions = append(query.conditions, " + conditionType + "{column: column, operator: operators[index], value: values[index]})")
	g.indent--
	g.line("}")
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
	return "TrbOrm" + goIdentifier(model.Name, true) + "Query"
}

func goORMConditionType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Condition"
}

func goORMWhere(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Where"
}

func goORMLoader(model ormintegration.Model) string {
	return "TrbOrmLoad" + goIdentifier(model.Name, true)
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
