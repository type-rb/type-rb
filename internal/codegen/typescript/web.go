package typescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	jobsintegration "github.com/type-rb/type-rb/internal/jobs"
	ormintegration "github.com/type-rb/type-rb/internal/orm"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrationImports(extensions []ir.Extension) {
	if manifest := jobsintegration.ManifestFrom(extensions); manifest != nil {
		g.jobsIntegrationImports(manifest)
	}
	if manifest := ormintegration.ManifestFrom(extensions); manifest != nil {
		if g.modulePath == "trb/orm/index" {
			g.line(`import { SQL, type TransactionSQL } from "bun";`)
			g.line(`import { AsyncLocalStorage } from "node:async_hooks";`)
		} else {
			g.line("import * as __trbOrm from " + strconv.Quote(tsImportPath(g.modulePath, "trb/orm/index")) + ";")
		}
	}
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.ensureHTTPRuntime()
		g.ensureWebRuntime()
		g.line(`import { createServer } from "node:http";`)
		g.line(`import type { Socket } from "node:net";`)
		g.webRouteImports(manifest)
	}
}

func (g *generator) integrations(extensions []ir.Extension) {
	if manifest := jobsintegration.ManifestFrom(extensions); manifest != nil {
		g.jobsRuntime(manifest)
	}
	if manifest := ormintegration.ManifestFrom(extensions); manifest != nil {
		g.ormRuntime(manifest)
	}
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webDispatcher(manifest)
		g.webServer()
	}
}

func (g *generator) webRouteImports(manifest *webintegration.Manifest) {
	symbols := map[string][]string{}
	for _, route := range manifest.Routes {
		symbols[route.ModulePath] = append(symbols[route.ModulePath], route.TargetHandler)
	}
	for _, middleware := range manifest.Middlewares {
		symbols[middleware.ModulePath] = append(symbols[middleware.ModulePath], middleware.TargetHandler)
	}
	modulePaths := make([]string, 0, len(symbols))
	for modulePath := range symbols {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		names := symbols[modulePath]
		sort.Strings(names)
		g.line("import { " + strings.Join(names, ", ") + " } from " + strconv.Quote(tsImportPath(g.modulePath, modulePath)) + ";")
	}
}

