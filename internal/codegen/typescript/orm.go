package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) ormRuntime(manifest *ormintegration.Manifest) {
	if g.modulePath == "trb/orm/index" {
		g.ormCoreRuntime(manifest)
		return
	}
	models := manifest.ModelsForModule(g.modulePath)
	if len(models) > 0 {
		g.line("type Subquery<T> = __trbOrm.TrbOrmSubquery<T>;")
	}
	for _, model := range models {
		g.ormModelRuntime(model)
	}
}

func (g *generator) ormCoreRuntime(manifest *ormintegration.Manifest) {
	database := strconv.Quote(manifest.Database)
	environment := strconv.Quote(manifest.DatabaseEnvironment)
	adapter := strconv.Quote(manifest.Adapter)
	g.line("const __trbOrmAdapter: TrbOrmAdapter = " + adapter + ";")
	g.line("const __trbOrmConfiguredDatabase: string = " + database + ";")
	g.line("const __trbOrmDatabaseEnvironment: string = " + environment + ";")
	for _, line := range strings.Split(strings.TrimSpace(typescriptORMRuntime), "\n") {
		g.line(line)
	}
}

func (g *generator) ormModelRuntime(model ormintegration.Model) {
	g.line("type " + model.QueryType + " = __trbOrm.TrbOrmQuery;")
	g.line("type " + model.ScopeType() + " = __trbOrm.TrbOrmQuery;")
	g.line("type " + model.DraftType() + " = __trbOrm.TrbOrmDraft;")
	g.line("type " + model.ChangesType() + " = __trbOrm.TrbOrmChanges;")
	for _, column := range model.Columns {
		g.line("type " + model.GroupType(column) + " = __trbOrm.TrbOrmGroupedQuery;")
	}
	g.line("__trbOrm.registerModel(" + g.ormModelDescriptor(model) + ", " + model.Name + ");")
}

func (g *generator) ormModelDescriptor(model ormintegration.Model) string {
	columns := make([]string, 0, len(model.Columns))
	for _, column := range model.Columns {
		columnType := column.Type
		enumValues := "[]"
		parser := "undefined"
		if ormintegration.IsPortableTimeType(column.Type) {
			parser = g.runtimeName(column.Type.Name) + ".try_parse"
		}
		if column.Enum != nil {
			columnType = column.Enum.StorageType
			values := make([]string, 0, len(column.Enum.Values))
			owner := g.runtimeName(column.Enum.Name)
			for _, value := range column.Enum.Values {
				storage := strconv.Quote(value.StringValue)
				if column.Enum.StorageType.Kind == types.Int {
					storage = strconv.FormatInt(value.IntegerValue, 10)
				}
				values = append(values, "["+owner+"."+value.Name+", "+storage+"]")
			}
			enumValues = "[" + strings.Join(values, ", ") + "]"
		}
		columns = append(columns, "{ name: "+strconv.Quote(column.Name)+", kind: "+strconv.Quote(ormColumnKind(columnType))+", nullable: "+strconv.FormatBool(column.Type.Nullable)+", primary: "+strconv.FormatBool(column.PrimaryKey)+", enumValues: "+enumValues+", parse: "+parser+" }")
	}
	unique := make([]string, 0, len(model.UniqueConstraints))
	for _, constraint := range model.UniqueConstraints {
		values := make([]string, len(constraint.Columns))
		for index, column := range constraint.Columns {
			values[index] = strconv.Quote(column)
		}
		unique = append(unique, "["+strings.Join(values, ", ")+"]")
	}
	associations := make([]string, 0, len(model.Associations))
	for _, association := range model.Associations {
		scope := "undefined"
		if association.Scope != nil && len(association.Scope.Parameters) == 1 && len(association.Scope.Body) == 1 {
			if result, ok := association.Scope.Body[0].(*ir.ExpressionStatement); ok {
				scope = "(" + association.Scope.Parameters[0] + ": __trbOrm.TrbOrmQuery): __trbOrm.TrbOrmQuery => " + g.expr(result.Expression)
			}
		}
		associations = append(associations, "{ name: "+strconv.Quote(association.Name)+", kind: "+strconv.Quote(string(association.Kind))+", target: "+strconv.Quote(association.TargetModel)+", sourceColumn: "+strconv.Quote(association.SourceColumn)+", targetColumn: "+strconv.Quote(association.TargetColumn)+", inverse: "+strconv.Quote(association.Inverse)+", through: "+strconv.Quote(association.Through)+", source: "+strconv.Quote(association.Source)+", dependent: "+strconv.Quote(string(association.Dependent))+", scope: "+scope+" }")
	}
	return "{ name: " + strconv.Quote(model.Name) + ", table: " + strconv.Quote(model.Table) + ", columns: [" + strings.Join(columns, ", ") + "], unique: [" + strings.Join(unique, ", ") + "], associations: [" + strings.Join(associations, ", ") + "] }"
}

func ormColumnKind(value types.Type) string {
	if ormintegration.IsPortableTimeType(value) {
		return strings.ToLower(value.Name)
	}
	switch value.Kind {
	case types.Bool:
		return "boolean"
	case types.Int:
		return "integer"
	case types.Float:
		return "float"
	case types.String:
		return "string"
	case types.Bytes:
		return "bytes"
	default:
		return "any"
	}
}

func (g *generator) ormModelForCall(call *ir.Call) (ormintegration.Model, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok || g.orm == nil {
		return ormintegration.Model{}, false
	}
	name := member.Receiver.ExprType().Name
	if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
		if model, exists := g.orm.Model(identifier.Name); exists {
			return model, true
		}
	}
	if model, exists := g.orm.Model(name); exists {
		return model, true
	}
	if model, exists := g.orm.QueryModel(name); exists {
		return model, true
	}
	if model, exists := g.orm.ScopeModel(name); exists {
		return model, true
	}
	if model, _, exists := g.orm.GroupModel(name); exists {
		return model, true
	}
	if model, exists := g.orm.DraftModel(name); exists {
		return model, true
	}
	if model, exists := g.orm.ChangesModel(name); exists {
		return model, true
	}
	return ormintegration.Model{}, false
}

func (g *generator) ormBaseQuery(call *ir.Call) (ormintegration.Model, string, bool) {
	model, ok := g.ormModelForCall(call)
	if !ok {
		return ormintegration.Model{}, "undefined", false
	}
	member := call.Callee.(*ir.Member)
	if member.Receiver.ExprType().Name == model.QueryType || member.Receiver.ExprType().Name == model.ScopeType() {
		return model, g.expr(member.Receiver), true
	}
	return model, "__trbOrm.query(" + g.ormModelName(call, model) + ")", true
}

func (g *generator) ormModelName(call *ir.Call, model ormintegration.Model) string {
	name := strconv.Quote(model.Name)
	member, ok := call.Callee.(*ir.Member)
	if !ok || member.Receiver.ExprType().Name != model.Name {
		return name
	}
	return "__trbOrm.modelName(" + name + ", " + g.expr(member.Receiver) + ")"
}

func (g *generator) ormPredicate(call *ir.Call) string {
	columns := make([]string, len(call.Arguments))
	operators := make([]string, len(call.Arguments))
	values := make([]string, len(call.Arguments))
	if len(call.Arguments) == 3 && call.Arguments[0].Name == "" {
		return "[" + g.expr(call.Arguments[0].Value) + "], [" + g.expr(call.Arguments[1].Value) + "], [" + g.ormPredicateValue(call.Arguments[2].Value) + "]"
	}
	for index, argument := range call.Arguments {
		columns[index] = strconv.Quote(argument.Name)
		operators[index] = strconv.Quote("=")
		values[index] = g.ormPredicateValue(argument.Value)
	}
	return "[" + strings.Join(columns, ", ") + "], [" + strings.Join(operators, ", ") + "], [" + strings.Join(values, ", ") + "]"
}

func (g *generator) ormPredicateValue(value ir.Expression) string {
	if value == nil {
		return "null"
	}
	if value.ExprType().Kind == types.Range {
		if rangeValue, ok := value.(*ir.Range); ok {
			return "__trbOrm.range(" + g.expr(rangeValue.Start) + ", " + g.expr(rangeValue.End) + ", " + strconv.FormatBool(rangeValue.Exclusive) + ")"
		}
		bounds := g.expr(value)
		return "((bounds: [number, number, boolean]) => __trbOrm.range(bounds[0], bounds[1], bounds[2]))(" + bounds + ")"
	}
	return g.expr(value)
}

func (g *generator) ormWriteValues(call *ir.Call) (string, string) {
	columns := make([]string, len(call.Arguments))
	values := make([]string, len(call.Arguments))
	for index, argument := range call.Arguments {
		columns[index] = strconv.Quote(argument.Name)
		values[index] = g.expr(argument.Value)
	}
	return "[" + strings.Join(columns, ", ") + "]", "[" + strings.Join(values, ", ") + "]"
}

