package repl

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"

	"github.com/type-rb/type-rb/internal/types"
)

const timeModule = "trb/std/time/index"

func (e *Evaluator) timeIntrinsic(name string, arguments []evaluatedArgument, typ types.Type) (Value, bool, error) {
	if !strings.HasPrefix(name, "trb.internal.time.") {
		return Value{}, false, nil
	}
	values := make([]Value, len(arguments))
	for index := range arguments {
		values[index] = arguments[index].Value
	}
	integer := func(index int) (int64, error) {
		if index >= len(values) {
			return 0, errors.New("date/time intrinsic is missing an Integer argument")
		}
		value, ok := values[index].Data.(int64)
		if !ok {
			return 0, errors.New("date/time intrinsic expects Integer")
		}
		return value, nil
	}
	text := func(index int) (string, error) {
		if index >= len(values) {
			return "", errors.New("date/time intrinsic is missing a String argument")
		}
		value, ok := values[index].Data.(string)
		if !ok {
			return "", errors.New("date/time intrinsic expects String")
		}
		return value, nil
	}
	operation := strings.TrimPrefix(name, "trb.internal.time.")
	intValue := func(value int) (Value, bool, error) { return Value{Type: typ, Data: int64(value)}, true, nil }
	stringValue := func(value string) (Value, bool, error) { return Value{Type: typ, Data: value}, true, nil }
	object := func(index int, expected string) (*objectInstance, error) {
		if index >= len(values) {
			return nil, fmt.Errorf("%s value is missing", expected)
		}
		value, ok := values[index].Data.(*objectInstance)
		if !ok || value.Definition.Node.Name != expected {
			return nil, fmt.Errorf("date/time intrinsic expects %s", expected)
		}
		return value, nil
	}
	compare := func(leftSeconds, leftNanos, rightSeconds, rightNanos int64) int {
		if leftSeconds < rightSeconds || leftSeconds == rightSeconds && leftNanos < rightNanos {
			return -1
		}
		if leftSeconds > rightSeconds || leftSeconds == rightSeconds && leftNanos > rightNanos {
			return 1
		}
		return 0
	}

	switch operation {
	case "validate_date", "date_try_new":
		year, err := integer(0)
		if err != nil {
			return Value{}, true, err
		}
		month, err := integer(1)
		if err != nil {
			return Value{}, true, err
		}
		day, err := integer(2)
		if err != nil {
			return Value{}, true, err
		}
		valid := validDateParts(year, month, day)
		if operation == "validate_date" {
			if !valid {
				return Value{}, true, errors.New("invalid Date")
			}
			return Value{Type: typ}, true, nil
		}
		if !valid {
			value, err := e.timeResultError(typ, "InvalidDate", "", "invalid Date")
			return value, true, err
		}
		value, err := e.timeObject("Date", map[string]Value{"@_year": integerValue(year), "@_month": integerValue(month), "@_day": integerValue(day)})
		if err != nil {
			return Value{}, true, err
		}
		result, err := e.filesystemOK(typ, value)
		return result, true, err
	case "date_parse", "date_try_parse":
		input, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		parsed, parseErr := time.Parse("2006-01-02", input)
		if parseErr != nil {
			if operation == "date_try_parse" {
				value, err := e.timeResultError(typ, "InvalidDate", input, parseErr.Error())
				return value, true, err
			}
			return Value{}, true, fmt.Errorf("invalid Date: %w", parseErr)
		}
		value, err := e.timeObject("Date", map[string]Value{"@_year": integerValue(int64(parsed.Year())), "@_month": integerValue(int64(parsed.Month())), "@_day": integerValue(int64(parsed.Day()))})
		if err != nil {
			return Value{}, true, err
		}
		if operation == "date_try_parse" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	case "date_to_string", "date_add_days":
		date, err := object(0, "Date")
		if err != nil {
			return Value{}, true, err
		}
		year, month, day := timeDateFields(date)
		value := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
		if operation == "date_to_string" {
			return stringValue(value.Format("2006-01-02"))
		}
		days, err := integer(1)
		if err != nil {
			return Value{}, true, err
		}
		value = value.AddDate(0, 0, int(days))
		if value.Year() < 1 || value.Year() > 9999 {
			return Value{}, true, errors.New("Date is outside the portable range")
		}
		result, err := e.timeObject("Date", map[string]Value{"@_year": integerValue(int64(value.Year())), "@_month": integerValue(int64(value.Month())), "@_day": integerValue(int64(value.Day()))})
		return result, true, err
	case "date_compare":
		left, err := object(0, "Date")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "Date")
		if err != nil {
			return Value{}, true, err
		}
		ly, lm, ld := timeDateFields(left)
		ry, rm, rd := timeDateFields(right)
		return intValue(compare(ly*10000+lm*100+ld, 0, ry*10000+rm*100+rd, 0))
	case "validate_time", "time_of_day_try_new":
		parts := make([]int64, 4)
		for index := range parts {
			value, err := integer(index)
			if err != nil {
				return Value{}, true, err
			}
			parts[index] = value
		}
		valid := validTimeParts(parts[0], parts[1], parts[2], parts[3])
		if operation == "validate_time" {
			if !valid {
				return Value{}, true, errors.New("invalid TimeOfDay")
			}
			return Value{Type: typ}, true, nil
		}
		if !valid {
			value, err := e.timeResultError(typ, "InvalidTime", "", "invalid TimeOfDay")
			return value, true, err
		}
		value, err := e.timeObject("TimeOfDay", timeFields(parts))
		if err != nil {
			return Value{}, true, err
		}
		result, err := e.filesystemOK(typ, value)
		return result, true, err
	case "time_of_day_parse", "time_of_day_try_parse":
		input, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		parsed, parseErr := parseLocalTime(input, false)
		if parseErr != nil {
			if operation == "time_of_day_try_parse" {
				value, err := e.timeResultError(typ, "InvalidTime", input, parseErr.Error())
				return value, true, err
			}
			return Value{}, true, parseErr
		}
		value, err := e.timeObject("TimeOfDay", timeFields(parsed))
		if err != nil {
			return Value{}, true, err
		}
		if operation == "time_of_day_try_parse" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	case "time_of_day_to_string":
		clock, err := object(0, "TimeOfDay")
		if err != nil {
			return Value{}, true, err
		}
		hour, minute, second, nanosecond := timeClockFields(clock)
		return stringValue(formatLocalTime(hour, minute, second, nanosecond))
	case "time_of_day_compare":
		left, err := object(0, "TimeOfDay")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "TimeOfDay")
		if err != nil {
			return Value{}, true, err
		}
		lh, lm, ls, ln := timeClockFields(left)
		rh, rm, rs, rn := timeClockFields(right)
		return intValue(compare((lh*60+lm)*60+ls, ln, (rh*60+rm)*60+rs, rn))
	case "validate_datetime", "datetime_try_new":
		parts := make([]int64, 7)
		for index := range parts {
			value, err := integer(index)
			if err != nil {
				return Value{}, true, err
			}
			parts[index] = value
		}
		valid := validDateParts(parts[0], parts[1], parts[2]) && validTimeParts(parts[3], parts[4], parts[5], parts[6])
		if operation == "validate_datetime" {
			if !valid {
				return Value{}, true, errors.New("invalid DateTime")
			}
			return Value{Type: typ}, true, nil
		}
		if !valid {
			value, err := e.timeResultError(typ, "InvalidDateTime", "", "invalid DateTime")
			return value, true, err
		}
		value, err := e.timeObject("DateTime", dateTimeFields(parts))
		if err != nil {
			return Value{}, true, err
		}
		result, err := e.filesystemOK(typ, value)
		return result, true, err
	case "datetime_parse", "datetime_try_parse":
		input, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		parsed, parseErr := parseLocalTime(input, true)
		if parseErr != nil {
			if operation == "datetime_try_parse" {
				value, err := e.timeResultError(typ, "InvalidDateTime", input, parseErr.Error())
				return value, true, err
			}
			return Value{}, true, parseErr
		}
		value, err := e.timeObject("DateTime", dateTimeFields(parsed))
		if err != nil {
			return Value{}, true, err
		}
		if operation == "datetime_try_parse" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	case "datetime_to_string":
		local, err := object(0, "DateTime")
		if err != nil {
			return Value{}, true, err
		}
		year, month, day := timeDateFields(local)
		hour, minute, second, nanosecond := timeClockFields(local)
		return stringValue(fmt.Sprintf("%04d-%02d-%02dT%s", year, month, day, formatLocalTime(hour, minute, second, nanosecond)))
	case "datetime_compare":
		left, err := object(0, "DateTime")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "DateTime")
		if err != nil {
			return Value{}, true, err
		}
		lt := timeObjectAsUTC(left)
		rt := timeObjectAsUTC(right)
		return intValue(lt.Compare(rt))
	case "validate_duration", "validate_instant":
		seconds, err := integer(0)
		if err != nil {
			return Value{}, true, err
		}
		nanosecond, err := integer(1)
		if err != nil {
			return Value{}, true, err
		}
		invalidInstant := operation == "validate_instant" && (seconds < -62_135_596_800 || seconds > 253_402_300_799)
		if nanosecond < 0 || nanosecond >= 1_000_000_000 || invalidInstant {
			return Value{}, true, fmt.Errorf("%s is outside the portable range", strings.TrimPrefix(operation, "validate_"))
		}
		return Value{Type: typ}, true, nil
	case "duration_from_seconds", "duration_from_milliseconds":
		value, err := integer(0)
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanosecond := value, int64(0)
		if operation == "duration_from_milliseconds" {
			seconds, nanosecond = floorDiv(value, 1000)
			nanosecond *= 1_000_000
		}
		result, err := e.timeObject("Duration", map[string]Value{"@_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanosecond)})
		return result, true, err
	case "duration_parse", "duration_try_parse":
		input, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanosecond, parseErr := parseDuration(input)
		if parseErr != nil {
			if operation == "duration_try_parse" {
				value, err := e.timeResultError(typ, "InvalidDuration", input, parseErr.Error())
				return value, true, err
			}
			return Value{}, true, parseErr
		}
		value, err := e.timeObject("Duration", map[string]Value{"@_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanosecond)})
		if err != nil {
			return Value{}, true, err
		}
		if operation == "duration_try_parse" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	case "duration_add", "duration_subtract", "duration_negative":
		left, err := object(0, "Duration")
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanos := timeDurationFields(left)
		if operation == "duration_negative" {
			seconds, nanos = normalizeSeconds(-seconds, -nanos)
		} else {
			right, err := object(1, "Duration")
			if err != nil {
				return Value{}, true, err
			}
			rs, rn := timeDurationFields(right)
			if operation == "duration_add" {
				seconds, nanos = normalizeSeconds(seconds+rs, nanos+rn)
			} else {
				seconds, nanos = normalizeSeconds(seconds-rs, nanos-rn)
			}
		}
		result, err := e.timeObject("Duration", map[string]Value{"@_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanos)})
		return result, true, err
	case "duration_to_string":
		value, err := object(0, "Duration")
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanos := timeDurationFields(value)
		return stringValue(formatDuration(seconds, nanos))
	case "duration_compare":
		left, err := object(0, "Duration")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "Duration")
		if err != nil {
			return Value{}, true, err
		}
		ls, ln := timeDurationFields(left)
		rs, rn := timeDurationFields(right)
		return intValue(compare(ls, ln, rs, rn))
	case "validate_time_zone", "time_zone_try_get":
		identifier, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		_, loadErr := time.LoadLocation(identifier)
		if operation == "validate_time_zone" {
			if loadErr != nil {
				return Value{}, true, fmt.Errorf("invalid TimeZone: %w", loadErr)
			}
			return Value{Type: typ}, true, nil
		}
		if loadErr != nil {
			value, err := e.timeResultError(typ, "InvalidTimeZone", identifier, loadErr.Error())
			return value, true, err
		}
		value, err := e.timeObject("TimeZone", map[string]Value{"@_identifier": {Type: types.FromName("String"), Data: identifier}})
		if err != nil {
			return Value{}, true, err
		}
		result, err := e.filesystemOK(typ, value)
		return result, true, err
	case "instant_now":
		now := time.Now()
		result, err := e.instantObject(now.Unix(), int64(now.Nanosecond()))
		return result, true, err
	case "instant_parse", "instant_try_parse":
		input, err := text(0)
		if err != nil {
			return Value{}, true, err
		}
		parsed, parseErr := time.Parse(time.RFC3339Nano, input)
		if parseErr != nil {
			if operation == "instant_try_parse" {
				value, err := e.timeResultError(typ, "InvalidInstant", input, parseErr.Error())
				return value, true, err
			}
			return Value{}, true, fmt.Errorf("invalid Instant: %w", parseErr)
		}
		value, err := e.instantObject(parsed.Unix(), int64(parsed.Nanosecond()))
		if err != nil {
			return Value{}, true, err
		}
		if operation == "instant_try_parse" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	case "instant_to_string":
		instant, err := object(0, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanos := timeInstantFields(instant)
		return stringValue(time.Unix(seconds, nanos).UTC().Format(time.RFC3339Nano))
	case "instant_to_datetime":
		instant, err := object(0, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		zone, err := object(1, "TimeZone")
		if err != nil {
			return Value{}, true, err
		}
		location, err := time.LoadLocation(timeStringField(zone, "@_identifier"))
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanos := timeInstantFields(instant)
		local := time.Unix(seconds, nanos).In(location)
		result, err := e.dateTimeObject(local)
		return result, true, err
	case "instant_add", "instant_subtract":
		instant, err := object(0, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		duration, err := object(1, "Duration")
		if err != nil {
			return Value{}, true, err
		}
		seconds, nanos := timeInstantFields(instant)
		ds, dn := timeDurationFields(duration)
		if operation == "instant_add" {
			seconds, nanos = normalizeSeconds(seconds+ds, nanos+dn)
		} else {
			seconds, nanos = normalizeSeconds(seconds-ds, nanos-dn)
		}
		result, err := e.instantObject(seconds, nanos)
		return result, true, err
	case "instant_duration_since":
		left, err := object(0, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		ls, ln := timeInstantFields(left)
		rs, rn := timeInstantFields(right)
		seconds, nanos := normalizeSeconds(ls-rs, ln-rn)
		result, err := e.timeObject("Duration", map[string]Value{"@_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanos)})
		return result, true, err
	case "instant_compare":
		left, err := object(0, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		right, err := object(1, "Instant")
		if err != nil {
			return Value{}, true, err
		}
		ls, ln := timeInstantFields(left)
		rs, rn := timeInstantFields(right)
		return intValue(compare(ls, ln, rs, rn))
	case "datetime_to_instant", "datetime_try_to_instant":
		local, err := object(0, "DateTime")
		if err != nil {
			return Value{}, true, err
		}
		zone, err := object(1, "TimeZone")
		if err != nil {
			return Value{}, true, err
		}
		identifier := timeStringField(zone, "@_identifier")
		location, err := time.LoadLocation(identifier)
		if err != nil {
			return Value{}, true, err
		}
		matches := resolveLocalTime(local, location)
		if len(matches) != 1 {
			kind, message := "NonexistentLocalTime", "local DateTime does not exist in TimeZone"
			if len(matches) > 1 {
				kind, message = "AmbiguousLocalTime", "local DateTime is ambiguous in TimeZone"
			}
			if operation == "datetime_try_to_instant" {
				value, err := e.timeResultError(typ, kind, identifier, message)
				return value, true, err
			}
			return Value{}, true, errors.New(message)
		}
		value, err := e.instantObject(matches[0].Unix(), int64(matches[0].Nanosecond()))
		if err != nil {
			return Value{}, true, err
		}
		if operation == "datetime_try_to_instant" {
			result, err := e.filesystemOK(typ, value)
			return result, true, err
		}
		return value, true, nil
	default:
		return Value{}, false, nil
	}
}

func integerValue(value int64) Value { return Value{Type: types.FromName("Integer"), Data: value} }

func (e *Evaluator) timeObject(name string, fields map[string]Value) (Value, error) {
	definition, ok := e.definitions[symbolKey(timeModule, name)].(*classDefinition)
	if !ok {
		return Value{}, fmt.Errorf("operation requires trb/std/time %s", name)
	}
	return Value{Type: types.FromName(name), Data: &objectInstance{Definition: definition, Fields: fields}}, nil
}

func (e *Evaluator) instantObject(seconds, nanosecond int64) (Value, error) {
	return e.timeObject("Instant", map[string]Value{"@_epoch_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanosecond)})
}

func (e *Evaluator) decodeTimeJSONCodec(kind, input string) (Value, error) {
	switch kind {
	case "time_date":
		parsed, err := time.Parse("2006-01-02", input)
		if err != nil {
			return Value{}, err
		}
		return e.timeObject("Date", map[string]Value{"@_year": integerValue(int64(parsed.Year())), "@_month": integerValue(int64(parsed.Month())), "@_day": integerValue(int64(parsed.Day()))})
	case "time_of_day":
		parts, err := parseLocalTime(input, false)
		if err != nil {
			return Value{}, err
		}
		return e.timeObject("TimeOfDay", timeFields(parts))
	case "time_datetime":
		parts, err := parseLocalTime(input, true)
		if err != nil {
			return Value{}, err
		}
		return e.timeObject("DateTime", dateTimeFields(parts))
	case "time_instant":
		parsed, err := time.Parse(time.RFC3339Nano, input)
		if err != nil || parsed.Year() < 1 || parsed.Year() > 9999 {
			if err == nil {
				err = errors.New("Instant is outside the portable range")
			}
			return Value{}, err
		}
		return e.instantObject(parsed.Unix(), int64(parsed.Nanosecond()))
	case "time_duration":
		seconds, nanosecond, err := parseDuration(input)
		if err != nil {
			return Value{}, err
		}
		return e.timeObject("Duration", map[string]Value{"@_seconds": integerValue(seconds), "@_nanosecond": integerValue(nanosecond)})
	case "time_zone":
		if _, err := time.LoadLocation(input); err != nil {
			return Value{}, err
		}
		return e.timeObject("TimeZone", map[string]Value{"@_identifier": {Type: types.FromName("String"), Data: input}})
	default:
		return Value{}, errors.New("unsupported date/time JSON codec")
	}
}

func (e *Evaluator) dateTimeObject(value time.Time) (Value, error) {
	return e.timeObject("DateTime", dateTimeFields([]int64{int64(value.Year()), int64(value.Month()), int64(value.Day()), int64(value.Hour()), int64(value.Minute()), int64(value.Second()), int64(value.Nanosecond())}))
}

func (e *Evaluator) timeResultError(resultType types.Type, kind, input, message string) (Value, error) {
	definition, ok := e.definitions[symbolKey(timeModule, "DateTimeErrorKind")].(*enumDefinition)
	if !ok {
		return Value{}, errors.New("operation requires trb/std/time")
	}
	kindValue := Value{Type: types.FromName("DateTimeErrorKind"), Data: &enumValue{Definition: definition, Name: kind, Payload: map[string]Value{}}}
	return e.structuredResultErrFrom(resultType, timeModule, "DateTimeError", map[string]Value{"kind": kindValue, "input": {Type: types.FromName("String"), Data: input}, "message": {Type: types.FromName("String"), Data: message}})
}

func validDateParts(year, month, day int64) bool {
	if year < 1 || year > 9999 {
		return false
	}
	value := time.Date(int(year), time.Month(month), int(day), 0, 0, 0, 0, time.UTC)
	return int64(value.Year()) == year && int64(value.Month()) == month && int64(value.Day()) == day
}
func validTimeParts(hour, minute, second, nanosecond int64) bool {
	return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 && second >= 0 && second <= 59 && nanosecond >= 0 && nanosecond < 1_000_000_000
}
func timeFields(parts []int64) map[string]Value {
	return map[string]Value{"@_hour": integerValue(parts[0]), "@_minute": integerValue(parts[1]), "@_second": integerValue(parts[2]), "@_nanosecond": integerValue(parts[3])}
}
func dateTimeFields(parts []int64) map[string]Value {
	result := timeFields(parts[3:])
	result["@_year"] = integerValue(parts[0])
	result["@_month"] = integerValue(parts[1])
	result["@_day"] = integerValue(parts[2])
	return result
}
func timeIntegerField(value *objectInstance, name string) int64 {
	result, _ := value.Fields[name].Data.(int64)
	return result
}
func timeStringField(value *objectInstance, name string) string {
	result, _ := value.Fields[name].Data.(string)
	return result
}
func timeDateFields(value *objectInstance) (int64, int64, int64) {
	return timeIntegerField(value, "@_year"), timeIntegerField(value, "@_month"), timeIntegerField(value, "@_day")
}
func timeClockFields(value *objectInstance) (int64, int64, int64, int64) {
	return timeIntegerField(value, "@_hour"), timeIntegerField(value, "@_minute"), timeIntegerField(value, "@_second"), timeIntegerField(value, "@_nanosecond")
}
func timeDurationFields(value *objectInstance) (int64, int64) {
	return timeIntegerField(value, "@_seconds"), timeIntegerField(value, "@_nanosecond")
}
func timeInstantFields(value *objectInstance) (int64, int64) {
	return timeIntegerField(value, "@_epoch_seconds"), timeIntegerField(value, "@_nanosecond")
}
func timeObjectAsUTC(value *objectInstance) time.Time {
	year, month, day := timeDateFields(value)
	hour, minute, second, nanos := timeClockFields(value)
	return time.Date(int(year), time.Month(month), int(day), int(hour), int(minute), int(second), int(nanos), time.UTC)
}
func formatLocalTime(hour, minute, second, nanosecond int64) string {
	result := fmt.Sprintf("%02d:%02d:%02d", hour, minute, second)
	if nanosecond != 0 {
		result += "." + strings.TrimRight(fmt.Sprintf("%09d", nanosecond), "0")
	}
	return result
}
func normalizeSeconds(seconds, nanosecond int64) (int64, int64) {
	for nanosecond < 0 {
		seconds--
		nanosecond += 1_000_000_000
	}
	for nanosecond >= 1_000_000_000 {
		seconds++
		nanosecond -= 1_000_000_000
	}
	return seconds, nanosecond
}
func floorDiv(value, divisor int64) (int64, int64) {
	quotient, remainder := value/divisor, value%divisor
	if remainder < 0 {
		quotient--
		remainder += divisor
	}
	return quotient, remainder
}
func formatDuration(seconds, nanos int64) string {
	sign := ""
	if seconds < 0 {
		sign = "-"
		seconds = -seconds - 1
		if nanos == 0 {
			seconds++
		} else {
			nanos = 1_000_000_000 - nanos
		}
	}
	fraction := ""
	if nanos != 0 {
		fraction = "." + strings.TrimRight(fmt.Sprintf("%09d", nanos), "0")
	}
	return fmt.Sprintf("%sPT%d%sS", sign, seconds, fraction)
}

func parseDuration(input string) (int64, int64, error) {
	if !strings.HasSuffix(input, "S") {
		return 0, 0, errors.New("invalid Duration")
	}
	value := strings.TrimSuffix(input, "S")
	negative := strings.HasPrefix(value, "-")
	if negative {
		value = strings.TrimPrefix(value, "-")
	}
	if !strings.HasPrefix(value, "PT") {
		return 0, 0, errors.New("invalid Duration")
	}
	value = strings.TrimPrefix(value, "PT")
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" || len(parts) == 2 && (parts[1] == "" || len(parts[1]) > 9) {
		return 0, 0, errors.New("invalid Duration")
	}
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || seconds < 0 {
		return 0, 0, errors.New("invalid Duration")
	}
	nanosecond := int64(0)
	if len(parts) == 2 {
		for _, character := range parts[1] {
			if character < '0' || character > '9' {
				return 0, 0, errors.New("invalid Duration")
			}
		}
		fraction := parts[1] + strings.Repeat("0", 9-len(parts[1]))
		nanosecond, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, 0, errors.New("invalid Duration")
		}
	}
	if negative {
		seconds, nanosecond = normalizeSeconds(-seconds, -nanosecond)
	}
	return seconds, nanosecond, nil
}

func parseLocalTime(input string, datetime bool) ([]int64, error) {
	layouts := []string{"15:04", "15:04:05", "15:04:05.999999999"}
	if datetime {
		layouts = []string{"2006-01-02T15:04", "2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999"}
	}
	var parsed time.Time
	var err error
	for _, layout := range layouts {
		parsed, err = time.Parse(layout, input)
		if err == nil {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid date/time value: %w", err)
	}
	if datetime {
		return []int64{int64(parsed.Year()), int64(parsed.Month()), int64(parsed.Day()), int64(parsed.Hour()), int64(parsed.Minute()), int64(parsed.Second()), int64(parsed.Nanosecond())}, nil
	}
	return []int64{int64(parsed.Hour()), int64(parsed.Minute()), int64(parsed.Second()), int64(parsed.Nanosecond())}, nil
}

func resolveLocalTime(local *objectInstance, location *time.Location) []time.Time {
	civil := timeObjectAsUTC(local)
	offsets := map[int]bool{}
	for probe := civil.Add(-36 * time.Hour); !probe.After(civil.Add(36 * time.Hour)); probe = probe.Add(time.Hour) {
		_, offset := probe.In(location).Zone()
		offsets[offset] = true
	}
	matches := []time.Time{}
	year, month, day := timeDateFields(local)
	hour, minute, second, _ := timeClockFields(local)
	for offset := range offsets {
		candidate := civil.Add(-time.Duration(offset) * time.Second)
		viewed := candidate.In(location)
		if int64(viewed.Year()) == year && int64(viewed.Month()) == month && int64(viewed.Day()) == day && int64(viewed.Hour()) == hour && int64(viewed.Minute()) == minute && int64(viewed.Second()) == second {
			matches = append(matches, candidate)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].Before(matches[j]) })
	return matches
}
