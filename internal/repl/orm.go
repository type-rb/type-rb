//go:build !js || !wasm

package repl

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	"github.com/type-rb/type-rb/internal/types"
)

type ormRuntime struct {
	manifest *ormintegration.Manifest
	adapter  ormintegration.Adapter
	database *sql.DB
}

type ormQueryValue struct {
	model       ormintegration.Model
	transaction *ormTransactionValue
	predicate   *ormPredicate
	orders      []ormOrder
	limit       *int64
	offset      *int64
	lock        bool
	preloads    []string
	joins       []ormJoin
}

type ormJoin struct {
	kind         string
	table        string
	sourceColumn string
	targetColumn string
	predicate    *ormPredicate
}

type ormSubqueryValue struct {
	query  *ormQueryValue
	column ormintegration.Column
}

type ormGroupedValue struct {
	query            *ormQueryValue
	column           ormintegration.Column
	orders           []ormOrder
	limit            *int64
	offset           *int64
	havingExpression string
	havingOperator   string
	havingValue      Value
}

type ormTransactionValue struct {
	transaction   *sql.Tx
	connection    *sql.Conn
	parent        *ormTransactionValue
	savepoint     string
	closed        bool
	nextSavepoint int
}

type ormQueryExecutor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type ormPredicate struct {
	kind      string
	condition ormCondition
	children  []*ormPredicate
	exists    *ormExistsPredicate
}

type ormExistsPredicate struct {
	negated      bool
	table        string
	sourceTable  string
	sourceColumn string
	targetColumn string
	predicate    *ormPredicate
}

type ormCondition struct {
	column   string
	operator string
	value    Value
}

type ormOrder struct {
	column    string
	direction string
}

type ormFailure struct {
	kind    string
	message string
}

func (failure *ormFailure) Error() string { return failure.message }

type ormRuntimeProvider struct {
	runtime *ormRuntime
}

func init() {
	registerRuntimeProvider(func() runtimeProvider { return &ormRuntimeProvider{} })
}

func (*ormRuntimeProvider) Name() string { return "trb/orm" }

func (*ormRuntimeProvider) Handles(intrinsic string) bool {
	return strings.HasPrefix(intrinsic, "trb.orm.")
}

func (provider *ormRuntimeProvider) Configure(programs []*ir.Program) error {
	var manifest *ormintegration.Manifest
	for _, program := range programs {
		if current := ormintegration.ManifestFrom(program.Extensions); current != nil {
			manifest = current
			break
		}
	}
	if manifest == nil {
		return nil
	}
	adapter, err := ormintegration.AdapterFor(manifest.Adapter)
	if err != nil {
		return err
	}
	if provider.runtime != nil && provider.runtime.manifest.Adapter == manifest.Adapter &&
		provider.runtime.manifest.Database == manifest.Database &&
		provider.runtime.manifest.DatabaseEnvironment == manifest.DatabaseEnvironment {
		provider.runtime.manifest = manifest
		provider.runtime.adapter = adapter
		return nil
	}
	if provider.runtime != nil && provider.runtime.database != nil {
		_ = provider.runtime.database.Close()
	}
	provider.runtime = &ormRuntime{manifest: manifest, adapter: adapter}
	return nil
}

func (provider *ormRuntimeProvider) Call(evaluator *Evaluator, invocation runtimeInvocation) (Value, error) {
	return evaluator.ormIntrinsic(invocation.Name, invocation.Arguments, invocation.Type, invocation.Call, invocation.MemberName)
}

func (provider *ormRuntimeProvider) Block(evaluator *Evaluator, invocation runtimeBlockInvocation) (Value, error) {
	if invocation.Name != "trb.orm.transaction" {
		return Value{}, fmt.Errorf("unsupported ORM structured block %s", invocation.Name)
	}
	var parent *ormTransactionValue
	for _, argument := range invocation.Arguments {
		if transaction, ok := argument.Value.Data.(*ormTransactionValue); ok {
			parent = transaction
			break
		}
	}
	transaction, failure := evaluator.ormBeginTransaction(parent)
	if failure != nil {
		return evaluator.ormResultErr(invocation.Type, failure.kind, failure.message)
	}
	value, err := invocation.Evaluate([]Value{{Type: types.FromName("Transaction"), Data: transaction}})
	if err != nil {
		_ = evaluator.ormRollbackTransaction(transaction)
		return Value{}, err
	}
	result, ok := value.Data.(*enumValue)
	if !ok || result.Name != "Ok" && result.Name != "Err" {
		_ = evaluator.ormRollbackTransaction(transaction)
		return Value{}, errors.New("ORM transaction block must return DbResult<T>")
	}
	if result.Name == "Err" {
		if rollbackFailure := evaluator.ormRollbackTransaction(transaction); rollbackFailure != nil {
			return evaluator.ormResultErr(invocation.Type, rollbackFailure.kind, rollbackFailure.message)
		}
		return value, nil
	}
	if commitFailure := evaluator.ormCommitTransaction(transaction); commitFailure != nil {
		_ = evaluator.ormRollbackTransaction(transaction)
		return evaluator.ormResultErr(invocation.Type, commitFailure.kind, commitFailure.message)
	}
	return value, nil
}

func (provider *ormRuntimeProvider) Close() error {
	if provider.runtime == nil || provider.runtime.database == nil {
		return nil
	}
	err := provider.runtime.database.Close()
	provider.runtime.database = nil
	return err
}

func (e *Evaluator) ormRuntime() *ormRuntime {
	provider, _ := e.runtimeProvider("trb/orm").(*ormRuntimeProvider)
	if provider == nil {
		return nil
	}
	return provider.runtime
}

func (e *Evaluator) ormBeginTransaction(parent *ormTransactionValue) (*ormTransactionValue, *ormFailure) {
	if parent != nil {
		if !parent.active() {
			return nil, &ormFailure{kind: "InvalidData", message: "database transaction is closed"}
		}
		root := parent.root()
		root.nextSavepoint++
		savepoint := "trb_savepoint_" + strconv.Itoa(root.nextSavepoint)
		if _, err := parent.executor().ExecContext(e.context, "SAVEPOINT "+savepoint); err != nil {
			return nil, &ormFailure{kind: "Query", message: "database savepoint failed to begin"}
		}
		return &ormTransactionValue{
			transaction: parent.transaction, connection: parent.connection,
			parent: parent, savepoint: savepoint,
		}, nil
	}
	database, failure := e.ormDatabase()
	if failure != nil {
		return nil, failure
	}
	if e.ormRuntime().adapter.Name == "sqlite" {
		connection, err := database.Conn(e.context)
		if err != nil {
			return nil, &ormFailure{kind: "Connection", message: "database connection failed"}
		}
		if _, err := connection.ExecContext(e.context, "BEGIN IMMEDIATE"); err != nil {
			_ = connection.Close()
			return nil, &ormFailure{kind: "Query", message: "database transaction failed to begin"}
		}
		return &ormTransactionValue{connection: connection}, nil
	}
	transaction, err := database.BeginTx(e.context, nil)
	if err != nil {
		return nil, &ormFailure{kind: "Query", message: "database transaction failed to begin"}
	}
	return &ormTransactionValue{transaction: transaction}, nil
}

func (transaction *ormTransactionValue) root() *ormTransactionValue {
	for transaction != nil && transaction.parent != nil {
		transaction = transaction.parent
	}
	return transaction
}

func (transaction *ormTransactionValue) active() bool {
	if transaction == nil || transaction.transaction == nil && transaction.connection == nil || transaction.closed {
		return false
	}
	for parent := transaction.parent; parent != nil; parent = parent.parent {
		if parent.closed {
			return false
		}
	}
	return true
}

func (transaction *ormTransactionValue) executor() ormQueryExecutor {
	if transaction.connection != nil {
		return transaction.connection
	}
	return transaction.transaction
}

