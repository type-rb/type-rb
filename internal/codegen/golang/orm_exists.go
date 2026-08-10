package golang

import (
	"strconv"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) ormWhereExists(call *ir.Call, negated bool) string {
	member, ok := call.Callee.(*ir.Member)
	if !ok || len(call.Arguments) == 0 {
		return "nil"
	}
	modelName := member.Receiver.ExprType().Name
	model, queryReceiver := g.orm.QueryModel(modelName)
	query := ""
	if queryReceiver {
		query = g.expr(member.Receiver)
	} else {
		if identifier, identifierOK := member.Receiver.(*ir.Identifier); identifierOK {
			modelName = identifier.Name
		}
		var exists bool
		model, exists = g.orm.Model(modelName)
		if !exists {
			model, exists = g.orm.ScopeModel(member.Receiver.ExprType().Name)
		}
		if !exists {
			return "nil"
		}
		if _, scope := g.orm.ScopeModel(member.Receiver.ExprType().Name); scope {
			query = g.expr(member.Receiver)
		} else {
			query = g.ormModelQualifier(model) + goORMWhere(model) + "([]string{}, []string{}, []any{})"
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
	targetQuery := g.ormModelQualifier(target) + goORMWhere(target) + "([]string{}, []string{}, []any{})"
	if len(call.Arguments) > 1 {
		targetQuery = g.expr(call.Arguments[1].Value)
	}
	if association.Scope != nil {
		targetQuery = g.ormAssociationScope(association, target, targetQuery)
		predicate = g.ormModelQualifier(target) + goORMAssociationFilterPredicate(target) + "(" + targetQuery + ")"
	} else if len(call.Arguments) > 1 {
		predicate = g.ormModelQualifier(target) + goORMAssociationPredicate(target) + "(" + targetQuery + ")"
	}
	return g.ormModelQualifier(model) + goORMWhereExists(model) + "(" +
		query + ", " + strconv.Quote(target.Table) + ", " + strconv.Quote(association.SourceColumn) + ", " +
		strconv.Quote(association.TargetColumn) + ", " + predicate + ", " + strconv.FormatBool(negated) + ")"
}
