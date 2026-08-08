package typescript

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrationImports(extensions []ir.Extension) {
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.line(`import { createServer } from "node:http";`)
		g.line(`import type { Socket } from "node:net";`)
		g.webRouteImports(manifest)
	}
}

func (g *generator) integrations(extensions []ir.Extension) {
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
	g.line("type TrbWebRequest = { method: string; path: string; query_string: string; headers: Record<string, string[]>; body: Uint8Array };")
	g.line("type TrbWebContext = { request: TrbWebRequest; path_parameters: Record<string, string> };")
	g.line("type TrbWebResponse = { status: number; headers: Record<string, string[]>; body: Uint8Array };")
	g.line("type TrbWebServerConfig = { host: string; port: number; body_limit_bytes: number; shutdown_timeout_milliseconds: number };")
	g.webProtocolResponses()
	if len(manifest.Middlewares) > 0 {
		g.webNext()
	}
	g.line("function trb_web_dispatch(request: TrbWebRequest) {")
	g.indent++
	g.line("return trb_web_dispatch_with_body_limit(request, TRB_WEB_DEFAULT_MAX_BODY_BYTES);")
	g.indent--
	g.line("}")
	g.line("function trb_web_dispatch_with_body_limit(request: TrbWebRequest, max_body_bytes: number) {")
	g.indent++
	g.line("try {")
	g.indent++
	g.line("request = trb_web_normalize_request(request);")
	g.line("const normalized_path = trb_web_normalize_path(request.path);")
	g.line("if (normalized_path === undefined) return trb_web_finalize_response(request, trb_web_bad_request());")
	g.line("request = { ...request, path: normalized_path };")
	g.line("if (request.body.byteLength > max_body_bytes) return trb_web_finalize_response(request, trb_web_payload_too_large());")
	g.line("const method = request.method.toUpperCase();")
	g.line(`const segments = request.path === "/" ? [] : request.path.slice(1).split("/");`)
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
		g.line("const " + contextName + ": TrbWebContext = { request, path_parameters };")
		g.line("let " + handlerName + " = (middleware_context: TrbWebContext): TrbWebResponse => " + route.TargetHandler + "(middleware_context);")
		for middlewareIndex := len(route.Middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			middleware := route.Middlewares[middlewareIndex]
			nextName := "next_handler_" + strconv.Itoa(routeIndex) + "_" + strconv.Itoa(middlewareIndex)
			g.line("const " + nextName + " = " + handlerName + ";")
			g.line(handlerName + " = (middleware_context: TrbWebContext): TrbWebResponse => " + middleware.TargetHandler + "(middleware_context, new TrbWebNext(" + nextName + "));")
		}
		g.line("return trb_web_finalize_response(request, " + handlerName + "(" + contextName + "));")
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
		g.line("const " + contextName + ": TrbWebContext = { request, path_parameters };")
		g.line("let " + handlerName + ` = (_middleware_context: TrbWebContext): TrbWebResponse => ({ status: 204, headers: { "allow": [allowed_methods.join(", ")] }, body: new Uint8Array() });`)
		for middlewareIndex := len(route.Middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
			middleware := route.Middlewares[middlewareIndex]
			nextName := "options_next_handler_" + strconv.Itoa(routeIndex) + "_" + strconv.Itoa(middlewareIndex)
			g.line("const " + nextName + " = " + handlerName + ";")
			g.line(handlerName + " = (middleware_context: TrbWebContext): TrbWebResponse => " + middleware.TargetHandler + "(middleware_context, new TrbWebNext(" + nextName + "));")
		}
		g.line("return trb_web_finalize_response(request, " + handlerName + "(" + contextName + "));")
		g.indent--
		g.line("}")
	}
	g.line("if (allowed_methods.length > 0) {")
	g.indent++
	g.line(`return trb_web_finalize_response(request, { status: 405, headers: { "allow": [allowed_methods.join(", ")], "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"method_not_allowed\"}") });`)
	g.indent--
	g.line("}")
	g.line(`return trb_web_finalize_response(request, { status: 404, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"not_found\"}") });`)
	g.indent--
	g.line("} catch {")
	g.indent++
	g.line(`return trb_web_finalize_response(request, trb_web_internal_server_error());`)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("function trb_web_finalize_response(request: TrbWebRequest, response: TrbWebResponse): TrbWebResponse {")
	g.indent++
	g.line("if (!trb_web_valid_response(response)) response = trb_web_internal_server_error();")
	g.line(`if (request.method.toUpperCase() !== "HEAD") return response;`)
	g.line("return { ...response, body: new Uint8Array() };")
	g.indent--
	g.line("}")
}