func (e *Evaluator) ormCommitTransaction(transaction *ormTransactionValue) *ormFailure {
	if !transaction.active() {
		return &ormFailure{kind: "InvalidData", message: "database transaction is closed"}
	}
	if transaction.savepoint != "" {
		if _, err := transaction.executor().ExecContext(e.context, "RELEASE SAVEPOINT "+transaction.savepoint); err != nil {
			return &ormFailure{kind: "Query", message: "database savepoint failed to commit"}
		}
		transaction.closed = true
		return nil
	}
	if transaction.connection != nil {
		if _, err := transaction.connection.ExecContext(e.context, "COMMIT"); err != nil {
			return &ormFailure{kind: "Query", message: "database transaction failed to commit"}
		}
		transaction.closed = true
		_ = transaction.connection.Close()
		return nil
	}
	if err := transaction.transaction.Commit(); err != nil {
		return &ormFailure{kind: "Query", message: "database transaction failed to commit"}
	}
	transaction.closed = true
	return nil
}

func (e *Evaluator) ormRollbackTransaction(transaction *ormTransactionValue) *ormFailure {
	if transaction == nil || transaction.transaction == nil && transaction.connection == nil || transaction.closed {
		return nil
	}
	transaction.closed = true
	if transaction.savepoint != "" {
		if _, err := transaction.executor().ExecContext(e.context, "ROLLBACK TO SAVEPOINT "+transaction.savepoint); err != nil {
			return &ormFailure{kind: "Query", message: "database savepoint failed to roll back"}
		}
		if _, err := transaction.executor().ExecContext(e.context, "RELEASE SAVEPOINT "+transaction.savepoint); err != nil {
			return &ormFailure{kind: "Query", message: "database savepoint failed to release"}
		}
		return nil
	}
	if transaction.connection != nil {
		_, err := transaction.connection.ExecContext(e.context, "ROLLBACK")
		_ = transaction.connection.Close()
		if err != nil {
			return &ormFailure{kind: "Query", message: "database transaction failed to roll back"}
		}
		return nil
	}
	if err := transaction.transaction.Rollback(); err != nil {
		return &ormFailure{kind: "Query", message: "database transaction failed to roll back"}
	}
	return nil
}

