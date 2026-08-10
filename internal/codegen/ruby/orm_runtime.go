package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
)

func (g *generator) ormRuntime() {
	if g.orm == nil {
		return
	}
	if g.modulePath == "trb/orm/index" {
		g.b.WriteByte('\n')
		g.b.WriteString(strings.TrimSpace(rubyORMRuntimeSource))
		g.b.WriteByte('\n')
		g.b.WriteString("\nTrbOrmRuntime.configure(adapter: ")
		g.b.WriteString(strconv.Quote(g.orm.Adapter))
		g.b.WriteString(", database: ")
		if g.orm.DatabaseEnvironment != "" {
			g.b.WriteString("ENV[")
			g.b.WriteString(strconv.Quote(g.orm.DatabaseEnvironment))
			g.b.WriteString("]")
		} else {
			g.b.WriteString(strconv.Quote(g.orm.Database))
		}
		g.b.WriteString(", environment: ")
		g.b.WriteString(strconv.Quote(g.orm.DatabaseEnvironment))
		g.b.WriteString(")\n")
		g.b.WriteString("class Database; end\n")
		g.b.WriteString("Transaction = TrbOrmRuntime::Transaction\n")
		return
	}
	for _, model := range g.orm.ModelsForModule(g.modulePath) {
		g.ormRegisterModel(model)
	}
}

func (g *generator) ormRegisterModel(model ormintegration.Model) {
	columns := make([]string, 0, len(model.Columns))
	for _, column := range model.Columns {
		columns = append(columns, "{"+
			"name: "+strconv.Quote(column.Name)+
			", primary_key: "+strconv.FormatBool(column.PrimaryKey)+
			", generated: "+strconv.FormatBool(column.Generated)+
			", nullable: "+strconv.FormatBool(column.Nullable)+"}")
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
		scope := "nil"
		if association.Scope != nil && len(association.Scope.Parameters) == 1 && len(association.Scope.Body) == 1 {
			if expression, ok := association.Scope.Body[0].(*ir.ExpressionStatement); ok {
				scope = "->(" + association.Scope.Parameters[0] + ") { " + g.expr(expression.Expression) + " }"
			}
		}
		associations = append(associations, "{"+
			"name: "+strconv.Quote(association.Name)+
			", kind: "+strconv.Quote(string(association.Kind))+
			", target: "+strconv.Quote(association.TargetModel)+
			", source_column: "+strconv.Quote(association.SourceColumn)+
			", target_column: "+strconv.Quote(association.TargetColumn)+
			", through: "+strconv.Quote(association.Through)+
			", source: "+strconv.Quote(association.Source)+
			", dependent: "+strconv.Quote(string(association.Dependent))+
			", preloadable: "+strconv.FormatBool(association.Preloadable)+
			", scope: "+scope+"}")
	}
	g.b.WriteByte('\n')
	g.line("TrbOrmRuntime.register_model(", "")
	g.indent++
	g.line("model_class: "+model.Name+",", "")
	g.line("name: "+strconv.Quote(model.Name)+",", "")
	g.line("table: "+strconv.Quote(model.Table)+",", "")
	g.line("columns: ["+strings.Join(columns, ", ")+"],", "")
	g.line("unique_constraints: ["+strings.Join(unique, ", ")+"],", "")
	g.line("associations: ["+strings.Join(associations, ", ")+"],", "")
	g.indent--
	g.line(")", "")
}

