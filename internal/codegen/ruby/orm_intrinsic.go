package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

// ormIntrinsic only selects operations from typed ORM IR. Query construction,
// lifecycle behavior, and database error semantics remain in the generated
// TypeRB-owned runtime; the private Sequel dependency is only an execution
// adapter.
func (g *generator) ormIntrinsic(name string, call *ir.Call, arguments []string) string {
	switch name {
	case "trb.orm.where":
		return g.rubyORMInitialQuery(call) + ".where(" + g.rubyORMPredicates(call) + ")"
	case "trb.orm.query.where":
		return g.rubyORMReceiver(call) + ".where(" + g.rubyORMPredicates(call) + ")"
	case "trb.orm.not":
		return g.rubyORMInitialQuery(call) + ".not(" + g.rubyORMPredicates(call) + ")"
	case "trb.orm.query.not":
		return g.rubyORMReceiver(call) + ".not(" + g.rubyORMPredicates(call) + ")"
	case "trb.orm.query.or":
		if len(arguments) < 2 {
			return "nil"
		}
		return arguments[0] + ".or_query(" + arguments[1] + ")"
	case "trb.orm.distinct", "trb.orm.query.distinct":
		return g.rubyORMQuery(call) + ".distinct_rows"
	case "trb.orm.select", "trb.orm.query.select":
		return g.rubyORMQuery(call) + ".select_columns(" + g.rubyORMPositional(call) + ")"
	case "trb.orm.group", "trb.orm.query.group":
		return g.rubyORMQuery(call) + ".group_by_column(" + g.rubyORMPositional(call) + ")"
	case "trb.orm.group.having":
		return g.rubyORMReceiver(call) + ".having_values(" + strings.Join(arguments[1:], ", ") + ")"
	case "trb.orm.group.count":
		return g.rubyORMReceiver(call) + ".grouped_aggregate_result(\"count\", nil)"
	case "trb.orm.group.sum", "trb.orm.group.average", "trb.orm.group.minimum", "trb.orm.group.maximum":
		operation := name[strings.LastIndex(name, ".")+1:]
		column := "nil"
		if len(arguments) > 1 {
			column = arguments[1]
		}
		return g.rubyORMReceiver(call) + ".grouped_aggregate_result(" + strconv.Quote(operation) + ", " + column + ")"
	case "trb.orm.join", "trb.orm.query.join", "trb.orm.left_join", "trb.orm.query.left_join":
		kind := "INNER JOIN"
		if strings.Contains(name, "left_join") {
			kind = "LEFT JOIN"
		}
		return g.rubyORMQuery(call) + ".join_association(" + g.rubyORMPositional(call) + ", " + strconv.Quote(kind) + ")"
	case "trb.orm.where_exists", "trb.orm.query.where_exists", "trb.orm.where_not_exists", "trb.orm.query.where_not_exists":
		negated := strings.Contains(name, "not_exists")
		return g.rubyORMQuery(call) + ".where_association_exists(" + g.rubyORMPositional(call) + ", " + strconv.FormatBool(negated) + ")"
	case "trb.orm.using":
		if len(arguments) < 1 {
			return "nil"
		}
		return g.rubyORMInitialQuery(call) + ".using(" + arguments[len(arguments)-1] + ")"
	case "trb.orm.order", "trb.orm.query.order":
		return g.rubyORMQuery(call) + ".order_by(" + g.rubyORMNamedPairs(call) + ")"
	case "trb.orm.limit", "trb.orm.query.limit":
		return g.rubyORMQuery(call) + ".limit_rows(" + g.rubyORMLastArgument(call) + ")"
	case "trb.orm.offset", "trb.orm.query.offset":
		return g.rubyORMQuery(call) + ".offset_rows(" + g.rubyORMLastArgument(call) + ")"
	case "trb.orm.lock", "trb.orm.query.lock":
		return g.rubyORMQuery(call) + ".lock_rows"
	case "trb.orm.all", "trb.orm.query.all":
		return g.rubyORMQuery(call) + ".all_result"
	case "trb.orm.first", "trb.orm.query.first":
		return g.rubyORMQuery(call) + ".first_result"
	case "trb.orm.count", "trb.orm.query.count":
		return g.rubyORMQuery(call) + ".count_result"
	case "trb.orm.to_sql", "trb.orm.query.to_sql":
		return g.rubyORMQuery(call) + ".to_sql"
	case "trb.orm.explain", "trb.orm.query.explain":
		return g.rubyORMQuery(call) + ".explain_result"
	case "trb.orm.find_by", "trb.orm.query.find_by":
		return g.rubyORMQuery(call) + ".where(" + g.rubyORMPredicates(call) + ").first_result"
	case "trb.orm.exists", "trb.orm.query.exists":
		query := g.rubyORMQuery(call)
		if len(ormintegration.Predicates(call)) > 0 {
			query += ".where(" + g.rubyORMPredicates(call) + ")"
		}
		return query + ".exists_result"
	case "trb.orm.preload", "trb.orm.query.preload":
		if len(call.Arguments) == 0 {
			return "nil"
		}
		target := "nil"
		if len(call.Arguments) > 1 {
			target = g.expr(call.Arguments[1].Value)
		}
		return g.rubyORMQuery(call) + ".preload_association(" + g.expr(call.Arguments[0].Value) + ", " + target + ")"
	case "trb.orm.pluck", "trb.orm.query.pluck":
		return g.rubyORMQuery(call) + ".pluck_result(" + g.rubyORMPositional(call) + ")"
	case "trb.orm.pick", "trb.orm.query.pick":
		return g.rubyORMQuery(call) + ".pick_result(" + g.rubyORMPositional(call) + ")"
	case "trb.orm.ids", "trb.orm.query.ids":
		return g.rubyORMQuery(call) + ".ids_result"
	case "trb.orm.sum", "trb.orm.query.sum", "trb.orm.average", "trb.orm.query.average", "trb.orm.minimum", "trb.orm.query.minimum", "trb.orm.maximum", "trb.orm.query.maximum":
		operation := name[strings.LastIndex(name, ".")+1:]
		return g.rubyORMQuery(call) + ".aggregate_result(" + strconv.Quote(operation) + ", " + g.rubyORMLastArgument(call) + ")"
	case "trb.orm.find":
		model, ok := g.rubyORMModel(call)
		if !ok || len(call.Arguments) != 1 {
			return "nil"
		}
		primaryKey, ok := model.PrimaryKey()
		if !ok {
			return "nil"
		}
		return g.rubyORMInitialQuery(call) + ".where([[" + strconv.Quote(primaryKey.Name) + ", \"=\", " + g.expr(call.Arguments[0].Value) + "]]).first_result"
	case "trb.orm.build", "trb.orm.scope.build":
		return "TrbOrmRuntime.build(" + g.rubyORMQuery(call) + ", " + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.create", "trb.orm.scope.create":
		return "TrbOrmRuntime.create_result(" + g.rubyORMQuery(call) + ", " + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.scope.find":
		model, ok := g.rubyORMModel(call)
		if !ok || len(call.Arguments) == 0 {
			return "nil"
		}
		primaryKey, ok := model.PrimaryKey()
		if !ok {
			return "nil"
		}
		return g.rubyORMReceiver(call) + ".where([[" + strconv.Quote(primaryKey.Name) + ", \"=\", " + g.expr(call.Arguments[0].Value) + "]]).first_result"
	case "trb.orm.draft.save":
		return "TrbOrmRuntime.save_draft_result(" + g.rubyORMReceiver(call) + ")"
	case "trb.orm.insert_all":
		return "TrbOrmRuntime.insert_all_result(" + g.rubyORMInitialQuery(call) + ", " + g.rubyORMLastArgument(call) + ")"
	case "trb.orm.insert_if_absent":
		return "TrbOrmRuntime.insert_if_absent_result(" + g.rubyORMInitialQuery(call) + ", " + g.rubyORMArgument(call, "", 0) + ", " + g.rubyORMArgument(call, "unique_by", 1) + ")"
	case "trb.orm.draft.upsert":
		return "TrbOrmRuntime.upsert_result(" + g.rubyORMReceiver(call) + ", " + g.rubyORMArgument(call, "unique_by", 0) + ", " + g.rubyORMArgument(call, "update", 1) + ")"
	case "trb.orm.upsert_all":
		return "TrbOrmRuntime.upsert_all_result(" + g.rubyORMInitialQuery(call) + ", " + g.rubyORMArgument(call, "", 0) + ", " + g.rubyORMArgument(call, "unique_by", 1) + ", " + g.rubyORMArgument(call, "update", 2) + ")"
	case "trb.orm.with":
		return "TrbOrmRuntime.changes(" + g.rubyORMReceiver(call) + ", " + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.update":
		return "TrbOrmRuntime.update_model_result(" + g.rubyORMReceiver(call) + ", " + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.changes.save":
		return "TrbOrmRuntime.save_changes_result(" + g.rubyORMReceiver(call) + ")"
	case "trb.orm.delete":
		return "TrbOrmRuntime.delete_model_result(" + g.rubyORMReceiver(call) + ")"
	case "trb.orm.destroy":
		return "TrbOrmRuntime.destroy_model_result(" + g.rubyORMReceiver(call) + ")"
	case "trb.orm.destroy_all", "trb.orm.query.destroy_all":
		return g.rubyORMQuery(call) + ".destroy_all_result"
	case "trb.orm.update_all":
		return g.rubyORMInitialQuery(call) + ".update_all_result(" + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.query.update_all":
		return g.rubyORMReceiver(call) + ".update_all_result(" + g.rubyORMNamedHash(call) + ")"
	case "trb.orm.delete_all":
		return g.rubyORMInitialQuery(call) + ".delete_all_result"
	case "trb.orm.query.delete_all":
		return g.rubyORMReceiver(call) + ".delete_all_result"
	case "trb.orm.association.query.belongs_to", "trb.orm.association.query.has_many", "trb.orm.association.query.has_one":
		return "TrbOrmRuntime.association_query(" + g.rubyORMReceiver(call) + ", " + strconv.Quote(g.rubyORMAssociationName(call)) + ")"
	case "trb.orm.association.value.belongs_to", "trb.orm.association.value.has_many", "trb.orm.association.value.has_one", "trb.orm.association.load.belongs_to", "trb.orm.association.load.has_many", "trb.orm.association.load.has_one":
		return "TrbOrmRuntime.load_association_result(" + g.rubyORMReceiver(call) + ", " + strconv.Quote(g.rubyORMAssociationName(call)) + ", false)"
	case "trb.orm.association.reload.belongs_to", "trb.orm.association.reload.has_many", "trb.orm.association.reload.has_one":
		return "TrbOrmRuntime.load_association_result(" + g.rubyORMReceiver(call) + ", " + strconv.Quote(g.rubyORMAssociationName(call)) + ", true)"
	case "trb.orm.association.loaded.belongs_to", "trb.orm.association.loaded.has_many", "trb.orm.association.loaded.has_one":
		return "TrbOrmRuntime.association_loaded?(" + g.rubyORMReceiver(call) + ", " + strconv.Quote(g.rubyORMAssociationName(call)) + ")"
	}
	return "nil"
}

