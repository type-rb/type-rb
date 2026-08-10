//go:build !js || !wasm

package repl

import (
	"errors"
	"strings"

	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

func (e *Evaluator) ormWriteIntrinsic(name string, arguments []evaluatedArgument, typ types.Type) (Value, bool, error) {
	switch name {
	case "trb.orm.build", "trb.orm.scope.build":
		query, remaining, err := e.ormQueryReceiver(arguments)
		if err != nil {
			return Value{}, true, err
		}
		value, err := e.ormBuildDraft(typ, query, remaining)
		return value, true, err
	case "trb.orm.create", "trb.orm.scope.create":
		query, remaining, err := e.ormQueryReceiver(arguments)
		if err != nil {
			return Value{}, true, err
		}
		draft, err := e.ormBuildDraft(types.FromName(query.model.DraftType()), query, remaining)
		if err != nil {
			return Value{}, true, err
		}
		value, err := e.ormSaveDraft(typ, draft)
		return value, true, err
	case "trb.orm.draft.save":
		if len(arguments) != 1 {
			return Value{}, true, errors.New("ORM draft save requires a draft receiver")
		}
		value, err := e.ormSaveDraft(typ, arguments[0].Value)
		return value, true, err
	case "trb.orm.with":
		if len(arguments) == 0 {
			return Value{}, true, errors.New("ORM with requires a model receiver")
		}
		value, err := e.ormBuildChanges(typ, arguments[0].Value, arguments[1:])
		return value, true, err
	case "trb.orm.update":
		if len(arguments) == 0 {
			return Value{}, true, errors.New("ORM update requires a model receiver")
		}
		model, ok := e.ormModelForValue(arguments[0].Value)
		if !ok {
			return Value{}, true, errors.New("ORM update requires a persisted model")
		}
		changes, err := e.ormBuildChanges(types.FromName(model.ChangesType()), arguments[0].Value, arguments[1:])
		if err != nil {
			return Value{}, true, err
		}
		value, err := e.ormSaveChanges(typ, changes)
		return value, true, err
	case "trb.orm.changes.save":
		if len(arguments) != 1 {
			return Value{}, true, errors.New("ORM changes save requires a changes receiver")
		}
		value, err := e.ormSaveChanges(typ, arguments[0].Value)
		return value, true, err
	case "trb.orm.delete":
		if len(arguments) != 1 {
			return Value{}, true, errors.New("ORM delete requires a model receiver")
		}
		value, err := e.ormDeleteModel(typ, arguments[0].Value)
		return value, true, err
	case "trb.orm.query.update_all":
		query, remaining, err := e.ormQueryReceiver(arguments)
		if err != nil {
			return Value{}, true, err
		}
		value, err := e.ormUpdateAll(typ, query, remaining)
		return value, true, err
	case "trb.orm.query.delete_all":
		query, _, err := e.ormQueryReceiver(arguments)
		if err != nil {
			return Value{}, true, err
		}
		value, err := e.ormDeleteAll(typ, query)
		return value, true, err
	default:
		return Value{}, false, nil
	}
}

func (e *Evaluator) ormBuildDraft(typ types.Type, query *ormQueryValue, arguments []evaluatedArgument) (Value, error) {
	definition, ok := e.definitions[symbolKey(query.model.ModulePath, query.model.DraftType())].(*classDefinition)
	if !ok {
		return Value{}, errors.New("ORM draft runtime is not loaded")
	}
	fields := map[string]Value{
		"@__trb_orm_query_scope": {Type: types.FromName(query.model.QueryType), Data: cloneORMQuery(query)},
	}
	for _, argument := range arguments {
		if argument.Name == "" {
			return Value{}, errors.New("ORM build attributes must be keyword arguments")
		}
		fields["@"+argument.Name] = argument.Value
	}
	return Value{Type: typ, Data: &objectInstance{Definition: definition, Fields: fields}}, nil
}

func (e *Evaluator) ormBuildChanges(typ types.Type, source Value, arguments []evaluatedArgument) (Value, error) {
	model, ok := e.ormModelForValue(source)
	if !ok {
		return Value{}, errors.New("ORM with requires a persisted model")
	}
	definition, ok := e.definitions[symbolKey(model.ModulePath, model.ChangesType())].(*classDefinition)
	if !ok {
		return Value{}, errors.New("ORM changes runtime is not loaded")
	}
	fields := map[string]Value{"@__trb_orm_source": source}
	for _, argument := range arguments {
		if argument.Name == "" {
			return Value{}, errors.New("ORM changes must be keyword arguments")
		}
		fields["@"+argument.Name] = argument.Value
	}
	return Value{Type: typ, Data: &objectInstance{Definition: definition, Fields: fields}}, nil
}

func (e *Evaluator) ormSaveDraft(resultType types.Type, draft Value) (Value, error) {
	object, ok := draft.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("ORM save requires a draft")
	}
	model, query, ok := e.ormDraftModelAndQuery(object)
	if !ok {
		return Value{}, errors.New("ORM draft is missing its model scope")
	}
	columns, values := ormWriteValues(model, object.Fields)
	adapter := e.ormRuntime().adapter
	statement := "INSERT INTO " + adapter.QuoteIdentifier(model.Table)
	arguments := make([]any, len(values))
	if len(columns) == 0 {
		statement += adapter.DefaultInsert
	} else {
		quoted := make([]string, len(columns))
		placeholders := make([]string, len(columns))
		for index, column := range columns {
			quoted[index] = adapter.QuoteIdentifier(column)
			placeholders[index] = adapter.Placeholder(index + 1)
			arguments[index] = ormDatabaseValue(values[index])
		}
		statement += " (" + strings.Join(quoted, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ")"
	}
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	if adapter.InsertReturning {
		statement += " RETURNING " + e.ormModelProjection(model)
		rows, err := executor.QueryContext(e.context, statement, arguments...)
		if err != nil {
			return e.ormResultErr(resultType, "Constraint", "database insert failed")
		}
		defer rows.Close()
		if !rows.Next() {
			return e.ormResultErr(resultType, "InvalidData", "database insert did not return a row")
		}
		value, scanFailure := e.ormScanModel(rows, query)
		if scanFailure != nil {
			return e.ormResultErr(resultType, scanFailure.kind, scanFailure.message)
		}
		return e.ormResultOK(resultType, value)
	}
	result, err := executor.ExecContext(e.context, statement, arguments...)
	if err != nil {
		return e.ormResultErr(resultType, "Constraint", "database insert failed")
	}
	primaryKey, primaryOK := model.PrimaryKey()
	if !primaryOK {
		return e.ormResultErr(resultType, "InvalidData", "database insert cannot reload a model without one primary key")
	}
	key, provided := object.Fields["@"+primaryKey.Name]
	if !provided {
		identifier, identifierErr := result.LastInsertId()
		if identifierErr != nil || primaryKey.Type.Kind != types.Int {
			return e.ormResultErr(resultType, "InvalidData", "database insert did not return a portable primary key")
		}
		key = Value{Type: primaryKey.Type, Data: identifier}
	}
	return e.ormReloadByPrimaryKey(resultType, query, primaryKey, key)
}

func (e *Evaluator) ormSaveChanges(resultType types.Type, changes Value) (Value, error) {
	object, ok := changes.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("ORM changes save requires a changes value")
	}
	source, ok := object.Fields["@__trb_orm_source"]
	if !ok {
		return Value{}, errors.New("ORM changes are missing their source model")
	}
	model, ok := e.ormModelForValue(source)
	if !ok {
		return Value{}, errors.New("ORM changes source is not a persisted model")
	}
	sourceObject := source.Data.(*objectInstance)
	query, ok := sourceObject.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue)
	if !ok {
		query = &ormQueryValue{model: model}
	}
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return e.ormResultErr(resultType, "InvalidData", "database update requires one primary key")
	}
	key, ok := sourceObject.Fields["@"+primaryKey.Name]
	if !ok || key.Data == nil {
		return e.ormResultErr(resultType, "InvalidData", "database update requires a primary key value")
	}
	columns, values := ormWriteValues(model, object.Fields)
	if len(columns) == 0 {
		return e.ormResultOK(resultType, source)
	}
	adapter := e.ormRuntime().adapter
	assignments := make([]string, len(columns))
	arguments := make([]any, 0, len(values)+1)
	for index, column := range columns {
		assignments[index] = adapter.QuoteIdentifier(column) + " = " + adapter.Placeholder(index+1)
		arguments = append(arguments, ormDatabaseValue(values[index]))
	}
	arguments = append(arguments, ormDatabaseValue(key))
	statement := "UPDATE " + adapter.QuoteIdentifier(model.Table) + " SET " + strings.Join(assignments, ", ") +
		" WHERE " + adapter.QuoteIdentifier(primaryKey.Name) + " = " + adapter.Placeholder(len(arguments))
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	result, err := executor.ExecContext(e.context, statement, arguments...)
	if err != nil {
		return e.ormResultErr(resultType, "Constraint", "database update failed")
	}
	if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
		return e.ormResultErr(resultType, "InvalidData", "database update did not affect one row")
	}
	return e.ormReloadByPrimaryKey(resultType, query, primaryKey, key)
}