func (g *generator) webDispatcher(manifest *webintegration.Manifest) {
	routes := manifest.Routes
	rootMiddlewares := webintegration.RootMiddlewares(manifest.Middlewares)
	g.line("type TrbWebRequest = __trb_web.Request;")
	g.line("type TrbWebContext = __trb_web.Context;")
	g.line("type TrbWebResponse = __trb_web.Response;")
	g.line("type TrbWebServerConfig = { host: string; port: number; body_limit_bytes: number; shutdown_timeout_milliseconds: number };")
	g.line("function trb_web_request(method: string, path: string, query_string: string, headers: __trb_http.Headers, body: __trb_http.Body): TrbWebRequest { return new __trb_web.Request(new __trb_http.HttpMethod(method), path, query_string, headers, body); }")
	g.line("function trb_web_context(request: TrbWebRequest, path_parameters: Record<string, string>): TrbWebContext { return new __trb_web.Context(request, path_parameters); }")
	g.line("function trb_web_response(status: number, headers: __trb_http.Headers, body: __trb_http.Body): TrbWebResponse { return new __trb_web.Response(status, headers, body); }")
	g.webProtocolResponses()
	if len(manifest.Middlewares) > 0 {
		g.webNext()
	}
	g.line("async function trb_web_dispatch(signal: AbortSignal | undefined, request: TrbWebRequest): Promise<TrbWebResponse> {")
	g.indent++
	g.line("return trb_web_dispatch_with_body_limit(signal, request, TRB_WEB_DEFAULT_MAX_BODY_BYTES);")
	g.indent--
	g.line("}")
	g.line("async function trb_web_dispatch_with_body_limit(signal: AbortSignal | undefined, request: TrbWebRequest, max_body_bytes: number): Promise<TrbWebResponse> {")
	g.indent++
	g.line("try {")
	g.indent++
	g.line("request = trb_web_normalize_request(request);")
	g.line("const normalized_path = trb_web_normalize_path(request.__trb_path);")
	g.line("if (normalized_path === undefined) return await trb_web_dispatch_protocol_response(signal, request, trb_web_bad_request());")
	g.line("request = trb_web_request(request.__trb_method.to_s(), normalized_path, request.__trb_query_string, request.__trb_headers, request.__trb_body);")
	g.line("const context = trb_web_context(request, {});")
	g.line("let handler = async (handler_signal: AbortSignal | undefined, middleware_context: TrbWebContext): Promise<TrbWebResponse> => await trb_web_dispatch_core(handler_signal, middleware_context.__trb_request, max_body_bytes);")
	g.typescriptWebMiddlewareChain(rootMiddlewares, "handler", "root_next_handler_")
	g.line("return trb_web_finalize_response(request, await handler(signal, context));")
	g.indent--
	g.line("} catch (error) {")
	g.indent++
	g.line(`if (error instanceof DOMException && error.name === "AbortError") throw error;`)
	g.line(`return trb_web_finalize_response(request, trb_web_internal_server_error());`)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("async function trb_web_dispatch_protocol_response(signal: AbortSignal | undefined, request: TrbWebRequest, protocol_response: TrbWebResponse): Promise<TrbWebResponse> {")
	g.indent++
	g.line("try {")
	g.indent++
	g.line("request = trb_web_normalize_request(request);")
	g.line("const normalized_path = trb_web_normalize_path(request.__trb_path);")
	g.line("if (normalized_path !== undefined) request = trb_web_request(request.__trb_method.to_s(), normalized_path, request.__trb_query_string, request.__trb_headers, request.__trb_body);")
	g.line("const context = trb_web_context(request, {});")
	g.line("let handler = async (_handler_signal: AbortSignal | undefined, _middleware_context: TrbWebContext): Promise<TrbWebResponse> => protocol_response;")
	g.typescriptWebMiddlewareChain(rootMiddlewares, "handler", "protocol_next_handler_")
	g.line("return trb_web_finalize_response(request, await handler(signal, context));")
	g.indent--
	g.line("} catch (error) {")
	g.indent++
	g.line(`if (error instanceof DOMException && error.name === "AbortError") throw error;`)
	g.line(`return trb_web_finalize_response(request, trb_web_internal_server_error());`)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("async function trb_web_dispatch_core(signal: AbortSignal | undefined, request: TrbWebRequest, max_body_bytes: number): Promise<TrbWebResponse> {")
	g.indent++
	g.line("try {")
	g.indent++
	g.line("if (request.__trb_body.bytes().byteLength > max_body_bytes) return trb_web_payload_too_large();")
	g.line("const method = request.__trb_method.to_s().toUpperCase();")
	g.line(`const segments = request.__trb_path === "/" ? [] : request.__trb_path.slice(1).split("/");`)
	g.line("let allowed_methods: string[] = [];")
	g.line("let explicit_head = false;")
	for _, route := range routes {
		routeSegments := typescriptWebRouteSegments(route.Path)
		condition := typescriptWebRouteConditions(routeSegments)
		g.line("if (" + strings.Join(condition, " && ") + ") allowed_methods.push(" + strconv.Quote(route.Method) + ");")
		if route.Method == "GET" {
			g.line("if (" + strings.Join(condition, " && ") + `) allowed_methods.push("HEAD");`)
		}
		if route.Method == "HEAD" {
			g.line("if (" + strings.Join(condition, " && ") + ") explicit_head = true;")
		}
	}
	g.line(`if (allowed_methods.length > 0) allowed_methods.push("OPTIONS");`)
	g.line("allowed_methods = [...new Set(allowed_methods)].sort();")
	g.line("let dispatch_method = method;")
	g.line(`if (method === "HEAD" && !explicit_head && allowed_methods.includes("GET")) dispatch_method = "GET";`)
	for routeIndex, route := range routes {
		segments := typescriptWebRouteSegments(route.Path)
		condition := append([]string{"dispatch_method === " + strconv.Quote(route.Method)}, typescriptWebRouteConditions(segments)...)
		g.line("if (" + strings.Join(condition, " && ") + ") {")
		g.indent++
		g.line("const path_parameters: Record<string, string> = {};")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]!;")
			} else if strings.HasPrefix(segment, "*") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, "*")) + "] = segments.slice(" + strconv.Itoa(index) + ").join(\"/\");")
			}
		}
		contextName := "route_context_" + strconv.Itoa(routeIndex)
		handlerName := "route_handler_" + strconv.Itoa(routeIndex)
		g.line("const " + contextName + " = trb_web_context(request, path_parameters);")
		g.line("let " + handlerName + " = async (handler_signal: AbortSignal | undefined, middleware_context: TrbWebContext): Promise<TrbWebResponse> => await " + route.TargetHandler + "(handler_signal, middleware_context);")
		g.typescriptWebMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "next_handler_"+strconv.Itoa(routeIndex)+"_")
		g.line("return trb_web_checked_response(await " + handlerName + "(signal, " + contextName + "));")
		g.indent--
		g.line("}")
	}
	for routeIndex, route := range webintegration.UniquePathRoutes(routes) {
		segments := typescriptWebRouteSegments(route.Path)
		condition := append([]string{`method === "OPTIONS"`}, typescriptWebRouteConditions(segments)...)
		g.line("if (" + strings.Join(condition, " && ") + ") {")
		g.indent++
		g.line("const path_parameters: Record<string, string> = {};")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]!;")
			} else if strings.HasPrefix(segment, "*") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, "*")) + "] = segments.slice(" + strconv.Itoa(index) + ").join(\"/\");")
			}
		}
		contextName := "options_context_" + strconv.Itoa(routeIndex)
		handlerName := "options_handler_" + strconv.Itoa(routeIndex)
		g.line("const " + contextName + " = trb_web_context(request, path_parameters);")
		g.line("let " + handlerName + ` = async (_handler_signal: AbortSignal | undefined, _middleware_context: TrbWebContext): Promise<TrbWebResponse> => trb_web_response(204, new __trb_http.Headers([{ name: "allow", value: allowed_methods.join(", ") }]), __trb_http.Body.empty());`)
		g.typescriptWebMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "options_next_handler_"+strconv.Itoa(routeIndex)+"_")
		g.line("return trb_web_checked_response(await " + handlerName + "(signal, " + contextName + "));")
		g.indent--
		g.line("}")
	}
	g.line("if (allowed_methods.length > 0) {")
	g.indent++
	g.line(`return trb_web_response(405, new __trb_http.Headers([{ name: "allow", value: allowed_methods.join(", ") }, { name: "content-type", value: "application/json; charset=utf-8" }]), new __trb_http.Body(new TextEncoder().encode("{\"error\":\"method_not_allowed\"}")));`)
	g.indent--
	g.line("}")
	g.line(`return trb_web_response(404, new __trb_http.Headers([{ name: "content-type", value: "application/json; charset=utf-8" }]), new __trb_http.Body(new TextEncoder().encode("{\"error\":\"not_found\"}")));`)
	g.indent--
	g.line("} catch (error) {")
	g.indent++
	g.line(`if (error instanceof DOMException && error.name === "AbortError") throw error;`)
	g.line(`return trb_web_internal_server_error();`)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("function trb_web_checked_response(response: TrbWebResponse): TrbWebResponse {")
	g.indent++
	g.line("return trb_web_valid_response(response) ? response : trb_web_internal_server_error();")
	g.indent--
	g.line("}")
	g.line("function trb_web_finalize_response(request: TrbWebRequest, response: TrbWebResponse): TrbWebResponse {")
	g.indent++
	g.line("if (!trb_web_valid_response(response)) response = trb_web_internal_server_error();")
	g.line(`if (request.__trb_method.to_s().toUpperCase() !== "HEAD") return response;`)
	g.line("return trb_web_response(response.__trb_status, response.__trb_headers, __trb_http.Body.empty());")
	g.indent--
	g.line("}")
}

