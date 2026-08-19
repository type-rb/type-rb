package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/codegen/effectplan"
	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	if generated, ok := g.testIntrinsic(name, call, arguments); ok {
		return generated
	}
	if generated, ok := g.oidcIntrinsic(name, call, arguments); ok {
		return generated
	}
	if generated, ok := g.browserHTTPIntrinsic(name, call, arguments); ok {
		return generated
	}
	if value, ok := g.timeIntrinsic(name, call, arguments); ok {
		return value
	}
	if generated, ok := g.ormIntrinsic(name, call, arguments); ok {
		if effectplan.ORMOperation(name) {
			return "__trbOrm.withScope(__trbScope, async () => " + generated + ")"
		}
		return generated
	}
	if name == "trb.internal.runtime.fail" {
		return "((): " + g.tsType(call.ExprType()) + " => { throw new Error(" + arguments[0] + "); })()"
	}
	if name == "trb.jobs.perform_later" || name == "trb.jobs.perform_in" || name == "trb.jobs.perform_at" {
		return g.jobsPerformLater(call, arguments)
	}
	unicodeAlias := "unicode"
	pathAlias := "path"
	reference := expressionReference(call.Callee)
	if reference != nil && reference.Alias != "" {
		unicodeAlias = reference.Alias
		pathAlias = reference.Alias
	}
	unicodeCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return unicodeAlias + ".Unicode." + symbol
	}
	pathCall := func(symbol string) string {
		if _, named := call.Callee.(*ir.Identifier); named {
			return symbol
		}
		return pathAlias + "." + symbol
	}
	filesystemResultType := func() (string, string, string) {
		result := call.ExprType()
		if len(result.Args) != 2 {
			return g.tsType(result), "unknown", "unknown"
		}
		return g.tsType(result), g.tsType(result.Args[0]), g.tsType(result.Args[1])
	}
	filesystemOK := func(value string) string {
		_, successType, errorType := filesystemResultType()
		return g.runtimeName("Result") + ".Ok<" + successType + ", " + errorType + ">(" + value + ")"
	}
	filesystemError := func(operation, path, message string) string {
		_, successType, errorType := filesystemResultType()
		value := "{ operation: " + strconv.Quote(operation) + ", path: " + path + ", message: " + message + " } satisfies " + errorType
		return g.runtimeName("Result") + ".Err<" + successType + ", " + errorType + ">(" + value + ")"
	}
	processError := func(operation, command, message string) string {
		_, successType, errorType := filesystemResultType()
		value := "{ operation: " + strconv.Quote(operation) + ", command: " + command + ", message: " + message + " } satisfies " + errorType
		return g.runtimeName("Result") + ".Err<" + successType + ", " + errorType + ">(" + value + ")"
	}
	resultError := func(value string) string {
		_, successType, errorType := filesystemResultType()
		return g.runtimeName("Result") + ".Err<" + successType + ", " + errorType + ">(" + value + ")"
	}
	numberParseError := func(kind, input, message string) string {
		value := "({ kind: " + g.runtimeName("NumberParseErrorKind") + "." + kind + ", input: " + input + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("NumberParseError") + ")"
		return resultError(value)
	}
	hexDecodeError := func(kind, input, index, message string) string {
		value := "({ kind: " + g.runtimeName("HexDecodeErrorKind") + "." + kind + ", input: " + input + ", index: " + index + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("HexDecodeError") + ")"
		return resultError(value)
	}
	base64DecodeError := func(kind, input, index, message string) string {
		value := "({ kind: " + g.runtimeName("Base64DecodeErrorKind") + "." + kind + ", input: " + input + ", index: " + index + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("Base64DecodeError") + ")"
		return resultError(value)
	}
	percentDecodeError := func(kind, input, message string) string {
		value := "({ kind: " + g.runtimeName("PercentDecodeErrorKind") + "." + kind + ", input: " + input + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("PercentDecodeError") + ")"
		return resultError(value)
	}
	indexLookupError := func(index, size, message string) string {
		value := "({ index: " + index + ", size: " + size + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("IndexLookupError") + ")"
		return resultError(value)
	}
	sliceRangeError := func(start, end, exclusive, size, message string) string {
		value := "({ start: " + start + ", finish: " + end + ", exclusive: " + exclusive + ", size: " + size + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("SliceRangeError") + ")"
		return resultError(value)
	}
	keyLookupError := func(key, message string) string {
		value := "({ key: " + key + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("KeyLookupError") + ")"
		return resultError(value)
	}
	filesystemHandle := `const fs = (globalThis as any).process?.getBuiltinModule?.("fs"); if (fs === undefined) { throw new Error("filesystem is unavailable"); } `
	filesystemMessage := `const message = error instanceof Error ? error.message : String(error); `
	switch name {
	case "trb.std.io.puts":
		if len(call.Arguments) == 1 && call.Arguments[0].Value.ExprType().Kind == types.Array {
			return "console.log(" + portableArrayString(arguments[0], call.Arguments[0].Value.ExprType()) + ")"
		}
		if len(call.Arguments) == 1 && call.Arguments[0].Value.ExprType().Kind == types.Float {
			return "console.log(" + portableFloatString(arguments[0]) + ")"
		}
		return "console.log(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.path.separator":
		return pathCall("separator") + "()"
	case "trb.std.path.clean":
		return pathCall("clean") + "(" + arguments[0] + ")"
	case "trb.std.path.join":
		return pathCall("join") + "(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.path.absolute":
		return pathCall("absolute") + "(" + arguments[0] + ")"
	case "trb.std.path.components":
		return pathCall("components") + "(" + arguments[0] + ")"
	case "trb.std.path.base":
		return pathCall("base") + "(" + arguments[0] + ")"
	case "trb.std.path.directory":
		return pathCall("directory") + "(" + arguments[0] + ")"
	case "trb.std.url.encode_component":
		return "((value: string): string => { const bytes = new TextEncoder().encode(value); let encoded = \"\"; for (const byte of bytes) { const unreserved = byte >= 65 && byte <= 90 || byte >= 97 && byte <= 122 || byte >= 48 && byte <= 57 || byte === 45 || byte === 46 || byte === 95 || byte === 126; encoded += unreserved ? String.fromCharCode(byte) : \"%\" + byte.toString(16).toUpperCase().padStart(2, \"0\"); } return encoded; })(" + arguments[0] + ")"
	case "trb.std.url.decode_component":
		resultType, _, _ := filesystemResultType()
		invalidEscape := percentDecodeError("InvalidEscape", "input", "invalid percent escape in URL component")
		invalidUtf8 := percentDecodeError("InvalidUtf8", "input", "decoded URL component is not valid UTF-8")
		return "((): " + resultType + " => { const input = " + arguments[0] + "; const characters = Array.from(input); const bytes: Array<number> = []; const encoder = new TextEncoder(); for (let index = 0; index < characters.length; index += 1) { const character = characters[index]!; if (character !== \"%\") { bytes.push(...encoder.encode(character)); continue; } if (index + 2 >= characters.length || !/^[0-9A-Fa-f]$/.test(characters[index + 1]!) || !/^[0-9A-Fa-f]$/.test(characters[index + 2]!)) { return " + invalidEscape + "; } bytes.push(Number.parseInt(characters[index + 1]! + characters[index + 2]!, 16)); index += 2; } try { const value = new TextDecoder(\"utf-8\", { fatal: true }).decode(Uint8Array.from(bytes)); return " + filesystemOK("value") + "; } catch { return " + invalidUtf8 + "; } })()"
	case "trb.internal.filesystem.exists":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "fs.statSync(__trbPath); return " + filesystemOK("true") + "; } catch (error) { if ((error as any)?.code === \"ENOENT\") { return " + filesystemOK("false") + "; } " + filesystemMessage + "return " + filesystemError("exists", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.read_text":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "const data: Uint8Array = fs.readFileSync(__trbPath); return " + filesystemOK("new TextDecoder(\"utf-8\").decode(data)") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("read_text", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.read_bytes":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "return " + filesystemOK("new Uint8Array(fs.readFileSync(__trbPath))") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("read_bytes", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.write_text":
		resultType, successType, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "fs.writeFileSync(__trbPath, " + arguments[1] + ", { encoding: \"utf8\" }); return " + filesystemOK("({} satisfies "+successType+")") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("write_text", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.write_bytes":
		resultType, successType, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "fs.writeFileSync(__trbPath, " + arguments[1] + "); return " + filesystemOK("({} satisfies "+successType+")") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("write_bytes", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.create_directory":
		resultType, successType, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "fs.mkdirSync(__trbPath, { recursive: true }); return " + filesystemOK("({} satisfies "+successType+")") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("create_directory", "__trbPath", "message") + "; } })()"
	case "trb.internal.filesystem.list":
		resultType, _, _ := filesystemResultType()
		compare := "(left, right) => { const leftBytes = new TextEncoder().encode(left); const rightBytes = new TextEncoder().encode(right); const length = Math.min(leftBytes.length, rightBytes.length); for (let index = 0; index < length; index += 1) { if (leftBytes[index] !== rightBytes[index]) { return leftBytes[index]! - rightBytes[index]!; } } return leftBytes.length - rightBytes.length; }"
		return "((): " + resultType + " => { const __trbPath = " + arguments[0] + "; try { " + filesystemHandle + "const names = (fs.readdirSync(__trbPath) as Array<string>).sort(" + compare + "); return " + filesystemOK("names") + "; } catch (error) { " + filesystemMessage + "return " + filesystemError("list", "__trbPath", "message") + "; } })()"
	case "trb.internal.process.arguments":
		return `(Reflect.get(globalThis, "process")?.argv ?? []).slice(2)`
	case "trb.internal.process.environment":
		return "Reflect.get(globalThis, \"process\")?.env?.[" + arguments[0] + "] ?? null"
	case "trb.internal.process.working_directory":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { try { const host = Reflect.get(globalThis, \"process\"); if (host?.cwd === undefined) { throw new Error(\"process working directory is unavailable\"); } return " + filesystemOK("host.cwd()") + "; } catch (error) { " + filesystemMessage + "return " + processError("working_directory", strconv.Quote(""), "message") + "; } })()"
	case "trb.internal.process.run":
		resultType, successType, _ := filesystemResultType()
		value := "{ status, stdout: decode(output.stdout), stderr: decode(output.stderr), success: status === 0 } satisfies " + successType
		return "((): " + resultType + " => { const __trbCommand = " + arguments[0] + "; const __trbArguments = " + arguments[1] + "; try { const host = Reflect.get(globalThis, \"process\"); const childProcess = host?.getBuiltinModule?.(\"child_process\"); if (childProcess === undefined) { throw new Error(\"process execution is unavailable\"); } const output = childProcess.spawnSync(__trbCommand, __trbArguments); if (output.error !== undefined) { throw output.error; } const status = typeof output.status === \"number\" ? output.status : -1; const decode = (value: Uint8Array | undefined): string => new TextDecoder(\"utf-8\").decode(value ?? new Uint8Array()); return " + filesystemOK(value) + "; } catch (error) { " + filesystemMessage + "return " + processError("run", "__trbCommand", "message") + "; } })()"
	case "trb.internal.json.parse":
		if runtimeCall, ok := g.importedJSONCall(call, "trb/std/json/index", arguments); ok {
			return runtimeCall
		}
		return tsJSONParse(call, arguments[0], false)
	case "trb.internal.json.parse_jsonc":
		if runtimeCall, ok := g.importedJSONCall(call, "trb/std/jsonc/index", arguments); ok {
			return runtimeCall
		}
		return tsJSONParse(call, arguments[0], true)
	case "trb.internal.json.stringify":
		if runtimeCall, ok := g.importedJSONCall(call, "trb/std/json/index", arguments); ok {
			return runtimeCall
		}
		return tsJSONStringify(call, arguments[0])
	case "trb.internal.json.decode":
		return g.tsJSONDecode(call, arguments[0])
	case "trb.internal.json.encode":
		return g.tsJSONEncode(call, arguments[0])
	case "trb.web.request_json":
		return g.tsWebRequestJSON(call, arguments[0])
	case "trb.web.request_query":
		return g.tsWebParameterBinding(call, arguments[0], "query")
	case "trb.web.context_params":
		return g.tsWebParameterBinding(call, arguments[0], "path")
	case "trb.web.context_bind":
		return g.tsWebEndpointInput(call, arguments[0])
	case "trb.web.context_with":
		return tsWebContextWith(call, arguments)
	case "trb.web.context_with_request":
		return tsWebContextWithRequest(call, arguments)
	case "trb.web.context_fetch":
		return tsWebContextFetch(call, arguments)
	case "trb.web.json":
		return g.tsWebJSON(call, arguments)
	case "trb.web.configure_server":
		values := map[string]string{
			"host":                          `"0.0.0.0"`,
			"port":                          "3000",
			"body_limit_bytes":              "1048576",
			"shutdown_timeout_milliseconds": "10000",
		}
		for index, argument := range call.Arguments {
			values[argument.Name] = g.expr(call.Arguments[index].Value)
		}
		return "{ host: " + values["host"] + ", port: " + values["port"] + ", body_limit_bytes: " + values["body_limit_bytes"] + ", shutdown_timeout_milliseconds: " + values["shutdown_timeout_milliseconds"] + " }"
	case "trb.web.serve":
		config := `{ host: "0.0.0.0", port: 3000, body_limit_bytes: 1048576, shutdown_timeout_milliseconds: 10000 }`
		if len(arguments) > 0 {
			config = arguments[0]
		}
		return "trb_web_serve(" + config + ")"
	case "trb.web.testing.dispatch":
		return "trb_web_dispatch(__trbScope, " + arguments[0] + ")"
	case "trb.web.middleware.logger.call":
		return tsWebLogger(call, arguments)
	case "trb.web.middleware.timeout.call":
		return tsWebTimeout(call, arguments)
	case "trb.web.middleware.compression.gzip":
		return tsWebGzip(arguments[0])
	case "trb.std.strings.length":
		return "Array.from(" + arguments[0] + ").length"
	case "trb.std.strings.empty":
		return arguments[0] + ".length === 0"
	case "trb.std.strings.strip", "trb.std.strings.lstrip", "trb.std.strings.rstrip":
		whitespace := `[\u0009-\u000d\u0020\u0085\u00a0\u1680\u2000-\u200a\u2028-\u2029\u202f\u205f\u3000]`
		value := "(" + arguments[0] + ")"
		if name != "trb.std.strings.rstrip" {
			value += `.replace(/^` + whitespace + `+/u, "")`
		}
		if name != "trb.std.strings.lstrip" {
			value += `.replace(/` + whitespace + `+$/u, "")`
		}
		return value
	case "trb.std.strings.uppercase":
		return arguments[0] + ".toUpperCase()"
	case "trb.std.strings.lowercase":
		return arguments[0] + ".toLowerCase()"
	case "trb.std.strings.starts_with":
		return arguments[0] + ".startsWith(" + arguments[1] + ")"
	case "trb.std.strings.ends_with":
		return arguments[0] + ".endsWith(" + arguments[1] + ")"
	case "trb.std.strings.split":
		return "((value: string, separator: string): Array<string> => { if (separator === \"\") { throw new Error(\"String split separator is empty\"); } return value.split(separator); })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.strings.contains":
		return arguments[0] + ".includes(" + arguments[1] + ")"
	case "trb.std.strings.replace_all":
		return "((value: string, pattern: string, replacement: string): string => { if (pattern === \"\") { throw new Error(\"String replacement pattern is empty\"); } return value.split(pattern).join(replacement); })(" + arguments[0] + ", " + arguments[1] + ", " + arguments[2] + ")"
	case "trb.std.strings.codepoints":
		return "Array.from(" + arguments[0] + ", (value): number => value.codePointAt(0)!)"
	case "trb.std.strings.characters":
		return "Array.from(" + arguments[0] + ")"
	case "trb.std.strings.reverse":
		return "Array.from(" + arguments[0] + ").reverse().join(\"\")"
	case "trb.std.strings.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbValue = Array.from(" + arguments[0] + "); const __trbRequested = " + arguments[1] + "; let __trbIndex = __trbRequested; if (__trbIndex < 0) __trbIndex += __trbValue.length; if (__trbIndex < 0 || __trbIndex >= __trbValue.length) { return " + indexLookupError("__trbRequested", "__trbValue.length", "String index is out of bounds") + "; } return " + filesystemOK("__trbValue[__trbIndex]!") + "; })()"
	case "trb.std.strings.slice", "trb.std.strings.try_slice":
		safe := name == "trb.std.strings.try_slice"
		returnType := "string"
		invalid := "throw new RangeError(\"String slice range is out of bounds\");"
		success := "return characters.slice(start, stop).join(\"\");"
		if safe {
			returnType, _, _ = filesystemResultType()
			invalid = "return " + sliceRangeError("start", "end", "exclusive", "characters.length", "String slice range is out of bounds") + ";"
			success = "return " + filesystemOK("characters.slice(start, stop).join(\"\")") + ";"
		}
		return "((): " + returnType + " => { const characters = Array.from(" + arguments[0] + "); const [start, end, exclusive] = " + arguments[1] + "; const valid = start >= 0 && end >= 0 && start <= end && (exclusive ? end <= characters.length : end < characters.length); if (!valid) { " + invalid + " } const stop = exclusive ? end : end + 1; " + success + " })()"
	case "trb.std.strings.index", "trb.std.strings.rindex":
		reverse := name == "trb.std.strings.rindex"
		return "((value: string, substring: string): number | null => { const characters = Array.from(value); const needle = Array.from(substring); if (needle.length === 0) return " + map[bool]string{false: "0", true: "characters.length"}[reverse] + "; if (needle.length > characters.length) return null; let index = " + map[bool]string{false: "0", true: "characters.length - needle.length"}[reverse] + "; const stop = " + map[bool]string{false: "characters.length - needle.length", true: "0"}[reverse] + "; const step = " + map[bool]string{false: "1", true: "-1"}[reverse] + "; for (;; index += step) { if (needle.every((character, offset) => characters[index + offset] === character)) return index; if (index === stop) break; } return null; })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.unicode.version":
		return unicodeCall("version") + "()"
	case "trb.std.unicode.valid_scalar":
		return unicodeCall("valid_scalar") + "(" + arguments[0] + ")"
	case "trb.std.unicode.letter":
		return unicodeCall("letter") + "(" + arguments[0] + ")"
	case "trb.std.unicode.digit":
		return unicodeCall("digit") + "(" + arguments[0] + ")"
	case "trb.std.unicode.uppercase":
		return unicodeCall("uppercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.lowercase":
		return unicodeCall("lowercase") + "(" + arguments[0] + ")"
	case "trb.std.unicode.whitespace":
		return unicodeCall("whitespace") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_start":
		return unicodeCall("identifier_start") + "(" + arguments[0] + ")"
	case "trb.std.unicode.identifier_part":
		return unicodeCall("identifier_part") + "(" + arguments[0] + ")"
	case "trb.std.unicode.from_codepoint":
		return unicodeCall("from_codepoint") + "(" + arguments[0] + ")"
	case "trb.std.bytes.from_string":
		return "new TextEncoder().encode(" + arguments[0] + ")"
	case "trb.std.bytes.to_string":
		return "new TextDecoder(\"utf-8\").decode(" + arguments[0] + ")"
	case "trb.std.bytes.length":
		return arguments[0] + ".byteLength"
	case "trb.std.bytes.at":
		return "((value: Uint8Array, index: number): number => { if (index < 0 || index >= value.byteLength) { throw new Error(\"Bytes index is out of bounds\"); } return value[index]!; })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.bytes.concat":
		return "((left: Uint8Array, right: Uint8Array): Uint8Array => { const value = new Uint8Array(left.byteLength + right.byteLength); value.set(left); value.set(right, left.byteLength); return value; })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.bytes.valid_utf8":
		return "((value: Uint8Array): boolean => { try { new TextDecoder(\"utf-8\", { fatal: true }).decode(value); return true; } catch { return false; } })(" + arguments[0] + ")"
	case "trb.std.encoding.hex.encode":
		return "Array.from(" + arguments[0] + ", (value) => value.toString(16).padStart(2, \"0\")).join(\"\")"
	case "trb.std.encoding.hex.decode":
		resultType, _, _ := filesystemResultType()
		invalid := hexDecodeError("InvalidCharacter", "__trbInput", "__trbInvalidIndex", "invalid hexadecimal character")
		odd := hexDecodeError("OddLength", "__trbInput", "__trbCharacters.length", "hex input has odd length")
		return "((): " + resultType + " => { const __trbInput = " + arguments[0] + "; const __trbCharacters = Array.from(__trbInput); const __trbInvalidIndex = __trbCharacters.findIndex((character) => !/^[0-9A-Fa-f]$/.test(character)); if (__trbInvalidIndex >= 0) { return " + invalid + "; } if (__trbCharacters.length % 2 !== 0) { return " + odd + "; } const __trbValue = new Uint8Array(__trbCharacters.length / 2); for (let index = 0; index < __trbCharacters.length; index += 2) { __trbValue[index / 2] = Number.parseInt(__trbCharacters[index]! + __trbCharacters[index + 1]!, 16); } return " + filesystemOK("__trbValue") + "; })()"
	case "trb.std.encoding.base64.encode":
		return "((value: Uint8Array): string => { let binary = \"\"; for (const byte of value) { binary += String.fromCharCode(byte); } return btoa(binary); })(" + arguments[0] + ")"
	case "trb.std.encoding.base64.url_encode":
		return "((value: Uint8Array): string => { let binary = \"\"; for (const byte of value) { binary += String.fromCharCode(byte); } return btoa(binary).replace(/\\+/g, \"-\").replace(/\\//g, \"_\").replace(/=+$/, \"\"); })(" + arguments[0] + ")"
	case "trb.std.encoding.base64.decode":
		resultType, _, _ := filesystemResultType()
		lengthError := base64DecodeError("InvalidLength", "__trbInput", "__trbCharacters.length", "base64 input length must be a multiple of 4")
		paddingError := base64DecodeError("InvalidPadding", "__trbInput", "index", "invalid base64 padding")
		characterError := base64DecodeError("InvalidCharacter", "__trbInput", "index", "invalid base64 character")
		nonCanonical := base64DecodeError("NonCanonical", "__trbInput", "__trbCharacters.length - padding - 1", "non-canonical base64 encoding")
		return "((): " + resultType + " => { const __trbInput = " + arguments[0] + "; const __trbCharacters = Array.from(__trbInput); if (__trbCharacters.length % 4 !== 0) { return " + lengthError + "; } let padding = 0; for (let index = 0; index < __trbCharacters.length; index += 1) { const character = __trbCharacters[index]!; if (character === \"=\") { padding += 1; if (index < __trbCharacters.length - 2 || padding > 2) { return " + paddingError + "; } continue; } if (padding > 0) { return " + paddingError + "; } if (!/^[A-Za-z0-9+/]$/.test(character)) { return " + characterError + "; } } const binary = atob(__trbInput); if (btoa(binary) !== __trbInput) { return " + nonCanonical + "; } return " + filesystemOK("Uint8Array.from(binary, (character) => character.charCodeAt(0))") + "; })()"
	case "trb.std.encoding.base64.url_decode":
		resultType, _, _ := filesystemResultType()
		lengthError := base64DecodeError("InvalidLength", "__trbInput", "__trbCharacters.length", "base64url input has invalid length")
		paddingError := base64DecodeError("InvalidPadding", "__trbInput", "index", "base64url input must not contain padding")
		characterError := base64DecodeError("InvalidCharacter", "__trbInput", "index", "invalid base64url character")
		nonCanonical := base64DecodeError("NonCanonical", "__trbInput", "__trbCharacters.length - 1", "non-canonical base64url encoding")
		return "((): " + resultType + " => { const __trbInput = " + arguments[0] + "; const __trbCharacters = Array.from(__trbInput); if (__trbCharacters.length % 4 === 1) { return " + lengthError + "; } for (let index = 0; index < __trbCharacters.length; index += 1) { const character = __trbCharacters[index]!; if (character === \"=\") { return " + paddingError + "; } if (!/^[A-Za-z0-9_-]$/.test(character)) { return " + characterError + "; } } const padded = __trbInput.replace(/-/g, \"+\").replace(/_/g, \"/\") + \"=\".repeat((4 - __trbInput.length % 4) % 4); const binary = atob(padded); const canonical = btoa(binary).replace(/\\+/g, \"-\").replace(/\\//g, \"_\").replace(/=+$/, \"\"); if (canonical !== __trbInput) { return " + nonCanonical + "; } return " + filesystemOK("Uint8Array.from(binary, (character) => character.charCodeAt(0))") + "; })()"
	case "trb.std.hash.md5":
		return md5Expression(arguments[0])
	case "trb.std.hash.sha1":
		return sha1Expression(arguments[0])
	case "trb.std.hash.sha256":
		return sha256Expression(arguments[0])
	case "trb.std.hash.sha512":
		return sha512Expression(arguments[0])
	case "trb.std.hmac.sha256":
		return hmacExpression(arguments[0], arguments[1], 64, sha256Function())
	case "trb.std.hmac.sha512":
		return hmacExpression(arguments[0], arguments[1], 128, sha512Function())
	case "trb.std.hmac.equal", "trb.std.secure_compare.equal":
		return "((left: Uint8Array, right: Uint8Array): boolean => { if (left.byteLength !== right.byteLength) { return false; } let difference = 0; for (let index = 0; index < left.byteLength; index += 1) { difference |= left[index]! ^ right[index]!; } return difference === 0; })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.random.float":
		return "Math.random()"
	case "trb.std.random.integer":
		return "((upper: number): number => { if (upper <= 0) { throw new RangeError(\"random.integer upper bound must be greater than zero\"); } return Math.floor(Math.random() * upper); })(" + arguments[0] + ")"
	case "trb.std.secure_random.bytes":
		return "((length: number): Uint8Array => { if (length < 0 || length > 65536) { throw new RangeError(\"secure_random.bytes length must be between 0 and 65536\"); } if (!globalThis.crypto) { throw new Error(\"secure random source is unavailable\"); } return globalThis.crypto.getRandomValues(new Uint8Array(length)); })(" + arguments[0] + ")"
	case "trb.std.string_builder.new":
		return "[]"
	case "trb.std.string_builder.from_string":
		return "[" + arguments[0] + "]"
	case "trb.std.string_builder.append":
		return arguments[0] + ".push(" + arguments[1] + ")"
	case "trb.std.string_builder.append_codepoint":
		return "((builder: Array<string>, value: number): void => { if (value < 0 || value > 0x10ffff || (value >= 0xd800 && value <= 0xdfff)) { throw new RangeError(\"invalid Unicode code point\"); } builder.push(String.fromCodePoint(value)); })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.string_builder.length":
		return "Array.from(" + arguments[0] + ".join(\"\")).length"
	case "trb.std.string_builder.empty":
		return arguments[0] + ".length === 0"
	case "trb.std.string_builder.to_string":
		return arguments[0] + ".join(\"\")"
	case "trb.std.string_builder.clear":
		return arguments[0] + ".splice(0)"
	case "trb.std.arrays.length":
		return arguments[0] + ".length"
	case "trb.std.arrays.empty":
		return arguments[0] + ".length === 0"
	case "trb.std.arrays.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbValues = " + arguments[0] + "; const __trbRequested = " + arguments[1] + "; let __trbIndex = __trbRequested; if (__trbIndex < 0) __trbIndex += __trbValues.length; if (__trbIndex < 0 || __trbIndex >= __trbValues.length) { return " + indexLookupError("__trbRequested", "__trbValues.length", "Array index is out of bounds") + "; } return " + filesystemOK("__trbValues[__trbIndex]!") + "; })()"
	case "trb.std.arrays.slice", "trb.std.arrays.try_slice":
		safe := name == "trb.std.arrays.try_slice"
		returnType := g.tsType(call.ExprType())
		invalid := "throw new RangeError(\"Array slice range is out of bounds\");"
		success := "return values.slice(start, stop);"
		if safe {
			returnType, _, _ = filesystemResultType()
			invalid = "return " + sliceRangeError("start", "end", "exclusive", "values.length", "Array slice range is out of bounds") + ";"
			success = "return " + filesystemOK("values.slice(start, stop)") + ";"
		}
		return "((): " + returnType + " => { const values = " + arguments[0] + "; const [start, end, exclusive] = " + arguments[1] + "; const valid = start >= 0 && end >= 0 && start <= end && (exclusive ? end <= values.length : end < values.length); if (!valid) { " + invalid + " } const stop = exclusive ? end : end + 1; " + success + " })()"
	case "trb.std.arrays.first":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; if (__trbValues.length === 0) { throw new Error(\"Array is empty\"); } return __trbValues[0]!; })()"
	case "trb.std.arrays.last":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; if (__trbValues.length === 0) { throw new Error(\"Array is empty\"); } return __trbValues[__trbValues.length - 1]!; })()"
	case "trb.std.arrays.copy":
		return "[..." + arguments[0] + "]"
	case "trb.std.arrays.contains":
		return "(" + arguments[0] + ".indexOf(" + arguments[1] + ") >= 0)"
	case "trb.std.arrays.index":
		return "((index: number): number | null => index < 0 ? null : index)(" + arguments[0] + ".indexOf(" + arguments[1] + "))"
	case "trb.std.arrays.count":
		return "((values: Array<unknown>, target: unknown): number => { let count = 0; for (const value of values) { if (value === target) { count++; } } return count; })(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.arrays.uniq":
		return "((values: " + g.tsType(call.ExprType()) + "): " + g.tsType(call.ExprType()) + " => { const result: " + g.tsType(call.ExprType()) + " = []; for (const value of values) { if (result.indexOf(value) < 0) { result.push(value); } } return result; })(" + arguments[0] + ")"
	case "trb.std.arrays.concat":
		return "[..." + arguments[0] + ", ..." + arguments[1] + "]"
	case "trb.std.arrays.join":
		return arguments[0] + ".join(" + arguments[1] + ")"
	case "trb.std.arrays.pop":
		return "((): " + g.tsType(call.ExprType()) + " => { const value = " + arguments[0] + ".pop(); if (value === undefined) { throw new Error(\"Array is empty\"); } return value; })()"
	case "trb.std.arrays.shift":
		return "((): " + g.tsType(call.ExprType()) + " => { const value = " + arguments[0] + ".shift(); if (value === undefined) { throw new Error(\"Array is empty\"); } return value; })()"
	case "trb.std.arrays.push":
		return arguments[0] + ".push(" + arguments[1] + ")"
	case "trb.std.arrays.unshift":
		return arguments[0] + ".unshift(" + arguments[1] + ")"
	case "trb.std.arrays.reverse":
		return "[..." + arguments[0] + "].reverse()"
	case "trb.std.arrays.sort", "trb.std.arrays.sort_descending":
		comparison := tsPortableSortComparison("left.value", "right.value", call.ExprType().Args[0], name == "trb.std.arrays.sort_descending")
		return arguments[0] + ".map((value, index) => ({ value, index })).sort((left, right) => { const compared = " + comparison + "; return compared === 0 ? left.index - right.index : compared; }).map((entry) => entry.value)"
	case "trb.std.ranges.to_array":
		return "((bounds: [number, number, boolean]): Array<number> => { const [start, end, exclusive] = bounds; return Array.from({ length: Math.max(0, end - start + (exclusive ? 0 : 1)) }, (_, index) => start + index); })(" + arguments[0] + ")"
	case "trb.std.hashes.length":
		return "Object.keys(" + arguments[0] + ").length"
	case "trb.std.hashes.empty":
		return "Object.keys(" + arguments[0] + ").length === 0"
	case "trb.std.hashes.fetch":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; const __trbKey = " + arguments[1] + "; if (!Object.prototype.hasOwnProperty.call(__trbValues, __trbKey)) { throw new Error(\"Hash key is missing\"); } return __trbValues[__trbKey]; })()"
	case "trb.std.hashes.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbValues = " + arguments[0] + "; const __trbKey = " + arguments[1] + "; if (!Object.prototype.hasOwnProperty.call(__trbValues, __trbKey)) { return " + keyLookupError("__trbKey", "Hash key is missing") + "; } return " + filesystemOK("__trbValues[__trbKey]") + "; })()"
	case "trb.std.hashes.contains_key":
		return "Object.prototype.hasOwnProperty.call(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.hashes.keys":
		if len(call.ExprType().Args) == 1 && call.ExprType().Args[0].Kind == types.Int {
			return "Object.keys(" + arguments[0] + ").map(Number)"
		}
		return "Object.keys(" + arguments[0] + ")"
	case "trb.std.hashes.values":
		return "Object.values(" + arguments[0] + ")"
	case "trb.std.hashes.copy":
		return "({ ..." + arguments[0] + " })"
	case "trb.std.hashes.delete":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; const __trbKey = " + arguments[1] + "; if (!Object.prototype.hasOwnProperty.call(__trbValues, __trbKey)) { throw new Error(\"Hash key is missing\"); } const __trbValue = __trbValues[__trbKey]; Reflect.deleteProperty(__trbValues, __trbKey); return __trbValue; })()"
	case "trb.std.hashes.merge":
		return "({ ..." + arguments[0] + ", ..." + arguments[1] + " })"
	case "trb.std.hashes.update":
		return "Object.assign(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.to_string":
		return "String(" + arguments[0] + ")"
	case "trb.std.numbers.integer_to_float":
		return "Number(" + arguments[0] + ")"
	case "trb.std.numbers.integer_absolute":
		return "Math.abs(" + arguments[0] + ")"
	case "trb.std.numbers.integer_min":
		return "Math.min(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.integer_max":
		return "Math.max(" + arguments[0] + ", " + arguments[1] + ")"
	case "trb.std.numbers.integer_clamp":
		return "((value: number, minimum: number, maximum: number): number => { if (minimum > maximum) throw new RangeError(\"clamp minimum exceeds maximum\"); return Math.min(Math.max(value, minimum), maximum); })(" + strings.Join(arguments, ", ") + ")"
	case "trb.std.numbers.integer_zero":
		return arguments[0] + " === 0"
	case "trb.std.numbers.integer_positive":
		return arguments[0] + " > 0"
	case "trb.std.numbers.integer_negative":
		return arguments[0] + " < 0"
	case "trb.std.numbers.integer_even":
		return arguments[0] + " % 2 === 0"
	case "trb.std.numbers.integer_odd":
		return arguments[0] + " % 2 !== 0"
	case "trb.std.numbers.float_to_string":
		return portableFloatString(arguments[0])
	case "trb.std.numbers.float_to_integer":
		return portableFloatInteger(arguments[0], "Math.trunc(value)")
	case "trb.std.numbers.float_floor":
		return portableFloatInteger(arguments[0], "Math.floor(value)")
	case "trb.std.numbers.float_ceil":
		return portableFloatInteger(arguments[0], "Math.ceil(value)")
	case "trb.std.numbers.float_round":
		return portableFloatInteger(arguments[0], "value < 0 ? -Math.round(-value) : Math.round(value)")
	case "trb.std.numbers.float_absolute":
		return "Math.abs(" + arguments[0] + ")"
	case "trb.std.numbers.float_finite":
		return "Number.isFinite(" + arguments[0] + ")"
	case "trb.std.numbers.float_infinite":
		return "((value: number): boolean => value === Infinity || value === -Infinity)(" + arguments[0] + ")"
	case "trb.std.numbers.float_nan":
		return "Number.isNaN(" + arguments[0] + ")"
	case "trb.std.numbers.parse_integer":
		return "((): number => { const __trbInput = " + arguments[0] + "; if (!/^[+-]?[0-9]+$/.test(__trbInput)) { throw new Error(\"invalid Integer\"); } const __trbValue = Number(__trbInput); if (!Number.isSafeInteger(__trbValue)) { throw new Error(\"Integer is outside the portable range\"); } return __trbValue; })()"
	case "trb.std.numbers.try_parse_integer":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbInput = " + arguments[0] + "; if (!/^[+-]?[0-9]+$/.test(__trbInput)) { return " + numberParseError("InvalidFormat", "__trbInput", "invalid Integer") + "; } const __trbValue = Number(__trbInput); if (!Number.isSafeInteger(__trbValue)) { return " + numberParseError("OutOfRange", "__trbInput", "Integer is outside the portable range") + "; } return " + filesystemOK("__trbValue") + "; })()"
	case "trb.std.numbers.parse_float":
		return "((): number => { const __trbInput = " + arguments[0] + "; if (!/^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.test(__trbInput)) { throw new Error(\"invalid Float\"); } const __trbValue = Number(__trbInput); if (!Number.isFinite(__trbValue)) { throw new RangeError(\"Float is outside the portable range\"); } return __trbValue; })()"
	case "trb.std.numbers.try_parse_float":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbInput = " + arguments[0] + "; if (!/^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.test(__trbInput)) { return " + numberParseError("InvalidFormat", "__trbInput", "invalid Float") + "; } const __trbValue = Number(__trbInput); if (!Number.isFinite(__trbValue)) { return " + numberParseError("OutOfRange", "__trbInput", "Float is outside the portable range") + "; } return " + filesystemOK("__trbValue") + "; })()"
	case "trb.std.math.sqrt":
		return "Math.sqrt(" + arguments[0] + ")"
	case "trb.std.math.exp":
		return "Math.exp(" + arguments[0] + ")"
	case "trb.std.math.log":
		return "Math.log(" + arguments[0] + ")"
	case "trb.std.math.log2":
		return "Math.log2(" + arguments[0] + ")"
	case "trb.std.math.log10":
		return "Math.log10(" + arguments[0] + ")"
	case "trb.std.booleans.to_string":
		return "String(" + arguments[0] + ")"
	case "trb.platform.typescript.node.argv":
		return "process.argv.slice(2)"
	case "trb.platform.typescript.react.mount":
		return "createRoot(document.getElementById(" + arguments[1] + ")!).render(" + arguments[0] + ")"
	case "trb.platform.typescript.react.use_state":
		return "useTrbState(" + arguments[0] + ")"
	default:
		return "undefined"
	}
}

func tsWebContextWith(call *ir.Call, arguments []string) string {
	if len(arguments) != 3 {
		return "undefined"
	}
	return "((): " + tsType(call.ExprType()) + " => { const contextValue = " + arguments[0] + "; const contextKey = " + arguments[1] + "; const existing = (contextValue as any).__trb_trb_context_state; const contextState = existing instanceof Map ? new Map<unknown, unknown>(existing) : new Map<unknown, unknown>(); contextState.set(contextKey, " + arguments[2] + "); const result = contextValue.with_request(contextValue.__trb_request); (result as any).__trb_trb_context_state = contextState; return result; })()"
}

func tsWebContextWithRequest(call *ir.Call, arguments []string) string {
	if len(arguments) != 2 {
		return "undefined"
	}
	return "((): " + tsType(call.ExprType()) + " => { const contextValue = " + arguments[0] + "; const result = contextValue.with_request(" + arguments[1] + "); (result as any).__trb_trb_context_state = (contextValue as any).__trb_trb_context_state; return result; })()"
}

func tsWebContextFetch(call *ir.Call, arguments []string) string {
	if len(arguments) != 2 || len(call.ExprType().Args) != 2 {
		return "undefined"
	}
	valueType := tsType(call.ExprType().Args[0])
	errorType := tsType(call.ExprType().Args[1])
	resultType := tsType(call.ExprType())
	return "((): " + resultType + " => { const contextValue = " + arguments[0] + "; const contextKey = " + arguments[1] + "; const contextState = (contextValue as any).__trb_trb_context_state; if (contextState instanceof Map && contextState.has(contextKey)) return Result.Ok<" + valueType + ", " + errorType + ">(contextState.get(contextKey) as " + valueType + "); return Result.Err<" + valueType + ", " + errorType + ">({ key: contextKey.__trb_name }); })()"
}

func (g *generator) tsWebEndpointInput(call *ir.Call, receiver string) string {
	if call.Codec == nil || call.Codec.Kind != "endpoint_input" || len(call.ExprType().Args) != 2 {
		return "undefined"
	}
	valueType := tsCodecType(call.Codec)
	errorType := "__trb_web.EndpointInputError"
	resultType := "Result<" + valueType + ", " + errorType + ">"
	errResult := func(variant, value string) string {
		return "Result.Err<" + valueType + ", " + errorType + ">(__trb_web.EndpointInputError." + variant + "(" + value + "))"
	}
	var body strings.Builder
	body.WriteString("const contextValue = " + receiver + "; ")
	constructor := make([]string, 0, len(call.Codec.Fields))
	for index, field := range call.Codec.Fields {
		fieldResult := "inputResult" + strconv.Itoa(index)
		fieldValue := "inputField" + strconv.Itoa(index)
		variant := "Params"
		fieldReceiver := "contextValue"
		if field.Name == "query" {
			variant = "Query"
			fieldReceiver = "contextValue.__trb_request"
		} else if field.Name == "body" {
			variant = "Body"
			fieldReceiver = "contextValue.__trb_request"
		}
		fieldCall := *call
		fieldCall.Codec = field.Schema
		errorName := "ParameterError"
		if field.Name == "body" {
			errorName = "RequestError"
		}
		fieldCall.ExprBase = ir.NewExprBase(call.Span, types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{field.Schema.Type, types.FromName(errorName)}})
		fieldExpression := g.tsWebParameterBinding(&fieldCall, fieldReceiver, field.Name)
		if field.Name == "body" {
			fieldExpression = g.tsWebRequestJSON(&fieldCall, fieldReceiver)
		}
		body.WriteString("const " + fieldResult + " = " + fieldExpression + "; if (" + fieldResult + ".kind === \"Err\") return " + errResult(variant, fieldResult+".error") + "; const " + fieldValue + " = " + fieldResult + ".value; ")
		constructor = append(constructor, field.Name+": "+fieldValue)
	}
	body.WriteString("return Result.Ok<" + valueType + ", " + errorType + ">({ " + strings.Join(constructor, ", ") + " } satisfies " + valueType + ");")
	return "((): " + resultType + " => { " + body.String() + " })()"
}

func portableArrayString(value string, typ types.Type) string {
	elementType := types.Type{Kind: types.Any, Name: "Any"}
	if len(typ.Args) > 0 {
		elementType = typ.Args[0]
	}
	element := "String(item)"
	switch elementType.Kind {
	case types.String:
		element = "JSON.stringify(item)"
	case types.Float:
		element = portableFloatString("item")
	case types.Array:
		element = portableArrayString("item", elementType)
	}
	return "(\"[\" + (" + value + ").map((item) => " + element + ").join(\", \") + \"]\")"
}

func tsWebLogger(call *ir.Call, arguments []string) string {
	options := "const loggerOptions: { stderr: boolean; exclude_paths: string[] } | undefined = undefined; "
	if len(arguments) > 2 {
		options = "const loggerOptions: { stderr: boolean; exclude_paths: string[] } | undefined = " + arguments[2] + "; "
	}
	return "(async (): Promise<" + tsType(call.ExprType()) + "> => { const loggerContext = " + arguments[0] + "; const loggerNextHandler = " + arguments[1] + "; " + options + "const excluded = loggerOptions !== undefined && loggerOptions.exclude_paths.includes(loggerContext.__trb_request.__trb_path); if (excluded) return await loggerNextHandler.call(__trbScope, loggerContext); const started = performance.now(); let status = 500; try { const response = await loggerNextHandler.call(__trbScope, loggerContext); status = response.__trb_status; return response; } finally { const entry = JSON.stringify({ timestamp: new Date().toISOString(), level: status >= 500 ? \"error\" : \"info\", event: \"http_request\", method: loggerContext.__trb_request.__trb_method.to_s(), path: loggerContext.__trb_request.__trb_path, status, duration_ms: performance.now() - started }); if (loggerOptions !== undefined && loggerOptions.stderr) console.error(entry); else console.log(entry); } })()"
}

func tsWebTimeout(call *ir.Call, arguments []string) string {
	return "(async (): Promise<" + tsType(call.ExprType()) + "> => { const controller = new AbortController(); const abort = () => controller.abort(); if (__trbScope?.aborted) abort(); else __trbScope?.addEventListener(\"abort\", abort, { once: true }); const parentDeadline = (__trbScope as (AbortSignal & { __trbDeadline?: number }) | undefined)?.__trbDeadline; const requestedDeadline = performance.now() + " + arguments[2] + "; const deadline = parentDeadline === undefined ? requestedDeadline : Math.min(parentDeadline, requestedDeadline); Object.defineProperties(controller.signal, { __trbDeadline: { value: deadline }, __trbCancel: { value: abort } }); let timer: ReturnType<typeof setTimeout> | undefined; const timeout = new Promise<" + tsType(call.ExprType()) + ">((resolve) => { timer = setTimeout(() => { abort(); resolve(" + arguments[3] + "); }, Math.max(0, deadline - performance.now())); }); try { const result = await Promise.race([" + arguments[1] + ".call(controller.signal, " + arguments[0] + "), timeout]); if (controller.signal.aborted && performance.now() >= deadline) return " + arguments[3] + "; return result; } catch (error) { if (controller.signal.aborted && performance.now() >= deadline) return " + arguments[3] + "; throw error; } finally { if (timer !== undefined) clearTimeout(timer); __trbScope?.removeEventListener(\"abort\", abort); } })()"
}

func tsWebGzip(value string) string {
	return "((value: Uint8Array): Uint8Array => { const bun = Reflect.get(globalThis, \"Bun\") as { gzipSync?: (input: Uint8Array) => Uint8Array } | undefined; if (typeof bun?.gzipSync === \"function\") return new Uint8Array(bun.gzipSync(value)); const host = Reflect.get(globalThis, \"process\") as { getBuiltinModule?: (name: string) => unknown } | undefined; const zlib = host?.getBuiltinModule?.(\"zlib\") as { gzipSync?: (input: Uint8Array) => Uint8Array } | undefined; if (typeof zlib?.gzipSync !== \"function\") throw new Error(\"trb/web gzip compression is unavailable in this TypeScript runtime\"); return new Uint8Array(zlib.gzipSync(value)); })(" + value + ")"
}

func (g *generator) tsWebRequestJSON(call *ir.Call, request string) string {
	if call.Codec == nil || len(call.ExprType().Args) != 2 {
		return "undefined"
	}
	decodeCall := *call
	decodeCall.ExprBase.Type = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{call.ExprType().Args[0], types.FromName("JsonError")}}
	decoded := g.tsJSONDecode(&decodeCall, "source")
	valueType := tsCodecType(call.Codec)
	errorType := "__trb_web.RequestError"
	errResult := func(value string) string {
		return "Result.Err<" + valueType + ", " + errorType + ">(" + value + ")"
	}
	okResult := "Result.Ok<" + valueType + ", " + errorType + ">"
	missing := "__trb_web.RequestError.MissingContentType"
	duplicate := "__trb_web.RequestError.DuplicateContentType"
	unsupported := "__trb_web.RequestError.UnsupportedContentType(contentTypes[0]!)"
	invalidUTF8 := "__trb_web.RequestError.InvalidUtf8"
	invalidJSON := func(value string) string { return "__trb_web.RequestError.InvalidJson(" + value + ")" }
	return "(() => { const requestValue = " + request + "; const contentTypes = requestValue.__trb_headers.entries().filter((entry) => entry.name.toLowerCase() === \"content-type\").map((entry) => entry.value); if (contentTypes.length === 0) return " + errResult(missing) + "; if (contentTypes.length !== 1) return " + errResult(duplicate) + "; const mediaType = contentTypes[0]!.split(\";\", 1)[0]!.trim().toLowerCase(); if (mediaType !== \"application/json\" && !(mediaType.startsWith(\"application/\") && mediaType.endsWith(\"+json\"))) return " + errResult(unsupported) + "; let source: string; try { source = new TextDecoder(\"utf-8\", { fatal: true }).decode(requestValue.__trb_body.bytes()); } catch { return " + errResult(invalidUTF8) + "; } const decoded = " + decoded + "; if (decoded.kind === \"Err\") return " + errResult(invalidJSON("decoded.error")) + "; return " + okResult + "(decoded.value); })()"
}

func (g *generator) tsWebParameterBinding(call *ir.Call, receiver, source string) string {
	if call.Codec == nil || call.Codec.Kind != "record" || len(call.ExprType().Args) != 2 {
		return "undefined"
	}
	valueType := tsCodecType(call.Codec)
	errorType := "__trb_web.ParameterError"
	resultType := "Result<" + valueType + ", " + errorType + ">"
	sourceName := "Path"
	if source == "query" {
		sourceName = "Query"
	}
	sourceValue := "__trb_web.ParameterSource." + sourceName
	errResult := func(value string) string {
		return "Result.Err<" + valueType + ", " + errorType + ">(" + value + ")"
	}
	missing := func(name string) string {
		return errResult("__trb_web.ParameterError.Missing(" + sourceValue + ", " + strconv.Quote(name) + ")")
	}
	duplicate := func(name string) string {
		return errResult("__trb_web.ParameterError.Duplicate(" + sourceValue + ", " + strconv.Quote(name) + ")")
	}
	invalid := func(name, value, expected string) string {
		return errResult("__trb_web.ParameterError.Invalid(" + sourceValue + ", " + strconv.Quote(name) + ", " + value + ", " + strconv.Quote(expected) + ")")
	}
	var body strings.Builder
	body.WriteString("const parameterValues = new Map<string, Array<string>>(); ")
	if source == "query" {
		body.WriteString("const queryResult = parameterReceiver.query_parameters(); if (queryResult.kind === \"Err\") return " + errResult("__trb_web.ParameterError.MalformedQuery(queryResult.error)") + "; for (const parameter of queryResult.value) { const values = parameterValues.get(parameter.name) ?? []; values.push(parameter.value); parameterValues.set(parameter.name, values); } ")
	} else {
		for _, field := range call.Codec.Fields {
			body.WriteString("parameterValues.set(" + strconv.Quote(field.Name) + ", [parameterReceiver.path_value(" + strconv.Quote(field.Name) + ")]); ")
		}
	}
	constructor := make([]string, 0, len(call.Codec.Fields))
	for index, field := range call.Codec.Fields {
		variable := "field" + strconv.Itoa(index)
		values := "values" + strconv.Itoa(index)
		body.WriteString("const " + values + " = parameterValues.get(" + strconv.Quote(field.Name) + ") ?? []; let " + variable + ": " + tsCodecType(field.Schema) + "; ")
		if field.Schema.Kind == "array" {
			parser := g.tsWebParameterParser(field.Schema.Element)
			body.WriteString("if (" + values + ".length === 0) { ")
			if field.Schema.Type.Nullable {
				body.WriteString(variable + " = null;")
			} else {
				body.WriteString(variable + " = [];")
			}
			body.WriteString(" } else { const parsedValues: Array<" + tsCodecType(field.Schema.Element) + "> = []; for (const rawValue of " + values + ") { const parsedValue = " + parser + "(rawValue); if (parsedValue === undefined) return " + invalid(field.Name, "rawValue", tsParameterExpected(field.Schema.Element)) + "; parsedValues.push(parsedValue); } " + variable + " = parsedValues; } ")
		} else {
			body.WriteString("if (" + values + ".length === 0) { ")
			if field.Schema.Type.Nullable {
				body.WriteString(variable + " = null;")
			} else {
				body.WriteString("return " + missing(field.Name) + ";")
			}
			body.WriteString(" } else if (" + values + ".length > 1) { return " + duplicate(field.Name) + "; } else { const rawValue = " + values + "[0]!; ")
			nonnull := *field.Schema
			nonnull.Type.Nullable = false
			parser := g.tsWebParameterParser(&nonnull)
			body.WriteString("const parsedValue = " + parser + "(rawValue); if (parsedValue === undefined) return " + invalid(field.Name, "rawValue", tsParameterExpected(&nonnull)) + "; " + variable + " = parsedValue; } ")
		}
		constructor = append(constructor, field.Name+": "+variable)
	}
	body.WriteString("return Result.Ok<" + valueType + ", " + errorType + ">({ " + strings.Join(constructor, ", ") + " } satisfies " + valueType + ");")
	return "((): " + resultType + " => { const parameterReceiver = " + receiver + "; " + body.String() + " })()"
}

func tsParameterExpected(schema *ir.CodecSchema) string {
	if schema == nil {
		return "value"
	}
	typ := schema.Type
	typ.Nullable = false
	return typ.String()
}

func (g *generator) tsWebParameterParser(schema *ir.CodecSchema) string {
	valueType := tsCodecType(schema)
	body := "return value;"
	switch schema.Kind {
	case "string":
		body = "return value;"
	case "boolean":
		body = "if (value === \"true\") return true; if (value === \"false\") return false; return undefined;"
	case "integer":
		body = "if (!/^[+-]?[0-9]+$/.test(value)) return undefined; const parsed = Number(value); return Number.isSafeInteger(parsed) ? parsed : undefined;"
	case "float":
		body = "if (!/^[+-]?(?:[0-9]+(?:\\.[0-9]*)?|\\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/.test(value)) return undefined; const parsed = Number(value); return Number.isFinite(parsed) ? parsed : undefined;"
	case "time_date", "time_of_day", "time_datetime", "time_instant", "time_duration", "time_zone":
		owner := schema.Type.Name
		if schema.Reference != nil && schema.Reference.Alias != "" {
			owner = schema.Reference.Alias + "." + owner
		}
		method := "try_parse"
		if schema.Kind == "time_zone" {
			method = "try_get"
		}
		body = "const parsed = " + owner + "." + method + "(value); return parsed.kind === \"Ok\" ? parsed.value : undefined;"
	case "raw_enum":
		owner := schema.Type.Name
		if schema.Reference != nil && schema.Reference.Alias != "" {
			owner = schema.Reference.Alias + "." + owner
		}
		branches := make([]string, 0, len(schema.RawValues))
		for _, item := range schema.RawValues {
			raw := item.Raw
			if schema.RawType.Kind == types.Int {
				raw = strconv.Quote(item.Raw)
			}
			branches = append(branches, "case "+raw+": return "+owner+"."+item.Member+";")
		}
		body = "switch (value) { " + strings.Join(branches, " ") + " } return undefined;"
	}
	return "((value: string): " + valueType + " | undefined => { " + body + " })"
}

func (g *generator) tsWebJSON(call *ir.Call, arguments []string) string {
	if call.Codec == nil || len(arguments) == 0 {
		return "undefined"
	}
	status := "200"
	if len(arguments) > 1 {
		status = arguments[1]
	}
	encodeCall := *call
	encodeCall.ExprBase.Type = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("String"), types.FromName("JsonError")}}
	encoded := g.tsJSONEncode(&encodeCall, arguments[0])
	headers := `new __trb_http.Headers([{ name: "content-type", value: "application/json; charset=utf-8" }])`
	return "(() => { const encoded = " + encoded + "; if (encoded.kind === \"Err\") { return new __trb_web.Response(500, " + headers + ", new __trb_http.Body(new TextEncoder().encode(\"{\\\"error\\\":\\\"internal_server_error\\\"}\"))); } return new __trb_web.Response(" + status + ", " + headers + ", new __trb_http.Body(new TextEncoder().encode(encoded.value))); })()"
}