func (g *generator) rubyORMModel(call *ir.Call) (ormintegration.Model, bool) {
	member, ok := call.Callee.(*ir.Member)
	if !ok || g.orm == nil {
		return ormintegration.Model{}, false
	}
	name := member.Receiver.ExprType().Name
	if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
		if model, found := g.orm.Model(identifier.Name); found {
			return model, true
		}
	}
	if model, found := g.orm.Model(name); found {
		return model, true
	}
	if model, found := g.orm.QueryModel(name); found {
		return model, true
	}
	if model, found := g.orm.ScopeModel(name); found {
		return model, true
	}
	if model, found := g.orm.DraftModel(name); found {
		return model, true
	}
	if model, found := g.orm.ChangesModel(name); found {
		return model, true
	}
	if model, _, found := g.orm.GroupModel(name); found {
		return model, true
	}
	return ormintegration.Model{}, false
}

func (g *generator) rubyORMInitialQuery(call *ir.Call) string {
	model, ok := g.rubyORMModel(call)
	if !ok {
		return "nil"
	}
	return "TrbOrmRuntime.query(" + model.Name + ")"
}

func (g *generator) rubyORMReceiver(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	return g.expr(member.Receiver)
}

func (g *generator) rubyORMQuery(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return "nil"
	}
	name := member.Receiver.ExprType().Name
	if _, found := g.orm.QueryModel(name); found {
		return g.expr(member.Receiver)
	}
	if _, found := g.orm.ScopeModel(name); found {
		return g.expr(member.Receiver)
	}
	if _, _, found := g.orm.GroupModel(name); found {
		return g.expr(member.Receiver)
	}
	return g.rubyORMInitialQuery(call)
}

