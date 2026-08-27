package typescript

import (
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
)

const browserHTTPPrefix = "trb.platform.typescript.browser."

func (g *generator) browserHTTPIntrinsic(name string, call *ir.Call, arguments []string) (string, bool) {
	if !strings.HasPrefix(name, browserHTTPPrefix) {
		return "", false
	}
	switch name {
	case browserHTTPPrefix + "file_read":
		return g.browserFileRead(call, arguments[0], false), true
	case browserHTTPPrefix + "file_read_text":
		return g.browserFileRead(call, arguments[0], true), true
	case browserHTTPPrefix + "request":
		return g.browserRequest(call, arguments), true
	case browserHTTPPrefix + "response_json":
		return g.browserResponseJSON(call, arguments[0]), true
	case browserHTTPPrefix + "response_text":
		return g.browserResponseMap(call, arguments[0], `new TextDecoder("utf-8").decode(response.__trb_body.bytes())`), true
	case browserHTTPPrefix + "response_bytes":
		return g.browserResponseMap(call, arguments[0], "response.__trb_body.bytes()"), true
	case browserHTTPPrefix + "response_no_body":
		return g.browserResponseNoBody(call, arguments[0]), true
	case browserHTTPPrefix + "json_body":
		return g.browserJSONBody(call, arguments[0]), true
	default:
		return "undefined", true
	}
}

func (g *generator) browserFileRead(call *ir.Call, file string, text bool) string {
	resultType, successType, failureType := g.browserResultParts(call)
	read := "new Uint8Array(await __trbFile.arrayBuffer())"
	if text {
		read = "await __trbFile.text()"
	}
	return "(async (): Promise<" + resultType + "> => { const __trbFile = " + file + " as unknown as globalThis.File; try { const __trbValue: " + successType + " = " + read + "; return " + g.runtimeName("Result") + ".Ok<" + successType + ", " + failureType + ">(__trbValue); } catch (__trbError) { const __trbMessage = __trbError instanceof Error ? __trbError.message : String(__trbError); const __trbFailure = { message: __trbMessage } satisfies " + failureType + "; return " + g.runtimeName("Result") + ".Err<" + successType + ", " + failureType + ">(__trbFailure); } })()"
}

func (g *generator) browserResponseNoBody(call *ir.Call, argument string) string {
	resultType, successType, _ := g.browserResultParts(call)
	return "((response): " + resultType + " => { if (response.__trb_body.bytes().byteLength !== 0) { const message = \"expected an empty response body\"; return " + g.browserError(call, "Contract", "message", "response") + "; } return " + g.browserOK(call, g.browserResponseValue("response", "({} satisfies "+g.runtimeName("NoBody")+")", successType)) + "; })(" + argument + ")"
}

func (g *generator) browserRuntimeName(name string) string {
	alias := g.browserRuntime
	if alias == "" {
		alias = "__trb_browser"
	}
	return alias + "." + name
}

func (g *generator) browserResultParts(call *ir.Call) (string, string, string) {
	result := call.ExprType()
	if len(result.Args) != 2 {
		return g.tsType(result), "unknown", "unknown"
	}
	return g.tsType(result), g.tsType(result.Args[0]), g.tsType(result.Args[1])
}

func (g *generator) browserOK(call *ir.Call, value string) string {
	_, success, failure := g.browserResultParts(call)
	return g.runtimeName("Result") + ".Ok<" + success + ", " + failure + ">(" + value + ")"
}

func (g *generator) browserError(call *ir.Call, kind, message, response string) string {
	return g.browserErrorValue(call, g.runtimeName("RequestErrorKind")+"."+kind, message, response)
}

func (g *generator) browserErrorValue(call *ir.Call, kind, message, response string) string {
	_, success, failure := g.browserResultParts(call)
	errorValue := "new " + g.runtimeName("RequestError") + "(" + kind + ", " + message + ", " + response + ")"
	return g.runtimeName("Result") + ".Err<" + success + ", " + failure + ">(" + errorValue + ")"
}

func (g *generator) browserResponseValue(response, body, responseType string) string {
	return "(" + g.browserRuntimeName("_map_response") + "(" + response + ", " + body + ") satisfies " + responseType + ")"
}

func (g *generator) browserResponseMap(call *ir.Call, argument, body string) string {
	responseType := g.tsType(call.ExprType())
	return "((response): " + responseType + " => " + g.browserResponseValue("response", body, responseType) + ")(" + argument + ")"
}

