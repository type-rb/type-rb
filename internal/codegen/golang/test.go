package golang

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) testAlias(call *ir.Call) string {
	if reference := expressionReference(call.Callee); reference != nil && reference.Alias != "" {
		return goImportAlias(reference.Alias)
	}
	return "__trb_test"
}

func (g *generator) testIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	switch name {
	case "trb.std.test.expect":
		position := call.SourceSpan().Start
		typeArgument := "any"
		if result := call.ExprType(); len(result.Args) == 1 && result.Args[0].Kind != types.Nil {
			typeArgument = g.goType(result.Args[0])
		}
		return g.testAlias(call) + ".NewExpectation[" + typeArgument + "](" + arguments[0] + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")", true
	case "trb.std.test.finish":
		return g.testAlias(call) + ".TrbTestFinish()", true
	case "trb.internal.test.assert_equal", "trb.internal.test.assert_not_equal", "trb.internal.test.assert_true", "trb.internal.test.assert_false", "trb.internal.test.assert_nil":
		return "trbTest" + goMethodName(strings.TrimPrefix(name, "trb.internal.test.")) + "(" + strings.Join(arguments, ", ") + ")", true
	case "trb.std.test.describe", "trb.std.test.test":
		return "", true
	default:
		return "", false
	}
}

func (g *generator) testCallBlock(call *ir.Call) bool {
	reference := expressionReference(call.Callee)
	if reference == nil || (reference.Intrinsic != "trb.std.test.describe" && reference.Intrinsic != "trb.std.test.test") {
		return false
	}
	name := g.expr(call.Arguments[0].Value)
	function := "TrbTestDescribe"
	arguments := name
	if reference.Intrinsic == "trb.std.test.test" {
		position := call.SourceSpan().Start
		function = "TrbTest"
		arguments += ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column)
	}
	g.line(g.testAlias(call) + "." + function + "(" + arguments + ", func() {")
	g.indent++
	g.statements(call.Block.Body)
	g.indent--
	g.line("})")
	return true
}

func (g *generator) testRuntimeSupport() {
	g.requireImport("encoding/json", "json")
	g.requireImport("fmt", "fmt")
	g.requireImport("os", "os")
	g.requireImport("reflect", "reflect")
	g.requireImport("strings", "strings")
	g.line(`type trbTestFailure struct { Path string; Line int; Column int; Message string }`)
	g.line(`func (failure trbTestFailure) Error() string { return failure.Message }`)
	g.line(`var trbTestSuites []string`)
	g.line(`var trbTestTotal int`)
	g.line(`var trbTestFailed int`)
	g.line(`func trbTestEvent(kind string, name string, testPath string, path string, line int, column int, message string) {`)
	g.indent++
	g.line(`if os.Getenv("TRB_TEST_REPORTER") == "json" {`)
	g.indent++
	g.line(`event := map[string]any{"type": kind, "name": name, "test_file": testPath, "file": path, "line": line, "column": column}`)
	g.line(`if message != "" { event["message"] = message }`)
	g.line(`encoded, _ := json.Marshal(event)`)
	g.line(`fmt.Println(string(encoded))`)
	g.line(`return`)
	g.indent--
	g.line(`}`)
	g.line(`if kind == "test_passed" { fmt.Println("PASS " + name) }`)
	g.line(`if kind == "test_failed" { fmt.Printf("FAIL %s\n  %s:%d:%d: %s\n", name, path, line, column, message) }`)
	g.indent--
	g.line(`}`)
	g.line(`func TrbTestDescribe(name string, body func()) { trbTestSuites = append(trbTestSuites, name); defer func() { trbTestSuites = trbTestSuites[:len(trbTestSuites)-1] }(); body() }`)
	g.line(`func TrbTest(name string, path string, line int, column int, body func()) {`)
	g.indent++
	g.line(`fullName := strings.Join(append(append([]string{}, trbTestSuites...), name), " / ")`)
	g.line(`if selected := os.Getenv("TRB_TEST_FILE"); selected != "" && selected != path { return }`)
	g.line(`if filter := os.Getenv("TRB_TEST_FILTER"); filter != "" && !strings.Contains(fullName, filter) { return }`)
	g.line(`trbTestTotal++`)
	g.line(`trbTestEvent("test_started", fullName, path, path, line, column, "")`)
	g.line(`failure := ""`)
	g.line(`failurePath, failureLine, failureColumn := path, line, column`)
	g.line(`func() { defer func() { if recovered := recover(); recovered != nil { if assertion, ok := recovered.(trbTestFailure); ok { failure = assertion.Message; failurePath, failureLine, failureColumn = assertion.Path, assertion.Line, assertion.Column } else { failure = fmt.Sprint(recovered) } } }(); body() }()`)
	g.line(`if failure == "" { trbTestEvent("test_passed", fullName, path, path, line, column, ""); return }`)
	g.line(`trbTestFailed++`)
	g.line(`trbTestEvent("test_failed", fullName, path, failurePath, failureLine, failureColumn, failure)`)
	g.indent--
	g.line(`}`)
	g.line(`func TrbTestFinish() {`)
	g.indent++
	g.line(`if os.Getenv("TRB_TEST_REPORTER") == "json" { encoded, _ := json.Marshal(map[string]any{"type": "test_summary", "total": trbTestTotal, "failed": trbTestFailed}); fmt.Println(string(encoded)) } else { fmt.Printf("\n%d test(s), %d failure(s)\n", trbTestTotal, trbTestFailed) }`)
	g.line(`if trbTestFailed > 0 { os.Exit(1) }`)
	g.indent--
	g.line(`}`)
	g.line(`func trbTestAssertEqual(actual any, expected any, path string, line int, column int) { if !reflect.DeepEqual(actual, expected) { panic(trbTestFailure{path, line, column, fmt.Sprintf("expected %#v to equal %#v", actual, expected)}) } }`)
	g.line(`func trbTestAssertNotEqual(actual any, expected any, path string, line int, column int) { if reflect.DeepEqual(actual, expected) { panic(trbTestFailure{path, line, column, fmt.Sprintf("expected %#v not to equal %#v", actual, expected)}) } }`)
	g.line(`func trbTestAssertTrue(actual any, path string, line int, column int) { if value, ok := actual.(bool); !ok || !value { panic(trbTestFailure{path, line, column, fmt.Sprintf("expected %#v to be true", actual)}) } }`)
	g.line(`func trbTestAssertFalse(actual any, path string, line int, column int) { if value, ok := actual.(bool); !ok || value { panic(trbTestFailure{path, line, column, fmt.Sprintf("expected %#v to be false", actual)}) } }`)
	g.line(`func trbTestAssertNil(actual any, path string, line int, column int) { if !reflect.ValueOf(&actual).Elem().IsNil() { panic(trbTestFailure{path, line, column, fmt.Sprintf("expected %#v to be nil", actual)}) } }`)
}
