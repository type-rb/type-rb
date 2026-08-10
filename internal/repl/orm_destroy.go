//go:build !js || !wasm

package repl

import (
	"errors"
	"strconv"

	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) ormDestroyModel(resultType types.Type, value Value) (Value, error) {
	model, ok := e.ormModelForValue(value)
	if !ok {
		return Value{}, errors.New("ORM destroy requires a persisted model")
	}
	query := e.ormModelQuery(value, model)
	transaction, owned, failure := e.ormDestroyTransaction(query)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	query = cloneORMQuery(query)
	query.transaction = transaction
	destroyed, failure := e.ormDestroyObject(query, model, value, map[string]bool{})
	if failure != nil {
		if owned {
			_ = e.ormRollbackTransaction(transaction)
		}
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	if owned {
		if failure := e.ormCommitTransaction(transaction); failure != nil {
			_ = e.ormRollbackTransaction(transaction)
			return e.ormResultErr(resultType, failure.kind, failure.message)
		}
	}
	return e.ormResultOK(resultType, Value{Type: types.FromName("Boolean"), Data: destroyed})
}

func (e *Evaluator) ormDestroyAll(resultType types.Type, source *ormQueryValue) (Value, error) {
	transaction, owned, failure := e.ormDestroyTransaction(source)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	query := cloneORMQuery(source)
	query.transaction = transaction
	values, failure := e.ormLoad(query)
	if failure != nil {
		if owned {
			_ = e.ormRollbackTransaction(transaction)
		}
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	destroyed := int64(0)
	visited := map[string]bool{}
	for _, value := range values {
		removed, destroyFailure := e.ormDestroyObject(query, query.model, value, visited)
		if destroyFailure != nil {
			if owned {
				_ = e.ormRollbackTransaction(transaction)
			}
			return e.ormResultErr(resultType, destroyFailure.kind, destroyFailure.message)
		}
		if removed {
			destroyed++
		}
	}
	if owned {
		if failure := e.ormCommitTransaction(transaction); failure != nil {
			_ = e.ormRollbackTransaction(transaction)
			return e.ormResultErr(resultType, failure.kind, failure.message)
		}
	}
	return e.ormResultOK(resultType, Value{Type: types.FromName("Integer"), Data: destroyed})
}

func (e *Evaluator) ormDestroyTransaction(query *ormQueryValue) (*ormTransactionValue, bool, *ormFailure) {
	if query.transaction != nil {
		if !query.transaction.active() {
			return nil, false, &ormFailure{kind: "InvalidData", message: "database transaction is closed"}
		}
		return query.transaction, false, nil
	}
	transaction, failure := e.ormBeginTransaction(nil)
	return transaction, true, failure
}

func (e *Evaluator) ormDestroyObject(query *ormQueryValue, model ormintegration.Model, value Value, visited map[string]bool) (bool, *ormFailure) {
	object, ok := value.Data.(*objectInstance)
	if !ok {
		return false, &ormFailure{kind: "InvalidData", message: "database destroy value was invalid"}
	}
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return false, &ormFailure{kind: "InvalidData", message: "database destroy requires one primary key"}
	}
	key, ok := object.Fields["@"+primaryKey.Name]
	if !ok || key.Data == nil {
		return false, &ormFailure{kind: "InvalidData", message: "database destroy requires a primary key value"}
	}
	identity := model.Name + ":" + ormDestroyIdentity(key)
	if visited[identity] {
		return false, nil
	}
	visited[identity] = true

	for _, association := range model.Associations {
		if association.Dependent == "" {
			continue
		}
		target, ok := e.ormRuntime().manifest.Model(association.TargetModel)
		if !ok {
			return false, &ormFailure{kind: "InvalidData", message: "dependent association target is unavailable"}
		}
		sourceKey, ok := object.Fields["@"+association.SourceColumn]
		if !ok || sourceKey.Data == nil {
			continue
		}
		targetQuery := &ormQueryValue{
			model: target, transaction: query.transaction,
			predicate: &ormPredicate{kind: "atom", condition: ormCondition{column: association.TargetColumn, operator: "=", value: sourceKey}},
		}
		if association.Scope != nil {
			scoped, scopeErr := e.ormAssociationScope(targetQuery, association, model.ModulePath)
			if scopeErr != nil {
				return false, &ormFailure{kind: "InvalidData", message: scopeErr.Error()}
			}
			targetQuery = scoped
			targetQuery.transaction = query.transaction
		}
		children, failure := e.ormLoad(targetQuery)
		if failure != nil {
			return false, failure
		}
		switch association.Dependent {
		case ormintegration.DependentRestrict:
			if len(children) > 0 {
				return false, &ormFailure{kind: "Constraint", message: "dependent association restricts destroy"}
			}
		case ormintegration.DependentDestroy:
			for _, child := range children {
				if _, failure := e.ormDestroyObject(targetQuery, target, child, visited); failure != nil {
					return false, failure
				}
			}
		case ormintegration.DependentDelete:
			for _, child := range children {
				if _, failure := e.ormDeleteObjectRow(targetQuery, target, child); failure != nil {
					return false, failure
				}
			}
		case ormintegration.DependentNullify:
			for _, child := range children {
				if failure := e.ormNullifyObject(targetQuery, target, child, association.TargetColumn); failure != nil {
					return false, failure
				}
			}
		}
	}
	return e.ormDeleteObjectRow(query, model, value)
}

func (e *Evaluator) ormDeleteObjectRow(query *ormQueryValue, model ormintegration.Model, value Value) (bool, *ormFailure) {
	object, ok := value.Data.(*objectInstance)
	if !ok {
		return false, &ormFailure{kind: "InvalidData", message: "database delete value was invalid"}
	}
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return false, &ormFailure{kind: "InvalidData", message: "database delete requires one primary key"}
	}
	key, ok := object.Fields["@"+primaryKey.Name]
	if !ok || key.Data == nil {
		return false, &ormFailure{kind: "InvalidData", message: "database delete requires a primary key value"}
	}
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return false, failure
	}
	adapter := e.ormRuntime().adapter
	statement := "DELETE FROM " + adapter.QuoteIdentifier(model.Table) + " WHERE " + adapter.QuoteIdentifier(primaryKey.Name) + " = " + adapter.Placeholder(1)
	result, err := executor.ExecContext(e.context, statement, ormDatabaseValue(key))
	if err != nil {
		return false, &ormFailure{kind: "Constraint", message: "database destroy delete failed"}
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, &ormFailure{kind: "Query", message: "database destroy delete result was invalid"}
	}
	return affected > 0, nil
}

