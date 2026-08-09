package golang

import (
	pathpkg "path"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
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

func (g *generator) ormFind(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(call.Arguments) != 1 {
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
	primaryKey, exists := model.PrimaryKey()
	if !exists {
		return "nil"
	}
	qualifier := g.ormModelQualifier(model)
	query := qualifier + goORMWhere(model) + "([]string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{" + g.expr(call.Arguments[0].Value) + "})"
	return qualifier + goORMFirst(model) + "(" + query + ")"
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

func (g *generator) ormResultType(value types.Type) string {
	return g.goType(types.Type{Kind: types.Named, Name: "DbResult", Args: []types.Type{value}})
}

func (g *generator) ormResultOK(valueType types.Type, value string) string {
	return g.ormPackageAlias() + ".NewDbResultOk[" + g.goType(valueType) + "](" + value + ")"
}

func (g *generator) ormResultErr(valueType types.Type, value string) string {
	return g.ormPackageAlias() + ".NewDbResultErr[" + g.goType(valueType) + "](" + value + ")"
}

func (g *generator) ormErrorKind(name string) string {
	return g.ormPackageAlias() + "." + goConstantIdentifier("DbErrorKind", name)
}

func (g *generator) ormErrorValue(kind, message string) string {
	return g.goType(types.FromName("DbError")) + "{Kind: " + g.ormErrorKind(kind) + ", Message: " + strconv.Quote(message) + "}"
}

func (g *generator) ormPackageAlias() string {
	if alias := g.typeAliases["DbResult"]; alias != "" {
		return alias
	}
	return "orm"
}

func (g *generator) ormAssociationQuery(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	source, ok := g.orm.Model(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	associationName := strings.TrimSuffix(member.Name, "_query")
	association, ok := source.Association(associationName)
	if !ok {
		return "nil"
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return "nil"
	}
	qualifier := g.ormModelQualifier(target)
	value := g.expr(member.Receiver) + "." + goORMColumnGetter(association.SourceColumn) + "()"
	return qualifier + goORMWhere(target) + "([]string{" + strconv.Quote(association.TargetColumn) + "}, []string{\"=\"}, []any{" + value + "})"
}

func (g *generator) ormPreload(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok || len(arguments) < 2 {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMPreload(model) + "(" + query + ", " + arguments[1] + ")"
}

func (g *generator) ormLoadedAssociation(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	source, ok := g.orm.Model(member.Receiver.ExprType().Name)
	if !ok {
		return "nil"
	}
	if _, ok := source.Association(member.Name); !ok {
		return "nil"
	}
	resultType := g.goType(call.ExprType())
	return "func() " + resultType + " { value, loaded := " + g.expr(member.Receiver) + "." + goORMAssociationGetter(member.Name) + "(); if !loaded { panic(" + strconv.Quote("ORM association "+source.Name+"."+member.Name+" was not preloaded") + ") }; if value == nil { var zero " + resultType + "; return zero }; return value.(" + resultType + ") }()"
}

func (g *generator) ormBatchIterate(iteration *ir.Iterate) {
	model, ok := g.orm.QueryModel(iteration.Source.ExprType().Name)
	querySource := ""
	if ok {
		querySource = g.expr(iteration.Source)
	} else {
		model, ok = g.orm.Model(iteration.Source.ExprType().Name)
		if !ok {
			return
		}
		qualifier := g.ormModelQualifier(model)
		querySource = qualifier + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	batchKey, ok := model.BatchKey()
	if !ok {
		return
	}
	qualifier := g.ormModelQualifier(model)
	batchSize := "1000"
	if iteration.SliceSize != nil {
		batchSize = g.expr(iteration.SliceSize)
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	query := "__trbBatchQuery" + suffix
	size := "__trbBatchSize" + suffix
	after := "__trbBatchAfter" + suffix
	done := "__trbBatchDone" + suffix
	loaded := "__trbBatchLoaded" + suffix
	batch := "__trbBatch" + suffix
	last := "__trbBatchLast" + suffix
	processed := "__trbBatchProcessed" + suffix
	failed := "__trbBatchFailed" + suffix
	label := "__trbBatchLoop" + suffix
	keyType := batchKey.Type
	keyType.Nullable = false
	binding := ir.IterationBinding{Name: "_", Type: types.Type{Kind: types.Any, Name: "Any"}}
	if len(iteration.Bindings) > 0 {
		binding = iteration.Bindings[0]
	}

	resultTarget := ""
	returnResult := iteration.Result != nil && iteration.Result.Return
	breakTarget := ""
	if !returnResult || ormBatchBodyBreaks(iteration.Body) {
		breakTarget = label
	}
	if iteration.Result != nil && iteration.Result.Variable != nil {
		resultTarget = goBindingIdentifier(iteration.Result.Variable.Name)
		g.line("var " + resultTarget + " " + g.goType(iteration.Result.Type))
	} else if iteration.Result != nil && iteration.Result.Target != nil {
		resultTarget = g.assignmentTarget(iteration.Result.Target)
	}
	integerType := types.FromName("Integer")
	success := func(value string) string { return g.ormResultOK(integerType, value) }
	failure := func(value string) string { return g.ormResultErr(integerType, value) }
	assignResult := func(value string) {
		if returnResult {
			g.line("return " + value)
		} else if resultTarget != "" {
			g.line(resultTarget + " = " + value)
		}
	}

	g.line("{")
	g.indent++
	g.line(query + " := " + querySource)
	g.line(size + " := " + batchSize)
	g.line(processed + " := 0")
	if !returnResult {
		g.line(failed + " := false")
	}
	g.line("if " + size + " <= 0 {")
	g.indent++
	assignResult(failure(g.ormErrorValue("InvalidData", "batch size must be greater than zero")))
	if !returnResult {
		g.line(failed + " = true")
	}
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("var " + after + " *" + g.goType(keyType))
	g.line(done + " := false")
	if breakTarget != "" {
		g.line(label + ":")
	}
	g.line("for {")
	g.indent++
	g.line("if " + done + " { break }")
	g.line(loaded + " := " + qualifier + goORMBatchLoader(model) + "(" + query + ", " + after + ", " + size + ")")
	g.line("if " + loaded + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag {")
	g.indent++
	assignResult(failure(loaded + ".ErrError"))
	if !returnResult {
		g.line(failed + " = true")
		g.line("break " + label)
	}
	g.indent--
	g.line("}")
	g.line(batch + " := " + loaded + ".OkValue")
	g.line("if len(" + batch + ") == 0 { break }")
	g.line(done + " = len(" + batch + ") < " + size)
	g.line(last + " := " + batch + "[len(" + batch + ")-1]." + goORMColumnGetter(batchKey.Name) + "()")
	g.line(after + " = &" + last)
	if iteration.Operation == "find_each" {
		if binding.Name == "_" {
			g.line("for range " + batch + " {")
		} else {
			g.line("for _, " + goBindingIdentifier(binding.Name) + " := range " + batch + " {")
		}
		g.indent++
		g.line(processed + "++")
		if binding.Name != "_" {
			g.line("_ = " + goBindingIdentifier(binding.Name))
		}
		previousBreakTarget := g.breakTarget
		g.breakTarget = breakTarget
		g.statements(iteration.Body)
		g.breakTarget = previousBreakTarget
		g.indent--
		g.line("}")
	} else {
		g.line(processed + " += len(" + batch + ")")
		if binding.Name != "_" {
			g.line(goBindingIdentifier(binding.Name) + " := " + batch)
			g.line("_ = " + goBindingIdentifier(binding.Name))
		}
		previousBreakTarget := g.breakTarget
		g.breakTarget = breakTarget
		g.statements(iteration.Body)
		g.breakTarget = previousBreakTarget
	}
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	if returnResult {
		g.line("return " + success(processed))
	} else if resultTarget != "" {
		g.line("if !" + failed + " { " + resultTarget + " = " + success(processed) + " }")
	}
	g.indent--
	g.line("}")
}

func ormBatchBodyBreaks(statements []ir.Statement) bool {
	for _, statement := range statements {
		switch node := statement.(type) {
		case *ir.Break:
			return true
		case *ir.If:
			if ormBatchBodyBreaks(node.Then) || ormBatchBodyBreaks(node.Else) {
				return true
			}
			for _, branch := range node.ElseIf {
				if ormBatchBodyBreaks(branch.Body) {
					return true
				}
			}
		case *ir.Case:
			if ormBatchBodyBreaks(node.Leading) || ormBatchBodyBreaks(node.Else) {
				return true
			}
			for _, branch := range node.Branches {
				if ormBatchBodyBreaks(branch.Body) {
					return true
				}
			}
		}
	}
	return false
}

func (g *generator) ormRuntime(manifest *ormintegration.Manifest) {
	models := manifest.ModelsForModule(g.modulePath)
	if len(models) == 0 {
		return
	}
	adapter, err := ormintegration.AdapterFor(manifest.Adapter)
	if err != nil {
		return
	}
	g.requireImport("database/sql", "sql")
	g.requireImport("database/sql/driver", "driver")
	g.requireImport(adapter.GoDriverImport, "_")
	g.requireImport("context", "")
	g.requireImport("errors", "")
	g.requireImport("net", "")
	g.requireImport("reflect", "")
	g.requireImport("strings", "")
	if adapter.NumberedBinds {
		g.requireImport("strconv", "")
	}
	g.ormDialectRuntime(adapter)
	for _, model := range models {
		g.ormModelRuntime(manifest, adapter, model)
	}
}

func (g *generator) ormDialectRuntime(adapter ormintegration.Adapter) {
	errorKind := g.goType(types.FromName("DbErrorKind"))
	errorType := g.goType(types.FromName("DbError"))
	g.line("func trbOrmError(err error, fallback " + errorKind + ", message string) " + errorType + " {")
	g.indent++
	g.line("kind := fallback")
	g.line("if errors.Is(err, context.DeadlineExceeded) {")
	g.indent++
	g.line("kind = " + g.ormErrorKind("Timeout"))
	g.indent--
	g.line("} else if errors.Is(err, driver.ErrBadConn) {")
	g.indent++
	g.line("kind = " + g.ormErrorKind("Connection"))
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("var networkError net.Error")
	g.line("if errors.As(err, &networkError) { if networkError.Timeout() { kind = " + g.ormErrorKind("Timeout") + " } else { kind = " + g.ormErrorKind("Connection") + " } }")
	g.indent--
	g.line("}")
	g.line("return " + errorType + "{Kind: kind, Message: message}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func trbOrmPlaceholder(position int) string {")
	g.indent++
	if adapter.NumberedBinds {
		g.line("return \"$\" + strconv.Itoa(position)")
	} else {
		g.line("return \"?\"")
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbOrmPlaceholders(count int) string {")
	g.indent++
	g.line("values := make([]string, count)")
	g.line("for index := range values { values[index] = trbOrmPlaceholder(index + 1) }")
	g.line("return strings.Join(values, \",\")")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbOrmQuoteIdentifier(name string) string {")
	g.indent++
	g.line("mark := " + strconv.Quote(adapter.IdentifierMark))
	g.line("return mark + strings.ReplaceAll(name, mark, mark+mark) + mark")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormModelRuntime(manifest *ormintegration.Manifest, adapter ormintegration.Adapter, model ormintegration.Model) {
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
	g.line("preloads []string")
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
	g.line("func " + goORMPreload(model) + "(query " + queryType + ", association string) " + queryType + " {")
	g.indent++
	g.line("result := query")
	g.line("result.preloads = append([]string(nil), query.preloads...)")
	g.line("for _, existing := range result.preloads { if existing == association { return result } }")
	g.line("result.preloads = append(result.preloads, association)")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	columns := make([]string, len(model.Columns))
	scanTargets := make([]string, len(model.Columns))
	for index, column := range model.Columns {
		columns[index] = adapter.QuoteIdentifier(column.Name)
		scanTargets[index] = "&value." + goFieldName(column.Name)
		g.line("func (self *" + goIdentifier(model.Name, true) + ") " + goORMColumnGetter(column.Name) + "() " + g.goType(column.Type) + " {")
		g.indent++
		g.line("return self." + goFieldName(column.Name))
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
	for _, association := range model.Associations {
		if !association.Preloadable {
			continue
		}
		g.line("func (self *" + goIdentifier(model.Name, true) + ") " + goORMAssociationGetter(association.Name) + "() (any, bool) {")
		g.indent++
		g.line("return self." + goORMAssociationValueField(association.Name) + ", self." + goORMAssociationLoadedField(association.Name))
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
	statement := " FROM " + adapter.QuoteIdentifier(model.Table)
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
	g.line("column := trbOrmQuoteIdentifier(condition.column)")
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
	g.line("clauses = append(clauses, column+\" \"+condition.operator+\" \"+trbOrmPlaceholder(len(arguments)+1))")
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
	g.line("column := trbOrmQuoteIdentifier(order.column)")
	g.line("orders = append(orders, column+\" \"+strings.ToUpper(order.direction))")
	g.indent--
	g.line("}")
	g.line("statement += \" ORDER BY \" + strings.Join(orders, \", \")")
	g.indent--
	g.line("}")
	g.line("if query.limit != nil { statement += \" LIMIT \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, *query.limit) } else if query.offset != nil { statement += " + strconv.Quote(adapter.OffsetNoLimit) + " }")
	g.line("if query.offset != nil { statement += \" OFFSET \" + trbOrmPlaceholder(len(arguments)+1); arguments = append(arguments, *query.offset) }")
	g.line("return statement, arguments")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + goORMToSQL(model) + "(query " + queryType + ") string {")
	g.indent++
	g.line("statement, _ := " + goORMStatement(model) + "(query, " + strconv.Quote(strings.Join(columns, ", ")) + ")")
	g.line("return statement")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	stringType := types.FromName("String")
	g.line("func " + goORMExplain(model) + "(query " + queryType + ") " + g.ormResultType(stringType) + " {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(adapter.DriverName) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { return " + g.ormResultErr(stringType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("defer database.Close()")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(strings.Join(columns, ", ")) + ")")
	explainPrefix := "EXPLAIN QUERY PLAN "
	if adapter.ExplainStyle == ormintegration.ExplainText {
		explainPrefix = "EXPLAIN "
	} else if adapter.ExplainStyle == ormintegration.ExplainJSON {
		explainPrefix = "EXPLAIN FORMAT=JSON "
	}
	g.line("rows, err := database.Query(" + strconv.Quote(explainPrefix) + "+statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(stringType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database explain failed\")") + " }")
	g.line("defer rows.Close()")
	g.line("details := []string{}")
	g.line("for rows.Next() {")
	g.indent++
	if adapter.ExplainStyle == ormintegration.ExplainSQLite {
		g.line("var id, parent, unused int")
		g.line("var detail string")
		g.line("if err := rows.Scan(&id, &parent, &unused, &detail); err != nil { return " + g.ormResultErr(stringType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database explain result was invalid\")") + " }")
		g.line("details = append(details, detail)")
	} else {
		g.line("var detail string")
		g.line("if err := rows.Scan(&detail); err != nil { return " + g.ormResultErr(stringType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database explain result was invalid\")") + " }")
		g.line("details = append(details, detail)")
	}
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(stringType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database explain failed\")") + " }")
	g.line("return " + g.ormResultOK(stringType, "strings.Join(details, \"\\n\")"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	for _, association := range model.Associations {
		if association.Preloadable {
			g.ormAssociationPreloader(manifest, adapter, model, association)
		}
	}

	modelType := types.FromName(model.Name)
	modelsType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{modelType}}
	g.line("func " + goORMLoader(model) + "(query " + queryType + ") " + g.ormResultType(modelsType) + " {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(adapter.DriverName) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("defer database.Close()")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(strings.Join(columns, ", ")) + ")")
	g.line("rows, err := database.Query(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("defer rows.Close()")
	g.line("result := []*" + goIdentifier(model.Name, true) + "{}")
	g.line("for rows.Next() {")
	g.indent++
	g.line("value := &" + goIdentifier(model.Name, true) + "{}")
	g.line("if err := rows.Scan(" + strings.Join(scanTargets, ", ") + "); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database row was invalid\")") + " }")
	g.line("result = append(result, value)")
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("if err := rows.Close(); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("for _, preload := range query.preloads {")
	g.indent++
	g.line("switch preload {")
	for _, association := range model.Associations {
		if association.Preloadable {
			g.line("case " + strconv.Quote(association.Name) + ":")
			g.indent++
			g.line("if preloadError := " + goORMAssociationPreloader(model, association) + "(database, result); preloadError != nil { return " + g.ormResultErr(modelsType, "*preloadError") + " }")
			g.indent--
		}
	}
	g.line("default:")
	g.indent++
	g.line("panic(\"unsupported ORM preload \" + preload)")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("return " + g.ormResultOK(modelsType, "result"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	firstType := modelType
	firstType.Nullable = true
	g.line("func " + goORMFirst(model) + "(query " + queryType + ") " + g.ormResultType(firstType) + " {")
	g.indent++
	g.line("if query.limit == nil || *query.limit > 1 { count := 1; query.limit = &count }")
	g.line("loaded := " + goORMLoader(model) + "(query)")
	g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(firstType, "loaded.ErrError") + " }")
	g.line("if len(loaded.OkValue) == 0 { return " + g.ormResultOK(firstType, "nil") + " }")
	g.line("return " + g.ormResultOK(firstType, "loaded.OkValue[0]"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	if batchKey, ok := model.BatchKey(); ok {
		keyType := batchKey.Type
		keyType.Nullable = false
		g.line("func " + goORMBatchLoader(model) + "(query " + queryType + ", after *" + g.goType(keyType) + ", size int) " + g.ormResultType(modelsType) + " {")
		g.indent++
		g.line("if size <= 0 { return " + g.ormResultErr(modelsType, g.ormErrorValue("InvalidData", "batch size must be greater than zero")) + " }")
		g.line("if len(query.orders) > 0 || query.limit != nil || query.offset != nil { return " + g.ormResultErr(modelsType, g.ormErrorValue("InvalidData", "batch queries do not accept order, limit, or offset")) + " }")
		g.line("if after != nil {")
		g.indent++
		g.line("query = " + goORMQueryWhere(model) + "(query, []string{" + strconv.Quote(batchKey.Name) + "}, []string{\">\"}, []any{*after})")
		g.indent--
		g.line("}")
		g.line("query.orders = []" + orderType + "{{column: " + strconv.Quote(batchKey.Name) + ", direction: \"asc\"}}")
		g.line("query.limit = &size")
		g.line("return " + goORMLoader(model) + "(query)")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}

	integerType := types.FromName("Integer")
	g.line("func " + goORMCount(model) + "(query " + queryType + ") " + g.ormResultType(integerType) + " {")
	g.indent++
	g.line("database, err := sql.Open(" + strconv.Quote(adapter.DriverName) + ", " + strconv.Quote(manifest.Database) + ")")
	g.line("if err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Connection")+", \"database connection failed\")") + " }")
	g.line("defer database.Close()")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, \"1\")")
	g.line("row := database.QueryRow(\"SELECT COUNT(*) FROM (\"+statement+\") AS trb_count\", arguments...)")
	g.line("var count int")
	g.line("if err := row.Scan(&count); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database count failed\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "count"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormAssociationPreloader(manifest *ormintegration.Manifest, adapter ormintegration.Adapter, model ormintegration.Model, association ormintegration.Association) {
	target, ok := manifest.Model(association.TargetModel)
	if !ok {
		return
	}
	sourceColumn, sourceOK := model.Column(association.SourceColumn)
	targetColumn, targetOK := target.Column(association.TargetColumn)
	if !sourceOK || !targetOK {
		return
	}
	keyColumn := targetColumn
	if association.Kind == ormintegration.HasMany {
		keyColumn = sourceColumn
	}
	keyType := keyColumn.Type
	keyType.Nullable = false
	targetColumns := make([]string, len(target.Columns))
	scanTargets := make([]string, len(target.Columns))
	for index, column := range target.Columns {
		targetColumns[index] = adapter.QuoteIdentifier(column.Name)
		scanTargets[index] = "&relatedValue." + goFieldName(column.Name)
	}
	function := goORMAssociationPreloader(model, association)
	sourceType := "*" + goIdentifier(model.Name, true)
	targetType := "*" + goIdentifier(target.Name, true)
	valueField := goORMAssociationValueField(association.Name)
	loadedField := goORMAssociationLoadedField(association.Name)

	g.line("func " + function + "(database *sql.DB, values []" + sourceType + ") *" + g.goType(types.FromName("DbError")) + " {")
	g.indent++
	g.line("arguments := []any{}")
	g.line("for _, value := range values {")
	g.indent++
	if sourceColumn.Nullable {
		g.line("if value." + goFieldName(sourceColumn.Name) + " != nil { arguments = append(arguments, *value." + goFieldName(sourceColumn.Name) + ") }")
	} else {
		g.line("arguments = append(arguments, value." + goFieldName(sourceColumn.Name) + ")")
	}
	g.indent--
	g.line("}")
	g.line("if len(arguments) == 0 {")
	g.indent++
	g.line("for _, value := range values {")
	g.indent++
	if association.Kind == ormintegration.HasMany {
		g.line("value." + valueField + " = []" + targetType + "{}")
	}
	g.line("value." + loadedField + " = true")
	g.indent--
	g.line("}")
	g.line("return nil")
	g.indent--
	g.line("}")
	statement := "SELECT " + strings.Join(targetColumns, ", ") + " FROM " + adapter.QuoteIdentifier(target.Table) + " WHERE " + adapter.QuoteIdentifier(association.TargetColumn) + " IN ("
	g.line("statement := " + strconv.Quote(statement) + " + trbOrmPlaceholders(len(arguments)) + \")\"")
	g.line("rows, err := database.Query(statement, arguments...)")
	g.line("if err != nil { databaseError := trbOrmError(err, " + g.ormErrorKind("Query") + ", \"database preload failed\"); return &databaseError }")
	g.line("defer rows.Close()")
	if association.Kind == ormintegration.BelongsTo {
		g.line("related := map[" + g.goType(keyType) + "]" + targetType + "{}")
	} else {
		g.line("related := map[" + g.goType(keyType) + "][]" + targetType + "{}")
	}
	g.line("for rows.Next() {")
	g.indent++
	g.line("relatedValue := &" + goIdentifier(target.Name, true) + "{}")
	g.line("if err := rows.Scan(" + strings.Join(scanTargets, ", ") + "); err != nil { databaseError := trbOrmError(err, " + g.ormErrorKind("InvalidData") + ", \"database preload row was invalid\"); return &databaseError }")
	if association.Kind == ormintegration.BelongsTo {
		g.line("related[relatedValue." + goFieldName(targetColumn.Name) + "] = relatedValue")
	} else if targetColumn.Nullable {
		g.line("if relatedValue." + goFieldName(targetColumn.Name) + " != nil { key := *relatedValue." + goFieldName(targetColumn.Name) + "; related[key] = append(related[key], relatedValue) }")
	} else {
		g.line("key := relatedValue." + goFieldName(targetColumn.Name))
		g.line("related[key] = append(related[key], relatedValue)")
	}
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { databaseError := trbOrmError(err, " + g.ormErrorKind("Query") + ", \"database preload failed\"); return &databaseError }")
	g.line("for _, value := range values {")
	g.indent++
	if association.Kind == ormintegration.BelongsTo {
		if sourceColumn.Nullable {
			g.line("if value." + goFieldName(sourceColumn.Name) + " != nil { value." + valueField + " = related[*value." + goFieldName(sourceColumn.Name) + "] }")
		} else {
			g.line("value." + valueField + " = related[value." + goFieldName(sourceColumn.Name) + "]")
		}
	} else {
		g.line("items := related[value." + goFieldName(sourceColumn.Name) + "]")
		g.line("if items == nil { items = []" + targetType + "{} }")
		g.line("value." + valueField + " = items")
	}
	g.line("value." + loadedField + " = true")
	g.indent--
	g.line("}")
	g.line("return nil")
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

func goORMPreload(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Preload"
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

func goORMToSQL(model ormintegration.Model) string {
	return "TrbOrmToSQL" + goIdentifier(model.Name, true)
}

func goORMExplain(model ormintegration.Model) string {
	return "TrbOrmExplain" + goIdentifier(model.Name, true)
}

func goORMBatchLoader(model ormintegration.Model) string {
	return "TrbOrmBatch" + goIdentifier(model.Name, true)
}

func goORMStatement(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Statement"
}

func goORMAssociationGetter(name string) string {
	return "TrbOrmAssociation" + goIdentifier(name, true)
}

func goORMAssociationValueField(name string) string {
	return goFieldName("@__trb_association_" + name)
}

func goORMAssociationLoadedField(name string) string {
	return goFieldName("@__trb_association_" + name + "_loaded")
}

func goORMAssociationPreloader(model ormintegration.Model, association ormintegration.Association) string {
	return "trbOrmPreload" + goIdentifier(model.Name, true) + goIdentifier(association.Name, true)
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
