package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) timeIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.internal.time.") {
		return "", false
	}
	operation := strings.TrimPrefix(name, "trb.internal.time.")
	field := func(value, name string) string { return value + ".__trb_" + name }
	resultTypes := func() (string, string, string) {
		result := call.ExprType()
		if len(result.Args) != 2 {
			return g.tsType(result), "unknown", "unknown"
		}
		return g.tsType(result), g.tsType(result.Args[0]), g.tsType(result.Args[1])
	}
	ok := func(value string) string {
		_, valueType, errorType := resultTypes()
		return g.runtimeName("Result") + ".Ok<" + valueType + ", " + errorType + ">(" + value + ")"
	}
	err := func(kind, input, message string) string {
		_, valueType, errorType := resultTypes()
		value := "({ kind: " + g.runtimeName("DateTimeErrorKind") + "." + kind + ", input: " + input + ", message: " + message + " } satisfies " + errorType + ")"
		return g.runtimeName("Result") + ".Err<" + valueType + ", " + errorType + ">(" + value + ")"
	}
	dateValid := func(year, month, day string) string {
		return "((): boolean => { const value = new globalThis.Date(0); value.setUTCHours(0, 0, 0, 0); value.setUTCFullYear(" + year + ", " + month + " - 1, " + day + "); return value.getUTCFullYear() === " + year + " && value.getUTCMonth() === " + month + " - 1 && value.getUTCDate() === " + day + "; })()"
	}
	timeValid := func(hour, minute, second, nanosecond string) string {
		return hour + " >= 0 && " + hour + " <= 23 && " + minute + " >= 0 && " + minute + " <= 59 && " + second + " >= 0 && " + second + " <= 59 && " + nanosecond + " >= 0 && " + nanosecond + " < 1000000000"
	}
	dateParts := func(value string) (string, string, string) {
		return value + ".year()", value + ".month()", value + ".day()"
	}
	timeParts := func(value string) (string, string, string, string) {
		return value + ".hour()", value + ".minute()", value + ".second()", value + ".nanosecond()"
	}
	pad := func(value string, width int) string {
		return "String(" + value + ").padStart(" + strconv.Itoa(width) + ", \"0\")"
	}
	fraction := func(value string) string {
		return "(" + value + " === 0 ? \"\" : \".\" + String(" + value + " + 1000000000).slice(1).replace(/0+$/, \"\"))"
	}
	compare := func(left, right string) string {
		return "(" + left + " < " + right + " ? -1 : (" + left + " > " + right + " ? 1 : 0))"
	}
	daysFromCivil := func(year, month, day string) string {
		return "((year: number, month: number, day: number): number => { year -= month <= 2 ? 1 : 0; const era = Math.floor(year / 400); const yoe = year - era * 400; const adjustedMonth = month + (month > 2 ? -3 : 9); const doy = Math.floor((153 * adjustedMonth + 2) / 5) + day - 1; const doe = yoe * 365 + Math.floor(yoe / 4) - Math.floor(yoe / 100) + doy; return era * 146097 + doe - 719468; })(" + year + ", " + month + ", " + day + ")"
	}
	civilFromDays := func(days string) string {
		return "((days: number): [number, number, number] => { const shifted = days + 719468; const era = Math.floor(shifted / 146097); const doe = shifted - era * 146097; const yoe = Math.floor((doe - Math.floor(doe / 1460) + Math.floor(doe / 36524) - Math.floor(doe / 146096)) / 365); let year = yoe + era * 400; const doy = doe - (365 * yoe + Math.floor(yoe / 4) - Math.floor(yoe / 100)); const mp = Math.floor((5 * doy + 2) / 153); const day = doy - Math.floor((153 * mp + 2) / 5) + 1; const month = mp + (mp < 10 ? 3 : -9); year += month <= 2 ? 1 : 0; return [year, month, day]; })(" + days + ")"
	}
	parseDate := func(input string, safe bool) string {
		resultType, _, _ := resultTypes()
		returnType := "Date"
		failure := "throw new RangeError(\"invalid Date\")"
		finish := "new Date(year, month, day)"
		if safe {
			returnType = resultType
			failure = "return " + err("InvalidDate", input, `"invalid Date"`)
			finish = "return " + ok(finish)
		} else {
			finish = "return " + finish
		}
		return "((): " + returnType + " => { const match = /^(\\d{4})-(\\d{2})-(\\d{2})$/.exec(" + input + "); if (match === null) { " + failure + "; } const year = Number(match[1]); const month = Number(match[2]); const day = Number(match[3]); if (!(" + dateValid("year", "month", "day") + ")) { " + failure + "; } " + finish + "; })()"
	}
	parseClock := func(input string, safe, datetime bool) string {
		resultType, _, _ := resultTypes()
		pattern := `/^(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?$/`
		kind, message, returnType := "InvalidTime", "invalid TimeOfDay", "TimeOfDay"
		captures := "const hour = Number(match[1]); const minute = Number(match[2]); const second = Number(match[3] ?? \"0\"); const fraction = match[4] ?? \"\"; const nanosecond = Number((fraction + \"000000000\").slice(0, 9)); const valid = " + timeValid("hour", "minute", "second", "nanosecond") + ";"
		construct := "new TimeOfDay(hour, minute, second, nanosecond)"
		if datetime {
			pattern = `/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?$/`
			kind, message, returnType = "InvalidDateTime", "invalid DateTime", "DateTime"
			captures = "const year = Number(match[1]); const month = Number(match[2]); const day = Number(match[3]); const hour = Number(match[4]); const minute = Number(match[5]); const second = Number(match[6] ?? \"0\"); const fraction = match[7] ?? \"\"; const nanosecond = Number((fraction + \"000000000\").slice(0, 9)); const valid = " + dateValid("year", "month", "day") + " && " + timeValid("hour", "minute", "second", "nanosecond") + ";"
			construct = "new DateTime(year, month, day, hour, minute, second, nanosecond)"
		}
		failure := "throw new RangeError(" + strconv.Quote(message) + ")"
		finish := "return " + construct
		if safe {
			returnType = resultType
			failure = "return " + err(kind, input, strconv.Quote(message))
			finish = "return " + ok(construct)
		}
		return "((): " + returnType + " => { const match = " + pattern + ".exec(" + input + "); if (match === null) { " + failure + "; } " + captures + " if (!valid) { " + failure + "; } " + finish + "; })()"
	}
	zoneValid := func(identifier string) string {
		return "((identifier: string): boolean => { try { new Intl.DateTimeFormat(\"en-US\", { timeZone: identifier }).format(0); return true; } catch { return false; } })(" + identifier + ")"
	}

	switch operation {
	case "validate_date":
		return "((): void => { if (!(" + dateValid(arguments[0], arguments[1], arguments[2]) + ")) { throw new RangeError(\"invalid Date\"); } })()", true
	case "date_try_new":
		resultType, _, _ := resultTypes()
		return "((): " + resultType + " => " + dateValid(arguments[0], arguments[1], arguments[2]) + " ? " + ok("new Date("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidDate", `""`, `"invalid Date"`) + ")()", true
	case "date_parse":
		return parseDate(arguments[0], false), true
	case "date_try_parse":
		return parseDate(arguments[0], true), true
	case "date_to_string":
		year, month, day := dateParts(arguments[0])
		return pad(year, 4) + " + \"-\" + " + pad(month, 2) + " + \"-\" + " + pad(day, 2), true
	case "date_add_days":
		year, month, day := dateParts(arguments[0])
		return "((): Date => { const [nextYear, nextMonth, nextDay] = " + civilFromDays(daysFromCivil(year, month, day)+" + "+arguments[1]) + "; if (nextYear < 1 || nextYear > 9999) { throw new RangeError(\"Date is outside the portable range\"); } return new Date(nextYear, nextMonth, nextDay); })()", true
	case "date_compare":
		ly, lm, ld := dateParts(arguments[0])
		ry, rm, rd := dateParts(arguments[1])
		return compare("("+ly+" * 10000 + "+lm+" * 100 + "+ld+")", "("+ry+" * 10000 + "+rm+" * 100 + "+rd+")"), true
	case "validate_time":
		return "((): void => { if (!(" + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]) + ")) { throw new RangeError(\"invalid TimeOfDay\"); } })()", true
	case "time_of_day_try_new":
		resultType, _, _ := resultTypes()
		return "((): " + resultType + " => " + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]) + " ? " + ok("new TimeOfDay("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidTime", `""`, `"invalid TimeOfDay"`) + ")()", true
	case "time_of_day_parse":
		return parseClock(arguments[0], false, false), true
	case "time_of_day_try_parse":
		return parseClock(arguments[0], true, false), true
	case "time_of_day_to_string":
		hour, minute, second, nanosecond := timeParts(arguments[0])
		return pad(hour, 2) + " + \":\" + " + pad(minute, 2) + " + \":\" + " + pad(second, 2) + " + " + fraction(nanosecond), true
	case "time_of_day_compare":
		lh, lm, ls, ln := timeParts(arguments[0])
		rh, rm, rs, rn := timeParts(arguments[1])
		return compare("(("+lh+" * 60 + "+lm+") * 60 + "+ls+") * 1000000000 + "+ln, "(("+rh+" * 60 + "+rm+") * 60 + "+rs+") * 1000000000 + "+rn), true
	case "validate_datetime":
		return "((): void => { if (!(" + dateValid(arguments[0], arguments[1], arguments[2]) + " && " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6]) + ")) { throw new RangeError(\"invalid DateTime\"); } })()", true
	case "datetime_try_new":
		resultType, _, _ := resultTypes()
		valid := dateValid(arguments[0], arguments[1], arguments[2]) + " && " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6])
		return "((): " + resultType + " => " + valid + " ? " + ok("new DateTime("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidDateTime", `""`, `"invalid DateTime"`) + ")()", true
	case "datetime_parse":
		return parseClock(arguments[0], false, true), true
	case "datetime_try_parse":
		return parseClock(arguments[0], true, true), true
	case "datetime_to_string":
		year, month, day := dateParts(arguments[0])
		hour, minute, second, nanosecond := timeParts(arguments[0])
		return pad(year, 4) + " + \"-\" + " + pad(month, 2) + " + \"-\" + " + pad(day, 2) + " + \"T\" + " + pad(hour, 2) + " + \":\" + " + pad(minute, 2) + " + \":\" + " + pad(second, 2) + " + " + fraction(nanosecond), true
	case "datetime_compare":
		ly, lm, ld := dateParts(arguments[0])
		lh, lmin, ls, ln := timeParts(arguments[0])
		ry, rm, rd := dateParts(arguments[1])
		rh, rmin, rs, rn := timeParts(arguments[1])
		left := "[" + strings.Join([]string{ly, lm, ld, lh, lmin, ls, ln}, ", ") + "]"
		right := "[" + strings.Join([]string{ry, rm, rd, rh, rmin, rs, rn}, ", ") + "]"
		return "((): number => { const left = " + left + "; const right = " + right + "; for (let index = 0; index < left.length; index += 1) { if (left[index]! < right[index]!) { return -1; } if (left[index]! > right[index]!) { return 1; } } return 0; })()", true
	case "validate_duration", "validate_instant":
		label := "Duration"
		condition := arguments[1] + " < 0 || " + arguments[1] + " >= 1000000000"
		if operation == "validate_instant" {
			label = "Instant"
			condition += " || " + arguments[0] + " < -62135596800 || " + arguments[0] + " > 253402300799"
		}
		return "((): void => { if (" + condition + ") { throw new RangeError(" + strconv.Quote(label+" is outside the portable range") + "); } })()", true
	case "duration_from_seconds":
		return "new Duration(" + arguments[0] + ")", true
	case "duration_from_milliseconds":
		return "((value: number): Duration => { let seconds = Math.trunc(value / 1000); let remainder = value % 1000; if (remainder < 0) { seconds -= 1; remainder += 1000; } return new Duration(seconds, remainder * 1000000); })(" + arguments[0] + ")", true
	case "duration_parse", "duration_try_parse":
		safe := operation == "duration_try_parse"
		resultType := "Duration"
		failure := "throw new RangeError(\"invalid Duration\")"
		finish := "return new Duration(seconds, nanosecond)"
		if safe {
			resultType, _, _ = resultTypes()
			failure = "return " + err("InvalidDuration", arguments[0], `"invalid Duration"`)
			finish = "return " + ok("new Duration(seconds, nanosecond)")
		}
		return "((): " + resultType + " => { const match = /^(-)?PT([0-9]+)(?:\\.([0-9]{1,9}))?S$/.exec(" + arguments[0] + "); if (match === null) { " + failure + "; } let seconds = Number(match[2]); const fraction = match[3] ?? \"\"; let nanosecond = Number((fraction + \"000000000\").slice(0, 9)); if (!Number.isSafeInteger(seconds)) { " + failure + "; } if (match[1] !== undefined) { seconds = -seconds; if (nanosecond !== 0) { seconds -= 1; nanosecond = 1000000000 - nanosecond; } } " + finish + "; })()", true
	case "duration_add", "duration_subtract":
		sign := "+"
		if operation == "duration_subtract" {
			sign = "-"
		}
		return "((): Duration => { let seconds = " + field(arguments[0], "seconds") + " " + sign + " " + field(arguments[1], "seconds") + "; let nanosecond = " + field(arguments[0], "nanosecond") + " " + sign + " " + field(arguments[1], "nanosecond") + "; while (nanosecond < 0) { seconds -= 1; nanosecond += 1000000000; } while (nanosecond >= 1000000000) { seconds += 1; nanosecond -= 1000000000; } return new Duration(seconds, nanosecond); })()", true
	case "duration_negative":
		return "((): Duration => { const seconds = " + field(arguments[0], "seconds") + "; const nanosecond = " + field(arguments[0], "nanosecond") + "; return nanosecond === 0 ? new Duration(-seconds) : new Duration(-seconds - 1, 1000000000 - nanosecond); })()", true
	case "duration_to_string":
		return "((): string => { let seconds = " + field(arguments[0], "seconds") + "; let nanosecond = " + field(arguments[0], "nanosecond") + "; let sign = \"\"; if (seconds < 0) { sign = \"-\"; seconds = -seconds - 1; if (nanosecond === 0) { seconds += 1; } else { nanosecond = 1000000000 - nanosecond; } } return sign + \"PT\" + String(seconds) + " + fraction("nanosecond") + " + \"S\"; })()", true
	case "duration_compare":
		left, right := arguments[0], arguments[1]
		return "((): number => { const leftSeconds = " + field(left, "seconds") + "; const rightSeconds = " + field(right, "seconds") + "; if (leftSeconds !== rightSeconds) { return leftSeconds < rightSeconds ? -1 : 1; } const leftNanos = " + field(left, "nanosecond") + "; const rightNanos = " + field(right, "nanosecond") + "; return leftNanos < rightNanos ? -1 : (leftNanos > rightNanos ? 1 : 0); })()", true
	case "validate_time_zone":
		return "((): void => { if (!(" + zoneValid(arguments[0]) + ")) { throw new RangeError(\"invalid TimeZone\"); } })()", true
	case "time_zone_try_get":
		resultType, _, _ := resultTypes()
		return "((): " + resultType + " => " + zoneValid(arguments[0]) + " ? " + ok("new TimeZone("+arguments[0]+")") + " : " + err("InvalidTimeZone", arguments[0], `"invalid TimeZone"`) + ")()", true
	case "instant_now":
		return "((): Instant => { const milliseconds = globalThis.Date.now(); const seconds = Math.floor(milliseconds / 1000); return new Instant(seconds, (milliseconds - seconds * 1000) * 1000000); })()", true
	case "instant_parse", "instant_try_parse":
		safe := operation == "instant_try_parse"
		resultType, _, _ := resultTypes()
		returnType := "Instant"
		failure := "throw new RangeError(\"invalid Instant\")"
		finish := "return new Instant(epochSeconds, nanosecond)"
		if safe {
			returnType = resultType
			failure = "return " + err("InvalidInstant", arguments[0], `"invalid Instant"`)
			finish = "return " + ok("new Instant(epochSeconds, nanosecond)")
		}
		return "((): " + returnType + " => { const match = /^(\\d{4})-(\\d{2})-(\\d{2})T(\\d{2}):(\\d{2}):(\\d{2})(?:\\.(\\d{1,9}))?(Z|([+-])(\\d{2}):(\\d{2}))$/.exec(" + arguments[0] + "); if (match === null) { " + failure + "; } const year = Number(match[1]); const month = Number(match[2]); const day = Number(match[3]); const hour = Number(match[4]); const minute = Number(match[5]); const second = Number(match[6]); const fraction = match[7] ?? \"\"; const nanosecond = Number((fraction + \"000000000\").slice(0, 9)); const offsetHour = Number(match[10] ?? \"0\"); const offsetMinute = Number(match[11] ?? \"0\"); if (!(" + dateValid("year", "month", "day") + " && " + timeValid("hour", "minute", "second", "nanosecond") + ") || offsetHour > 23 || offsetMinute > 59) { " + failure + "; } const offsetSign = match[9] === \"-\" ? -1 : 1; const offsetSeconds = match[8] === \"Z\" ? 0 : offsetSign * (offsetHour * 60 + offsetMinute) * 60; const epochSeconds = " + daysFromCivil("year", "month", "day") + " * 86400 + (hour * 60 + minute) * 60 + second - offsetSeconds; " + finish + "; })()", true
	case "instant_to_string":
		return "((): string => { const value = " + arguments[0] + "; const date = new globalThis.Date(value.epoch_seconds() * 1000); const base = date.toISOString().slice(0, 19); return base + " + fraction(arguments[0]+".nanosecond()") + " + \"Z\"; })()", true
	case "instant_add", "instant_subtract":
		sign := "+"
		if operation == "instant_subtract" {
			sign = "-"
		}
		return "((): Instant => { let seconds = " + arguments[0] + ".epoch_seconds() " + sign + " " + arguments[1] + ".whole_seconds(); let nanosecond = " + arguments[0] + ".nanosecond() " + sign + " " + arguments[1] + ".nanosecond(); while (nanosecond < 0) { seconds -= 1; nanosecond += 1000000000; } while (nanosecond >= 1000000000) { seconds += 1; nanosecond -= 1000000000; } return new Instant(seconds, nanosecond); })()", true
	case "instant_duration_since":
		return "((): Duration => { let seconds = " + arguments[0] + ".epoch_seconds() - " + arguments[1] + ".epoch_seconds(); let nanosecond = " + arguments[0] + ".nanosecond() - " + arguments[1] + ".nanosecond(); if (nanosecond < 0) { seconds -= 1; nanosecond += 1000000000; } return new Duration(seconds, nanosecond); })()", true
	case "instant_compare":
		left, right := arguments[0], arguments[1]
		return "((): number => { const leftSeconds = " + left + ".epoch_seconds(); const rightSeconds = " + right + ".epoch_seconds(); if (leftSeconds !== rightSeconds) { return leftSeconds < rightSeconds ? -1 : 1; } const leftNanos = " + left + ".nanosecond(); const rightNanos = " + right + ".nanosecond(); return leftNanos < rightNanos ? -1 : (leftNanos > rightNanos ? 1 : 0); })()", true
	case "instant_to_datetime":
		return tsInstantToDateTime(arguments[0], arguments[1]), true
	case "datetime_to_instant", "datetime_try_to_instant":
		return g.tsDateTimeToInstant(call, arguments[0], arguments[1], operation == "datetime_try_to_instant", daysFromCivil, ok, err), true
	default:
		return "", false
	}
}

