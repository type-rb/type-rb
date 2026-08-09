package golang

import (
	"strconv"
	"strings"

	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormCreateRuntime(adapter ormintegration.Adapter, model ormintegration.Model, projection, scanTargets []string) {
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return
	}
	modelType := types.FromName(model.Name)
	resultType := g.ormResultType(modelType)
	g.line("func " + goORMCreate(model) + "(columns []string, values []any) " + resultType + " {")
	g.indent++
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("statement := " + strconv.Quote("INSERT INTO "+adapter.QuoteIdentifier(model.Table)))
	g.line("if len(columns) == 0 {")
	g.indent++
	g.line("statement += " + strconv.Quote(adapter.DefaultInsert))
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("quotedColumns := make([]string, len(columns))")
	g.line("for index, column := range columns { quotedColumns[index] = trbOrmQuoteIdentifier(column) }")
	g.line("statement += \" (\" + strings.Join(quotedColumns, \", \") + \") VALUES (\" + trbOrmPlaceholders(len(values)) + \")\"")
	g.indent--
	g.line("}")
	if adapter.InsertReturning {
		g.line("row := database.QueryRow(statement+" + strconv.Quote(" RETURNING "+strings.Join(projection, ", ")) + ", values...)")
		g.line("value := &" + goIdentifier(model.Name, true) + "{}")
		g.line("if err := row.Scan(" + strings.Join(scanTargets, ", ") + "); err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database insert failed\")") + " }")
		g.line("return " + g.ormResultOK(modelType, "value"))
	} else {
		if primaryKey.Generated {
			g.line("written, err := database.Exec(statement, values...)")
		} else {
			g.line("_, err = database.Exec(statement, values...)")
		}
		g.line("if err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database insert failed\")") + " }")
		g.line("var primaryKeyValue any")
		g.line("for index, column := range columns { if column == " + strconv.Quote(primaryKey.Name) + " { primaryKeyValue = values[index]; break } }")
		if primaryKey.Generated {
			g.line("if primaryKeyValue == nil { generated, err := written.LastInsertId(); if err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database generated key was unavailable\")") + " }; primaryKeyValue = generated }")
		}
		g.line("if primaryKeyValue == nil { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "database insert did not produce a primary key")) + " }")
		g.line("query := " + goORMWhere(model) + "([]string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{primaryKeyValue})")
		g.line("return " + goORMFirst(model) + "(query)")
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}
