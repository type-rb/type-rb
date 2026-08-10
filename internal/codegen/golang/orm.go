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
		if _, modelReceiver := g.orm.Model(identifier.Name); modelReceiver {
			modelName = identifier.Name
		}
	}
	model, exists := g.orm.Model(modelName)
	if !exists {
		model, exists = g.orm.ScopeModel(modelName)
	}
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMWhere(model) + "(" + g.ormPredicateArguments(call) + ")"
}

func (g *generator) ormInitialQuery(call *ir.Call) (ormintegration.Model, string, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ormintegration.Model{}, "", false
	}
	modelName := member.Receiver.ExprType().Name
	if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
		modelName = identifier.Name
	}
	model, exists := g.orm.Model(modelName)
	if !exists {
		return ormintegration.Model{}, "", false
	}
	query := g.ormModelQualifier(model) + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	return model, query, true
}

func (g *generator) ormClassOrder(call *ir.Call) string {
	model, query, ok := g.ormInitialQuery(call)
	if !ok {
		return "nil"
	}
	return g.ormOrderExpression(model, query, call)
}

func (g *generator) ormClassInteger(call *ir.Call, operation func(ormintegration.Model) string) string {
	model, query, ok := g.ormInitialQuery(call)
	if !ok || len(call.Arguments) != 1 {
		return "nil"
	}
	return g.ormModelQualifier(model) + operation(model) + "(" + query + ", " + g.expr(call.Arguments[0].Value) + ")"
}

func (g *generator) ormClassTerminal(call *ir.Call, operation func(ormintegration.Model) string) string {
	model, query, ok := g.ormInitialQuery(call)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + operation(model) + "(" + query + ")"
}

func (g *generator) ormDistinct(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	modelName := member.Receiver.ExprType().Name
	model, queryReceiver := g.orm.QueryModel(modelName)
	query := ""
	if queryReceiver {
		query = g.expr(member.Receiver)
	} else if scopedModel, scope := g.orm.ScopeModel(modelName); scope {
		model = scopedModel
		query = g.expr(member.Receiver)
	} else {
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			modelName = identifier.Name
		}
		var exists bool
		model, exists = g.orm.Model(modelName)
		if !exists {
			return "nil"
		}
		query = g.ormModelQualifier(model) + goORMWhere(model) + "([]string{}, []string{}, []any{})"
	}
	return g.ormModelQualifier(model) + goORMDistinct(model) + "(" + query + ")"
}

