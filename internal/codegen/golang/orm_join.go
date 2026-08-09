package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

func (g *generator) ormJoin(call *ir.Call, kind string) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(call.Arguments) == 0 {
		return "nil"
	}
	modelName := member.Receiver.ExprType().Name
	model, exists := g.orm.QueryModel(modelName)
	query := ""
	if exists {
		query = g.expr(member.Receiver)
	} else {
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			modelName = identifier.Name
		}
		model, exists = g.orm.Model(modelName)
		if !exists {
			model, exists = g.orm.ScopeModel(modelName)
		}
		if !exists {
			return "nil"
		}
		if _, scope := g.orm.ScopeModel(member.Receiver.ExprType().Name); scope {
			query = g.expr(member.Receiver)
		} else {
			qualifier := g.ormModelQualifier(model)
			query = qualifier + goORMWhere(model) + "([]string{}, []string{}, []any{})"
		}
	}
	associationName, ok := ormJoinAssociation(call.Arguments[0].Value)
	if !ok {
		return "nil"
	}
	association, ok := model.Association(associationName)
	if !ok {
		return "nil"
	}
	target, ok := g.orm.Model(association.TargetModel)
	if !ok {
		return "nil"
	}
	predicate := "nil"
	if len(call.Arguments) > 1 {
		predicate = g.ormModelQualifier(target) + goORMJoinPredicate(target) + "(" + g.expr(call.Arguments[1].Value) + ")"
	}
	join := g.ormLifecycleAlias() + ".TrbOrmJoin{" +
		"Kind: " + strconv.Quote(kind) +
		", Table: " + strconv.Quote(target.Table) +
		", SourceColumn: " + strconv.Quote(association.SourceColumn) +
		", TargetColumn: " + strconv.Quote(association.TargetColumn) +
		", Predicate: " + predicate + "}"
	return g.ormModelQualifier(model) + goORMJoin(model) + "(" + query + ", " + join + ")"
}

func ormJoinAssociation(expression ir.Expression) (string, bool) {
	switch value := expression.(type) {
	case *ir.Symbol:
		return value.Name, true
	case *ir.Literal:
		if value.Kind != "string" {
			return "", false
		}
		decoded, err := strconv.Unquote(value.Raw)
		if err != nil {
			return "", false
		}
		return decoded, true
	default:
		return "", false
	}
}

func goORMJoin(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "Join"
}

func goORMJoinPredicate(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "JoinPredicate"
}