func (e *Evaluator) ormIntrinsic(name string, arguments []evaluatedArgument, typ types.Type, call *ir.Call, memberName string) (Value, error) {
	if e.ormRuntime() == nil {
		return Value{}, errors.New("trb/orm is not configured for this REPL project")
	}
	if name == "trb.orm.column" {
		if len(arguments) != 1 {
			return Value{}, errors.New("ORM column access requires a model value")
		}
		return e.ormColumn(arguments[0].Value, memberName)
	}
	if strings.HasPrefix(name, "trb.orm.group.") {
		return e.ormGroupedIntrinsic(name, arguments, typ)
	}
	query, remaining, err := e.ormQueryReceiver(arguments)
	if err != nil {
		return Value{}, err
	}
	switch name {
	case "trb.orm.using":
		if len(remaining) != 1 {
			return Value{}, errors.New("ORM using requires one transaction")
		}
		transaction, ok := remaining[0].Value.Data.(*ormTransactionValue)
		if !ok || !transaction.active() {
			return Value{}, errors.New("ORM using requires an active transaction")
		}
		query.transaction = transaction
		return ormQueryResult(typ, query), nil
	case "trb.orm.where", "trb.orm.query.where":
		predicate, err := ormPredicateFromArguments(remaining)
		if err != nil {
			return Value{}, err
		}
		if err := ormValidateSubqueryScopes(query, predicate); err != nil {
			return Value{}, err
		}
		query.predicate = ormCombinePredicates("and", query.predicate, predicate)
		return ormQueryResult(typ, query), nil
	case "trb.orm.select", "trb.orm.query.select":
		if len(remaining) != 1 {
			return Value{}, errors.New("ORM select requires one column")
		}
		columnName, ok := remaining[0].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("ORM select column must be a literal name")
		}
		column, ok := query.model.Column(columnName)
		if !ok {
			return Value{}, fmt.Errorf("ORM model %s has no column %s", query.model.Name, columnName)
		}
		if query.lock || len(query.preloads) > 0 {
			return Value{}, errors.New("ORM select subquery does not accept lock or preload")
		}
		return Value{Type: typ, Data: &ormSubqueryValue{query: query, column: column}}, nil
	case "trb.orm.group", "trb.orm.query.group":
		if len(remaining) != 1 {
			return Value{}, errors.New("ORM group requires one column")
		}
		columnName, _ := remaining[0].Value.Data.(string)
		column, ok := query.model.Column(columnName)
		if !ok {
			return Value{}, fmt.Errorf("ORM model %s has no column %s", query.model.Name, columnName)
		}
		if query.lock || len(query.preloads) > 0 {
			return Value{}, errors.New("ORM group does not accept lock or preload")
		}
		for _, order := range query.orders {
			if order.column != column.Name {
				return Value{}, errors.New("ORM grouped order must use the group key")
			}
		}
		grouped := &ormGroupedValue{query: query, column: column, orders: append([]ormOrder(nil), query.orders...), limit: query.limit, offset: query.offset}
		grouped.query.orders = nil
		grouped.query.limit = nil
		grouped.query.offset = nil
		return Value{Type: typ, Data: grouped}, nil
	case "trb.orm.join", "trb.orm.left_join", "trb.orm.query.join", "trb.orm.query.left_join":
		if len(remaining) < 1 || len(remaining) > 2 {
			return Value{}, errors.New("ORM join requires an association and optional predicate query")
		}
		associationName, ok := remaining[0].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("ORM join association must be a literal name")
		}
		association, ok := query.model.Association(associationName)
		if !ok {
			return Value{}, fmt.Errorf("ORM model %s has no association %s", query.model.Name, associationName)
		}
		target, ok := e.ormRuntime().manifest.Model(association.TargetModel)
		if !ok {
			return Value{}, errors.New("ORM join target is not available")
		}
		var predicate *ormPredicate
		if len(remaining) == 2 {
			targetQuery, ok := remaining[1].Value.Data.(*ormQueryValue)
			if !ok || targetQuery.model.Name != target.Name {
				return Value{}, fmt.Errorf("ORM join %s requires a %s query", associationName, target.Name)
			}
			if targetQuery.transaction != nil {
				return Value{}, errors.New("ORM association predicate query must not have a transaction scope; scope the base query instead")
			}
			if ormQueryModified(targetQuery) || ormPredicateContainsExists(targetQuery.predicate) {
				return Value{}, errors.New("ORM association predicate query accepts only where, not, and or")
			}
			predicate = targetQuery.predicate
		}
		kind := "INNER JOIN"
		if name == "trb.orm.left_join" || name == "trb.orm.query.left_join" {
			kind = "LEFT JOIN"
		}
		query.joins = append(query.joins, ormJoin{
			kind: kind, table: target.Table,
			sourceColumn: association.SourceColumn, targetColumn: association.TargetColumn,
			predicate: predicate,
		})
		return ormQueryResult(typ, query), nil
	case "trb.orm.where_exists", "trb.orm.where_not_exists", "trb.orm.query.where_exists", "trb.orm.query.where_not_exists":
		if len(remaining) < 1 || len(remaining) > 2 {
			return Value{}, errors.New("ORM where_exists requires an association and optional predicate query")
		}
		associationName, ok := remaining[0].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("ORM where_exists association must be a literal name")
		}
		association, ok := query.model.Association(associationName)
		if !ok {
			return Value{}, fmt.Errorf("ORM model %s has no association %s", query.model.Name, associationName)
		}
		target, ok := e.ormRuntime().manifest.Model(association.TargetModel)
		if !ok {
			return Value{}, errors.New("ORM where_exists target is not available")
		}
		var predicate *ormPredicate
		if len(remaining) == 2 {
			targetQuery, ok := remaining[1].Value.Data.(*ormQueryValue)
			if !ok || targetQuery.model.Name != target.Name {
				return Value{}, fmt.Errorf("ORM where_exists %s requires a %s query", associationName, target.Name)
			}
			if targetQuery.transaction != nil {
				return Value{}, errors.New("ORM where_exists predicate query must not have a transaction scope; scope the base query instead")
			}
			if ormQueryModified(targetQuery) || ormPredicateContainsExists(targetQuery.predicate) {
				return Value{}, errors.New("ORM where_exists predicate query accepts only where, not, and or")
			}
			predicate = targetQuery.predicate
		}
		negated := name == "trb.orm.where_not_exists" || name == "trb.orm.query.where_not_exists"
		query.predicate = ormCombinePredicates("and", query.predicate, &ormPredicate{
			kind: "exists",
			exists: &ormExistsPredicate{
				negated: negated, table: target.Table, sourceTable: query.model.Table,
				sourceColumn: association.SourceColumn, targetColumn: association.TargetColumn,
				predicate: predicate,
			},
		})
		return ormQueryResult(typ, query), nil
	case "trb.orm.not", "trb.orm.query.not":
		predicate, err := ormPredicateFromArguments(remaining)
		if err != nil {
			return Value{}, err
		}
		if predicate == nil {
			return Value{}, errors.New("ORM not requires one condition")
		}
		if err := ormValidateSubqueryScopes(query, predicate); err != nil {
			return Value{}, err
		}
		query.predicate = ormCombinePredicates("and", query.predicate, &ormPredicate{kind: "not", children: []*ormPredicate{predicate}})
		return ormQueryResult(typ, query), nil
	case "trb.orm.query.or":
		if len(remaining) != 1 {
			return Value{}, errors.New("ORM or requires one query")
		}
		other, ok := remaining[0].Value.Data.(*ormQueryValue)
		if !ok || other.model.Name != query.model.Name {
			return Value{}, errors.New("ORM or requires a query for the same model")
		}
		if query.predicate == nil || other.predicate == nil {
			return Value{}, errors.New("ORM or requires conditions on both queries")
		}
		if ormQueryModified(query) || ormQueryModified(other) {
			return Value{}, errors.New("ORM or requires unmodified predicate queries; apply joins, order, limit, offset, lock, and preload after or")
		}
		if query.transaction != other.transaction {
			return Value{}, errors.New("ORM or requires queries from the same transaction scope")
		}
		query.predicate = ormCombinePredicates("or", query.predicate, other.predicate)
		return ormQueryResult(typ, query), nil
	case "trb.orm.query.order":
		for _, argument := range remaining {
			direction, ok := argument.Value.Data.(string)
			if !ok || direction != "asc" && direction != "desc" {
				return Value{}, errors.New("ORM order direction must be asc or desc")
			}
			query.orders = append(query.orders, ormOrder{column: argument.Name, direction: direction})
		}
		return ormQueryResult(typ, query), nil
	case "trb.orm.query.limit", "trb.orm.query.offset":
		if len(remaining) != 1 {
			return Value{}, fmt.Errorf("%s requires one count", strings.TrimPrefix(name, "trb.orm.query."))
		}
		count, ok := remaining[0].Value.Data.(int64)
		if !ok || count < 0 {
			return Value{}, fmt.Errorf("ORM %s must be non-negative", strings.TrimPrefix(name, "trb.orm.query."))
		}
		if name == "trb.orm.query.limit" {
			query.limit = &count
		} else {
			query.offset = &count
		}
		return ormQueryResult(typ, query), nil
	case "trb.orm.query.lock":
		query.lock = true
		return ormQueryResult(typ, query), nil
	case "trb.orm.query.preload":
		if len(remaining) != 1 {
			return Value{}, errors.New("ORM preload requires one association")
		}
		association, ok := remaining[0].Value.Data.(string)
		if !ok {
			return Value{}, errors.New("ORM preload association must be a literal name")
		}
		for _, existing := range query.preloads {
			if existing == association {
				return ormQueryResult(typ, query), nil
			}
		}
		query.preloads = append(query.preloads, association)
		return ormQueryResult(typ, query), nil
	case "trb.orm.find", "trb.orm.scope.find":
		primaryKey, ok := query.model.PrimaryKey()
		if !ok || len(remaining) != 1 {
			return Value{}, errors.New("ORM find requires a model with one primary key")
		}
		query.predicate = ormCombinePredicates("and", query.predicate, ormPredicateGroup([]ormCondition{{column: primaryKey.Name, operator: "=", value: remaining[0].Value}}))
		return e.ormFirstResult(typ, query)
	case "trb.orm.find_by", "trb.orm.query.find_by":
		predicate, err := ormPredicateFromArguments(remaining)
		if err != nil {
			return Value{}, err
		}
		if err := ormValidateSubqueryScopes(query, predicate); err != nil {
			return Value{}, err
		}
		query.predicate = ormCombinePredicates("and", query.predicate, predicate)
		return e.ormFirstResult(typ, query)
	case "trb.orm.exists", "trb.orm.query.exists":
		if name == "trb.orm.exists" {
			predicate, err := ormPredicateFromArguments(remaining)
			if err != nil {
				return Value{}, err
			}
			if err := ormValidateSubqueryScopes(query, predicate); err != nil {
				return Value{}, err
			}
			query.predicate = ormCombinePredicates("and", query.predicate, predicate)
		}
		exists, failure := e.ormExists(query)
		if failure != nil {
			return e.ormResultErr(typ, failure.kind, failure.message)
		}
		return e.ormResultOK(typ, Value{Type: types.FromName("Boolean"), Data: exists})
	case "trb.orm.query.all":
		values, failure := e.ormLoad(query)
		if failure != nil {
			return e.ormResultErr(typ, failure.kind, failure.message)
		}
		return e.ormResultOK(typ, Value{Type: ormResultValueType(typ), Data: &arrayValue{Items: values}})
	case "trb.orm.query.first":
		return e.ormFirstResult(typ, query)
	case "trb.orm.query.count":
		count, failure := e.ormCount(query)
		if failure != nil {
			return e.ormResultErr(typ, failure.kind, failure.message)
		}
		return e.ormResultOK(typ, Value{Type: types.FromName("Integer"), Data: count})
	case "trb.orm.query.to_sql":
		statement, _, err := e.ormStatement(query, e.ormModelProjection(query.model))
		if err != nil {
			return Value{}, err
		}
		return Value{Type: typ, Data: statement}, nil
	case "trb.orm.query.explain":
		detail, failure := e.ormExplain(query)
		if failure != nil {
			return e.ormResultErr(typ, failure.kind, failure.message)
		}
		return e.ormResultOK(typ, Value{Type: types.FromName("String"), Data: detail})
	case "trb.orm.pluck", "trb.orm.query.pluck", "trb.orm.pick", "trb.orm.query.pick", "trb.orm.ids", "trb.orm.query.ids":
		return e.ormProjectionResult(name, typ, query, remaining)
	case "trb.orm.sum", "trb.orm.query.sum", "trb.orm.average", "trb.orm.query.average", "trb.orm.minimum", "trb.orm.query.minimum", "trb.orm.maximum", "trb.orm.query.maximum":
		operation := name[strings.LastIndex(name, ".")+1:]
		return e.ormAggregateResult(operation, typ, query, remaining)
	case "trb.orm.association.query.belongs_to", "trb.orm.association.query.has_many", "trb.orm.association.query.has_one":
		return e.ormAssociationQuery(typ, query.model, arguments[0].Value, call)
	case "trb.orm.association.loaded.belongs_to", "trb.orm.association.loaded.has_many", "trb.orm.association.loaded.has_one":
		return e.ormLoadedAssociation(typ, arguments[0].Value, call)
	default:
		return Value{}, fmt.Errorf("%s is type-checked, but ORM writes and batch iteration are not executable in the REPL yet; use trb run", name)
	}
}

