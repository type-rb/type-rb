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
	bodySetup := "let nativeBody: BodyInit | undefined; "
	if values["body"] != "null" {
		bodySetup += "const requestBody: " + g.runtimeName("RequestBody") + " | null = " + values["body"] + "; if (requestBody !== null) { switch (requestBody.kind) { case \"Text\": nativeBody = requestBody.value; if (!headers.has(\"content-type\")) headers.set(\"content-type\", \"text/plain; charset=utf-8\"); break; case \"Bytes\": nativeBody = new Uint8Array(requestBody.value); break; case \"Form\": nativeBody = requestBody.value.map((item) => encodeURIComponent(item.name) + \"=\" + encodeURIComponent(item.value)).join(\"&\"); if (!headers.has(\"content-type\")) headers.set(\"content-type\", \"application/x-www-form-urlencoded;charset=UTF-8\"); break; case \"Json\": nativeBody = requestBody.value; if (!headers.has(\"content-type\")) headers.set(\"content-type\", \"application/json\"); break; } } "
	}
	return "(async (): Promise<" + resultType + "> => { " +
		"const __trbClient = " + arguments[0] + "; const path = " + values["path"] + "; const base = __trbClient.__trb_base_url; " +
		"let url = base.length === 0 ? path : new URL(path, base.endsWith(\"/\") ? base : base + \"/\").toString(); " +
		"const query = " + values["query"] + "; if (query.length > 0) { const encoded = query.map((item) => encodeURIComponent(item.name) + \"=\" + encodeURIComponent(item.value)).join(\"&\"); url += (url.includes(\"?\") ? \"&\" : \"?\") + encoded; } " +
		"const headers = new globalThis.Headers(); const requestHeaders = " + values["headers"] + "; if (requestHeaders !== null) { for (const header of requestHeaders.entries()) headers.append(header.name, header.value); } " +
		bodySetup +
		"const timeout: number | null = " + values["timeout_milliseconds"] + "; const controller = new globalThis.AbortController(); const abort = () => controller.abort(); if (__trbScope?.aborted) abort(); else __trbScope?.addEventListener(\"abort\", abort, { once: true }); let timedOut = false; const timer = timeout === null ? null : globalThis.setTimeout(() => { timedOut = true; controller.abort(); }, timeout); try { " +
		"const nativeResponse = await globalThis.fetch(url, { method: " + values["method"] + ", headers, body: nativeBody, signal: controller.signal }); const responseHeaders: Array<{ name: string; value: string }> = []; nativeResponse.headers.forEach((value, name) => responseHeaders.push({ name, value })); const bytes = new Uint8Array(await nativeResponse.arrayBuffer()); const response: " + successType + " = " + g.browserRuntimeName("_transport_response") + "(nativeResponse.status, responseHeaders, nativeResponse.url, bytes); return " + g.browserOK(call, "response") + "; " +
		"} catch (error) { const message = error instanceof Error ? error.message : String(error); const aborted = error instanceof DOMException && error.name === \"AbortError\"; const kind = timedOut ? " + g.runtimeName("RequestErrorKind") + ".Timeout : aborted ? " + g.runtimeName("RequestErrorKind") + ".Abort : " + g.runtimeName("RequestErrorKind") + ".Network; return " + g.browserErrorValue(call, "kind", "message", "null") + "; } finally { if (timer !== null) globalThis.clearTimeout(timer); __trbScope?.removeEventListener(\"abort\", abort); } })()"
}

func (g *generator) browserResponseJSON(call *ir.Call, argument string) string {
	if call.Codec == nil {
		return "undefined"
	}
	resultType, successType, _ := g.browserResultParts(call)
	builder := &tsJSONCodecBuilder{}
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
	builder := &tsJSONCodecBuilder{}
	encoder := builder.encoder(call.Codec)
	contractError := g.browserError(call, "Contract", "message", "null")
	return "((): " + resultType + " => { " + builder.source.String() +
		"const convert = (value: JsonValue): unknown => { switch (value.kind) { case \"Null\": return null; case \"Boolean\": return value.value; case \"Integer\": if (!Number.isSafeInteger(value.value)) throw new Error(\"JSON integer is outside the portable range\"); return value.value; case \"Float\": if (!Number.isFinite(value.value)) throw new Error(\"JSON Float must be finite\"); return value.value; case \"String\": return value.value; case \"Array\": return value.value.map(convert); case \"Object\": { const fields: Record<string, unknown> = {}; for (const [key, item] of Object.entries(value.value)) fields[key] = convert(item); return fields; } } }; " +
		"try { const source = JSON.stringify(convert(" + encoder + "(" + argument + "))); if (source === undefined) throw new Error(\"JSON encoding produced no value\"); return " + g.browserOK(call, "({ kind: \"Json\", value: source } satisfies "+g.runtimeName("RequestBody")+")") + "; } catch (error) { const message = error instanceof Error ? error.message : String(error); return " + contractError + "; } })()"
}