func (g *generator) typescriptWebMiddlewareChain(middlewares []webintegration.Middleware, handlerName, nextPrefix string) {
	for middlewareIndex := len(middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
		middleware := middlewares[middlewareIndex]
		nextName := nextPrefix + strconv.Itoa(middlewareIndex)
		g.line("const " + nextName + " = " + handlerName + ";")
		g.line(handlerName + " = async (handler_signal: AbortSignal | undefined, middleware_context: TrbWebContext): Promise<TrbWebResponse> => await " + middleware.TargetHandler + "(handler_signal, middleware_context, new TrbWebNext(" + nextName + "));")
	}
}

func (g *generator) webProtocolResponses() {
	g.line("const TRB_WEB_DEFAULT_MAX_BODY_BYTES = " + strconv.Itoa(webintegration.MaxBodyBytes) + ";")
	g.line("function trb_web_normalize_request(request: TrbWebRequest): TrbWebRequest {")
	g.indent++
	g.line("const entries: Array<{ name: string; value: string }> = [];")
	g.line("for (const entry of request.__trb_headers.entries()) {")
	g.indent++
	g.line("entries.push({ name: entry.name.toLowerCase(), value: entry.value });")
	g.indent--
	g.line("}")
	g.line("return trb_web_request(request.__trb_method.to_s().toUpperCase(), request.__trb_path, request.__trb_query_string, new __trb_http.Headers(entries), request.__trb_body);")
	g.indent--
	g.line("}")
	g.line("function trb_web_normalize_path(path: string): string | undefined {")
	g.indent++
	g.line(`if (!path.startsWith("/") || path.includes("\\")) return undefined;`)
	g.line(`const segments = path.split("/");`)
	g.line("for (let index = 0; index < segments.length; index += 1) {")
	g.indent++
	g.line("let decoded: string;")
	g.line("try {")
	g.indent++
	g.line("decoded = decodeURIComponent(segments[index]!);")
	g.indent--
	g.line("} catch {")
	g.indent++
	g.line("return undefined;")
	g.indent--
	g.line("}")
	g.line(`if (decoded === "." || decoded === ".." || decoded.includes("/") || decoded.includes("\\")) return undefined;`)
	g.line("segments[index] = decoded;")
	g.indent--
	g.line("}")
	g.line(`return segments.join("/");`)
	g.indent--
	g.line("}")
	g.line("function trb_web_internal_server_error(): TrbWebResponse {")
	g.indent++
	g.line(`return trb_web_response(500, new __trb_http.Headers([{ name: "content-type", value: "application/json; charset=utf-8" }]), new __trb_http.Body(new TextEncoder().encode("{\"error\":\"internal_server_error\"}")));`)
	g.indent--
	g.line("}")
	g.line("function trb_web_bad_request(): TrbWebResponse {")
	g.indent++
	g.line(`return trb_web_response(400, new __trb_http.Headers([{ name: "content-type", value: "application/json; charset=utf-8" }]), new __trb_http.Body(new TextEncoder().encode("{\"error\":\"bad_request\"}")));`)
	g.indent--
	g.line("}")
	g.line("function trb_web_payload_too_large(): TrbWebResponse {")
	g.indent++
	g.line(`return trb_web_response(413, new __trb_http.Headers([{ name: "content-type", value: "application/json; charset=utf-8" }]), new __trb_http.Body(new TextEncoder().encode("{\"error\":\"payload_too_large\"}")));`)
	g.indent--
	g.line("}")
	g.line("function trb_web_valid_response(response: TrbWebResponse): boolean {")
	g.indent++
	g.line("if (!Number.isInteger(response.__trb_status) || response.__trb_status < 100 || response.__trb_status > 999) return false;")
	g.line(`return response.__trb_headers.entries().every((entry) => /^[!#$%&'*+\-.^_\x60|~0-9A-Za-z]+$/.test(entry.name) && !entry.value.includes("\r") && !entry.value.includes("\n"));`)
	g.indent--
	g.line("}")
}