// This runtime deliberately owns TypeRB ORM semantics. Sequel is restricted to
// opening connections, executing TypeRB-generated SQL, and transaction scopes,
// so replacing the Ruby database adapter does not affect the provider or IR.
const rubyORMRuntimeSource = `
require "sequel"
require "timeout"
require "uri"

module TrbOrmRuntime
	Bounds = Data.define(:start, :finish, :exclusive)
	Column = Data.define(:name, :primary_key, :generated, :nullable)
	Association = Data.define(:name, :kind, :target, :source_column, :target_column, :through, :source, :dependent, :preloadable, :scope)
	Metadata = Data.define(:model_class, :name, :table, :columns, :unique_constraints, :associations)

	class Failure < StandardError
		attr_reader :db_error

		def initialize(db_error)
			@db_error = db_error
			super(db_error.message)
		end
	end

	class TransactionAbort < StandardError
		attr_reader :result

		def initialize(result)
			@result = result
			super("TypeRB ORM transaction aborted")
		end
	end

	class Transaction
		attr_reader :parent

		def initialize(parent = nil)
			@parent = parent
			@active = true
		end

		def active?
			@active
		end

		def close
			@active = false
		end
	end

	class Draft
		attr_reader :metadata, :values, :query

		def initialize(metadata, values, query)
			@metadata = metadata
			@values = values.freeze
			@query = query
		end
	end

	class Changes
		attr_reader :model, :values

		def initialize(model, values)
			@model = model
			@values = values.freeze
		end
	end

	class Query
		attr_reader :metadata, :transaction, :predicate, :joins, :selections, :group_column,
			:having, :orders, :row_limit, :row_offset, :preloads

		def initialize(metadata, transaction: nil, predicate: nil, joins: [], selections: [], group_column: nil,
			having: [], orders: [], row_limit: nil, row_offset: nil, distinct: false, lock: false, preloads: [])
			@metadata = metadata
			@transaction = transaction
			@predicate = predicate
			@joins = joins.freeze
			@selections = selections.freeze
			@group_column = group_column
			@having = having.freeze
			@orders = orders.freeze
			@row_limit = row_limit
			@row_offset = row_offset
			@distinct = distinct
			@lock = lock
			@preloads = preloads.freeze
		end

		def copy(**changes)
			Query.new(
				changes.fetch(:metadata, @metadata),
				transaction: changes.fetch(:transaction, @transaction),
				predicate: changes.fetch(:predicate, @predicate),
				joins: changes.fetch(:joins, @joins),
				selections: changes.fetch(:selections, @selections),
				group_column: changes.fetch(:group_column, @group_column),
				having: changes.fetch(:having, @having),
				orders: changes.fetch(:orders, @orders),
				row_limit: changes.fetch(:row_limit, @row_limit),
				row_offset: changes.fetch(:row_offset, @row_offset),
				distinct: changes.fetch(:distinct, @distinct),
				lock: changes.fetch(:lock, @lock),
				preloads: changes.fetch(:preloads, @preloads),
			)
		end

		def where(predicates)
			copy(predicate: TrbOrmRuntime.and_predicates(@predicate, ["and", predicates]))
		end

		def not(predicates)
			copy(predicate: TrbOrmRuntime.and_predicates(@predicate, ["not", ["and", predicates]]))
		end

		def or_query(other)
			TrbOrmRuntime.invalid!("or() requires queries for the same model") unless other.is_a?(Query) && other.metadata.name == @metadata.name
			copy(predicate: ["or", @predicate || ["true"], other.predicate || ["true"]])
		end

		def distinct_rows
			copy(distinct: true)
		end

		def select_columns(columns)
			copy(selections: columns)
		end

		def group_by_column(columns)
			copy(group_column: columns.fetch(0))
		end

		def having_values(*values)
			copy(having: @having + [values])
		end

		def join_association(values, kind)
			name = values.is_a?(Array) ? values.fetch(0) : values
			target_query = values.is_a?(Array) ? values.fetch(1, nil) : nil
			copy(joins: @joins + TrbOrmRuntime.association_joins(@metadata, name, kind, target_query))
		end

		def where_association_exists(values, negated)
			name = values.is_a?(Array) ? values.fetch(0) : values
			target_query = values.is_a?(Array) ? values.fetch(1, nil) : nil
			association = TrbOrmRuntime.association!(@metadata, name)
			exists_query = TrbOrmRuntime.correlated_association_query(@metadata, association, target_query)
			copy(predicate: TrbOrmRuntime.and_predicates(@predicate, [negated ? "not_exists" : "exists", exists_query]))
		end

		def using(value)
			TrbOrmRuntime.invalid!("database transaction scope is no longer active") unless value.is_a?(Transaction) && value.active?
			copy(transaction: value)
		end

		def order_by(values)
			copy(orders: @orders + values)
		end

		def limit_rows(value)
			TrbOrmRuntime.invalid!("limit must be non-negative") if value < 0
			copy(row_limit: value)
		end

		def offset_rows(value)
			TrbOrmRuntime.invalid!("offset must be non-negative") if value < 0
			copy(row_offset: value)
		end

		def lock_rows
			copy(lock: true)
		end

		def preload_association(name, target_query)
			association = TrbOrmRuntime.association!(@metadata, name)
			TrbOrmRuntime.invalid!("association is not preloadable") unless association.preloadable
			target_query ||= TrbOrmRuntime.query_by_name(association.target)
			copy(preloads: @preloads + [[association.name, target_query]])
		end

		def all_result
			TrbOrmRuntime.result do
				rows = TrbOrmRuntime.select_rows(self)
				models = rows.map { |row| TrbOrmRuntime.instantiate(@metadata, row, @transaction) }
				TrbOrmRuntime.apply_preloads(models, @preloads)
				models
			end
		end

		def first_result
			TrbOrmRuntime.result do
				rows = TrbOrmRuntime.select_rows(copy(row_limit: 1))
				value = rows.empty? ? nil : TrbOrmRuntime.instantiate(@metadata, rows.fetch(0), @transaction)
				TrbOrmRuntime.apply_preloads([value].compact, @preloads)
				value
			end
		end

		def count_result
			TrbOrmRuntime.result { TrbOrmRuntime.scalar(self, "COUNT(*)").to_i }
		end

		def exists_result
			TrbOrmRuntime.result { !TrbOrmRuntime.select_rows(select_columns(["1"]).copy(row_limit: 1)).empty? }
		end

		def to_sql
			TrbOrmRuntime.select_sql(self, execution: false).fetch(0)
		end

		def explain_result
			TrbOrmRuntime.result do
				sql, arguments = TrbOrmRuntime.select_sql(self, execution: true)
				prefix = case TrbOrmRuntime.adapter
				when "sqlite" then "EXPLAIN QUERY PLAN "
				when "mysql" then "EXPLAIN FORMAT=JSON "
				else "EXPLAIN "
				end
				TrbOrmRuntime.fetch_rows(prefix + sql, arguments).map { |row| row.values.join(" ") }.join("\n")
			end
		end

		def pluck_result(columns)
			TrbOrmRuntime.result do
				rows = TrbOrmRuntime.select_rows(select_columns(columns))
				if columns.length == 1
					rows.map { |row| TrbOrmRuntime.row_value(row, columns.fetch(0)) }
				else
					rows.map { |row| columns.map { |column| TrbOrmRuntime.row_value(row, column) } }
				end
			end
		end

		def pick_result(columns)
			result = pluck_result(columns)
			return result if result.is_a?(Result::Err)
			Result::Ok.new(result.value.fetch(0, nil))
		end

		def ids_result
			primary_key = TrbOrmRuntime.primary_key!(@metadata)
			pluck_result([primary_key.name])
		end

		def aggregate_result(operation, column)
			TrbOrmRuntime.result do
				function = { "sum" => "SUM", "average" => "AVG", "minimum" => "MIN", "maximum" => "MAX" }.fetch(operation)
				value = TrbOrmRuntime.scalar(self, function + "(" + TrbOrmRuntime.qualified(@metadata.table, column) + ")")
				operation == "average" && !value.nil? ? value.to_f : value
			end
		end

		def grouped_aggregate_result(operation, column)
			TrbOrmRuntime.result do
				TrbOrmRuntime.invalid!("group() is required before a grouped aggregate") if @group_column.nil?
				function = operation == "count" ? "COUNT(*)" : { "sum" => "SUM", "average" => "AVG", "minimum" => "MIN", "maximum" => "MAX" }.fetch(operation) + "(" + TrbOrmRuntime.qualified(@metadata.table, column) + ")"
				rows = TrbOrmRuntime.select_rows(select_columns([@group_column, TrbOrmRuntime::RawSelection.new(function, "__trb_value")]))
				rows.to_h { |row| [TrbOrmRuntime.row_value(row, @group_column), TrbOrmRuntime.row_value(row, "__trb_value")] }
			end
		end

		def update_all_result(values)
			TrbOrmRuntime.result { TrbOrmRuntime.update_query(self, values) }
		end

		def delete_all_result
			TrbOrmRuntime.result { TrbOrmRuntime.delete_query(self) }
		end

		def destroy_all_result
			loaded = all_result
			return loaded if loaded.is_a?(Result::Err)
			count = 0
			loaded.value.each do |model|
				outcome = TrbOrmRuntime.destroy_model_result(model)
				return outcome if outcome.is_a?(Result::Err)
				count += 1 if outcome.value
			end
			Result::Ok.new(count)
		end

		def scope_values
			TrbOrmRuntime.equality_values(@predicate)
		end

		def each_batch(batch_size)
			TrbOrmRuntime.invalid!("batch size must be greater than zero") unless batch_size > 0
			primary_key = TrbOrmRuntime.primary_key!(@metadata)
			after = nil
			loop do
				batch_query = order_by([[primary_key.name, "asc"]]).limit_rows(batch_size)
				batch_query = batch_query.where([[primary_key.name, ">", after]]) unless after.nil?
				loaded = batch_query.all_result
				raise Failure.new(loaded.error) if loaded.is_a?(Result::Err)
				batch = loaded.value
				break if batch.empty?
				yield batch
				break if batch.length < batch_size
				after = batch.fetch(-1).instance_variable_get("@" + primary_key.name)
			end
		end
	end

	RawSelection = Data.define(:sql, :alias_name)
	RawColumn = Data.define(:table, :column)

	class << self
		attr_reader :adapter

		def configure(adapter:, database:, environment:)
			@adapter = adapter
			@database_source = database
			@database_environment = environment
			@models = {}
			@models_by_class = {}
		end

		def register_model(model_class:, name:, table:, columns:, unique_constraints:, associations:)
			metadata = Metadata.new(
				model_class,
				name,
				table,
				columns.map { |value| Column.new(value.fetch(:name), value.fetch(:primary_key), value.fetch(:generated), value.fetch(:nullable)) },
				unique_constraints,
				associations.map { |value| Association.new(value.fetch(:name), value.fetch(:kind), value.fetch(:target), value.fetch(:source_column), value.fetch(:target_column), value.fetch(:through), value.fetch(:source), value.fetch(:dependent), value.fetch(:preloadable), value.fetch(:scope)) },
			)
			@models[name] = metadata
			@models_by_class[model_class] = metadata
			model_class.class_eval { attr_reader(*metadata.columns.map { |column| column.name.to_sym }) }
			metadata
		end

		def metadata(value)
			key = value.is_a?(Class) ? value : value.class
			@models_by_class.fetch(key) { invalid!("unregistered ORM model") }
		end

		def metadata_by_name(name)
			@models.fetch(name) { invalid!("unregistered ORM model " + name) }
		end

		def query(model_class)
			Query.new(metadata(model_class))
		end

		def query_by_name(name)
			Query.new(metadata_by_name(name))
		end

		def database
			return @database unless @database.nil?
			if @database_source.nil? || @database_source.empty?
				message = @database_environment.empty? ? "database source is empty" : "database environment variable is not set or empty"
				raise Failure.new(db_error("Connection", message))
			end
			@database = case @adapter
			when "sqlite"
				Sequel.sqlite(@database_source)
			when "postgresql"
				Sequel.connect(@database_source)
			when "mysql"
				Sequel.connect(mysql_options(@database_source))
			else
				raise Failure.new(db_error("Connection", "unsupported ORM adapter"))
			end
		rescue Failure
			raise
		rescue StandardError
			raise Failure.new(db_error("Connection", "database connection failed"))
		end

		def mysql_options(source)
			return source if source.start_with?("mysql://", "mysql2://")
			match = source.match(/\A(?:(?<user>[^:@]+)(?::(?<password>[^@]*))?@)?tcp\((?<host>[^:)]+)(?::(?<port>[0-9]+))?\)\/(?<database>[^?]+)(?:\?.*)?\z/)
			invalid!("MySQL database source must be a mysql2 URL or Go-style TCP DSN") if match.nil?
			{
				adapter: "mysql2", host: match[:host], port: (match[:port] || "3306").to_i,
				database: match[:database], user: match[:user], password: match[:password],
			}
		end

		def transaction_result(parent = nil)
			invalid!("parent transaction scope is no longer active") if !parent.nil? && !parent.active?
			transaction = Transaction.new(parent)
			value = nil
			begin
				options = { savepoint: !parent.nil? }
				options[:mode] = :immediate if @adapter == "sqlite" && parent.nil?
				database.transaction(**options) do
					value = yield transaction
					raise TransactionAbort.new(value) if value.is_a?(Result::Err)
				end
				value
			rescue TransactionAbort => error
				error.result
			rescue Failure => error
				Result::Err.new(error.db_error)
			rescue StandardError => error
				Result::Err.new(map_error(error, "Query", "database transaction failed"))
			ensure
				transaction.close
			end
		end

		def result
			Result::Ok.new(yield)
		rescue Failure => error
			Result::Err.new(error.db_error)
		rescue StandardError => error
			Result::Err.new(map_error(error, "Query", "database operation failed"))
		end

		def db_error(kind, message)
			DbError.new(kind: DbErrorKind.const_get(kind), message: message)
		end

		def invalid!(message)
			raise Failure.new(db_error("InvalidData", message))
		end

		def map_error(error, fallback, message)
			kind = case error
			when Sequel::ConstraintViolation then "Constraint"
			when Sequel::DatabaseConnectionError, Sequel::DatabaseDisconnectError then "Connection"
			when Timeout::Error then "Timeout"
			else fallback
			end
			db_error(kind, message)
		end

		def quote(name)
			mark = @adapter == "mysql" ? 96.chr : "\""
			mark + name.to_s.gsub(mark, mark + mark) + mark
		end

		def qualified(table, column)
			quote(table) + "." + quote(column)
		end

		def placeholder(position, execution)
			return "?" if execution || @adapter != "postgresql"
			"$" + position.to_s
		end

		def fetch_rows(sql, arguments)
			database.fetch(sql, *arguments).all
		rescue StandardError => error
			raise Failure.new(map_error(error, "Query", "database query failed"))
		end

		def execute_dui(sql, arguments)
			database.execute_dui(database.literal(Sequel.lit(sql, *arguments)))
		rescue StandardError => error
			raise Failure.new(map_error(error, "Constraint", "database write failed"))
		end

		def execute_insert(sql, arguments)
			database.execute_insert(database.literal(Sequel.lit(sql, *arguments)))
		rescue StandardError => error
			raise Failure.new(map_error(error, "Constraint", "database insert failed"))
		end

		def row_value(row, column)
			return row[column.to_sym] if row.key?(column.to_sym)
			row[column.to_s]
		end

		def and_predicates(left, right)
			return right if left.nil?
			return left if right.nil?
			["and", [left, right]]
		end

		def select_sql(query, execution:, override_selection: nil)
			arguments = []
			selection = override_selection || query.selections
			columns = if selection.empty?
				query.metadata.columns.map { |column| qualified(query.metadata.table, column.name) }
			else
				selection.map do |column|
					if column.is_a?(RawSelection)
						column.sql + " AS " + quote(column.alias_name)
					elsif column == "1"
						"1"
					else
						qualified(query.metadata.table, column)
					end
				end
			end
			sql = "SELECT " + (query.instance_variable_get(:@distinct) ? "DISTINCT " : "") + columns.join(", ") + " FROM " + quote(query.metadata.table)
			join_aliases = { query.metadata.table => query.metadata.table }
			query.joins.each_with_index do |join, index|
				join_sql, join_alias = render_join(join, index, arguments, execution, join_aliases)
				sql += " " + join_sql
				join_aliases[join.fetch(:table)] = join_alias
			end
			unless query.predicate.nil?
				predicate_sql = render_predicate(query.metadata, query.predicate, arguments, execution, nil, join_aliases)
				sql += " WHERE " + predicate_sql unless predicate_sql.empty?
			end
			unless query.group_column.nil?
				sql += " GROUP BY " + qualified(query.metadata.table, query.group_column)
				unless query.having.empty?
					having_sql = query.having.map { |value| render_having(query, value, arguments, execution) }.join(" AND ")
					sql += " HAVING " + having_sql
				end
			end
			unless query.orders.empty?
				sql += " ORDER BY " + query.orders.map { |column, direction| qualified(query.metadata.table, column) + " " + direction.to_s.upcase }.join(", ")
			end
			if !query.row_limit.nil?
				sql += " LIMIT " + query.row_limit.to_i.to_s
			elsif !query.row_offset.nil?
				sql += @adapter == "mysql" ? " LIMIT 18446744073709551615" : " LIMIT -1" if @adapter != "postgresql"
			end
			sql += " OFFSET " + query.row_offset.to_i.to_s unless query.row_offset.nil?
			if query.instance_variable_get(:@lock)
				invalid!("database lock requires an explicit transaction scope") unless query.transaction&.active?
				sql += " FOR UPDATE" unless @adapter == "sqlite"
			end
			[sql, arguments]
		end

		def render_predicate(metadata, predicate, arguments, execution, table_alias = nil, aliases = {})
			table_alias ||= metadata.table
			kind = predicate.fetch(0)
			case kind
			when "true" then "1 = 1"
			when "and", "or"
				values = predicate.fetch(1)
				values = predicate.drop(1) if kind == "or"
				"(" + values.compact.map { |value| render_predicate(metadata, value, arguments, execution, table_alias, aliases) }.join(kind == "and" ? " AND " : " OR ") + ")"
			when "not"
				"NOT (" + render_predicate(metadata, predicate.fetch(1), arguments, execution, table_alias, aliases) + ")"
			when "exists", "not_exists"
				subquery_sql, subquery_arguments = select_sql(predicate.fetch(1).select_columns(["1"]), execution: execution)
				arguments.concat(subquery_arguments)
				(kind == "not_exists" ? "NOT EXISTS (" : "EXISTS (") + subquery_sql + ")"
			when "qualified"
				table, column, operator, value = predicate.fetch(1)
				table = aliases.fetch(table, table)
				left = qualified(table, column)
				if value.is_a?(RawColumn)
					left + " " + operator + " " + qualified(aliases.fetch(value.table, value.table), value.column)
				else
					left + " " + operator + " " + bind(arguments, value, execution)
				end
			else
				column, operator, value = predicate
				left = qualified(table_alias, column)
				if value.is_a?(Bounds)
					first = bind(arguments, value.start, execution)
					last = bind(arguments, value.finish, execution)
					return "(" + left + " >= " + first + " AND " + left + (value.exclusive ? " < " : " <= ") + last + ")"
				end
				if value.is_a?(Query)
					subquery_sql, subquery_arguments = select_sql(value, execution: execution)
					arguments.concat(subquery_arguments)
					return left + (operator == "NOT_IN" || operator == "!=" ? " NOT IN (" : " IN (") + subquery_sql + ")"
				end
				if value.is_a?(RawColumn)
					return left + " " + operator + " " + qualified(aliases.fetch(value.table, value.table), value.column)
				end
				if value.is_a?(Array) || operator == "IN" || operator == "NOT_IN"
					values = value.is_a?(Array) ? value : [value]
					return operator == "NOT_IN" ? "1 = 1" : "1 = 0" if values.empty?
					marks = values.map { |item| bind(arguments, item, execution) }
					return left + (operator == "NOT_IN" ? " NOT IN (" : " IN (") + marks.join(", ") + ")"
				end
				if value.nil?
					return left + (operator == "!=" ? " IS NOT NULL" : " IS NULL")
				end
				left + " " + operator + " " + bind(arguments, value, execution)
			end
		end

		def bind(arguments, value, execution)
			arguments << value
			placeholder(arguments.length, execution)
		end

		def render_join(join, index, arguments, execution, aliases)
			table_alias = "__trb_join_" + index.to_s
			left_alias = aliases.fetch(join.fetch(:left_table), join.fetch(:left_table))
			sql = join.fetch(:kind) + " " + quote(join.fetch(:table)) + " AS " + quote(table_alias) + " ON " + qualified(left_alias, join.fetch(:left_column)) + " = " + qualified(table_alias, join.fetch(:right_column))
			unless join[:predicate].nil?
				sql += " AND " + render_predicate(metadata_by_name(join.fetch(:target_model)), join.fetch(:predicate), arguments, execution, table_alias, aliases.merge(join.fetch(:table) => table_alias))
			end
			[sql, table_alias]
		end

		def render_having(query, values, arguments, execution)
			if values.length == 3
				aggregate, operator, value = values
				expression = aggregate.to_s == "count" ? "COUNT(*)" : aggregate.to_s.upcase + "(*)"
			else
				aggregate, column, operator, value = values
				expression = aggregate.to_s.upcase + "(" + qualified(query.metadata.table, column) + ")"
			end
			expression + " " + operator + " " + bind(arguments, value, execution)
		end

		def select_rows(query)
			sql, arguments = select_sql(query, execution: true)
			fetch_rows(sql, arguments)
		end

		def scalar(query, expression)
			row = fetch_rows(*select_sql(query, execution: true, override_selection: [RawSelection.new(expression, "__trb_value")])).fetch(0, nil)
			row.nil? ? nil : row_value(row, "__trb_value")
		end

		def instantiate(metadata, row, transaction)
			value = metadata.model_class.allocate
			metadata.columns.each { |column| value.instance_variable_set("@" + column.name, row_value(row, column.name)) }
			value.instance_variable_set(:@__trb_orm_transaction, transaction)
			metadata.associations.each do |association|
				value.instance_variable_set("@__trb_association_" + association.name + "_loaded", false)
			end
			value
		end

		def primary_key!(metadata)
			keys = metadata.columns.select(&:primary_key)
			invalid!("ORM model requires exactly one primary key") unless keys.length == 1
			keys.fetch(0)
		end

		def association!(metadata, name)
			metadata.associations.find { |association| association.name == name } || invalid!("unknown ORM association " + metadata.name + "." + name)
		end

		def association_target_query(association, target_query = nil)
			target_query ||= query_by_name(association.target)
			invalid!("association query targets the wrong model") unless target_query.metadata.name == association.target
			association.scope.nil? ? target_query : association.scope.call(target_query)
		end

		def association_joins(metadata, name, kind, target_query = nil)
			association = association!(metadata, name)
			target_query = association_target_query(association, target_query)
			if association.through.empty?
				target = metadata_by_name(association.target)
				return [{ kind: kind, table: target.table, left_table: metadata.table, left_column: association.source_column, right_column: association.target_column, target_model: target.name, predicate: target_query.predicate }]
			end
			through = association!(metadata, association.through)
			middle = metadata_by_name(through.target)
			via = association!(middle, association.source)
			target = metadata_by_name(association.target)
			[
				{ kind: kind, table: middle.table, left_table: metadata.table, left_column: through.source_column, right_column: through.target_column, target_model: middle.name },
				{ kind: kind, table: target.table, left_table: middle.table, left_column: via.source_column, right_column: via.target_column, target_model: target.name, predicate: target_query.predicate },
			]
		end

		def correlated_association_query(metadata, association, target_query = nil)
			target = association_target_query(association, target_query)
			if association.through.empty?
				return target.where([[association.target_column, "=", RawColumn.new(metadata.table, association.source_column)]])
			end
			through = association!(metadata, association.through)
			middle = metadata_by_name(through.target)
			via = association!(middle, association.source)
			target_metadata = metadata_by_name(association.target)
			join = { kind: "INNER JOIN", table: middle.table, left_table: target_metadata.table, left_column: via.target_column, right_column: via.source_column, target_model: middle.name }
			correlation = ["qualified", [middle.table, through.target_column, "=", RawColumn.new(metadata.table, through.source_column)]]
			target.copy(joins: target.joins + [join], predicate: and_predicates(target.predicate, correlation))
		end

		def association_query(model, name)
			metadata = metadata(model)
			association = association!(metadata, name)
			if !association.through.empty?
				through = association!(metadata, association.through)
				middle = metadata_by_name(through.target)
				via = association!(middle, association.source)
				target = metadata_by_name(association.target)
				join = { kind: "INNER JOIN", table: middle.table, left_table: target.table, left_column: via.target_column, right_column: via.source_column, target_model: middle.name }
				predicate = [middle.table, through.target_column, "=", model.instance_variable_get("@" + through.source_column)]
				query = query_by_name(association.target).copy(joins: [join], predicate: ["qualified", predicate])
			else
				query = query_by_name(association.target).where([[association.target_column, "=", model.instance_variable_get("@" + association.source_column)]])
			end
			transaction = model.instance_variable_get(:@__trb_orm_transaction)
			query = query.using(transaction) if transaction&.active?
			association_target_query(association, query)
		end

		def association_loaded?(model, name)
			model.instance_variable_get("@__trb_association_" + name + "_loaded") == true
		end

		def load_association_result(model, name, reload)
			return Result::Ok.new(model.instance_variable_get("@__trb_association_" + name)) if !reload && association_loaded?(model, name)
			association = association!(metadata(model), name)
			query = association_query(model, name)
			loaded = association.kind == "belongs_to" ? query.first_result : query.all_result
			return loaded if loaded.is_a?(Result::Err)
			value = loaded.value
			if association.kind == "has_one"
				return Result::Err.new(db_error("InvalidData", "database has_one association returned multiple rows")) if value.length > 1
				value = value.fetch(0, nil)
			end
			model.instance_variable_set("@__trb_association_" + name, value)
			model.instance_variable_set("@__trb_association_" + name + "_loaded", true)
			Result::Ok.new(value)
		end

		def apply_preloads(models, preloads)
			models.each do |model|
				preloads.each do |name, target_query|
					association = association!(metadata(model), name)
					query = association_query(model, name)
					query = merge_target_query(query, target_query)
					loaded = association.kind == "belongs_to" ? query.first_result : query.all_result
					raise Failure.new(loaded.error) if loaded.is_a?(Result::Err)
					value = loaded.value
					if association.kind == "has_one"
						invalid!("database has_one association returned multiple rows") if value.length > 1
						value = value.fetch(0, nil)
					end
					model.instance_variable_set("@__trb_association_" + name, value)
					model.instance_variable_set("@__trb_association_" + name + "_loaded", true)
				end
			end
		end

		def merge_target_query(base, target)
			invalid!("preload query targets the wrong model") unless base.metadata.name == target.metadata.name
			base.copy(
				predicate: and_predicates(base.predicate, target.predicate), joins: base.joins + target.joins,
				selections: target.selections, group_column: target.group_column, having: target.having,
				orders: target.orders, row_limit: target.row_limit, row_offset: target.row_offset,
				preloads: target.preloads,
			)
		end

		def equality_values(predicate)
			return {} if predicate.nil?
			kind = predicate.fetch(0)
			return predicate.fetch(1).each_with_object({}) { |item, values| values.merge!(equality_values(item)) } if kind == "and"
			return {} unless predicate.length == 3 && predicate.fetch(1) == "="
			{ predicate.fetch(0) => predicate.fetch(2) }
		end

		def build(query, values)
			scoped = query.scope_values
			values.each { |key, value| invalid!("build value conflicts with relation scope") if scoped.key?(key) && scoped[key] != value }
			Draft.new(query.metadata, scoped.merge(values), query)
		end

		def create_result(query, values)
			save_draft_result(build(query, values))
		end

		def validate_values(metadata, values, allow_primary_key: true)
			columns = metadata.columns.to_h { |column| [column.name, column] }
			values.each_key do |name|
				column = columns[name] || invalid!("unknown ORM attribute " + name)
				invalid!("generated ORM attribute cannot be written") if column.generated
				invalid!("primary key cannot be changed") if column.primary_key && !allow_primary_key
			end
		end

		def save_draft_result(draft)
			result do
				validate_values(draft.metadata, draft.values)
				columns = draft.values.keys
				values = columns.map { |column| draft.values.fetch(column) }
				if columns.empty?
					sql = "INSERT INTO " + quote(draft.metadata.table) + (@adapter == "mysql" ? " () VALUES ()" : " DEFAULT VALUES")
				else
					sql = "INSERT INTO " + quote(draft.metadata.table) + " (" + columns.map { |column| quote(column) }.join(", ") + ") VALUES (" + (["?"] * columns.length).join(", ") + ")"
				end
				primary_key = primary_key!(draft.metadata)
				if @adapter == "postgresql" || @adapter == "sqlite"
					row = fetch_rows(sql + " RETURNING " + draft.metadata.columns.map { |column| quote(column.name) }.join(", "), values).fetch(0)
					instantiate(draft.metadata, row, draft.query.transaction)
				else
					generated = execute_insert(sql, values)
					key = draft.values.fetch(primary_key.name, generated)
					loaded = draft.query.where([[primary_key.name, "=", key]]).first_result
					raise Failure.new(loaded.error) if loaded.is_a?(Result::Err)
					loaded.value || invalid!("database insert did not produce a primary key")
				end
			end
		end

		def insert_all_result(query, drafts)
			transaction_result(query.transaction) do |transaction|
				query = query.using(transaction)
				count = 0
				failure = nil
				drafts.each do |draft|
					outcome = save_draft_result(Draft.new(draft.metadata, draft.values, query))
					if outcome.is_a?(Result::Err)
						failure = outcome
						break
					end
					count += 1
				end
				failure || Result::Ok.new(count)
			end
		end

		def valid_unique?(metadata, columns)
			metadata.unique_constraints.any? { |constraint| constraint == columns }
		end

		def insert_if_absent_result(query, draft, unique_by)
			return Result::Err.new(db_error("InvalidData", "unique_by must match a primary or unique constraint")) unless valid_unique?(query.metadata, unique_by)
			created = transaction_result(query.transaction) do |transaction|
				save_draft_result(Draft.new(draft.metadata, draft.values, query.using(transaction)))
			end
			return Result::Ok.new(true) if created.is_a?(Result::Ok)
			return created unless created.error.kind == DbErrorKind::Constraint
			values = unique_by.to_h { |column| [column, draft.values.fetch(column)] }
			loaded = query.where(values.map { |column, value| [column, "=", value] }).first_result
			return loaded if loaded.is_a?(Result::Err)
			loaded.value.nil? ? created : Result::Ok.new(false)
		end

		def upsert_result(draft, unique_by, update_columns)
			return Result::Err.new(db_error("InvalidData", "unique_by must match a primary or unique constraint")) unless valid_unique?(draft.metadata, unique_by)
			created = transaction_result(draft.query.transaction) do |transaction|
				save_draft_result(Draft.new(draft.metadata, draft.values, draft.query.using(transaction)))
			end
			return created unless created.is_a?(Result::Err) && created.error.kind == DbErrorKind::Constraint
			where_values = unique_by.to_h { |column| [column, draft.values.fetch(column)] }
			loaded = draft.query.where(where_values.map { |column, value| [column, "=", value] }).first_result
			return loaded if loaded.is_a?(Result::Err)
			return created if loaded.value.nil?
			updates = update_columns.to_h { |column| [column, draft.values.fetch(column)] }
			update_model_result(loaded.value, updates)
		end

		def upsert_all_result(query, drafts, unique_by, update_columns)
			transaction_result(query.transaction) do |transaction|
				count = 0
				failure = nil
				drafts.each do |draft|
					outcome = upsert_result(Draft.new(draft.metadata, draft.values, query.using(transaction)), unique_by, update_columns)
					if outcome.is_a?(Result::Err)
						failure = outcome
						break
					end
					count += 1
				end
				failure || Result::Ok.new(count)
			end
		end

		def changes(model, values)
			validate_values(metadata(model), values, allow_primary_key: false)
			Changes.new(model, values)
		end

		def save_changes_result(value)
			update_model_result(value.model, value.values)
		end

		def update_model_result(model, values)
			result do
				metadata = metadata(model)
				validate_values(metadata, values, allow_primary_key: false)
				invalid!("database update requires at least one value") if values.empty?
				primary_key = primary_key!(metadata)
				key = model.instance_variable_get("@" + primary_key.name)
				query = Query.new(metadata, transaction: model.instance_variable_get(:@__trb_orm_transaction)).where([[primary_key.name, "=", key]])
				update_query(query, values)
				loaded = query.first_result
				raise Failure.new(loaded.error) if loaded.is_a?(Result::Err)
				loaded.value || invalid!("database update target was not found")
			end
		end

		def update_query(query, values)
			validate_values(query.metadata, values, allow_primary_key: false)
			invalid!("database update requires at least one value") if values.empty?
			arguments = []
			set = values.map { |column, value| arguments << value; quote(column) + " = ?" }.join(", ")
			where = ""
			unless query.predicate.nil?
				where_arguments = []
				where = " WHERE " + render_predicate(query.metadata, query.predicate, where_arguments, true)
				arguments.concat(where_arguments)
			end
			execute_dui("UPDATE " + quote(query.metadata.table) + " SET " + set + where, arguments)
		end

		def delete_model_result(model)
			metadata = metadata(model)
			primary_key = primary_key!(metadata)
			query = Query.new(metadata, transaction: model.instance_variable_get(:@__trb_orm_transaction)).where([[primary_key.name, "=", model.instance_variable_get("@" + primary_key.name)]])
			result { delete_query(query) > 0 }
		end

		def delete_query(query)
			arguments = []
			where = query.predicate.nil? ? "" : " WHERE " + render_predicate(query.metadata, query.predicate, arguments, true)
			execute_dui("DELETE FROM " + quote(query.metadata.table) + where, arguments)
		end

		def destroy_model_result(model)
			transaction_result(model.instance_variable_get(:@__trb_orm_transaction)) do |transaction|
				failure = nil
				metadata(model).associations.each do |association|
					next if association.dependent.empty?
					query = association_query(model, association.name).using(transaction)
					outcome = case association.dependent
					when "delete" then query.delete_all_result
					when "destroy" then query.destroy_all_result
					when "nullify" then query.update_all_result({ association.target_column => nil })
					when "restrict"
						exists = query.exists_result
						if exists.is_a?(Result::Err)
							exists
						elsif exists.value
							Result::Err.new(db_error("Constraint", "dependent association restricts destroy"))
						else
							Result::Ok.new(0)
						end
					else Result::Ok.new(0)
					end
					if outcome.is_a?(Result::Err)
						failure = outcome
						break
					end
				end
				failure || delete_model_result(model)
			end
		end
	end
end
`
