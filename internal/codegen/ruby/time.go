package ruby

import (
	"strconv"
	"strings"
)

func rubyTimeIntrinsic(name string, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, "trb.internal.time.") {
		return "", false
	}
	operation := strings.TrimPrefix(name, "trb.internal.time.")
	dateClass := rubyTimeRuntimeClass("Date")
	timeOfDayClass := rubyTimeRuntimeClass("TimeOfDay")
	dateTimeClass := rubyTimeRuntimeClass("DateTime")
	durationClass := rubyTimeRuntimeClass("Duration")
	timeZoneClass := rubyTimeRuntimeClass("TimeZone")
	instantClass := rubyTimeRuntimeClass("Instant")
	field := func(value, name string) string { return value + ".instance_variable_get(:@_" + name + ")" }
	errorValue := func(kind, input, message string) string {
		return "DateTimeError.new(kind: DateTimeErrorKind::" + kind + ", input: " + input + ", message: " + message + ")"
	}
	ok := func(value string) string { return "Result::Ok.new(" + value + ")" }
	err := func(kind, input, message string) string {
		return "Result::Err.new(" + errorValue(kind, input, message) + ")"
	}
	dateParts := func(value string) (string, string, string) {
		return field(value, "year"), field(value, "month"), field(value, "day")
	}
	timeParts := func(value string) (string, string, string, string) {
		return field(value, "hour"), field(value, "minute"), field(value, "second"), field(value, "nanosecond")
	}
	dateValid := func(year, month, day string) string {
		return "-> { value = Time.utc(" + year + ", " + month + ", " + day + "); value.year == " + year + " && value.month == " + month + " && value.day == " + day + " }.call"
	}
	timeValid := func(hour, minute, second, nanosecond string) string {
		return hour + ".between?(0, 23) && " + minute + ".between?(0, 59) && " + second + ".between?(0, 59) && " + nanosecond + ".between?(0, 999999999)"
	}
	compare := func(left, right string) string {
		return "-> { left = " + left + "; right = " + right + "; left < right ? -1 : (left > right ? 1 : 0) }.call"
	}
	parseDate := func(input string, safe bool) string {
		failure := "raise \"invalid Date\""
		finish := dateClass + ".new(year, month, day)"
		if safe {
			failure = err("InvalidDate", input, `"invalid Date"`)
			finish = ok(finish)
		}
		return "-> { match = /\\A(\\d{4})-(\\d{2})-(\\d{2})\\z/.match(" + input + "); unless match; next " + failure + "; end; year, month, day = match.captures.map(&:to_i); begin; " + dateValid("year", "month", "day") + " or raise ArgumentError; rescue StandardError; next " + failure + "; end; " + finish + " }.call"
	}
	parseClock := func(input string, safe, datetime bool) string {
		pattern := `\A(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?\z`
		kind := "InvalidTime"
		failureText := "invalid TimeOfDay"
		construct := timeOfDayClass + ".new(hour, minute, second, nanosecond)"
		capture := "hour = match[1].to_i; minute = match[2].to_i; second = (match[3] || \"0\").to_i; fraction = match[4] || \"\"; nanosecond = (fraction + \"0\" * (9 - fraction.length)).to_i; valid = " + timeValid("hour", "minute", "second", "nanosecond")
		if datetime {
			pattern = `\A(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2})(?:\.(\d{1,9}))?)?\z`
			kind = "InvalidDateTime"
			failureText = "invalid DateTime"
			construct = dateTimeClass + ".new(year, month, day, hour, minute, second, nanosecond)"
			capture = "year = match[1].to_i; month = match[2].to_i; day = match[3].to_i; hour = match[4].to_i; minute = match[5].to_i; second = (match[6] || \"0\").to_i; fraction = match[7] || \"\"; nanosecond = (fraction + \"0\" * (9 - fraction.length)).to_i; begin; date_ok = " + dateValid("year", "month", "day") + "; rescue StandardError; date_ok = false; end; valid = date_ok && " + timeValid("hour", "minute", "second", "nanosecond")
		}
		failure := "raise " + strconv.Quote(failureText)
		finish := construct
		if safe {
			failure = err(kind, input, strconv.Quote(failureText))
			finish = ok(construct)
		}
		return "-> { match = /" + pattern + "/.match(" + input + "); unless match; next " + failure + "; end; " + capture + "; unless valid; next " + failure + "; end; " + finish + " }.call"
	}
	formatFraction := func(nanosecond string) string {
		return "->(value) { value.zero? ? \"\" : \".\" + format(\"%09d\", value).sub(/0+\\z/, \"\") }.call(" + nanosecond + ")"
	}
	zoneValid := func(identifier string) string {
		return "-> { identifier = " + identifier + "; next true if identifier == \"UTC\" || identifier == \"Etc/UTC\"; next false unless /\\A[A-Za-z0-9._+-]+(?:\\/[A-Za-z0-9._+-]+)+\\z/.match?(identifier); roots = [ENV[\"TZDIR\"], \"/usr/share/zoneinfo\", \"/usr/share/lib/zoneinfo\", \"/usr/lib/zoneinfo\"].compact; roots.any? { |root| File.file?(File.join(root, identifier)) } }.call"
	}
	withZone := func(identifier, body string) string {
		return "-> { require \"thread\"; mutex = if Object.const_defined?(:TRB_TIME_ZONE_MUTEX, false); Object.const_get(:TRB_TIME_ZONE_MUTEX); else; Object.const_set(:TRB_TIME_ZONE_MUTEX, Mutex.new); end; mutex.synchronize { previous = ENV[\"TZ\"]; begin; ENV[\"TZ\"] = " + identifier + "; " + body + "; ensure; previous.nil? ? ENV.delete(\"TZ\") : ENV[\"TZ\"] = previous; end } }.call"
	}

	switch operation {
	case "validate_date":
		return "-> { begin; " + dateValid(arguments[0], arguments[1], arguments[2]) + " or raise ArgumentError; rescue StandardError; raise \"invalid Date\"; end }.call", true
	case "date_try_new":
		return "-> { begin; valid = " + dateValid(arguments[0], arguments[1], arguments[2]) + "; rescue StandardError; valid = false; end; valid ? " + ok(dateClass+".new("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidDate", `""`, `"invalid Date"`) + " }.call", true
	case "date_parse":
		return parseDate(arguments[0], false), true
	case "date_try_parse":
		return parseDate(arguments[0], true), true
	case "date_to_string":
		year, month, day := dateParts(arguments[0])
		return "format(\"%04d-%02d-%02d\", " + year + ", " + month + ", " + day + ")", true
	case "date_add_days":
		year, month, day := dateParts(arguments[0])
		return "-> { value = Time.utc(" + year + ", " + month + ", " + day + ") + " + arguments[1] + " * 86400; raise \"Date is outside the portable range\" unless value.year.between?(1, 9999); " + dateClass + ".new(value.year, value.month, value.day) }.call", true
	case "date_compare":
		ly, lm, ld := dateParts(arguments[0])
		ry, rm, rd := dateParts(arguments[1])
		return compare("("+ly+" * 10000 + "+lm+" * 100 + "+ld+")", "("+ry+" * 10000 + "+rm+" * 100 + "+rd+")"), true
	case "validate_time":
		return "raise(\"invalid TimeOfDay\") unless " + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]), true
	case "time_of_day_try_new":
		return "(" + timeValid(arguments[0], arguments[1], arguments[2], arguments[3]) + " ? " + ok(timeOfDayClass+".new("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidTime", `""`, `"invalid TimeOfDay"`) + ")", true
	case "time_of_day_parse":
		return parseClock(arguments[0], false, false), true
	case "time_of_day_try_parse":
		return parseClock(arguments[0], true, false), true
	case "time_of_day_to_string":
		hour, minute, second, nanosecond := timeParts(arguments[0])
		return "format(\"%02d:%02d:%02d\", " + hour + ", " + minute + ", " + second + ") + " + formatFraction(nanosecond), true
	case "time_of_day_compare":
		lh, lm, ls, ln := timeParts(arguments[0])
		rh, rm, rs, rn := timeParts(arguments[1])
		return compare("(("+lh+" * 60 + "+lm+") * 60 + "+ls+") * 1000000000 + "+ln, "(("+rh+" * 60 + "+rm+") * 60 + "+rs+") * 1000000000 + "+rn), true
	case "validate_datetime":
		return "-> { begin; date_ok = " + dateValid(arguments[0], arguments[1], arguments[2]) + "; rescue StandardError; date_ok = false; end; raise \"invalid DateTime\" unless date_ok && " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6]) + " }.call", true
	case "datetime_try_new":
		return "-> { begin; date_ok = " + dateValid(arguments[0], arguments[1], arguments[2]) + "; rescue StandardError; date_ok = false; end; valid = date_ok && " + timeValid(arguments[3], arguments[4], arguments[5], arguments[6]) + "; valid ? " + ok(dateTimeClass+".new("+strings.Join(arguments, ", ")+")") + " : " + err("InvalidDateTime", `""`, `"invalid DateTime"`) + " }.call", true
	case "datetime_parse":
		return parseClock(arguments[0], false, true), true
	case "datetime_try_parse":
		return parseClock(arguments[0], true, true), true
	case "datetime_to_string":
		year, month, day := dateParts(arguments[0])
		hour, minute, second, nanosecond := timeParts(arguments[0])
		return "format(\"%04d-%02d-%02dT%02d:%02d:%02d\", " + strings.Join([]string{year, month, day, hour, minute, second}, ", ") + ") + " + formatFraction(nanosecond), true
	case "datetime_compare":
		ly, lm, ld := dateParts(arguments[0])
		lh, lmin, ls, ln := timeParts(arguments[0])
		ry, rm, rd := dateParts(arguments[1])
		rh, rmin, rs, rn := timeParts(arguments[1])
		left := "Time.utc(" + strings.Join([]string{ly, lm, ld, lh, lmin, ls}, ", ") + ", Rational(" + ln + ", 1000))"
		right := "Time.utc(" + strings.Join([]string{ry, rm, rd, rh, rmin, rs}, ", ") + ", Rational(" + rn + ", 1000))"
		return compare(left, right), true
	case "validate_duration", "validate_instant":
		label := "Duration"
		condition := arguments[1] + ".between?(0, 999999999)"
		if operation == "validate_instant" {
			label = "Instant"
			condition += " && " + arguments[0] + ".between?(-62135596800, 253402300799)"
		}
		return "raise(" + strconv.Quote(label+" is outside the portable range") + ") unless " + condition, true
	case "duration_from_seconds":
		return durationClass + ".new(" + arguments[0] + ")", true
	case "duration_from_milliseconds":
		return "-> { seconds, remainder = " + arguments[0] + ".divmod(1000); " + durationClass + ".new(seconds, remainder * 1000000) }.call", true
	case "duration_parse", "duration_try_parse":
		safe := operation == "duration_try_parse"
		failure := "raise \"invalid Duration\""
		finish := durationClass + ".new(seconds, nanosecond)"
		if safe {
			failure = err("InvalidDuration", arguments[0], `"invalid Duration"`)
			finish = ok(finish)
		}
		return "-> { match = /\\A(-)?PT([0-9]+)(?:\\.([0-9]{1,9}))?S\\z/.match(" + arguments[0] + "); next " + failure + " unless match; seconds = match[2].to_i; fraction = match[3] || \"\"; nanosecond = (fraction + \"0\" * (9 - fraction.length)).to_i; if match[1]; seconds = -seconds; if !nanosecond.zero?; seconds -= 1; nanosecond = 1000000000 - nanosecond; end; end; " + finish + " }.call", true
	case "duration_add", "duration_subtract":
		sign := "+"
		if operation == "duration_subtract" {
			sign = "-"
		}
		return "-> { seconds = " + field(arguments[0], "seconds") + " " + sign + " " + field(arguments[1], "seconds") + "; nanosecond = " + field(arguments[0], "nanosecond") + " " + sign + " " + field(arguments[1], "nanosecond") + "; carry, nanosecond = nanosecond.divmod(1000000000); " + durationClass + ".new(seconds + carry, nanosecond) }.call", true
	case "duration_negative":
		return "-> { seconds = " + field(arguments[0], "seconds") + "; nanosecond = " + field(arguments[0], "nanosecond") + "; nanosecond.zero? ? " + durationClass + ".new(-seconds) : " + durationClass + ".new(-seconds - 1, 1000000000 - nanosecond) }.call", true
	case "duration_to_string":
		return "-> { seconds = " + field(arguments[0], "seconds") + "; nanosecond = " + field(arguments[0], "nanosecond") + "; sign = \"\"; if seconds.negative?; sign = \"-\"; seconds = -seconds - 1; if nanosecond.zero?; seconds += 1; else; nanosecond = 1000000000 - nanosecond; end; end; sign + \"PT\" + seconds.to_s + " + formatFraction("nanosecond") + " + \"S\" }.call", true
	case "duration_compare":
		left, right := arguments[0], arguments[1]
		return "-> { left_seconds = " + field(left, "seconds") + "; right_seconds = " + field(right, "seconds") + "; next(left_seconds < right_seconds ? -1 : 1) unless left_seconds == right_seconds; left_nanos = " + field(left, "nanosecond") + "; right_nanos = " + field(right, "nanosecond") + "; left_nanos < right_nanos ? -1 : (left_nanos > right_nanos ? 1 : 0) }.call", true
	case "validate_time_zone":
		return "raise(\"invalid TimeZone\") unless " + zoneValid(arguments[0]), true
	case "time_zone_try_get":
		return "(" + zoneValid(arguments[0]) + " ? " + ok(timeZoneClass+".new("+arguments[0]+")") + " : " + err("InvalidTimeZone", arguments[0], `"invalid TimeZone"`) + ")", true
	case "instant_now":
		return "-> { value = Time.now; " + instantClass + ".new(value.to_i, value.nsec) }.call", true
	case "instant_parse", "instant_try_parse":
		safe := operation == "instant_try_parse"
		failure := "raise \"invalid Instant\""
		finish := instantClass + ".new(parsed.to_i, nanosecond)"
		if safe {
			failure = err("InvalidInstant", arguments[0], `"invalid Instant"`)
			finish = ok(finish)
		}
		return "-> { match = /\\A(\\d{4})-(\\d{2})-(\\d{2})T(\\d{2}):(\\d{2}):(\\d{2})(?:\\.(\\d{1,9}))?(Z|[+-]\\d{2}:\\d{2})\\z/.match(" + arguments[0] + "); unless match; next " + failure + "; end; year, month, day, hour, minute, second = match.captures[0, 6].map(&:to_i); fraction = match[7] || \"\"; nanosecond = (fraction + \"0\" * (9 - fraction.length)).to_i; offset = match[8] == \"Z\" ? \"UTC\" : match[8]; begin; parsed = Time.new(year, month, day, hour, minute, second + Rational(nanosecond, 1000000000), offset); rescue StandardError; next " + failure + "; end; " + finish + " }.call", true
	case "instant_to_string":
		return "-> { value = Time.at(" + field(arguments[0], "epoch_seconds") + ", " + field(arguments[0], "nanosecond") + ", :nanosecond).utc; value.strftime(\"%Y-%m-%dT%H:%M:%S\") + " + formatFraction("value.nsec") + " + \"Z\" }.call", true
	case "instant_to_datetime":
		body := "value = Time.at(" + field(arguments[0], "epoch_seconds") + ", " + field(arguments[0], "nanosecond") + ", :nanosecond).localtime; " + dateTimeClass + ".new(value.year, value.month, value.day, value.hour, value.min, value.sec, value.nsec)"
		return withZone(field(arguments[1], "identifier"), body), true
	case "instant_add", "instant_subtract":
		sign := "+"
		if operation == "instant_subtract" {
			sign = "-"
		}
		return "-> { seconds = " + field(arguments[0], "epoch_seconds") + " " + sign + " " + field(arguments[1], "seconds") + "; nanosecond = " + field(arguments[0], "nanosecond") + " " + sign + " " + field(arguments[1], "nanosecond") + "; carry, nanosecond = nanosecond.divmod(1000000000); " + instantClass + ".new(seconds + carry, nanosecond) }.call", true
	case "instant_duration_since":
		return "-> { seconds = " + field(arguments[0], "epoch_seconds") + " - " + field(arguments[1], "epoch_seconds") + "; nanosecond = " + field(arguments[0], "nanosecond") + " - " + field(arguments[1], "nanosecond") + "; carry, nanosecond = nanosecond.divmod(1000000000); " + durationClass + ".new(seconds + carry, nanosecond) }.call", true
	case "instant_compare":
		left, right := arguments[0], arguments[1]
		return "-> { left_seconds = " + field(left, "epoch_seconds") + "; right_seconds = " + field(right, "epoch_seconds") + "; next(left_seconds < right_seconds ? -1 : 1) unless left_seconds == right_seconds; left_nanos = " + field(left, "nanosecond") + "; right_nanos = " + field(right, "nanosecond") + "; left_nanos < right_nanos ? -1 : (left_nanos > right_nanos ? 1 : 0) }.call", true
	case "datetime_to_instant", "datetime_try_to_instant":
		safe := operation == "datetime_try_to_instant"
		year, month, day := dateParts(arguments[0])
		hour, minute, second, nanosecond := timeParts(arguments[0])
		identifier := field(arguments[1], "identifier")
		none := "raise \"local DateTime does not exist in TimeZone\""
		ambiguous := "raise \"local DateTime is ambiguous in TimeZone\""
		finish := instantClass + ".new(matches[0].to_i, matches[0].nsec)"
		if safe {
			none = err("NonexistentLocalTime", identifier, `"local DateTime does not exist in TimeZone"`)
			ambiguous = err("AmbiguousLocalTime", identifier, `"local DateTime is ambiguous in TimeZone"`)
			finish = ok(finish)
		}
		body := "civil = Time.utc(" + strings.Join([]string{year, month, day, hour, minute, second}, ", ") + ", Rational(" + nanosecond + ", 1000)); offsets = {}; probe = civil - 36 * 3600; while probe <= civil + 36 * 3600; viewed = probe.localtime; offsets[viewed.utc_offset] = true; probe += 3600; end; matches = offsets.keys.filter_map { |offset| candidate = civil - offset; viewed = candidate.localtime; viewed.year == " + year + " && viewed.month == " + month + " && viewed.day == " + day + " && viewed.hour == " + hour + " && viewed.min == " + minute + " && viewed.sec == " + second + " ? candidate : nil }; next " + none + " if matches.empty?; next " + ambiguous + " if matches.length > 1; " + finish
		return withZone(identifier, body), true
	default:
		return "", false
	}
}
