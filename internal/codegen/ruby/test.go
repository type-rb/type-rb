package ruby

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) testIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	switch name {
	case "trb.std.test.expect":
		position := call.SourceSpan().Start
		return "Expectation.new(" + arguments[0] + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")", true
	case "trb.std.test.expect_ok", "trb.std.test.expect_err":
		position := call.SourceSpan().Start
		method := "ok"
		if name == "trb.std.test.expect_err" {
			method = "err"
		}
		return "ResultExpectation.new(" + arguments[0] + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")." + method, true
	case "trb.std.test.describe":
		return "trb_test_describe(" + strings.Join(arguments, ", ") + ")", true
	case "trb.std.test.test":
		position := call.SourceSpan().Start
		return "trb_test(" + strings.Join(arguments, ", ") + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")", true
	case "trb.std.test.finish":
		return "trb_test_finish()", true
	case "trb.internal.test.assert_equal", "trb.internal.test.assert_not_equal", "trb.internal.test.assert_true", "trb.internal.test.assert_false", "trb.internal.test.assert_nil":
		return "trb_test_" + strings.TrimPrefix(name, "trb.internal.test.") + "(" + strings.Join(arguments, ", ") + ")", true
	case "trb.internal.test.assert_result_ok":
		return "(->(actual) { raise TrbTestFailure.new(" + arguments[1] + ", " + arguments[2] + ", " + arguments[3] + ", \"expected Ok, got Err(#{actual.error.inspect})\") unless actual.is_a?(Result::Ok); actual.value }).call(" + arguments[0] + ")", true
	case "trb.internal.test.assert_result_err":
		return "(->(actual) { raise TrbTestFailure.new(" + arguments[1] + ", " + arguments[2] + ", " + arguments[3] + ", \"expected Err, got Ok(#{actual.value.inspect})\") unless actual.is_a?(Result::Err); actual.error }).call(" + arguments[0] + ")", true
	default:
		return "", false
	}
}

func (g *generator) testRuntimeSupport() {
	g.line(`require "json"`, "")
	g.line(`class TrbTestFailure < StandardError`, "")
	g.indent++
	g.line(`attr_reader :path, :line, :column`, "")
	g.line(`def initialize(path, line, column, message); @path = path; @line = line; @column = column; super(message); end`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`$trb_test_suites = []`, "")
	g.line(`$trb_test_total = 0`, "")
	g.line(`$trb_test_failed = 0`, "")
	g.line(`trb_test_names_json = ENV["TRB_TEST_NAMES"]`, "")
	g.line(`$trb_test_names = trb_test_names_json && !trb_test_names_json.empty? ? JSON.parse(trb_test_names_json) : []`, "")
	g.line(`def trb_test_equal(left, right)`, "")
	g.indent++
	g.line(`return true if left.equal?(right)`, "")
	g.line(`return false unless left.class == right.class`, "")
	g.line(`if left.is_a?(Array)`, "")
	g.indent++
	g.line(`return left.length == right.length && left.each_index.all? { |index| trb_test_equal(left[index], right[index]) }`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`if left.is_a?(Hash)`, "")
	g.indent++
	g.line(`return left.length == right.length && left.all? { |key, value| right.key?(key) && trb_test_equal(value, right[key]) }`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`variables = left.instance_variables`, "")
	g.line(`other_variables = right.instance_variables`, "")
	g.line(`return variables.sort == other_variables.sort && variables.all? { |name| trb_test_equal(left.instance_variable_get(name), right.instance_variable_get(name)) } if variables.any? || other_variables.any?`, "")
	g.line(`left == right`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_test_event(type, name, test_file, file, line, column, message = nil)`, "")
	g.indent++
	g.line(`event = { type: type, name: name, test_file: test_file, file: file, line: line, column: column }`, "")
	g.line(`event[:message] = message if message`, "")
	g.line(`if ENV["TRB_TEST_REPORTER"] == "json"`, "")
	g.indent++
	g.line(`puts(JSON.generate(event))`, "")
	g.line(`return`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`puts("PASS #{name}") if type == "test_passed"`, "")
	g.line(`puts("FAIL #{name}\n  #{file}:#{line}:#{column}: #{message}") if type == "test_failed"`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_test_describe(name)`, "")
	g.indent++
	g.line(`$trb_test_suites << name`, "")
	g.line(`yield`, "")
	g.line(`ensure`, "")
	g.line(`$trb_test_suites.pop`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_test(name, file, line, column)`, "")
	g.indent++
	g.line(`full_name = ($trb_test_suites + [name]).join(" / ")`, "")
	g.line(`selected_file = ENV["TRB_TEST_FILE"]; return if selected_file && !selected_file.empty? && selected_file != file`, "")
	g.line(`return if !$trb_test_names.empty? && !$trb_test_names.include?(full_name)`, "")
	g.line(`$trb_test_total += 1`, "")
	g.line(`trb_test_event("test_started", full_name, file, file, line, column)`, "")
	g.line(`begin`, "")
	g.indent++
	g.line(`yield`, "")
	g.line(`trb_test_event("test_passed", full_name, file, file, line, column)`, "")
	g.indent--
	g.line(`rescue StandardError => error`, "")
	g.indent++
	g.line(`$trb_test_failed += 1`, "")
	g.line(`failure_file = error.is_a?(TrbTestFailure) ? error.path : file`, "")
	g.line(`failure_line = error.is_a?(TrbTestFailure) ? error.line : line`, "")
	g.line(`failure_column = error.is_a?(TrbTestFailure) ? error.column : column`, "")
	g.line(`trb_test_event("test_failed", full_name, file, failure_file, failure_line, failure_column, error.message)`, "")
	g.indent--
	g.line(`end`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_test_finish`, "")
	g.indent++
	g.line(`if ENV["TRB_TEST_REPORTER"] == "json"`, "")
	g.indent++
	g.line(`puts(JSON.generate(type: "test_summary", total: $trb_test_total, failed: $trb_test_failed))`, "")
	g.indent--
	g.line(`else`, "")
	g.indent++
	g.line(`puts("\n#{$trb_test_total} test(s), #{$trb_test_failed} failure(s)")`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`exit(1) if $trb_test_failed > 0`, "")
	g.indent--
	g.line(`end`, "")
	g.line(`def trb_test_assert_equal(actual, expected, path, line, column); raise TrbTestFailure.new(path, line, column, "expected #{actual.inspect} to equal #{expected.inspect}") unless trb_test_equal(actual, expected); end`, "")
	g.line(`def trb_test_assert_not_equal(actual, expected, path, line, column); raise TrbTestFailure.new(path, line, column, "expected #{actual.inspect} not to equal #{expected.inspect}") if trb_test_equal(actual, expected); end`, "")
	g.line(`def trb_test_assert_true(actual, path, line, column); raise TrbTestFailure.new(path, line, column, "expected #{actual.inspect} to be true") unless actual == true; end`, "")
	g.line(`def trb_test_assert_false(actual, path, line, column); raise TrbTestFailure.new(path, line, column, "expected #{actual.inspect} to be false") unless actual == false; end`, "")
	g.line(`def trb_test_assert_nil(actual, path, line, column); raise TrbTestFailure.new(path, line, column, "expected #{actual.inspect} to be nil") unless actual.nil?; end`, "")
}
