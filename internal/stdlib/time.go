package stdlib

import "github.com/type-rb/type-rb/internal/types"

func timeIntrinsicSymbols() map[string]Symbol {
	result := func(value types.Type) types.Type {
		return types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{value, dateTimeErrorType}}
	}
	parameters := func(values ...types.Type) []Parameter {
		items := make([]Parameter, len(values))
		for index, value := range values {
			items[index] = Parameter{Name: "value", Type: value}
		}
		return items
	}
	symbol := func(name string, input []types.Type, output types.Type) Symbol {
		return Symbol{Name: name, Intrinsic: "trb.internal.time." + name, Parameters: parameters(input...), Return: output}
	}
	integer3 := []types.Type{integerType, integerType, integerType}
	time4 := []types.Type{integerType, integerType, integerType, integerType}
	datetime7 := []types.Type{integerType, integerType, integerType, integerType, integerType, integerType, integerType}
	return map[string]Symbol{
		"validate_date":              symbol("validate_date", integer3, voidType),
		"date_try_new":               symbol("date_try_new", integer3, result(dateType)),
		"date_parse":                 symbol("date_parse", []types.Type{stringType}, dateType),
		"date_try_parse":             symbol("date_try_parse", []types.Type{stringType}, result(dateType)),
		"date_to_string":             symbol("date_to_string", []types.Type{dateType}, stringType),
		"date_add_days":              symbol("date_add_days", []types.Type{dateType, integerType}, dateType),
		"date_compare":               symbol("date_compare", []types.Type{dateType, dateType}, integerType),
		"validate_time":              symbol("validate_time", time4, voidType),
		"time_of_day_try_new":        symbol("time_of_day_try_new", time4, result(timeOfDayType)),
		"time_of_day_parse":          symbol("time_of_day_parse", []types.Type{stringType}, timeOfDayType),
		"time_of_day_try_parse":      symbol("time_of_day_try_parse", []types.Type{stringType}, result(timeOfDayType)),
		"time_of_day_to_string":      symbol("time_of_day_to_string", []types.Type{timeOfDayType}, stringType),
		"time_of_day_compare":        symbol("time_of_day_compare", []types.Type{timeOfDayType, timeOfDayType}, integerType),
		"validate_datetime":          symbol("validate_datetime", datetime7, voidType),
		"datetime_try_new":           symbol("datetime_try_new", datetime7, result(dateTimeType)),
		"datetime_parse":             symbol("datetime_parse", []types.Type{stringType}, dateTimeType),
		"datetime_try_parse":         symbol("datetime_try_parse", []types.Type{stringType}, result(dateTimeType)),
		"datetime_to_string":         symbol("datetime_to_string", []types.Type{dateTimeType}, stringType),
		"datetime_to_instant":        symbol("datetime_to_instant", []types.Type{dateTimeType, timeZoneType}, instantType),
		"datetime_try_to_instant":    symbol("datetime_try_to_instant", []types.Type{dateTimeType, timeZoneType}, result(instantType)),
		"datetime_compare":           symbol("datetime_compare", []types.Type{dateTimeType, dateTimeType}, integerType),
		"validate_duration":          symbol("validate_duration", []types.Type{integerType, integerType}, voidType),
		"duration_from_seconds":      symbol("duration_from_seconds", []types.Type{integerType}, durationType),
		"duration_from_milliseconds": symbol("duration_from_milliseconds", []types.Type{integerType}, durationType),
		"duration_parse":             symbol("duration_parse", []types.Type{stringType}, durationType),
		"duration_try_parse":         symbol("duration_try_parse", []types.Type{stringType}, result(durationType)),
		"duration_add":               symbol("duration_add", []types.Type{durationType, durationType}, durationType),
		"duration_subtract":          symbol("duration_subtract", []types.Type{durationType, durationType}, durationType),
		"duration_negative":          symbol("duration_negative", []types.Type{durationType}, durationType),
		"duration_to_string":         symbol("duration_to_string", []types.Type{durationType}, stringType),
		"duration_compare":           symbol("duration_compare", []types.Type{durationType, durationType}, integerType),
		"validate_time_zone":         symbol("validate_time_zone", []types.Type{stringType}, voidType),
		"time_zone_try_get":          symbol("time_zone_try_get", []types.Type{stringType}, result(timeZoneType)),
		"validate_instant":           symbol("validate_instant", []types.Type{integerType, integerType}, voidType),
		"instant_now":                symbol("instant_now", nil, instantType),
		"instant_parse":              symbol("instant_parse", []types.Type{stringType}, instantType),
		"instant_try_parse":          symbol("instant_try_parse", []types.Type{stringType}, result(instantType)),
		"instant_to_string":          symbol("instant_to_string", []types.Type{instantType}, stringType),
		"instant_to_datetime":        symbol("instant_to_datetime", []types.Type{instantType, timeZoneType}, dateTimeType),
		"instant_add":                symbol("instant_add", []types.Type{instantType, durationType}, instantType),
		"instant_subtract":           symbol("instant_subtract", []types.Type{instantType, durationType}, instantType),
		"instant_duration_since":     symbol("instant_duration_since", []types.Type{instantType, instantType}, durationType),
		"instant_compare":            symbol("instant_compare", []types.Type{instantType, instantType}, integerType),
	}
}