func (g *generator) webNext() {
	g.line("class TrbWebNext {")
	g.indent++
	g.line("private called = false;")
	g.line("private readonly handler: (signal: AbortSignal | undefined, context: TrbWebContext) => Promise<TrbWebResponse>;")
	g.line("constructor(handler: (signal: AbortSignal | undefined, context: TrbWebContext) => Promise<TrbWebResponse>) {")
	g.indent++
	g.line("this.handler = handler;")
	g.indent--
	g.line("}")
	g.line("async call(signal: AbortSignal | undefined, context: TrbWebContext): Promise<TrbWebResponse> {")
	g.indent++
	g.line("if (this.called) {")
	g.indent++
	g.line(`throw new Error("trb/web Next.call may only be called once");`)
	g.indent--
	g.line("}")
	g.line("this.called = true;")
	g.line("return await this.handler(signal, context);")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
}

func typescriptWebRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func typescriptWebRouteConditions(segments []string) []string {
	lengthOperator := "==="
	if len(segments) > 0 && strings.HasPrefix(segments[len(segments)-1], "*") {
		lengthOperator = ">="
	}
	conditions := []string{"segments.length " + lengthOperator + " " + strconv.Itoa(len(segments))}
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			continue
		}
		conditions = append(conditions, "segments["+strconv.Itoa(index)+"] === "+strconv.Quote(segment))
	}
	return conditions
}

