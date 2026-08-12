package ruby

import (
	"sort"
	"strconv"
	"strings"

	"github.com/type-rb/type-rb/internal/ir"
	webintegration "github.com/type-rb/type-rb/internal/web"
)

func (g *generator) integrations(extensions []ir.Extension) {
	if g.orm != nil {
		g.ormRuntime()
	}
	if manifest := webintegration.ManifestFrom(extensions); manifest != nil {
		g.webDispatcher(manifest)
		g.webServer()
	}
}

func (g *generator) webDispatcher(manifest *webintegration.Manifest) {
	routes := manifest.Routes
	rootMiddlewares := webintegration.RootMiddlewares(manifest.Middlewares)
	modules := map[string]bool{}
	for _, route := range routes {
		modules[route.ModulePath] = true
	}
	for _, middleware := range manifest.Middlewares {
		modules[middleware.ModulePath] = true
	}
	modulePaths := make([]string, 0, len(modules))
	for modulePath := range modules {
		modulePaths = append(modulePaths, modulePath)
	}
	sort.Strings(modulePaths)
	for _, modulePath := range modulePaths {
		g.line("require_relative "+strconv.Quote(rubyImportPath(g.modulePath, modulePath)), "")
	}
	if len(modulePaths) > 0 {
		g.b.WriteByte('\n')
	}
	if len(manifest.Middlewares) > 0 {
		g.webNext()
	}
	g.webProtocolResponses()

	g.line("def trb_web_dispatch(request)", "")
	g.indent++
	g.line("trb_web_dispatch_with_body_limit(request, TRB_WEB_DEFAULT_MAX_BODY_BYTES)", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_dispatch_with_body_limit(request, max_body_bytes)", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line("request = trb_web_normalize_request(request)", "")
	g.line("normalized_path = trb_web_normalize_path(request.path)", "")
	g.line("return trb_web_dispatch_protocol_response(request, trb_web_bad_request) if normalized_path.nil?", "")
	g.line("request = Request.new(method: request.method, path: normalized_path, query_string: request.query_string, headers: request.headers, body: request.body)", "")
	g.line("context = Context.new(request: request, path_parameters: {})", "")
	g.line("handler = ->(middleware_context) { trb_web_dispatch_core(middleware_context.request, max_body_bytes) }", "")
	g.rubyWebMiddlewareChain(rootMiddlewares, "handler", "root_next_handler_", "")
	g.line("trb_web_finalize_response(request, handler.call(context))", "")
	g.indent--
	g.line("rescue StandardError", "")
	g.indent++
	g.line(`trb_web_finalize_response(request, trb_web_internal_server_error)`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_dispatch_protocol_response(request, protocol_response)", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line("request = trb_web_normalize_request(request)", "")
	g.line("normalized_path = trb_web_normalize_path(request.path)", "")
	g.line("request = Request.new(method: request.method, path: normalized_path, query_string: request.query_string, headers: request.headers, body: request.body) unless normalized_path.nil?", "")
	g.line("context = Context.new(request: request, path_parameters: {})", "")
	g.line("handler = ->(_middleware_context) { protocol_response }", "")
	g.rubyWebMiddlewareChain(rootMiddlewares, "handler", "protocol_next_handler_", "")
	g.line("trb_web_finalize_response(request, handler.call(context))", "")
	g.indent--
	g.line("rescue StandardError", "")
	g.indent++
	g.line(`trb_web_finalize_response(request, trb_web_internal_server_error)`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_dispatch_core(request, max_body_bytes)", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line("return trb_web_payload_too_large if request.body.bytes.bytesize > max_body_bytes", "")
	g.line("method = request.method.upcase", "")
	g.line(`segments = request.path == "/" ? [] : request.path.delete_prefix("/").split("/", -1)`, "")
	g.line("allowed_methods = []", "")
	g.line("explicit_head = false", "")
	for _, route := range routes {
		routeSegments := rubyWebRouteSegments(route.Path)
		condition := rubyWebRouteConditions(routeSegments)
		g.line("allowed_methods << "+strconv.Quote(route.Method)+" if "+strings.Join(condition, " && "), "")
		if route.Method == "GET" {
			g.line(`allowed_methods << "HEAD" if `+strings.Join(condition, " && "), "")
		}
		if route.Method == "HEAD" {
			g.line("explicit_head = true if "+strings.Join(condition, " && "), "")
		}
	}
	g.line(`allowed_methods << "OPTIONS" unless allowed_methods.empty?`, "")
	g.line("allowed_methods = allowed_methods.sort.uniq", "")
	g.line("dispatch_method = method", "")
	g.line(`dispatch_method = "GET" if method == "HEAD" && !explicit_head && allowed_methods.include?("GET")`, "")
	for routeIndex, route := range routes {
		segments := rubyWebRouteSegments(route.Path)
		condition := append([]string{"dispatch_method == " + strconv.Quote(route.Method)}, rubyWebRouteConditions(segments)...)
		g.line("if "+strings.Join(condition, " && "), "")
		g.indent++
		g.line("path_parameters = {}", "")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters["+strconv.Quote(strings.TrimPrefix(segment, ":"))+"] = segments["+strconv.Itoa(index)+"]", "")
			} else if strings.HasPrefix(segment, "*") {
				g.line("path_parameters["+strconv.Quote(strings.TrimPrefix(segment, "*"))+"] = segments["+strconv.Itoa(index)+"..].join(\"/\")", "")
			}
		}
		contextName := "route_context_" + strconv.Itoa(routeIndex)
		handlerName := "route_handler_" + strconv.Itoa(routeIndex)
		g.line(contextName+" = Context.new(request: request, path_parameters: path_parameters)", "")
		g.line(handlerName+" = ->(middleware_context) { "+route.TargetHandler+"(middleware_context) }", "")
		g.rubyWebMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "next_handler_"+strconv.Itoa(routeIndex)+"_", "")
		g.line("return trb_web_checked_response("+handlerName+".call("+contextName+"))", "")
		g.indent--
		g.line("end", "")
	}
	for routeIndex, route := range webintegration.UniquePathRoutes(routes) {
		segments := rubyWebRouteSegments(route.Path)
		condition := append([]string{`method == "OPTIONS"`}, rubyWebRouteConditions(segments)...)
		g.line("if "+strings.Join(condition, " && "), "")
		g.indent++
		g.line("path_parameters = {}", "")
		for index, segment := range segments {
			if strings.HasPrefix(segment, ":") {
				g.line("path_parameters["+strconv.Quote(strings.TrimPrefix(segment, ":"))+"] = segments["+strconv.Itoa(index)+"]", "")
			} else if strings.HasPrefix(segment, "*") {
				g.line("path_parameters["+strconv.Quote(strings.TrimPrefix(segment, "*"))+"] = segments["+strconv.Itoa(index)+"..].join(\"/\")", "")
			}
		}
		contextName := "options_context_" + strconv.Itoa(routeIndex)
		handlerName := "options_handler_" + strconv.Itoa(routeIndex)
		g.line(contextName+" = Context.new(request: request, path_parameters: path_parameters)", "")
		g.line(handlerName+` = ->(_middleware_context) { Response.new(status: 204, headers: Headers.new([Header.new(name: "allow", value: allowed_methods.join(", "))]), body: Body.empty) }`, "")
		g.rubyWebMiddlewareChain(webintegration.NestedMiddlewares(route.Middlewares), handlerName, "options_next_handler_"+strconv.Itoa(routeIndex)+"_", "")
		g.line("return trb_web_checked_response("+handlerName+".call("+contextName+"))", "")
		g.indent--
		g.line("end", "")
	}
	g.line("unless allowed_methods.empty?", "")
	g.indent++
	g.line(`return Response.new(status: 405, headers: Headers.new([Header.new(name: "allow", value: allowed_methods.join(", ")), Header.new(name: "content-type", value: "application/json; charset=utf-8")]), body: Body.new("{\"error\":\"method_not_allowed\"}".b))`, "")
	g.indent--
	g.line("end", "")
	g.line(`Response.new(status: 404, headers: Headers.new([Header.new(name: "content-type", value: "application/json; charset=utf-8")]), body: Body.new("{\"error\":\"not_found\"}".b))`, "")
	g.indent--
	g.line("rescue StandardError", "")
	g.indent++
	g.line(`trb_web_internal_server_error`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
}

func (g *generator) rubyWebMiddlewareChain(middlewares []webintegration.Middleware, handlerName, nextPrefix, suffix string) {
	for middlewareIndex := len(middlewares) - 1; middlewareIndex >= 0; middlewareIndex-- {
		middleware := middlewares[middlewareIndex]
		nextName := nextPrefix + strconv.Itoa(middlewareIndex)
		g.line(nextName+" = "+handlerName, suffix)
		g.line(handlerName+" = ->(middleware_context) { "+middleware.TargetHandler+"(middleware_context, TrbWebNext.new("+nextName+")) }", suffix)
	}
}

func (g *generator) webProtocolResponses() {
	g.line("TRB_WEB_DEFAULT_MAX_BODY_BYTES = "+strconv.Itoa(webintegration.MaxBodyBytes), "")
	g.line("", "")
	g.line("def trb_web_headers_from_hash(values)", "")
	g.indent++
	g.line("entries = []", "")
	g.line("values.each do |name, header_values|", "")
	g.indent++
	g.line("header_values.each { |value| entries << Header.new(name: name, value: value) }", "")
	g.indent--
	g.line("end", "")
	g.line("Headers.new(entries)", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_normalize_request(request)", "")
	g.indent++
	g.line("entries = []", "")
	g.line("request.headers.entries.each do |header|", "")
	g.indent++
	g.line("entries << Header.new(name: header.name.downcase, value: header.value)", "")
	g.indent--
	g.line("end", "")
	g.line("Request.new(method: request.method.upcase, path: request.path, query_string: request.query_string, headers: Headers.new(entries), body: request.body)", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_normalize_path(path)", "")
	g.indent++
	g.line(`return nil unless path.start_with?("/") && !path.include?("\\")`, "")
	g.line(`segments = path.split("/", -1)`, "")
	g.line("segments.each_with_index do |segment, index|", "")
	g.indent++
	g.line(`return nil if segment.match?(/%(?![0-9A-Fa-f]{2})/)`, "")
	g.line(`decoded = segment.b.gsub(/%([0-9A-Fa-f]{2})/) { [$1.to_i(16)].pack("C") }.force_encoding(Encoding::UTF_8)`, "")
	g.line(`return nil unless decoded.valid_encoding?`, "")
	g.line(`return nil if decoded == "." || decoded == ".." || decoded.include?("/") || decoded.include?("\\")`, "")
	g.line("segments[index] = decoded", "")
	g.indent--
	g.line("end", "")
	g.line(`segments.join("/")`, "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_internal_server_error", "")
	g.indent++
	g.line(`Response.new(status: 500, headers: Headers.new([Header.new(name: "content-type", value: "application/json; charset=utf-8")]), body: Body.new("{\"error\":\"internal_server_error\"}".b))`, "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_bad_request", "")
	g.indent++
	g.line(`Response.new(status: 400, headers: Headers.new([Header.new(name: "content-type", value: "application/json; charset=utf-8")]), body: Body.new("{\"error\":\"bad_request\"}".b))`, "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_payload_too_large", "")
	g.indent++
	g.line(`Response.new(status: 413, headers: Headers.new([Header.new(name: "content-type", value: "application/json; charset=utf-8")]), body: Body.new("{\"error\":\"payload_too_large\"}".b))`, "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_finalize_response(request, response)", "")
	g.indent++
	g.line("response = trb_web_internal_server_error unless trb_web_valid_response?(response)", "")
	g.line(`return response unless request.method.upcase == "HEAD"`, "")
	g.line(`Response.new(status: response.status, headers: response.headers, body: Body.empty)`, "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_checked_response(response)", "")
	g.indent++
	g.line("trb_web_valid_response?(response) ? response : trb_web_internal_server_error", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_valid_response?(response)", "")
	g.indent++
	g.line("return false unless response.status.between?(100, 999)", "")
	g.line("return false unless response.headers && response.body", "")
	g.line(`response.headers.entries.all? do |header|`, "")
	g.indent++
	g.line(`header.name.match?(/\A[!#$%&'*+\-.^_\x60|~0-9A-Za-z]+\z/) && !header.value.include?("\r") && !header.value.include?("\n")`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
}

func (g *generator) webNext() {
	g.line("class TrbWebNext", "")
	g.indent++
	g.line("def initialize(handler)", "")
	g.indent++
	g.line("@handler = handler", "")
	g.line("@called = false", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def call(context)", "")
	g.indent++
	g.line(`raise "trb/web Next.call may only be called once" if @called`, "")
	g.line("@called = true", "")
	g.line("@handler.call(context)", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
}

func rubyWebRouteSegments(path string) []string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

func rubyWebRouteConditions(segments []string) []string {
	lengthOperator := "=="
	if len(segments) > 0 && strings.HasPrefix(segments[len(segments)-1], "*") {
		lengthOperator = ">="
	}
	conditions := []string{"segments.length " + lengthOperator + " " + strconv.Itoa(len(segments))}
	for index, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			continue
		}
		conditions = append(conditions, "segments["+strconv.Itoa(index)+"] == "+strconv.Quote(segment))
	}
	return conditions
}

func (g *generator) webServer() {
	g.line("", "")
	g.line("def trb_web_serve(config)", "")
	g.indent++
	g.line(`require "socket"`, "")
	g.line("server = nil", "")
	g.line("connections = {}", "")
	g.line("state_lock = Mutex.new", "")
	g.line("stopping = false", "")
	g.line("previous_signal_handlers = {}", "")
	g.line("trb_web_validate_server_config(config)", "")
	g.line(`server = TCPServer.new(config.host, config.port)`, "")
	g.line(`%w[INT TERM].each do |signal|`, "")
	g.indent++
	g.line("previous_signal_handlers[signal] = Signal.trap(signal) do", "")
	g.indent++
	g.line("stopping = true", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("until stopping", "")
	g.indent++
	g.line("ready = IO.select([server], nil, nil, 0.1)", "")
	g.line("next if ready.nil?", "")
	g.line("client = server.accept_nonblock(exception: false)", "")
	g.line("next if client == :wait_readable", "")
	g.line("state_lock.synchronize { connections[client] = nil }", "")
	g.line("worker = Thread.new(client) do |connection|", "")
	g.indent++
	g.line("begin", "")
	g.indent++
	g.line("loop do", "")
	g.indent++
	g.line(`request_line = connection.gets("\r\n")`, "")
	g.line("break if request_line.nil?", "")
	g.line("request_line = request_line.strip", "")
	g.line("next if request_line.empty?", "")
	g.line(`method, target, version = request_line.split(" ", 3)`, "")
	g.line("headers = {}", "")
	g.line(`while (line = connection.gets("\r\n"))`, "")
	g.indent++
	g.line(`line = line.delete_suffix("\r\n")`, "")
	g.line("break if line.empty?", "")
	g.line(`name, value = line.split(":", 2)`, "")
	g.line("next if value.nil?", "")
	g.line("key = name.downcase", "")
	g.line("(headers[key] ||= []) << value.strip", "")
	g.indent--
	g.line("end", "")
	g.line(`path, query_string = target.split("?", 2)`, "")
	g.line(`query_string ||= ""`, "")
	g.line(`content_length = (headers["content-length"]&.first || "0").to_i`, "")
	g.line("body_too_large = content_length > config.body_limit_bytes", "")
	g.line("if body_too_large", "")
	g.indent++
	g.line(`web_request = Request.new(method: method, path: path, query_string: query_string, headers: trb_web_headers_from_hash(headers), body: Body.empty)`, "")
	g.line("response = trb_web_dispatch_protocol_response(web_request, trb_web_payload_too_large)", "")
	g.indent--
	g.line("else", "")
	g.indent++
	g.line(`body = content_length.positive? ? connection.read(content_length) : "".b`, "")
	g.line("response = trb_web_dispatch_with_body_limit(Request.new(method: method, path: path, query_string: query_string, headers: trb_web_headers_from_hash(headers), body: Body.new(body)), config.body_limit_bytes)", "")
	g.indent--
	g.line("end", "")
	g.line(`reason = { 200 => "OK", 201 => "Created", 204 => "No Content", 400 => "Bad Request", 404 => "Not Found", 405 => "Method Not Allowed", 413 => "Content Too Large", 500 => "Internal Server Error" }[response.status] || "Response"`, "")
	g.line("response_headers = {}", "")
	g.line("response.headers.entries.each { |header| (response_headers[header.name] ||= []) << header.value }", "")
	g.line(`response_headers["content-length"] ||= [response.body.bytes.bytesize.to_s]`, "")
	g.line(`keep_alive = !body_too_large && version == "HTTP/1.1" && headers.fetch("connection", [""]).first.downcase != "close"`, "")
	g.line(`response_headers["connection"] ||= [keep_alive ? "keep-alive" : "close"]`, "")
	g.line(`connection.write("HTTP/1.1 #{response.status} #{reason}\r\n")`, "")
	g.line("response_headers.each do |name, values|", "")
	g.indent++
	g.line("values.each do |value|", "")
	g.indent++
	g.line(`connection.write("#{name}: #{value}\r\n")`, "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line(`connection.write("\r\n")`, "")
	g.line("connection.write(response.body.bytes)", "")
	g.line("break unless keep_alive", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("rescue StandardError", "")
	g.line("ensure", "")
	g.indent++
	g.line("connection.close", "")
	g.line("state_lock.synchronize { connections.delete(connection) }", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("end", "")
	g.line("state_lock.synchronize { connections[client] = worker if connections.key?(client) }", "")
	g.indent--
	g.line("end", "")
	g.indent--
	g.line("ensure", "")
	g.indent++
	g.line("server&.close", "")
	g.line("active_connections = state_lock.synchronize { connections.dup }", "")
	g.line("active_workers = active_connections.values.compact", "")
	g.line("deadline = Process.clock_gettime(Process::CLOCK_MONOTONIC) + (config.shutdown_timeout_milliseconds / 1000.0)", "")
	g.line("active_workers.each do |worker|", "")
	g.indent++
	g.line("remaining = deadline - Process.clock_gettime(Process::CLOCK_MONOTONIC)", "")
	g.line("break if remaining <= 0", "")
	g.line("worker.join(remaining)", "")
	g.indent--
	g.line("end", "")
	g.line("active_connections.each_key { |connection| connection.close unless connection.closed? }", "")
	g.line("active_workers.each do |worker|", "")
	g.indent++
	g.line("worker.kill if worker.alive?", "")
	g.line("worker.join", "")
	g.indent--
	g.line("end", "")
	g.line("previous_signal_handlers.each { |signal, handler| Signal.trap(signal, handler) }", "")
	g.indent--
	g.line("end", "")
	g.line("", "")
	g.line("def trb_web_validate_server_config(config)", "")
	g.indent++
	g.line(`raise ArgumentError, "trb/web ServerConfig.host must not be empty" if config.host.strip.empty?`, "")
	g.line(`raise ArgumentError, "trb/web ServerConfig.port must be between 1 and 65535" unless config.port.between?(1, 65_535)`, "")
	g.line(`raise ArgumentError, "trb/web ServerConfig.body_limit_bytes must be greater than zero" unless config.body_limit_bytes.positive?`, "")
	g.line(`raise ArgumentError, "trb/web ServerConfig.shutdown_timeout_milliseconds must not be negative" if config.shutdown_timeout_milliseconds.negative?`, "")
	g.indent--
	g.line("end", "")
}