func timeSource() string {
	return `import { Result } from trb/std/result
import trb/internal/time as native_time

enum DateTimeErrorKind
	InvalidDate
	InvalidTime
	InvalidDateTime
	InvalidInstant
	InvalidDuration
	InvalidTimeZone
	AmbiguousLocalTime
	NonexistentLocalTime
end

record DateTimeError
	kind: DateTimeErrorKind
	input: String
	message: String
end

class Date
	readonly @_year: Integer
	readonly @_month: Integer
	readonly @_day: Integer

	def initialize(year: Integer, month: Integer, day: Integer)
		native_time.validate_date(year, month, day)
		@_year = year
		@_month = month
		@_day = day
	end

	def self.try_new(year: Integer, month: Integer, day: Integer): Result<Date, DateTimeError>
		return native_time.date_try_new(year, month, day)
	end

	def self.parse(value: String): Date
		return native_time.date_parse(value)
	end

	def self.try_parse(value: String): Result<Date, DateTimeError>
		return native_time.date_try_parse(value)
	end

	def year(): Integer
		return @_year
	end

	def month(): Integer
		return @_month
	end

	def day(): Integer
		return @_day
	end

	def to_s(): String
		return native_time.date_to_string(self)
	end

	def add_days(days: Integer): Date
		return native_time.date_add_days(self, days)
	end

	def before?(other: Date): Boolean
		return native_time.date_compare(self, other) < 0
	end

	def after?(other: Date): Boolean
		return native_time.date_compare(self, other) > 0
	end

	def same?(other: Date): Boolean
		return native_time.date_compare(self, other) == 0
	end

	def at(time: TimeOfDay): DateTime
		return DateTime.new(
			@_year,
			@_month,
			@_day,
			time.hour(),
			time.minute(),
			time.second(),
			time.nanosecond()
		)
	end
end

class TimeOfDay
	readonly @_hour: Integer
	readonly @_minute: Integer
	readonly @_second: Integer
	readonly @_nanosecond: Integer

	def initialize(hour: Integer, minute: Integer, second: Integer = 0, nanosecond: Integer = 0)
		native_time.validate_time(hour, minute, second, nanosecond)
		@_hour = hour
		@_minute = minute
		@_second = second
		@_nanosecond = nanosecond
	end

	def self.midnight(): TimeOfDay
		return TimeOfDay.new(0, 0)
	end

	def self.try_new(hour: Integer, minute: Integer, second: Integer = 0, nanosecond: Integer = 0): Result<TimeOfDay, DateTimeError>
		return native_time.time_of_day_try_new(hour, minute, second, nanosecond)
	end

	def self.parse(value: String): TimeOfDay
		return native_time.time_of_day_parse(value)
	end

	def self.try_parse(value: String): Result<TimeOfDay, DateTimeError>
		return native_time.time_of_day_try_parse(value)
	end

	def hour(): Integer
		return @_hour
	end

	def minute(): Integer
		return @_minute
	end

	def second(): Integer
		return @_second
	end

	def nanosecond(): Integer
		return @_nanosecond
	end

	def to_s(): String
		return native_time.time_of_day_to_string(self)
	end

	def before?(other: TimeOfDay): Boolean
		return native_time.time_of_day_compare(self, other) < 0
	end

	def after?(other: TimeOfDay): Boolean
		return native_time.time_of_day_compare(self, other) > 0
	end

	def same?(other: TimeOfDay): Boolean
		return native_time.time_of_day_compare(self, other) == 0
	end
end

class DateTime
	readonly @_year: Integer
	readonly @_month: Integer
	readonly @_day: Integer
	readonly @_hour: Integer
	readonly @_minute: Integer
	readonly @_second: Integer
	readonly @_nanosecond: Integer

	def initialize(year: Integer, month: Integer, day: Integer, hour: Integer, minute: Integer, second: Integer = 0, nanosecond: Integer = 0)
		native_time.validate_datetime(year, month, day, hour, minute, second, nanosecond)
		@_year = year
		@_month = month
		@_day = day
		@_hour = hour
		@_minute = minute
		@_second = second
		@_nanosecond = nanosecond
	end

	def self.try_new(year: Integer, month: Integer, day: Integer, hour: Integer, minute: Integer, second: Integer = 0, nanosecond: Integer = 0): Result<DateTime, DateTimeError>
		return native_time.datetime_try_new(year, month, day, hour, minute, second, nanosecond)
	end

	def self.parse(value: String): DateTime
		return native_time.datetime_parse(value)
	end

	def self.try_parse(value: String): Result<DateTime, DateTimeError>
		return native_time.datetime_try_parse(value)
	end

	def year(): Integer
		return @_year
	end

	def month(): Integer
		return @_month
	end

	def day(): Integer
		return @_day
	end

	def hour(): Integer
		return @_hour
	end

	def minute(): Integer
		return @_minute
	end

	def second(): Integer
		return @_second
	end

	def nanosecond(): Integer
		return @_nanosecond
	end

	def date(): Date
		return Date.new(@_year, @_month, @_day)
	end

	def time_of_day(): TimeOfDay
		return TimeOfDay.new(@_hour, @_minute, @_second, @_nanosecond)
	end

	def to_s(): String
		return native_time.datetime_to_string(self)
	end

	def to_instant(time_zone: TimeZone): Instant
		return native_time.datetime_to_instant(self, time_zone)
	end

	def try_to_instant(time_zone: TimeZone): Result<Instant, DateTimeError>
		return native_time.datetime_try_to_instant(self, time_zone)
	end

	def before?(other: DateTime): Boolean
		return native_time.datetime_compare(self, other) < 0
	end

	def after?(other: DateTime): Boolean
		return native_time.datetime_compare(self, other) > 0
	end

	def same?(other: DateTime): Boolean
		return native_time.datetime_compare(self, other) == 0
	end
end

class Duration
	readonly @_seconds: Integer
	readonly @_nanosecond: Integer

	def initialize(seconds: Integer, nanosecond: Integer = 0)
		native_time.validate_duration(seconds, nanosecond)
		@_seconds = seconds
		@_nanosecond = nanosecond
	end

	def self.seconds(value: Integer): Duration
		return Duration.new(value)
	end

	def self.milliseconds(value: Integer): Duration
		return native_time.duration_from_milliseconds(value)
	end

	def self.minutes(value: Integer): Duration
		return native_time.duration_from_seconds(value * 60)
	end

	def self.hours(value: Integer): Duration
		return native_time.duration_from_seconds(value * 3600)
	end

	def self.parse(value: String): Duration
		return native_time.duration_parse(value)
	end

	def self.try_parse(value: String): Result<Duration, DateTimeError>
		return native_time.duration_try_parse(value)
	end

	def whole_seconds(): Integer
		return @_seconds
	end

	def nanosecond(): Integer
		return @_nanosecond
	end

	def total_seconds(): Float
		return @_seconds.to_f() + @_nanosecond.to_f() / 1000000000.0
	end

	def add(other: Duration): Duration
		return native_time.duration_add(self, other)
	end

	def subtract(other: Duration): Duration
		return native_time.duration_subtract(self, other)
	end

	def negative(): Duration
		return native_time.duration_negative(self)
	end

	def to_s(): String
		return native_time.duration_to_string(self)
	end

	def before?(other: Duration): Boolean
		return native_time.duration_compare(self, other) < 0
	end

	def after?(other: Duration): Boolean
		return native_time.duration_compare(self, other) > 0
	end

	def same?(other: Duration): Boolean
		return native_time.duration_compare(self, other) == 0
	end
end

class TimeZone
	readonly @_identifier: String

	def initialize(identifier: String)
		native_time.validate_time_zone(identifier)
		@_identifier = identifier
	end

	def self.utc(): TimeZone
		return TimeZone.new("UTC")
	end

	def self.get(identifier: String): TimeZone
		return TimeZone.new(identifier)
	end

	def self.try_get(identifier: String): Result<TimeZone, DateTimeError>
		return native_time.time_zone_try_get(identifier)
	end

	def identifier(): String
		return @_identifier
	end

	def same?(other: TimeZone): Boolean
		return @_identifier == other.identifier()
	end
end

class Instant
	readonly @_epoch_seconds: Integer
	readonly @_nanosecond: Integer

	def initialize(epoch_seconds: Integer, nanosecond: Integer = 0)
		native_time.validate_instant(epoch_seconds, nanosecond)
		@_epoch_seconds = epoch_seconds
		@_nanosecond = nanosecond
	end

	def self.now(): Instant
		return native_time.instant_now()
	end

	def self.parse(value: String): Instant
		return native_time.instant_parse(value)
	end

	def self.try_parse(value: String): Result<Instant, DateTimeError>
		return native_time.instant_try_parse(value)
	end

	def epoch_seconds(): Integer
		return @_epoch_seconds
	end

	def nanosecond(): Integer
		return @_nanosecond
	end

	def to_s(): String
		return native_time.instant_to_string(self)
	end

	def to_datetime(time_zone: TimeZone): DateTime
		return native_time.instant_to_datetime(self, time_zone)
	end

	def to_date(time_zone: TimeZone): Date
		return to_datetime(time_zone).date()
	end

	def add(duration: Duration): Instant
		return native_time.instant_add(self, duration)
	end

	def subtract(duration: Duration): Instant
		return native_time.instant_subtract(self, duration)
	end

	def duration_since(other: Instant): Duration
		return native_time.instant_duration_since(self, other)
	end

	def before?(other: Instant): Boolean
		return native_time.instant_compare(self, other) < 0
	end

	def after?(other: Instant): Boolean
		return native_time.instant_compare(self, other) > 0
	end

	def same?(other: Instant): Boolean
		return native_time.instant_compare(self, other) == 0
	end
end

def now(): Instant
	return Instant.now()
end
`
}