func (e *Evaluator) ormGroupedIntrinsic(name string, arguments []evaluatedArgument, typ types.Type) (Value, error) {
	if len(arguments) == 0 {
		return Value{}, errors.New("ORM grouped operation is missing its receiver")
	}
	grouped, ok := arguments[0].Value.Data.(*ormGroupedValue)
	if !ok {
		return Value{}, errors.New("ORM grouped operation requires a grouped query")
	}
	copy := *grouped
	if name == "trb.orm.group.having" {
		if len(arguments) < 4 || len(arguments) > 5 {
			return Value{}, errors.New("ORM having requires aggregate, operator, and value")
		}
		expression, operatorIndex, valueIndex := "COUNT(*)", 2, 3
		if len(arguments) == 5 {
			operation, _ := arguments[1].Value.Data.(string)
			expression, operatorIndex, valueIndex = groupedAggregateExpression(operation, "trb_value"), 3, 4
		}
		op, _ := arguments[operatorIndex].Value.Data.(string)
		copy.havingExpression = expression
		copy.havingOperator = op
		copy.havingValue = arguments[valueIndex].Value
		return Value{Type: typ, Data: &copy}, nil
	}
	operation := name[strings.LastIndex(name, ".")+1:]
	valueType := types.FromName("Integer")
	projection := e.ormRuntime().adapter.QuoteIdentifier(copy.column.Name) + " AS " + e.ormRuntime().adapter.QuoteIdentifier("trb_group")
	expression := "COUNT(*)"
	if operation != "count" {
		if len(arguments) != 2 {
			return Value{}, errors.New("ORM grouped aggregate requires one column")
		}
		columnName, _ := arguments[1].Value.Data.(string)
		target, ok := copy.query.model.Column(columnName)
		if !ok {
			return Value{}, fmt.Errorf("ORM model %s has no column %s", copy.query.model.Name, columnName)
		}
		var supported bool
		valueType, supported = ormintegration.AggregateResultType(operation, target)
		if !supported {
			return Value{}, fmt.Errorf("ORM %s does not support column %s", operation, columnName)
		}
		projection += ", " + e.ormRuntime().adapter.QuoteIdentifier(target.Name) + " AS " + e.ormRuntime().adapter.QuoteIdentifier("trb_value")
		expression = groupedAggregateExpression(operation, "trb_value")
	}
	statement, queryArguments, err := e.ormStatement(copy.query, projection)
	if err != nil {
		return Value{}, err
	}
	statement = "SELECT " + e.ormRuntime().adapter.QuoteIdentifier("trb_group") + ", " + expression + " FROM (" + statement + ") AS trb_grouped GROUP BY " + e.ormRuntime().adapter.QuoteIdentifier("trb_group")
	if copy.havingExpression != "" {
		statement += " HAVING " + copy.havingExpression + " " + copy.havingOperator + " " + e.ormRuntime().adapter.Placeholder(len(queryArguments)+1)
		queryArguments = append(queryArguments, copy.havingValue.Data)
	}
	if len(copy.orders) > 0 {
		orders := make([]string, len(copy.orders))
		for index, order := range copy.orders {
			orders[index] = e.ormRuntime().adapter.QuoteIdentifier("trb_group") + " " + strings.ToUpper(order.direction)
		}
		statement += " ORDER BY " + strings.Join(orders, ", ")
	}
	if copy.limit != nil {
		statement += " LIMIT " + e.ormRuntime().adapter.Placeholder(len(queryArguments)+1)
		queryArguments = append(queryArguments, *copy.limit)
	} else if copy.offset != nil {
		statement += e.ormRuntime().adapter.OffsetNoLimit
	}
	if copy.offset != nil {
		statement += " OFFSET " + e.ormRuntime().adapter.Placeholder(len(queryArguments)+1)
		queryArguments = append(queryArguments, *copy.offset)
	}
	database, failure := e.ormQueryExecutor(copy.query)
	if failure != nil {
		return e.ormResultErr(typ, failure.kind, failure.message)
	}
	rows, err := database.QueryContext(e.context, statement, queryArguments...)
	if err != nil {
		return e.ormResultErr(typ, "Query", "database grouped count query failed")
	}
	defer rows.Close()
	entries := []hashEntry{}
	for rows.Next() {
		var raw any
		var rawValue any
		if err := rows.Scan(&raw, &rawValue); err != nil {
			return e.ormResultErr(typ, "InvalidData", "database grouped count row was invalid")
		}
		key, err := ormColumnValue(copy.column.Type, raw)
		if err != nil {
			return e.ormResultErr(typ, "InvalidData", "database grouped count row was invalid")
		}
		value, err := ormColumnValue(valueType, rawValue)
		if err != nil {
			return e.ormResultErr(typ, "InvalidData", "database grouped aggregate row was invalid")
		}
		entries = append(entries, hashEntry{Key: key, Value: value})
	}
	if err := rows.Err(); err != nil {
		return e.ormResultErr(typ, "Query", "database grouped count query failed")
	}
	return e.ormResultOK(typ, Value{Type: ormResultValueType(typ), Data: &hashValue{Entries: entries}})
}

func groupedAggregateExpression(operation, column string) string {
	function := strings.ToUpper(operation)
	switch operation {
	case "average":
		function = "AVG"
	case "minimum":
		function = "MIN"
	case "maximum":
		function = "MAX"
	}
	expression := function + "(" + column + ")"
	if operation == "sum" {
		expression = "COALESCE(" + expression + ", 0)"
	}
	return expression
}

func (e *Evaluator) ormQueryReceiver(arguments []evaluatedArgument) (*ormQueryValue, []evaluatedArgument, error) {
	if len(arguments) == 0 {
		return nil, nil, errors.New("ORM operation is missing its receiver")
	}
	runtime := e.ormRuntime()
	switch receiver := arguments[0].Value.Data.(type) {
	case *typeValue:
		if receiver.Class == nil {
			return nil, nil, errors.New("ORM operation requires a model class")
		}
		model, ok := runtime.manifest.Model(receiver.Class.Node.Name)
		if !ok {
			return nil, nil, fmt.Errorf("ORM model %s is not available", receiver.Class.Node.Name)
		}
		return &ormQueryValue{model: model}, arguments[1:], nil
	case *ormQueryValue:
		return cloneORMQuery(receiver), arguments[1:], nil
	case *objectInstance:
		model, ok := runtime.manifest.Model(receiver.Definition.Node.Name)
		if !ok {
			return nil, nil, fmt.Errorf("ORM model %s is not available", receiver.Definition.Node.Name)
		}
		return &ormQueryValue{model: model}, arguments[1:], nil
	default:
		return nil, nil, errors.New("ORM operation requires a model or query receiver")
	}
}

func cloneORMQuery(query *ormQueryValue) *ormQueryValue {
	result := *query
	result.orders = append([]ormOrder(nil), query.orders...)
	result.preloads = append([]string(nil), query.preloads...)
	result.joins = append([]ormJoin(nil), query.joins...)
	return &result
}

func ormQueryResult(typ types.Type, query *ormQueryValue) Value {
	return Value{Type: typ, Data: query}
}

func ormQueryModified(query *ormQueryValue) bool {
	return len(query.orders) > 0 || query.limit != nil || query.offset != nil || query.lock || len(query.preloads) > 0 || len(query.joins) > 0
}

func ormPredicateContainsExists(predicate *ormPredicate) bool {
	if predicate == nil {
		return false
	}
	if predicate.kind == "exists" {
		return true
	}
	for _, child := range predicate.children {
		if ormPredicateContainsExists(child) {
			return true
		}
	}
	return false
}

func ormPredicateFromArguments(arguments []evaluatedArgument) (*ormPredicate, error) {
	if len(arguments) == 3 && arguments[0].Name == "" && arguments[1].Name == "" && arguments[2].Name == "" {
		column, columnOK := arguments[0].Value.Data.(string)
		operator, operatorOK := arguments[1].Value.Data.(string)
		if !columnOK || !operatorOK {
			return nil, errors.New("ORM comparison requires a column and operator")
		}
		if _, subquery := arguments[2].Value.Data.(*ormSubqueryValue); subquery {
			switch operator {
			case "=":
				operator = "IN"
			case "!=":
				operator = "NOT_IN"
			}
		}
		return ormPredicateGroup([]ormCondition{{column: column, operator: operator, value: arguments[2].Value}}), nil
	}
	conditions := make([]ormCondition, 0, len(arguments))
	for _, argument := range arguments {
		if argument.Name == "" {
			return nil, errors.New("ORM predicate arguments must be keywords or a column/operator/value triple")
		}
		operator := "="
		switch argument.Value.Data.(type) {
		case *arrayValue:
			operator = "IN"
		case *ormSubqueryValue:
			operator = "IN"
		case *rangeValue:
			operator = "RANGE_INCLUSIVE"
			if argument.Value.Data.(*rangeValue).Exclusive {
				operator = "RANGE_EXCLUSIVE"
			}
		}
		conditions = append(conditions, ormCondition{column: argument.Name, operator: operator, value: argument.Value})
	}
	return ormPredicateGroup(conditions), nil
}