func (e *Evaluator) ormNullifyObject(query *ormQueryValue, model ormintegration.Model, value Value, column string) *ormFailure {
	object, ok := value.Data.(*objectInstance)
	if !ok {
		return &ormFailure{kind: "InvalidData", message: "database nullify value was invalid"}
	}
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return &ormFailure{kind: "InvalidData", message: "database nullify requires one primary key"}
	}
	key, ok := object.Fields["@"+primaryKey.Name]
	if !ok || key.Data == nil {
		return &ormFailure{kind: "InvalidData", message: "database nullify requires a primary key value"}
	}
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return failure
	}
	adapter := e.ormRuntime().adapter
	statement := "UPDATE " + adapter.QuoteIdentifier(model.Table) + " SET " + adapter.QuoteIdentifier(column) + " = NULL WHERE " + adapter.QuoteIdentifier(primaryKey.Name) + " = " + adapter.Placeholder(1)
	result, err := executor.ExecContext(e.context, statement, ormDatabaseValue(key))
	if err != nil {
		return &ormFailure{kind: "Constraint", message: "database dependent nullify failed"}
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return &ormFailure{kind: "InvalidData", message: "database dependent nullify did not affect one row"}
	}
	return nil
}

func (e *Evaluator) ormModelQuery(value Value, model ormintegration.Model) *ormQueryValue {
	if object, ok := value.Data.(*objectInstance); ok {
		if query, queryOK := object.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue); queryOK {
			return cloneORMQuery(query)
		}
	}
	return &ormQueryValue{model: model}
}

func ormDestroyIdentity(value Value) string {
	switch item := value.Data.(type) {
	case int64:
		return strconv.FormatInt(item, 10)
	case float64:
		return strconv.FormatFloat(item, 'g', -1, 64)
	case string:
		return item
	default:
		return Inspect(value)
	}
}