func tsInstantToDateTime(instant, zone string) string {
	return "((): DateTime => { const formatter = new Intl.DateTimeFormat(\"en-US-u-ca-iso8601-nu-latn\", { timeZone: " + zone + ".identifier(), year: \"numeric\", month: \"2-digit\", day: \"2-digit\", hour: \"2-digit\", minute: \"2-digit\", second: \"2-digit\", hourCycle: \"h23\" }); const parts = Object.fromEntries(formatter.formatToParts(" + instant + ".epoch_seconds() * 1000).filter((part) => part.type !== \"literal\").map((part) => [part.type, Number(part.value)])); return new DateTime(parts.year, parts.month, parts.day, parts.hour, parts.minute, parts.second, " + instant + ".nanosecond()); })()"
}

func (g *generator) tsDateTimeToInstant(call *ir.Call, local, zone string, safe bool, daysFromCivil func(string, string, string) string, ok func(string) string, err func(string, string, string) string) string {
	resultType := "Instant"
	none := "throw new RangeError(\"local DateTime does not exist in TimeZone\")"
	ambiguous := "throw new RangeError(\"local DateTime is ambiguous in TimeZone\")"
	finish := "return new Instant(matches[0]!, " + local + ".nanosecond())"
	if safe {
		resultType = g.tsType(call.ExprType())
		none = "return " + err("NonexistentLocalTime", zone+".identifier()", `"local DateTime does not exist in TimeZone"`)
		ambiguous = "return " + err("AmbiguousLocalTime", zone+".identifier()", `"local DateTime is ambiguous in TimeZone"`)
		finish = "return " + ok("new Instant(matches[0]!, "+local+".nanosecond())")
	}
	localSeconds := daysFromCivil(local+".year()", local+".month()", local+".day()") + " * 86400 + (" + local + ".hour() * 60 + " + local + ".minute()) * 60 + " + local + ".second()"
	return "((): " + resultType + " => { const formatter = new Intl.DateTimeFormat(\"en-US-u-ca-iso8601-nu-latn\", { timeZone: " + zone + ".identifier(), year: \"numeric\", month: \"2-digit\", day: \"2-digit\", hour: \"2-digit\", minute: \"2-digit\", second: \"2-digit\", hourCycle: \"h23\" }); const components = (epochSeconds: number): Record<string, number> => Object.fromEntries(formatter.formatToParts(epochSeconds * 1000).filter((part) => part.type !== \"literal\").map((part) => [part.type, Number(part.value)])); const localSeconds = " + localSeconds + "; const offsets = new Set<number>(); for (let probe = localSeconds - 36 * 3600; probe <= localSeconds + 36 * 3600; probe += 3600) { const viewed = components(probe); const viewedSeconds = " + daysFromCivil("viewed.year!", "viewed.month!", "viewed.day!") + " * 86400 + (viewed.hour! * 60 + viewed.minute!) * 60 + viewed.second!; offsets.add(viewedSeconds - probe); } const matches: Array<number> = []; for (const offset of offsets) { const candidate = localSeconds - offset; const viewed = components(candidate); if (viewed.year === " + local + ".year() && viewed.month === " + local + ".month() && viewed.day === " + local + ".day() && viewed.hour === " + local + ".hour() && viewed.minute === " + local + ".minute() && viewed.second === " + local + ".second()) { matches.push(candidate); } } if (matches.length === 0) { " + none + "; } if (matches.length > 1) { " + ambiguous + "; } " + finish + "; })()"
}