func ormValidateSubqueryScopes(query *ormQueryValue, predicate *ormPredicate) error {
	if predicate == nil {
		return nil
	}
	if predicate.kind == "atom" {
		subquery, ok := predicate.condition.value.Data.(*ormSubqueryValue)
		if ok && subquery.query.transaction != nil && subquery.query.transaction != query.transaction {
			return errors.New("ORM subquery transaction scope must match the base query")
		}
		return nil
	}
	for _, child := range predicate.children {
		if err := ormValidateSubqueryScopes(query, child); err != nil {
			return err
		}
	}
	return nil
}

func ormPredicateGroup(conditions []ormCondition) *ormPredicate {
	if len(conditions) == 0 {
		return nil
	}
	if len(conditions) == 1 {
		return &ormPredicate{kind: "atom", condition: conditions[0]}
	}
	children := make([]*ormPredicate, len(conditions))
	for index := range conditions {
		children[index] = &ormPredicate{kind: "atom", condition: conditions[index]}
	}
	return &ormPredicate{kind: "and", children: children}
}

func ormCombinePredicates(kind string, left, right *ormPredicate) *ormPredicate {
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return &ormPredicate{kind: kind, children: []*ormPredicate{left, right}}
}

func (e *Evaluator) ormStatement(query *ormQueryValue, projection string) (string, []any, error) {
	arguments := []any{}
	statement, err := e.ormStatementAppend(query, projection, &arguments)
	return statement, arguments, err
}

func (e *Evaluator) ormStatementAppend(query *ormQueryValue, projection string, arguments *[]any) (string, error) {
	runtime := e.ormRuntime()
	statement := "SELECT " + projection + " FROM " + runtime.adapter.QuoteIdentifier(query.model.Table)
	for index, join := range query.joins {
		if join.kind != "INNER JOIN" && join.kind != "LEFT JOIN" {
			return "", errors.New("unsupported ORM join kind")
		}
		alias := "__trb_join_" + strconv.Itoa(index)
		key := "__trb_join_key"
		subquery := "SELECT " + runtime.adapter.QuoteIdentifier(join.targetColumn) + " AS " + runtime.adapter.QuoteIdentifier(key) +
			" FROM " + runtime.adapter.QuoteIdentifier(join.table)
		if join.predicate != nil {
			predicate, err := e.ormPredicateSQL(join.predicate, arguments)
			if err != nil {
				return "", err
			}
			if predicate != "" {
				subquery += " WHERE " + predicate
			}
		}
		statement += " " + join.kind + " (" + subquery + ") AS " + runtime.adapter.QuoteIdentifier(alias) +
			" ON " + runtime.adapter.QuoteIdentifier(join.sourceColumn) + " = " + runtime.adapter.QuoteIdentifier(alias) +
			"." + runtime.adapter.QuoteIdentifier(key)
	}
	if query.predicate != nil {
		predicate, err := e.ormPredicateSQL(query.predicate, arguments)
		if err != nil {
			return "", err
		}
		statement += " WHERE " + predicate
	}
	if len(query.orders) > 0 {
		orders := make([]string, len(query.orders))
		for index, order := range query.orders {
			orders[index] = runtime.adapter.QuoteIdentifier(order.column) + " " + strings.ToUpper(order.direction)
		}
		statement += " ORDER BY " + strings.Join(orders, ", ")
	}
	if query.limit != nil {
		statement += " LIMIT " + runtime.adapter.Placeholder(len(*arguments)+1)
		*arguments = append(*arguments, *query.limit)
	} else if query.offset != nil {
		statement += runtime.adapter.OffsetNoLimit
	}
	if query.offset != nil {
		statement += " OFFSET " + runtime.adapter.Placeholder(len(*arguments)+1)
		*arguments = append(*arguments, *query.offset)
	}
	if query.lock && runtime.adapter.Name != "sqlite" {
		statement += " FOR UPDATE"
	}
	return statement, nil
}

func (e *Evaluator) ormPredicateSQL(predicate *ormPredicate, arguments *[]any) (string, error) {
	if predicate == nil {
		return "", nil
	}
	runtime := e.ormRuntime()
	switch predicate.kind {
	case "atom":
		condition := predicate.condition
		column := runtime.adapter.QuoteIdentifier(condition.column)
		switch condition.operator {
		case "IN", "NOT_IN":
			if subquery, ok := condition.value.Data.(*ormSubqueryValue); ok {
				statement, err := e.ormStatementAppend(subquery.query, e.ormRuntime().adapter.QuoteIdentifier(subquery.column.Name), arguments)
				if err != nil {
					return "", err
				}
				operator := " IN "
				if condition.operator == "NOT_IN" {
					operator = " NOT IN "
				}
				return column + operator + "(" + statement + ")", nil
			}
			array, ok := condition.value.Data.(*arrayValue)
			if !ok {
				return "", errors.New("ORM IN predicate requires an Array")
			}
			if len(array.Items) == 0 {
				return "1 = 0", nil
			}
			placeholders := make([]string, len(array.Items))
			for index, item := range array.Items {
				placeholders[index] = runtime.adapter.Placeholder(len(*arguments) + 1)
				*arguments = append(*arguments, ormDatabaseValue(item))
			}
			return column + " IN (" + strings.Join(placeholders, ", ") + ")", nil
		case "RANGE_INCLUSIVE", "RANGE_EXCLUSIVE":
			bounds, ok := condition.value.Data.(*rangeValue)
			if !ok {
				return "", errors.New("ORM range predicate requires a Range")
			}
			lower := runtime.adapter.Placeholder(len(*arguments) + 1)
			*arguments = append(*arguments, bounds.Start)
			upper := runtime.adapter.Placeholder(len(*arguments) + 1)
			*arguments = append(*arguments, bounds.End)
			upperOperator := "<="
			if condition.operator == "RANGE_EXCLUSIVE" {
				upperOperator = "<"
			}
			return "(" + column + " >= " + lower + " AND " + column + " " + upperOperator + " " + upper + ")", nil
		case "=", "!=", "<", "<=", ">", ">=":
		default:
			return "", errors.New("unsupported ORM comparison operator")
		}
		if condition.value.Data == nil && condition.operator == "=" {
			return column + " IS NULL", nil
		}
		if condition.value.Data == nil && condition.operator == "!=" {
			return column + " IS NOT NULL", nil
		}
		placeholder := runtime.adapter.Placeholder(len(*arguments) + 1)
		*arguments = append(*arguments, ormDatabaseValue(condition.value))
		return column + " " + condition.operator + " " + placeholder, nil
	case "and", "or":
		clauses := make([]string, len(predicate.children))
		for index, child := range predicate.children {
			clause, err := e.ormPredicateSQL(child, arguments)
			if err != nil {
				return "", err
			}
			clauses[index] = clause
		}
		join := " AND "
		if predicate.kind == "or" {
			join = " OR "
		}
		return "(" + strings.Join(clauses, join) + ")", nil
	case "not":
		if len(predicate.children) != 1 {
			return "", errors.New("invalid ORM not predicate")
		}
		clause, err := e.ormPredicateSQL(predicate.children[0], arguments)
		if err != nil {
			return "", err
		}
		return "NOT (" + clause + ")", nil
	case "exists":
		if predicate.exists == nil {
			return "", errors.New("invalid ORM exists predicate")
		}
		exists := predicate.exists
		correlation := runtime.adapter.QuoteIdentifier(exists.table) + "." + runtime.adapter.QuoteIdentifier(exists.targetColumn) +
			" = " + runtime.adapter.QuoteIdentifier(exists.sourceTable) + "." + runtime.adapter.QuoteIdentifier(exists.sourceColumn)
		statement := "SELECT 1 FROM " + runtime.adapter.QuoteIdentifier(exists.table) + " WHERE " + correlation
		if exists.predicate != nil {
			clause, err := e.ormPredicateSQL(exists.predicate, arguments)
			if err != nil {
				return "", err
			}
			if clause != "" {
				statement += " AND (" + clause + ")"
			}
		}
		operator := "EXISTS"
		if exists.negated {
			operator = "NOT EXISTS"
		}
		return operator + " (" + statement + ")", nil
	default:
		return "", errors.New("unsupported ORM predicate")
	}
}