func (g *generator) ormUsing(call *ir.Call, arguments []string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(arguments) != 1 {
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
	return g.ormModelQualifier(model) + goORMUsing(model) + "(" + arguments[0] + ")"
}

func (g *generator) ormNot(call *ir.Call) string {
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
	return g.ormModelQualifier(model) + goORMNot(model) + "(" + g.ormPredicateArguments(call) + ")"
}

func (g *generator) ormFindBy(call *ir.Call) string {
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
	qualifier := g.ormModelQualifier(model)
	query := qualifier + goORMWhere(model) + "(" + g.ormPredicateArguments(call) + ")"
	return qualifier + goORMFirst(model) + "(" + query + ")"
}

func (g *generator) ormExists(call *ir.Call) string {
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
	qualifier := g.ormModelQualifier(model)
	query := qualifier + goORMWhere(model) + "(" + g.ormPredicateArguments(call) + ")"
	return qualifier + goORMExists(model) + "(" + query + ")"
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

func (g *generator) ormCreate(call *ir.Call) string {
	model, columns, values, ok := g.ormModelWriteArguments(call)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMCreate(model) + "([]string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormScopeFind(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormScopeModel(call)
	if !ok || len(call.Arguments) != 1 {
		return "nil"
	}
	primaryKey, exists := model.PrimaryKey()
	if !exists {
		return "nil"
	}
	qualifier := g.ormModelQualifier(model)
	filtered := qualifier + goORMQueryWhere(model) + "(" + query + ", []string{" + strconv.Quote(primaryKey.Name) + "}, []string{\"=\"}, []any{" + g.expr(call.Arguments[0].Value) + "})"
	return qualifier + goORMFirst(model) + "(" + filtered + ")"
}

func (g *generator) ormScopeBuild(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormScopeModel(call)
	if !ok {
		return "nil"
	}
	_, columns, values, valuesOK := g.ormModelWriteArguments(call)
	if !valuesOK {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMBuildScoped(model) + "(" + query + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormScopeCreate(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormScopeModel(call)
	if !ok {
		return "nil"
	}
	_, columns, values, valuesOK := g.ormModelWriteArguments(call)
	if !valuesOK {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMCreateScoped(model) + "(" + query + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormScopeModel(call *ir.Call) (ormintegration.Model, string, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ormintegration.Model{}, "", false
	}
	model, ok := g.orm.ScopeModel(member.Receiver.ExprType().Name)
	if !ok {
		return ormintegration.Model{}, "", false
	}
	return model, g.expr(member.Receiver), true
}

func (g *generator) ormBuild(call *ir.Call) string {
	model, columns, values, ok := g.ormModelWriteArguments(call)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMBuild(model) + "([]string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormModelWriteArguments(call *ir.Call) (ormintegration.Model, []string, []string, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ormintegration.Model{}, nil, nil, false
	}
	modelName := member.Receiver.ExprType().Name
	if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
		if _, modelReceiver := g.orm.Model(identifier.Name); modelReceiver {
			modelName = identifier.Name
		}
	}
	model, exists := g.orm.Model(modelName)
	if !exists {
		model, exists = g.orm.ScopeModel(modelName)
	}
	if !exists {
		return ormintegration.Model{}, nil, nil, false
	}
	columns := make([]string, 0, len(call.Arguments))
	values := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			continue
		}
		columns = append(columns, strconv.Quote(argument.Name))
		values = append(values, g.expr(argument.Value))
	}
	return model, columns, values, true
}

func (g *generator) ormDraftSave(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.DraftModel(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMDraftSave(model) + "(" + g.expr(member.Receiver) + ")"
}

func (g *generator) ormInsertAll(call *ir.Call) string {
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
	return g.ormModelQualifier(model) + goORMInsertAll(model) + "(" + g.expr(call.Arguments[0].Value) + ")"
}

func (g *generator) ormInsertIfAbsent(call *ir.Call) string {
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
	draft, uniqueBy := "nil", "nil"
	for _, argument := range call.Arguments {
		switch argument.Name {
		case "unique_by":
			uniqueBy = g.expr(argument.Value)
		case "":
			if draft == "nil" {
				draft = g.expr(argument.Value)
			}
		}
	}
	return g.ormModelQualifier(model) + goORMInsertIfAbsent(model) + "(" + draft + ", " + uniqueBy + ")"
}

func (g *generator) ormUpsert(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.DraftModel(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	uniqueBy, update := "nil", "nil"
	for _, argument := range call.Arguments {
		switch argument.Name {
		case "unique_by":
			uniqueBy = g.expr(argument.Value)
		case "update":
			update = g.expr(argument.Value)
		}
	}
	return g.ormModelQualifier(model) + goORMUpsert(model) + "(" + g.expr(member.Receiver) + ", " + uniqueBy + ", " + update + ")"
}

func (g *generator) ormUpsertAll(call *ir.Call) string {
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
	drafts, uniqueBy, update := "nil", "nil", "nil"
	for _, argument := range call.Arguments {
		switch argument.Name {
		case "unique_by":
			uniqueBy = g.expr(argument.Value)
		case "update":
			update = g.expr(argument.Value)
		case "":
			if drafts == "nil" {
				drafts = g.expr(argument.Value)
			}
		}
	}
	return g.ormModelQualifier(model) + goORMUpsertAll(model) + "(" + drafts + ", " + uniqueBy + ", " + update + ")"
}

func (g *generator) ormUpdate(call *ir.Call) string {
	model, receiver, columns, values, ok := g.ormModelChangeArguments(call)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMUpdate(model) + "(" + receiver + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormWith(call *ir.Call) string {
	model, receiver, columns, values, ok := g.ormModelChangeArguments(call)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMWith(model) + "(" + receiver + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormModelChangeArguments(call *ir.Call) (ormintegration.Model, string, []string, []string, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ormintegration.Model{}, "", nil, nil, false
	}
	model, exists := g.orm.Model(member.Receiver.ExprType().Name)
	if !exists {
		return ormintegration.Model{}, "", nil, nil, false
	}
	columns := make([]string, 0, len(call.Arguments))
	values := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			continue
		}
		columns = append(columns, strconv.Quote(argument.Name))
		values = append(values, g.expr(argument.Value))
	}
	return model, g.expr(member.Receiver), columns, values, true
}

func (g *generator) ormChangesSave(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.ChangesModel(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMChangesSave(model) + "(" + g.expr(member.Receiver) + ")"
}

func (g *generator) ormDelete(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.Model(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMDelete(model) + "(" + g.expr(member.Receiver) + ")"
}

func (g *generator) ormDestroy(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	model, exists := g.orm.Model(member.Receiver.ExprType().Name)
	if !exists {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMDestroy(model) + "(" + g.expr(member.Receiver) + ")"
}

func (g *generator) ormPredicateArguments(call *ir.Call) string {
	predicates := ormintegration.Predicates(call)
	columns := make([]string, len(predicates))
	operators := make([]string, len(predicates))
	values := make([]string, len(predicates))
	for index, predicate := range predicates {
		columns[index] = strconv.Quote(predicate.Column)
		operators[index] = strconv.Quote(string(predicate.Operator))
		if bounds, ok := predicate.Value.(*ir.Range); ok {
			values[index] = "trbOrmRange{start: " + g.expr(bounds.Start) + ", end: " + g.expr(bounds.End) + "}"
		} else {
			values[index] = g.expr(predicate.Value)
		}
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
		model, exists = g.orm.ScopeModel(member.Receiver.ExprType().Name)
	}
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

func (g *generator) ormQueryNot(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMQueryNot(model) + "(" + query + ", " + g.ormPredicateArguments(call) + ")"
}

func (g *generator) ormQueryOr(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok || len(arguments) != 2 {
		return "nil"
	}
	return g.ormModelQualifier(model) + goORMQueryOr(model) + "(" + query + ", " + arguments[1] + ")"
}

func (g *generator) ormQueryFindBy(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	qualifier := g.ormModelQualifier(model)
	filtered := qualifier + goORMQueryWhere(model) + "(" + query + ", " + g.ormPredicateArguments(call) + ")"
	return qualifier + goORMFirst(model) + "(" + filtered + ")"
}

func (g *generator) ormQueryUpdateAll(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	columns := make([]string, 0, len(call.Arguments))
	values := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name != "" {
			columns = append(columns, strconv.Quote(argument.Name))
			values = append(values, g.expr(argument.Value))
		}
	}
	return g.ormModelQualifier(model) + goORMUpdateAll(model) + "(" + query + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormClassUpdateAll(call *ir.Call) string {
	model, query, ok := g.ormInitialQuery(call)
	if !ok {
		return "nil"
	}
	columns := make([]string, 0, len(call.Arguments))
	values := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name != "" {
			columns = append(columns, strconv.Quote(argument.Name))
			values = append(values, g.expr(argument.Value))
		}
	}
	return g.ormModelQualifier(model) + goORMUpdateAll(model) + "(" + query + ", []string{" + strings.Join(columns, ", ") + "}, []any{" + strings.Join(values, ", ") + "})"
}

func (g *generator) ormOrder(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	return g.ormOrderExpression(model, query, call)
}

func (g *generator) ormOrderExpression(model ormintegration.Model, query string, call *ir.Call) string {
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

func (g *generator) ormLifecycleAlias() string {
	alias := g.ormPackageAlias()
	if alias == "orm" {
		g.requireImport(pathpkg.Join(g.goModule, "trb/orm"), alias)
	}
	return alias
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
	if association.Through != "" {
		return g.ormThroughAssociationQuery(member, source, association)
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return "nil"
	}
	qualifier := g.ormModelQualifier(target)
	value := g.expr(member.Receiver) + "." + goORMColumnGetter(association.SourceColumn) + "()"
	scope := qualifier + goORMUsing(target) + "(" + g.expr(member.Receiver) + ".TrbOrmTransaction())"
	query := qualifier + goORMQueryWhere(target) + "(" + scope + ", []string{" + strconv.Quote(association.TargetColumn) + "}, []string{\"=\"}, []any{" + value + "})"
	return g.ormAssociationScope(association, target, query)
}

func (g *generator) ormThroughAssociationQuery(member *ir.Member, source ormintegration.Model, association ormintegration.Association) string {
	through, ok := source.Association(association.Through)
	if !ok {
		return "nil"
	}
	middle, ok := g.orm.Model(through.TargetModel)
	if !ok {
		return "nil"
	}
	via, ok := middle.Association(association.Source)
	if !ok {
		return "nil"
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return "nil"
	}
	receiver := g.expr(member.Receiver)
	transaction := receiver + ".TrbOrmTransaction()"
	middleQualifier := g.ormModelQualifier(middle)
	middleScope := middleQualifier + goORMUsing(middle) + "(" + transaction + ")"
	middleQuery := middleQualifier + goORMQueryWhere(middle) + "(" + middleScope + ", []string{" + strconv.Quote(through.TargetColumn) + "}, []string{\"=\"}, []any{" + receiver + "." + goORMColumnGetter(through.SourceColumn) + "()})"
	predicate := middleQualifier + goORMAssociationPredicate(middle) + "(" + middleQuery + ")"
	join := g.ormLifecycleAlias() + ".TrbOrmJoin{" +
		"Kind: \"INNER JOIN\", Table: " + strconv.Quote(middle.Table) +
		", SourceColumn: " + strconv.Quote(via.TargetColumn) +
		", TargetColumn: " + strconv.Quote(via.SourceColumn) +
		", Predicate: " + predicate + "}"
	targetQualifier := g.ormModelQualifier(target)
	targetScope := targetQualifier + goORMUsing(target) + "(" + transaction + ")"
	query := targetQualifier + goORMJoin(target) + "(" + targetScope + ", " + join + ")"
	return g.ormAssociationScope(association, target, query)
}

func (g *generator) ormAssociationScope(association ormintegration.Association, target ormintegration.Model, query string) string {
	if association.Scope == nil || len(association.Scope.Parameters) != 1 || len(association.Scope.Body) != 1 {
		return query
	}
	result, ok := association.Scope.Body[0].(*ir.ExpressionStatement)
	if !ok {
		return query
	}
	parameter := goBindingIdentifier(association.Scope.Parameters[0])
	queryType := g.ormModelQualifier(target) + goORMQueryType(target)
	return "func(" + parameter + " " + queryType + ") " + queryType + " { return " + g.expr(result.Expression) + " }(" + query + ")"
}

func (g *generator) ormPreload(call *ir.Call, arguments []string) string {
	model, query, ok := g.ormQueryModel(call, arguments)
	if !ok {
		return "nil"
	}
	return g.ormPreloadExpression(call, model, query)
}

func (g *generator) ormClassPreload(call *ir.Call) string {
	model, query, ok := g.ormInitialQuery(call)
	if !ok {
		return "nil"
	}
	return g.ormPreloadExpression(call, model, query)
}

func (g *generator) ormPreloadExpression(call *ir.Call, model ormintegration.Model, query string) string {
	if len(call.Arguments) == 0 {
		return "nil"
	}
	associationName, ok := ormJoinAssociation(call.Arguments[0].Value)
	if !ok {
		return "nil"
	}
	association, ok := model.Association(associationName)
	if !ok || !association.Preloadable {
		return "nil"
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return "nil"
	}
	targetQuery := g.ormModelQualifier(target) + goORMWhere(target) + "([]string{}, []string{}, []any{})"
	if len(call.Arguments) > 1 {
		targetQuery = g.expr(call.Arguments[1].Value)
	}
	targetQuery = g.ormAssociationScope(association, target, targetQuery)
	return g.ormModelQualifier(model) + goORMTypedPreload(model, association) + "(" + query + ", " + targetQuery + ")"
}

func (g *generator) ormAssociationDefinition(call *ir.Call) (*ir.Member, ormintegration.Model, ormintegration.Association, ormintegration.Model, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return nil, ormintegration.Model{}, ormintegration.Association{}, ormintegration.Model{}, false
	}
	source, ok := g.orm.Model(member.Receiver.ExprType().Name)
	if !ok {
		return nil, ormintegration.Model{}, ormintegration.Association{}, ormintegration.Model{}, false
	}
	association, ok := source.Association(member.Name)
	if !ok {
		return nil, ormintegration.Model{}, ormintegration.Association{}, ormintegration.Model{}, false
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return nil, ormintegration.Model{}, ormintegration.Association{}, ormintegration.Model{}, false
	}
	return member, source, association, target, true
}

func (g *generator) ormLoadAssociation(call *ir.Call, reload bool) string {
	member, source, association, target, ok := g.ormAssociationDefinition(call)
	if !ok || len(call.ExprType().Args) == 0 {
		return "nil"
	}
	valueType := call.ExprType().Args[0]
	resultType := g.goType(call.ExprType())
	receiver := g.expr(member.Receiver)
	getter := receiver + "." + goORMAssociationGetter(association.Name) + "()"
	setter := receiver + "." + goORMAssociationSetter(association.Name)
	query := g.ormAssociationQuery(call)
	qualifier := g.ormModelQualifier(target)
	load := qualifier + goORMLoader(target) + "(" + query + ")"
	loadedValue := "loaded.OkValue"
	if association.Kind == ormintegration.BelongsTo {
		load = qualifier + goORMFirst(target) + "(" + query + ")"
	}

	var result strings.Builder
	result.WriteString("func() " + resultType + " { ")
	if !reload {
		result.WriteString("if cached, ok := " + getter + "; ok { value, valid := cached.(" + g.goType(valueType) + "); if !valid { return " + g.ormResultErr(valueType, g.ormErrorValue("InvalidData", "cached ORM association "+source.Name+"."+association.Name+" has an invalid type")) + " }; return " + g.ormResultOK(valueType, "value") + " }; ")
	}
	result.WriteString("loaded := " + load + "; if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(valueType, "loaded.ErrError") + " }; ")
	if association.Kind == ormintegration.HasOne {
		result.WriteString("if len(loaded.OkValue) > 1 { return " + g.ormResultErr(valueType, g.ormErrorValue("InvalidData", "database has_one association returned multiple rows")) + " }; var value " + g.goType(valueType) + "; if len(loaded.OkValue) == 1 { value = loaded.OkValue[0] }; ")
	} else {
		result.WriteString("value := " + loadedValue + "; ")
	}
	result.WriteString(setter + "(value); return " + g.ormResultOK(valueType, "value") + " }()")
	return result.String()
}

func (g *generator) ormAssociationLoaded(call *ir.Call) string {
	member, _, association, _, ok := g.ormAssociationDefinition(call)
	if !ok {
		return "false"
	}
	return "func() bool { _, loaded := " + g.expr(member.Receiver) + "." + goORMAssociationGetter(association.Name) + "(); return loaded }()"
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
	sourceReturn := iteration.Result != nil && iteration.Result.Return
	fallible := iteration.Fails.Kind != "" && iteration.Fails.Kind != types.Never
	propagateEffect := fallible
	captureEffect := iteration.CaptureEffect
	returnResult := sourceReturn || captureEffect
	needsFailureFlag := !propagateEffect && !returnResult
	breakTarget := ""
	if needsFailureFlag || ormBatchBodyBreaks(iteration.Body) {
		breakTarget = label
	}
	if captureEffect && iteration.Result != nil {
		resultType := g.goType(iteration.Result.Type)
		switch {
		case iteration.Result.Variable != nil:
			resultTarget = goBindingIdentifier(iteration.Result.Variable.Name)
			g.line(resultTarget + " := func() " + resultType + " {")
		case iteration.Result.Target != nil:
			resultTarget = g.assignmentTarget(iteration.Result.Target)
			g.line(resultTarget + " = func() " + resultType + " {")
		case iteration.Result.Return:
			g.line("return func() " + resultType + " {")
		}
		g.indent++
	} else if iteration.Result != nil && iteration.Result.Variable != nil {
		resultTarget = goBindingIdentifier(iteration.Result.Variable.Name)
		g.line("var " + resultTarget + " " + g.goType(iteration.Result.Type))
	} else if iteration.Result != nil && iteration.Result.Target != nil {
		resultTarget = g.assignmentTarget(iteration.Result.Target)
	}
	integerType := types.FromName("Integer")
	resultValueType := integerType
	if propagateEffect && iteration.EffectSuccess.Kind != "" {
		resultValueType = iteration.EffectSuccess
	}
	success := func(value string) string { return g.ormResultOK(resultValueType, value) }
	failure := func(value string) string { return g.ormResultErr(resultValueType, value) }
	assignFailure := func(value string) {
		if propagateEffect || returnResult {
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
	if needsFailureFlag {
		g.line(failed + " := false")
	}
	g.line("if " + size + " <= 0 {")
	g.indent++
	assignFailure(failure(g.ormErrorValue("InvalidData", "batch size must be greater than zero")))
	if needsFailureFlag {
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
	assignFailure(failure(loaded + ".ErrError"))
	if needsFailureFlag {
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
		if propagateEffect {
			g.line(resultTarget + " = " + processed)
		} else {
			g.line("if !" + failed + " { " + resultTarget + " = " + success(processed) + " }")
		}
	}
	g.indent--
	g.line("}")
	if captureEffect && iteration.Result != nil {
		g.indent--
		g.line("}()")
	}
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
	adapter, err := ormintegration.AdapterFor(manifest.Adapter)
	if err != nil {
		return
	}
	if g.modulePath == "trb/orm/index" {
		g.ormPoolRuntime(manifest, adapter)
		return
	}
	models := manifest.ModelsForModule(g.modulePath)
	if len(models) == 0 {
		return
	}
	g.requireImport("database/sql/driver", "driver")
	g.requireImport("context", "")
	g.requireImport("database/sql", "sql")
	g.requireImport("errors", "")
	g.requireImport("net", "")
	g.requireImport("reflect", "")
	g.requireImport("strings", "")
	if adapter.NumberedBinds {
		g.requireImport("strconv", "")
	}
	g.line("type trbOrmRange struct { start any; end any }")
	g.b.WriteByte('\n')
	g.line("type trbOrmExecutor interface {")
	g.indent++
	g.line("Exec(string, ...any) (sql.Result, error)")
	g.line("Query(string, ...any) (*sql.Rows, error)")
	g.line("QueryRow(string, ...any) *sql.Row")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbOrmExecutorForTransaction(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction) (trbOrmExecutor, *" + g.goType(types.FromName("DbError")) + ") {")
	g.indent++
	g.line("if transaction != nil { return transaction, nil }")
	g.line("database, err := " + g.ormPackageAlias() + ".TrbOrmDatabase()")
	g.line("if err != nil { value := trbOrmError(err, " + g.ormErrorKind("Connection") + ", \"database connection failed\"); return nil, &value }")
	g.line("return database, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func trbOrmExecutorForQuery(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction, lock bool) (trbOrmExecutor, *" + g.goType(types.FromName("DbError")) + ") {")
	g.indent++
	g.line("if lock && transaction == nil { value := " + g.ormErrorValue("InvalidData", "database lock requires an explicit transaction scope") + "; return nil, &value }")
	g.line("return trbOrmExecutorForTransaction(transaction)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.ormDialectRuntime(adapter)
	for _, model := range models {
		g.ormModelRuntime(manifest, adapter, model)
	}
}

func (g *generator) ormPoolRuntime(manifest *ormintegration.Manifest, adapter ormintegration.Adapter) {
	g.requireImport("database/sql", "sql")
	g.requireImport(adapter.GoDriverImport, "_")
	g.requireImport("context", "")
	g.requireImport("strconv", "")
	g.requireImport("sync", "")
	if manifest.DatabaseEnvironment != "" {
		g.requireImport("errors", "")
		g.requireImport("os", "")
	}
	g.line("var trbOrmDatabaseOnce sync.Once")
	g.line("var trbOrmDatabase *sql.DB")
	g.line("var trbOrmDatabaseError error")
	g.b.WriteByte('\n')
	g.line("func TrbOrmDatabase() (*sql.DB, error) {")
	g.indent++
	g.line("trbOrmDatabaseOnce.Do(func() {")
	g.indent++
	database := strconv.Quote(manifest.Database)
	if manifest.DatabaseEnvironment != "" {
		g.line("databaseSource, found := os.LookupEnv(" + strconv.Quote(manifest.DatabaseEnvironment) + ")")
		g.line("if !found || databaseSource == \"\" { trbOrmDatabaseError = errors.New(\"database environment variable is not set or empty\"); return }")
		database = "databaseSource"
	}
	g.line("trbOrmDatabase, trbOrmDatabaseError = sql.Open(" + strconv.Quote(adapter.DriverName) + ", " + database + ")")
	g.line("if trbOrmDatabaseError == nil { trbOrmDatabaseError = trbOrmDatabase.Ping() }")
	g.line("if trbOrmDatabaseError != nil && trbOrmDatabase != nil { _ = trbOrmDatabase.Close(); trbOrmDatabase = nil }")
	g.indent--
	g.line("})")
	g.line("return trbOrmDatabase, trbOrmDatabaseError")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func TrbOrmCloseDatabase() error {")
	g.indent++
	g.line("if trbOrmDatabase == nil { return nil }")
	g.line("return trbOrmDatabase.Close()")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type TrbOrmTransaction struct {")
	g.indent++
	g.line("transaction *sql.Tx")
	g.line("connection *sql.Conn")
	g.line("parent *TrbOrmTransaction")
	g.line("savepoint string")
	g.line("closed bool")
	g.line("nextSavepoint int")
	g.line("mutex sync.Mutex")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type TrbOrmAssociationPredicate func(arguments *[]any) string")
	g.b.WriteByte('\n')
	g.line("type TrbOrmExistsPredicate func(arguments *[]any) string")
	g.b.WriteByte('\n')
	g.line("type TrbOrmJoin struct {")
	g.indent++
	g.line("Kind string")
	g.line("Table string")
	g.line("SourceColumn string")
	g.line("TargetColumn string")
	g.line("Predicate TrbOrmAssociationPredicate")
	g.line("Build TrbOrmAssociationPredicate")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type TrbOrmSubqueryValue interface {")
	g.indent++
	g.line("TrbOrmBuild(arguments *[]any) string")
	g.line("TrbOrmTransactionScope() *TrbOrmTransaction")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("type TrbOrmSubquery[T any] struct {")
	g.indent++
	g.line("transaction *TrbOrmTransaction")
	g.line("build func(arguments *[]any) string")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func NewTrbOrmSubquery[T any](transaction *TrbOrmTransaction, build func(arguments *[]any) string) *TrbOrmSubquery[T] { return &TrbOrmSubquery[T]{transaction: transaction, build: build} }")
	g.line("func (subquery *TrbOrmSubquery[T]) TrbOrmBuild(arguments *[]any) string { return subquery.build(arguments) }")
	g.line("func (subquery *TrbOrmSubquery[T]) TrbOrmTransactionScope() *TrbOrmTransaction { return subquery.transaction }")
	g.b.WriteByte('\n')
	g.line("func TrbOrmBeginTransaction() (*TrbOrmTransaction, *DbError) {")
	g.indent++
	g.line("database, err := TrbOrmDatabase()")
	g.line("if err != nil { value := DbError{Kind: DbErrorKindConnection, Message: \"database connection failed\"}; return nil, &value }")
	if adapter.Name == "sqlite" {
		g.line("connection, err := database.Conn(context.Background())")
		g.line("if err != nil { value := DbError{Kind: DbErrorKindConnection, Message: \"database connection failed\"}; return nil, &value }")
		g.line("if _, err := connection.ExecContext(context.Background(), \"BEGIN IMMEDIATE\"); err != nil { _ = connection.Close(); value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to begin\"}; return nil, &value }")
		g.line("return &TrbOrmTransaction{connection: connection}, nil")
	} else {
		g.line("transaction, err := database.Begin()")
		g.line("if err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to begin\"}; return nil, &value }")
		g.line("return &TrbOrmTransaction{transaction: transaction}, nil")
	}
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func TrbOrmBeginNestedTransaction(parent *TrbOrmTransaction) (*TrbOrmTransaction, *DbError) {")
	g.indent++
	g.line("if !parent.active() { value := DbError{Kind: " + goConstantIdentifier("DbErrorKind", "InvalidData") + ", Message: \"database transaction is closed\"}; return nil, &value }")
	g.line("root := parent.root()")
	g.line("root.mutex.Lock()")
	g.line("root.nextSavepoint++")
	g.line("savepoint := \"trb_savepoint_\" + strconv.Itoa(root.nextSavepoint)")
	g.line("root.mutex.Unlock()")
	g.line("if _, err := parent.Exec(\"SAVEPOINT \" + savepoint); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database savepoint failed to begin\"}; return nil, &value }")
	g.line("return &TrbOrmTransaction{transaction: parent.transaction, connection: parent.connection, parent: parent, savepoint: savepoint}, nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (transaction *TrbOrmTransaction) root() *TrbOrmTransaction {")
	g.indent++
	g.line("for transaction != nil && transaction.parent != nil { transaction = transaction.parent }")
	g.line("return transaction")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (transaction *TrbOrmTransaction) active() bool {")
	g.indent++
	g.line("if transaction == nil || transaction.transaction == nil && transaction.connection == nil || transaction.closed { return false }")
	g.line("for parent := transaction.parent; parent != nil; parent = parent.parent { if parent.closed { return false } }")
	g.line("return true")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (transaction *TrbOrmTransaction) Commit() *DbError {")
	g.indent++
	g.line("if !transaction.active() { value := DbError{Kind: " + goConstantIdentifier("DbErrorKind", "InvalidData") + ", Message: \"database transaction is closed\"}; return &value }")
	g.line("if transaction.savepoint != \"\" { if _, err := transaction.Exec(\"RELEASE SAVEPOINT \" + transaction.savepoint); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database savepoint failed to commit\"}; return &value }; transaction.closed = true; return nil }")
	g.line("if transaction.connection != nil { if _, err := transaction.connection.ExecContext(context.Background(), \"COMMIT\"); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to commit\"}; return &value }; transaction.closed = true; _ = transaction.connection.Close(); return nil }")
	g.line("if err := transaction.transaction.Commit(); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to commit\"}; return &value }")
	g.line("transaction.closed = true")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (transaction *TrbOrmTransaction) Rollback() *DbError {")
	g.indent++
	g.line("if transaction == nil || transaction.transaction == nil && transaction.connection == nil || transaction.closed { return nil }")
	g.line("transaction.closed = true")
	g.line("if transaction.savepoint != \"\" {")
	g.indent++
	g.line("if _, err := transaction.Exec(\"ROLLBACK TO SAVEPOINT \" + transaction.savepoint); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database savepoint failed to roll back\"}; return &value }")
	g.line("if _, err := transaction.Exec(\"RELEASE SAVEPOINT \" + transaction.savepoint); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database savepoint failed to release\"}; return &value }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.line("if transaction.connection != nil { _, err := transaction.connection.ExecContext(context.Background(), \"ROLLBACK\"); _ = transaction.connection.Close(); if err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to roll back\"}; return &value }; return nil }")
	g.line("if err := transaction.transaction.Rollback(); err != nil { value := DbError{Kind: DbErrorKindQuery, Message: \"database transaction failed to roll back\"}; return &value }")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func (transaction *TrbOrmTransaction) Exec(statement string, arguments ...any) (sql.Result, error) { if transaction.connection != nil { return transaction.connection.ExecContext(context.Background(), statement, arguments...) }; return transaction.transaction.Exec(statement, arguments...) }")
	g.line("func (transaction *TrbOrmTransaction) Query(statement string, arguments ...any) (*sql.Rows, error) { if transaction.connection != nil { return transaction.connection.QueryContext(context.Background(), statement, arguments...) }; return transaction.transaction.Query(statement, arguments...) }")
	g.line("func (transaction *TrbOrmTransaction) QueryRow(statement string, arguments ...any) *sql.Row { if transaction.connection != nil { return transaction.connection.QueryRowContext(context.Background(), statement, arguments...) }; return transaction.transaction.QueryRow(statement, arguments...) }")
	g.b.WriteByte('\n')
}

func (g *generator) structuredBlock(block *ir.StructuredBlock) {
	if block.Intrinsic != "trb.orm.transaction" || block.Result == nil {
		return
	}
	g.temporary++
	id := strconv.Itoa(g.temporary)
	transaction := "__trbTransaction" + id
	transactionError := "__trbTransactionError" + id
	committed := "__trbTransactionCommitted" + id
	result := "__trbTransactionResult" + id
	raw := "__trbTransactionEffect" + id
	valueType := block.EffectSuccess
	if valueType.Kind == types.Void {
		valueType = types.FromName("Unit")
	}
	rawType := block.Call.ExprType()
	fallible := block.Fails.Kind != "" && block.Fails.Kind != types.Never
	if !fallible {
		rawType = block.Result.Type
	}
	target := ""
	if block.Result.Variable != nil {
		target = goBindingIdentifier(block.Result.Variable.Name)
	} else if block.Result.Target != nil {
		target = g.assignmentTarget(block.Result.Target)
	}
	if target == "" && !block.Result.Return {
		return
	}

	prefix := ""
	sourceResult := fallible && !block.CaptureEffect
	if sourceResult && !block.Result.Return {
		prefix = raw + " := "
	} else if block.Result.Variable != nil {
		prefix = target + " := "
	} else if block.Result.Target != nil {
		prefix = target + " = "
	} else if block.Result.Return {
		prefix = "return "
	}
	g.line(prefix + "func() " + g.goType(rawType) + " {")
	g.indent++
	begin := g.ormLifecycleAlias() + ".TrbOrmBeginTransaction()"
	if member, ok := block.Call.Callee.(*ir.Member); ok && member.Receiver.ExprType().Name == "Transaction" {
		begin = g.ormLifecycleAlias() + ".TrbOrmBeginNestedTransaction(" + g.expr(member.Receiver) + ")"
	}
	g.line(transaction + ", " + transactionError + " := " + begin)
	if fallible {
		g.line("if " + transactionError + " != nil { return " + g.ormResultErr(valueType, "*"+transactionError) + " }")
	}
	g.line(committed + " := false")
	g.line("defer func() { if !" + committed + " { _ = " + transaction + ".Rollback() } }()")
	if len(block.Bindings) > 0 && block.Bindings[0].Name != "_" {
		binding := goBindingIdentifier(block.Bindings[0].Name)
		g.line(binding + " := " + transaction)
		g.line("_ = " + binding)
	}
	g.statements(block.Body)
	g.line(result + " := " + g.expr(block.Value))
	if fallible {
		g.line("if " + transactionError + " := " + transaction + ".Commit(); " + transactionError + " != nil { return " + g.ormResultErr(valueType, "*"+transactionError) + " }")
	} else {
		g.line("if " + transactionError + " := " + transaction + ".Commit(); " + transactionError + " != nil { panic(" + transactionError + ".Message) }")
	}
	g.line(committed + " = true")
	if fallible {
		g.line("return " + g.ormResultOK(valueType, result))
	} else {
		g.line("return " + result)
	}
	g.indent--
	g.line("}()")

	if !sourceResult || block.Result.Return {
		return
	}
	outerSuccess := block.PropagateSuccess
	if outerSuccess.Kind == "" {
		outerSuccess = valueType
	}
	g.line("if " + raw + ".Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return " + g.ormResultErr(outerSuccess, raw+".ErrError") + " }")
	if block.Result.Variable != nil {
		g.line(target + " := " + raw + ".OkValue")
	} else {
		g.line(target + " = " + raw + ".OkValue")
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
	g.requireImport("strconv", "")
	conditionType := goORMConditionType(model)
	predicateType := goORMPredicateType(model)
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
	g.line("type " + predicateType + " struct {")
	g.indent++
	g.line("kind string")
	g.line("condition " + conditionType)
	g.line("children []" + predicateType)
	g.line("exists " + g.ormLifecycleAlias() + ".TrbOrmExistsPredicate")
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
	preloadType := goORMPreloadType(model)
	g.line("type " + preloadType + " struct {")
	g.indent++
	g.line("name string")
	g.line("load func(*" + g.ormLifecycleAlias() + ".TrbOrmTransaction, []*" + goIdentifier(model.Name, true) + ") *" + g.goType(types.FromName("DbError")))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("type " + queryType + " struct {")
	g.indent++
	g.line("transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction")
	g.line("predicate *" + predicateType)
	g.line("orders []" + orderType)
	g.line("limit *int")
	g.line("offset *int")
	g.line("lock bool")
	g.line("distinct bool")
	g.line("preloads []" + preloadType)
	g.line("joins []" + g.ormLifecycleAlias() + ".TrbOrmJoin")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMUsing(model) + "(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction) " + queryType + " {")
	g.indent++
	g.line("return " + queryType + "{transaction: transaction}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMPredicateGroup(model) + "(columns []string, operators []string, values []any) *" + predicateType + " {")
	g.indent++
	g.line("if len(columns) == 0 { return nil }")
	g.line("children := make([]" + predicateType + ", 0, len(columns))")
	g.line("for index, column := range columns {")
	g.indent++
	g.line("children = append(children, " + predicateType + "{kind: \"atom\", condition: " + conditionType + "{column: column, operator: operators[index], value: values[index]}})")
	g.indent--
	g.line("}")
	g.line("if len(children) == 1 { return &children[0] }")
	g.line("return &" + predicateType + "{kind: \"and\", children: children}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMCombinePredicates(model) + "(kind string, left *" + predicateType + ", right *" + predicateType + ") *" + predicateType + " {")
	g.indent++
	g.line("if left == nil { return right }")
	g.line("if right == nil { return left }")
	g.line("return &" + predicateType + "{kind: kind, children: []" + predicateType + "{*left, *right}}")
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
	g.line(goORMValidateSubqueries(model) + "(query, values)")
	g.line("result := query")
	g.line("result.predicate = " + goORMCombinePredicates(model) + "(\"and\", query.predicate, " + goORMPredicateGroup(model) + "(columns, operators, values))")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMNot(model) + "(columns []string, operators []string, values []any) " + queryType + " {")
	g.indent++
	g.line("return " + goORMQueryNot(model) + "(" + queryType + "{}, columns, operators, values)")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMQueryNot(model) + "(query " + queryType + ", columns []string, operators []string, values []any) " + queryType + " {")
	g.indent++
	g.line(goORMValidateSubqueries(model) + "(query, values)")
	g.line("predicate := " + goORMPredicateGroup(model) + "(columns, operators, values)")
	g.line("if predicate == nil { panic(\"ORM not requires one condition\") }")
	g.line("negated := &" + predicateType + "{kind: \"not\", children: []" + predicateType + "{*predicate}}")
	g.line("result := query")
	g.line("result.predicate = " + goORMCombinePredicates(model) + "(\"and\", query.predicate, negated)")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMValidateSubqueries(model) + "(query " + queryType + ", values []any) {")
	g.indent++
	g.line("for _, value := range values {")
	g.indent++
	g.line("subquery, ok := value.(" + g.ormLifecycleAlias() + ".TrbOrmSubqueryValue)")
	g.line("if !ok { continue }")
	g.line("if transaction := subquery.TrbOrmTransactionScope(); transaction != nil && transaction != query.transaction { panic(\"ORM subquery transaction scope must match the base query\") }")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMQueryOr(model) + "(left " + queryType + ", right " + queryType + ") " + queryType + " {")
	g.indent++
	g.line("if left.predicate == nil || right.predicate == nil { panic(\"ORM or requires conditions on both queries\") }")
	g.line("if len(left.orders) > 0 || left.limit != nil || left.offset != nil || left.lock || left.distinct || len(left.preloads) > 0 || len(left.joins) > 0 || len(right.orders) > 0 || right.limit != nil || right.offset != nil || right.lock || right.distinct || len(right.preloads) > 0 || len(right.joins) > 0 { panic(\"ORM or requires unmodified predicate queries; apply distinct, joins, order, limit, offset, lock, and preload after or\") }")
	g.line("if left.transaction != right.transaction { panic(\"ORM or requires queries from the same transaction scope\") }")
	g.line("left.predicate = " + goORMCombinePredicates(model) + "(\"or\", left.predicate, right.predicate)")
	g.line("return left")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMPredicateContainsExists(model) + "(predicate *" + predicateType + ") bool {")
	g.indent++
	g.line("if predicate == nil { return false }")
	g.line("if predicate.kind == \"exists\" { return true }")
	g.line("for index := range predicate.children { if " + goORMPredicateContainsExists(model) + "(&predicate.children[index]) { return true } }")
	g.line("return false")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMJoin(model) + "(query " + queryType + ", join " + g.ormLifecycleAlias() + ".TrbOrmJoin) " + queryType + " {")
	g.indent++
	g.line("switch join.Kind { case \"INNER JOIN\", \"LEFT JOIN\": default: panic(\"unsupported ORM join kind\") }")
	g.line("result := query")
	g.line("result.joins = append([]" + g.ormLifecycleAlias() + ".TrbOrmJoin(nil), query.joins...)")
	g.line("result.joins = append(result.joins, join)")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMWhereExists(model) + "(query " + queryType + ", table string, sourceColumn string, targetColumn string, predicate " + g.ormLifecycleAlias() + ".TrbOrmAssociationPredicate, negated bool) " + queryType + " {")
	g.indent++
	g.line("exists := " + g.ormLifecycleAlias() + ".TrbOrmExistsPredicate(func(arguments *[]any) string {")
	g.indent++
	g.line("correlation := trbOrmQuoteIdentifier(table) + \".\" + trbOrmQuoteIdentifier(targetColumn) + \" = \" + trbOrmQuoteIdentifier(" + strconv.Quote(model.Table) + ") + \".\" + trbOrmQuoteIdentifier(sourceColumn)")
	g.line("statement := \"SELECT 1 FROM \" + trbOrmQuoteIdentifier(table) + \" WHERE \" + correlation")
	g.line("if predicate != nil { if clause := predicate(arguments); clause != \"\" { statement += \" AND (\" + clause + \")\" } }")
	g.line("operator := \"EXISTS\"; if negated { operator = \"NOT EXISTS\" }")
	g.line("return operator + \" (\" + statement + \")\"")
	g.indent--
	g.line("})")
	g.line("result := query")
	g.line("result.predicate = " + goORMCombinePredicates(model) + "(\"and\", query.predicate, &" + predicateType + "{kind: \"exists\", exists: exists})")
	g.line("return result")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMPredicateSQL(model) + "(predicate *" + predicateType + ", arguments *[]any) string {")
	g.indent++
	g.line("if predicate == nil { return \"\" }")
	g.line("switch predicate.kind {")
	g.line("case \"atom\":")
	g.indent++
	g.line("condition := predicate.condition")
	g.line("switch condition.operator { case \"=\", \"!=\", \"<\", \"<=\", \">\", \">=\", \"IN\", \"NOT_IN\", \"RANGE_INCLUSIVE\", \"RANGE_EXCLUSIVE\": default: panic(\"unsupported ORM comparison operator\") }")
	g.line("column := trbOrmQuoteIdentifier(condition.column)")
	g.line("if condition.operator == \"IN\" || condition.operator == \"NOT_IN\" {")
	g.indent++
	g.line("if subquery, ok := condition.value.(" + g.ormLifecycleAlias() + ".TrbOrmSubqueryValue); ok { operator := \" IN \"; if condition.operator == \"NOT_IN\" { operator = \" NOT IN \" }; return column + operator + \"(\" + subquery.TrbOrmBuild(arguments) + \")\" }")
	g.line("values := reflect.ValueOf(condition.value)")
	g.line("if values.Kind() != reflect.Array && values.Kind() != reflect.Slice { panic(\"ORM IN predicate requires an Array\") }")
	g.line("if values.Len() == 0 { return \"1 = 0\" }")
	g.line("placeholders := make([]string, values.Len())")
	g.line("for index := 0; index < values.Len(); index++ { placeholders[index] = trbOrmPlaceholder(len(*arguments)+1); *arguments = append(*arguments, values.Index(index).Interface()) }")
	g.line("return column + \" IN (\" + strings.Join(placeholders, \", \" ) + \")\"")
	g.indent--
	g.line("}")
	g.line("if condition.operator == \"RANGE_INCLUSIVE\" || condition.operator == \"RANGE_EXCLUSIVE\" {")
	g.indent++
	g.line("bounds, ok := condition.value.(trbOrmRange); if !ok { panic(\"ORM range predicate requires a Range\") }")
	g.line("lower := trbOrmPlaceholder(len(*arguments)+1); *arguments = append(*arguments, bounds.start)")
	g.line("upper := trbOrmPlaceholder(len(*arguments)+1); *arguments = append(*arguments, bounds.end)")
	g.line("upperOperator := \"<=\"; if condition.operator == \"RANGE_EXCLUSIVE\" { upperOperator = \"<\" }")
	g.line("return \"(\" + column + \" >= \" + lower + \" AND \" + column + \" \" + upperOperator + \" \" + upper + \")\"")
	g.indent--
	g.line("}")
	g.line("nilValue := condition.value == nil")
	g.line("if !nilValue { reflected := reflect.ValueOf(condition.value); nilValue = reflected.Kind() == reflect.Ptr && reflected.IsNil() }")
	g.line("if nilValue && condition.operator == \"=\" { return column + \" IS NULL\" }")
	g.line("if nilValue && condition.operator == \"!=\" { return column + \" IS NOT NULL\" }")
	g.line("clause := column + \" \" + condition.operator + \" \" + trbOrmPlaceholder(len(*arguments)+1)")
	g.line("*arguments = append(*arguments, condition.value)")
	g.line("return clause")
	g.indent--
	g.line("case \"and\", \"or\":")
	g.indent++
	g.line("clauses := make([]string, 0, len(predicate.children))")
	g.line("for index := range predicate.children { clauses = append(clauses, " + goORMPredicateSQL(model) + "(&predicate.children[index], arguments)) }")
	g.line("join := \" AND \"; if predicate.kind == \"or\" { join = \" OR \" }")
	g.line("return \"(\" + strings.Join(clauses, join) + \")\"")
	g.indent--
	g.line("case \"not\":")
	g.indent++
	g.line("if len(predicate.children) != 1 { panic(\"invalid ORM not predicate\") }")
	g.line("return \"NOT (\" + " + goORMPredicateSQL(model) + "(&predicate.children[0], arguments) + \")\"")
	g.indent--
	g.line("case \"exists\":")
	g.indent++
	g.line("if predicate.exists == nil { panic(\"invalid ORM exists predicate\") }")
	g.line("return predicate.exists(arguments)")
	g.indent--
	g.line("default:")
	g.indent++
	g.line("panic(\"unsupported ORM predicate\")")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMAssociationPredicate(model) + "(query " + queryType + ") " + g.ormLifecycleAlias() + ".TrbOrmAssociationPredicate {")
	g.indent++
	g.line("if query.transaction != nil { panic(\"ORM association predicate query must not have a transaction scope; scope the base query instead\") }")
	g.line("if len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || query.distinct || len(query.preloads) > 0 || len(query.joins) > 0 || " + goORMPredicateContainsExists(model) + "(query.predicate) { panic(\"ORM association predicate query accepts only where, not, and or\") }")
	g.line("return func(arguments *[]any) string { return " + goORMPredicateSQL(model) + "(query.predicate, arguments) }")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMAssociationFilterPredicate(model) + "(query " + queryType + ") " + g.ormLifecycleAlias() + ".TrbOrmAssociationPredicate {")
	g.indent++
	g.line("if query.transaction != nil { panic(\"ORM association scope must not have a transaction scope; scope the base query instead\") }")
	g.line("if query.limit != nil || query.offset != nil || query.lock || len(query.joins) > 0 || " + goORMPredicateContainsExists(model) + "(query.predicate) { panic(\"ORM association scope accepts filters, order, distinct, and preload only\") }")
	g.line("return func(arguments *[]any) string { return " + goORMPredicateSQL(model) + "(query.predicate, arguments) }")
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
	g.line("func " + goORMLock(model) + "(query " + queryType + ") " + queryType + " {")
	g.indent++
	g.line("query.lock = true")
	g.line("return query")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMDistinct(model) + "(query " + queryType + ") " + queryType + " { query.distinct = true; return query }")
	g.b.WriteByte('\n')
	g.line("func " + goORMPreload(model) + "(query " + queryType + ", association string, load func(*" + g.ormLifecycleAlias() + ".TrbOrmTransaction, []*" + goIdentifier(model.Name, true) + ") *" + g.goType(types.FromName("DbError")) + ") " + queryType + " {")
	g.indent++
	g.line("result := query")
	g.line("result.preloads = append([]" + preloadType + "(nil), query.preloads...)")
	g.line("preload := " + preloadType + "{name: association, load: load}")
	g.line("for index, existing := range result.preloads { if existing.name == association { result.preloads[index] = preload; return result } }")
	g.line("result.preloads = append(result.preloads, preload)")
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
	g.line("func (self *" + goIdentifier(model.Name, true) + ") TrbOrmTransaction() *" + g.ormLifecycleAlias() + ".TrbOrmTransaction { return self." + goORMQueryScopeField() + ".transaction }")
	g.b.WriteByte('\n')
	g.ormCreateRuntime(adapter, model, columns, scanTargets)
	g.ormRelationWriteRuntime(adapter, model)
	for _, column := range model.Columns {
		g.ormProjectionRuntime(adapter, model, column)
		g.ormSubqueryRuntime(adapter, model, column)
		g.ormGroupRuntime(adapter, model, column)
		for _, operation := range ormintegration.AggregateOperations() {
			if resultType, ok := ormintegration.AggregateResultType(operation, column); ok {
				g.ormAggregateRuntime(adapter, model, column, operation, resultType)
			}
		}
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
		g.line("func (self *" + goIdentifier(model.Name, true) + ") " + goORMAssociationSetter(association.Name) + "(value any) {")
		g.indent++
		g.line("self." + goORMAssociationValueField(association.Name) + " = value")
		g.line("self." + goORMAssociationLoadedField(association.Name) + " = true")
		g.indent--
		g.line("}")
		g.b.WriteByte('\n')
	}
	statement := " FROM " + adapter.QuoteIdentifier(model.Table)
	g.line("func " + goORMStatement(model) + "(query " + queryType + ", projection string) (string, []any) {")
	g.indent++
	g.line("arguments := []any{}")
	g.line("statement := " + goORMStatementAppend(model) + "(query, projection, &arguments)")
	g.line("return statement, arguments")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
	g.line("func " + goORMStatementAppend(model) + "(query " + queryType + ", projection string, arguments *[]any) string {")
	g.indent++
	g.line("prefix := \"SELECT \"; if query.distinct { prefix += \"DISTINCT \" }; statement := prefix + projection + " + strconv.Quote(statement))
	g.line("for index, join := range query.joins {")
	g.indent++
	g.line("switch join.Kind { case \"INNER JOIN\", \"LEFT JOIN\": default: panic(\"unsupported ORM join kind\") }")
	g.line("alias := \"__trb_join_\" + strconv.Itoa(index)")
	g.line("key := \"__trb_join_key\"")
	g.line("subquery := \"\"")
	g.line("if join.Build != nil { subquery = join.Build(arguments) } else {")
	g.indent++
	g.line("subquery = \"SELECT \" + trbOrmQuoteIdentifier(join.TargetColumn) + \" AS \" + trbOrmQuoteIdentifier(key) + \" FROM \" + trbOrmQuoteIdentifier(join.Table)")
	g.line("if join.Predicate != nil { if predicate := join.Predicate(arguments); predicate != \"\" { subquery += \" WHERE \" + predicate } }")
	g.indent--
	g.line("}")
	g.line("statement += \" \" + join.Kind + \" (\" + subquery + \") AS \" + trbOrmQuoteIdentifier(alias) + \" ON \" + trbOrmQuoteIdentifier(join.SourceColumn) + \" = \" + trbOrmQuoteIdentifier(alias) + \".\" + trbOrmQuoteIdentifier(key)")
	g.indent--
	g.line("}")
	g.line("if query.predicate != nil { statement += \" WHERE \" + " + goORMPredicateSQL(model) + "(query.predicate, arguments) }")
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
	g.line("if query.limit != nil { statement += \" LIMIT \" + trbOrmPlaceholder(len(*arguments)+1); *arguments = append(*arguments, *query.limit) } else if query.offset != nil { statement += " + strconv.Quote(adapter.OffsetNoLimit) + " }")
	g.line("if query.offset != nil { statement += \" OFFSET \" + trbOrmPlaceholder(len(*arguments)+1); *arguments = append(*arguments, *query.offset) }")
	if adapter.Name != "sqlite" {
		g.line("if query.lock { statement += \" FOR UPDATE\" }")
	}
	g.line("return statement")
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
	g.line("database, databaseError := trbOrmExecutorForQuery(query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(stringType, "*databaseError") + " }")
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
			g.ormAssociationPreloader(manifest, model, association)
		}
	}

	modelType := types.FromName(model.Name)
	modelsType := types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{modelType}}
	g.line("func " + goORMLoader(model) + "(query " + queryType + ") " + g.ormResultType(modelsType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(modelsType, "*databaseError") + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, " + strconv.Quote(strings.Join(columns, ", ")) + ")")
	g.line("rows, err := database.Query(statement, arguments...)")
	g.line("if err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("defer rows.Close()")
	g.line("result := []*" + goIdentifier(model.Name, true) + "{}")
	g.line("for rows.Next() {")
	g.indent++
	g.line("value := &" + goIdentifier(model.Name, true) + "{}")
	g.line("if err := rows.Scan(" + strings.Join(scanTargets, ", ") + "); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("InvalidData")+", \"database row was invalid\")") + " }")
	g.line("value." + goORMQueryScopeField() + " = query")
	g.line("result = append(result, value)")
	g.indent--
	g.line("}")
	g.line("if err := rows.Err(); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("if err := rows.Close(); err != nil { return " + g.ormResultErr(modelsType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database query failed\")") + " }")
	g.line("for _, preload := range query.preloads {")
	g.indent++
	g.line("if preload.load == nil { panic(\"unsupported ORM preload \" + preload.name) }")
	g.line("if preloadError := preload.load(query.transaction, result); preloadError != nil { return " + g.ormResultErr(modelsType, "*preloadError") + " }")
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
		g.line("if len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || len(query.joins) > 0 { return " + g.ormResultErr(modelsType, g.ormErrorValue("InvalidData", "batch queries do not accept joins, order, limit, offset, or lock")) + " }")
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
	g.line("database, databaseError := trbOrmExecutorForQuery(query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(integerType, "*databaseError") + " }")
	g.line("projection := \"1\"; if query.distinct { projection = " + strconv.Quote(strings.Join(columns, ", ")) + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, projection)")
	g.line("row := database.QueryRow(\"SELECT COUNT(*) FROM (\"+statement+\") AS trb_count\", arguments...)")
	g.line("var count int")
	g.line("if err := row.Scan(&count); err != nil { return " + g.ormResultErr(integerType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database count failed\")") + " }")
	g.line("return " + g.ormResultOK(integerType, "count"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	booleanType := types.FromName("Boolean")
	g.line("func " + goORMExists(model) + "(query " + queryType + ") " + g.ormResultType(booleanType) + " {")
	g.indent++
	g.line("database, databaseError := trbOrmExecutorForQuery(query.transaction, query.lock)")
	g.line("if databaseError != nil { return " + g.ormResultErr(booleanType, "*databaseError") + " }")
	g.line("statement, arguments := " + goORMStatement(model) + "(query, \"1\")")
	g.line("row := database.QueryRow(\"SELECT EXISTS(\"+statement+\")\", arguments...)")
	g.line("var exists bool")
	g.line("if err := row.Scan(&exists); err != nil { return " + g.ormResultErr(booleanType, "trbOrmError(err, "+g.ormErrorKind("Query")+", \"database existence query failed\")") + " }")
	g.line("return " + g.ormResultOK(booleanType, "exists"))
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')
}

func (g *generator) ormAssociationPreloader(manifest *ormintegration.Manifest, model ormintegration.Model, association ormintegration.Association) {
	if association.Through != "" {
		g.ormThroughAssociationPreloader(manifest, model, association)
		return
	}
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
	if association.Kind == ormintegration.HasMany || association.Kind == ormintegration.HasOne {
		keyColumn = sourceColumn
	}
	keyType := keyColumn.Type
	keyType.Nullable = false
	function := goORMAssociationPreloader(model, association)
	sourceType := "*" + goIdentifier(model.Name, true)
	targetType := "*" + goIdentifier(target.Name, true)
	sourceQueryType := goORMQueryType(model)
	targetQueryType := g.ormModelQualifier(target) + goORMQueryType(target)
	valueField := goORMAssociationValueField(association.Name)
	loadedField := goORMAssociationLoadedField(association.Name)

	g.line("func " + goORMTypedPreload(model, association) + "(query " + sourceQueryType + ", targetQuery " + targetQueryType + ") " + sourceQueryType + " {")
	g.indent++
	g.line("return " + goORMPreload(model) + "(query, " + strconv.Quote(association.Name) + ", func(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction, values []" + sourceType + ") *" + g.goType(types.FromName("DbError")) + " {")
	g.indent++
	g.line("return " + function + "(transaction, values, targetQuery)")
	g.indent--
	g.line("})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + function + "(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction, values []" + sourceType + ", targetQuery " + targetQueryType + ") *" + g.goType(types.FromName("DbError")) + " {")
	g.indent++
	g.line("if targetQuery.transaction != nil { databaseError := " + g.ormErrorValue("InvalidData", "ORM preload query must not have a transaction scope; scope the base query instead") + "; return &databaseError }")
	g.line("if targetQuery.limit != nil || targetQuery.offset != nil || targetQuery.lock { databaseError := " + g.ormErrorValue("InvalidData", "ORM preload query does not accept limit, offset, or lock") + "; return &databaseError }")
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
	g.line("targetQuery.transaction = transaction")
	g.line("targetQuery = " + g.ormModelQualifier(target) + goORMQueryWhere(target) + "(targetQuery, []string{" + strconv.Quote(association.TargetColumn) + "}, []string{\"IN\"}, []any{arguments})")
	g.line("loaded := " + g.ormModelQualifier(target) + goORMLoader(target) + "(targetQuery)")
	g.line("if loaded.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return &loaded.ErrError }")
	if association.Kind == ormintegration.HasMany {
		g.line("related := map[" + g.goType(keyType) + "][]" + targetType + "{}")
	} else {
		g.line("related := map[" + g.goType(keyType) + "]" + targetType + "{}")
	}
	g.line("for _, relatedValue := range loaded.OkValue {")
	g.indent++
	if association.Kind == ormintegration.BelongsTo {
		g.line("related[relatedValue." + goFieldName(targetColumn.Name) + "] = relatedValue")
	} else if association.Kind == ormintegration.HasOne {
		if targetColumn.Nullable {
			g.line("if relatedValue." + goFieldName(targetColumn.Name) + " != nil {")
			g.indent++
			g.line("key := *relatedValue." + goFieldName(targetColumn.Name))
			g.line("if related[key] != nil { databaseError := " + g.ormErrorValue("InvalidData", "database has_one association returned multiple rows") + "; return &databaseError }")
			g.line("related[key] = relatedValue")
			g.indent--
			g.line("}")
		} else {
			g.line("key := relatedValue." + goFieldName(targetColumn.Name))
			g.line("if related[key] != nil { databaseError := " + g.ormErrorValue("InvalidData", "database has_one association returned multiple rows") + "; return &databaseError }")
			g.line("related[key] = relatedValue")
		}
	} else if targetColumn.Nullable {
		g.line("if relatedValue." + goFieldName(targetColumn.Name) + " != nil { key := *relatedValue." + goFieldName(targetColumn.Name) + "; related[key] = append(related[key], relatedValue) }")
	} else {
		g.line("key := relatedValue." + goFieldName(targetColumn.Name))
		g.line("related[key] = append(related[key], relatedValue)")
	}
	g.indent--
	g.line("}")
	g.line("for _, value := range values {")
	g.indent++
	if association.Kind == ormintegration.BelongsTo || association.Kind == ormintegration.HasOne {
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

func (g *generator) ormThroughAssociationPreloader(manifest *ormintegration.Manifest, model ormintegration.Model, association ormintegration.Association) {
	through, throughOK := model.Association(association.Through)
	middle, middleOK := manifest.Model(through.TargetModel)
	if !throughOK || !middleOK {
		return
	}
	via, viaOK := middle.Association(association.Source)
	target, targetOK := manifest.Model(association.TargetModel)
	if !viaOK || !targetOK {
		return
	}
	sourceColumn, sourceOK := model.Column(through.SourceColumn)
	middleParentColumn, parentOK := middle.Column(through.TargetColumn)
	middleTargetColumn, linkOK := middle.Column(via.SourceColumn)
	targetColumn, targetColumnOK := target.Column(via.TargetColumn)
	if !sourceOK || !parentOK || !linkOK || !targetColumnOK {
		return
	}
	parentKeyType := sourceColumn.Type
	parentKeyType.Nullable = false
	targetKeyType := targetColumn.Type
	targetKeyType.Nullable = false
	function := goORMAssociationPreloader(model, association)
	sourceType := "*" + goIdentifier(model.Name, true)
	targetType := "*" + goIdentifier(target.Name, true)
	sourceQueryType := goORMQueryType(model)
	middleQualifier := g.ormModelQualifier(middle)
	targetQualifier := g.ormModelQualifier(target)
	targetQueryType := targetQualifier + goORMQueryType(target)
	valueField := goORMAssociationValueField(association.Name)
	loadedField := goORMAssociationLoadedField(association.Name)

	g.line("func " + goORMTypedPreload(model, association) + "(query " + sourceQueryType + ", targetQuery " + targetQueryType + ") " + sourceQueryType + " {")
	g.indent++
	g.line("return " + goORMPreload(model) + "(query, " + strconv.Quote(association.Name) + ", func(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction, values []" + sourceType + ") *" + g.goType(types.FromName("DbError")) + " {")
	g.indent++
	g.line("return " + function + "(transaction, values, targetQuery)")
	g.indent--
	g.line("})")
	g.indent--
	g.line("}")
	g.b.WriteByte('\n')

	g.line("func " + function + "(transaction *" + g.ormLifecycleAlias() + ".TrbOrmTransaction, values []" + sourceType + ", targetQuery " + targetQueryType + ") *" + g.goType(types.FromName("DbError")) + " {")
	g.indent++
	g.line("if targetQuery.transaction != nil { databaseError := " + g.ormErrorValue("InvalidData", "ORM preload query must not have a transaction scope; scope the base query instead") + "; return &databaseError }")
	g.line("if targetQuery.limit != nil || targetQuery.offset != nil || targetQuery.lock { databaseError := " + g.ormErrorValue("InvalidData", "ORM preload query does not accept limit, offset, or lock") + "; return &databaseError }")
	g.line("parentArguments := []any{}")
	g.line("for _, value := range values {")
	g.indent++
	if sourceColumn.Nullable {
		g.line("if value." + goFieldName(sourceColumn.Name) + " != nil { parentArguments = append(parentArguments, *value." + goFieldName(sourceColumn.Name) + ") }")
	} else {
		g.line("parentArguments = append(parentArguments, value." + goFieldName(sourceColumn.Name) + ")")
	}
	g.indent--
	g.line("}")
	g.line("if len(parentArguments) == 0 {")
	g.indent++
	g.line("for _, value := range values { value." + loadedField + " = true")
	if association.Kind == ormintegration.HasMany {
		g.line("value." + valueField + " = []" + targetType + "{}")
	}
	g.line("}")
	g.line("return nil")
	g.indent--
	g.line("}")
	g.line("middleQuery := " + middleQualifier + goORMUsing(middle) + "(transaction)")
	g.line("middleQuery = " + middleQualifier + goORMQueryWhere(middle) + "(middleQuery, []string{" + strconv.Quote(middleParentColumn.Name) + "}, []string{\"IN\"}, []any{parentArguments})")
	g.line("loadedMiddle := " + middleQualifier + goORMLoader(middle) + "(middleQuery)")
	g.line("if loadedMiddle.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return &loadedMiddle.ErrError }")
	g.line("links := map[" + g.goType(parentKeyType) + "][]" + g.goType(targetKeyType) + "{}")
	g.line("targetArguments := []any{}")
	g.line("for _, middleValue := range loadedMiddle.OkValue {")
	g.indent++
	parentExpression := "middleValue." + goFieldName(middleParentColumn.Name)
	linkExpression := "middleValue." + goFieldName(middleTargetColumn.Name)
	conditions := []string{}
	if middleParentColumn.Nullable {
		conditions = append(conditions, parentExpression+" != nil")
		parentExpression = "*" + parentExpression
	}
	if middleTargetColumn.Nullable {
		conditions = append(conditions, linkExpression+" != nil")
		linkExpression = "*" + linkExpression
	}
	if len(conditions) > 0 {
		g.line("if " + strings.Join(conditions, " && ") + " {")
		g.indent++
	}
	g.line("links[" + parentExpression + "] = append(links[" + parentExpression + "], " + linkExpression + ")")
	g.line("targetArguments = append(targetArguments, " + linkExpression + ")")
	if len(conditions) > 0 {
		g.indent--
		g.line("}")
	}
	g.indent--
	g.line("}")
	g.line("related := map[" + g.goType(targetKeyType) + "]" + targetType + "{}")
	g.line("if len(targetArguments) > 0 {")
	g.indent++
	g.line("targetQuery.transaction = transaction")
	g.line("targetQuery = " + targetQualifier + goORMQueryWhere(target) + "(targetQuery, []string{" + strconv.Quote(targetColumn.Name) + "}, []string{\"IN\"}, []any{targetArguments})")
	g.line("loadedTargets := " + targetQualifier + goORMLoader(target) + "(targetQuery)")
	g.line("if loadedTargets.Kind == " + g.ormPackageAlias() + ".DbResultErrTag { return &loadedTargets.ErrError }")
	g.line("for _, relatedValue := range loadedTargets.OkValue {")
	g.indent++
	targetExpression := "relatedValue." + goFieldName(targetColumn.Name)
	if targetColumn.Nullable {
		g.line("if " + targetExpression + " != nil { related[*" + targetExpression + "] = relatedValue }")
	} else {
		g.line("related[" + targetExpression + "] = relatedValue")
	}
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("for _, value := range values {")
	g.indent++
	sourceExpression := "value." + goFieldName(sourceColumn.Name)
	if sourceColumn.Nullable {
		g.line("if " + sourceExpression + " == nil { value." + loadedField + " = true")
		if association.Kind == ormintegration.HasMany {
			g.line("value." + valueField + " = []" + targetType + "{}")
		}
		g.line("continue }")
		sourceExpression = "*" + sourceExpression
	}
	g.line("items := []" + targetType + "{}")
	g.line("for _, key := range links[" + sourceExpression + "] { if relatedValue := related[key]; relatedValue != nil { items = append(items, relatedValue) } }")
	if association.Kind == ormintegration.HasMany {
		g.line("value." + valueField + " = items")
	} else {
		g.line("if len(items) > 1 { databaseError := " + g.ormErrorValue("InvalidData", "database has_one through association returned multiple rows") + "; return &databaseError }")
		g.line("if len(items) == 1 { value." + valueField + " = items[0] }")
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

func goORMQueryScopeField() string {
	return goFieldName("@__trb_orm_query_scope")
}

func goORMConditionType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Condition"
}

func goORMPredicateType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Predicate"
}

func goORMOrderType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Order"
}

func goORMPreloadType(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "Preload"
}

func goORMDistinct(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Distinct"
}

func goORMTypedPreload(model ormintegration.Model, association ormintegration.Association) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Preload" + goIdentifier(association.Name, true)
}

func goORMWhere(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Where"
}

func goORMUsing(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Using"
}

func goORMQueryWhere(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "QueryWhere"
}

func goORMNot(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Not"
}

func goORMQueryNot(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "QueryNot"
}

func goORMQueryOr(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "QueryOr"
}

func goORMPredicateGroup(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "PredicateGroup"
}

func goORMCombinePredicates(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "CombinePredicates"
}

func goORMPredicateSQL(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "PredicateSQL"
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

func goORMLock(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Lock"
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

func goORMCreate(model ormintegration.Model) string {
	return "TrbOrmCreate" + goIdentifier(model.Name, true)
}

func goORMBuild(model ormintegration.Model) string {
	return "TrbOrmBuild" + goIdentifier(model.Name, true)
}

func goORMDraftSave(model ormintegration.Model) string {
	return "TrbOrmSave" + goIdentifier(model.Name, true) + "Draft"
}

func goORMInsertAll(model ormintegration.Model) string {
	return "TrbOrmInsertAll" + goIdentifier(model.Name, true)
}

func goORMInsertIfAbsent(model ormintegration.Model) string {
	return "TrbOrmInsert" + goIdentifier(model.Name, true) + "IfAbsent"
}

func goORMUpsert(model ormintegration.Model) string {
	return "TrbOrmUpsert" + goIdentifier(model.Name, true)
}

func goORMUpsertAll(model ormintegration.Model) string {
	return "TrbOrmUpsertAll" + goIdentifier(model.Name, true)
}

func goORMDraftColumnValues(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "DraftColumnValues"
}

func goORMUniqueColumns(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "UniqueColumns"
}

func goORMWritableColumn(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "WritableColumn"
}

func goORMValuesContainNil(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "ValuesContainNil"
}

func goORMUpdate(model ormintegration.Model) string {
	return "TrbOrmUpdate" + goIdentifier(model.Name, true)
}

func goORMWith(model ormintegration.Model) string {
	return "TrbOrmWith" + goIdentifier(model.Name, true)
}

func goORMChangesSave(model ormintegration.Model) string {
	return "TrbOrmSave" + goIdentifier(model.Name, true) + "Changes"
}

func goORMDelete(model ormintegration.Model) string {
	return "TrbOrmDelete" + goIdentifier(model.Name, true)
}

func goORMDestroy(model ormintegration.Model) string {
	return "TrbOrmDestroy" + goIdentifier(model.Name, true)
}

func goORMDestroyAll(model ormintegration.Model) string {
	return "TrbOrmDestroyAll" + goIdentifier(model.Name, true)
}

func goORMUpdateAll(model ormintegration.Model) string {
	return "TrbOrmUpdateAll" + goIdentifier(model.Name, true)
}

func goORMDeleteAll(model ormintegration.Model) string {
	return "TrbOrmDeleteAll" + goIdentifier(model.Name, true)
}

func goORMCount(model ormintegration.Model) string {
	return "TrbOrmCount" + goIdentifier(model.Name, true)
}

func goORMExists(model ormintegration.Model) string {
	return "TrbOrmExists" + goIdentifier(model.Name, true)
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

func goORMStatementAppend(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "StatementAppend"
}

func goORMValidateSubqueries(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "ValidateSubqueries"
}

func goORMWhereExists(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "WhereExists"
}

func goORMPredicateContainsExists(model ormintegration.Model) string {
	return "trbOrm" + goIdentifier(model.Name, true) + "PredicateContainsExists"
}

func goORMAssociationGetter(name string) string {
	return "TrbOrmAssociation" + goIdentifier(name, true)
}

func goORMAssociationSetter(name string) string {
	return "TrbOrmSetAssociation" + goIdentifier(name, true)
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