func (g *generator) browserRequest(call *ir.Call, arguments []string) string {
	resultType, successType, _ := g.browserResultParts(call)
	values := map[string]string{
		"method":               strconv.Quote("GET"),
		"query":                "[]",
		"headers":              "null",
		"body":                 "null",
		"timeout_milliseconds": "null",
	}
	if len(arguments) > 1 {
		values["path"] = arguments[1]
	}
	for index, argument := range call.Arguments {
		if index+1 >= len(arguments) {
			break
		}
		if argument.Name != "" {
			if argument.Name == "method" {
				values[argument.Name] = arguments[index+1] + ".to_s()"
			} else {
				values[argument.Name] = arguments[index+1]
			}
		}
	}
	bodySetup := "let __trbNativeBody: BodyInit | undefined; "
	if values["body"] != "null" {
		bodySetup += "const __trbRequestBody: " + g.runtimeName("RequestBody") + " | null = " + values["body"] + "; if (__trbRequestBody !== null) { switch (__trbRequestBody.kind) { case \"Text\": __trbNativeBody = __trbRequestBody.value; if (!__trbHeaders.has(\"content-type\")) __trbHeaders.set(\"content-type\", \"text/plain; charset=utf-8\"); break; case \"Bytes\": __trbNativeBody = new Uint8Array(__trbRequestBody.value); break; case \"File\": __trbNativeBody = __trbRequestBody.value as unknown as globalThis.File; if (!__trbHeaders.has(\"content-type\") && __trbRequestBody.value.type.length > 0) __trbHeaders.set(\"content-type\", __trbRequestBody.value.type); break; case \"Form\": __trbNativeBody = __trbRequestBody.value.map((item) => encodeURIComponent(item.name) + \"=\" + encodeURIComponent(item.value)).join(\"&\"); if (!__trbHeaders.has(\"content-type\")) __trbHeaders.set(\"content-type\", \"application/x-www-form-urlencoded;charset=UTF-8\"); break; case \"Json\": __trbNativeBody = __trbRequestBody.value; if (!__trbHeaders.has(\"content-type\")) __trbHeaders.set(\"content-type\", \"application/json\"); break; } } "
	}
	querySetup := ""
	if values["query"] != "[]" {
		querySetup = "const __trbQuery = " + values["query"] + "; if (__trbQuery.length > 0) { const __trbEncodedQuery = __trbQuery.map((item) => encodeURIComponent(item.name) + \"=\" + encodeURIComponent(item.value)).join(\"&\"); __trbUrl += (__trbUrl.includes(\"?\") ? \"&\" : \"?\") + __trbEncodedQuery; } "
	}
	headersSetup := "const __trbHeaders = new globalThis.Headers(); "
	if values["headers"] != "null" {
		headersSetup += "const __trbRequestHeaders = " + values["headers"] + "; if (__trbRequestHeaders !== null) { for (const __trbHeader of __trbRequestHeaders.entries()) __trbHeaders.append(__trbHeader.name, __trbHeader.value); } "
	}
	return "(async (): Promise<" + resultType + "> => { " +
		"const __trbClient = " + arguments[0] + "; const __trbPath = " + values["path"] + "; const __trbBase = __trbClient.__trb_base_url; " +
		"let __trbUrl = __trbBase.length === 0 ? __trbPath : new URL(__trbPath, __trbBase.endsWith(\"/\") ? __trbBase : __trbBase + \"/\").toString(); " +
		querySetup + headersSetup +
		bodySetup +
		"const __trbTimeout: number | null = " + values["timeout_milliseconds"] + "; const __trbController = new globalThis.AbortController(); const __trbAbort = () => __trbController.abort(); if (__trbScope?.aborted) __trbAbort(); else __trbScope?.addEventListener(\"abort\", __trbAbort, { once: true }); let __trbTimedOut = false; const __trbTimer = __trbTimeout === null ? null : globalThis.setTimeout(() => { __trbTimedOut = true; __trbController.abort(); }, __trbTimeout); try { " +
		"const __trbNativeResponse = await globalThis.fetch(__trbUrl, { method: " + values["method"] + ", headers: __trbHeaders, body: __trbNativeBody, signal: __trbController.signal }); const __trbResponseHeaders: Array<{ name: string; value: string }> = []; __trbNativeResponse.headers.forEach((value, name) => __trbResponseHeaders.push({ name, value })); const __trbBytes = new Uint8Array(await __trbNativeResponse.arrayBuffer()); const __trbResponse: " + successType + " = " + g.browserRuntimeName("_transport_response") + "(__trbNativeResponse.status, __trbResponseHeaders, __trbNativeResponse.url, __trbBytes); return " + g.browserOK(call, "__trbResponse") + "; " +
		"} catch (__trbError) { const __trbMessage = __trbError instanceof Error ? __trbError.message : String(__trbError); const __trbAborted = __trbError instanceof DOMException && __trbError.name === \"AbortError\"; const __trbKind = __trbTimedOut ? " + g.runtimeName("RequestErrorKind") + ".Timeout : __trbAborted ? " + g.runtimeName("RequestErrorKind") + ".Abort : " + g.runtimeName("RequestErrorKind") + ".Network; return " + g.browserErrorValue(call, "__trbKind", "__trbMessage", "null") + "; } finally { if (__trbTimer !== null) globalThis.clearTimeout(__trbTimer); __trbScope?.removeEventListener(\"abort\", __trbAbort); } })()"
}

