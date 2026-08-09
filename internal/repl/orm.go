//go:build !js || !wasm

package repl

import (
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
	model     ormintegration.Model
	predicate *ormPredicate
	orders    []ormOrder
	limit     *int64
	offset    *int64
	preloads  []string
}

type ormPredicate struct {
	kind      string
	condition ormCondition
	children  []*ormPredicate
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
	query, remaining, err := e.ormQueryReceiver(arguments)
	if err != nil {
		return Value{}, err
	}
	switch name {
	case "trb.orm.where", "trb.orm.query.where":
		predicate, err := ormPredicateFromArguments(remaining)
		if err != nil {
			return Value{}, err
		}
		query.predicate = ormCombinePredicates("and", query.predicate, predicate)
		return ormQueryResult(typ, query), nil
	case "trb.orm.not", "trb.orm.query.not":
		predicate, err := ormPredicateFromArguments(remaining)
		if err != nil {
			return Value{}, err
		}
		if predicate == nil {
			return Value{}, errors.New("ORM not requires one condition")
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
			return Value{}, errors.New("ORM or requires unmodified predicate queries; apply order, limit, offset, and preload after or")
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
	case "trb.orm.find":
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
		query.predicate = ormCombinePredicates("and", query.predicate, predicate)
		return e.ormFirstResult(typ, query)
	case "trb.orm.exists", "trb.orm.query.exists":
		if name == "trb.orm.exists" {
			predicate, err := ormPredicateFromArguments(remaining)
			if err != nil {
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
	case "trb.orm.association.query.belongs_to", "trb.orm.association.query.has_many":
		return e.ormAssociationQuery(typ, query.model, arguments[0].Value, call)
	case "trb.orm.association.loaded.belongs_to", "trb.orm.association.loaded.has_many":
		return e.ormLoadedAssociation(typ, arguments[0].Value, call)
	default:
		return Value{}, fmt.Errorf("%s is type-checked, but ORM writes and batch iteration are not executable in the REPL yet; use trb run", name)
	}
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
	return &result
}

func ormQueryResult(typ types.Type, query *ormQueryValue) Value {
	return Value{Type: typ, Data: query}
}

func ormQueryModified(query *ormQueryValue) bool {
	return len(query.orders) > 0 || query.limit != nil || query.offset != nil || len(query.preloads) > 0
}

func ormPredicateFromArguments(arguments []evaluatedArgument) (*ormPredicate, error) {
	if len(arguments) == 3 && arguments[0].Name == "" && arguments[1].Name == "" && arguments[2].Name == "" {
		column, columnOK := arguments[0].Value.Data.(string)
		operator, operatorOK := arguments[1].Value.Data.(string)
		if !columnOK || !operatorOK {
			return nil, errors.New("ORM comparison requires a column and operator")
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
	runtime := e.ormRuntime()
	statement := "SELECT " + projection + " FROM " + runtime.adapter.QuoteIdentifier(query.model.Table)
	arguments := []any{}
	if query.predicate != nil {
		predicate, err := e.ormPredicateSQL(query.predicate, &arguments)
		if err != nil {
			return "", nil, err
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
		statement += " LIMIT " + runtime.adapter.Placeholder(len(arguments)+1)
		arguments = append(arguments, *query.limit)
	} else if query.offset != nil {
		statement += runtime.adapter.OffsetNoLimit
	}
	if query.offset != nil {
		statement += " OFFSET " + runtime.adapter.Placeholder(len(arguments)+1)
		arguments = append(arguments, *query.offset)
	}
	return statement, arguments, nil
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
		case "IN":
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

func (e *Evaluator) ormLoad(query *ormQueryValue) ([]Value, *ormFailure) {
	database, failure := e.ormDatabase()
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
		value, failure := e.ormScanModel(rows, query.model)
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

func (e *Evaluator) ormScanModel(rows *sql.Rows, model ormintegration.Model) (Value, *ormFailure) {
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
	database, failure := e.ormDatabase()
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
	database, failure := e.ormDatabase()
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
	database, failure := e.ormDatabase()
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
	database, failure := e.ormDatabase()
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