func (e *Evaluator) ormDeleteModel(resultType types.Type, value Value) (Value, error) {
	model, ok := e.ormModelForValue(value)
	if !ok {
		return Value{}, errors.New("ORM delete requires a persisted model")
	}
	object := value.Data.(*objectInstance)
	primaryKey, ok := model.PrimaryKey()
	if !ok {
		return e.ormResultErr(resultType, "InvalidData", "database delete requires one primary key")
	}
	key, ok := object.Fields["@"+primaryKey.Name]
	if !ok || key.Data == nil {
		return e.ormResultErr(resultType, "InvalidData", "database delete requires a primary key value")
	}
	query, _ := object.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue)
	if query == nil {
		query = &ormQueryValue{model: model}
	}
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	adapter := e.ormRuntime().adapter
	statement := "DELETE FROM " + adapter.QuoteIdentifier(model.Table) + " WHERE " + adapter.QuoteIdentifier(primaryKey.Name) + " = " + adapter.Placeholder(1)
	result, err := executor.ExecContext(e.context, statement, ormDatabaseValue(key))
	if err != nil {
		return e.ormResultErr(resultType, "Constraint", "database delete failed")
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return e.ormResultErr(resultType, "Query", "database delete result was invalid")
	}
	return e.ormResultOK(resultType, Value{Type: types.FromName("Boolean"), Data: affected > 0})
}