func ormDatabaseValue(value Value) any {
	switch data := value.Data.(type) {
	case bytesValue:
		return []byte(data)
	default:
		return data
	}
}

func (e *Evaluator) ormDatabase() (*sql.DB, *ormFailure) {
	runtime := e.ormRuntime()
	if runtime.database != nil {
		return runtime.database, nil
	}
	databaseSource := runtime.manifest.Database
	if environment := runtime.manifest.DatabaseEnvironment; environment != "" {
		value, found := os.LookupEnv(environment)
		if !found || strings.TrimSpace(value) == "" {
			return nil, &ormFailure{kind: "Connection", message: "database environment variable is not set or empty"}
		}
		databaseSource = value
	}
	database, err := sql.Open(runtime.adapter.DriverName, databaseSource)
	if err == nil {
		err = database.PingContext(e.context)
	}
	if err != nil {
		if database != nil {
			_ = database.Close()
		}
		return nil, &ormFailure{kind: "Connection", message: "database connection failed"}
	}
	runtime.database = database
	return database, nil
}

func (e *Evaluator) ormQueryExecutor(query *ormQueryValue) (ormQueryExecutor, *ormFailure) {
	if query.lock && query.transaction == nil {
		return nil, &ormFailure{kind: "InvalidData", message: "database lock requires an explicit transaction scope"}
	}
	if query.transaction != nil {
		if !query.transaction.active() {
			return nil, &ormFailure{kind: "InvalidData", message: "database transaction is closed"}
		}
		return query.transaction.executor(), nil
	}
	return e.ormDatabase()
}

func (e *Evaluator) ormLoad(query *ormQueryValue) ([]Value, *ormFailure) {
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return nil, failure
	}
	statement, arguments, err := e.ormStatement(query, e.ormModelProjection(query.model))
	if err != nil {
		return nil, &ormFailure{kind: "InvalidData", message: err.Error()}
	}
	rows, err := database.QueryContext(e.context, statement, arguments...)
	if err != nil {
		return nil, &ormFailure{kind: "Query", message: "database query failed"}
	}
	defer rows.Close()
	values := []Value{}
	for rows.Next() {
		value, failure := e.ormScanModel(rows, query)
		if failure != nil {
			return nil, failure
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, &ormFailure{kind: "Query", message: "database query failed"}
	}
	if err := rows.Close(); err != nil {
		return nil, &ormFailure{kind: "Query", message: "database query failed"}
	}
	for _, preload := range query.preloads {
		if failure := e.ormPreload(values, query.model, preload); failure != nil {
			return nil, failure
		}
	}
	return values, nil
}

func (e *Evaluator) ormScanModel(rows *sql.Rows, query *ormQueryValue) (Value, *ormFailure) {
	model := query.model
	definition, ok := e.definitions[symbolKey(model.ModulePath, model.Name)].(*classDefinition)
	if !ok {
		return Value{}, &ormFailure{kind: "InvalidData", message: "ORM model runtime is not loaded"}
	}
	raw := make([]any, len(model.Columns))
	targets := make([]any, len(raw))
	for index := range raw {
		targets[index] = &raw[index]
	}
	if err := rows.Scan(targets...); err != nil {
		return Value{}, &ormFailure{kind: "InvalidData", message: "database row was invalid"}
	}
	fields := map[string]Value{}
	for _, field := range allFields(definition) {
		value := Value{Type: field.Type}
		if strings.HasSuffix(field.Name, "_loaded") {
			value.Data = false
		}
		fields[field.Name] = value
	}
	for index, column := range model.Columns {
		value, err := ormColumnValue(column.Type, raw[index])
		if err != nil {
			return Value{}, &ormFailure{kind: "InvalidData", message: "database row was invalid"}
		}
		fields["@"+column.Name] = value
	}
	fields["@__trb_orm_query_scope"] = Value{Type: types.FromName(model.QueryType), Data: cloneORMQuery(query)}
	return Value{Type: types.FromName(model.Name), Data: &objectInstance{Definition: definition, Fields: fields}}, nil
}

func ormColumnValue(typ types.Type, raw any) (Value, error) {
	value := Value{Type: typ}
	if raw == nil {
		if !typ.Nullable {
			return Value{}, errors.New("non-nullable database column contained NULL")
		}
		return value, nil
	}
	switch typ.Kind {
	case types.Int:
		integer, ok := ormInteger(raw)
		if !ok || integer < -9007199254740991 || integer > 9007199254740991 {
			return Value{}, errors.New("database Integer is outside the portable range")
		}
		value.Data = integer
	case types.Float:
		floating, ok := ormFloat(raw)
		if !ok || math.IsInf(floating, 0) || math.IsNaN(floating) {
			return Value{}, errors.New("database Float is invalid")
		}
		value.Data = floating
	case types.String:
		switch item := raw.(type) {
		case string:
			value.Data = item
		case []byte:
			value.Data = string(item)
		default:
			return Value{}, errors.New("database String is invalid")
		}
	case types.Bool:
		switch item := raw.(type) {
		case bool:
			value.Data = item
		default:
			integer, ok := ormInteger(raw)
			if !ok || integer != 0 && integer != 1 {
				return Value{}, errors.New("database Boolean is invalid")
			}
			value.Data = integer == 1
		}
	case types.Bytes:
		switch item := raw.(type) {
		case []byte:
			value.Data = bytesValue(append([]byte(nil), item...))
		case string:
			value.Data = bytesValue([]byte(item))
		default:
			return Value{}, errors.New("database Bytes is invalid")
		}
	default:
		return Value{}, fmt.Errorf("unsupported ORM column type %s", typ)
	}
	return value, nil
}

func ormInteger(value any) (int64, bool) {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return reflected.Int(), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		unsigned := reflected.Uint()
		if unsigned > math.MaxInt64 {
			return 0, false
		}
		return int64(unsigned), true
	case reflect.String:
		parsed, err := strconv.ParseInt(reflected.String(), 10, 64)
		return parsed, err == nil
	case reflect.Slice:
		if bytes, ok := value.([]byte); ok {
			parsed, err := strconv.ParseInt(string(bytes), 10, 64)
			return parsed, err == nil
		}
	}
	return 0, false
}

func ormFloat(value any) (float64, bool) {
	reflected := reflect.ValueOf(value)
	switch reflected.Kind() {
	case reflect.Float32, reflect.Float64:
		return reflected.Float(), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(reflected.Int()), true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(reflected.Uint()), true
	case reflect.String:
		parsed, err := strconv.ParseFloat(reflected.String(), 64)
		return parsed, err == nil
	case reflect.Slice:
		if bytes, ok := value.([]byte); ok {
			parsed, err := strconv.ParseFloat(string(bytes), 64)
			return parsed, err == nil
		}
	}
	return 0, false
}

func (e *Evaluator) ormFirstResult(typ types.Type, query *ormQueryValue) (Value, error) {
	query = cloneORMQuery(query)
	if query.limit == nil || *query.limit > 1 {
		limit := int64(1)
		query.limit = &limit
	}
	values, failure := e.ormLoad(query)
	if failure != nil {
		return e.ormResultErr(typ, failure.kind, failure.message)
	}
	value := Value{Type: ormResultValueType(typ)}
	if len(values) > 0 {
		value = values[0]
		value.Type = ormResultValueType(typ)
	}
	return e.ormResultOK(typ, value)
}