func (g *generator) rubyORMPredicates(call *ir.Call) string {
	predicates := ormintegration.Predicates(call)
	parts := make([]string, 0, len(predicates))
	for _, predicate := range predicates {
		value := g.expr(predicate.Value)
		if predicate.Value.ExprType().Kind == types.Range {
			if bounds, ok := predicate.Value.(*ir.Range); ok {
				value = "TrbOrmRuntime::Bounds.new(" + g.expr(bounds.Start) + ", " + g.expr(bounds.End) + ", " + strconv.FormatBool(bounds.Exclusive) + ")"
			} else {
				value = "->(bounds) { TrbOrmRuntime::Bounds.new(bounds.begin, bounds.end, bounds.exclude_end?) }.call(" + value + ")"
			}
		}
		parts = append(parts, "["+strconv.Quote(predicate.Column)+", "+strconv.Quote(string(predicate.Operator))+", "+value+"]")
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (g *generator) rubyORMNamedPairs(call *ir.Call) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name != "" {
			parts = append(parts, "["+strconv.Quote(argument.Name)+", "+g.expr(argument.Value)+"]")
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (g *generator) rubyORMNamedHash(call *ir.Call) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name != "" && argument.Name != "unique_by" && argument.Name != "update" {
			parts = append(parts, strconv.Quote(argument.Name)+" => "+g.expr(argument.Value))
		}
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

func (g *generator) rubyORMPositional(call *ir.Call) string {
	parts := make([]string, 0, len(call.Arguments))
	for _, argument := range call.Arguments {
		if argument.Name == "" {
			parts = append(parts, g.expr(argument.Value))
		}
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func (g *generator) rubyORMLastArgument(call *ir.Call) string {
	if len(call.Arguments) == 0 {
		return "nil"
	}
	return g.expr(call.Arguments[len(call.Arguments)-1].Value)
}

func (g *generator) rubyORMArgument(call *ir.Call, name string, fallback int) string {
	position := 0
	for _, argument := range call.Arguments {
		if argument.Name == name || name == "" && argument.Name == "" {
			if position == fallback || name != "" {
				return g.expr(argument.Value)
			}
			position++
		}
	}
	if fallback >= 0 && fallback < len(call.Arguments) {
		return g.expr(call.Arguments[fallback].Value)
	}
	return "nil"
}

func (g *generator) rubyORMAssociationName(call *ir.Call) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok {
		return ""
	}
	return strings.TrimSuffix(member.Name, "_query")
}
