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
	g.webProtocolResponses()
	if len(manifest.Middlewares) > 0 {
		g.webNext()
	}
	g.line("function trb_web_dispatch(request: TrbWebRequest) {")
	g.indent++
	g.line("try {")
	g.indent++
	g.line("if (request.body.byteLength > TRB_WEB_MAX_BODY_BYTES) return trb_web_finalize_response(request, trb_web_payload_too_large());")
	g.line("const method = request.method.toUpperCase();")
	g.line(`const segments = request.path.split("/").filter((segment) => segment.length > 0);`)
	g.line("let allowed_methods: string[] = [];")
	g.line("let explicit_head = false;")
	for _, route := range routes {
		routeSegments := typescriptWebRouteSegments(route.Path)
		condition := []string{"segments.length === " + strconv.Itoa(len(routeSegments))}
		for segmentIndex, segment := range routeSegments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(segmentIndex)+"] === "+strconv.Quote(segment))
			}
		}
		g.line("if (" + strings.Join(condition, " && ") + ") allowed_methods.push(" + strconv.Quote(route.Method) + ");")
		if route.Method == "GET" {
			g.line("if (" + strings.Join(condition, " && ") + `) allowed_methods.push("HEAD");`)
		}
		if route.Method == "HEAD" {
			g.line("if (" + strings.Join(condition, " && ") + ") explicit_head = true;")
		}
	}
	g.line("allowed_methods = [...new Set(allowed_methods)].sort();")
	g.line("let dispatch_method = method;")
	g.line(`if (method === "HEAD" && !explicit_head && allowed_methods.includes("GET")) dispatch_method = "GET";`)
	for routeIndex, route := range routes {
		segments := typescriptWebRouteSegments(route.Path)
		condition := []string{"dispatch_method === " + strconv.Quote(route.Method), "segments.length === " + strconv.Itoa(len(segments))}
		for index, segment := range segments {
			if !strings.HasPrefix(segment, ":") {
				condition = append(condition, "segments["+strconv.Itoa(index)+"] === "+strconv.Quote(segment))
			}
		}
		g.line("if (" + strings.Join(condition, " && ") + ") {")
		g.indent++
		g.line("const path_parameters: Record<string, string> = {};")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters[" + strconv.Quote(strings.TrimPrefix(segment, ":")) + "] = segments[" + strconv.Itoa(index) + "]!;")
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
	g.line("if (allowed_methods.length > 0) {")
	g.indent++
	g.line(`return trb_web_finalize_response(request, { status: 405, headers: { "allow": [allowed_methods.join(", ")], "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"method_not_allowed\"}") });`)
	g.indent--
	g.line("}")
	g.line(`return trb_web_finalize_response(request, { status: 404, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"not_found\"}") });`)
	g.indent--
	g.line("} catch {")
	g.indent++
	g.line(`return trb_web_finalize_response(request, { status: 500, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"internal_server_error\"}") });`)
	g.indent--
	g.line("}")
	g.indent--
	g.line("}")
	g.line("function trb_web_finalize_response(request: TrbWebRequest, response: TrbWebResponse): TrbWebResponse {")
	g.indent++
	g.line(`if (request.method.toUpperCase() !== "HEAD") return response;`)
	g.line("return { ...response, body: new Uint8Array() };")
	g.indent--
	g.line("}")
}

func (g *generator) webProtocolResponses() {
	g.line("const TRB_WEB_MAX_BODY_BYTES = " + strconv.Itoa(webintegration.MaxBodyBytes) + ";")
	g.line("function trb_web_payload_too_large(): TrbWebResponse {")
	g.indent++
	g.line(`return { status: 413, headers: { "content-type": ["application/json; charset=utf-8"] }, body: new TextEncoder().encode("{\"error\":\"payload_too_large\"}") };`)
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

func (g *generator) webServer() {
	g.line("function trb_web_serve(port: number) {")
	g.indent++
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
	g.line("if (size > TRB_WEB_MAX_BODY_BYTES) {")
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
	g.line(`const url = new URL(incoming.url ?? "/", "http://localhost");`)
	g.line(`response = trb_web_dispatch({ method: incoming.method ?? "GET", path: url.pathname, query_string: url.search.slice(1), headers, body });`)
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
	g.line(`server.listen(port, "0.0.0.0");`)
	g.indent--
	g.line("}")
}