func (g *generator) ormIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.orm.") {
		return "", false
	}
	member, _ := call.Callee.(*ir.Member)
	receiver := "undefined"
	if member != nil {
		receiver = g.expr(member.Receiver)
	}
	model, hasModel := g.ormModelForCall(call)
	modelName := ""
	if hasModel {
		modelName = g.ormModelName(call, model)
	}
	baseModel, base, _ := g.ormBaseQuery(call)
	_ = baseModel
	switch name {
	case "trb.orm.column":
		return "__trbOrm.column(" + receiver + ", " + strconv.Quote(member.Name) + ")", true
	case "trb.orm.where", "trb.orm.query.where":
		return "__trbOrm.where(" + base + ", " + g.ormPredicate(call) + ")", true
	case "trb.orm.not", "trb.orm.query.not":
		return "__trbOrm.not(" + base + ", " + g.ormPredicate(call) + ")", true
	case "trb.orm.query.or":
		return "__trbOrm.or(" + receiver + ", " + arguments[len(arguments)-1] + ")", true
	case "trb.orm.distinct", "trb.orm.query.distinct":
		return "__trbOrm.distinct(" + base + ")", true
	case "trb.orm.order", "trb.orm.query.order":
		columns, directions := g.ormWriteValues(call)
		return "__trbOrm.order(" + base + ", " + columns + ", " + directions + ")", true
	case "trb.orm.limit", "trb.orm.query.limit":
		return "__trbOrm.limit(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.offset", "trb.orm.query.offset":
		return "__trbOrm.offset(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.lock", "trb.orm.query.lock":
		return "__trbOrm.lock(" + base + ")", true
	case "trb.orm.using":
		return "__trbOrm.using(" + modelName + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.select", "trb.orm.query.select":
		return "__trbOrm.select(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.group", "trb.orm.query.group":
		return "__trbOrm.group(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.group.having":
		expression := g.expr(call.Arguments[0].Value)
		operatorIndex := 1
		if len(call.Arguments) == 4 {
			expression = g.expr(call.Arguments[0].Value) + " + '(' + " + g.expr(call.Arguments[1].Value) + " + ')'"
			operatorIndex = 2
		}
		return "__trbOrm.having(" + receiver + ", " + expression + ", " + g.expr(call.Arguments[operatorIndex].Value) + ", " + g.expr(call.Arguments[operatorIndex+1].Value) + ")", true
	case "trb.orm.join", "trb.orm.query.join", "trb.orm.left_join", "trb.orm.query.left_join":
		kind := `"INNER JOIN"`
		if strings.Contains(name, "left_join") {
			kind = `"LEFT JOIN"`
		}
		target := "undefined"
		if len(call.Arguments) > 1 {
			target = g.expr(call.Arguments[1].Value)
		}
		return "__trbOrm.join(" + base + ", " + g.expr(call.Arguments[0].Value) + ", " + target + ", " + kind + ")", true
	case "trb.orm.where_exists", "trb.orm.query.where_exists", "trb.orm.where_not_exists", "trb.orm.query.where_not_exists":
		target := "undefined"
		if len(call.Arguments) > 1 {
			target = g.expr(call.Arguments[1].Value)
		}
		return "__trbOrm.whereExists(" + base + ", " + g.expr(call.Arguments[0].Value) + ", " + target + ", " + strconv.FormatBool(strings.Contains(name, "not_exists")) + ")", true
	case "trb.orm.preload", "trb.orm.query.preload":
		target := "undefined"
		if len(call.Arguments) > 1 {
			target = g.expr(call.Arguments[1].Value)
		}
		return "__trbOrm.preload(" + base + ", " + g.expr(call.Arguments[0].Value) + ", " + target + ")", true
	case "trb.orm.all", "trb.orm.query.all":
		return "__trbOrm.all(" + base + ")", true
	case "trb.orm.first", "trb.orm.query.first":
		return "__trbOrm.first(" + base + ")", true
	case "trb.orm.count", "trb.orm.query.count":
		return "__trbOrm.count(" + base + ")", true
	case "trb.orm.to_sql", "trb.orm.query.to_sql":
		return "__trbOrm.toSQL(" + base + ")", true
	case "trb.orm.explain", "trb.orm.query.explain":
		return "__trbOrm.explain(" + base + ")", true
	case "trb.orm.find_by", "trb.orm.query.find_by":
		return "__trbOrm.first(__trbOrm.where(" + base + ", " + g.ormPredicate(call) + "))", true
	case "trb.orm.exists", "trb.orm.query.exists":
		filtered := base
		if len(call.Arguments) > 0 {
			filtered = "__trbOrm.where(" + base + ", " + g.ormPredicate(call) + ")"
		}
		return "__trbOrm.exists(" + filtered + ")", true
	case "trb.orm.pluck", "trb.orm.query.pluck":
		return "__trbOrm.pluck(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.pick", "trb.orm.query.pick":
		return "__trbOrm.pick(" + base + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.ids", "trb.orm.query.ids":
		primary, _ := model.PrimaryKey()
		return "__trbOrm.pluck(" + base + ", " + strconv.Quote(primary.Name) + ")", true
	case "trb.orm.sum", "trb.orm.query.sum", "trb.orm.average", "trb.orm.query.average", "trb.orm.minimum", "trb.orm.query.minimum", "trb.orm.maximum", "trb.orm.query.maximum":
		operation := strings.TrimPrefix(strings.TrimPrefix(name, "trb.orm.query."), "trb.orm.")
		return "__trbOrm.aggregate(" + base + ", " + strconv.Quote(operation) + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.group.count":
		return "__trbOrm.groupedAggregate(" + receiver + ", \"count\")", true
	case "trb.orm.group.sum", "trb.orm.group.average", "trb.orm.group.minimum", "trb.orm.group.maximum":
		operation := strings.TrimPrefix(name, "trb.orm.group.")
		return "__trbOrm.groupedAggregate(" + receiver + ", " + strconv.Quote(operation) + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.find", "trb.orm.scope.find":
		primary, _ := model.PrimaryKey()
		return "__trbOrm.first(__trbOrm.where(" + base + ", [" + strconv.Quote(primary.Name) + "], [\"=\"], [" + g.expr(call.Arguments[0].Value) + "]))", true
	case "trb.orm.build", "trb.orm.scope.build":
		columns, values := g.ormWriteValues(call)
		return "__trbOrm.build(" + modelName + ", " + base + ", " + columns + ", " + values + ")", true
	case "trb.orm.create", "trb.orm.scope.create":
		columns, values := g.ormWriteValues(call)
		return "__trbOrm.create(" + modelName + ", " + base + ", " + columns + ", " + values + ")", true
	case "trb.orm.draft.save":
		return "__trbOrm.saveDraft(" + receiver + ")", true
	case "trb.orm.with":
		columns, values := g.ormWriteValues(call)
		return "__trbOrm.changes(" + receiver + ", " + modelName + ", " + columns + ", " + values + ")", true
	case "trb.orm.update":
		columns, values := g.ormWriteValues(call)
		return "__trbOrm.update(" + modelName + ", " + receiver + ", " + columns + ", " + values + ")", true
	case "trb.orm.changes.save":
		return "__trbOrm.saveChanges(" + receiver + ")", true
	case "trb.orm.insert_all":
		return "__trbOrm.insertAll(" + modelName + ", " + g.expr(call.Arguments[0].Value) + ")", true
	case "trb.orm.insert_if_absent":
		return "__trbOrm.insertIfAbsent(" + g.expr(call.Arguments[0].Value) + ", " + g.expr(call.Arguments[1].Value) + ")", true
	case "trb.orm.draft.upsert":
		return "__trbOrm.upsert(" + receiver + ", " + g.expr(call.Arguments[0].Value) + ", " + g.expr(call.Arguments[1].Value) + ")", true
	case "trb.orm.upsert_all":
		return "__trbOrm.upsertAll(" + modelName + ", " + g.expr(call.Arguments[0].Value) + ", " + g.expr(call.Arguments[1].Value) + ", " + g.expr(call.Arguments[2].Value) + ")", true
	case "trb.orm.update_all", "trb.orm.query.update_all":
		columns, values := g.ormWriteValues(call)
		return "__trbOrm.updateAll(" + base + ", " + columns + ", " + values + ")", true
	case "trb.orm.delete_all", "trb.orm.query.delete_all":
		return "__trbOrm.deleteAll(" + base + ")", true
	case "trb.orm.delete":
		return "__trbOrm.deleteValue(" + modelName + ", " + receiver + ")", true
	case "trb.orm.destroy":
		return "__trbOrm.destroy(" + modelName + ", " + receiver + ")", true
	case "trb.orm.destroy_all", "trb.orm.query.destroy_all":
		return "__trbOrm.destroyAll(" + base + ")", true
	case "trb.orm.association.query.belongs_to", "trb.orm.association.query.has_many", "trb.orm.association.query.has_one":
		associationName := strings.TrimSuffix(member.Name, "_query")
		return "__trbOrm.associationRelation(" + receiver + ", " + modelName + ", " + strconv.Quote(associationName) + ")", true
	case "trb.orm.association.value.belongs_to", "trb.orm.association.value.has_many", "trb.orm.association.value.has_one", "trb.orm.association.load.belongs_to", "trb.orm.association.load.has_many", "trb.orm.association.load.has_one":
		return "__trbOrm.loadAssociation(" + receiver + ", " + strconv.Quote(member.Name) + ", false)", true
	case "trb.orm.association.reload.belongs_to", "trb.orm.association.reload.has_many", "trb.orm.association.reload.has_one":
		associationName := strings.TrimSuffix(member.Name, ".reload")
		return "__trbOrm.loadAssociation(" + receiver + ", " + strconv.Quote(associationName) + ", true)", true
	case "trb.orm.association.loaded.belongs_to", "trb.orm.association.loaded.has_many", "trb.orm.association.loaded.has_one":
		associationName := strings.TrimSuffix(member.Name, ".loaded?")
		return "__trbOrm.associationLoaded(" + receiver + ", " + strconv.Quote(associationName) + ")", true
	}
	return "", false
}