func (e *Evaluator) ormExists(query *ormQueryValue) (bool, *ormFailure) {
	statement, arguments, err := e.ormStatement(query, "1")
	if err != nil {
		return false, &ormFailure{kind: "InvalidData", message: err.Error()}
	}
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return false, failure
	}
	row := database.QueryRowContext(e.context, "SELECT 1 FROM ("+statement+") AS trb_exists LIMIT 1", arguments...)
	var found int
	err = row.Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, &ormFailure{kind: "Query", message: "database existence query failed"}
	}
	return true, nil
}

func (e *Evaluator) ormCount(query *ormQueryValue) (int64, *ormFailure) {
	statement, arguments, err := e.ormStatement(query, "1")
	if err != nil {
		return 0, &ormFailure{kind: "InvalidData", message: err.Error()}
	}
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return 0, failure
	}
	row := database.QueryRowContext(e.context, "SELECT COUNT(*) FROM ("+statement+") AS trb_count", arguments...)
	var count int64
	if err := row.Scan(&count); err != nil {
		return 0, &ormFailure{kind: "Query", message: "database count failed"}
	}
	return count, nil
}

func (e *Evaluator) ormProjectionResult(name string, typ types.Type, query *ormQueryValue, arguments []evaluatedArgument) (Value, error) {
	columnName := ""
	if strings.HasSuffix(name, ".ids") {
		primaryKey, ok := query.model.PrimaryKey()
		if !ok {
			return Value{}, errors.New("ORM ids requires a model with one primary key")
		}
		columnName = primaryKey.Name
	} else {
		if len(arguments) != 1 {
			return Value{}, errors.New("ORM projection requires one column")
		}
		columnName, _ = arguments[0].Value.Data.(string)
	}
	column, ok := query.model.Column(columnName)
	if !ok {
		return Value{}, fmt.Errorf("ORM model %s has no column %s", query.model.Name, columnName)
	}
	pick := strings.Contains(name, ".pick")
	if pick {
		query = cloneORMQuery(query)
		limit := int64(1)
		if query.limit == nil || *query.limit > 1 {
			query.limit = &limit
		}
	}
	values, failure := e.ormLoadProjection(query, column)
	if failure != nil {
		return e.ormResultErr(typ, failure.kind, failure.message)
	}
	if pick {
		value := Value{Type: ormResultValueType(typ)}
		if len(values) > 0 {
			value = values[0]
			value.Type = ormResultValueType(typ)
		}
		return e.ormResultOK(typ, value)
	}
	return e.ormResultOK(typ, Value{Type: ormResultValueType(typ), Data: &arrayValue{Items: values}})
}

func (e *Evaluator) ormLoadProjection(query *ormQueryValue, column ormintegration.Column) ([]Value, *ormFailure) {
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return nil, failure
	}
	statement, arguments, err := e.ormStatement(query, e.ormRuntime().adapter.QuoteIdentifier(column.Name))
	if err != nil {
		return nil, &ormFailure{kind: "InvalidData", message: err.Error()}
	}
	rows, err := database.QueryContext(e.context, statement, arguments...)
	if err != nil {
		return nil, &ormFailure{kind: "Query", message: "database projection query failed"}
	}
	defer rows.Close()
	values := []Value{}
	for rows.Next() {
		var raw any
		if err := rows.Scan(&raw); err != nil {
			return nil, &ormFailure{kind: "InvalidData", message: "database projection row was invalid"}
		}
		value, err := ormColumnValue(column.Type, raw)
		if err != nil {
			return nil, &ormFailure{kind: "InvalidData", message: "database projection row was invalid"}
		}
		values = append(values, value)
	}
	if err := rows.Err(); err != nil {
		return nil, &ormFailure{kind: "Query", message: "database projection query failed"}
	}
	return values, nil
}

func (e *Evaluator) ormAggregateResult(operation string, typ types.Type, query *ormQueryValue, arguments []evaluatedArgument) (Value, error) {
	if len(arguments) != 1 {
		return Value{}, errors.New("ORM aggregate requires one column")
	}
	columnName, _ := arguments[0].Value.Data.(string)
	column, ok := query.model.Column(columnName)
	if !ok {
		return Value{}, fmt.Errorf("ORM model %s has no column %s", query.model.Name, columnName)
	}
	resultType, supported := ormintegration.AggregateResultType(operation, column)
	if !supported {
		return Value{}, fmt.Errorf("ORM %s does not support column %s", operation, columnName)
	}
	runtime := e.ormRuntime()
	quotedValue := runtime.adapter.QuoteIdentifier("trb_value")
	projection := runtime.adapter.QuoteIdentifier(column.Name) + " AS " + quotedValue
	statement, queryArguments, err := e.ormStatement(query, projection)
	if err != nil {
		return Value{}, err
	}
	function := strings.ToUpper(operation)
	switch operation {
	case "average":
		function = "AVG"
	case "minimum":
		function = "MIN"
	case "maximum":
		function = "MAX"
	}
	expression := function + "(" + quotedValue + ")"
	if operation == "sum" {
		expression = "COALESCE(" + expression + ", 0)"
	}
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return e.ormResultErr(typ, failure.kind, failure.message)
	}
	rows, err := database.QueryContext(e.context, "SELECT "+expression+" FROM ("+statement+") AS trb_aggregate", queryArguments...)
	if err != nil {
		return e.ormResultErr(typ, "Query", "database aggregate query failed")
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return e.ormResultErr(typ, "Query", "database aggregate query failed")
		}
		return e.ormResultErr(typ, "InvalidData", "database aggregate result was missing")
	}
	var raw any
	if err := rows.Scan(&raw); err != nil {
		return e.ormResultErr(typ, "InvalidData", "database aggregate result was invalid")
	}
	if err := rows.Err(); err != nil {
		return e.ormResultErr(typ, "Query", "database aggregate query failed")
	}
	value, err := ormColumnValue(resultType, raw)
	if err != nil {
		return e.ormResultErr(typ, "InvalidData", "database aggregate result was invalid")
	}
	return e.ormResultOK(typ, value)
}

func (e *Evaluator) ormExplain(query *ormQueryValue) (string, *ormFailure) {
	statement, arguments, err := e.ormStatement(query, e.ormModelProjection(query.model))
	if err != nil {
		return "", &ormFailure{kind: "InvalidData", message: err.Error()}
	}
	prefix := "EXPLAIN QUERY PLAN "
	runtime := e.ormRuntime()
	switch runtime.adapter.ExplainStyle {
	case ormintegration.ExplainText:
		prefix = "EXPLAIN "
	case ormintegration.ExplainJSON:
		prefix = "EXPLAIN FORMAT=JSON "
	}
	database, failure := e.ormQueryExecutor(query)
	if failure != nil {
		return "", failure
	}
	rows, err := database.QueryContext(e.context, prefix+statement, arguments...)
	if err != nil {
		return "", &ormFailure{kind: "Query", message: "database explain failed"}
	}
	defer rows.Close()
	columns, err := rows.Columns()
	if err != nil {
		return "", &ormFailure{kind: "InvalidData", message: "database explain result was invalid"}
	}
	details := []string{}
	for rows.Next() {
		raw := make([]any, len(columns))
		targets := make([]any, len(raw))
		for index := range raw {
			targets[index] = &raw[index]
		}
		if err := rows.Scan(targets...); err != nil {
			return "", &ormFailure{kind: "InvalidData", message: "database explain result was invalid"}
		}
		if runtime.adapter.ExplainStyle == ormintegration.ExplainSQLite && len(raw) >= 4 {
			details = append(details, ormExplainText(raw[3]))
			continue
		}
		parts := make([]string, len(raw))
		for index, value := range raw {
			parts[index] = ormExplainText(value)
		}
		details = append(details, strings.Join(parts, "\t"))
	}
	if err := rows.Err(); err != nil {
		return "", &ormFailure{kind: "Query", message: "database explain failed"}
	}
	return strings.Join(details, "\n"), nil
}

