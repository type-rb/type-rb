package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	"github.com/type-rb/type-rb/internal/types"
)

func (g *generator) intrinsic(name string, call *ir.Call, arguments []string) string {
	if name == "trb.internal.runtime.fail" {
		return "((): " + g.tsType(call.ExprType()) + " => { throw new Error(" + arguments[0] + "); })()"
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
	keyLookupError := func(key, message string) string {
		value := "({ key: " + key + ", message: " + strconv.Quote(message) + " } satisfies " + g.runtimeName("KeyLookupError") + ")"
		return resultError(value)
	}
	filesystemHandle := `const fs = (globalThis as any).process?.getBuiltinModule?.("fs"); if (fs === undefined) { throw new Error("filesystem is unavailable"); } `
	filesystemMessage := `const message = error instanceof Error ? error.message : String(error); `
	switch name {
	case "trb.std.io.puts":
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
		return "trb_web_dispatch(" + arguments[0] + ")"
	case "trb.web.middleware.logger.call":
		return tsWebLogger(call, arguments)
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
	case "trb.std.strings.codepoints":
		return "Array.from(" + arguments[0] + ", (value): number => value.codePointAt(0)!)"
	case "trb.std.strings.characters":
		return "Array.from(" + arguments[0] + ")"
	case "trb.std.strings.reverse":
		return "Array.from(" + arguments[0] + ").reverse().join(\"\")"
	case "trb.std.strings.fetch":
		return "((): string => { const __trbValue = Array.from(" + arguments[0] + "); const __trbIndex = " + arguments[1] + "; if (__trbIndex < 0 || __trbIndex >= __trbValue.length) { throw new Error(\"String index is out of bounds\"); } return __trbValue[__trbIndex]!; })()"
	case "trb.std.strings.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbValue = Array.from(" + arguments[0] + "); const __trbIndex = " + arguments[1] + "; if (__trbIndex < 0 || __trbIndex >= __trbValue.length) { return " + indexLookupError("__trbIndex", "__trbValue.length", "String index is out of bounds") + "; } return " + filesystemOK("__trbValue[__trbIndex]!") + "; })()"
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
	case "trb.std.hmac.equal":
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
	case "trb.std.arrays.fetch":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; const __trbIndex = " + arguments[1] + "; if (__trbIndex < 0 || __trbIndex >= __trbValues.length) { throw new Error(\"Array index is out of bounds\"); } return __trbValues[__trbIndex]!; })()"
	case "trb.std.arrays.try_fetch":
		resultType, _, _ := filesystemResultType()
		return "((): " + resultType + " => { const __trbValues = " + arguments[0] + "; const __trbIndex = " + arguments[1] + "; if (__trbIndex < 0 || __trbIndex >= __trbValues.length) { return " + indexLookupError("__trbIndex", "__trbValues.length", "Array index is out of bounds") + "; } return " + filesystemOK("__trbValues[__trbIndex]!") + "; })()"
	case "trb.std.arrays.first":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; if (__trbValues.length === 0) { throw new Error(\"Array is empty\"); } return __trbValues[0]!; })()"
	case "trb.std.arrays.last":
		return "((): " + g.tsType(call.ExprType()) + " => { const __trbValues = " + arguments[0] + "; if (__trbValues.length === 0) { throw new Error(\"Array is empty\"); } return __trbValues[__trbValues.length - 1]!; })()"
	case "trb.std.arrays.copy":
		return "[..." + arguments[0] + "]"
	case "trb.std.arrays.contains":
		return "(" + arguments[0] + ".indexOf(" + arguments[1] + ") >= 0)"
	case "trb.std.arrays.count":
		return "((values: Array<unknown>, target: unknown): number => { let count = 0; for (const value of values) { if (value === target) { count++; } } return count; })(" + arguments[0] + ", " + arguments[1] + ")"
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
	case "trb.platform.typescript.react.element":
		return "React.createElement(" + arguments[0] + ", " + arguments[1] + " as any, ..." + arguments[2] + ")"
	case "trb.platform.typescript.react.mount":
		return "createRoot(document.getElementById(" + arguments[1] + ")!).render(React.createElement(" + arguments[0] + "))"
	case "trb.platform.typescript.react.refresh":
		return arguments[0] + ".forceUpdate()"
	case "trb.platform.typescript.react.prevent_default":
		return arguments[0] + ".preventDefault()"
	case "trb.platform.typescript.react.input_value":
		return "(" + arguments[0] + ".currentTarget as HTMLInputElement).value"
	case "trb.platform.typescript.react.data_integer":
		return "Number((" + arguments[0] + ".currentTarget as HTMLElement).dataset[" + arguments[1] + "])"
	case "trb.platform.typescript.react.data_boolean":
		return "((" + arguments[0] + ".currentTarget as HTMLElement).dataset[" + arguments[1] + "] === \"true\")"
	case "trb.platform.typescript.web.get_json":
		return "void fetch(" + arguments[0] + ").then((response) => { if (!response.ok) throw new Error(`HTTP ${response.status}`); return response.json(); }).then(" + arguments[1] + " as any)"
	case "trb.platform.typescript.web.post_json":
		return g.fetchJSON("POST", arguments)
	case "trb.platform.typescript.web.patch_json":
		return g.fetchJSON("PATCH", arguments)
	default:
		return "undefined"
	}
}

func tsWebLogger(call *ir.Call, arguments []string) string {
	options := "const loggerOptions: { stderr: boolean; exclude_paths: string[] } | undefined = undefined; "
	if len(arguments) > 2 {
		options = "const loggerOptions: { stderr: boolean; exclude_paths: string[] } | undefined = " + arguments[2] + "; "
	}
	return "((): " + tsType(call.ExprType()) + " => { const loggerContext = " + arguments[0] + "; const loggerNextHandler = " + arguments[1] + "; " + options + "const excluded = loggerOptions !== undefined && loggerOptions.exclude_paths.includes(loggerContext.request.path); if (excluded) return loggerNextHandler.call(loggerContext); const started = performance.now(); let status = 500; try { const response = loggerNextHandler.call(loggerContext); status = response.status; return response; } finally { const entry = JSON.stringify({ timestamp: new Date().toISOString(), level: status >= 500 ? \"error\" : \"info\", event: \"http_request\", method: loggerContext.request.method, path: loggerContext.request.path, status, duration_ms: performance.now() - started }); if (loggerOptions !== undefined && loggerOptions.stderr) console.error(entry); else console.log(entry); } })()"
}

func (g *generator) tsWebRequestJSON(call *ir.Call, request string) string {
	if call.Codec == nil {
		return "undefined"
	}
	parseCall := *call
	parseCall.ExprBase.Type = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("JsonValue"), types.FromName("JsonError")}}
	parsed := tsJSONParse(&parseCall, "source", false)
	builder := &tsJSONCodecBuilder{}
	decoder := builder.decoder(call.Codec)
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
	return "(() => { const requestValue = " + request + "; const contentTypes = Object.entries(requestValue.headers).filter(([name]) => name.toLowerCase() === \"content-type\").flatMap(([, values]) => values); if (contentTypes.length === 0) return " + errResult(missing) + "; if (contentTypes.length !== 1) return " + errResult(duplicate) + "; const mediaType = contentTypes[0]!.split(\";\", 1)[0]!.trim().toLowerCase(); if (mediaType !== \"application/json\" && !(mediaType.startsWith(\"application/\") && mediaType.endsWith(\"+json\"))) return " + errResult(unsupported) + "; let source: string; try { source = new TextDecoder(\"utf-8\", { fatal: true }).decode(requestValue.body); } catch { return " + errResult(invalidUTF8) + "; } const codecError = (path: string, message: string): JsonError => ({ kind: JsonErrorKind.Decode, message, path, line: null, column: null }); const fail = (path: string, message: string): never => { throw { __trbJSONCodecError: true, error: codecError(path, message) }; }; " + builder.source.String() + " const parsed = " + parsed + "; if (parsed.kind === \"Err\") return " + errResult(invalidJSON("parsed.error")) + "; try { return " + okResult + "(" + decoder + "(parsed.value, \"\")); } catch (error) { if (typeof error === \"object\" && error !== null && (error as any).__trbJSONCodecError === true) return " + errResult(invalidJSON("(error as any).error as JsonError")) + "; throw error; } })()"
}

func (g *generator) tsWebJSON(call *ir.Call, arguments []string) string {
	if call.Codec == nil || len(arguments) == 0 {
		return "undefined"
	}
	status := "200"
	if len(arguments) > 1 {
		status = arguments[1]
	}
	builder := &tsJSONCodecBuilder{}
	encoder := builder.encoder(call.Codec)
	stringifyCall := *call
	stringifyCall.ExprBase.Type = types.Type{Kind: types.Named, Name: "Result", Args: []types.Type{types.FromName("String"), types.FromName("JsonError")}}
	encoded := tsJSONStringify(&stringifyCall, encoder+"("+arguments[0]+")")
	headers := `{ "content-type": ["application/json; charset=utf-8"] }`
	return "(() => { " + builder.source.String() + " const encoded = " + encoded + "; if (encoded.kind === \"Err\") { return { status: 500, headers: " + headers + ", body: new TextEncoder().encode(\"{\\\"error\\\":\\\"internal_server_error\\\"}\") }; } return { status: " + status + ", headers: " + headers + ", body: new TextEncoder().encode(encoded.value) }; })()"
}