func (g *generator) structuredBlock(block *ir.StructuredBlock) {
	if block.Intrinsic != "trb.orm.transaction" || block.Result == nil {
		return
	}
	parent := "null"
	if member, ok := block.Call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Name == "Transaction" {
		parent = g.expr(member.Receiver)
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	tx := "__trbTransaction" + suffix
	raw := "__trbTransactionResult" + suffix
	success := block.EffectSuccess
	if success.Kind == "" || success.Kind == types.Void {
		success = types.FromName("Unit")
	}
	g.line("const " + raw + ": " + g.runtimeName("DbResult") + "<" + g.tsType(success) + "> = await __trbOrm.withScope(__trbScope, async () => __trbOrm.transaction(" + parent + ", async (" + tx + ") => {")
	g.indent++
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		g.line("const " + block.Bindings[0].Name + " = " + tx + ";")
		if namedUnusedBinding(block.Bindings[0].Name) {
			g.line("void " + block.Bindings[0].Name + ";")
		}
	}
	g.statements(block.Body)
	value := "({} satisfies Unit)"
	if block.Value != nil {
		value = g.expr(block.Value)
	}
	g.line("return { kind: \"Ok\", value: " + value + " };")
	g.indent--
	g.line("}));")
	if block.Result.Return {
		g.line("return " + raw + ";")
		return
	}
	if block.CaptureEffect {
		if block.Result.Variable != nil {
			keyword := "const"
			if block.Result.Variable.Mutable {
				keyword = "let"
			}
			g.line(keyword + " " + block.Result.Variable.Name + ": " + g.tsType(block.Result.Type) + " = " + raw + ";")
		} else if block.Result.Target != nil {
			g.line(g.assignmentTarget(block.Result.Target) + " = " + raw + ";")
		}
		return
	}
	g.line("if (" + raw + ".kind === \"Err\") return " + g.runtimeName("Result") + ".Err<" + g.tsType(block.PropagateSuccess) + ", " + g.tsType(block.Fails) + ">(" + raw + ".error);")
	if block.Result.Variable != nil {
		keyword := "const"
		if block.Result.Variable.Mutable {
			keyword = "let"
		}
		g.line(keyword + " " + block.Result.Variable.Name + ": " + g.tsType(block.Result.Type) + " = " + raw + ".value;")
	} else if block.Result.Target != nil {
		g.line(g.assignmentTarget(block.Result.Target) + " = " + raw + ".value;")
	}
}

func (g *generator) ormBatchIterate(iteration *ir.Iterate) {
	model, ok := g.orm.QueryModel(iteration.Source.ExprType().Name)
	source := g.expr(iteration.Source)
	if !ok {
		model, ok = g.orm.ScopeModel(iteration.Source.ExprType().Name)
	}
	if !ok {
		model, ok = g.orm.Model(iteration.Source.ExprType().Name)
		if !ok {
			return
		}
		source = "__trbOrm.query(__trbOrm.modelName(" + strconv.Quote(model.Name) + ", " + g.expr(iteration.Source) + "))"
	}
	primary, ok := model.BatchKey()
	if !ok {
		return
	}
	size := "1000"
	if iteration.SliceSize != nil {
		size = g.expr(iteration.SliceSize)
	}
	g.temporary++
	suffix := strconv.Itoa(g.temporary)
	label := "__trbBatchLoop" + suffix
	queryName := "__trbBatchQuery" + suffix
	sizeName := "__trbBatchSize" + suffix
	after := "__trbBatchAfter" + suffix
	loaded := "__trbBatchLoaded" + suffix
	batch := "__trbBatch" + suffix
	processed := "__trbBatchProcessed" + suffix
	if iteration.Result == nil {
		return
	}
	success := iteration.EffectSuccess
	if success.Kind == "" {
		success = types.FromName("Integer")
	}
	resultType := g.runtimeName("DbResult") + "<" + g.tsType(success) + ">"
	raw := "__trbBatchResult" + suffix
	g.line("const " + raw + ": " + resultType + " = await (async (): Promise<" + resultType + "> => {")
	g.indent++
	g.line("const " + queryName + " = " + source + ";")
	g.line("const " + sizeName + " = " + size + ";")
	g.line("if (" + sizeName + " <= 0) return { kind: \"Err\", error: { kind: DbErrorKind.InvalidData, message: \"batch size must be greater than zero\" } };")
	g.line("let " + after + ": unknown = null;")
	g.line("let " + processed + " = 0;")
	g.line(label + ": while (true) {")
	g.indent++
	g.line("const " + loaded + " = await __trbOrm.batch(" + queryName + ", " + after + ", " + sizeName + ");")
	g.line("if (" + loaded + ".kind === \"Err\") return " + loaded + ";")
	g.line("const " + batch + " = " + loaded + ".value;")
	g.line("if (" + batch + ".length === 0) break;")
	g.line(after + " = __trbOrm.column(" + batch + "[" + batch + ".length - 1]!, " + strconv.Quote(primary.Name) + ");")
	binding := "_"
	if len(iteration.Bindings) > 0 {
		binding = iteration.Bindings[0].Name
	}
	previousBreak := g.breakTarget
	g.breakTarget = label
	if iteration.Operation == "find_each" {
		loopBinding := binding
		if loopBinding == "_" {
			loopBinding = "__trbBatchItem" + suffix
		}
		g.line("for (const " + loopBinding + " of " + batch + ") {")
		g.indent++
		g.line(processed + " += 1;")
		if namedUnusedBinding(binding) {
			g.line("void " + binding + ";")
		}
		g.statements(iteration.Body)
		g.indent--
		g.line("}")
	} else {
		g.line(processed + " += " + batch + ".length;")
		if binding != "_" {
			g.line("const " + binding + " = " + batch + ";")
			if namedUnusedBinding(binding) {
				g.line("void " + binding + ";")
			}
		}
		g.statements(iteration.Body)
	}
	g.breakTarget = previousBreak
	g.line("if (" + batch + ".length < " + sizeName + ") break;")
	g.indent--
	g.line("}")
	g.line("return { kind: \"Ok\", value: " + processed + " };")
	g.indent--
	g.line("})();")
	if iteration.CaptureEffect {
		g.ormAssignIterationResult(raw, iteration)
		return
	}
	if iteration.Result.Return {
		g.line("return " + raw + ";")
		return
	}
	g.line("if (" + raw + ".kind === \"Err\") return { kind: \"Err\", error: " + raw + ".error };")
	g.ormAssignIterationResult(raw+".value", iteration)
}

func (g *generator) ormAssignIterationResult(value string, iteration *ir.Iterate) {
	if iteration.Result == nil {
		return
	}
	switch {
	case iteration.Result.Return:
		g.line("return " + value + ";")
	case iteration.Result.Variable != nil:
		keyword := "const"
		if iteration.Result.Variable.Mutable {
			keyword = "let"
		}
		g.line(keyword + " " + iteration.Result.Variable.Name + ": " + g.tsType(iteration.Result.Type) + " = " + value + ";")
	case iteration.Result.Target != nil:
		g.line(g.assignmentTarget(iteration.Result.Target) + " = " + value + ";")
	}
}

