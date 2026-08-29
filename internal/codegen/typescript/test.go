package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

func (g *generator) testAlias(call *ir.Call) string {
	if reference := expressionReference(call.Callee); reference != nil && reference.Alias != "" {
		return reference.Alias
	}
	return "__trb_test"
}

func (g *generator) testIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	switch name {
	case "trb.std.test.expect":
		position := call.SourceSpan().Start
		return "new " + g.testAlias(call) + ".Expectation(" + arguments[0] + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")", true
	case "trb.std.test.expect_ok", "trb.std.test.expect_err":
		position := call.SourceSpan().Start
		method := "ok"
		if name == "trb.std.test.expect_err" {
			method = "err"
		}
		return "new " + g.testAlias(call) + ".ResultExpectation(" + arguments[0] + ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column) + ")." + method + "()", true
	case "trb.std.test.finish":
		return g.testAlias(call) + ".trbTestFinish()", true
	case "trb.internal.test.assert_equal", "trb.internal.test.assert_not_equal", "trb.internal.test.assert_true", "trb.internal.test.assert_false", "trb.internal.test.assert_nil":
		return "trbTest" + upperCamel(strings.TrimPrefix(name, "trb.internal.test.")) + "(" + strings.Join(arguments, ", ") + ")", true
	case "trb.internal.test.assert_result_ok":
		return "((): " + g.tsType(call.ExprType()) + " => { const actual = " + arguments[0] + "; if (actual.kind !== \"Ok\") throw new TrbTestFailure(" + arguments[1] + ", " + arguments[2] + ", " + arguments[3] + ", \"expected Ok, got Err(\" + trbTestInspect(actual.error) + \")\"); return actual.value; })()", true
	case "trb.internal.test.assert_result_err":
		return "((): " + g.tsType(call.ExprType()) + " => { const actual = " + arguments[0] + "; if (actual.kind !== \"Err\") throw new TrbTestFailure(" + arguments[1] + ", " + arguments[2] + ", " + arguments[3] + ", \"expected Err, got Ok(\" + trbTestInspect(actual.value) + \")\"); return actual.error; })()", true
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
	function := "trbTestDescribe"
	arguments := name
	if reference.Intrinsic == "trb.std.test.test" {
		position := call.SourceSpan().Start
		function = "trbTest"
		arguments += ", " + strconv.Quote(g.sourcePath) + ", " + strconv.Itoa(position.Line) + ", " + strconv.Itoa(position.Column)
	}
	g.line("await " + g.testAlias(call) + "." + function + "(" + arguments + ", async () => {")
	g.indent++
	g.statements(call.Block.Body)
	g.indent--
	g.line("});")
	return true
}

func upperCamel(value string) string {
	parts := strings.Split(value, "_")
	for index := range parts {
		if parts[index] != "" {
			parts[index] = strings.ToUpper(parts[index][:1]) + parts[index][1:]
		}
	}
	return strings.Join(parts, "")
}

func (g *generator) testRuntimeSupport() {
	g.line(`class TrbTestFailure extends Error { constructor(readonly path: string, readonly line: number, readonly column: number, message: string) { super(message); } }`)
	g.line(`const trbTestSuites: string[] = [];`)
	g.line(`let trbTestTotal = 0;`)
	g.line(`let trbTestFailed = 0;`)
	g.line(`const trbTestEnv = (): Record<string, string | undefined> => ((globalThis as any).process?.env ?? {});`)
	g.line(`const trbTestNamesJSON = trbTestEnv().TRB_TEST_NAMES;`)
	g.line(`const trbTestNames: string[] = trbTestNamesJSON ? JSON.parse(trbTestNamesJSON) : [];`)
	g.line(`const trbTestEqual = (left: any, right: any): boolean => { if (left === right) return true; if (typeof left !== "object" || left === null || typeof right !== "object" || right === null) return false; if (Array.isArray(left) !== Array.isArray(right)) return false; const leftKeys = Object.keys(left).sort(); const rightKeys = Object.keys(right).sort(); return leftKeys.length === rightKeys.length && leftKeys.every((key, index) => key === rightKeys[index] && trbTestEqual(left[key], right[key])); };`)
	g.line(`const trbTestInspect = (value: any): string => { try { return JSON.stringify(value); } catch { return String(value); } };`)
	g.line(`const trbTestEvent = (type: string, name: string, test_file: string, file: string, line: number, column: number, message = ""): void => { const event: any = { type, name, test_file, file, line, column }; if (message) event.message = message; if (trbTestEnv().TRB_TEST_REPORTER === "json") { console.log(JSON.stringify(event)); return; } if (type === "test_passed") console.log("PASS " + name); if (type === "test_failed") console.log("FAIL " + name + "\n  " + file + ":" + line + ":" + column + ": " + message); };`)
	g.line(`export async function trbTestDescribe(name: string, body: () => Promise<void>): Promise<void> { trbTestSuites.push(name); try { await body(); } finally { trbTestSuites.pop(); } }`)
	g.line(`export async function trbTest(name: string, file: string, line: number, column: number, body: () => Promise<void>): Promise<void> { const fullName = [...trbTestSuites, name].join(" / "); const environment = trbTestEnv(); if (environment.TRB_TEST_FILE && environment.TRB_TEST_FILE !== file) return; if (trbTestNames.length > 0 && !trbTestNames.includes(fullName)) return; trbTestTotal++; trbTestEvent("test_started", fullName, file, file, line, column); try { await body(); trbTestEvent("test_passed", fullName, file, file, line, column); } catch (error) { trbTestFailed++; const failure = error instanceof TrbTestFailure ? error : undefined; trbTestEvent("test_failed", fullName, file, failure?.path ?? file, failure?.line ?? line, failure?.column ?? column, error instanceof Error ? error.message : String(error)); } }`)
	g.line(`export function trbTestFinish(): void { if (trbTestEnv().TRB_TEST_REPORTER === "json") console.log(JSON.stringify({ type: "test_summary", total: trbTestTotal, failed: trbTestFailed })); else console.log("\n" + trbTestTotal + " test(s), " + trbTestFailed + " failure(s)"); if (trbTestFailed > 0 && (globalThis as any).process) (globalThis as any).process.exitCode = 1; }`)
	g.line(`function trbTestAssertEqual(actual: any, expected: any, path: string, line: number, column: number): void { if (!trbTestEqual(actual, expected)) throw new TrbTestFailure(path, line, column, "expected " + trbTestInspect(actual) + " to equal " + trbTestInspect(expected)); }`)
	g.line(`function trbTestAssertNotEqual(actual: any, expected: any, path: string, line: number, column: number): void { if (trbTestEqual(actual, expected)) throw new TrbTestFailure(path, line, column, "expected " + trbTestInspect(actual) + " not to equal " + trbTestInspect(expected)); }`)
	g.line(`function trbTestAssertTrue(actual: any, path: string, line: number, column: number): void { if (actual !== true) throw new TrbTestFailure(path, line, column, "expected " + trbTestInspect(actual) + " to be true"); }`)
	g.line(`function trbTestAssertFalse(actual: any, path: string, line: number, column: number): void { if (actual !== false) throw new TrbTestFailure(path, line, column, "expected " + trbTestInspect(actual) + " to be false"); }`)
	g.line(`function trbTestAssertNil(actual: any, path: string, line: number, column: number): void { if (actual !== null && actual !== undefined) throw new TrbTestFailure(path, line, column, "expected " + trbTestInspect(actual) + " to be nil"); }`)
}
