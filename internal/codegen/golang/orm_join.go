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
	if association.Through != "" {
		return g.ormThroughJoin(model, query, association, target, predicate, kind)
	}
	join := g.ormLifecycleAlias() + ".TrbOrmJoin{" +
		"Kind: " + strconv.Quote(kind) +
		", Table: " + strconv.Quote(target.Table) +
		", SourceColumn: " + strconv.Quote(association.SourceColumn) +
		", TargetColumn: " + strconv.Quote(association.TargetColumn) +
		", Predicate: " + predicate + "}"
	return g.ormModelQualifier(model) + goORMJoin(model) + "(" + query + ", " + join + ")"
}

func (g *generator) ormThroughJoin(model ormintegration.Model, query string, association ormintegration.Association, target ormintegration.Model, predicate, kind string) string {
	through, ok := model.Association(association.Through)
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
	predicateStatement := ""
	if predicate != "nil" {
		predicateStatement = "if clause := " + predicate + "(arguments); clause != \"\" { targetStatement += \" WHERE \" + clause }; "
	}
	build := "func(arguments *[]any) string { " +
		"targetKey := \"__trb_through_key\"; targetAlias := \"__trb_through_target\"; " +
		"targetStatement := \"SELECT \" + trbOrmQuoteIdentifier(" + strconv.Quote(via.TargetColumn) + ") + \" AS \" + trbOrmQuoteIdentifier(targetKey) + \" FROM \" + trbOrmQuoteIdentifier(" + strconv.Quote(target.Table) + "); " +
		predicateStatement +
		"return \"SELECT \" + trbOrmQuoteIdentifier(" + strconv.Quote(through.TargetColumn) + ") + \" AS \" + trbOrmQuoteIdentifier(\"__trb_join_key\") + \" FROM \" + trbOrmQuoteIdentifier(" + strconv.Quote(middle.Table) + ") + \" INNER JOIN (\" + targetStatement + \") AS \" + trbOrmQuoteIdentifier(targetAlias) + \" ON \" + trbOrmQuoteIdentifier(" + strconv.Quote(via.SourceColumn) + ") + \" = \" + trbOrmQuoteIdentifier(targetAlias) + \".\" + trbOrmQuoteIdentifier(targetKey) }"
	join := g.ormLifecycleAlias() + ".TrbOrmJoin{" +
		"Kind: " + strconv.Quote(kind) +
		", SourceColumn: " + strconv.Quote(through.SourceColumn) +
		", Build: " + build + "}"
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

func goORMAssociationPredicate(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "AssociationPredicate"
}

func goORMAssociationFilterPredicate(model ormintegration.Model) string {
	return "TrbOrm" + goIdentifier(model.Name, true) + "AssociationFilterPredicate"
}