func ormExplainText(value any) string {
	if bytes, ok := value.([]byte); ok {
		return string(bytes)
	}
	return fmt.Sprint(value)
}

func (e *Evaluator) ormPreload(values []Value, model ormintegration.Model, name string) *ormFailure {
	association, ok := model.Association(name)
	if !ok || !association.Preloadable {
		return &ormFailure{kind: "InvalidData", message: "unsupported ORM preload " + name}
	}
	target, ok := e.ormRuntime().manifest.Model(association.TargetModel)
	if !ok {
		return &ormFailure{kind: "InvalidData", message: "ORM association target is not available"}
	}
	items := []Value{}
	seen := map[string]bool{}
	for _, value := range values {
		object := value.Data.(*objectInstance)
		key := object.Fields["@"+association.SourceColumn]
		if key.Data == nil {
			continue
		}
		encoded := ormValueKey(key)
		if !seen[encoded] {
			seen[encoded] = true
			items = append(items, key)
		}
	}
	condition := Value{Type: types.Type{Kind: types.Array, Name: "Array"}, Data: &arrayValue{Items: items}}
	query := &ormQueryValue{model: target, predicate: ormPredicateGroup([]ormCondition{{column: association.TargetColumn, operator: "IN", value: condition}})}
	if len(values) > 0 {
		if source, ok := values[0].Data.(*objectInstance); ok {
			if scope, ok := source.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue); ok {
				query.transaction = scope.transaction
			}
		}
	}
	related, failure := e.ormLoad(query)
	if failure != nil {
		return &ormFailure{kind: failure.kind, message: "database preload failed"}
	}
	grouped := map[string][]Value{}
	for _, value := range related {
		object := value.Data.(*objectInstance)
		key := object.Fields["@"+association.TargetColumn]
		grouped[ormValueKey(key)] = append(grouped[ormValueKey(key)], value)
	}
	for _, value := range values {
		object := value.Data.(*objectInstance)
		key := object.Fields["@"+association.SourceColumn]
		matches := grouped[ormValueKey(key)]
		loadedField := "@__trb_association_" + name + "_loaded"
		valueField := "@__trb_association_" + name
		object.Fields[loadedField] = Value{Type: types.FromName("Boolean"), Data: true}
		if association.Kind == ormintegration.HasMany {
			itemType := types.FromName(target.Name)
			object.Fields[valueField] = Value{Type: types.Type{Kind: types.Array, Name: "Array", Args: []types.Type{itemType}}, Data: &arrayValue{Items: matches}}
		} else {
			if association.Kind == ormintegration.HasOne && len(matches) > 1 {
				return &ormFailure{kind: "InvalidData", message: "database has_one association returned multiple rows"}
			}
			associationType := types.FromName(target.Name)
			associationType.Nullable = true
			loaded := Value{Type: associationType}
			if len(matches) > 0 {
				loaded = matches[0]
				loaded.Type = associationType
			}
			object.Fields[valueField] = loaded
		}
	}
	return nil
}

func ormValueKey(value Value) string {
	if value.Data == nil {
		return "nil"
	}
	if bytes, ok := value.Data.(bytesValue); ok {
		return "bytes:" + string(bytes)
	}
	return fmt.Sprintf("%T:%v", value.Data, value.Data)
}

func (e *Evaluator) ormAssociationQuery(typ types.Type, model ormintegration.Model, receiver Value, call *ir.Call) (Value, error) {
	object, ok := receiver.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("ORM association query requires a model value")
	}
	name := ormCallMemberName(call)
	name = strings.TrimSuffix(name, "_query")
	association, ok := model.Association(name)
	if !ok {
		return Value{}, fmt.Errorf("ORM model %s has no association %s", model.Name, name)
	}
	target, ok := e.ormRuntime().manifest.Model(association.TargetModel)
	if !ok {
		return Value{}, errors.New("ORM association target is not available")
	}
	value := object.Fields["@"+association.SourceColumn]
	query := &ormQueryValue{model: target, predicate: ormPredicateGroup([]ormCondition{{column: association.TargetColumn, operator: "=", value: value}})}
	if scope, ok := object.Fields["@__trb_orm_query_scope"].Data.(*ormQueryValue); ok {
		query.transaction = scope.transaction
	}
	return ormQueryResult(typ, query), nil
}

func (e *Evaluator) ormLoadedAssociation(typ types.Type, receiver Value, call *ir.Call) (Value, error) {
	object, ok := receiver.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("ORM association requires a model value")
	}
	name := ormCallMemberName(call)
	loaded, ok := object.Fields["@__trb_association_"+name+"_loaded"]
	if !ok || loaded.Data != true {
		return Value{}, fmt.Errorf("ORM association %s.%s was not preloaded", object.Definition.Node.Name, name)
	}
	value := object.Fields["@__trb_association_"+name]
	value.Type = typ
	return value, nil
}

func ormCallMemberName(call *ir.Call) string {
	if call == nil {
		return ""
	}
	if member, ok := call.Callee.(*ir.Member); ok {
		return member.Name
	}
	return ""
}

func (e *Evaluator) ormColumn(receiver Value, name string) (Value, error) {
	object, ok := receiver.Data.(*objectInstance)
	if !ok {
		return Value{}, errors.New("ORM column access requires a model value")
	}
	value, ok := object.Fields["@"+name]
	if !ok {
		return Value{}, fmt.Errorf("ORM model %s has no column %s", object.Definition.Node.Name, name)
	}
	return value, nil
}

func (e *Evaluator) ormModelProjection(model ormintegration.Model) string {
	columns := make([]string, len(model.Columns))
	for index, column := range model.Columns {
		columns[index] = e.ormRuntime().adapter.QuoteIdentifier(column.Name)
	}
	return strings.Join(columns, ", ")
}

func ormResultValueType(resultType types.Type) types.Type {
	if len(resultType.Args) > 0 {
		return resultType.Args[0]
	}
	return types.FromName("Any")
}

func (e *Evaluator) ormResultOK(resultType types.Type, value Value) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/orm/index", "DbResult")].(*enumDefinition)
	if !ok {
		definition, ok = e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	}
	if !ok {
		return Value{}, errors.New("ORM requires trb/std/result")
	}
	value.Type = ormResultValueType(resultType)
	return Value{Type: resultType, Data: &enumValue{Definition: definition, Name: "Ok", Payload: map[string]Value{"value": value}}}, nil
}

func (e *Evaluator) ormResultErr(resultType types.Type, kind, message string) (Value, error) {
	definition, ok := e.definitions[symbolKey("trb/orm/index", "DbResult")].(*enumDefinition)
	if !ok {
		definition, ok = e.definitions[symbolKey("trb/std/result/index", "Result")].(*enumDefinition)
	}
	kindDefinition, kindOK := e.definitions[symbolKey("trb/orm/index", "DbErrorKind")].(*enumDefinition)
	errorDefinition, errorOK := e.definitions[symbolKey("trb/orm/index", "DbError")].(*recordDefinition)
	if !ok || !kindOK || !errorOK {
		return Value{}, errors.New("ORM result runtime is not loaded")
	}
	kindValue := Value{Type: types.FromName("DbErrorKind"), Data: &enumValue{Definition: kindDefinition, Name: kind, Payload: map[string]Value{}}}
	errorType := types.FromName("DbError")
	if len(resultType.Args) == 2 {
		errorType = resultType.Args[1]
	}
	errorValue := Value{Type: errorType, Data: &recordInstance{Definition: errorDefinition, Fields: map[string]Value{
		"kind":    {Type: types.FromName("DbErrorKind"), Data: kindValue.Data},
		"message": {Type: types.FromName("String"), Data: message},
	}}}
	return Value{Type: resultType, Data: &enumValue{Definition: definition, Name: "Err", Payload: map[string]Value{"error": errorValue}}}, nil
}
