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
	queryType := goORMQueryType(model)
	g.line("type " + draftType + " struct {")
	g.indent++
	g.line("query " + queryType)
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
	g.line("func " + goORMBuildScoped(model) + "(query " + queryType + ", columns []string, values []any) *" + draftType + " {")
	g.indent++
	g.line("draft := " + goORMBuild(model) + "(columns, values)")
	g.line("draft.query = query")
	g.line("return draft")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMDraftSave(model) + "(draft *" + draftType + ") " + resultType + " {")
	g.indent++
	g.line("return " + goORMCreateScoped(model) + "(draft.query, draft.columns, draft.values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMCreate(model) + "(columns []string, values []any) " + resultType + " {")
	g.indent++
	g.line("return " + goORMCreateScoped(model) + "(" + queryType + "{}, columns, values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMCreateScoped(model) + "(query " + queryType + ", columns []string, values []any) " + resultType + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForTransaction(query.transaction)")
	g.line("if databaseError != nil { return " + g.ormResultErr(modelType, "*databaseError") + " }")
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
		g.line("value." + goORMQueryScopeField() + " = query")
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
		g.line("query = " + goORMQueryWhere(model) + "(query, []string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{primaryKeyValue})")
		g.line("return " + goORMFirst(model) + "(query)")
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.ormInsertAllRuntime(adapter, model)
	g.ormConflictRuntime(model)
	g.ormUpsertAllRuntime(adapter, model)
	g.ormUpdateRuntime(adapter, model, primaryKey, projection, scanTargets)
	g.ormDeleteRuntime(adapter, model, primaryKey)
	g.ormDestroyRuntime(model)
}

func goORMBuildScoped(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "BuildScoped"
}

func goORMCreateScoped(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "CreateScoped"
}

func (g *generator) ormConflictRuntime(model ormintegration.Model) {
	modelType := types.FromName(model.Name)
	booleanType := types.FromName("Boolean")
	draftType := goIdentifier(model.DraftType(), true)
	g.line("func " + goORMDraftColumnValues(model) + "(draft *" + draftType + ", columns []string) ([]any, bool) {")
	g.indent++
	g.line("indexed := make(map[string]any, len(draft.columns))")
	g.line("for index, column := range draft.columns { indexed[column] = draft.values[index] }")
	g.line("values := make([]any, len(columns))")
	g.line("for index, column := range columns { value, ok := indexed[column]; if !ok { return nil, false }; values[index] = value }")
	g.line("return values, true")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMValuesContainNil(model) + "(values []any) bool {")
	g.indent++
	g.line("for _, value := range values { if value == nil { return true }; reflected := reflect.ValueOf(value); if reflected.Kind() == reflect.Ptr && reflected.IsNil() { return true } }")
	g.line("return false")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	var uniqueChecks []string
	for _, constraint := range model.UniqueConstraints {
		parts := []string{"len(columns) == " + strconv.Itoa(len(constraint.Columns))}
		for index, column := range constraint.Columns {
			parts = append(parts, "columns["+strconv.Itoa(index)+"] == "+strconv.Quote(column))
		}
		uniqueChecks = append(uniqueChecks, "("+strings.Join(parts, " && ")+")")
	}
	if len(uniqueChecks) == 0 {
		uniqueChecks = append(uniqueChecks, "false")
	}
	g.line("func " + goORMUniqueColumns(model) + "(columns []string) bool { return " + strings.Join(uniqueChecks, " || ") + " }")
	g.b.WriteByte('\n')
	var writableColumns []string
	for _, column := range model.Columns {
		if !column.PrimaryKey && !column.Generated {
			writableColumns = append(writableColumns, strconv.Quote(column.Name))
		}
	}
	g.line("func " + goORMWritableColumn(model) + "(column string) bool {")
	g.indent++
	if len(writableColumns) > 0 {
		g.line("switch column { case " + strings.Join(writableColumns, ", ") + ": return true; default: return false }")
	} else {
		g.line("return false")
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMInsertIfAbsent(model) + "(draft *" + draftType + ", uniqueColumns []string) " + g.ormResultType(booleanType) + " {")
	g.indent++
	g.line("if !" + goORMUniqueColumns(model) + "(uniqueColumns) { return " + g.ormResultErr(booleanType, g.ormErrorValue("InvalidData", "unique_by must match a primary or unique constraint")) + " }")
	g.line("uniqueValues, ok := " + goORMDraftColumnValues(model) + "(draft, uniqueColumns)")
	g.line("if !ok { return " + g.ormResultErr(booleanType, g.ormErrorValue("InvalidData", "unique_by columns must be present in the draft")) + " }")
	g.line("created := " + goORMCreate(model) + "(draft.columns, draft.values)")
	g.line("if created.Kind == " + g.ormPackageAlias() + ".DbResultOkTag { return " + g.ormResultOK(booleanType, "true") + " }")
	g.line("if " + goORMValuesContainNil(model) + "(uniqueValues) { return " + g.ormResultErr(booleanType, "created.ErrError") + " }")
	g.line("if created.ErrError.Kind != " + g.ormErrorKind("Constraint") + " { return " + g.ormResultErr(booleanType, "created.ErrError") + " }")
	g.line("operators := make([]string, len(uniqueColumns)); for index := range operators { operators[index] = \"=\" }")
	g.line("loaded := " + goORMFirst(model) + "(" + goORMWhere(model) + "(uniqueColumns, operators, uniqueValues))")
	g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, "loaded.ErrError") + " }")
	g.line("if loaded.OkValue != nil { return " + g.ormResultOK(booleanType, "false") + " }")
	g.line("return " + g.ormResultErr(booleanType, "created.ErrError"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMUpsert(model) + "(draft *" + draftType + ", uniqueColumns []string, updateColumns []string) " + g.ormResultType(modelType) + " {")
	g.indent++
	g.line("if !" + goORMUniqueColumns(model) + "(uniqueColumns) { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "unique_by must match a primary or unique constraint")) + " }")
	g.line("uniqueValues, ok := " + goORMDraftColumnValues(model) + "(draft, uniqueColumns)")
	g.line("if !ok { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "unique_by columns must be present in the draft")) + " }")
	g.line("seenUpdates := map[string]bool{}")
	g.line("for _, column := range updateColumns {")
	g.indent++
	g.line("if !" + goORMWritableColumn(model) + "(column) || seenUpdates[column] { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "upsert update columns must be unique writable attributes")) + " }")
	g.line("for _, uniqueColumn := range uniqueColumns { if column == uniqueColumn { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "upsert cannot update unique_by columns")) + " } }")
	g.line("seenUpdates[column] = true")
	g.indent--
	g.line("}")
	g.line("updateValues, ok := " + goORMDraftColumnValues(model) + "(draft, updateColumns)")
	g.line("if !ok { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "upsert update columns must be present in the draft")) + " }")
	g.line("created := " + goORMCreate(model) + "(draft.columns, draft.values)")
	g.line("if created.Kind == " + g.ormPackageAlias() + ".DbResultOkTag || created.ErrError.Kind != " + g.ormErrorKind("Constraint") + " { return created }")
	g.line("if " + goORMValuesContainNil(model) + "(uniqueValues) { return created }")
	g.line("operators := make([]string, len(uniqueColumns)); for index := range operators { operators[index] = \"=\" }")
	g.line("loaded := " + goORMFirst(model) + "(" + goORMWhere(model) + "(uniqueColumns, operators, uniqueValues))")
	g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(modelType, "loaded.ErrError") + " }")
	g.line("if loaded.OkValue == nil { return created }")
	g.line("return " + goORMUpdate(model) + "(loaded.OkValue, updateColumns, updateValues)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormUpsertAllRuntime(adapter ormintegration.Adapter, model ormintegration.Model) {
	integerType := types.FromName("Integer")
	draftType := goIdentifier(model.DraftType(), true)
	resultType := g.ormResultType(integerType)
	g.line("func " + goORMUpsertAll(model) + "(drafts []*" + draftType + ", uniqueColumns []string, updateColumns []string) " + resultType + " {")
	g.indent++
	g.line("if len(drafts) == 0 { return " + g.ormResultOK(integerType, "0") + " }")
	g.line("if !" + goORMUniqueColumns(model) + "(uniqueColumns) { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "unique_by must match a primary or unique constraint")) + " }")
	g.line("if len(updateColumns) == 0 { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "upsert update requires at least one attribute")) + " }")
	g.line("seenUpdates := map[string]bool{}")
	g.line("for _, column := range updateColumns {")
	g.indent++
	g.line("if !" + goORMWritableColumn(model) + "(column) || seenUpdates[column] { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "upsert update columns must be unique writable attributes")) + " }")
	g.line("for _, uniqueColumn := range uniqueColumns { if column == uniqueColumn { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "upsert cannot update unique_by columns")) + " } }")
	g.line("seenUpdates[column] = true")
	g.indent--
	g.line("}")
	g.line("columns := append([]string(nil), drafts[0].columns...)")
	g.line("rows := make([][]any, len(drafts))")
	g.line("uniqueRows := make([][]any, len(drafts))")
	g.line("for rowIndex, draft := range drafts {")
	g.indent++
	g.line("if len(draft.columns) != len(columns) { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "bulk upsert drafts must use the same attributes")) + " }")
	g.line("indexed := make(map[string]any, len(draft.columns))")
	g.line("for index, column := range draft.columns { indexed[column] = draft.values[index] }")
	g.line("row := make([]any, len(columns))")
	g.line("for index, column := range columns { value, ok := indexed[column]; if !ok { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "bulk upsert drafts must use the same attributes")) + " }; row[index] = value }")
	g.line("rows[rowIndex] = row")
	g.line("uniqueValues, ok := " + goORMDraftColumnValues(model) + "(draft, uniqueColumns)")
	g.line("if !ok { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "unique_by columns must be present in every draft")) + " }")
	g.line("if _, ok := " + goORMDraftColumnValues(model) + "(draft, updateColumns); !ok { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "upsert update columns must be present in every draft")) + " }")
	g.line("if !" + goORMValuesContainNil(model) + "(uniqueValues) { for previous := 0; previous < rowIndex; previous++ { if reflect.DeepEqual(uniqueRows[previous], uniqueValues) { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "bulk upsert contains duplicate unique_by values")) + " } } }")
	g.line("uniqueRows[rowIndex] = uniqueValues")
	g.indent--
	g.line("}")
	if adapter.Name == "mysql" {
		g.ormMySQLUpsertAllBody(adapter, model, integerType)
	} else {
		g.ormNativeUpsertAllBody(adapter, model, integerType)
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormNativeUpsertAllBody(adapter ormintegration.Adapter, model ormintegration.Model, integerType types.Type) {
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("quotedColumns := make([]string, len(columns)); for index, column := range columns { quotedColumns[index] = trbOrmQuoteIdentifier(column) }")
	g.line("groups := make([]string, len(rows))")
	g.line("arguments := make([]any, 0, len(rows)*len(columns))")
	g.line("for rowIndex, row := range rows { placeholders := make([]string, len(columns)); for index := range columns { placeholders[index] = trbOrmPlaceholder(len(arguments) + 1); arguments = append(arguments, row[index]) }; groups[rowIndex] = \"(\" + strings.Join(placeholders, \", \") + \")\" }")
	g.line("conflictColumns := make([]string, len(uniqueColumns)); for index, column := range uniqueColumns { conflictColumns[index] = trbOrmQuoteIdentifier(column) }")
	g.line("assignments := make([]string, len(updateColumns)); for index, column := range updateColumns { quoted := trbOrmQuoteIdentifier(column); assignments[index] = quoted + \" = excluded.\" + quoted }")
	g.line("statement := " + strconv.Quote("INSERT INTO "+adapter.QuoteIdentifier(model.Table)+" (") + " + strings.Join(quotedColumns, \", \") + \") VALUES \" + strings.Join(groups, \", \") + \" ON CONFLICT (\" + strings.Join(conflictColumns, \", \") + \") DO UPDATE SET \" + strings.Join(assignments, \", \")")
	g.line("written, err := database.Exec(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk upsert failed\")") + " }")
	g.line("affected, err := written.RowsAffected(); if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk upsert result was unavailable\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "int(affected)"))
}

func (g *generator) ormMySQLUpsertAllBody(adapter ormintegration.Adapter, model ormintegration.Model, integerType types.Type) {
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("transaction, err := database.Begin(); if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk upsert failed\")") + " }")
	g.line("defer transaction.Rollback()")
	g.line("quotedColumns := make([]string, len(columns)); for index, column := range columns { quotedColumns[index] = trbOrmQuoteIdentifier(column) }")
	g.line("insertStatement := " + strconv.Quote("INSERT INTO "+adapter.QuoteIdentifier(model.Table)+" (") + " + strings.Join(quotedColumns, \", \") + \") VALUES (\" + trbOrmPlaceholders(len(columns)) + \")\"")
	g.line("predicates := make([]string, len(uniqueColumns)); for index, column := range uniqueColumns { predicates[index] = trbOrmQuoteIdentifier(column) + \" = ?\" }")
	g.line("selectStatement := " + strconv.Quote("SELECT 1 FROM "+adapter.QuoteIdentifier(model.Table)+" WHERE ") + " + strings.Join(predicates, \" AND \") + \" LIMIT 1 FOR UPDATE\"")
	g.line("assignments := make([]string, len(updateColumns)); for index, column := range updateColumns { assignments[index] = trbOrmQuoteIdentifier(column) + \" = ?\" }")
	g.line("updateStatement := " + strconv.Quote("UPDATE "+adapter.QuoteIdentifier(model.Table)+" SET ") + " + strings.Join(assignments, \", \") + \" WHERE \" + strings.Join(predicates, \" AND \")")
	g.line("for rowIndex, draft := range drafts {")
	g.indent++
	g.line("uniqueValues := uniqueRows[rowIndex]")
	g.line("updateValues, _ := " + goORMDraftColumnValues(model) + "(draft, updateColumns)")
	g.line("exists := false")
	g.line("if !" + goORMValuesContainNil(model) + "(uniqueValues) { var marker int; err := transaction.QueryRow(selectStatement, uniqueValues...).Scan(&marker); if err == nil { exists = true } else if !errors.Is(err, sql.ErrNoRows) { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk upsert failed\")") + " } }")
	g.line("if exists { arguments := append(append([]any(nil), updateValues...), uniqueValues...); if _, err := transaction.Exec(updateStatement, arguments...); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk upsert failed\")") + " }; continue }")
	g.line("if _, err := transaction.Exec(insertStatement, rows[rowIndex]...); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk upsert failed\")") + " }")
	g.indent--
	g.line("}")
	g.line("if err := transaction.Commit(); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk upsert failed\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "len(drafts)"))
}