func (g *generator) webServer() {
	g.line("function trb_web_serve(config: TrbWebServerConfig) {")
	g.indent++
	g.line("trb_web_validate_server_config(config);")
	g.line("const server = createServer(async (incoming, writer) => {")
	g.indent++
	g.line("const request_controller = new AbortController();")
	g.line("incoming.once(\"aborted\", () => request_controller.abort());")
	g.line("writer.once(\"close\", () => request_controller.abort());")
	g.line("const header_entries: Array<{ name: string; value: string }> = [];")
	g.line("for (const [name, value] of Object.entries(incoming.headers)) {")
	g.indent++
	g.line("if (Array.isArray(value)) {")
	g.indent++
	g.line("for (const item of value) header_entries.push({ name, value: item });")
	g.indent--
	g.line("} else if (value !== undefined) {")
	g.indent++
	g.line("header_entries.push({ name, value });")
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("const chunks: Uint8Array[] = [];")
	g.line("let size = 0;")
	g.line("let body_too_large = false;")
	g.line("for await (const chunk of incoming) {")
	g.indent++
	g.line(`const bytes = typeof chunk === "string" ? new TextEncoder().encode(chunk) : new Uint8Array(chunk);`)
	g.line("size += bytes.byteLength;")
	g.line("if (size > config.body_limit_bytes) {")
	g.indent++
	g.line("body_too_large = true;")
	g.line("continue;")
	g.indent--
	g.line("}")
	g.line("chunks.push(bytes);")
	g.indent--
	g.line("}")
	g.line(`const target = incoming.url ?? "/";`)
	g.line(`const query_index = target.indexOf("?");`)
	g.line(`const path = query_index === -1 ? target : target.slice(0, query_index);`)
	g.line(`const query_string = query_index === -1 ? "" : target.slice(query_index + 1);`)
	g.line("let response: TrbWebResponse;")
	g.line("if (body_too_large) {")
	g.indent++
	g.line(`response = await trb_web_dispatch_protocol_response(request_controller.signal, trb_web_request(incoming.method ?? "GET", path, query_string, new __trb_http.Headers(header_entries), __trb_http.Body.empty()), trb_web_payload_too_large());`)
	g.indent--
	g.line("} else {")
	g.indent++
	g.line("const body = new Uint8Array(size);")
	g.line("let offset = 0;")
	g.line("for (const chunk of chunks) {")
	g.indent++
	g.line("body.set(chunk, offset);")
	g.line("offset += chunk.byteLength;")
	g.indent--
	g.line("}")
	g.line(`response = await trb_web_dispatch_with_body_limit(request_controller.signal, trb_web_request(incoming.method ?? "GET", path, query_string, new __trb_http.Headers(header_entries), new __trb_http.Body(body)), config.body_limit_bytes);`)
	g.indent--
	g.line("}")
	g.line("const response_headers = new Map<string, string[]>();")
	g.line("for (const entry of response.__trb_headers.entries()) {")
	g.indent++
	g.line("const name = entry.name.toLowerCase();")
	g.line("response_headers.set(name, [...(response_headers.get(name) ?? []), entry.value]);")
	g.indent--
	g.line("}")
	g.line("for (const [name, values] of response_headers) writer.setHeader(name, values);")
	g.line("writer.statusCode = response.__trb_status;")
	g.line("writer.end(response.__trb_body.bytes());")
	g.indent--
	g.line("});")
	g.line("const connections = new Set<Socket>();")
	g.line("server.on(\"connection\", (connection) => {")
	g.indent++
	g.line("connections.add(connection);")
	g.line("connection.once(\"close\", () => connections.delete(connection));")
	g.indent--
	g.line("});")
	g.line("let stopping = false;")
	g.line("const shutdown = () => {")
	g.indent++
	g.line("if (stopping) return;")
	g.line("stopping = true;")
	g.line("const force_shutdown = setTimeout(() => {")
	g.indent++
	g.line("for (const connection of connections) connection.destroy();")
	g.indent--
	g.line("}, config.shutdown_timeout_milliseconds);")
	g.line("server.close((error) => {")
	g.indent++
	g.line("clearTimeout(force_shutdown);")
	g.line("process.off(\"SIGINT\", shutdown);")
	g.line("process.off(\"SIGTERM\", shutdown);")
	g.line("if (error !== undefined) {")
	g.indent++
	g.line("console.error(error);")
	g.line("process.exitCode = 1;")
	g.indent--
	g.line("}")
	g.indent--
	g.line("});")
	g.indent--
	g.line("};")
	g.line("process.once(\"SIGINT\", shutdown);")
	g.line("process.once(\"SIGTERM\", shutdown);")
	g.line("server.listen(config.port, config.host);")
	g.indent--
	g.line("}")
	g.line("function trb_web_validate_server_config(config: TrbWebServerConfig) {")
	g.indent++
	g.line(`if (config.host.trim() === "") throw new Error("trb/web ServerConfig.host must not be empty");`)
	g.line(`if (!Number.isInteger(config.port) || config.port < 1 || config.port > 65535) throw new Error("trb/web ServerConfig.port must be between 1 and 65535");`)
	g.line(`if (!Number.isInteger(config.body_limit_bytes) || config.body_limit_bytes < 1) throw new Error("trb/web ServerConfig.body_limit_bytes must be greater than zero");`)
	g.line(`if (!Number.isInteger(config.shutdown_timeout_milliseconds) || config.shutdown_timeout_milliseconds < 0) throw new Error("trb/web ServerConfig.shutdown_timeout_milliseconds must not be negative");`)
	g.indent--
	g.line("}")
}