func (g *generator) browserResponseJSON(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "undefined"
	}
	resultType, successType, _ := g.browserResultParts(call)
	builder := g.jsonCodecBuilder("")
	decoder := builder.decoder(call.Codec)
	jsonValue := "JsonValue"
	contractError := g.browserError(call, "Contract", "message", "response")
	return "((response): " + resultType + " => { const source = new TextDecoder(\"utf-8\", { fatal: true }); " +
		"const fail = (path: string, detail: string): never => { throw new Error((path.length === 0 ? \"/\" : path) + \": \" + detail); }; " + builder.source.String() +
		"const convert = (value: unknown, path: string): " + jsonValue + " => { if (value === null) return JsonValue.Null; if (typeof value === \"boolean\") return JsonValue.Boolean(value); if (typeof value === \"string\") return JsonValue.String(value); if (typeof value === \"number\") { if (!Number.isFinite(value)) return fail(path, \"JSON number is not finite\"); if (Number.isInteger(value)) { if (!Number.isSafeInteger(value)) return fail(path, \"JSON integer is outside the portable range\"); return JsonValue.Integer(value); } return JsonValue.Float(value); } if (Array.isArray(value)) return JsonValue.Array(value.map((item, index) => convert(item, path + \"/\" + String(index)))); if (typeof value === \"object\") { const fields: Record<string, JsonValue> = {}; for (const [key, item] of Object.entries(value)) fields[key] = convert(item, path + \"/\" + key.replaceAll(\"~\", \"~0\").replaceAll(\"/\", \"~1\")); return JsonValue.Object(fields); } return fail(path, \"unsupported JSON value\"); }; " +
		"try { const parsed: unknown = JSON.parse(source.decode(response.__trb_body.bytes())); const value = " + decoder + "(convert(parsed, \"\"), \"\"); return " + g.browserOK(call, g.browserResponseValue("response", "value", successType)) + "; } catch (error) { const message = error instanceof Error ? error.message : String(error); return " + contractError + "; } })(" + argument + ")"
}

func (g *generator) browserJSONBody(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "undefined"
	}
	resultType, _, _ := g.browserResultParts(call)
	builder := g.jsonCodecBuilder("")
	encoder := builder.encoder(call.Codec)
	contractError := g.browserError(call, "Contract", "message", "null")
	return "((): " + resultType + " => { " + builder.source.String() +
		"const convert = (value: JsonValue): unknown => { switch (value.kind) { case \"Null\": return null; case \"Boolean\": return value.value; case \"Integer\": if (!Number.isSafeInteger(value.value)) throw new Error(\"JSON integer is outside the portable range\"); return value.value; case \"Float\": if (!Number.isFinite(value.value)) throw new Error(\"JSON Float must be finite\"); return value.value; case \"String\": return value.value; case \"Array\": return value.value.map(convert); case \"Object\": { const fields: Record<string, unknown> = {}; for (const [key, item] of Object.entries(value.value)) fields[key] = convert(item); return fields; } } }; " +
		"try { const source = JSON.stringify(convert(" + encoder + "(" + argument + "))); if (source === undefined) throw new Error(\"JSON encoding produced no value\"); return " + g.browserOK(call, "({ kind: \"Json\", value: source } satisfies "+g.runtimeName("RequestBody")+")") + "; } catch (error) { const message = error instanceof Error ? error.message : String(error); return " + contractError + "; } })()"
}