const typescriptORMRuntime = `
type TrbOrmAdapter = "sqlite" | "postgresql" | "mysql";
type TrbOrmColumn = { name: string; kind: string; nullable: boolean; primary: boolean; enumValues: Array<[unknown, unknown]>; parse?: (value: string) => { kind: "Ok"; value: unknown } | { kind: "Err" } };
type TrbOrmAssociation = { name: string; kind: string; target: string; sourceColumn: string; targetColumn: string; inverse: string; through: string; source: string; dependent: string; scope?: (query: TrbOrmQuery) => TrbOrmQuery };
type TrbOrmModel = { name: string; table: string; columns: TrbOrmColumn[]; unique: string[][]; associations: TrbOrmAssociation[]; factory?: new () => any };
type TrbOrmPredicate = { kind: "conditions"; columns: string[]; operators: string[]; values: unknown[] } | { kind: "and" | "or"; left: TrbOrmPredicate; right: TrbOrmPredicate };
type TrbOrmJoin = { kind: "INNER JOIN" | "LEFT JOIN"; association: string; query?: TrbOrmQuery };
type TrbOrmPreload = { association: string; query?: TrbOrmQuery };
export type TrbOrmQuery = { model: string; transaction: Transaction | null; predicate: TrbOrmPredicate | null; joins: TrbOrmJoin[]; orders: Array<[string, string]>; limit: number | null; offset: number | null; lock: boolean; distinct: boolean; preloads: TrbOrmPreload[] };
export type TrbOrmDraft = { model: string; query: TrbOrmQuery; columns: string[]; values: unknown[] };
export type TrbOrmChanges = { model: string; value: any; columns: string[]; values: unknown[] };
export type TrbOrmGroupedQuery = { query: TrbOrmQuery; column: string; having: { expression: string; operator: string; value: unknown } | null };
export type TrbOrmSubquery<T> = { query: TrbOrmQuery; column: string; readonly __value?: T };
type TrbOrmRange = { __trbRange: true; start: unknown; end: unknown; exclusive: boolean };

const __trbOrmModels = new Map<string, TrbOrmModel>();
const __trbOrmScopes = new WeakMap<object, TrbOrmQuery>();
const __trbOrmAssociations = new WeakMap<object, Map<string, { loaded: boolean; value: unknown }>>();
const __trbOrmExecutionScopes = new AsyncLocalStorage<AbortSignal | undefined>();
let __trbOrmDatabase: SQL | null = null;

export function withScope<T>(signal: AbortSignal | undefined, callback: () => Promise<T>): Promise<T> {
  return __trbOrmExecutionScopes.run(signal, callback);
}

export class Transaction {
  constructor(readonly sql: SQL | TransactionSQL) {}
}
export class Database {}

export function registerModel(model: TrbOrmModel, constructor: new () => any): void {
  __trbOrmModels.set(model.name, { ...model, factory: constructor });
}

export function range(start: unknown, end: unknown, exclusive: boolean): TrbOrmRange {
  return { __trbRange: true, start, end, exclusive };
}

function resultOk<T>(value: T): DbResult<T> { return Result.Ok<T, DbError>(value); }
function resultErr<T>(kind: DbErrorKind, message: string): DbResult<T> { return Result.Err<T, DbError>({ kind, message }); }
function errorKind(error: unknown): DbErrorKind {
  if (error instanceof TrbOrmExecutionError) return error.kind;
  const code = String((error as any)?.code ?? "");
  if (/CONSTRAINT|UNIQUE|FOREIGN|NOT_NULL|^23|23000|1062|1451|1452/i.test(code)) return DbErrorKind.Constraint;
  if (/TIMEOUT|TIMEDOUT|57014/i.test(code)) return DbErrorKind.Timeout;
  if (/CONNECT|CONNECTION|ECONN/i.test(code)) return DbErrorKind.Connection;
  return DbErrorKind.Query;
}
function databaseError<T>(error: unknown, fallback: string): DbResult<T> {
  const message = error instanceof Error && error.message !== "" ? error.message : fallback;
  return resultErr<T>(errorKind(error), message);
}
class TrbOrmExecutionError extends Error { constructor(readonly kind: DbErrorKind, message: string) { super(message); } }
function configuredDatabase(): string {
  const environment = (globalThis as any).process?.env as Record<string, string | undefined> | undefined;
  const value = __trbOrmDatabaseEnvironment === "" ? "" : (environment?.[__trbOrmDatabaseEnvironment] ?? "");
  const configured = value === "" ? __trbOrmConfiguredDatabase : value;
  if (__trbOrmAdapter !== "mysql" || configured.startsWith("mysql://")) return configured;
  const match = configured.match(/^([^:@]+)(?::([^@]*))?@tcp\(([^)]+)\)\/([^?]+)(?:\?.*)?$/);
  if (match === null) return configured;
  const credentials = encodeURIComponent(match[1]!) + (match[2] === undefined ? "" : ":" + encodeURIComponent(match[2]));
  return "mysql://" + credentials + "@" + match[3] + "/" + match[4];
}
function database(): SQL {
  if (__trbOrmDatabase !== null) return __trbOrmDatabase;
  const value = configuredDatabase();
  if (value === "") throw new Error("database configuration is empty");
  __trbOrmDatabase = __trbOrmAdapter === "sqlite" ? new SQL({ adapter: "sqlite", filename: value }) : new SQL(value);
  return __trbOrmDatabase;
}
function executor(query: TrbOrmQuery): SQL | TransactionSQL { return query.transaction?.sql ?? database(); }
function model(name: string): TrbOrmModel {
  const value = __trbOrmModels.get(name);
  if (value === undefined) throw new Error("ORM model is not registered: " + name);
  return value;
}
function quote(name: string): string {
  const mark = __trbOrmAdapter === "mysql" ? String.fromCharCode(96) : "\"";
  return mark + name.replaceAll(mark, mark + mark) + mark;
}
function placeholder(index: number): string { return __trbOrmAdapter === "postgresql" ? "$" + index : "?"; }
function cloneQuery(value: TrbOrmQuery): TrbOrmQuery {
  return { ...value, joins: [...value.joins], orders: [...value.orders], preloads: [...value.preloads] };
}
export function query(name: string): TrbOrmQuery {
  return { model: name, transaction: null, predicate: null, joins: [], orders: [], limit: null, offset: null, lock: false, distinct: false, preloads: [] };
}
export function modelName(name: string, constructor: Function): string { void constructor; return name; }
export function using(name: string, transaction: Transaction): TrbOrmQuery { return { ...query(name), transaction }; }
function combine(kind: "and" | "or", left: TrbOrmPredicate | null, right: TrbOrmPredicate | null): TrbOrmPredicate | null {
  if (left === null) return right;
  if (right === null) return left;
  return { kind, left, right };
}
export function where(source: TrbOrmQuery, columns: string[], operators: string[], values: unknown[]): TrbOrmQuery {
  const predicate: TrbOrmPredicate = { kind: "conditions", columns, operators, values };
  return { ...cloneQuery(source), predicate: combine("and", source.predicate, predicate) };
}
export function not(source: TrbOrmQuery, columns: string[], operators: string[], values: unknown[]): TrbOrmQuery {
  return where(source, columns, operators.map(value => value === "=" ? "!=" : value), values);
}
export function or(left: TrbOrmQuery, right: TrbOrmQuery): TrbOrmQuery {
  return { ...cloneQuery(left), predicate: combine("or", left.predicate, right.predicate) };
}
export function order(source: TrbOrmQuery, columns: string[], directions: string[]): TrbOrmQuery {
  return { ...cloneQuery(source), orders: [...source.orders, ...columns.map((column, index) => [column, directions[index]!] as [string, string])] };
}
export function limit(source: TrbOrmQuery, value: number): TrbOrmQuery { return { ...cloneQuery(source), limit: value }; }
export function offset(source: TrbOrmQuery, value: number): TrbOrmQuery { return { ...cloneQuery(source), offset: value }; }
export function lock(source: TrbOrmQuery): TrbOrmQuery { return { ...cloneQuery(source), lock: true }; }
export function distinct(source: TrbOrmQuery): TrbOrmQuery { return { ...cloneQuery(source), distinct: true }; }
export function preload(source: TrbOrmQuery, association: string, target?: TrbOrmQuery): TrbOrmQuery {
  return { ...cloneQuery(source), preloads: [...source.preloads, { association, query: target }] };
}
export function select<T>(source: TrbOrmQuery, column: string): TrbOrmSubquery<T> { return { query: cloneQuery(source), column }; }
export function group(source: TrbOrmQuery, column: string): TrbOrmGroupedQuery { return { query: cloneQuery(source), column, having: null }; }
export function having(source: TrbOrmGroupedQuery, expression: string, operator: string, value: unknown): TrbOrmGroupedQuery { return { ...source, having: { expression, operator, value } }; }
export function join(source: TrbOrmQuery, association: string, target: TrbOrmQuery | undefined, kind: "INNER JOIN" | "LEFT JOIN"): TrbOrmQuery {
  return { ...cloneQuery(source), joins: [...source.joins, { kind, association, query: target }] };
}
export function whereExists(source: TrbOrmQuery, association: string, target: TrbOrmQuery | undefined, negated: boolean): TrbOrmQuery {
  const value = { __trbExists: true, association, target, negated };
  return where(source, [""], ["EXISTS"], [value]);
}

function association(owner: TrbOrmModel, name: string): TrbOrmAssociation {
  const value = owner.associations.find(item => item.name === name);
  if (value === undefined) throw new Error("ORM association is not registered: " + owner.name + "." + name);
  return value;
}
function associationScope(relation: TrbOrmAssociation, source: TrbOrmQuery): TrbOrmQuery { return relation.scope === undefined ? source : relation.scope(source); }
function relationPredicate(owner: TrbOrmModel, relation: TrbOrmAssociation, targetQuery: TrbOrmQuery, args: unknown[], ownerAlias: string): string {
  const target = model(relation.target);
  const targetAlias = "trb_assoc_" + args.length;
  const scoped = associationScope(relation, targetQuery);
  if (relation.through === "") {
    const targetSQL = statement(scoped, quote(targetAlias) + "." + quote(relation.targetColumn), args, targetAlias, false);
    return quote(ownerAlias) + "." + quote(relation.sourceColumn) + " IN (" + targetSQL + ")";
  }
  const through = association(owner, relation.through);
  const middle = model(through.target);
  const via = association(middle, relation.source);
  const middleAlias = "trb_through_" + args.length;
  const targetSQL = statement(scoped, quote(targetAlias) + "." + quote(via.targetColumn), args, targetAlias, false);
  const keys = "SELECT " + quote(middleAlias) + "." + quote(through.targetColumn) + " FROM " + quote(middle.table) + " AS " + quote(middleAlias) +
    " INNER JOIN (" + targetSQL + ") AS " + quote(targetAlias) + " ON " + quote(middleAlias) + "." + quote(via.sourceColumn) + " = " + quote(targetAlias) + "." + quote(via.targetColumn);
  return quote(ownerAlias) + "." + quote(through.sourceColumn) + " IN (" + keys + ")";
}
function predicateSQL(owner: TrbOrmModel, predicate: TrbOrmPredicate, args: unknown[], tableAlias = owner.table): string {
  if (predicate.kind !== "conditions") {
    return "(" + predicateSQL(owner, predicate.left, args, tableAlias) + " " + predicate.kind.toUpperCase() + " " + predicateSQL(owner, predicate.right, args, tableAlias) + ")";
  }
  const conditions: string[] = [];
  for (let index = 0; index < predicate.columns.length; index += 1) {
    const column = predicate.columns[index]!;
    let operator = predicate.operators[index] ?? "=";
    const value = predicate.values[index];
    if (operator === "EXISTS" && typeof value === "object" && value !== null && (value as any).__trbExists === true) {
      const exists = value as any;
      const relation = association(owner, exists.association);
      const targetQuery = exists.target ?? query(relation.target);
      const relationSQL = relationPredicate(owner, relation, targetQuery, args, tableAlias);
      conditions.push((exists.negated ? "NOT " : "") + "(" + relationSQL + ")");
      continue;
    }
    const qualified = quote(tableAlias) + "." + quote(column);
    if (value === null) {
      conditions.push(qualified + (operator === "!=" ? " IS NOT NULL" : " IS NULL"));
      continue;
    }
    if (typeof value === "object" && value !== null && (value as any).__trbRange === true) {
      const interval = value as TrbOrmRange;
      const upper = interval.exclusive ? "<" : "<=";
      const lowerBind = bindValue(owner, column, interval.start, args); const upperBind = bindValue(owner, column, interval.end, args);
      conditions.push("(" + qualified + " >= " + lowerBind + " AND " + qualified + " " + upper + " " + upperBind + ")");
      continue;
    }
    if (typeof value === "object" && value !== null && "query" in (value as any) && "column" in (value as any)) {
      const subquery = value as TrbOrmSubquery<unknown>;
      const sql = statement(subquery.query, quote(subquery.column), args);
      conditions.push(qualified + (operator === "!=" ? " NOT IN " : " IN ") + "(" + sql + ")");
      continue;
    }
    if (Array.isArray(value)) {
      if (value.length === 0) { conditions.push(operator === "!=" ? "1 = 1" : "1 = 0"); continue; }
      const binds = value.map(item => bindValue(owner, column, item, args));
      conditions.push(qualified + (operator === "!=" ? " NOT IN (" : " IN (") + binds.join(", ") + ")");
      continue;
    }
    conditions.push(qualified + " " + operator + " " + bindValue(owner, column, value, args));
  }
  return conditions.length === 0 ? "1 = 1" : conditions.join(" AND ");
}
function joinSQL(owner: TrbOrmModel, entry: TrbOrmJoin, args: unknown[], index: number, ownerAlias: string): string {
  const relation = association(owner, entry.association);
  const middleAlias = "trb_join_" + index + "_middle";
  const targetAlias = "trb_join_" + index + "_target";
  if (relation.through !== "") {
    const through = association(owner, relation.through);
    const middle = model(through.target);
    const source = association(middle, relation.source);
    const target = model(relation.target);
    const first = entry.kind + " " + quote(middle.table) + " AS " + quote(middleAlias) + " ON " + quote(ownerAlias) + "." + quote(through.sourceColumn) + " = " + quote(middleAlias) + "." + quote(through.targetColumn);
    const second = entry.kind + " " + quote(target.table) + " AS " + quote(targetAlias) + " ON " + quote(middleAlias) + "." + quote(source.sourceColumn) + " = " + quote(targetAlias) + "." + quote(source.targetColumn);
    const scoped = associationScope(relation, entry.query ?? query(relation.target));
    const extra = scoped.predicate === null ? "" : " AND " + predicateSQL(target, scoped.predicate, args, targetAlias);
    return first + " " + second + extra;
  }
  const target = model(relation.target);
  let sql = entry.kind + " " + quote(target.table) + " AS " + quote(targetAlias) + " ON " + quote(ownerAlias) + "." + quote(relation.sourceColumn) + " = " + quote(targetAlias) + "." + quote(relation.targetColumn);
  const scoped = associationScope(relation, entry.query ?? query(relation.target));
  if (scoped.predicate !== null) sql += " AND " + predicateSQL(target, scoped.predicate, args, targetAlias);
  return sql;
}
function statement(source: TrbOrmQuery, projection: string, args: unknown[], alias?: string, includeTail = true): string {
  const owner = model(source.model);
  const ownerAlias = alias ?? owner.table;
  let sql = "SELECT " + (source.distinct ? "DISTINCT " : "") + projection + " FROM " + quote(owner.table);
  if (alias !== undefined) sql += " AS " + quote(alias);
  for (let index = 0; index < source.joins.length; index += 1) sql += " " + joinSQL(owner, source.joins[index]!, args, index, ownerAlias);
  if (source.predicate !== null) sql += " WHERE " + predicateSQL(owner, source.predicate, args, ownerAlias);
  if (!includeTail) return sql;
  if (source.orders.length > 0) sql += " ORDER BY " + source.orders.map(([column, direction]) => quote(ownerAlias) + "." + quote(column) + " " + direction.toUpperCase()).join(", ");
  if (source.limit !== null) { args.push(source.limit); sql += " LIMIT " + placeholder(args.length); }
  if (source.offset !== null) {
    if (source.limit === null) sql += __trbOrmAdapter === "mysql" ? " LIMIT 18446744073709551615" : (__trbOrmAdapter === "sqlite" ? " LIMIT -1" : "");
    args.push(source.offset); sql += " OFFSET " + placeholder(args.length);
  }
  if (source.lock && __trbOrmAdapter !== "sqlite") sql += " FOR UPDATE";
  return sql;
}
function readColumn(column: TrbOrmColumn, expression: string): string {
  if (__trbOrmAdapter === "mysql") {
    if (column.kind === "date") return "DATE_FORMAT(" + expression + ", '%Y-%m-%d')";
    if (column.kind === "timeofday") return "TIME_FORMAT(" + expression + ", '%H:%i:%s.%f')";
    if (column.kind === "datetime") return "DATE_FORMAT(" + expression + ", '%Y-%m-%dT%H:%i:%s.%f')";
    if (column.kind === "instant") return "DATE_FORMAT(CONVERT_TZ(" + expression + ", @@session.time_zone, '+00:00'), '%Y-%m-%dT%H:%i:%s.%fZ')";
  }
  if (__trbOrmAdapter === "postgresql") {
    if (column.kind === "date") return "to_char(" + expression + ", 'YYYY-MM-DD')";
    if (column.kind === "timeofday") return "to_char(" + expression + ", 'HH24:MI:SS.US')";
    if (column.kind === "datetime") return "to_char(" + expression + ", 'YYYY-MM-DD\"T\"HH24:MI:SS.US')";
    if (column.kind === "instant") return "to_char(" + expression + " AT TIME ZONE 'UTC', 'YYYY-MM-DD\"T\"HH24:MI:SS.US\"Z\"')";
  }
  return expression;
}
function projection(owner: TrbOrmModel): string { return owner.columns.map(column => readColumn(column, quote(owner.table) + "." + quote(column.name)) + " AS " + quote(column.name)).join(", "); }
function normalizeColumn(column: TrbOrmColumn, value: unknown): unknown {
  if (value === null || value === undefined) return null;
  if (column.enumValues.length > 0) {
    const mapped = column.enumValues.find(([, storage]) => storage === value || String(storage) === String(value));
    if (mapped === undefined) throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database enum column " + column.name + " contains an unknown value");
    return mapped[0];
  }
  if (column.kind === "boolean") return value === true || value === 1 || value === "1";
  if (column.kind === "integer") return Number(value);
  if (column.kind === "float") return Number(value);
  if (column.kind === "bytes" && !(value instanceof Uint8Array)) return new Uint8Array(value as ArrayLike<number>);
	if (column.kind === "date" || column.kind === "timeofday" || column.kind === "datetime" || column.kind === "instant") {
		let text: string;
		if (value instanceof globalThis.Date) {
			const base = value.toISOString();
			text = column.kind === "date" ? base.slice(0, 10) : (column.kind === "timeofday" ? base.slice(11, 23).replace(/\.000$/, "") : (column.kind === "datetime" ? base.slice(0, 19) : base));
		} else {
			text = String(value).trim();
		}
		if (column.kind === "date") {
			if (text.length > 10) text = text.slice(0, 10);
			const parsed = column.parse!(text); if (parsed.kind === "Err") throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database Date column " + column.name + " is invalid"); return parsed.value;
		}
		if (column.kind === "timeofday") {
			const separator = Math.max(text.lastIndexOf("T"), text.lastIndexOf(" ")); if (separator >= 0) text = text.slice(separator + 1);
			const parsed = column.parse!(text); if (parsed.kind === "Err") throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database TimeOfDay column " + column.name + " is invalid"); return parsed.value;
		}
		if (column.kind === "datetime") {
			text = text.replace(" ", "T"); const parsed = column.parse!(text); if (parsed.kind === "Err") throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database DateTime column " + column.name + " is invalid"); return parsed.value;
		}
		text = text.replace(" ", "T"); const tail = text.slice(text.indexOf("T") + 1); if (!text.endsWith("Z") && !/[+-]/.test(tail)) text += "Z";
		const parsed = column.parse!(text); if (parsed.kind === "Err") throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database Instant column " + column.name + " is invalid"); return parsed.value;
	}
  return value;
}
function databaseValue(owner: TrbOrmModel, columnName: string, value: unknown): unknown {
  if (value === null || value === undefined) return null;
  const column = owner.columns.find(item => item.name === columnName);
	if (column === undefined) return value;
	if (column.kind === "date" || column.kind === "timeofday") return (value as any).to_s();
	if (column.kind === "instant") { const text = (value as any).to_s(); return __trbOrmAdapter === "mysql" ? text.replace("T", " ").replace(/Z$/, "") : text; }
	if (column.kind === "datetime") return (value as any).to_s().replace("T", " ");
	if (column.enumValues.length === 0) return value;
  const mapped = column.enumValues.find(([member]) => member === value);
  if (mapped === undefined) throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "enum column " + columnName + " received an unknown value");
  return mapped[1];
}
function bindValue(owner: TrbOrmModel, columnName: string, value: unknown, args: unknown[]): string {
  args.push(databaseValue(owner, columnName, value));
  const bind = placeholder(args.length); const column = owner.columns.find(item => item.name === columnName);
  return __trbOrmAdapter === "mysql" && column?.kind === "instant" ? "CONVERT_TZ(" + bind + ", '+00:00', @@session.time_zone)" : bind;
}
function instantiate(owner: TrbOrmModel, row: Record<string, unknown>, scope: TrbOrmQuery): any {
  const Constructor = owner.factory;
  if (Constructor === undefined) throw new Error("ORM model constructor is unavailable: " + owner.name);
  const value = new Constructor();
  for (const column of owner.columns) value["__trb_" + column.name] = normalizeColumn(column, row[column.name]);
  __trbOrmScopes.set(value, scope);
  return value;
}
export function column(value: any, name: string): any { return value["__trb_" + name]; }
function scopeFor(value: any, name: string): TrbOrmQuery { return __trbOrmScopes.get(value) ?? query(name); }
async function unsafe(source: TrbOrmQuery, sql: string, args: unknown[]): Promise<any[]> {
  if (source.lock && source.transaction === null) throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "database lock requires an explicit transaction scope");
  const signal = __trbOrmExecutionScopes.getStore();
  if (signal?.aborted) throw new TrbOrmExecutionError(DbErrorKind.Timeout, "database operation was cancelled");
  const operation = executor(source).unsafe(sql, args) as Promise<any[]> & { cancel?: () => void };
  const cancel = () => operation.cancel?.();
  signal?.addEventListener("abort", cancel, { once: true });
  try { return await operation; }
  catch (error) { if (signal?.aborted) throw new TrbOrmExecutionError(DbErrorKind.Timeout, "database operation was cancelled"); throw error; }
  finally { signal?.removeEventListener("abort", cancel); }
}
function affectedRows(result: any): number { return Number(result.affectedRows ?? result.count ?? result.changes ?? result.length ?? 0); }

export function toSQL(source: TrbOrmQuery): string { const args: unknown[] = []; return statement(source, projection(model(source.model)), args); }
export async function all(source: TrbOrmQuery): Promise<DbResult<any[]>> {
  try {
    const owner = model(source.model); const args: unknown[] = []; const sql = statement(source, projection(owner), args);
    const rows = await unsafe(source, sql, args); const values = rows.map(row => instantiate(owner, row, source));
    for (const preloadEntry of source.preloads) for (const value of values) { const loaded = await loadAssociation(value, preloadEntry.association, false, preloadEntry.query); if (loaded.kind === "Err") return loaded; }
    return resultOk(values);
  } catch (error) { return databaseError<any[]>(error, "database query failed"); }
}
export async function first(source: TrbOrmQuery): Promise<DbResult<any | null>> {
  const loaded = await all(limit(source, 1));
  return loaded.kind === "Err" ? loaded : resultOk(loaded.value[0] ?? null);
}
export async function count(source: TrbOrmQuery): Promise<DbResult<number>> {
  try { const args: unknown[] = []; const rows = await unsafe(source, statement(source, "COUNT(*) AS trb_count", args), args); return resultOk(Number(rows[0]?.trb_count ?? 0)); }
  catch (error) { return databaseError<number>(error, "database count failed"); }
}
export async function exists(source: TrbOrmQuery): Promise<DbResult<boolean>> {
  const value = await count(limit(source, 1)); return value.kind === "Err" ? value : resultOk(value.value > 0);
}
export async function explain(source: TrbOrmQuery): Promise<DbResult<string>> {
  try { const args: unknown[] = []; const prefix = __trbOrmAdapter === "sqlite" ? "EXPLAIN QUERY PLAN " : (__trbOrmAdapter === "mysql" ? "EXPLAIN FORMAT=JSON " : "EXPLAIN "); const rows = await unsafe(source, prefix + statement(source, projection(model(source.model)), args), args); return resultOk(JSON.stringify(rows)); }
  catch (error) { return databaseError<string>(error, "database explain failed"); }
}
export async function pluck(source: TrbOrmQuery, columnName: string): Promise<DbResult<any[]>> {
  try { const owner = model(source.model); const column = owner.columns.find(item => item.name === columnName); if (column === undefined) throw new TrbOrmExecutionError(DbErrorKind.InvalidData, "unknown projection column " + columnName); const args: unknown[] = []; const rows = await unsafe(source, statement(source, readColumn(column, quote(columnName)) + " AS trb_value", args), args); return resultOk(rows.map(row => normalizeColumn(column, row.trb_value))); }
  catch (error) { return databaseError<any[]>(error, "database projection query failed"); }
}
export async function pick(source: TrbOrmQuery, columnName: string): Promise<DbResult<any | null>> {
  const values = await pluck(limit(source, 1), columnName); return values.kind === "Err" ? values : resultOk(values.value[0] ?? null);
}
export async function aggregate(source: TrbOrmQuery, operation: string, columnName: string): Promise<DbResult<any>> {
  try {
    const sqlOperation = operation === "average" ? "AVG" : (operation === "minimum" ? "MIN" : (operation === "maximum" ? "MAX" : operation.toUpperCase()));
    const column = model(source.model).columns.find(item => item.name === columnName); let expression = operation === "sum" ? "COALESCE(SUM(" + quote(columnName) + "), 0)" : sqlOperation + "(" + quote(columnName) + ")"; if ((operation === "minimum" || operation === "maximum") && column !== undefined) expression = readColumn(column, expression);
    const args: unknown[] = []; const rows = await unsafe(source, statement(source, expression + " AS trb_aggregate", args), args); const value = rows[0]?.trb_aggregate ?? null;
		const normalized = value === null || column === undefined ? value : normalizeColumn(column, value);
		return resultOk(operation === "average" && normalized !== null ? Number(normalized) : normalized);
  } catch (error) { return databaseError<any>(error, "database aggregate query failed"); }
}
export async function groupedAggregate(source: TrbOrmGroupedQuery, operation: string, target?: string): Promise<DbResult<Record<string, any>>> {
  try {
    const owner = model(source.query.model); let expression = "COUNT(*)";
    if (operation !== "count") { const sqlOperation = operation === "average" ? "AVG" : (operation === "minimum" ? "MIN" : (operation === "maximum" ? "MAX" : operation.toUpperCase())); expression = operation === "sum" ? "COALESCE(SUM(" + quote(target!) + "), 0)" : sqlOperation + "(" + quote(target!) + ")"; const targetColumn = owner.columns.find(column => column.name === target); if ((operation === "minimum" || operation === "maximum") && targetColumn !== undefined) expression = readColumn(targetColumn, expression); }
    const args: unknown[] = []; let sql = statement(source.query, quote(source.column) + " AS trb_group, " + expression + " AS trb_value", args, undefined, false) + " GROUP BY " + quote(source.column);
    if (source.having !== null) { args.push(source.having.value); const havingExpression = source.having.expression === "count" ? "COUNT(*)" : source.having.expression; sql += " HAVING " + havingExpression + " " + source.having.operator + " " + placeholder(args.length); }
    if (source.query.orders.length > 0) sql += " ORDER BY " + source.query.orders.map(([column, direction]) => quote(column) + " " + direction.toUpperCase()).join(", ");
    const rows = await unsafe(source.query, sql, args); const result: Record<string, any> = {}; const targetColumn = target === undefined ? undefined : owner.columns.find(column => column.name === target); const groupColumn = owner.columns.find(column => column.name === source.column)!; for (const row of rows) { let value = row.trb_value; if (operation === "count" || operation === "average" || targetColumn?.kind === "integer" || targetColumn?.kind === "float") value = Number(value); else if (targetColumn !== undefined) value = normalizeColumn(targetColumn, value); result[String(normalizeColumn(groupColumn, row.trb_group))] = value; } return resultOk(result);
  } catch (error) { return databaseError<Record<string, any>>(error, "database grouped aggregate failed"); }
}

export function build(name: string, source: TrbOrmQuery, columns: string[], values: unknown[]): TrbOrmDraft { return { model: name, query: cloneQuery(source), columns, values }; }
function draftValue(draft: TrbOrmDraft, column: string): unknown { const index = draft.columns.indexOf(column); return index < 0 ? undefined : draft.values[index]; }
function validUnique(owner: TrbOrmModel, columns: string[]): boolean { return owner.unique.some(unique => unique.length === columns.length && unique.every((column, index) => column === columns[index])); }
async function inserted(source: TrbOrmQuery, owner: TrbOrmModel, columns: string[], values: unknown[]): Promise<DbResult<any>> {
  try {
    const args: unknown[] = []; const binds = values.map((value, index) => bindValue(owner, columns[index]!, value, args)); let sql = "INSERT INTO " + quote(owner.table);
    if (columns.length === 0) sql += __trbOrmAdapter === "mysql" ? " () VALUES ()" : " DEFAULT VALUES";
    else sql += " (" + columns.map(quote).join(", ") + ") VALUES (" + binds.join(", ") + ")";
    const primary = owner.columns.find(column => column.primary);
    if (__trbOrmAdapter !== "mysql") sql += " RETURNING " + projection(owner).replaceAll(quote(owner.table) + ".", "");
    const rows = await unsafe(source, sql, args);
    if (__trbOrmAdapter !== "mysql") return resultOk(instantiate(owner, rows[0]!, source));
    const insertId = Number((rows as any).lastInsertRowid ?? (rows as any).insertId ?? 0);
    if (primary !== undefined && insertId > 0) return await first(where(source, [primary.name], ["="], [insertId]));
    const predicateColumns = columns.filter((column, index) => values[index] !== null);
    const predicateValues = predicateColumns.map(column => values[columns.indexOf(column)]);
    return await first(where(source, predicateColumns, predicateColumns.map(() => "="), predicateValues));
  } catch (error) { return databaseError<any>(error, "database insert failed"); }
}
export async function create(name: string, source: TrbOrmQuery, columns: string[], values: unknown[]): Promise<DbResult<any>> { return inserted(source, model(name), columns, values); }
export async function saveDraft(draft: TrbOrmDraft): Promise<DbResult<any>> { return create(draft.model, draft.query, draft.columns, draft.values); }
export function changes(value: any, name: string, columns: string[], values: unknown[]): TrbOrmChanges { return { model: name, value, columns, values }; }
export async function saveChanges(value: TrbOrmChanges): Promise<DbResult<any>> { return update(value.model, value.value, value.columns, value.values); }
export async function update(name: string, value: any, columns: string[], values: unknown[]): Promise<DbResult<any>> {
  try {
    const owner = model(name); const primary = owner.columns.find(column => column.primary); if (primary === undefined) return resultErr(DbErrorKind.InvalidData, "model has no primary key");
    const args: unknown[] = []; const binds = values.map((item, index) => bindValue(owner, columns[index]!, item, args)); const primaryBind = bindValue(owner, primary.name, column(value, primary.name), args); let sql = "UPDATE " + quote(owner.table) + " SET " + columns.map((item, index) => quote(item) + " = " + binds[index]).join(", ") + " WHERE " + quote(primary.name) + " = " + primaryBind;
    if (__trbOrmAdapter !== "mysql") sql += " RETURNING " + projection(owner).replaceAll(quote(owner.table) + ".", "");
    const source = scopeFor(value, name); const rows = await unsafe(source, sql, args);
    if (__trbOrmAdapter !== "mysql") return resultOk(instantiate(owner, rows[0]!, source));
    return await first(where(source, [primary.name], ["="], [column(value, primary.name)]));
  } catch (error) { return databaseError<any>(error, "database update failed"); }
}
export async function updateAll(source: TrbOrmQuery, columns: string[], values: unknown[]): Promise<DbResult<number>> {
  try { const owner = model(source.model); const args: unknown[] = []; const binds = values.map((value, index) => bindValue(owner, columns[index]!, value, args)); let sql = "UPDATE " + quote(owner.table) + " SET " + columns.map((item, index) => quote(item) + " = " + binds[index]).join(", "); if (source.predicate !== null) sql += " WHERE " + predicateSQL(owner, source.predicate, args); const rows = await unsafe(source, sql, args); return resultOk(affectedRows(rows)); }
  catch (error) { return databaseError<number>(error, "database bulk update failed"); }
}
export async function deleteAll(source: TrbOrmQuery): Promise<DbResult<number>> {
  try { const owner = model(source.model); const args: unknown[] = []; let sql = "DELETE FROM " + quote(owner.table); if (source.predicate !== null) sql += " WHERE " + predicateSQL(owner, source.predicate, args); const rows = await unsafe(source, sql, args); return resultOk(affectedRows(rows)); }
  catch (error) { return databaseError<number>(error, "database bulk delete failed"); }
}
export async function deleteValue(name: string, value: any): Promise<DbResult<boolean>> {
  const owner = model(name); const primary = owner.columns.find(column => column.primary); if (primary === undefined) return resultErr(DbErrorKind.InvalidData, "model has no primary key");
  const deleted = await deleteAll(where(scopeFor(value, name), [primary.name], ["="], [column(value, primary.name)])); return deleted.kind === "Err" ? deleted : resultOk(deleted.value > 0);
}
async function destroyInTransaction(name: string, value: any, transactionValue: Transaction): Promise<DbResult<boolean>> {
  const owner = model(name);
  __trbOrmScopes.set(value, using(name, transactionValue));
  for (const relation of owner.associations) {
    if (relation.dependent === "") continue;
    const related = associationQuery(value, name, relation.name, transactionValue);
    if (relation.dependent === "restrict") { const present = await exists(related); if (present.kind === "Err") return present; if (present.value) return resultErr(DbErrorKind.Constraint, "dependent association " + owner.name + "." + relation.name + " restricts destroy"); }
    if (relation.dependent === "delete") { const removed = await deleteAll(related); if (removed.kind === "Err") return removed; }
    if (relation.dependent === "nullify") { const changed = await updateAll(related, [relation.targetColumn], [null]); if (changed.kind === "Err") return changed; }
    if (relation.dependent === "destroy") { const loaded = await all(related); if (loaded.kind === "Err") return loaded; for (const item of loaded.value) { const removed = await destroyInTransaction(relation.target, item, transactionValue); if (removed.kind === "Err") return removed; } }
  }
  return deleteValue(name, value);
}
export async function destroy(name: string, value: any): Promise<DbResult<boolean>> {
  const existing = scopeFor(value, name).transaction;
  if (existing !== null) return destroyInTransaction(name, value, existing);
  return transaction(null, async tx => destroyInTransaction(name, value, tx));
}
export async function destroyAll(source: TrbOrmQuery): Promise<DbResult<number>> {
  const loaded = await all(source); if (loaded.kind === "Err") return loaded; let count = 0; for (const value of loaded.value) { const result = await destroy(source.model, value); if (result.kind === "Err") return result; if (result.value) count += 1; } return resultOk(count);
}
export async function insertAll(name: string, drafts: TrbOrmDraft[]): Promise<DbResult<number>> {
  return transaction(null, async tx => { let count = 0; for (const draft of drafts) { const value = await create(name, using(name, tx), draft.columns, draft.values); if (value.kind === "Err") return value as DbResult<number>; count += 1; } return resultOk(count); });
}
export async function insertIfAbsent(draft: TrbOrmDraft, unique: string[]): Promise<DbResult<boolean>> {
  const owner = model(draft.model); if (!validUnique(owner, unique)) return resultErr(DbErrorKind.InvalidData, "unique_by must match a primary or unique constraint"); const values = unique.map(column => draftValue(draft, column));
  const found = await first(where(draft.query, unique, unique.map(() => "="), values)); if (found.kind === "Err") return found; if (found.value !== null) return resultOk(false);
  const created = await saveDraft(draft); if (created.kind === "Ok") return resultOk(true); if (created.error.kind !== DbErrorKind.Constraint) return created; return resultOk(false);
}
export async function upsert(draft: TrbOrmDraft, unique: string[], updates: string[]): Promise<DbResult<any>> {
  const owner = model(draft.model); if (!validUnique(owner, unique)) return resultErr(DbErrorKind.InvalidData, "unique_by must match a primary or unique constraint"); const values = unique.map(column => draftValue(draft, column));
  const found = await first(where(draft.query, unique, unique.map(() => "="), values)); if (found.kind === "Err") return found;
  if (found.value === null) { const created = await saveDraft(draft); if (created.kind === "Ok" || created.error.kind !== DbErrorKind.Constraint) return created; const retried = await first(where(draft.query, unique, unique.map(() => "="), values)); if (retried.kind === "Err" || retried.value === null) return created; return update(draft.model, retried.value, updates, updates.map(column => draftValue(draft, column))); }
  return update(draft.model, found.value, updates, updates.map(column => draftValue(draft, column)));
}
export async function upsertAll(name: string, drafts: TrbOrmDraft[], unique: string[], updates: string[]): Promise<DbResult<number>> {
  return transaction(null, async tx => { let count = 0; for (const draft of drafts) { const value = await upsert({ ...draft, query: using(name, tx) }, unique, updates); if (value.kind === "Err") return value as DbResult<number>; count += 1; } return resultOk(count); });
}

function associationQuery(value: any, ownerName: string, associationName: string, transactionOverride?: Transaction): TrbOrmQuery {
  const owner = model(ownerName); const relation = association(owner, associationName); const transactionValue = transactionOverride ?? scopeFor(value, ownerName).transaction;
  if (relation.through !== "") {
    const through = association(owner, relation.through); const middle = model(through.target); const source = association(middle, relation.source);
    let target = transactionValue === null ? query(relation.target) : using(relation.target, transactionValue);
    target = join(target, source.inverse === "" ? middle.name.toLowerCase() : source.inverse, where(transactionValue === null ? query(middle.name) : using(middle.name, transactionValue), [through.targetColumn], ["="], [column(value, through.sourceColumn)]), "INNER JOIN");
    return associationScope(relation, target);
  }
  const target = transactionValue === null ? query(relation.target) : using(relation.target, transactionValue);
  return associationScope(relation, where(target, [relation.targetColumn], ["="], [column(value, relation.sourceColumn)]));
}
export function associationRelation(value: any, ownerName: string, associationName: string): TrbOrmQuery { return associationQuery(value, ownerName, associationName); }
function associationCache(value: object): Map<string, { loaded: boolean; value: unknown }> { let cache = __trbOrmAssociations.get(value); if (cache === undefined) { cache = new Map(); __trbOrmAssociations.set(value, cache); } return cache; }
export function associationLoaded(value: object, name: string): boolean { return associationCache(value).get(name)?.loaded === true; }
export async function loadAssociation(value: any, name: string, reload: boolean, target?: TrbOrmQuery): Promise<DbResult<any>> {
  const ownerEntry = [...__trbOrmModels.values()].find(entry => value instanceof (entry.factory as any)); if (ownerEntry === undefined) return resultErr(DbErrorKind.InvalidData, "association owner is not an ORM model");
  const cache = associationCache(value); const cached = cache.get(name); if (!reload && cached?.loaded === true) return resultOk(cached.value);
  const relation = association(ownerEntry, name); let source = associationQuery(value, ownerEntry.name, name);
  if (target !== undefined) { target = { ...cloneQuery(target), transaction: source.transaction }; if (target.predicate !== null) source = { ...cloneQuery(source), predicate: combine("and", source.predicate, target.predicate), orders: target.orders, limit: target.limit, offset: target.offset, preloads: target.preloads }; }
  const loaded = relation.kind === "has_many" ? await all(source) : await first(source); if (loaded.kind === "Err") return loaded;
  if (relation.kind === "has_one") { const countValue = await count(limit(source, 2)); if (countValue.kind === "Err") return countValue; if (countValue.value > 1) return resultErr(DbErrorKind.InvalidData, "database has_one association returned multiple rows"); }
  cache.set(name, { loaded: true, value: loaded.value });
  if (relation.inverse !== "") {
    const values = Array.isArray(loaded.value) ? loaded.value : (loaded.value === null ? [] : [loaded.value]);
    const inverse = association(model(relation.target), relation.inverse);
    for (const related of values) associationCache(related).set(relation.inverse, { loaded: true, value: inverse.kind === "has_many" ? [value] : value });
  }
  return loaded;
}

class TrbOrmRollback extends Error { constructor(readonly result: DbResult<any>) { super("TypeRB ORM rollback"); } }
export async function transaction<T>(parent: Transaction | null, callback: (transaction: Transaction) => Promise<DbResult<T>>): Promise<DbResult<T>> {
  try {
    const run = async (client: SQL | TransactionSQL): Promise<T> => { const result = await callback(new Transaction(client)); if (result.kind === "Err") throw new TrbOrmRollback(result); return result.value; };
    const value = parent === null
      ? await (__trbOrmAdapter === "sqlite" ? database().begin("immediate", run) : database().begin(run))
      : await (parent.sql as TransactionSQL).savepoint(run);
    return resultOk(value);
  } catch (error) { if (error instanceof TrbOrmRollback) return error.result; return databaseError<T>(error, "database transaction failed"); }
}

export async function batch(source: TrbOrmQuery, after: unknown, size: number): Promise<DbResult<any[]>> {
  const owner = model(source.model); const primary = owner.columns.find(column => column.primary); if (primary === undefined) return resultErr(DbErrorKind.InvalidData, "batch query requires a primary key");
  if (source.joins.length > 0 || source.orders.length > 0 || source.limit !== null || source.offset !== null || source.lock) return resultErr(DbErrorKind.InvalidData, "batch queries do not accept joins, order, limit, offset, or lock");
  let paged = source; if (after !== null) paged = where(paged, [primary.name], [">"], [after]); paged = order(paged, [primary.name], ["asc"]); paged = limit(paged, size); return all(paged);
}
`