func (g *generator) webProtocolResponses() {
	g.line("const TRB_WEB_DEFAULT_MAX_BODY_BYTES = " + strconv.Itoa(webintegration.MaxBodyBytes) + ";")
	g.line("function trb_web_normalize_request(request: TrbWebRequest): TrbWebRequest {")
	g.indent++
	g.line("const headers: Record<string, string[]> = {};")
	g.line("for (const [name, values] of Object.entries(request.headers)) {")
	g.indent++
	g.line("const normalized_name = name.toLowerCase();")
	g.line("headers[normalized_name] = [...(headers[normalized_name] ?? []), ...values];")
	g.indent--
	g.line("}")
	g.line("return { ...request, method: request.method.toUpperCase(), headers };")
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
	g.line(`return { status: 500, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"internal_server_error\"}") };`)
	g.indent--
	g.line("}")
	g.line("function trb_web_bad_request(): TrbWebResponse {")
	g.indent++
	g.line(`return { status: 400, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"bad_request\"}") };`)
	g.indent--
	g.line("}")
	g.line("function trb_web_payload_too_large(): TrbWebResponse {")
	g.indent++
	g.line(`return { status: 413, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"payload_too_large\"}") };`)
	g.indent--
	g.line("}")
	g.line("function trb_web_valid_response(response: TrbWebResponse): boolean {")
	g.indent++
	g.line("if (!Number.isInteger(response.status) || response.status < 100 || response.status > 999) return false;")
	g.line(`return Object.entries(response.headers).every(([name, values]) => /^[!#$%&'*+\-.^_\x60|~0-9A-Za-z]+$/.test(name) && values.every((value) => !value.includes("\r") && !value.includes("\n")));`)
	g.indent--
	g.line("}")
}

func (g *generator) webNext() {
	g.line("class TrbWebNext {")
	g.indent++
	g.line("private called = false;")
	g.line("private readonly handler: (context: TrbWebContext) => TrbWebResponse;")
	g.line("constructor(handler: (context: TrbWebContext) => TrbWebResponse) {")
	g.indent++
	g.line("this.handler = handler;")
	g.indent--
	g.line("}")
	g.line("call(context: TrbWebContext): TrbWebResponse {")
	g.indent++
	g.line("if (this.called) {")
	g.indent++
	g.line(`throw new Error("trb/web Next.call may only be called once");`)
	g.indent--
	g.line("}")
	g.line("this.called = true;")
	g.line("return this.handler(context);")
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
	g.line("const headers: Record<string, string[]> = {};")
	g.line("for (const [name, value] of Object.entries(incoming.headers)) {")
	g.indent++
	g.line("if (Array.isArray(value)) {")
	g.indent++
	g.line("headers[name] = value;")
	g.indent--
	g.line("} else if (value !== undefined) {")
	g.indent++
	g.line("headers[name] = [value];")
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
	g.line("let response: TrbWebResponse;")
	g.line("if (body_too_large) {")
	g.indent++
	g.line("response = trb_web_payload_too_large();")
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
	g.line(`const target = incoming.url ?? "/";`)
	g.line(`const query_index = target.indexOf("?");`)
	g.line(`const path = query_index === -1 ? target : target.slice(0, query_index);`)
	g.line(`const query_string = query_index === -1 ? "" : target.slice(query_index + 1);`)
	g.line(`response = trb_web_dispatch_with_body_limit({ method: incoming.method ?? "GET", path, query_string, headers, body }, config.body_limit_bytes);`)
	g.indent--
	g.line("}")
	g.line("for (const [name, values] of Object.entries(response.headers)) {")
	g.indent++
	g.line("writer.setHeader(name, values);")
	g.indent--
	g.line("}")
	g.line("writer.statusCode = response.status;")
	g.line("writer.end(response.body);")
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
