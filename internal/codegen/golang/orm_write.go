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
	draftType := goIdentifier(model.DraftType(), true)
	g.line("type " + draftType + " struct {")
	g.indent++
	g.line("columns []string")
	g.line("values []any")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMBuild(model) + "(columns []string, values []any) *" + draftType + " {")
	g.indent++
	g.line("draftValues := append([]any(nil), values...)")
	g.line("for index, value := range draftValues { if bytes, ok := value.([]byte); ok { draftValues[index] = append([]byte(nil), bytes...) } }")
	g.line("return &" + draftType + "{columns: append([]string(nil), columns...), values: draftValues}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMDraftSave(model) + "(draft *" + draftType + ") " + resultType + " {")
	g.indent++
	g.line("return " + goORMCreate(model) + "(draft.columns, draft.values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
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
	g.ormUpdateRuntime(adapter, model, primaryKey, projection, scanTargets)
	g.ormDeleteRuntime(adapter, model, primaryKey)
}

func (g *generator) ormUpdateRuntime(adapter ormintegration.Adapter, model ormintegration.Model, primaryKey ormintegration.Column, projection, scanTargets []string) {
	modelType := types.FromName(model.Name)
	resultType := g.ormResultType(modelType)
	modelName := goIdentifier(model.Name, true)
	g.line("func " + goORMUpdate(model) + "(value *" + modelName + ", columns []string, values []any) " + resultType + " {")
	g.indent++
	g.line("if len(columns) == 0 { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "database update requires at least one value")) + " }")
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("assignments := make([]string, len(columns))")
	g.line("for index, column := range columns { assignments[index] = trbOrmQuoteIdentifier(column) + \" = \" + trbOrmPlaceholder(index + 1) }")
	g.line("arguments := append([]any(nil), values...)")
	g.line("arguments = append(arguments, value." + goORMColumnGetter(primaryKey.Name) + "())")
	statement := "UPDATE " + adapter.QuoteIdentifier(model.Table) + " SET "
	g.line("statement := " + strconv.Quote(statement) + " + strings.Join(assignments, \", \") + " + strconv.Quote(" WHERE "+adapter.QuoteIdentifier(primaryKey.Name)+" = ") + " + trbOrmPlaceholder(len(arguments))")
	if adapter.InsertReturning {
		g.line("row := database.QueryRow(statement+" + strconv.Quote(" RETURNING "+strings.Join(projection, ", ")) + ", arguments...)")
		g.line("updated := &" + modelName + "{}")
		g.line("if err := row.Scan(" + strings.Join(replaceScanReceiver(scanTargets, "value", "updated"), ", ") + "); err != nil {")
		g.indent++
		g.line("if errors.Is(err, sql.ErrNoRows) { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "database update target was not found")) + " }")
		g.line("return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database update failed\")"))
		g.indent--
		g.line("}")
		g.line("return " + g.ormResultOK(modelType, "updated"))
	} else {
		g.line("if _, err := database.Exec(statement, arguments...); err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database update failed\")") + " }")
		g.line("query := " + goORMWhere(model) + "([]string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{value." + goORMColumnGetter(primaryKey.Name) + "()})")
		g.line("loaded := " + goORMFirst(model) + "(query)")
		g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(modelType, "loaded.ErrError") + " }")
		g.line("if loaded.OkValue == nil { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "database update target was not found")) + " }")
		g.line("return " + g.ormResultOK(modelType, "loaded.OkValue"))
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormDeleteRuntime(adapter ormintegration.Adapter, model ormintegration.Model, primaryKey ormintegration.Column) {
	booleanType := types.FromName("Boolean")
	modelName := goIdentifier(model.Name, true)
	g.line("func " + goORMDelete(model) + "(value *" + modelName + ") " + g.ormResultType(booleanType) + " {")
	g.indent++
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(booleanType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	statement := "DELETE FROM " + adapter.QuoteIdentifier(model.Table) + " WHERE " + adapter.QuoteIdentifier(primaryKey.Name) + " = "
	g.line("deleted, err := database.Exec(" + strconv.Quote(statement) + "+trbOrmPlaceholder(1), value." + goORMColumnGetter(primaryKey.Name) + "())")
	g.line("if err != nil { return " + g.ormResultErr(booleanType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database delete failed\")") + " }")
	g.line("affected, err := deleted.RowsAffected()")
	g.line("if err != nil { return " + g.ormResultErr(booleanType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database delete result was unavailable\")") + " }")
	g.line("return " + g.ormResultOK(booleanType, "affected > 0"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func replaceScanReceiver(targets []string, from, to string) []string {
	result := make([]string, len(targets))
	for index, target := range targets {
		result[index] = strings.Replace(target, "."+from+".", "."+to+".", 1)
		result[index] = strings.Replace(result[index], "&"+from+".", "&"+to+".", 1)
	}
	return result
}
