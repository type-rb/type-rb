package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

// timeDatabaseInterop is a Go-backend adapter boundary. TypeRB's public time
// API remains database-independent while database/sql receives canonical
// values when a portable time object is used as a query parameter.
func (g *generator) timeDatabaseInterop() {
	g.requireImport("database/sql/driver", "driver")
	g.requireImport("strings", "")
	g.line("func (self *Date) Value() (driver.Value, error) { if self == nil { return nil, nil }; return self.ToS(), nil }")
	g.line("func (self *TimeOfDay) Value() (driver.Value, error) { if self == nil { return nil, nil }; return self.ToS(), nil }")
	g.line("func (self *DateTime) Value() (driver.Value, error) { if self == nil { return nil, nil }; return strings.Replace(self.ToS(), \"T\", \" \", 1), nil }")
	g.line("func (self *Instant) Value() (driver.Value, error) { if self == nil { return nil, nil }; return stdtime.Unix(int64(self.trbFieldEpochSeconds), int64(self.trbFieldNanosecond)).UTC(), nil }")
	g.b.WriteByte('\n')
}

func (g *generator) timeIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.internal.time.") {
		return "", false
	}
	g.requireImport("time", "stdtime")
	operation := strings.TrimPrefix(name, "trb.internal.time.")
	dateValid := func(year, month, day string) string {
		return "value := stdtime.Date(" + year + ", stdtime.Month(" + month + "), " + day + ", 0, 0, 0, 0, stdtime.UTC); if value.Year() != " + year + " || int(value.Month()) != " + month + " || value.Day() != " + day
	}
	timeValid := func(hour, minute, second, nanosecond string) string {
		return hour + " < 0 || " + hour + " > 23 || " + minute + " < 0 || " + minute + " > 59 || " + second + " < 0 || " + second + " > 59 || " + nanosecond + " < 0 || " + nanosecond + " >= 1000000000"
	}
	panicMessage := func(message string) string { return "panic(" + strconv.Quote(message) + ")" }
	resultTypes := func() (string, string, string) {
		result := call.ExprType()
		if len(result.Args) != 2 {
			return g.goType(result), "any", "any"
		}
		return g.goType(result), g.goType(result.Args[0]), g.goType(result.Args[1])
	}
	resultOK := func(value string) string {
		_, valueType, errorType := resultTypes()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		return alias + ".NewResultOk[" + valueType + ", " + errorType + "](" + value + ")"
	}
	resultError := func(kind, input, message string) string {
		_, valueType, errorType := resultTypes()
		alias := g.typeAliases["Result"]
		if alias == "" {
			alias = "__trb_result"
		}
		value := "DateTimeError{Kind: " + goConstantIdentifier("DateTimeErrorKind", kind) + ", Input: " + input + ", Message: " + message + "}"
		return alias + ".NewResultErr[" + valueType + ", " + errorType + "](" + value + ")"
	}
	compare := func(left, right string) string {
		return "func() int { if " + left + " < " + right + " { return -1 }; if " + left + " > " + right + " { return 1 }; return 0 }()"
	}
	parseDate := func(input string, safe bool) string {
		resultType, _, _ := resultTypes()
		body := "parsed, err := stdtime.Parse(\"2006-01-02\", " + input + "); if err != nil {"
		if safe {
			body += " return " + resultError("InvalidDate", input, "err.Error()")
		} else {
			body += " panic(\"invalid Date: \" + err.Error())"
		}
		body += " }; result := NewDate(parsed.Year(), int(parsed.Month()), parsed.Day())"
		if safe {
			body += "; return " + resultOK("result")
		} else {
			body += "; return result"
		}
		return "func() " + map[bool]string{true: resultType, false: "*Date"}[safe] + " { " + body + " }()"
	}
	parseClock := func(input string, safe bool, datetime bool) string {
		resultType, _, _ := resultTypes()
		layouts := "[]string{\"15:04\", \"15:04:05\", \"15:04:05.999999999\"}"
		kind := "InvalidTime"
		constructor := "NewTimeOfDay(parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond())"
		returnType := "*TimeOfDay"
		if datetime {
			layouts = "[]string{\"2006-01-02T15:04\", \"2006-01-02T15:04:05\", \"2006-01-02T15:04:05.999999999\"}"
			kind = "InvalidDateTime"
			constructor = "NewDateTime(parsed.Year(), int(parsed.Month()), parsed.Day(), parsed.Hour(), parsed.Minute(), parsed.Second(), parsed.Nanosecond())"
			returnType = "*DateTime"
		}
		body := "var parsed stdtime.Time; var err error; for _, layout := range " + layouts + " { parsed, err = stdtime.Parse(layout, " + input + "); if err == nil { break } }; if err != nil {"
		if safe {
			body += " return " + resultError(kind, input, "err.Error()")
		} else {
			body += " panic(\"invalid date/time value: \" + err.Error())"
		}
		body += " }; result := " + constructor
		if safe {
			body += "; return " + resultOK("result")
			returnType = resultType
		} else {
			body += "; return result"
		}
		return "func() " + returnType + " { " + body + " }()"
	}

	switch operation {
	case "validate_date":
		return "func() { " + dateValid(arguments[0], arguments[1], arguments[2]) + " { " + panicMessage("invalid Date") + " } }()", true
	case "date_try_new":
		resultType, _, _ := resultTypes()
		condition := dateValid(arguments[0], arguments[1], arguments[2])
		return "func() " + resultType + " { " + condition + " { return " + resultError("InvalidDate", "\"\"", "\"invalid Date\"") + " }; return " + resultOK("NewDate("+strings.Join(arguments, ", ")+")") + " }()", true
	case "date_parse":
		return parseDate(arguments[0], false), true
	case "date_try_parse":
		return parseDate(arguments[0], true), true
	case "date_to_string":
		value := arguments[0]
		return "stdtime.Date(" + value + ".trbFieldYear, stdtime.Month(" + value + ".trbFieldMonth), " + value + ".trbFieldDay, 0, 0, 0, 0, stdtime.UTC).Format(\"2006-01-02\")", true
	case "date_add_days":
		value := arguments[0]
		return "func() *Date { result := stdtime.Date(" + value + ".trbFieldYear, stdtime.Month(" + value + ".trbFieldMonth), " + value + ".trbFieldDay, 0, 0, 0, 0, stdtime.UTC).AddDate(0, 0, " + arguments[1] + "); if result.Year() < 1 || result.Year() > 9999 { panic(\"Date is outside the portable range\") }; return NewDate(result.Year(), int(result.Month()), result.Day()) }()", true
	case "date_compare":
		left := arguments[0] + ".trbFieldYear*10000 + " + arguments[0] + ".trbFieldMonth*100 + " + arguments[0] + ".trbFieldDay"
		right := arguments[1] + ".trbFieldYear*10000 + " + arguments[1] + ".trbFieldMonth*100 + " + arguments[1] + ".trbFieldDay"
		return compare(left, right), true
	case "validate_time":
		return "func() { if " + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]) + " { " + panicMessage("invalid TimeOfDay") + " } }()", true
	case "time_of_day_try_new":
		resultType, _, _ := resultTypes()
		return "func() " + resultType + " { if " + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]) + " { return " + resultError("InvalidTime", "\"\"", "\"invalid TimeOfDay\"") + " }; return " + resultOK("NewTimeOfDay("+strings.Join(arguments, ", ")+")") + " }()", true
	case "time_of_day_parse":
		return parseClock(arguments[0], false, false), true
	case "time_of_day_try_parse":
		return parseClock(arguments[0], true, false), true
	case "time_of_day_to_string":
		value := arguments[0]
		return "stdtime.Date(2000, 1, 1, " + value + ".trbFieldHour, " + value + ".trbFieldMinute, " + value + ".trbFieldSecond, " + value + ".trbFieldNanosecond, stdtime.UTC).Format(\"15:04:05.999999999\")", true
	case "time_of_day_compare":
		left := "((" + arguments[0] + ".trbFieldHour*60+" + arguments[0] + ".trbFieldMinute)*60+" + arguments[0] + ".trbFieldSecond)*1000000000+" + arguments[0] + ".trbFieldNanosecond"
		right := "((" + arguments[1] + ".trbFieldHour*60+" + arguments[1] + ".trbFieldMinute)*60+" + arguments[1] + ".trbFieldSecond)*1000000000+" + arguments[1] + ".trbFieldNanosecond"
		return compare(left, right), true
	case "validate_datetime":
		dateCondition := dateValid(arguments[0], arguments[1], arguments[2])
		return "func() { " + dateCondition + " || " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6]) + " { " + panicMessage("invalid DateTime") + " } }()", true
	case "datetime_try_new":
		resultType, _, _ := resultTypes()
		dateCondition := dateValid(arguments[0], arguments[1], arguments[2])
		return "func() " + resultType + " { " + dateCondition + " || " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6]) + " { return " + resultError("InvalidDateTime", "\"\"", "\"invalid DateTime\"") + " }; return " + resultOK("NewDateTime("+strings.Join(arguments, ", ")+")") + " }()", true
	case "datetime_parse":
		return parseClock(arguments[0], false, true), true
	case "datetime_try_parse":
		return parseClock(arguments[0], true, true), true
	case "datetime_to_string":
		value := arguments[0]
		return "stdtime.Date(" + value + ".trbFieldYear, stdtime.Month(" + value + ".trbFieldMonth), " + value + ".trbFieldDay, " + value + ".trbFieldHour, " + value + ".trbFieldMinute, " + value + ".trbFieldSecond, " + value + ".trbFieldNanosecond, stdtime.UTC).Format(\"2006-01-02T15:04:05.999999999\")", true
	case "datetime_compare":
		left := "stdtime.Date(" + arguments[0] + ".trbFieldYear, stdtime.Month(" + arguments[0] + ".trbFieldMonth), " + arguments[0] + ".trbFieldDay, " + arguments[0] + ".trbFieldHour, " + arguments[0] + ".trbFieldMinute, " + arguments[0] + ".trbFieldSecond, " + arguments[0] + ".trbFieldNanosecond, stdtime.UTC)"
		right := "stdtime.Date(" + arguments[1] + ".trbFieldYear, stdtime.Month(" + arguments[1] + ".trbFieldMonth), " + arguments[1] + ".trbFieldDay, " + arguments[1] + ".trbFieldHour, " + arguments[1] + ".trbFieldMinute, " + arguments[1] + ".trbFieldSecond, " + arguments[1] + ".trbFieldNanosecond, stdtime.UTC)"
		return "func() int { return " + left + ".Compare(" + right + ") }()", true
	case "validate_duration", "validate_instant":
		label := "Duration"
		condition := arguments[1] + " < 0 || " + arguments[1] + " >= 1000000000"
		if operation == "validate_instant" {
			label = "Instant"
			condition += " || " + arguments[0] + " < -62135596800 || " + arguments[0] + " > 253402300799"
		}
		return "func() { if " + condition + " { panic(" + strconv.Quote(label+" is outside the portable range") + ") } }()", true
	case "duration_from_seconds":
		return "NewDuration(" + arguments[0] + ")", true
	case "duration_from_milliseconds":
		value := arguments[0]
		return "func() *Duration { seconds, remainder := " + value + "/1000, " + value + "%1000; if remainder < 0 { seconds--; remainder += 1000 }; return NewDuration(seconds, remainder*1000000) }()", true
	case "duration_parse", "duration_try_parse":
		g.requireImport("regexp", "")
		g.requireImport("strconv", "")
		g.requireImport("strings", "")
		safe := operation == "duration_try_parse"
		returnType := "*Duration"
		failure := "panic(\"invalid Duration\")"
		finish := "return NewDuration(seconds, nanosecond)"
		if safe {
			resultType, _, _ := resultTypes()
			returnType = resultType
			failure = "return " + resultError("InvalidDuration", arguments[0], "\"invalid Duration\"")
			finish = "return " + resultOK("NewDuration(seconds, nanosecond)")
		}
		return "func() " + returnType + " { match := regexp.MustCompile(`^(-)?PT([0-9]+)(?:\\.([0-9]{1,9}))?S$`).FindStringSubmatch(" + arguments[0] + "); if match == nil { " + failure + " }; magnitude, parseErr := strconv.ParseInt(match[2], 10, 64); if parseErr != nil { " + failure + " }; fraction := match[3] + strings.Repeat(\"0\", 9-len(match[3])); nanosecond64, parseErr := strconv.ParseInt(fraction, 10, 64); if parseErr != nil { " + failure + " }; seconds, nanosecond := int(magnitude), int(nanosecond64); if match[1] != \"\" { seconds = -seconds; if nanosecond != 0 { seconds--; nanosecond = 1000000000-nanosecond } }; " + finish + " }()", true
	case "duration_add", "duration_subtract":
		rightSign := "+"
		if operation == "duration_subtract" {
			rightSign = "-"
		}
		return "func() *Duration { seconds := " + arguments[0] + ".trbFieldSeconds " + rightSign + " " + arguments[1] + ".trbFieldSeconds; nanosecond := " + arguments[0] + ".trbFieldNanosecond " + rightSign + " " + arguments[1] + ".trbFieldNanosecond; for nanosecond < 0 { seconds--; nanosecond += 1000000000 }; for nanosecond >= 1000000000 { seconds++; nanosecond -= 1000000000 }; return NewDuration(seconds, nanosecond) }()", true
	case "duration_negative":
		return "func() *Duration { if " + arguments[0] + ".trbFieldNanosecond == 0 { return NewDuration(-" + arguments[0] + ".trbFieldSeconds) }; return NewDuration(-" + arguments[0] + ".trbFieldSeconds-1, 1000000000-" + arguments[0] + ".trbFieldNanosecond) }()", true
	case "duration_to_string":
		g.requireImport("strconv", "")
		g.requireImport("strings", "")
		value := arguments[0]
		return "func() string { seconds, nanosecond := " + value + ".trbFieldSeconds, " + value + ".trbFieldNanosecond; sign := \"\"; if seconds < 0 { sign = \"-\"; seconds = -seconds-1; if nanosecond == 0 { seconds++; } else { nanosecond = 1000000000-nanosecond } }; fraction := \"\"; if nanosecond != 0 { fraction = \".\" + strings.TrimRight(strconv.Itoa(nanosecond+1000000000)[1:], \"0\") }; return sign + \"PT\" + strconv.Itoa(seconds) + fraction + \"S\" }()", true
	case "duration_compare":
		left, right := arguments[0], arguments[1]
		return "func() int { if " + left + ".trbFieldSeconds < " + right + ".trbFieldSeconds { return -1 }; if " + left + ".trbFieldSeconds > " + right + ".trbFieldSeconds { return 1 }; if " + left + ".trbFieldNanosecond < " + right + ".trbFieldNanosecond { return -1 }; if " + left + ".trbFieldNanosecond > " + right + ".trbFieldNanosecond { return 1 }; return 0 }()", true
	case "validate_time_zone":
		g.requireImport("time/tzdata", "_")
		return "func() { if _, err := stdtime.LoadLocation(" + arguments[0] + "); err != nil { panic(\"invalid TimeZone: \" + err.Error()) } }()", true
	case "time_zone_try_get":
		g.requireImport("time/tzdata", "_")
		resultType, _, _ := resultTypes()
		return "func() " + resultType + " { if _, err := stdtime.LoadLocation(" + arguments[0] + "); err != nil { return " + resultError("InvalidTimeZone", arguments[0], "err.Error()") + " }; return " + resultOK("NewTimeZone("+arguments[0]+")") + " }()", true
	case "instant_now":
		return "func() *Instant { value := stdtime.Now(); return NewInstant(int(value.Unix()), value.Nanosecond()) }()", true
	case "instant_parse", "instant_try_parse":
		safe := operation == "instant_try_parse"
		returnType := "*Instant"
		if safe {
			returnType, _, _ = resultTypes()
		}
		body := "parsed, err := stdtime.Parse(stdtime.RFC3339Nano, " + arguments[0] + "); if err != nil {"
		if safe {
			body += " return " + resultError("InvalidInstant", arguments[0], "err.Error()")
		} else {
			body += " panic(\"invalid Instant: \" + err.Error())"
		}
		body += " }; result := NewInstant(int(parsed.Unix()), parsed.Nanosecond())"
		if safe {
			body += "; return " + resultOK("result")
		} else {
			body += "; return result"
		}
		return "func() " + returnType + " { " + body + " }()", true
	case "instant_to_string":
		return "stdtime.Unix(int64(" + arguments[0] + ".trbFieldEpochSeconds), int64(" + arguments[0] + ".trbFieldNanosecond)).UTC().Format(stdtime.RFC3339Nano)", true
	case "instant_to_datetime":
		g.requireImport("time/tzdata", "_")
		return "func() *DateTime { location, err := stdtime.LoadLocation(" + arguments[1] + ".trbFieldIdentifier); if err != nil { panic(\"invalid TimeZone: \" + err.Error()) }; value := stdtime.Unix(int64(" + arguments[0] + ".trbFieldEpochSeconds), int64(" + arguments[0] + ".trbFieldNanosecond)).In(location); return NewDateTime(value.Year(), int(value.Month()), value.Day(), value.Hour(), value.Minute(), value.Second(), value.Nanosecond()) }()", true
	case "instant_add", "instant_subtract":
		sign := "+"
		if operation == "instant_subtract" {
			sign = "-"
		}
		return "func() *Instant { seconds := " + arguments[0] + ".trbFieldEpochSeconds " + sign + " " + arguments[1] + ".trbFieldSeconds; nanosecond := " + arguments[0] + ".trbFieldNanosecond " + sign + " " + arguments[1] + ".trbFieldNanosecond; for nanosecond < 0 { seconds--; nanosecond += 1000000000 }; for nanosecond >= 1000000000 { seconds++; nanosecond -= 1000000000 }; return NewInstant(seconds, nanosecond) }()", true
	case "instant_duration_since":
		return "func() *Duration { seconds := " + arguments[0] + ".trbFieldEpochSeconds - " + arguments[1] + ".trbFieldEpochSeconds; nanosecond := " + arguments[0] + ".trbFieldNanosecond - " + arguments[1] + ".trbFieldNanosecond; if nanosecond < 0 { seconds--; nanosecond += 1000000000 }; return NewDuration(seconds, nanosecond) }()", true
	case "instant_compare":
		left, right := arguments[0], arguments[1]
		return "func() int { if " + left + ".trbFieldEpochSeconds < " + right + ".trbFieldEpochSeconds { return -1 }; if " + left + ".trbFieldEpochSeconds > " + right + ".trbFieldEpochSeconds { return 1 }; if " + left + ".trbFieldNanosecond < " + right + ".trbFieldNanosecond { return -1 }; if " + left + ".trbFieldNanosecond > " + right + ".trbFieldNanosecond { return 1 }; return 0 }()", true
	case "datetime_to_instant", "datetime_try_to_instant":
		return g.goTimeResolveLocal(call, arguments, operation == "datetime_try_to_instant", resultOK, resultError), true
	default:
		return "", false
	}
}