func (e *Evaluator) ormUpdateAll(resultType types.Type, query *ormQueryValue, values []evaluatedArgument) (Value, error) {
	if failure := ormBulkQueryFailure(query); failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	adapter := e.ormRuntime().adapter
	assignments := make([]string, len(values))
	arguments := make([]any, 0, len(values))
	for index, value := range values {
		if value.Name == "" {
			return Value{}, errors.New("ORM update_all attributes must be keyword arguments")
		}
		assignments[index] = adapter.QuoteIdentifier(value.Name) + " = " + adapter.Placeholder(index+1)
		arguments = append(arguments, ormDatabaseValue(value.Value))
	}
	if len(assignments) == 0 {
		return Value{}, errors.New("ORM update_all requires at least one attribute")
	}
	predicate, err := e.ormPredicateSQL(query.predicate, &arguments)
	if err != nil {
		return Value{}, err
	}
	statement := "UPDATE " + adapter.QuoteIdentifier(query.model.Table) + " SET " + strings.Join(assignments, ", ")
	if predicate != "" {
		statement += " WHERE " + predicate
	}
	return e.ormExecuteBulk(resultType, query, statement, arguments, "database bulk update failed")
}

func (e *Evaluator) ormDeleteAll(resultType types.Type, query *ormQueryValue) (Value, error) {
	if failure := ormBulkQueryFailure(query); failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	arguments := []any{}
	predicate, err := e.ormPredicateSQL(query.predicate, &arguments)
	if err != nil {
		return Value{}, err
	}
	statement := "DELETE FROM " + e.ormRuntime().adapter.QuoteIdentifier(query.model.Table)
	if predicate != "" {
		statement += " WHERE " + predicate
	}
	return e.ormExecuteBulk(resultType, query, statement, arguments, "database bulk delete failed")
}

func (e *Evaluator) ormExecuteBulk(resultType types.Type, query *ormQueryValue, statement string, arguments []any, message string) (Value, error) {
	executor, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	result, err := executor.ExecContext(e.context, statement, arguments...)
	if err != nil {
		return e.ormResultErr(resultType, "Constraint", message)
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return e.ormResultErr(resultType, "Query", message)
	}
	return e.ormResultOK(resultType, Value{Type: types.FromName("Integer"), Data: affected})
}

func ormBulkQueryFailure(query *ormQueryValue) *ormFailure {
	if len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || query.distinct || len(query.preloads) > 0 || len(query.joins) > 0 {
		return &ormFailure{kind: "InvalidData", message: "bulk writes do not accept distinct, joins, order, limit, offset, lock, or preload"}
	}
	return nil
}

func (e *Evaluator) ormReloadByPrimaryKey(resultType types.Type, query *ormQueryValue, primaryKey ormintegration.Column, key Value) (Value, error) {
	loaded := cloneORMQuery(query)
	loaded.predicate = &ormPredicate{kind: "atom", condition: ormCondition{column: primaryKey.Name, operator: "=", value: key}}
	loaded.orders = nil
	loaded.limit = nil
	loaded.offset = nil
	loaded.preloads = nil
	values, failure := e.ormLoad(loaded)
	if failure != nil {
		return e.ormResultErr(resultType, failure.kind, failure.message)
	}
	if len(values) != 1 {
		return e.ormResultErr(resultType, "InvalidData", "database write could not reload one row")
	}
	return e.ormResultOK(resultType, values[0])
}

func (e *Evaluator) ormDraftModelAndQuery(object *objectInstance) (ormintegration.Model, *ormQueryValue, bool) {
	for _, model := range e.ormRuntime().manifest.Models {
		if object.Definition.Node.Name != model.DraftType() {
			continue
		}
		query, ok := object.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue)
		if !ok {
			query = &ormQueryValue{model: model}
		}
		return model, query, true
	}
	return ormintegration.Model{}, nil, false
}

func (e *Evaluator) ormModelForValue(value Value) (ormintegration.Model, bool) {
	object, ok := value.Data.(*objectInstance)
	if !ok {
		return ormintegration.Model{}, false
	}
	return e.ormRuntime().manifest.Model(object.Definition.Node.Name)
}

func ormWriteValues(model ormintegration.Model, fields map[string]Value) ([]string, []Value) {
	columns := []string{}
	values := []Value{}
	for _, column := range model.Columns {
		value, exists := fields["@"+column.Name]
		if !exists || column.Generated {
			continue
		}
		columns = append(columns, column.Name)
		values = append(values, value)
	}
	return columns, values
}