func (g *generator) ormInsertAllRuntime(adapter ormintegration.Adapter, model ormintegration.Model) {
	integerType := types.FromName("Integer")
	draftType := goIdentifier(model.DraftType(), true)
	resultType := g.ormResultType(integerType)
	g.line("func " + goORMInsertAll(model) + "(drafts []*" + draftType + ") " + resultType + " {")
	g.indent++
	g.line("if len(drafts) == 0 { return " + g.ormResultOK(integerType, "0") + " }")
	g.line("columns := append([]string(nil), drafts[0].columns...)")
	g.line("rows := make([][]any, len(drafts))")
	g.line("for rowIndex, draft := range drafts {")
	g.indent++
	g.line("if len(draft.columns) != len(columns) { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "bulk insert drafts must use the same attributes")) + " }")
	g.line("indexed := make(map[string]any, len(draft.columns))")
	g.line("for index, column := range draft.columns { indexed[column] = draft.values[index] }")
	g.line("row := make([]any, len(columns))")
	g.line("for index, column := range columns { value, ok := indexed[column]; if !ok { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "bulk insert drafts must use the same attributes")) + " }; row[index] = value }")
	g.line("rows[rowIndex] = row")
	g.indent--
	g.line("}")
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("if len(columns) == 0 {")
	g.indent++
	g.line("transaction, err := database.Begin()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk insert failed\")") + " }")
	g.line("for range drafts { if _, err := transaction.Exec(" + strconv.Quote("INSERT INTO "+adapter.QuoteIdentifier(model.Table)+adapter.DefaultInsert) + "); err != nil { _ = transaction.Rollback(); return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk insert failed\")") + " } }")
	g.line("if err := transaction.Commit(); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk insert failed\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "len(drafts)"))
	g.indent--
	g.line("}")
	g.line("quotedColumns := make([]string, len(columns))")
	g.line("for index, column := range columns { quotedColumns[index] = trbOrmQuoteIdentifier(column) }")
	g.line("groups := make([]string, len(rows))")
	g.line("arguments := make([]any, 0, len(rows)*len(columns))")
	g.line("for rowIndex, row := range rows {")
	g.indent++
	g.line("placeholders := make([]string, len(columns))")
	g.line("for index := range columns { placeholders[index] = trbOrmPlaceholder(len(arguments) + 1); arguments = append(arguments, row[index]) }")
	g.line("groups[rowIndex] = \"(\" + strings.Join(placeholders, \", \") + \")\"")
	g.indent--
	g.line("}")
	g.line("statement := " + strconv.Quote("INSERT INTO "+adapter.QuoteIdentifier(model.Table)+" (") + " + strings.Join(quotedColumns, \", \") + \") VALUES \" + strings.Join(groups, \", \")")
	g.line("written, err := database.Exec(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk insert failed\")") + " }")
	g.line("affected, err := written.RowsAffected()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk insert result was unavailable\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "int(affected)"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormUpdateRuntime(adapter ormintegration.Adapter, model ormintegration.Model, primaryKey ormintegration.Column, projection, scanTargets []string) {
	modelType := types.FromName(model.Name)
	resultType := g.ormResultType(modelType)
	modelName := goIdentifier(model.Name, true)
	changesType := goIdentifier(model.ChangesType(), true)
	g.line("type " + changesType + " struct {")
	g.indent++
	g.line("value *" + modelName)
	g.line("columns []string")
	g.line("values []any")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMWith(model) + "(value *" + modelName + ", columns []string, values []any) *" + changesType + " {")
	g.indent++
	g.line("changeValues := append([]any(nil), values...)")
	g.line("for index, value := range changeValues { if bytes, ok := value.([]byte); ok { changeValues[index] = append([]byte(nil), bytes...) } }")
	g.line("return &" + changesType + "{value: value, columns: append([]string(nil), columns...), values: changeValues}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMChangesSave(model) + "(changes *" + changesType + ") " + resultType + " {")
	g.indent++
	g.line("return " + goORMUpdate(model) + "(changes.value, changes.columns, changes.values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMUpdate(model) + "(value *" + modelName + ", columns []string, values []any) " + resultType + " {")
	g.indent++
	g.line("if len(columns) == 0 { return " + g.ormResultErr(modelType, g.ormErrorValue("InvalidData", "database update requires at least one value")) + " }")
	g.line("database, databaseError := trbOrmExecutorForTransaction(value." + goORMQueryScopeField() + ".transaction)")
	g.line("if databaseError != nil { return " + g.ormResultErr(modelType, "*databaseError") + " }")
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
		g.line("updated." + goORMQueryScopeField() + " = value." + goORMQueryScopeField())
		g.line("return " + g.ormResultOK(modelType, "updated"))
	} else {
		g.line("if _, err := database.Exec(statement, arguments...); err != nil { return " + g.ormResultErr(modelType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database update failed\")") + " }")
		g.line("query := " + goORMQueryWhere(model) + "(value." + goORMQueryScopeField() + ", []string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{value." + goORMColumnGetter(primaryKey.Name) + "()})")
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
	g.line("database, databaseError := trbOrmExecutorForTransaction(value." + goORMQueryScopeField() + ".transaction)")
	g.line("if databaseError != nil { return " + g.ormResultErr(booleanType, "*databaseError") + " }")
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

func (g *generator) ormDestroyRuntime(model ormintegration.Model) {
	booleanType := types.FromName("Boolean")
	integerType := types.FromName("Integer")
	modelName := goIdentifier(model.Name, true)
	queryType := goORMQueryType(model)
	transactionType := "*" + g.ormLifecycleAlias() + ".TrbOrmTransaction"
	core := goORMDestroy(model) + "InTransaction"
	g.line("func " + goORMDestroy(model) + "(value *" + modelName + ") " + g.ormResultType(booleanType) + " {")
	g.indent++
	g.line("if transaction := value." + goORMQueryScopeField() + ".transaction; transaction != nil { return " + core + "(value, transaction) }")
	g.line("transaction, databaseError := " + g.ormLifecycleAlias() + ".TrbOrmBeginTransaction()")
	g.line("if databaseError != nil { return " + g.ormResultErr(booleanType, "*databaseError") + " }")
	g.line("value." + goORMQueryScopeField() + ".transaction = transaction")
	g.line("result := " + core + "(value, transaction)")
	g.line("if result.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { _ = transaction.Rollback(); return result }")
	g.line("if commitError := transaction.Commit(); commitError != nil { return " + g.ormResultErr(booleanType, "*commitError") + " }")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + core + "(value *" + modelName + ", transaction " + transactionType + ") " + g.ormResultType(booleanType) + " {")
	g.indent++
	g.line("value." + goORMQueryScopeField() + ".transaction = transaction")
	for _, association := range model.Associations {
		if association.Dependent == "" {
			continue
		}
		target, ok := g.orm.Model(association.TargetModel)
		if !ok {
			continue
		}
		qualifier := g.ormModelQualifier(target)
		query := qualifier + goORMUsing(target) + "(transaction)"
		query = qualifier + goORMQueryWhere(target) + "(" + query + ", []string{" + strconv.Quote(association.TargetColumn) + "}, []string{\"=\"}, []any{value." + goORMColumnGetter(association.SourceColumn) + "()})"
		query = g.ormAssociationScope(association, target, query)
		variable := "dependent" + goIdentifier(association.Name, true)
		switch association.Dependent {
		case ormintegration.DependentRestrict:
			g.line(variable + " := " + qualifier + goORMExists(target) + "(" + query + ")")
			g.line("if " + variable + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, variable+".ErrError") + " }")
			g.line("if " + variable + ".OkValue { return " + g.ormResultErr(booleanType, g.ormErrorValue("Constraint", "dependent association "+model.Name+"."+association.Name+" restricts destroy")) + " }")
		case ormintegration.DependentDelete:
			g.line(variable + " := " + qualifier + goORMDeleteAll(target) + "(" + query + ")")
			g.line("if " + variable + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, variable+".ErrError") + " }")
		case ormintegration.DependentNullify:
			g.line(variable + " := " + qualifier + goORMUpdateAll(target) + "(" + query + ", []string{" + strconv.Quote(association.TargetColumn) + "}, []any{nil})")
			g.line("if " + variable + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, variable+".ErrError") + " }")
		case ormintegration.DependentDestroy:
			g.line(variable + " := " + qualifier + goORMLoader(target) + "(" + query + ")")
			g.line("if " + variable + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, variable+".ErrError") + " }")
			g.line("for _, related := range " + variable + ".OkValue {")
			g.indent++
			g.line("destroyed := " + qualifier + goORMDestroy(target) + "(related)")
			g.line("if destroyed.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(booleanType, "destroyed.ErrError") + " }")
			g.indent--
			g.line("}")
		}
	}
	g.line("return " + goORMDelete(model) + "(value)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + goORMDestroyAll(model) + "(query " + queryType + ") " + g.ormResultType(integerType) + " {")
	g.indent++
	g.line("owned := query.transaction == nil")
	g.line("transaction := query.transaction")
	g.line("if owned { var databaseError *" + g.goType(types.FromName("DbError")) + "; transaction, databaseError = " + g.ormLifecycleAlias() + ".TrbOrmBeginTransaction(); if databaseError != nil { return " + g.ormResultErr(integerType, "*databaseError") + " } }")
	g.line("query.transaction = transaction")
	g.line("loaded := " + goORMLoader(model) + "(query)")
	g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { if owned { _ = transaction.Rollback() }; return " + g.ormResultErr(integerType, "loaded.ErrError") + " }")
	g.line("count := 0")
	g.line("for _, value := range loaded.OkValue { destroyed := " + core + "(value, transaction); if destroyed.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { if owned { _ = transaction.Rollback() }; return " + g.ormResultErr(integerType, "destroyed.ErrError") + " }; if destroyed.OkValue { count++ } }")
	g.line("if owned { if commitError := transaction.Commit(); commitError != nil { return " + g.ormResultErr(integerType, "*commitError") + " } }")
	g.line("return " + g.ormResultOK(integerType, "count"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormRelationWriteRuntime(adapter ormintegration.Adapter, model ormintegration.Model) {
	integerType := types.FromName("Integer")
	queryType := goORMQueryType(model)
	invalidModifiers := "len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || query.distinct || len(query.preloads) > 0 || len(query.joins) > 0"
	g.line("func " + goORMUpdateAll(model) + "(query " + queryType + ", columns []string, values []any) " + g.ormResultType(integerType) + " {")
	g.indent++
	g.line("if len(columns) == 0 { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "database bulk update requires at least one value")) + " }")
	g.line("if " + invalidModifiers + " { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "database bulk update does not accept distinct, joins, order, limit, offset, lock, or preload")) + " }")
	g.line("database, databaseError := trbOrmExecutorForTransaction(query.transaction)")
	g.line("if databaseError != nil { return " + g.ormResultErr(integerType, "*databaseError") + " }")
	g.line("assignments := make([]string, len(columns))")
	g.line("for index, column := range columns { assignments[index] = trbOrmQuoteIdentifier(column) + \" = \" + trbOrmPlaceholder(index + 1) }")
	g.line("arguments := append([]any(nil), values...)")
	g.line("statement := " + strconv.Quote("UPDATE "+adapter.QuoteIdentifier(model.Table)+" SET ") + " + strings.Join(assignments, \", \")")
	g.line("if query.predicate != nil { statement += \" WHERE \" + " + goORMPredicateSQL(model) + "(query.predicate, &arguments) }")
	g.line("updated, err := database.Exec(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk update failed\")") + " }")
	g.line("affected, err := updated.RowsAffected()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk update result was unavailable\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "int(affected)"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMDeleteAll(model) + "(query " + queryType + ") " + g.ormResultType(integerType) + " {")
	g.indent++
	g.line("if " + invalidModifiers + " { return " + g.ormResultErr(integerType, g.ormErrorValue("InvalidData", "database bulk delete does not accept distinct, joins, order, limit, offset, lock, or preload")) + " }")
	g.line("database, databaseError := trbOrmExecutorForTransaction(query.transaction)")
	g.line("if databaseError != nil { return " + g.ormResultErr(integerType, "*databaseError") + " }")
	g.line("arguments := []any{}")
	g.line("statement := " + strconv.Quote("DELETE FROM "+adapter.QuoteIdentifier(model.Table)))
	g.line("if query.predicate != nil { statement += \" WHERE \" + " + goORMPredicateSQL(model) + "(query.predicate, &arguments) }")
	g.line("deleted, err := database.Exec(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Constraint")+", \"database bulk delete failed\")") + " }")
	g.line("affected, err := deleted.RowsAffected()")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database bulk delete result was unavailable\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "int(affected)"))
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