func (g *generator) goTimeResolveLocal(call *ir.Call, arguments []string, safe bool, resultOK func(string) string, resultError func(string, string, string) string) string {
	g.requireImport("time/tzdata", "_")
	local, zone := arguments[0], arguments[1]
	returnType := "*Instant"
	if safe {
		returnType = g.goType(call.ExprType())
	}
	failure := func(kind, message string) string {
		if safe {
			return "return " + resultError(kind, zone+".trbFieldIdentifier", strconv.Quote(message))
		}
		return "panic(" + strconv.Quote(message) + ")"
	}
	finish := "return result"
	if safe {
		finish = "return " + resultOK("result")
	}
	return "func() " + returnType + " { location, err := stdtime.LoadLocation(" + zone + ".trbFieldIdentifier); if err != nil { " + failure("InvalidTimeZone", "invalid TimeZone") + " }; civil := stdtime.Date(" + local + ".trbFieldYear, stdtime.Month(" + local + ".trbFieldMonth), " + local + ".trbFieldDay, " + local + ".trbFieldHour, " + local + ".trbFieldMinute, " + local + ".trbFieldSecond, " + local + ".trbFieldNanosecond, stdtime.UTC); offsets := map[int]bool{}; for probe := civil.Add(-36 * stdtime.Hour); !probe.After(civil.Add(36 * stdtime.Hour)); probe = probe.Add(stdtime.Hour) { _, offset := probe.In(location).Zone(); offsets[offset] = true }; matches := []stdtime.Time{}; for offset := range offsets { candidate := civil.Add(-stdtime.Duration(offset) * stdtime.Second); viewed := candidate.In(location); if viewed.Year() == " + local + ".trbFieldYear && int(viewed.Month()) == " + local + ".trbFieldMonth && viewed.Day() == " + local + ".trbFieldDay && viewed.Hour() == " + local + ".trbFieldHour && viewed.Minute() == " + local + ".trbFieldMinute && viewed.Second() == " + local + ".trbFieldSecond { matches = append(matches, candidate) } }; if len(matches) == 0 { " + failure("NonexistentLocalTime", "local DateTime does not exist in TimeZone") + " }; if len(matches) > 1 { " + failure("AmbiguousLocalTime", "local DateTime is ambiguous in TimeZone") + " }; result := NewInstant(int(matches[0].Unix()), matches[0].Nanosecond()); " + finish + " }()"
}
